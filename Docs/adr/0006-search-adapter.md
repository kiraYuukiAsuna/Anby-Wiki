# ADR-0006：搜索 Adapter 策略

状态：已接受（M7-T11 已落实内部 Beta Meilisearch）
日期：2026-07-21

## 背景

实施方案要求搜索走 Adapter，P0 后段才接独立搜索引擎；业务代码禁止依赖具体搜索产品。

## 决策

- 接口：`internal/projection/search`（后迁 `internal/search`）定义 `SearchAdapter` 窄接口：`Index(doc SearchDocument)`、`Delete(pageID)`、`Search(query) ([]Hit, total)`、`Rebuild()`。
- **P0 实现：PostgreSQL FTS Adapter**（`tsvector`/`tsquery`，`websearch_to_tsquery`），索引由 Outbox 驱动的投影更新，可全量重建。满足 M7-T01 的标题、别名、正文、Entity、字段过滤需求。
- 独立引擎候选（P0 后段按容量基线决定）：Meilisearch（运维简单）优先，Typesense/Elasticsearch 备选。本地 compose 不预置引擎，接入时再补。
- SearchDocument 是契约对象（contracts/schemas），字段与版本固定，Adapter 实现可替换。
- **内部 Beta 实现：Meilisearch**。API 通过 `SEARCH_BACKEND=meilisearch` 查询；
  production 强制该值。实现只调用官方 HTTP API，不引入 SDK。
- `search_document` 保留为可重建 staging/fallback。Builder 与 staging/
  `projection_state` 先在 PostgreSQL 提交，随后在 Page 行锁保护下再次确认 Current
  Revision、提交 Meili 文档并等待异步 task 终态；失败交给 Outbox 至少一次重试。
  这避免远端非事务写先于数据库提交，也阻止旧 Revision/失败重试覆盖新索引。
- Meili 主键为 `page_id`，重复写天然幂等；索引设置固定 searchable、displayed、
  filterable 字段，查询使用 Adapter 既有字段过滤、纯文本高亮与 offset/limit 契约。

## 备选方案

- 直接上 Elasticsearch：违反「P0 先验证架构」与非目标精神，运维成本高。
- 只用 Postgres 长期运行：P2 语义搜索必然需要独立引擎，故接口先行。

## 影响

- 查询路径（API handler）只依赖 `SearchAdapter` 接口。
- M7-T05/ADR-0012 的 100k 实测判定 PostgreSQL FTS 不保留为内部 Beta 最终实现：
  延迟刚好达标但吞吐仅 1.146 req/s，真实 Adapter 慢查询顺序扫描 100,000 行。
  `SearchAdapter` 契约继续保留；M7-T11 已接入 Meilisearch，并提供同口径
  `make perf-meili-full` 复测命令。
- 回滚可在非 production 环境切回 `SEARCH_BACKEND=postgres`；production 不允许
  静默回退，避免容量不合格实现被误用。Meili 索引可整体删除后从 staging 重建。
