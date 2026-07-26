# ADR-0016：早期阶段部署简化

- 状态：已接受（第 6 节被 ADR-0017 取代）
- 日期：2026-07-25

## 背景

项目处于早期阶段，尚未公开发布，也没有真实用户。既有部署形态是为
「内部 Beta 上线」设计的：Nginx 网关、Meilisearch、外部 OIDC 身份提供方、
Swarm external secrets、以及一套本地 Docker 基础设施。

这套形态对当前阶段成本过高：每次本地开发都要拉起 5 个容器，
部署前要先准备身份提供方与搜索引擎。运维负担与当前的验证目标不匹配。

## 决策

### 1. 开发环境不使用 Docker

`scripts/dev.sh` 只启动本仓库的三个进程（api / worker / web）。
PostgreSQL、Redis、对象存储视为外部依赖，连接信息经仓库根 `.env` 注入。
脚本在启动前显式探测三者可达性，不可达即失败并指明地址。

删除 `infra/local/`（compose、Prometheus、OTel Collector、初始化 SQL）。

### 2. 移除 Meilisearch

搜索只保留 PostgreSQL FTS Adapter。`SearchAdapter` 接口不变，
未来可在其后重新接入独立引擎。

**已知代价**：ADR-0012 的 10 万页面实测判定 PostgreSQL FTS 吞吐仅
1.146 req/s，不满足内部 Beta 容量门槛。本决策接受该回退，
理由是早期阶段数据量远低于该量级。容量重新成为瓶颈时必须重开 Task。
遗留项记录在 `Docs/OutstandingIssues.md`。

### 3. 移除 OIDC，改用引导令牌登录

删除 OIDC 客户端、PKCE 登录事务与 `oidc_login_attempt` 相关写入路径。
新增 `POST /api/v1/auth/dev-login`：以共享引导令牌换取服务端 session，
Actor 映射、会话存储与吊销机制沿用 ADR-0010 的服务端 session 模型。

**已知代价**：该端点不验证调用者真实身份，令牌持有者即可写入，
审计无法区分自然人。仅适用于封闭的早期部署，
接入真实身份提供方前不得公开暴露。

令牌只存 SHA-256 哈希并用常量时间比较；非 development 环境强制要求
非空且非弱值。

### 4. 移除 Nginx，安全职责下沉到 Go API

生产 Compose 不再包含反向代理，`web` 是唯一发布端口的服务，
通过 Next.js rewrites 把 `/api/*` 转发到内网 api。

Nginx 原先独有的四项职责改由 API 中间件承担：

| 原 Nginx 职责 | 现实现 |
|---|---|
| 限流（general / auth / upload 三桶） | `RateLimit` 中间件 + Redis 固定窗口 |
| 清空可伪造身份头 | `StripSpoofableAuthHeaders` 中间件 |
| API 响应安全头 | `SecurityHeaders` 中间件 |
| 请求体上限 | 全局 `RequestBodyLimit` 中间件；上传 handler 可施加更紧的限制 |

限流在 Redis 不可达时**放行**并记录日志：可用性优先于严格限流，
因此限流不能作为唯一滥用防线。

客户端身份取自直连对端 IP；只有当对端属于 `TRUSTED_PROXY_IPS` 时
才采信 `X-Forwarded-For`，否则任何客户端都能自选限流桶。

### 5. 不再默认终结 TLS

Compose 不提供 HTTPS。配置校验相应放宽：不再强制
`SESSION_COOKIE_SECURE=true`，`TRUSTED_ORIGINS` 允许 HTTP origin。
需要 HTTPS 时在外层终结并同步调整这两项。

### 6. Secrets 改为文件挂载

`external: true` 只在 Swarm 下可用，普通 `docker compose` 无法挂载。
改为 `file:` 引用 `SECRETS_DIR` 下的文件，`deploy.sh` 在部署前校验
每个文件存在且非空。`*_FILE` 间接注入保留，避免机密出现在
`docker inspect` 与日志中。

### 7. 生产 Compose 自带数据层

postgres / redis / minio 与应用一同部署，使用命名卷持久化。
一次性 `storage-init` job 先修正新卷属主，数据服务随后始终以非 root 运行；
应用容器通过 `depends_on: service_healthy` 等待其就绪。

## 影响

- 取代 ADR-0010（OIDC）与 ADR-0011 第 3、5、6、7 条中关于 Nginx 与 OIDC 的部分。
- 部分推翻 ADR-0006 / ADR-0012 关于接入 Meilisearch 的结论。
- ADR-0009 的进程内 slog / Prometheus / OTel 保持不变，
  仅移除本地 collector 边车；OTLP 改为显式配置才启用。
- 上述 1–5 的安全与容量代价全部登记在 `Docs/OutstandingIssues.md`，
  是公开发布的阻塞项。
