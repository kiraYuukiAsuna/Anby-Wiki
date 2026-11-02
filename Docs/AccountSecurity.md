# 账号安全与系统管理

系统管理台为 `/admin`；个人中心为 `/account`。
系统管理员和 Wiki 管理员是两种独立权限。系统管理员可以配置全站邮件、
查看账号安全概况及审计、授权其他系统管理员，不能代替用户恢复密码或读取私人 Wiki。

## 启用与存量升级

先备份现有数据库。在部署新版 API 前，对版本 4 存量数据库显式执行：

```sh
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/upgrade-account-security.sql
```

此脚本只添加账号安全列和表，可重复执行；不改变迁移版本，不执行生产 down。
新建数据库的完整初始化已包含相同 Schema。
所有 API 实例使用一致的 `AI_CONFIG_MASTER_KEY`（已有 32 字节 base64 基础设施密钥）。
备份和恢复时必须同时恢复数据库及此密钥，丢失密钥会使 TOTP 与 SMTP 密码无法解密。
不要通过更换密钥静默关闭已有 MFA。

通过运维命令显式指定首个系统管理员，执行环境须已加载 `.env`：

```sh
go run ./cmd/system-admin -username USER
```

该命令在 `backend` 目录执行；生产迁移工具镜像提供 `wiki-system-admin`，
使用同一环境执行 `wiki-system-admin -username USER`。无需修改注册开关，
不需要把账号密码写到 CLI 参数。运维撤销添加 `-revoke`，
最后一名有效管理员受相同保护；每次权限变化均由领域服务记录审计。

## 邮件配置

管理员打开系统管理台，填写 SMTP 主机、端口、TLS 模式、用户名、授权码/密码、
发件地址、发件名和本站 HTTPS 地址，启用并保存。保存与发送测试要求当前账号密码，
已启用 MFA 时还需新的验证码或一次性恢复码。测试只发送到管理员本人的账号邮箱。
SMTP 密码留空保留已有值；管理台和 API 永不返回 SMTP 密码。
465 通常选择 TLS，587 通常选择 STARTTLS；禁止供应商 TLS 失败后回退明文。

供应商拒绝登录、网络超时、发件限制等统一返回邮件配置不可用，失败事务不使已有
可用验证链接失效。忘记密码的响应始终不透露目标账号是否存在；
只有已有验证邮箱的有效账号收到邮件。生产运维应通过受控测试邮件检查实际投递、
垃圾箱、SPF/DKIM/DMARC 及供应商配额，不能把 SMTP 接受等同于用户收件成功。

## 个人账号流程

- 邮箱验证：个人中心发送验证邮件，30 分钟内点击邮件链接并确认。
  重发一分钟最多一次，每账号每种邮件每日最多 20 次。
- 邮箱变更：重新验证当前密码及 MFA，在个人中心填写新邮箱；验证新邮箱后才替换，
  并撤销旧设备和 CLI 凭证。
- 密码恢复：登录页“忘记密码”，输入用户名或邮箱，15 分钟内打开最新邮件；
  设置 12–128 字符新密码。已启用 MFA 时必须提供验证码或恢复码。
  成功后全部旧设备会话、本人/机器 CLI Token 和待兑换授权码失效。
- MFA：在个人中心重新认证，手动将显示的密钥添加到验证器或使用验证器链接，
  输入 6 位验证码确认绑定。保存只显示一次的十个恢复码。一个验证码不可重复使用；
  紧接着执行另一个敏感操作时需等下一个 30 秒时间步或使用恢复码。
- 恢复码：验证器丢失时用于登录及安全操作。登录后可重新生成恢复码或停用 MFA
  再重新绑定。重新生成后旧码全部失效；停用 MFA 要求密码及现有第二凭证。
- 登录设备：个人中心显示浏览器标识、活动/到期时间和当前设备，可单个撤销，
  或重新认证后撤销全部会话和 CLI 凭证。浏览器标识仅用于辨认，不能证明物理设备身份。

验证邮件中的令牌在 URL fragment，页面读取后从地址栏移除。
仅 POST 确认产生修改，不通过 GET 自动消费。敏感 API 返回 `Cache-Control: no-store`。
安全审计只记录动作、Actor、目标和时间。

## 支持与验收边界

账号恢复不会创建新账号、改变 Actor ID、私人 Wiki 所有者或分享/成员关系。
邮箱和全部 MFA/恢复码均丢失时，不提供人工绕过。系统管理员也不能跳过这些验证。
私人 Wiki 不支持跨账号所有者转移；原所有者可主动使用既有协作和导入能力，
所有 Proposal 最终仍需人工审核。

本地自动化覆盖真实 PostgreSQL、本机 SMTP、哈希令牌单次/并发消费、
TOTP RFC 向量与重放、恢复码、密码恢复后的浏览器/CLI 撤销，以及完整 Schema up/down/up。
存量升级在隔离库中还原旧账号/会话表结构后执行两次，原账号密码与已有会话保持有效。

2026-11-02 使用隔离账号、本机 SMTP 和真实 Chrome 完成 18 项页面验收：
管理台保存/测试邮件及密码不回显、最后管理员保护、系统权限授予与本人撤销后刷新、
邮箱验证/变更、TOTP 绑定、恢复码登录与重新生成、保留 MFA 的密码恢复、
停用 MFA、修改密码、单设备及全部会话撤销、普通 Wiki 管理员拒绝访问系统邮件配置。
个人中心和管理台分别通过 320/390px 宽度检查，无水平溢出；没有未捕获页面异常，
邮件 fragment 令牌没有进入网络 URL。验收修复了开发模式重复初始化导致令牌状态被清空的问题。
测试实例、账号所在隔离 Schema 和临时邮件均已清理。

本轮不代表生产供应商邮件和正式 HTTPS 域名已验收。
正式验收需实测邮件投递、邮箱验证/变更、MFA/恢复码登录、密码恢复、
旧会话/机器 CLI 失效及恢复后私人 Wiki 隔离；记录在独立部署验收证据中。

## 本轮交付记录

- 后端：`backend/internal/auth/{security,mail,service,cli}.go`、
  `backend/cmd/api/{account_security,auth,middleware,server,main}.go`、
  `backend/cmd/system-admin/main.go`、`backend/internal/governance/user_admin.go`，
  完成领域服务、HTTP 接入、引导命令和权限保护。
- 前端：`apps/web/app/{admin,forgot-password,reset-password,verify-email}`、
  登录表单、`components/account`、`components/admin/system-console.tsx`、
  `lib/account-security.ts` 及会话导航，提供完整个人与系统设置入口。
- 契约与升级：`contracts/openapi/openapi.yaml`、生成 TypeScript 客户端、
  CLI 契约副本及操作覆盖测试、初始化 up/down、`scripts/upgrade-account-security.sql`、
  `Dockerfile`，保持契约一致并提供显式存量升级。
- 文档：本指南、ADR-0025、实现状态与待解决问题已同步。`make check`、认证/API/治理 race、
  CLI 测试及上述真实页面验收通过；本机无 Docker，真实镜像/Compose 动态检查未执行。
- 数据库 Schema 与根构建文件修改已结束，无高冲突资源继续持有。
  后续仅需正式部署与邮件/域名验收，关闭条件见 `Docs/OutstandingIssues.md` 第 2 项。
