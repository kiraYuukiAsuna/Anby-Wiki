# ADR-0010：OIDC 与服务端会话

状态：已接受（M7-T02）
日期：2026-07-23

## 背景

P0 Beta 需要接入实际身份系统，把外部用户稳定映射为领域 Actor，并替换 handler 直接读取
`X-Actor-ID` 的过渡方案。认证不能把 IdP token 暴露给浏览器应用，也不能让权限缓存延迟角色
撤销、Actor 停用或页面保护变更。

## 决策

- 使用通用 OpenID Connect discovery 与 Authorization Code flow，强制 PKCE S256、state 和 nonce。
- state 绑定一个 HttpOnly、SameSite=Lax 的短期浏览器 cookie；登录事务一次消费，默认 10 分钟过期。
- ID token 由 discovery 得到的签名密钥和 Client ID 校验；只在校验成功后使用 `iss`、`sub` 和显示名。
- `(issuer, subject)` 通过 `external_identity` 唯一映射一个 `human` Actor。映射和 session 创建都经
  `internal/auth.Service` 的事务写入，不由 HTTP handler 直接改权威状态。
- 浏览器只持有高熵不透明 session cookie。数据库只保存令牌 SHA-256 哈希；cookie 设置 HttpOnly、
  SameSite=Lax，production 强制 Secure。服务端不持久化 access token 或 ID token。
- 认证中间件把 `ActorID`、Actor 类型和认证方式写入 request context。写 handler 只从 context 取 Actor。
- session 不包含 Role、权限、PageProtection 或授权版本。`AuthorizationService` 在每个授权点实时查询
  Actor、ActorRole 和 PageProtection；角色撤销、保护规则变化在下一次权限检查即时生效，Actor 停用和
  session 吊销在下一请求即时生效。
- `X-Actor-ID` 仅作为显式启用的 development/test 适配器。production 配置出现该开关时拒绝启动，
  且 production 必须启用 OIDC 和 Secure cookie。
- 认证日志只记录稳定错误类别与 request ID，不记录 token、authorization code、state、nonce、
  PKCE verifier、issuer subject、邮箱或显示名。
- 浏览器协议统一位于 `/api/v1/auth`：`GET login` 开始 OIDC 跳转，`GET callback` 建立 session，
  `GET session` 返回当前 Actor 的 `actor_id`、`actor_type`、`display_name`、`method`，
  `POST logout` 吊销 session 并清除 cookie。
- OpenAPI 和生成客户端不接受 Actor ID 参数。Web 通过 SWR 读取 session，写请求只携带同源 cookie；
  Nginx 在 `/api` 代理边界无条件清空外来 `X-Actor-ID`。

## 备选方案

- 浏览器保存 IdP access token：扩大 token 泄漏面，并把 session 生命周期耦合到具体 IdP，拒绝。
- JWT 自包含应用 session：难以立即吊销，且容易把角色写入 claim 形成权限缓存，拒绝。
- 缓存 ActorRole 并用 TTL 失效：撤权存在窗口，不满足即时失效要求，P0 不采用。若未来性能测试证明
  必须缓存，应另立 ADR，引入单调授权版本或可靠失效事件，不得只依赖 TTL。
- 继续信任 Nginx 注入身份头：代理绕过或错误拓扑会造成身份伪造，不能作为 Go API 的生产信任边界。

## 影响

- OIDC discovery 在 API 启动时执行；启用 OIDC 但 discovery 失败时拒绝启动。
- 回滚 `000015` 会删除 session 和外部身份映射并使所有用户退出，但保留 Actor 和审计归属。
- `/api/v1/auth/login` 与 `/api/v1/auth/callback` 是浏览器重定向端点；`session` 与 `logout` 由
  OpenAPI `AuthApi` 覆盖。
- CSRF token、CSP 和 Nginx 限流留给 M7-T03；本 ADR 的 SameSite cookie、POST logout 和身份头清理
  是基础防线，不替代 M7-T03 的完整安全验收。
