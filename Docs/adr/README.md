# 架构决策记录（ADR）

| 编号 | 决策 | 状态 |
|---|---|---|
| [ADR-0001](./0001-go-project-layout.md) | Go 工程结构与模块边界 | 已接受 |
| [ADR-0002](./0002-database-migrations-and-data-access.md) | SQL 迁移（golang-migrate）与数据访问（pgx/v5） | 已接受 |
| [ADR-0003](./0003-async-tasks-outbox-queue.md) | 异步任务：Postgres Outbox + SKIP LOCKED 轮询 | 已接受 |
| [ADR-0004](./0004-object-storage.md) | 对象存储：S3 API + 本地 MinIO | 已接受 |
| [ADR-0005](./0005-editor-selection-blocknote.md) | 编辑器选型：BlockNote（Adapter 验收门槛） | 已接受 |
| [ADR-0006](./0006-search-adapter.md) | 搜索 Adapter：P0 用 PostgreSQL FTS | 已接受（Meilisearch 部分被 ADR-0016 取代） |
| [ADR-0007](./0007-api-client-generation.md) | 客户端生成：OpenAPI Generator v7 typescript-fetch | 已接受 |
| [ADR-0008](./0008-id-strategy-uuidv7.md) | ID 策略：UUIDv7 | 已接受 |
| [ADR-0009](./0009-observability-baseline.md) | 可观测性：slog + OpenTelemetry + Prometheus | 已接受 |
| [ADR-0010](./0010-oidc-server-side-session.md) | 通用 OIDC、Actor 映射与服务端会话 | OIDC 部分被 ADR-0016 取代 |
| [ADR-0011](./0011-web-gateway-security-baseline.md) | Web、cookie 写请求与网关安全基线 | 网关部分被 ADR-0016、Origin CSRF/COOP/CORP 部分被 ADR-0020 取代 |
| [ADR-0012](./0012-internal-beta-slo-and-search-capacity.md) | 内部 Beta SLO 与搜索容量门槛 | 已接受（搜索结论被 ADR-0016 推翻） |
| [ADR-0013](./0013-defer-beta-gates-for-p1-development.md) | 延后 Beta 门禁但继续 P1 研发 | 已接受 |
| [ADR-0014](./0014-yjs-working-document-crdt.md) | WorkingDocument CRDT 采用 Yjs | 已接受 |
| [ADR-0015](./0015-client-assisted-yjs-ai-merge.md) | AI 合并采用客户端辅助 Yjs CAS | 已接受 |
| [ADR-0016](./0016-early-stage-deployment-simplification.md) | 早期阶段部署简化：去 Docker 开发、去 Nginx/Meili/OIDC | 已接受（认证被 ADR-0019、Secrets 注入被 ADR-0017 取代） |
| [ADR-0017](./0017-production-secrets-in-env-file.md) | 生产机密统一写入部署环境文件 | 已接受（AI Provider 密钥部分被 ADR-0021 取代） |
| [ADR-0018](./0018-local-only-commercial-images.md) | 商业业务镜像仅在部署机本地构建和使用 | 已接受 |
| [ADR-0019](./0019-local-account-authentication.md) | 正式本地账号、密码哈希与首账号管理员初始化 | 已接受 |
| [ADR-0020](./0020-remove-origin-csrf-and-cross-origin-headers.md) | 移除 Origin CSRF 门禁与 COOP/CORP | 已接受 |
| [ADR-0021](./0021-semantic-kernel-and-admin-ai-config.md) | Semantic Kernel Sidecar 与管理员 AI 配置 | 已接受 |

新增 ADR 规则：编号只增不复用；状态为「提议 / 已接受 / 已废弃 / 被 ADR-XXXX 取代」；
涉及实施方案 §8.2 高冲突资源的决策变更必须先更新 ADR 再改代码。
