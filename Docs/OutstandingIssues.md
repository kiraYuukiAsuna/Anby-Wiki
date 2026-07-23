# 待解决问题

当前研发方案已完成，但以下两个问题仍阻塞生产发布。

## 1. Web 生产依赖高危漏洞

### 现状

- 当前 Next.js 为 `16.2.11`，其依赖链仍包含存在 high 漏洞的 `sharp 0.34.5`。
- 安全版本要求 `sharp >=0.35.0`，但当前 Next.js 发布版本尚未提供兼容升级路径。
- Next.js 内嵌的 `postcss <=8.5.11` 也命中 high，安全版本要求 `>=8.5.22`。
- `npm audit fix --force` 只提供破坏性降级方案，不能用于生产修复。
- Go `govulncheck` 当前没有可达漏洞。

### 关闭条件

1. 上游 Next.js 发布包含安全版本 `sharp` 和 `postcss` 的兼容版本，或提供受支持的升级路径。
2. 升级 Next.js 及锁文件，不使用 unsupported override 或强制降级。
3. 重新通过 `npm audit --omit=dev --audit-level=high`、Web 全量测试和生产构建。
4. 更新 [安全基线](security.md) 并记录实际修复版本。

## 2. 生产发布输入与 Beta 验收未完成

### 缺少输入

- 正式域名和 TLS/DNS 配置。
- OIDC issuer、client ID、client secret 与正式 redirect URL。
- Internal Beta 用户范围、权限分配和数据范围。
- Beta 观察期长度、SLO 判定窗口和发布负责人。

### 关闭条件

1. 将正式非密钥配置写入仓库外的部署环境文件，密钥写入外部 Secret Store。
2. 验证 OIDC 登录、回调、退出、Actor 停用和角色撤销在正式域名下生效。
3. 按部署 runbook 完成迁移 gate、Doctor、API/Worker/Web/Nginx 健康检查。
4. 在约定 Beta 范围和观察期内满足错误率、延迟、队列积压、Projection lag、恢复和安全门禁。
5. 安全问题 1 同时关闭后，才可给出生产发布授权。

## 参考

- [安全基线](security.md)
- [部署 Runbook](runbooks/deployment.md)
- [可观测性](observability.md)
- [ADR-0013](adr/0013-defer-beta-gates-for-p1-development.md)
