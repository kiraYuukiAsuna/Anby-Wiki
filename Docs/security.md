# 安全基线

## Production 配置

production 启动前必须满足：

- `TRUSTED_ORIGINS` 为逗号分隔的精确 HTTPS origin，例如 `https://wiki.example.com`；禁止 wildcard、path、query、fragment 和 userinfo。
- `OIDC_ENABLED=true`，`OIDC_ISSUER_URL` 与 `OIDC_REDIRECT_URL` 使用 HTTPS，`OIDC_CLIENT_SECRET` 非空、至少 12 字符且不是常见占位值。
- `SESSION_COOKIE_SECURE=true` 且 `AUTH_DEV_HEADER_ENABLED=false`。
- `S3_ACCESS_KEY`/`S3_SECRET_KEY` 不使用 `minioadmin`、`minioadmin_dev`、`wiki_dev_password`、`ci-placeholder`、`changeme` 等开发/占位值，长度至少 12 字符。

development 可配置精确 HTTP localhost，例如：

```dotenv
TRUSTED_ORIGINS=http://localhost:3000,http://localhost:8000
```

## 浏览器写请求

携带 `SESSION_COOKIE_NAME` cookie 的 `POST/PUT/PATCH/DELETE` 在进入业务 handler 前执行来源校验：

1. 存在 `Origin` 时必须精确匹配 trusted origin。
2. `Origin` 缺失时从 `Referer` 提取 origin 并精确匹配。
3. 两者缺失、格式非法或不可信时返回 `403 forbidden`。
4. 无 session cookie 的 development/test `X-Actor-ID` 请求不受该 cookie CSRF 规则影响；production 配置和 Nginx 均禁止该身份来源。

负向测试必须证明拒绝 logout 不撤销 session、拒绝 upload 不写对象、拒绝页面创建不写数据库。

## URL 与渲染

- 编辑器外链、导入 URL、AST `external_link` 共用 `apps/web/lib/http-url.ts`，只接受绝对 `http:`/`https:`。
- AST 和 Citation 渲染再次调用 `safeHttpUrl`；异常或危险协议降级为不可点击文本。
- 禁止直接把未校验 URL 写入 `href`，禁止 `dangerouslySetInnerHTML` 渲染 AST/API HTML。

## Nginx

- 普通 API 请求体上限 2 MiB；`/api/v1/import-jobs/uploads` 上限 11 MiB，业务文件上限仍为 10 MiB。
- auth、upload、general API 分别限流；超限返回 429。
- API 代理清空 `X-Actor-ID`、`X-Authenticated-User`、`X-Auth-Request-User`、`X-Remote-User`。
- CSP、Permissions Policy、COOP/CORP、nosniff、frame 和 referrer 头在网关设置；localhost CSP 仅为 Next HMR 保留 `unsafe-eval`。
- `Strict-Transport-Security` 只在 Nginx 自身处理 HTTPS 时发送，普通 HTTP 开发入口不发送。

## 扫描与已知阻塞

`make security` 执行：

- `go mod verify`
- `govulncheck v1.1.4`
- `npm audit --omit=dev --audit-level=high`
- `gitleaks v8.28.0`

2026-07-23 实跑发现并修复 Go `GO-2026-5970`（`x/text`）、`GO-2026-4945`（`go-jose`）和 `GO-2026-4394`（`go.opentelemetry.io/otel/sdk`）；OpenTelemetry Go v1 模块已统一升级到稳定版 `v1.40.0`。

2026-07-24 最终门禁结果：

- `govulncheck v1.1.4 ./...` 报告 0 个可达漏洞，Go 全量测试、vet 与构建通过；
- npm registry 最新 Next 仍为 `16.2.11`，其 production 依赖 `sharp 0.34.5`
  命中 `GHSA-f88m-g3jw-g9cj`（high），安全版本为 `sharp 0.35.3`；
- Next 内嵌 `postcss <=8.5.11` 命中 `GHSA-qx2v-qp2m-jg93` 与
  `GHSA-6g55-p6wh-862q`（high），npm 最新安全版本为 `postcss 8.5.22`；
- npm 提供的 `audit fix --force` 会破坏性降级到 Next 9，不能采用，也不通过
  unsupported override 绕过框架锁定依赖。

因此 npm security gate 保持失败，production 发布继续阻塞，直到 Next 发布兼容安全
sharp/postcss 的版本，并重新通过 typecheck、Lint、unit、build、E2E 与 audit 全部门禁。

gitleaks 首次无白名单扫描只命中 Next `.next` 生成密钥和 `imports_test.go` 固定 UUID 幂等键；`.gitleaks.toml` 仅精确豁免这两个路径，其他规则与源码继续扫描。
