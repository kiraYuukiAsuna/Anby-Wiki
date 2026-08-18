# 待解决问题

主要业务链路与静态质量门禁已经闭环；以下项目仍阻塞生产发布或“全部设计能力已
完成真实联调”的结论。

## 1. 备份恢复与人工可访问性尚未在发布环境验收

### 1.1 现状

- 本轮已在隔离环境启动 PostgreSQL、Redis、MinIO、Meilisearch、API、Linux Worker
  与 Web，并完成最新 Schema `up → down → up`、Doctor、真实数据联调、投影重建、
  dead 回放、Revision 冷归档回源和桌面/移动端浏览器回归。
- 远端生产机已对提交 `c84233c` 完成 Compose config、五个 OCI target 顺序构建、
  Migration gate、Doctor 和完整拓扑滚动替换；全部数据/应用容器及 Web/API 探针健康。
- Chrome headless 已覆盖 34 个真实数据路由、导入持久队列、治理流程、斜杠标题和
  390×844 移动端退化；完整键盘顺序、读屏器语义、对比度和正式网络失败态仍需要
  人工验收。
- 新增的 References/Related topics/Related outlines/信息框阅读结构已通过本地构建与
  隔离迁移验证，但尚未在生产现有数据上完成投影全量重建、真实导入和浏览器验收。

### 1.2 关闭条件

1. 在远端生产拓扑完成 PostgreSQL 和对象存储备份恢复演练并记录 RPO/RTO；当前迁移、
   Doctor 与服务 healthcheck 已通过。
2. 在浏览器人工覆盖正文、编辑、历史、搜索、治理和所有 Hub 的键盘、焦点、读屏、
   对比度，以及断网、超时和 5xx 状态。
3. 部署后全量重建 Projection，确认 References 编号/多回链、Related topics 原因、
   Collection 归属形成的 Related outlines 和信息框在新旧页面及章节懒加载路径上一致。

## 2. 账号恢复与二次验证未完成

### 2.1 现状

- 已提供用户名/邮箱唯一、Argon2id 密码哈希、独立 Actor、服务端 Session、
  注册/登录/退出和账号停用即时生效。
- 当前没有邮件发送基础设施，尚未验证邮箱所有权，也不提供忘记密码流程。
- 尚未提供 MFA、备用恢复码、登录设备列表与全部会话吊销。

### 2.2 关闭条件

1. 接入邮件供应商，实现邮箱验证、验证状态与重发限流。
2. 实现一次性、短时效、只存哈希的密码重置令牌，并在重置后吊销已有会话。
3. 提供 TOTP 或 WebAuthn MFA、恢复码和登录设备/会话管理。
4. 在正式域名验证注册关闭、登录、退出、账号停用和角色撤销。

## 3. TLS 已终结，显式 CSRF 与 HSTS 尚未恢复

### 3.1 现状

- 生产 Compose 不终结 TLS；宿主机 Nginx 已为
  `anbywiki.kirayuukiasuna.cloud` 终结 HTTPS，并代理到仅绑定
  `127.0.0.1:4444` 的 Web。
- `SESSION_COOKIE_SECURE=true`，正式 HTTPS/WSS 域名的登录和协作 E2E 已通过；
  当前响应尚未提供 HSTS。
- Session Cookie 使用 HttpOnly、Secure 与 `SameSite=Lax`，但 ADR-0020 已移除
  Origin/Referer 门禁；`SameSite=Lax` 不是完整 CSRF 防护。

### 3.2 关闭条件

1. 为正式域名启用 HSTS，并验证证书自动续期、失败告警和代理配置备份恢复。
2. 采用同步 CSRF Token、Fetch Metadata 或等价可信来源校验，并完成浏览器验证。
3. 若按最终客户端 IP 限流，先由外层清洗 `X-Forwarded-For`，再只把 API 的可信
   直连对端加入 `TRUSTED_PROXY_IPS`。

## 4. Meilisearch 容量与语义质量尚未在目标环境验收

### 4.1 现状

- PostgreSQL-only 的结构性容量阻塞已经解除：生产路径已接入 Meilisearch，支持
  关键词、facet、混合与语义检索，并可从 PostgreSQL staging 重建。
- 本轮使用真实 Meilisearch 与本地 HuggingFace embedder 验证了关键词、语义、
  混合查询、远端索引任务等待和全量投影重建；功能链路已闭环。
- 尚未按 ADR-0012 的 10 万页面同口径复测吞吐、P95/P99、索引任务延迟和重建
  时间；中文召回质量、内存占用和无外网模型预置策略也尚未在目标硬件确认。
- 远端 API 重建时曾有一次 Meilisearch 索引配置任务在单次 15 秒 HTTP 超时内未返回，
  进程由 restart policy 重启后恢复 healthy；常驻 Worker 指标和后续日志正常。

### 4.2 关闭条件

1. 在目标规格上导入 10 万页面并运行关键词/过滤/混合/语义基准。
2. 记录吞吐、P50/P95/P99、索引积压、全量重建时间、内存和磁盘水位。
3. 用固定中文查询集评估语义召回，并定义无模型或模型下载失败时的显式降级策略。
4. 验证旧 Revision 重试不能覆盖新索引，删除 Page 会同步移除远端文档。
5. 调整索引配置启动重试与单次请求超时，避免健康服务因慢 task 轮询产生一次性退出。

## 5. 正式发布输入与 Beta 验收未确定

### 5.1 缺少输入

- 正式域名证书续期负责人、Nginx 配置恢复责任和最终网络拓扑变更流程。
- Internal Beta 用户范围、角色分配和数据范围。
- Beta 观察期、SLO 判定窗口、告警接收人和发布负责人。

### 5.2 关闭条件

1. 将正式配置与机密写入仓库外、权限为 `0600` 的部署环境文件；替换所有占位值。
2. 按 [Deploy.md](../Deploy.md) 完成迁移 gate、Doctor、备份恢复、
   API/Worker/Web/Meilisearch 健康检查。
3. 在约定 Beta 范围和观察期内满足错误率、延迟、队列积压、Projection lag、
   搜索容量、恢复和安全门禁。
4. 问题 1–4 同时关闭后，才可给出生产发布授权。

## 6. 非阻断协作演进项

### 6.1 现状

- 在线 Yjs update、重连游标、AI Proposal 合并和普通 WorkingDocument 发布均已使用
  durable sequence。普通发布会在同一事务检查 `expected_sequence`，前端也会等待
  本地 update 的服务端回显并从同一 Y.Doc 物化 AST。
- 同一标签页短暂断线时，未确认 Yjs update 会保留原幂等 ID，在恢复远端 update 后
  合并并重发；浏览器关闭或重载后的恢复仍依赖完整 AST 草稿，不是持久化 Yjs 操作日志。
- Presence 已接入心跳、服务端排除发送者广播、过期清理和 Block 级位置展示；目前显示
  Actor 短 ID 与 Block 短 ID，不包含名称解析或字符级光标装饰。
- WorkingDocument 已在每 100 个确认 sequence 后自动上传 snapshot，并在同一事务
  compact covered update；远端 E2E 已验证旧游标恢复 snapshot 且不再返回被覆盖 update。
- 实时 Hub 仅在单 API 进程内广播。当前生产 Compose 是单 API 实例，但水平扩展前需要
  跨实例广播或 WorkingDocument 级连接粘性。

### 6.2 关闭条件

这些能力不阻塞当前单 API Internal Beta：

1. 首版只承诺同一标签页瞬时断线的 CRDT update 合并；浏览器关闭或重启后明确降级为
   AST 草稿恢复和人工冲突处理。未来若扩展承诺，再持久化未确认 Yjs update。
2. Block 级 Presence 满足首版范围；Actor 名称解析、字符位置协议和编辑器 Decoration
   作为体验增强。
3. 当前生产清单明确保持单 API 实例；进入水平扩展里程碑前必须实现跨实例广播或
   WorkingDocument 级连接粘性，并新增多副本 E2E。

## 7. AI Trust Actor 身份闭环尚未完成

### 7.1 现状

- AI Trust 档案、管理员页面、风险策略和审计写入已经实现，契约只允许
  `untrusted`、`assisted`、`trusted`。
- 档案只列出并更新预先存在的 `ai/import` Actor；本地账号注册只创建 human Actor，
  当前没有公开 API 或部署命令负责 provision 这两类 Actor。
- 用户发起的 ImportJob 会以发起者 human Actor 创建 Proposal，因此当前普通导入不会
  命中 `AITrustService.ApplyPolicy` 的 `ai/import` 分支。页面可达和空状态正常，但不能
  把 AI Trust 概括为已进入用户导入闭环。

### 7.2 关闭条件

该项不阻塞当前全人工审核的 Internal Beta，但在承诺 AI 分级抽样或自动批准前必须关闭：

1. 冻结 AI/import Actor、Job 发起者和 Proposal 作者之间的身份/归属模型。
2. 提供经领域服务和审计保护的 provision/disable/rotation 运维入口，不直接改表。
3. 让 Import/AI Proposal 使用对应机器 Actor，同时保留 human 发起者的可见性与权限。
4. 增加 `untrusted/assisted/trusted` 三档策略、抽样命中和管理员更新成功路径 E2E。

## 本轮已关闭

- 全 API 修复生产部署：`2e14c32` 已完成五镜像构建、Migration gate、Doctor 与
  API/Worker/Web/AI Kernel 滚动替换；缺失的 `WEB_BIND/WEB_PORT` 已持久化回部署
  环境文件，正式域名和 `127.0.0.1:4444` 绑定验证通过。
- 144 个 OpenAPI operation 的动态审计：隔离全栈已逐个命中 handler，并完成核心、
  治理、BulkReview、Import/AI 配置与双用户协作成功工作流；故意不可达模型地址按
  契约返回 502，其余没有非预期 5xx。
- `create_entity` Schema 合法 payload 曾因严格解码结构缺少 `language/description`
  在 Apply 预检误报 422；运行时结构与 Schema 已对齐并增加回归测试。
- ImportJob 在 Parse 后、模型失败前曾不暴露已持久化 SourceVersion；Parse stage
  完成/跳过与 `source_version_id` 现已同事务提交，失败详情和重试均复用该恢复点。
- 普通协作发布缺少 sequence 防护：OpenAPI、生成客户端、Web 和 Go 发布事务现要求
  `working_document_id + expected_sequence`；sequence 不一致返回 409，前端重新恢复
  WorkingDocument。未确认 update 会按稳定幂等 ID 跨瞬时断线重发，Presence 形成
  Block 级最小 UI 闭环；远端双用户 E2E 已覆盖广播、幂等重放、断线恢复与发布 CAS。
- WorkingDocument 自动压缩：浏览器在 100 个已确认 sequence 后上传 Yjs state，
  服务端保存 snapshot 并原子 compact covered update；远端恢复 E2E 已通过。
- 首版协作范围已冻结：跨浏览器重启离线 CRDT 与多 API 实时广播不属于当前
  Internal Beta 承诺，分别降级为 AST 草稿/人工冲突和单 API 部署约束。
- 生产 Compose/OCI 验收：远端生产机已完成 `c84233c` 五个本地业务镜像构建、迁移
  gate、Doctor、滚动替换和全服务 healthcheck；仅保留备份恢复与人工可访问性验收。
- BlockEditor 的 Client Component 预渲染曾因 BlockNote 访问 `window` 返回 500；
  真实编辑会话与开发验证页现均通过 `next/dynamic({ssr:false})` 加载，浏览器冒烟已
  验证编辑器加载和 AST 更新。
- 导入 Proposal 的身份占用误报 500：Apply 现会预检并持久化 Page/Entity 身份
  冲突，并发唯一索引竞态映射为 409；批量审核保留分类原因并停止重试不可变的
  过期提案，Web 提示改为可操作的“基于 Current 重新导入”。
- 2026-08 JavaScript 新披露漏洞：生产链 `nanoid` 与开发工具链 `fast-uri`、Hono、
  `ip-address`、`js-yaml`、Undici 均锁定已修复版本；完整和 production-only
  `npm audit` 恢复为 0。
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
