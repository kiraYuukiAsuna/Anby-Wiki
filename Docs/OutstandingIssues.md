# 待解决问题

设计能力与静态质量门禁已经闭环；以下项目仍阻塞生产发布或“已完成真实联调”的
结论。

## 1. 生产容器与人工可访问性尚未在发布环境验收

### 现状

- 本轮已在隔离环境启动 PostgreSQL、Redis、MinIO、Meilisearch、API、Linux Worker
  与 Web，并完成最新 Schema `up → down → up`、Doctor、真实数据联调、投影重建、
  dead 回放、Revision 冷归档回源和桌面/移动端浏览器回归。
- Docker daemon 不可用；生产部署静态检查与 YAML 解析通过，但 Compose 展开、
  healthcheck 和四个 OCI target 未在本机实跑。
- Chrome headless 已覆盖 34 个真实数据路由、导入持久队列、治理流程、斜杠标题和
  390×844 移动端退化；完整键盘顺序、读屏器语义、对比度和正式网络失败态仍需要
  人工验收。

### 关闭条件

1. 在具备 Docker daemon 的 CI/发布机通过 Compose config、四个 target 构建、
   非 root 用户及 healthcheck 元数据校验。
2. 用发布清单启动一次完整容器拓扑并运行迁移 gate、Doctor、备份恢复和服务
   healthcheck。
3. 在浏览器人工覆盖正文、编辑、历史、搜索、治理和所有 Hub 的键盘、焦点、读屏、
   对比度，以及断网、超时和 5xx 状态。

## 2. 账号恢复与二次验证未完成

### 现状

- 已提供用户名/邮箱唯一、Argon2id 密码哈希、独立 Actor、服务端 Session、
  注册/登录/退出和账号停用即时生效。
- 当前没有邮件发送基础设施，尚未验证邮箱所有权，也不提供忘记密码流程。
- 尚未提供 MFA、备用恢复码、登录设备列表与全部会话吊销。

### 关闭条件

1. 接入邮件供应商，实现邮箱验证、验证状态与重发限流。
2. 实现一次性、短时效、只存哈希的密码重置令牌，并在重置后吊销已有会话。
3. 提供 TOTP 或 WebAuthn MFA、恢复码和登录设备/会话管理。
4. 在正式域名验证注册关闭、登录、退出、账号停用和角色撤销。

## 3. 默认无 TLS，显式 CSRF 防护未恢复

### 现状

- 生产 Compose 不终结 TLS；`web` 是唯一发布端口的服务。
- 明文公网暴露时，登录凭据和 Session Cookie 可被网络观察者读取。
- Session Cookie 使用 HttpOnly 与 `SameSite=Lax`，但 ADR-0020 已移除
  Origin/Referer 门禁；`SameSite=Lax` 不是完整 CSRF 防护。

### 关闭条件

1. 在云 LB、Cloudflare 或独立代理终结 HTTPS，并启用 HSTS。
2. 设置 `SESSION_COOKIE_SECURE=true`。
3. 采用同步 CSRF Token、Fetch Metadata 或等价可信来源校验，并完成浏览器验证。
4. 若按最终客户端 IP 限流，先由外层清洗 `X-Forwarded-For`，再只把 API 的可信
   直连对端加入 `TRUSTED_PROXY_IPS`。

## 4. Meilisearch 容量与语义质量尚未在目标环境验收

### 现状

- PostgreSQL-only 的结构性容量阻塞已经解除：生产路径已接入 Meilisearch，支持
  关键词、facet、混合与语义检索，并可从 PostgreSQL staging 重建。
- 本轮使用真实 Meilisearch 与本地 HuggingFace embedder 验证了关键词、语义、
  混合查询、远端索引任务等待和全量投影重建；功能链路已闭环。
- 尚未按 ADR-0012 的 10 万页面同口径复测吞吐、P95/P99、索引任务延迟和重建
  时间；中文召回质量、内存占用和无外网模型预置策略也尚未在目标硬件确认。

### 关闭条件

1. 在目标规格上导入 10 万页面并运行关键词/过滤/混合/语义基准。
2. 记录吞吐、P50/P95/P99、索引积压、全量重建时间、内存和磁盘水位。
3. 用固定中文查询集评估语义召回，并定义无模型或模型下载失败时的显式降级策略。
4. 验证旧 Revision 重试不能覆盖新索引，删除 Page 会同步移除远端文档。

## 5. 正式发布输入与 Beta 验收未确定

### 缺少输入

- 正式域名、TLS/DNS 边界和最终网络拓扑。
- Internal Beta 用户范围、角色分配和数据范围。
- Beta 观察期、SLO 判定窗口、告警接收人和发布负责人。

### 关闭条件

1. 将正式配置与机密写入仓库外、权限为 `0600` 的部署环境文件；替换所有占位值。
2. 按 [Deploy.md](../Deploy.md) 完成迁移 gate、Doctor、备份恢复、
   API/Worker/Web/Meilisearch 健康检查。
3. 在约定 Beta 范围和观察期内满足错误率、延迟、队列积压、Projection lag、
   搜索容量、恢复和安全门禁。
4. 问题 1–4 同时关闭后，才可给出生产发布授权。

## 本轮已关闭

- Web production high 漏洞：Next `16.2.12` 的实际 PostCSS/Sharp 依赖已固定到
  安全版本，完整及 production-only `npm audit` 均为 0，类型、Lint 与构建通过。
- Go 可达漏洞：gRPC `1.82.1`、OpenTelemetry `1.44.0` 等升级后，
  `govulncheck` 为 0。
- PostgreSQL-only 搜索架构：Meilisearch Adapter、生产服务、Worker 投影提交、
  混合/语义查询与前端工作台均已实现并完成真实实例联调；仅保留目标环境容量
  与召回质量验收。
- 最新初始化 Schema 与真实依赖链：PostgreSQL 16 空库 `up → down → up` 通过，
  API/Worker/Web/Redis/MinIO/Meilisearch 联调和 Doctor 均已完成。
- 可重建投影与异步恢复：全量重建 0 失败，一致性状态 30/30，旧
  `claim.changed` dead 事件兼容回放成功，当前 Outbox 为 0 backlog/0 dead。
- 多格式导入：文本、JSON、CSV、图片 OCR 与扫描 PDF OCR 均通过真实解析；文本、
  PNG、JSON、CSV Job 经完整 Worker 管线生成 Proposal，浏览器重开后仍可找回。
- Revision 冷热分层：非当前快照经领域服务写入 MinIO 并切换 cold tier，历史读取
  端点完成哈希校验和透明回源。

## 参考

- [安全基线](security.md)
- [开发与部署指南](../Deploy.md)
- [可观测性](observability.md)
- [搜索 Adapter ADR](adr/0006-search-adapter.md)
