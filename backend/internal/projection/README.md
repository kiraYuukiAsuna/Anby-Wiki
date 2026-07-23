# projection — Outbox 消费框架（M3-T01）

依据：ADR-0003、设计 §15（Projection 与自动重建）/§16（事件与最终一致性）。
本包提供 `outbox_event` 表的可靠消费框架；M3-T02 的 Projection Builder 与
M3-T03+ 的各投影（链接/渲染/搜索）都作为本框架的 Handler 接入。

## 投递语义：至少一次

事件在 Handler 返回成功**之前**，可能因以下原因被重复投递：

- Worker 进程在 Handler 执行中或标记 `done` 前崩溃（租约过期后被重新领取）；
- 租约到期但 Handler 仍在执行（长任务未续租），事件被其他 Consumer 抢走。

框架不保证恰好一次。**Handler 必须幂等**。

## Handler 幂等契约

- 每个 `Event` 携带 `IdempotencyKey`（= 事件 ID 的字符串形式，事件本身唯一），
  Handler 以它为去重键。本包提供 `ProcessedKeys`（内存去重器）供测试与
  单进程场景使用；跨进程/重启场景的幂等应落在存储层（投影表唯一约束 +
  UPSERT，按 `IdempotencyKey` 或 `(projection, source_revision_id)` 去重）。
- 版本防护（设计 §15）：投影 Handler 更新前必须确认事件的 `revision_id`
  仍是页面当前 Revision，旧 Revision 触发的事件不得覆盖新投影——
  由 M3-T02 的 Projection Builder 统一落实。

## 领取与租约

单事务内完成领取并提交：

```sql
SELECT ... FROM outbox_event
WHERE status IN ('pending','claimed')
  AND next_attempt_at <= now()
  AND (status = 'pending' OR claimed_at < now() - $lease)
ORDER BY created_at
LIMIT $batch FOR UPDATE SKIP LOCKED;
-- 同事务：
UPDATE outbox_event SET status='claimed', claimed_at=now(),
       attempt_count=attempt_count+1 WHERE id = ANY($ids);
```

- `FOR UPDATE SKIP LOCKED` 保证多 Consumer 并发时同行只被一个领取。
- 领取事务提交后 Handler 在事务外执行；进程崩溃留下 `claimed` 行，
  `claimed_at < now() - LeaseDuration` 后可被任意 Consumer 重新领取（崩溃恢复）。
- 长任务 Handler 应周期调用 `Consumer.RenewLease(ctx, eventID)` 刷新
  `claimed_at`（间隔明显小于 `LeaseDuration`），防止被抢占。

## 配置与退避参数

| 配置 | 默认 | 说明 |
|---|---|---|
| `BatchSize` | 20 | 单次领取事件数 |
| `PollInterval` | 500ms | 无事件时轮询间隔 |
| `LeaseDuration` | 30s | 领取租约（崩溃恢复窗口） |
| `MaxAttempts` | 10 | 最大尝试次数（含首次），达到后死信 |
| `BackoffBase` | 1s | 指数退避基数 |
| `ShutdownTimeout` | 10s | 优雅关闭等待在途事件的上限 |

第 `n` 次失败后的退避：`min(BackoffBase * 2^(n-1), 5min)`，再加 ±10% jitter。
失败时事件置回 `pending` 并写 `last_error`/`next_attempt_at`；
`attempt_count >= MaxAttempts` 时置 `dead`。

## 死信处理流程

`status='dead'` 的事件不再被自动领取，框架记 Error 级日志
（event_id、event_type、attempt_count、last_error）。处置方式：

1. 监控死信数量与日志告警（M3-T07 `-metrics` 与常驻结构化指标日志）；
2. 人工定位失败原因并修复（投影代码 bug、数据异常等）；
3. 修复后执行 `go run ./cmd/worker -replay-dead`。命令把全部 dead 行的
   status/attempt/next_attempt_at 重置为可立即领取的 pending；Builder 仍须幂等。

## 优雅关闭

`Run(ctx)` 在 ctx 取消后停止领取新批次；在途事件继续处理，
最长等待 `ShutdownTimeout`，超时强制取消 Handler 的 ctx
（未完成事件保持 `claimed`，租约过期后被重新领取，不丢事件）。

---

# projection — Projection 通用框架（M3-T02）

依据：设计 §15（每个投影必须记录来源 Revision；旧 Revision 任务不得覆盖新投影）、
§16（投影失败必须记录状态、支持重试、支持按 Page 重建）。
本框架在 M3-T01 消费框架之上提供 Builder 契约、版本防护、状态簿记与重建命令；
具体投影（PageLinkProjection 等）在 M3-T03+ 作为 Builder 注册。

## Builder 契约

```go
type Builder interface {
    Type() string                                              // 投影类型标识，如 "page_links"
    Rebuild(ctx, tx, pageID, revisionID) error                 // 从权威 AST 重建该页本投影
    HandleEvent(ctx, event) error                              // 处理一条 Outbox 事件
}
```

- `Rebuild` 在调用方给定的 `tx` 内执行：`revisionID` 是页面当前 Revision，
  权威 AST 用 `RevisionAST(ctx, tx, revisionID)` 在同一事务内读取；
  返回 error 则整个重建事务回滚。
- `HandleEvent` 的通用实现可直接委托 `HandleRebuildEvent(ctx, pool, b, pageID)`
  （单事务读当前 Revision → Rebuild → 提交）。
- Builder 必须幂等：同一 `(pageID, revisionID)` 的 Rebuild 可重复执行。
- Builder 经 `Registry` 注册（`reg.Register(b)`，nil/重复 Type panic），
  cmd/worker 启动时装配；事件分发与重建命令都经注册表枚举。

## 版本防护语义（设计 §15，INV-04）

`WithVersionGuard(b, pool, logger)` 装饰 Builder 后处理
`page.revision_published` 事件时：

1. 解析 payload 的 `revision_id`，**处理前**断言它仍是
   `page.current_revision_id`；
2. 内层处理完成后**再断言一次**（处理期间页面可能又发布了新 Revision）。

任一断言失败即「跳过」：记 info 日志，事件视为处理成功（done）——
旧 Revision 任务不重试循环，也不会覆盖新投影；新 Revision 自己的事件
（或 `-rebuild-page`）负责把投影推进到最新。被跳过的 Builder 不写
`projection_state`。不带 `revision_id` 的事件直接透传给内层 Builder。

cmd/worker 用 `NewRevisionPublishedHandler(pool, registry, logger)` 把
`page.revision_published` 分发给注册表中全部 Builder（每个带版本防护）：
任一 Builder 失败 → 该 Builder 记 error 状态、事件进入退避重试
（已成功的 Builder 靠幂等承受重放）。

## projection_state 状态簿记

迁移 000005 建表，主键 `(aggregate_type, aggregate_id, projection_type)`，
每行记录 `source_revision_id`（FK → revision）、`status`（`ok`/`error`）、
`projected_at`、`last_error`。写入入口 `UpsertState(ctx, q, state)`
（`q` 可为 pool 或 tx）+ `OKState`/`ErrorState` 构造器：

- 事件路径：分发器在 Builder 事务外 best-effort 簿记（落库失败只记 warn）；
- 重建路径：`RebuildPage` 在重建事务内簿记，与投影写入同生共死。

## 重建命令（投影可丢弃、可重建）

```bash
# 重建指定页面的全部已注册投影（未发布页面跳过；页面不存在退出码 1）
go run ./cmd/worker -rebuild-page <page-uuid>

# 全量重放：分页扫描全部活页面逐页重建，单页失败收集进报告继续，
# 有失败时退出码 1
go run ./cmd/worker -rebuild-all
```

两个命令执行完即退出，不进入常驻消费循环。程序级 API：
`Rebuilder.RebuildPage(ctx, pageID) (rebuilt bool, err error)`
（未发布返回 `(false, nil)` 并记 info 日志）与
`Rebuilder.RebuildAll(ctx) (*RebuildReport, error)`
（报告含 Total/Rebuilt/Skipped/Failed 与逐页失败明细）。

---

# projection — 页面链接与目录投影（M3-T03）

依据：设计 §18.9（页面关系投影）、§9/§18.1（章节锚点）、§17.3（关系查询走投影表）。
迁移 000006 建表；两个 Builder 在 cmd/worker `registerBuilders` 注册，
事件路径（page.revision_published）与 RebuildPage/RebuildAll 共用同一份 Rebuild 实现。

## PageLinksBuilder（type `page_links`）

Walk AST 收集全部 `page_reference` 行内节点写入 `page_link_projection`
（`external_link` 不在本 Builder，属 M3-T06）：

- **已解析引用**（设计 §5.1）：`resolution_status='resolved'`，`target_page_id`
  来自 AST，`target_heading_block_id`（如有）落 `target_anchor_block_id`，
  `display_text` 取节点的 `display_text`；
- **未解析引用**（设计 §5.2）：`resolution_status='unresolved'`，
  `target_namespace_id` 由 AST 的 `target_namespace`（namespace key）在落库时
  经子查询解析（页面所属 wiki 内的 namespace，解析不了落 NULL），
  `target_title` 记录 `normalized_title` 原文，`display_text` 同样取
  `normalized_title`（未解析节点无 `display_text` 字段）。

定位键：`source_block_id` 为节点所属 Block ID；`source_node_id` 为行内节点在
Block content 内的序号路径字符串——v1 行内层是扁平数组，即节点下标的十进制形式。

写入策略：tx 内 `DELETE WHERE source_page_id` 全删重插（幂等；投影可丢弃可重建）。

## OutlineBuilder（type `document_outline`）

按文档顺序遍历 heading Block，同事务写两张表（行同构，`page_anchor` 多 `current_slug`）：

- `document_outline_projection`：阅读页 TOC 数据源；
- `page_anchor`：章节锚点（设计 §9：`page_id + heading_block_id` 是权威身份，
  slug 只是展示地址）。

推导规则（`buildOutlineRows`，纯计算）：

- **parent**：level 栈推导——弹栈至栈顶 level < 当前 level 后的栈顶即父；
  跳级（如 H1 后直出 H3）时父为最近更低层级 heading（H1）；
- **position_key**：同级 1 起编号的序号路径（`'1.2.3'`），栈深即路径深度，
  同级再次出现时在原计数上累加（H1→H2→H2 得 `1`、`1.1`、`1.2`）；
- **title**：heading 纯文本——text/inline_code 取 `text`，page_reference 取
  `display_text`（未解析取 `normalized_title`），external_link 取 `display_text`。

写入策略：tx 内两表均 `DELETE WHERE page_id` 全删重插（幂等）。

## 锚点 slug 规则（`slug.go`，确定性纯函数）

`anchorSlug(title)`：

1. Unicode 小写折叠（逐 rune `unicode.ToLower`）；
2. 空白与连字符统一折叠为一个连字符（不产生前导/尾随连字符）；
3. 保留 Unicode 字母（`unicode.IsLetter`，CJK 等按原样保留、不转拼音）与数字，
   其余字符（标点、符号、下划线等）去除；
4. 结果为空时回落 `section`。

`slugAssigner` 同页去重：按文档顺序分配，重复基础 slug 依次加 `-2`/`-3` 后缀
（含与「基础 slug 恰好带数字后缀」的标题碰撞的避让）。同一文档同一遍历顺序
产出同一组 slug。

## 投影查询 API（`query.go` + cmd/api，设计 §17.3）

匿名可读（tag `projection`），走投影表不扫 AST，内容与投影最终一致：

- `GET /api/v1/pages/{id}/backlinks?cursor=&page_size=`：指向该页的已解析引用
  来源（来源页 id/标题 + source_block_id + display_text），
  按 `(source_page_id, source_block_id, source_node_id)` 升序游标分页；
- `GET /api/v1/pages/{id}/outline`：document_outline JOIN page_anchor，
  含 slug 与 position_key，按 position_key 数值序返回，供阅读页 TOC。

页面不存在 404、软删除 410、非法游标 400（与阅读端点语义一致）。

---

# projection — 未解析链接 Resolver（M3-T04）

依据：设计 §5.2（后台 Resolver 在新页面创建后尝试自动解析未解析页面引用）、
§15（旧 Revision 任务不得覆盖新投影）。实现在 `resolver.go`。

## 原则：只影响投影，不改权威 AST

Resolver 的自动解析**只更新 `page_link_projection`**（展示层）：命中后把
`resolution_status` 置 `resolved` 并写 `target_page_id`。
ContentSnapshot / AST 中的引用保持未解析形态，直到人工或 Proposal 修改——
投影可丢弃可重建，权威内容不经投影反向修改（设计 §3.1）。

## 触发点

1. **`page.created` / `page.renamed` 事件**（page 领域在创建/改名事务内写
   outbox，payload 含 `namespace_id` 与 `normalized_title`）：
   按 `(target_namespace_id, target_title = normalized_title)` 查
   `page_link_projection` 中的 `unresolved` 行尝试解析。
   改名只对新标题触发——`target_page_id` 解析与标题无关（Page ID 稳定），
   旧标题上已 resolved 的投影行无需动作。
2. **`page.revision_published` 路径的 hook**（`WrapPublishedHandler`）：
   Builder 分发成功后对该页 current Revision 投影中的全部未解析行再跑一遍
   `ResolvePageLinks(ctx, pageID)`。

## 解析语义

- **候选** = 该 namespace 下 `normalized_title` 命中的活页面
  **加上** 命中别名（`page_alias`）指向的活页面（UNION 去重）。
- **恰好一个候选** → UPDATE 匹配的 unresolved 行：
  `resolution_status='resolved', target_page_id=候选`。
- **多个候选（歧义）** → 保持 `unresolved`，记 info 日志（含候选数），
  不做猜测性解析；消歧留给人工/Proposal。
- **版本防护（设计 §15）**：只更新 `source_revision_id` 仍是 source 页面
  `current_revision_id` 的行——旧 Revision 的投影不被迟到的生命周期事件更新，
  其去留由新 Revision 自己的 published 事件（重建）负责。
- **幂等**：候选匹配与 UPDATE 都是现状驱动（不依赖事件负载之外的状态），
  同一事件重复投递结果一致。

## 决策：乱序兜底走 published hook

`page.created` 可能先于引用所在 Revision 的 `page.revision_published` 被消费
（created 处理时投影行尚不存在，属正常 no-op）。为让两种到达顺序都最终一致，
published 分发成功后固定跑一次 `ResolvePageLinks`：对该页 current Revision
投影中的每个未解析 `(namespace, title)` 重新执行候选匹配。代价是每次发布多
一次小查询，换来不依赖事件顺序的最终一致性。

## Worker 注册（cmd/worker）

```go
resolver := projection.NewLinkResolver(pool, logger)
consumer.Register(page.OutboxEventPageCreated, resolver)
consumer.Register(page.OutboxEventPageRenamed, resolver)
consumer.Register(page.OutboxEventRevisionPublished,
    resolver.WrapPublishedHandler(projection.NewRevisionPublishedHandler(pool, registry, logger)))
```

## 测试（`resolver_test.go`，真实 PostgreSQL）

创建/改名触发解析（AST 原样断言）、同名页面+别名歧义保持 unresolved、
created 先于 published 的乱序兜底、v2 发布后旧 Revision 投影的版本防护、
同一事件重复投递幂等、非法 payload 返回错误进入退避重试。

---

# projection — RenderedPage 渲染投影（M3-T05）

依据：设计 §15（投影记录来源 Revision）、§17.1（阅读路径：
CDN → RenderedPage Cache → RenderedPage Storage → PostgreSQL ContentSnapshot）。
迁移 000008 建 `rendered_page` 表；实现在 `builder_render.go`。

## RenderedPageBuilder（type `rendered_page`）

Rebuild：tx 内 `RevisionAST` 读当前 Revision 的权威 AST → `render.RenderHTML`
生成安全 HTML 片段 → `INSERT ... ON CONFLICT (page_id) DO UPDATE` 覆盖写
（每页一行，`renderer_version = render.RendererVersion`，`content_hash =
ast.ContentHash(doc)`，与 content_snapshot 的哈希同源）。

- **优先级（阅读路径）**：`cmd/api/reading.go` 先查投影
  （`page.Service.RenderedHTML`），命中直接返回投影 HTML，不再解析/渲染 AST。
- **兜底语义（受控降级）**：以下任一不满足即未命中，降级为从 ContentSnapshot
  实时渲染（M1-T06 现状行为，响应结构不变）：
  1. 投影行缺失（Worker 尚未处理、行被清掉）；
  2. 投影 `revision_id` ≠ 页面当前 Revision（版本防护：绝不返回旧 html）；
  3. 投影 `renderer_version` ≠ 当前 `render.RendererVersion`。
- **renderer_version 升级行为**：渲染规则升版后，全部旧行因条件 3 自然未命中，
  阅读路径自动降级实时渲染，无脏读窗口；投影由后续发布事件逐页更新，或
  `-rebuild-all` 全量重建（Rebuild 总是以当前 Revision 覆盖写）。
- **覆盖与版本防护**：Rebuild 直调用给定 Revision 直接覆盖；事件侧
  「旧 Revision 任务不得覆盖新渲染」由 M3-T02 `WithVersionGuard`
  处理前后断言保证（事件分发路径），RebuildPage 则锁内读当前 Revision。
- **幂等**：同一 `(pageID, revisionID)` 重复 Rebuild 产出同一行
  （`created_at` 刷新但内容列不变），承受至少一次投递。

## 测试（`builder_render_test.go` + `cmd/api/reading_rendered_test.go`）

Builder 侧：投影行 html 与实时渲染一致、revision/renderer_version/content_hash
与权威一致、重发新版覆盖更新、删行后 RebuildPage 恢复（INV-03）、重复事件幂等。
API 侧：SQL 篡改投影 html 为标记串断言命中走投影（by-title/by-id）、删行后实时
兜底、投影 Revision 落后 current 时兜底、renderer_version 不匹配时兜底。

---

# projection — 外链使用投影（M3-T06）

依据：设计 §8（普通外链与引用来源复用 ExternalResource）、§15（投影可重建）、
§18.9（页面关系投影）。迁移 000009 建 `external_link_usage`，实现在
`builder_links_ext.go`，Builder type 为 `external_links`。

## 构建语义

Builder Walk 当前 Revision AST 的全部 `external_link`：

1. 经 Evidence 领域唯一的 `NormalizeURL` 规范化；
2. 经 `evidence.ExternalResourceService.UpsertInTx` 按 `normalized_url` 幂等
   upsert `external_resource`，`original_url` 保留首次出现原文；
3. 同一事务内 `DELETE WHERE page_id` 后重插 usage，`block_id` 为所属 Block ID，
   `node_id` 为块内 inline 下标的十进制字符串，`link_role='inline'`。

不合法或非 http/https 的单个 URL 记 warn 并跳过，不阻断该页其他合法链接投影；
其他数据库/领域错误则回滚整页 Builder。重复事件、按页重建和全量重建均复用同一
幂等实现，仍受 M3-T02 Revision 版本防护约束。

## 资源生命周期与 INV-12

usage 是可丢弃投影，`external_resource` 则是 Source 与普通外链共享的权威资源：
页面不再引用某 URL 时只删除 usage，资源行保留，不做投影反向清理。

健康检查通过 `evidence.UpdateExternalResourceStatus` 只更新资源自身的 status、
HTTP 状态、hash、canonical/redirect 与检查时间；它不写 Page、Revision 或 Outbox，
因此资源可用性变化不会伪造正文版本。集成测试
`TestUpdateExternalResourceStatusDoesNotCreateRevision` 固化 INV-12。

---

# projection — 运维与一致性工具（M3-T07）

实现在 `ops.go` 与 `cmd/worker`。所有数据直接来自 `outbox_event`、
`projection_state` 与 Page Current，不另建第二套运维状态。

## 指标

常驻 Worker 启动后立即采集，之后每 30 秒输出一次结构化日志；也可单次执行：

```bash
go run ./cmd/worker -metrics
```

指标字段：`outbox_backlog/pending/claimed/retrying/dead`、
`outbox_oldest_backlog_seconds`、`projection_error_states`、
`projection_stale_states`。积压为 pending+claimed；延迟取最老积压事件的年龄；
stale 表示状态来源 Revision 已非 Page Current（或 Page 已删除/不存在）。

## 一致性抽检

```bash
go run ./cmd/worker -check-consistency -sample-size 100
```

对稳定排序后的最多 N 个活跃已发布页面，逐一检查所有已注册 Builder 的
`projection_state`：缺失、status=error、source_revision_id 落后 Current 都记问题；
有问题退出码 1，可据 page_id + projection_type 执行 `-rebuild-page` 修复。
内容级等价由各 Builder 的 INV-03 重建测试保障，抽检负责发现运行态控制面漂移。

## 故障注入与整体恢复门禁

- `TestFaultInjectionDelayedAndDuplicateMessages`：新版消息重复两次后旧版延迟到达，
  最终投影仍唯一且指向 Current；
- `TestM3AllPageProjectionsRebuild`：删除同页 page links、outline/anchor、rendered page、
  external links 与全部 state 后，按页重建得到等价快照；
- `TestProjectionStateOKAndError`：Builder 失败只写 error 状态，不回滚已发布 Revision。

---

# projection — 知识与证据使用投影（M4-T07）

迁移 000010 建 `entity_mention_projection`、`claim_usage`、`citation_usage`；
`builder_knowledge.go` 将三类 AST inline 引用分别注册成独立 Builder：
`entity_mentions`、`claim_usage`、`citation_usage`。每个 Builder 都按 page_id
全删重插并由 M3 版本防护保证只反映 Current Revision。

匿名反向查询 API：

- `GET /api/v1/entities/{id}/mentions`
- `GET /api/v1/claims/{id}/usages`
- `GET /api/v1/citations/{id}/usages`

三者按 `(page_id, block_id, node_id)` 游标分页，每条返回页面 ID/标题、
`revision_id`、`block_id`、`node_id`；Entity 额外返回 mention_text，Citation
保留可空 claim_id。查询 SQL 只读投影表并要求投影 revision 等于 Page Current，
不解析或扫描 AST JSON；投影缺失时返回空列表而不是同步回源。

测试：`builder_knowledge_test.go` 覆盖构建、重复重建、重发删旧行、反向查询与
“删投影后返回空（证明不扫 AST）”；`cmd/api/projection_test.go` 覆盖真实 HTTP
分页、Revision/Block/Node 定位与 400/404。

M4 里程碑验收由 `cmd/api/evidence_chain_test.go` 串起 Page Current AST、
ClaimUsage、ClaimSource 与 Citation/SourceChunk，并证明 Supersede 后旧投影与证据链
仍可按稳定 ID 审计。

---

# projection — Component 信息框依赖（M9-T03）

迁移 000019 建 `component_dependency`。`ComponentDependencyBuilder` 从 Current
Revision 的 ComponentBlock 写入冻结组件版本与 Entity 依赖；`ClaimUsageBuilder`
同时把信息框当前实际展示的 published Claim 写入 `claim_usage`。

Knowledge Service 在 Claim 创建、替代、状态/验证状态及来源变化的同一事务写入
`claim.changed`。Worker 只查询当前 Revision 的 `claim_usage` 与
`component_dependency`，按页锁定后重建依赖、ClaimUsage 和 RenderedPage；不扫描
AST JSON，不创建 Revision，重复事件幂等。

# projection — 搜索投影（M7-T01）

迁移 000014 建 `search_document`；`SearchBuilder` 从 Current Revision AST 提取正文，
并汇总 Page 标题、旧别名、语言、命名空间及主 Entity 的标签/描述/别名。投影行记录
`source_revision_id`，发布事件走通用 `WithVersionGuard`，创建/改名事件刷新元数据。

`internal/search.SearchAdapter` 是 API 与具体搜索产品之间的窄接口。P0 的
`PostgresAdapter` 使用加权 `tsvector`、`websearch_to_tsquery` 和 GIN；字段过滤支持
title/alias/body/entity，元数据过滤支持 namespace/language/entity_type。中文正文与
短词在 FTS 外提供转义后的字面匹配兜底。高亮只返回 `[[...]]` 标记的纯文本，Web
拆成 React 文本节点渲染，不信任或注入 HTML。

全量重建开始前 Search Builder 清空搜索文档，再由 `RebuildAll` 扫描全部活页面的
Current Revision 重建，避免软删除页或历史遗留文档残留。查询端只读搜索投影，不实时
扫描 AST；投影积压、错误和陈旧状态沿用 Outbox 与 `projection_state` 运维口径。

测试：`internal/search/postgres_test.go` 覆盖标题/别名/中英文正文/Entity、过滤、
高亮、幂等更新与替换重建；`builder_search_test.go` 覆盖 AST/元数据构建和来源
Revision；`cmd/api/search_test.go` 覆盖 HTTP 参数、分页、错误与响应契约。

## 内部 Beta Meilisearch（M7-T11）

`search_document` 继续是可丢弃、可重建的 Adapter-neutral staging。SearchBuilder
只在 Builder 数据库事务内更新 staging；事务提交后才同步 Meilisearch。远端同步
开启新事务并 `SELECT page ... FOR UPDATE`，确认 staging 的
`source_revision_id` 仍等于 Page Current 后发 HTTP 写入，并等待 Meilisearch task
进入 `succeeded` 才释放行锁。发布事务更新同一 Page 行，因此新 Revision 不可能在
最终检查和旧远端写之间插入；HTTP/task 失败返回 Outbox 重试，重复 `page_id` 写入
幂等。旧事件晚到时只会 no-op，不会回退远端索引。

显式按页/全量重建在 staging 事务提交后执行同一 post-commit 同步；全量重建先清空
staging 与远端索引，再逐页恢复。API 与 Worker 由 `SEARCH_BACKEND` 装配：
development 可用 `postgres`，production 强制 `meilisearch`。Meili key 只进入
Authorization header，不写日志、错误消息或索引文档。
