# ADR-0011：Web 与网关安全基线

- 状态：已接受
- 日期：2026-07-23
- Task：M7-T03

## 背景

M7-T02 已建立 HttpOnly 服务端 session，但 SameSite 不能替代写请求来源校验；同时外部 URL 会从编辑器、导入和证据数据进入浏览器，Next.js 与 Nginx 也需要一致的响应头、大小、限流和供应链门禁。

## 决策

1. 携带 session cookie 的 `POST/PUT/PATCH/DELETE` 必须由 API 的 `BrowserWriteGuard` 校验来源。优先校验 `Origin`，仅在缺失时从 `Referer` 提取 origin；来源必须与 `TRUSTED_ORIGINS` 精确匹配。
2. `TRUSTED_ORIGINS` 禁止 wildcard、userinfo、path、query 和 fragment。production 必填且只允许 HTTPS；development 可显式配置精确 HTTP localhost origin。
3. 无 session cookie 的 development/test `X-Actor-ID` 请求不进入 cookie CSRF 规则。该例外不能在 production 启用，且 Nginx 清空所有受信身份头。
4. Web 只使用统一 `httpUrlSchema`/`safeHttpUrl` 接受绝对 HTTP(S) URL。AST、编辑器、导入输入使用同一 Zod；AST 与 Citation 渲染再次校验，失败时输出不可点击文本。
5. Next.js 通过 Next 16 `headers()` 提供 CSP、nosniff、frame、referrer、Permissions Policy 和跨源隔离基线。Nginx 在边界重复提供基线，按 host 区分开发 CSP；HSTS 只在 Nginx 自身 HTTPS 时发出。
6. Nginx 普通 API 请求体上限为 2 MiB，导入上传入口为 11 MiB，业务层继续限制文件内容为 10 MiB；auth、upload、general API 使用独立 IP 速率桶。
7. production 拒绝已知开发/占位 S3 Secret、短 Secret、HTTP OIDC、空或弱 OIDC client secret。
8. CI 固定 `govulncheck v1.1.4`、`gitleaks v8.28.0`，并执行 `go mod verify` 与 `npm audit --omit=dev --audit-level=high`。真实漏洞不得白名单；gitleaks 只允许对已确认误报做精确路径豁免。

## 取舍

- origin 校验不引入前端 token 状态，适合同源 API 和 HttpOnly session；非浏览器 cookie 客户端必须发送可信 `Origin`/`Referer` 或改用未来的非 cookie 认证。
- 静态 CSP 为兼容 Next 当前内联 bootstrap 保留 `script-src 'unsafe-inline'`；后续可用每请求 nonce 收紧，但会迫使动态渲染并影响缓存。
- IP 限流是边界保护而非业务配额；多副本部署时如需全局配额，应另行引入共享限流服务。
- 安全扫描是阻断门禁。上游尚无兼容修复时，发布保持阻塞而不是降低 audit level。
