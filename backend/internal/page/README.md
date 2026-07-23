# backend/internal/page — Page 领域服务

Page 身份与 Revision 发布的唯一权威写入入口（M1-T04 页面身份、M1-T05 发布事务、M1-T07 历史/Diff/回滚）。
API handler、Worker、Agent 不得绕过本包直接写 `page` / `revision` / `content_snapshot` 等权威表。

## 文件

| 文件 | 内容 |
|---|---|
| `types.go` | Page / Alias / Actor / Revision / ContentSnapshot / AuditEvent / OutboxEvent 类型与全部哨兵错误 |
| `title.go` | 标题规范化（NFC、大小写折叠、空白折叠、合法性校验） |
| `repository.go` | 手写 SQL 数据访问（ADR-0002），方法接收可 nil 的 `pgx.Tx` |
| `service.go` | 创建/查询/改名/别名解析/重定向，Actor 写权限校验 |
| `publish.go` | Revision 发布事务（`runPublishTx` 为 Publish 与 Rollback 共用的事务核心） |
| `history.go` | Revision 历史列表/详情、结构 Diff、回滚 |

## 发布事务（Service.Publish）

入参 `PublishParams{PageID, ActorID, ExpectedRevisionID, AST, Summary, IsMinor}`。

事务外的预校验：

1. `checkWriteActor`：Actor 存在、active、类型 ∈ {human, bot, system}（ai 直接发布拒绝）。
2. `buildSnapshot`：AST 必须是 `type="document"` 且 `schema_version=1` 的合法 AST v1
   （`ast.ValidateJSON` 按内嵌 JSON Schema 校验）；随后 `ast.CanonicalizeJSON`
   得到 canonical 字节，**服务端**计算 `content_hash = SHA-256(canonical)` 与
   `size_bytes = len(canonical)`——客户端传来的任何 hash/size 不被信任。

事务内（`TxManager.InTx`，单事务原子提交）：

1. `SELECT page ... FOR UPDATE` —— 行锁把同一页面的并发发布串行化；
2. 乐观锁断言 `expected_revision_id == page.current_revision_id`
   （首发布要求两者同时为 nil），不一致返回 `ErrStaleRevision`；
3. `INSERT content_snapshot`（canonical AST + hash + size）；
4. `INSERT revision`（`parent_revision_id` = 旧 current，visibility 默认 `public`）；
5. `UPDATE page.current_revision_id` —— DB 触发器 `page_current_revision_check`
   复核目标 Revision 属于本页面（不变量 INV-01 的兜底）；
6. `INSERT audit_event`（`revision.published`，payload 含
   page_id / revision_id / parent_revision_id / content_hash）；
7. `INSERT outbox_event`（`page.revision_published`，payload 同上加 schema_version，
   status `pending`、`next_attempt_at now()`，由 Worker 按 ADR-0003 领取消费）。

## 并发与失败语义

- **并发**：两个以相同 expected 并发发布的请求，行锁保证恰一个成功，
  另一个拿到 `ErrStaleRevision`（HTTP 409 `stale_revision`），不产生双写。
- **失败原子性**：任一步失败整体回滚——不留孤立 snapshot/revision/audit/outbox，
  current 指针不移动。集成测试用包内钩子 `beforePublishCommit`
  （publish.go，生产恒为 nil）在全部写入后、提交前注入错误验证回滚。
- **不可变性**：Revision 与 ContentSnapshot 发布后不可修改，
  由 000001 迁移的行级触发器拒绝 UPDATE/DELETE；正文修改只能产生新 Revision。

## 历史、Diff 与回滚（M1-T07，设计 §3.3/§4.6）

- `ListRevisions(ctx, pageID, cursor, limit)`：按 `(created_at DESC, id DESC)` 游标分页
  （索引 `revision_page_history_idx`；ADR-0008 不依赖 ID 时间序，游标显式携带
  created_at + id，base64url 编码的 `"unixnano:id"`，无法解析返回 `ErrInvalidCursor`）。
  每行冗余快照的 `content_hash`/`schema_version`，不取 AST 字节。
- `GetRevision(ctx, pageID, revisionID)`：单版详情（含快照 AST）。Revision 不存在
  或不属于该页面返回 `ErrRevisionNotFound`——跨页访问不泄露存在性。
- `DiffRevisions(ctx, pageID, fromID, toID)`：以 from 为 base、to 为 current 调
  `ast.Diff` 返回结构 Diff；from == to 返回空 changes。
- `Rollback(ctx, RollbackParams{PageID, TargetRevisionID, ActorID, Summary})`：
  **回滚不是修改旧 Revision**。读取目标旧版快照的 AST，复用发布事务核心
  `runPublishTx`（`useCurrentExpected`：以行锁内读到的 current 为基线）追加一个
  新 Revision——parent = 锁内 current，行锁把回滚与并发发布串行化，回滚本身不会
  过期；交错时携带旧基线的并发 Publish 收到 `ErrStaleRevision`。
  审计事件 `revision.rolled_back`（payload 含 `rolled_back_to`），Outbox 仍为
  `page.revision_published`（回滚后投影必须重建）。summary 缺省
  「回滚到 {target_revision_id}」，可由调用方覆盖。

### 决策：回滚的 ContentSnapshot 去重

回滚目标内容的 canonical hash 必然与目标版本相同。`runPublishTx` 以
`dedupSnapshot` 模式先按 `(content_hash, schema_version)` 查重
（索引 `content_snapshot_hash_idx`）：命中则**复用已有快照行**，不重复存储；
未命中才插入新行。Revision 永远新建（历史链完整），旧 Revision 与旧快照不动（INV-02）。
内容寻址复用是全局的（不限本页），hash 相同即 canonical 内容相同，语义安全。

## 页面生命周期事件（M3-T04，设计 §5.2/§16）

`CreatePage` 与 `RenamePage` 与页面写入**同事务**追加审计与 Outbox 事件
（`emitPageEvent`，写法与发布事务一致），驱动 projection 的未解析链接 Resolver：

| 操作 | event_type（audit 与 outbox 相同） | payload |
|---|---|---|
| `CreatePage` | `page.created` | `page_id` / `wiki_id` / `namespace_id` / `normalized_title` / `display_title` |
| `RenamePage`（规范化标题实际变化） | `page.renamed` | 同上 + `old_normalized_title` |

`RenamePage` 仅显示名变化（规范化标题不变）时不产别名、不发事件——标题占用无变化。
消费端语义（歧义保持 unresolved、不改权威 AST 等）见
`backend/internal/projection/README.md` 的「未解析链接 Resolver（M3-T04）」节。

## 稳定锚点与 BlockRedirect（M9-T01）

- 解析顺序保持为当前 slug、历史 alias、BlockRedirect 链、目标当前 slug；链最多 16 跳，
  环返回 `ErrBlockRedirectLoop`，悬空目标返回 `ErrAnchorNotFound`。
- `CreateBlockRedirect` 在事务内获取命名的 `pg_advisory_xact_lock`，把整张 redirect 图的
  环校验与 upsert 线性化。相反方向的 A→B 与 B→A 并发写入恰一成功，另一请求在看到
  已提交边后返回 `ErrBlockRedirectLoop`；更长链和既有 source 改写遵循相同边界。
- 锁仅覆盖 BlockRedirect 权威写事务并在提交或回滚时自动释放；匿名锚点解析不取锁，
  不改变现有读取语义。

## 错误码映射（cmd/api → 契约 Error 模型）

| 哨兵错误 | HTTP | code |
|---|---|---|
| `ErrInvalidTitle` / `ErrInvalidAST` / `ErrInvalidCursor` | 400 | `validation_failed` |
| `ErrInvalidActor` / `ErrActorNotAllowed` | 403 | `forbidden` |
| `ErrPageNotFound` / `ErrNamespaceNotFound` / `ErrRevisionNotFound` | 404 | `not_found` |
| `ErrTitleConflict` | 409 | `conflict` |
| `ErrStaleRevision` | 409 | `stale_revision` |
| `ErrRedirectLoop` / `ErrRedirectTooDeep` | 422 | `validation_failed` |
| 其他 | 500 | `internal` |

错误响应体统一由 `backend/internal/platform/httpx` 写出，
`request_id` 取自 RequestID 中间件写入的上下文（与响应头 `X-Request-ID` 一致）。

## 测试

- `service_test.go` / `title_test.go`：页面身份与重定向（M1-T04），
  以及 page.created / page.renamed 的审计与 Outbox 事件 payload（M3-T04）。
- `publish_test.go`：首发布→二次发布、事件 payload、陈旧基线、并发双发布
  （barrier 同时放行）、非法 AST、Actor 规则、页面不存在。
- `publish_internal_test.go`：提交前注入失败的回滚原子性。
- `history_test.go`：历史顺序与游标分页、单版详情与跨页 404、结构 Diff、
  回滚（新 Revision + 快照去重复用 + rolled_back 审计 + INV-02 未改动断言）、
  回滚与并发发布交错的陈旧基线拒绝。
- 均需 `TEST_DATABASE_URL`（未设置时 testkit 自动 skip）。
