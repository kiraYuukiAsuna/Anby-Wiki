# 安全基线

## Production 配置

production 启动前必须满足：

- 注册默认关闭。首次受控部署时显式设置 `AUTH_REGISTRATION_ENABLED=true`，
  首个注册账号自动获得管理员角色；完成初始化后立即改回 `false`。
- `AUTH_DEV_HEADER_ENABLED=false`。外层实际提供 HTTPS 时必须设置 `SESSION_COOKIE_SECURE=true`；明文本地/封闭部署才允许 `false`。
- `S3_ACCESS_KEY`/`S3_SECRET_KEY` 不使用 `minioadmin`、`minioadmin_dev`、`wiki_dev_password`、`ci-placeholder`、`changeme` 等开发/占位值，长度至少 12 字符。

## 浏览器写请求

ADR-0020 已删除 `BrowserWriteGuard` 和 `TRUSTED_ORIGINS`。携带 session cookie 的
`POST/PUT/PATCH/DELETE` 不再检查 `Origin` 或 `Referer`。

Session cookie 仍设置 HttpOnly 与 `SameSite=Lax`，Web 仍通过当前 origin 的
`/api/*` rewrite 访问 Go API。此配置没有启用 CORS，也不支持跨 origin 携凭据直接
访问 API。`SameSite=Lax` 不是完整 CSRF 防护；正式公网发布前必须恢复同步 token、
Fetch Metadata 或等价来源校验，并且不得在缺少等价防护时使用 `SameSite=None`。

## 本地账号

- 用户名和邮箱规范化后分别做数据库唯一约束；登录错误不区分账号不存在、密码错误
  或账号停用，减少账号枚举。
- 密码要求 12–128 个字符，使用 Argon2id（64 MiB、3 次迭代、并行度 2、
  16-byte 随机 salt、32-byte key）保存为带参数的哈希字符串，不记录密码或哈希。
- 注册、Actor 创建、初始角色和首个 session 在同一事务中完成；数据库 advisory lock
  保证并发注册时只有一个首账号获得管理员角色。
- session 使用 32-byte CSPRNG 随机令牌，客户端只收到 HttpOnly cookie，数据库只保存
  SHA-256；退出立即设置 `revoked_at`。
- 当前缺少邮箱验证、密码找回、MFA 和设备会话管理，见
  [待解决问题](OutstandingIssues.md)。

## URL 与渲染

- 编辑器外链、导入 URL、AST `external_link` 共用 `apps/web/lib/http-url.ts`，只接受绝对 `http:`/`https:`。
- AST 和 Citation 渲染再次调用 `safeHttpUrl`；异常或危险协议降级为不可点击文本。
- 禁止直接把未校验 URL 写入 `href`，禁止 `dangerouslySetInnerHTML` 渲染 AST/API HTML。

## 应用边界控制

- 普通 API 请求体上限 2 MiB；`/api/v1/import-jobs/uploads` 上限 11 MiB，业务文件上限仍为 10 MiB。
- auth、upload、general API 由 Go + Redis 分别限流；超限返回 429。Redis 不可达时放行并记录告警。
- Go API 清空 `X-Authenticated-User`、`X-Auth-Request-User`、`X-Remote-User`；除显式本地开发模式外也清空 `X-Actor-ID`。
- Web 与 API 分别设置 CSP、nosniff、frame 和 referrer 头；Web 另设置 Permissions Policy，localhost CSP 仅为 Next HMR 保留 `unsafe-eval`。COOP/CORP 已由 ADR-0020 移除。
- 当前 Compose 不终结 TLS，因此应用不发送 HSTS。由外层 TLS 边界负责 HSTS 时，必须同步启用 Secure cookie。

## 扫描与已知阻塞

`make security` 执行：

- `go mod verify`
- `govulncheck v1.1.4`
- `npm audit --omit=dev --audit-level=high`
- `gitleaks v8.28.0`

2026-07-23 实跑发现并修复 Go `GO-2026-5970`（`x/text`）、`GO-2026-4945`
（`go-jose`）和 `GO-2026-4394`（`go.opentelemetry.io/otel/sdk`）。

2026-07-26 门禁又发现并修复 `GO-2026-5506`、`GO-2026-5158`
（OpenTelemetry baggage header 解析资源消耗）与 `GO-2026-4559`
（`x/net` HTTP/2 frame 可触发 panic）；OpenTelemetry Go v1 模块已统一升级到
`v1.42.0`，`x/net` 已升级到 `v0.51.0`。
最终门禁结果：

- `govulncheck v1.1.4 ./...` 报告 0 个可达漏洞，Go vet 与构建通过；
- 当前 Next `16.2.11` 的 production 依赖 `sharp 0.34.5`
  命中 `GHSA-f88m-g3jw-g9cj`（high），修复要求 `sharp >=0.35.0`；
- Next 内嵌 `postcss <=8.5.17` 命中 `GHSA-qx2v-qp2m-jg93`、
  `GHSA-6g55-p6wh-862q` 与 `GHSA-r28c-9q8g-f849`（high）；
- 非破坏性 `npm audit fix` 已升级可独立修复的 `brace-expansion`，production
  审计仍报告 3 个 high 和 3 个 moderate；
- npm 提供的 `audit fix --force` 会破坏性降级到 Next 9，不能采用，也不通过
  unsupported override 绕过框架锁定依赖。

因此 npm security gate 保持失败，production 发布继续阻塞，直到 Next 发布兼容安全
sharp/postcss 的版本，并重新通过 typecheck、Lint、build 与 audit 门禁。

`.gitleaks.toml` 仅精确豁免 Next `.next` 生成目录，其他规则与源码继续扫描。
