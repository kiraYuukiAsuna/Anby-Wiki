# 架构决策记录（ADR）

| 编号 | 决策 | 状态 |
|---|---|---|
| [ADR-0001](./0001-go-project-layout.md) | Go 工程结构与模块边界 | 已接受 |
| [ADR-0002](./0002-database-migrations-and-data-access.md) | SQL 迁移（golang-migrate）与数据访问（pgx/v5） | 已接受 |
| [ADR-0003](./0003-async-tasks-outbox-queue.md) | 异步任务：Postgres Outbox + SKIP LOCKED 轮询 | 已接受 |
| [ADR-0004](./0004-object-storage.md) | 对象存储：S3 API + 本地 MinIO | 已接受 |
| [ADR-0005](./0005-editor-selection-blocknote.md) | 编辑器选型：BlockNote（Adapter 验收门槛） | 已接受 |
| [ADR-0006](./0006-search-adapter.md) | 搜索 Adapter：P0 用 PostgreSQL FTS | 已接受 |
| [ADR-0007](./0007-api-client-generation.md) | 客户端生成：OpenAPI Generator v7 typescript-fetch | 已接受 |
| [ADR-0008](./0008-id-strategy-uuidv7.md) | ID 策略：UUIDv7 | 已接受 |
| [ADR-0009](./0009-observability-baseline.md) | 可观测性：slog + OpenTelemetry + Prometheus | 已接受 |
| [ADR-0010](./0010-oidc-server-side-session.md) | 通用 OIDC、Actor 映射与服务端会话 | 已接受 |
| [ADR-0011](./0011-web-gateway-security-baseline.md) | Web、cookie 写请求与网关安全基线 | 已接受 |
| [ADR-0012](./0012-internal-beta-slo-and-search-capacity.md) | 内部 Beta SLO 与搜索容量门槛 | 已接受 |
| [ADR-0013](./0013-defer-beta-gates-for-p1-development.md) | 延后 Beta 门禁但继续 P1 研发 | 已接受 |
| [ADR-0014](./0014-yjs-working-document-crdt.md) | WorkingDocument CRDT 采用 Yjs | 已接受 |
| [ADR-0015](./0015-client-assisted-yjs-ai-merge.md) | AI 合并采用客户端辅助 Yjs CAS | 已接受 |

新增 ADR 规则：编号只增不复用；状态为「提议 / 已接受 / 已废弃 / 被 ADR-XXXX 取代」；
涉及实施方案 §8.2 高冲突资源的决策变更必须先更新 ADR 再改代码。
