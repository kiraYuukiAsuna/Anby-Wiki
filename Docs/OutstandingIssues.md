# 待解决问题

当前研发方案已完成，但以下问题仍阻塞生产发布。

## 1. Web 生产依赖高危漏洞

### 现状

- 当前 Next.js 为 `16.2.11`，其依赖链仍包含存在 high 漏洞的 `sharp 0.34.5`。
- 安全版本要求 `sharp >=0.35.0`，但当前 Next.js 发布版本尚未提供兼容升级路径。
- Next.js 内嵌的 `postcss <=8.5.17` 命中 3 个 high advisory，当前审计给出的
  自动修复会破坏性降级到 Next 9。
- `npm audit fix --force` 只提供破坏性降级方案，不能用于生产修复。
- Go `govulncheck` 当前没有可达漏洞。

### 关闭条件

1. 上游 Next.js 发布包含安全版本 `sharp` 和 `postcss` 的兼容版本，或提供受支持的升级路径。
2. 升级 Next.js 及锁文件，不使用 unsupported override 或强制降级。
3. 重新通过 `npm audit --omit=dev --audit-level=high`、Web 全量测试和生产构建。
4. 更新 [安全基线](security.md) 并记录实际修复版本。

## 2. 登录不验证真实身份（早期阶段占位）

### 现状

- OIDC 已整体移除。当前唯一登录路径是 `POST /api/v1/auth/dev-login`：
  用共享引导令牌换取服务端 session。
- 该端点**不验证调用者是谁**，任何持有令牌的人都能以对应 Actor 身份写入，
  审计记录也无法区分具体自然人。
- 仅适用于封闭的早期部署；令牌泄露等同于账号泄露。

### 关闭条件

1. 接入真实身份提供方（OIDC 或等价方案），恢复可归因的 Actor 身份。
2. 设置 `AUTH_DEV_LOGIN_ENABLED=false` 并从部署环境移除引导令牌。
3. 验证登录、退出、Actor 停用、角色撤销在正式域名下生效。

## 3. 默认无 TLS

### 现状

- 生产 Compose 已移除反向代理，不再终结 TLS；`web` 是唯一发布端口的服务。
- 明文暴露时，会话 cookie 与引导令牌在网络上可见。
- 配置校验不再强制 `SESSION_COOKIE_SECURE=true` 与 HTTPS origin，
  以便早期阶段用明文 HTTP 跑通。

### 关闭条件

1. 在外层（云 LB / Cloudflare / 独立代理）终结 HTTPS。
2. 设置 `SESSION_COOKIE_SECURE=true`，并把 `TRUSTED_ORIGINS` 改为 HTTPS origin。
3. 若要按最终客户端 IP 限流，先确保外层清洗 `X-Forwarded-For`，再仅将 API
   的可信直连对端加入 `TRUSTED_PROXY_IPS`；否则保持为空并接受按代理 IP 聚合。

## 4. 搜索容量不足

### 现状

- Meilisearch 已整体移除，搜索只剩 PostgreSQL FTS。
- ADR-0012 的 10 万页面实测：延迟达标但吞吐仅 1.146 req/s，
  真实 Adapter 慢查询顺序扫描 100,000 行。
- 早期阶段数据量小可以接受；数据量或并发上升后会成为瓶颈。

### 关闭条件

1. 在 `SearchAdapter` 后重新接入独立搜索引擎。
2. 用同口径基准复测并记录吞吐与延迟。

## 5. 生产发布输入与 Beta 验收未完成

### 缺少输入

- 正式域名和 TLS/DNS 配置。
- Internal Beta 用户范围、权限分配和数据范围。
- Beta 观察期长度、SLO 判定窗口和发布负责人。

### 关闭条件

1. 将正式配置与机密写入仓库外、权限为 `0600` 的部署环境文件，并确认接受
   Docker 管理员可通过容器环境查看这些机密的风险。
2. 按 [Deploy.md](../Deploy.md) 完成迁移 gate、Doctor、API/Worker/Web 健康检查。
3. 在约定 Beta 范围和观察期内满足错误率、延迟、队列积压、Projection lag、恢复和安全门禁。
4. 问题 1–4 同时关闭后，才可给出生产发布授权。

## 参考

- [安全基线](security.md)
- [开发与部署指南](../Deploy.md)
- [可观测性](observability.md)
- [ADR-0013](adr/0013-defer-beta-gates-for-p1-development.md)
