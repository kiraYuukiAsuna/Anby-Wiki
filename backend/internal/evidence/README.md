# evidence 模块（M4-T04/T05）

Evidence 领域服务：媒体资产（Asset）上传、外部资源（ExternalResource）查重、
来源（Source/SourceVersion/SourceChunk）登记与证据引用（Citation）。
表结构见 `backend/migrations/000001_initial_schema.up.sql`（设计 §7.1-7.5、§18.5）。

## 范围

- **asset 域（M4-T04）**：`Service.StoreAsset` 是 asset / asset_revision 的唯一权威写入入口。
- **source/citation 域（M4-T05）**：`Service.UpsertExternalResource` / `CreateSource` /
  `AddSourceVersion` / `CreateCitation` 是 external_resource / source /
  source_version / source_chunk / citation 的唯一权威写入入口（`source_service.go`）。

## 存储布局（ADR-0004）

对象键内容寻址：`{env}/asset/{content_hash前2位}/{content_hash}`，
由 `platform/storage.ContentKey` 构造，`content_hash` 为内容的小写 hex SHA-256。
`StoreAsset` 流式计算摘要（`io.TeeReader`，内容只读一遍），再以该键
`Put` 到 `storage.Store`（生产为 S3 兼容服务，测试为内存 Fake）。

## 去重语义

`StoreAsset` 以 `(wiki_id, name)` 定位 active asset（DB 有部分唯一索引
`asset_wiki_name_key ... WHERE status='active'` 保证并发安全）：

- 同 asset 同 `content_hash` 重复上传：**去重命中**——复用既有
  `asset_revision` 行，不重复 `Put`（`StoreAssetResult.Reused=true`）；
- 同 asset 新内容：新增 `asset_revision`，`asset.current_revision_id` 指向新版本；
- 重传旧内容：去重命中旧 revision，`current_revision_id` 指回该版本
  （上传使所传内容成为 current）。

写入顺序：先 `Put` 对象再开 DB 事务。`Put` 按内容寻址幂等，事务失败只留
无害残留对象；反向顺序会产生「DB 指向缺失对象」，不可取。事务内对 asset
行 `FOR UPDATE` 加锁并复查去重，序列化同名并发上传。

## 不可变边界

- `asset_revision` / `source_version` / `source_chunk` / `citation`：
  000007 复用 000001 的 `reject_immutable_mutation()` 触发器拒绝 UPDATE/DELETE
  （TRUNCATE 不触发行级触发器，testkit Reset 可安全清库）。
- `asset` 允许 UPDATE（status / name / current_revision_id 流转），软删除经
  `status='deleted'`，删除后名字可被新资产复用（与 page 标题同模式）。

## 测试

- `service_test.go`：testkit + 内存 Fake Store 的集成测试——上传落库与存储布局、
  去重（不重复 Put）、新内容新 revision、入参与 Actor 校验，
  以及 SQL 级不可变触发器与 `claim_source_citation_fk` 外键验证。
- `normalize_test.go`：URL 规范化纯函数表驱动单测（无 DB）。
- `source_service_test.go`：ExternalResource 幂等查重、CreateSource 三种关联与
  Actor 校验、AddSourceVersion 幂等/ordinal/locator/raw_asset 校验、
  CreateCitation 校验矩阵。
- 未设 `TEST_DATABASE_URL` 时集成用例全部 skip。

## Source / Citation（M4-T05）

### URL 规范化（`normalize.go`，纯函数）

`NormalizeURL` 是 external_resource 查重键的唯一来源，规则（表驱动单测覆盖）：

- 仅接受 http/https 绝对 URL，其余返回 `ErrInvalidURL`；
- scheme 与 host 小写；剔除默认端口（http:80 / https:443），非默认端口保留；
- 剔除 fragment（`#...`）；
- 路径 `path.Clean` 去 `./..` 段，非根尾部斜杠剔除，空路径归为 `/`；
- 查询参数剔除常见追踪参数（`utm_*` / `gclid` / `fbclid`，参数名大小写不敏感），
  剩余按 key 排序、同 key 值按字典序排序（输出确定）。

`UpsertExternalResource` 以 normalized_url 查重：存在返回既有行（original_url
保留首次输入原文），不存在创建（domain/path 从规范化结果提取，初始
status='unknown'，健康检查流转属 M3-T06）；并发撞唯一索引回查既有行幂等返回。

M3-T06 起，投影事务通过系统级 `ExternalResourceService.UpsertInTx` 复用完全相同
的规范化/upsert 规则；Projection 不直接拼 SQL 修改 `external_resource`。
`UpdateExternalResourceStatus` 是健康检查的系统写入口，只更新资源状态字段与检查时间，
不触碰 Page/Revision/Outbox（INV-12）。

### 定位粒度（设计 §7.4/§7.5）

`source_version`（必有）→ `source_chunk`（可选，`ordinal` 从 0 连续，
`text_hash` 为服务端计算的 text_content 小写 hex SHA-256，不信任调用方）→
`locator_json`（可选）。`Locator` 全部字段可选：`page`（>=1）、`section`（非空）、
`char_start`/`char_end`（0 起，`char_end >= char_start`）；citation.locator 与
chunk.locator 叠加表示更细粒度定位（如 chunk 定位到页，citation 定位到页内字符段）。

### Quotation 规则（INV-07 定位侧）

- quotation 非空时 `quotation_hash = SHA-256(quotation)`，服务端计算；
- 提供 chunk 时 quotation 必须是 chunk 文本子串，否则**严格拒绝**
  `ErrQuotationMismatch`——citation 是权威证据数据，无法在定位文本中复核的
  引文不应落库；
- 不提供 chunk 的 quotation 无子串校验对象，只记 hash。

### 幂等语义

- `AddSourceVersion`：`unique(source_id, version_hash)` 是重复导入不重复抽取的
  DB 基础。version_hash 重复（含并发撞唯一索引）返回既有版本与其 chunks，
  `AddSourceVersionResult.Reused=true`，未重复插入；
- source_version / source_chunk / citation 均不可变（000007 触发器），
  「修改」语义靠登记新版本实现。

### Actor 与存在性校验

- `CreateSource` / `CreateCitation` 复用 `page.CheckWriteActor` 准入
  （human/bot/system 可写，ai actor 拒绝）；source_version 表无 actor 列，
  `AddSourceVersion` 属导入管道行为，不做 Actor 准入；
- `CreateSource` 关联：URL 输入自动 `UpsertExternalResource`（优先），或显式
  `ExternalResourceID`（必须已存在），或 `AssetID`（必须 active），皆空则只登记元数据；
- `CreateCitation`：source_version 必须存在；chunk 非空时必须属于该 version
  （跨版本拒绝 `ErrChunkVersionMismatch`）。

### 与 knowledge 的边界（INV-07 完整化）

knowledge 侧定义 `CitationChecker` 只读接口（`CitationExists`），
`evidence.Repository.CitationExists` 实现之，装配层经
`knowledge.Service.WithCitationChecker` 注入（knowledge 不 import evidence，
evidence 也不 import knowledge）。`AddClaimSource` 据此校验 citation 存在性，
不存在的 citation 返回 `knowledge.ErrCitationNotFound`（未注入时跳过，
DB 外键 `claim_source_citation_fk` 兜底）。

### 只读详情与端到端证据链（M4-T08/T09）

`Service.GetCitationDetail` 按稳定 ID 沿不可变
`Citation → SourceVersion → Source` 读取，并在存在时补全 `SourceChunk` 与
`ExternalResource`。HTTP `GET /api/v1/citations/{id}` 将该定位链匿名只读暴露；
前端 `/citations/{id}` 显示引文、Locator、Chunk 原文、版本哈希和规范化外部 URL。

`backend/cmd/api/evidence_chain_test.go` 的 `TestM4EvidenceChain` 从页面
ClaimReference 出发，经使用投影、ClaimSource、Citation 到 SourceChunk，随后
Supersede Claim 并断言旧事实、旧证据绑定与已发布 AST 引用均不被改写。
