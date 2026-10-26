# 安全基线

## Production 配置

production 启动前必须满足：

- 注册默认关闭。受控创建账号时显式设置 `AUTH_REGISTRATION_ENABLED=true`；
  每个账号创建独立私人 Wiki 并成为其管理员，完成后立即改回 `false`。
- `AUTH_DEV_HEADER_ENABLED=false`。外层实际提供 HTTPS 时必须设置 `SESSION_COOKIE_SECURE=true`；明文本地/封闭部署才允许 `false`。
- `S3_ACCESS_KEY`/`S3_SECRET_KEY` 与 `MEILI_MASTER_KEY` 不使用开发值或模板中的
  `replace-with-*` 占位值；机密长度至少 12 字符。

## 浏览器写请求

ADR-0020 已删除 `BrowserWriteGuard` 和 `TRUSTED_ORIGINS`。携带 session cookie 的
`POST/PUT/PATCH/DELETE` 不再检查 `Origin` 或 `Referer`。

Session cookie 仍设置 HttpOnly 与 `SameSite=Lax`，Web 仍通过当前 origin 的
`/api/*` rewrite 访问 Go API。此配置没有启用 CORS，也不支持跨 origin 携凭据直接
访问 API。`SameSite=Lax` 不是完整 CSRF 防护；在宣称商业发布安全门禁关闭前必须恢复
同步 token、Fetch Metadata 或等价来源校验，并且不得在缺少等价防护时使用
`SameSite=None`。

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
- 禁止直接把未校验 URL 写入 `href`。只有专用 `ServerRenderedDocument` 可挂载
  后端从已校验 AST 生成并逐值转义的 `rendered_html` 投影；不得把用户原始 HTML、
  Prompt、Source 全文或任意 API 字符串交给 `dangerouslySetInnerHTML`。

## 应用边界控制

- 普通 API 请求体上限 2 MiB；`/api/v1/import-jobs/uploads` envelope 上限 301 MiB，
  业务文件上限为 300 MiB。上传仍受独立 Redis 速率桶、MIME/magic、恶意签名和内容
  哈希复核约束。
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

2026-07-31 门禁发现并修复 gRPC `GO-2026-6061` 与 OpenTelemetry
`GO-2026-5158` 的可达调用链：

- gRPC 升级到 `v1.82.1`，OpenTelemetry Go v1 家族统一到 `v1.44.0`；
- `x/crypto`、`x/net`、`x/sys` 与 genproto 随兼容依赖链升级；
- `govulncheck@latest` 和 CI 固定的 `govulncheck v1.1.4` 均报告 0 个可达漏洞。

Web 安全依赖同步完成：

- Next 升级到 `16.3.8`，受控 npm override 使用 PostCSS `8.5.25`、
  Sharp `0.35.5` 与 `source-map-js` `1.2.2`；
- shadcn 移到 devDependencies；MCP/Hono、`shell-quote` 与
  `brace-expansion` 均固定到已修复版本；
- `brace-expansion` 5 改变了 CommonJS 导出形态，安装期幂等脚本只适配仍由
  ESLint 9 使用的 `minimatch@3` 导入语句；未知源码会失败并要求人工复核；
- `npm audit --audit-level=high` 与 `npm audit --omit=dev --audit-level=high`
  均为 0，`npm ci`、typecheck、Lint、OpenAPI 生成和 production build 通过。

Go 构建基线为 `1.26.10`；OpenTelemetry Go 为 `1.45.0`，gRPC 为 `1.83.1`，
`golang.org/x/net` 为 `0.60.0`，`golang.org/x/text` 为 `0.42.0`。版本更新后仍需以
`govulncheck` 的实际可达调用链作为门禁，不能只根据依赖版本文字判断安全状态。

`.gitleaks.toml` 只精确豁免 Next `.next` 生成目录与部署模板中的两个完整字面
占位值；不会豁免整个 `.env.example`，其他规则与源码继续扫描。
