# ADR-0024：使用 Go 标准库保护浏览器写请求

- 状态：已接受
- 日期：2026-10-30
- 取代：ADR-0020 中移除浏览器写请求 CSRF 校验的决策

## 决策

API 路由统一接入 `net/http.CrossOriginProtection`，在认证与业务处理前校验请求。
不配置可信来源或路径绕过规则；拒绝响应沿用 `forbidden`、HTTP 403 和请求 ID。
注册、登录、退出与 CLI 授权码兑换同样受校验，不依赖已有 Session Cookie 才启用。

标准库优先检查 `Sec-Fetch-Site`：`same-origin` 和 `none` 允许，其余值拒绝。
缺少该请求头时，比较 `Origin` 的 Host 与请求 Host；跨来源或 `null` Origin 拒绝。
GET、HEAD、OPTIONS 允许；两种来源头均缺失的调用按非浏览器或同源请求允许，
随后继续执行认证、角色授权和领域校验。

浏览器继续使用同源 API。生产 Nginx 保留公开 Host，Next.js 同源 rewrite 保留
浏览器的 Fetch Metadata；不信任客户端提供的 `X-Forwarded-Host` 来放行请求。
不恢复 `TRUSTED_ORIGINS`、旧自定义门禁、CORS、COOP 或 CORP 配置。

## 边界与验证

来源校验不替代会话认证或角色授权。CLI、Worker 和 Server Component 的无来源头
调用保持可用，WebSocket 的 GET 握手继续使用既有独立 Origin 与权限校验。
业务读取方法不得用于执行权威状态变更。

验证覆盖同源写入、同站点跨端口/子域写入拒绝、Origin 回退、公共认证路由、
结构化错误与请求 ID、CLI 认证，以及真实浏览器经同源 rewrite 的登录、编辑、
上传和跨来源请求。生产部署后的同等验收与证书恢复演练分开记录。
