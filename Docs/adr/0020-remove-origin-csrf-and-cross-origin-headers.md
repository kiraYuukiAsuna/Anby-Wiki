# ADR-0020：移除 Origin CSRF 门禁与跨源隔离响应头

- 状态：已接受
- 日期：2026-07-28

## 背景

当前早期部署直接通过 HTTP IP 和 Web 端口提供服务。Cookie 写请求依赖
`BrowserWriteGuard` 对 `Origin`/`Referer` 与 `TRUSTED_ORIGINS` 做精确匹配，
同时 Web/API 返回 COOP/CORP。该组合增加部署配置负担，且 COOP 在非可信 HTTP
origin 上会被浏览器忽略并产生告警。

## 决策

1. 删除 `BrowserWriteGuard`。携带 session cookie 的写请求不再检查
   `Origin` 或 `Referer`。
2. 删除 `TRUSTED_ORIGINS` 配置、校验和 Compose 注入。
3. Web 与 API 不再返回 `Cross-Origin-Opener-Policy` 和
   `Cross-Origin-Resource-Policy`。
4. 保留 HttpOnly、`SameSite=Lax` session cookie、服务端会话认证、角色授权、
   CSP、frame、nosniff、请求体限制和应用限流。
5. 本决策不增加 CORS 响应头，也不支持跨 origin 携凭据直接调用 API；浏览器仍通过
   Next.js 的同源 `/api/*` rewrite 访问 Go API。

## 安全影响

`SameSite=Lax` 可降低常见跨站表单携带 cookie 的风险，但不能替代完整 CSRF 防护。
正式公网发布前应重新评估同步 token、Fetch Metadata 或恢复精确来源校验；不得在没有
等价防护时把 session cookie 改为 `SameSite=None`。

## 取代范围

- 取代 ADR-0011 决策 1～3，以及决策 5 中的 COOP/CORP 部分。
- 取代 ADR-0016 第 5 节中关于 `TRUSTED_ORIGINS` 的配置要求。
