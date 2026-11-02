# 待解决问题

> 更新时间：2026-11-02

本文只保留尚未完成的实现、环境验收和责任确认。已交付功能统一描述在
[当前实现状态](CurrentImplementationStatus.md)，历史测试数据见
[商业验收证据](CommercialAcceptance-2026-10-11.md)。问题关闭后，将可复用的功能描述
补入相应文档，并从本文移除。

## 2. 账号安全的正式邮件与域名验收

系统管理台、独立系统权限、SMTP 配置、邮箱验证与变更、密码恢复、
TOTP、一次性恢复码和设备会话管理已实现；账号恢复与私人 Wiki 支持边界见
[账号安全指南](AccountSecurity.md) 和 [ADR-0025](adr/0025-account-security-and-system-administration.md)。
生产备份、显式 Schema 补齐、首个系统管理员授权及正式 HTTPS 登录/页面/接口冒烟
已完成，见 [发布证据](AccountSecurityProductionAcceptance-2026-11-02.json)。
SMTP 尚未配置，本地真实 PostgreSQL/SMTP 验证不代表生产邮件与完整恢复链路已验收。

关闭条件：

1. 在管理台配置真实邮件供应商；生产备份、Schema 补齐和系统管理员指定已完成。
2. 正式 HTTPS 域名验证邮件实际收件、验证与变更邮箱、MFA/恢复码登录及密码恢复。
3. 恢复后确认旧浏览器会话和本人/机器 CLI 凭证失效，原私人 Wiki 与成员/分享关系保持隔离。

## 参考

- [当前实现状态与功能描述](CurrentImplementationStatus.md)
- [历史商业验收证据](CommercialAcceptance-2026-10-11.md)
- [安全基线](security.md)
- [开发与部署指南](../Deploy.md)
- [可观测性](observability.md)
- [搜索 Adapter ADR](adr/0006-search-adapter.md)
