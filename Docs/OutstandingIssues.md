# 待解决问题

> 更新时间：2026-10-30

主要业务链路与静态质量门禁已经闭环；以下项目仍阻塞“商业发布门禁全部关闭”或
“全部设计能力已完成真实联调”的结论。

## 1. 备份恢复与人工可访问性尚未在发布环境验收

### 1.1 现状

- 此前版本已在隔离环境启动 PostgreSQL、Redis、MinIO、Meilisearch、API、Linux Worker
  与 Web，并完成最新 Schema `up → down → up`、Doctor、真实数据联调、投影重建、
  dead 回放、Revision 冷归档回源和桌面/移动端浏览器回归。
- 当前生产 release 为 `d0285ff6b4e3`。六个业务镜像、Migration gate、Doctor、
  HTTPS smoke、API/Worker/Web/AI Kernel 健康检查和验收后垃圾清理均已通过；
  公网 `healthz`/`readyz` 为 200。
- Chrome headless 已覆盖 34 个真实数据路由、导入持久队列、治理流程、斜杠标题和
  390×844 移动端退化；完整键盘顺序、读屏器语义、对比度和正式网络失败态仍需要
  人工验收。
- Agent CLI 已部署到正式域名和生产 tools image；历史 149-operation 全量 transport
  验收、公开调用、请求校验和无效 Bearer 拒绝均已通过。当前契约已增长到 172 个
  operation；正式域名的授权码创建、兑换、受保护调用、撤销和撤销后 401 仍需再完成
  一次管理员冒烟。
- References/Related topics/Related outlines、信息框、私有页面和知识详情已在生产真实
  导入数据上完成登录态浏览器验证。尚未以当前 release 再执行一次全量 Projection
  重建并记录全部 Builder 的前后计数。
- 一键部署按当前运维决策不创建持久化备份，成功后还会删除旧业务镜像和 BuildKit
  缓存。手工 PostgreSQL/对象存储灾备脚本仍在，但生产恢复演练和 RPO/RTO 尚未记录。

### 1.2 关闭条件

1. 另行安排维护窗口，在远端生产等价隔离资源上显式运行 PostgreSQL 和对象存储备份/
   恢复演练并记录 RPO/RTO；不要把该步骤重新塞回日常一键部署。
2. 在浏览器人工覆盖正文、编辑、历史、搜索、治理和所有 Hub 的键盘、焦点、读屏、
   对比度，以及断网、超时和 5xx 状态。
3. 以当前 release 全量重建 Projection，记录重建前后 Builder 状态与页面计数，并确认
   References 编号/多回链、Related topics 原因、Related outlines 和信息框一致。
4. 管理员从正式 `/settings/cli` 创建一次性授权码，完成生产 CLI 兑换、受保护读调用、
   后台撤销与撤销后 401 验证。

## 2. 账号恢复与二次验证未完成

### 2.1 现状

- 已提供用户名/邮箱唯一、Argon2id 密码哈希、每账号私人 Wiki、服务端 Session
  及注册/登录/退出；账号恢复和私人 Wiki 转移尚未实现。
- 当前没有邮件发送基础设施，尚未验证邮箱所有权，也不提供忘记密码流程。
- 尚未提供 MFA、备用恢复码、登录设备列表与全部会话吊销。

### 2.2 关闭条件

1. 接入邮件供应商，实现邮箱验证、验证状态与重发限流。
2. 实现一次性、短时效、只存哈希的密码重置令牌，并在重置后吊销已有会话。
3. 提供 TOTP 或 WebAuthn MFA、恢复码和登录设备/会话管理。
4. 在正式域名验证注册关闭、登录、退出、私人 Wiki 隔离和账号停用。

## 3. TLS/HSTS 已终结，显式 CSRF 与证书恢复演练尚未完成

### 3.1 现状

- 生产 Compose 不终结 TLS；宿主机 Nginx 已为
  `anbywiki.momiya.cloud` 终结 HTTPS。页面代理到仅绑定回环地址的 Web，
  `/api/*` 与健康检查直送仅绑定 `127.0.0.1:60018` 的 Go API。
- `SESSION_COOKIE_SECURE=true`，正式 HTTPS/WSS 域名的登录和协作 E2E 已通过；
  当前响应提供一年期 HSTS。
- Session Cookie 使用 HttpOnly、Secure 与 `SameSite=Lax`，但 ADR-0020 已移除
  Origin/Referer 门禁；`SameSite=Lax` 不是完整 CSRF 防护。

### 3.2 关闭条件

1. 验证证书自动续期、失败告警和代理配置备份恢复。
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

## 5. 商业发布责任与 Beta 验收窗口未确定

### 5.1 缺少输入

- 正式域名和 Nginx 拓扑已落地，但证书续期告警、代理恢复和变更审批负责人未明确。
- Internal Beta 用户范围、账号停用和数据范围。
- Beta 观察期、SLO 判定窗口、告警接收人和发布负责人。

### 5.2 关闭条件

1. 将正式配置与机密写入仓库外、权限为 `0600` 的部署环境文件；替换所有占位值。
2. 按 [Deploy.md](../Deploy.md) 完成迁移 gate、Doctor、备份恢复、
   API/Worker/Web/Meilisearch 健康检查。
3. 在约定 Beta 范围和观察期内满足错误率、延迟、队列积压、Projection lag、
   搜索容量、恢复和安全门禁。
4. 问题 1–4 同时关闭后，才可给出“商业发布门禁全部关闭”的结论；当前受控生产实例
   的存在不等于这些验收已完成。

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

## 7. 普通导入的 AI Trust Actor 身份闭环尚未完成

### 7.1 现状

- AI Trust 档案、管理员页面、风险策略和审计写入已经实现，契约只允许
  `untrusted`、`assisted`、`trusted`。
- 已提供机器 `ai/import` Actor 的公开创建、列表与启停 API，以及 AI Trust 页创建
  入口；由真人管理员经 auth 领域服务写入并记录审计。Actor 使用所属 Wiki 和创建者
  local_account 绑定身份，创建重试幂等。
- Agent CLI 已支持在浏览器授权时选择本人创建的机器 Actor。机器 Token 可读取所属
  Wiki、创建和提交署名提案；停用会撤销凭证并清除授权码，启用后必须重新授权。
  每次使用重查创建者账号和管理员权限；最终机器 Proposal 始终真人审核。
- AI Trust 档案已按 Wiki 隔离，CLI Proposal 会命中三档策略和稳定抽样；抽样标记重点
  复核，未抽中的机器提案也不能跳过真人最终审核。
- 用户发起的 ImportJob 会以发起者 human Actor 创建 Proposal，因此当前普通导入不会
  命中 `AITrustService.ApplyPolicy` 的 `ai/import` 分支。页面可达和空状态正常，但不能
  把 AI Trust 概括为已进入用户导入闭环。

### 7.2 关闭条件

该项不阻塞当前全人工审核的 Internal Beta，但在承诺普通导入绑定机器身份前必须关闭：

1. 补齐普通 ImportJob 选择机器 Actor、human 发起者和 Proposal 作者之间的身份模型。
2. 让普通 Import Proposal 使用选定机器 Actor，同时保留 human 发起者的可见性与权限。
3. 增加该导入路径的信任、抽样及停用恢复 E2E；机器最终 Proposal 仍必须人工审核。

## 8. 复杂文档持续压力与供应商成本边界尚未验收

### 8.1 现状

- 2026-10-27 按 [MinerU 借鉴方案](MinerUReferencePlan.md) 将 PDF/图片升级为纯 LLM
  布局与区域识别两阶段流程。已有整页视觉版的生产验收仍为历史证据；新流程的真实
  供应商识别质量、细小区域、调用量、峰值内存和成本需要重新比较，尚未部署验收。
- 文本与视觉双轨 Parser、逐页 Poppler 渲染、页面分析 Schema、AI Kernel 图片输入、
  SourceVersion/SourceChunk 持久化、Extraction/ImportPlan 映射及个人中心文档理解
  模型配置探针均已实现。
- `18944a7` 已在正式域名使用真实多模态 Provider 完成三页论文、原生文本+扫描混合、
  纯扫描表格和扫描提示注入语料。成功路径生成的 Proposal 均已通过治理 API 拒绝；
  低于 0.70 的结果进入 `action_required`，提示注入在 Extraction 失败且不生成 Proposal。
- 三份各 3 页的关联架构、运维、事故文档已按正序和逆序合并验收。两次均得到 26 个
  Chunk、9 页覆盖、19 个区域定位和三个指定 create route；顺序扰动暴露的错误
  `updates` Claim 已由直接关系证据守卫修复，唯一前置主语的同 Chunk 代词证据仍可保留。
- 正序与逆序 dossier 拼接为 18 页后得到 50 个 Chunk、36 个区域定位、完整 1–18 页
  覆盖，仍只规划三个指定 create route；质量 0.690，安全停在 `action_required`。
- 关联语料经重复、重排、0/90/180/270 度旋转及 A4/Letter 混合尺寸后，12 页仍完整
  覆盖并只规划三个指定页面。中英双语 36 行跨页表格在正常和扰乱页序下均得到
  6 page、2 table、1 chart，并只规划一个指定页面。
- 带 33 个 EmbeddedFile 的跨页语料不会把附件内提示注入载荷写入 Chunk；同样措辞
  出现在可见正文时则稳定触发 `quality_gate`。密码加密 PDF 以 `parse_failed`
  原子失败且无 SourceVersion/Proposal。
- 畸形 PDF、501 页边界、上传幂等冲突、AI Kernel 不可用/恢复重试及在途 Worker
  Compose restart 均已通过。重试复用不可变 SourceVersion；Worker 在 parse 阶段收到
  restart 后等待 55 秒完成单一 Run，再健康启动。
- 三页并发视觉分析的目标机峰值为 API 145.2 MiB、AI Kernel 119.7 MiB、Web 45.3 MiB、
  Worker 20.2 MiB。Doctor、Outbox backlog/dead 与 Projection error 均为 0。
- Next.js 外部 rewrite 默认只缓冲 10 MiB，超限仍会转发截断请求，曾让 128 MiB
  上传表现为非 JSON 500。生产 Nginx 现直连回环 API 并关闭请求缓冲，Next 的
  301 MiB 上限只作兼容路径；修复后小 PDF 的 Session/multipart 201 与取消链路通过。
- multipart 现以 1 MiB 内存阈值落盘，MIME/恶意签名/摘要和对象存储写入均为流式；
  Worker 按对象元数据精确保留一份解析内容。单 API 实例上传并发默认为 1，第二个
  在途请求实测返回结构化 503，临时目录在请求后清零。
- 128 MiB/298 MiB 存储前 422 的 API anon 峰值为 75.9/80.0 MiB。298 MiB 完整
  Worker Job 在 20 秒内七阶段成功、质量 1.0，API/Worker 峰值 461.2/774.0 MiB，
  30 秒后回落为 150.3/312.8 MiB，无 OOM、重启或人工内存回收。
- 12×32 MiB 顺序预检共扫描约 384 MiB；12 次 422 后临时目录均清零，三个限流窗口
  各返回一次带 `Retry-After` 的 429，等待后恢复。API/Worker 峰值为
  184.2/25.74 MiB，权威计数不变且无 OOM、重启或 Doctor issue。
- 近 300 MiB 上限的长时上传/重试压力、异常退出后的临时文件清理、多副本全局并发
  策略，以及真实供应商超时/限流和账单成本仍未验证。

### 8.2 关闭条件

1. 在已完成 12×32 MiB 短窗口压力基础上，执行近上限长时上传和客户端重试压力，
   验证磁盘水位、503 重试、异常退出残留清理和服务恢复；水平扩展前定义跨
   API/Worker 副本的全局并发上限。
2. 用真实供应商超时、限流和配额耗尽场景验证告警、重试、错误分类与账单成本。
3. 在当前重复、旋转、混合尺寸及中英双语跨页表格/图表回归集基础上增加其他语言，
   并定义模型升级后的固定质量基线和抽样频率。

已完成证据见 [商业验收报告](CommercialAcceptance-2026-10-11.md)。

## 本轮已关闭

- 只读分享仍显示编辑器和离线同步提示：读取响应现提供页面级编辑权限，阅读页和
  编辑路由据此限制入口；协作拒绝访问返回 403，权限失效后停止编辑和自动重连。
  隔离 PostgreSQL 17 分享 E2E 与真实 Next.js/Chromium 验收通过，包含编辑中降权
  拒绝同步；本地修复尚未部署。
- CI 迁移与安全检查：第 4 号兼容迁移与完整初始化 Schema 重复创建
  `page_share_link` 的问题已修复；隔离 PostgreSQL 17 往返迁移及已有链接数据保留通过。
  `x/net 0.62.0` 修复 `GO-2026-6860`，Next.js `16.4.3` 与 Zod `4.6.7` 补齐后续
  npm 审计发现的漏洞；本地 `make check`、Go 测试和完整 `make security` 通过。
- 导入计划与匹配可靠性：Entity 身份归一化保留语义符号，缩写按全候选集合消歧；
  Claim 按完整值/有效期去重；人工 Entity 决定持久化；同批新页面可稳定互链；计划
  固化目标 Revision/Block hash，陈旧计划不会覆盖人工修改。
- 导入运行稳定性：修复 PostgreSQL `min(uuid)` 队列查询；缺少 AI 配置的 Wiki 不阻塞
  其他任务；不可恢复窗口会取消剩余工作；结构化 JSON 计入预算；质量门禁清理空路由、
  薄页面和导航噪声。
- 大型 AST Compose OOM：Block 校验改为按 `type` 直接分派子 Schema，生产内存从约
  4 GiB 降到约 15 MiB；空表格单元格改为合法空 inline 数组。
- 私有 Wiki 服务端详情鉴权：Page/History/Revision/Diff/Entity/Claim/Citation/
  Asset Revision Server Component 统一转发 Session Cookie，生产登录态与 API 200
  已验证。
- 部署垃圾与备份策略：日常部署停止创建持久化备份，成功后清理旧业务镜像、退出容器、
  BuildKit 缓存和遗留部署备份目录；权威数据卷保持不动。手工灾备恢复仍是问题 1。
- 私人 Wiki 多用户隔离：应用 release `962b8198425a` 已在生产从空库迁移并部署，
  旧 PostgreSQL、对象存储、搜索索引与 Redis 数据已删除。双账号实测确认 Wiki 与
  主 Namespace ID 不同、页面目录和内置 Component 隔离、外部 Page ID 返回 404、
  CLI Token 继承原 Wiki；匿名 API 返回 401，Web 根路径跳转登录，注册关闭后返回
  403。最终库仅保留一个引导账号私人 Wiki、七个 Namespace、一个内置 Component
  和六个 ChangeTag，无 Page/Source/Import/Proposal/CLI Token 或活动 Session。
- Agent CLI：Go CLI 已通过嵌入 OpenAPI 覆盖全部 HTTP operation，并单独覆盖 Yjs
  WebSocket；输入输出统一 JSON，path/query/header/body/response 有契约校验。浏览器
  后台可签发一次性授权码、查看和撤销 SHA-256 存储的 Bearer Token，权限实时继承账号。
  `0196ec6` 六镜像已部署，生产 CLI 的版本、149 operation、公开调用和校验已通过；
  登录态授权闭环保留为上述人工冒烟项。
- CLI 本地输入错误分类：缺失必填 body、非法 timeout、未知 operation 曾被包装成
  `request_failed`/exit 1；现按协议返回 `validation_failed` 或
  `operation_not_found`/exit 2，非法 logout 输入也不会误删本地 Token。
- CLI nullable enum 校验：OpenAPI `nullable: true` 与 `enum` 组合曾只扩展 type、
  未把 `null` 纳入 enum，导致合法的空 `change_batch_status` 被响应校验拒绝；
  规范化逻辑和回归测试已修复，隔离管理员 Token 的 149/149 operation 全量调用通过。
- Go `1.26.5`、`1.26.6` 与 `1.26.9` 后续披露的标准库可达漏洞：构建基线已升级到 `1.26.10`，
  `golang.org/x/net` 同步升级到 `0.60.0`；Nano ID 保持 `3.3.18`，
  Go/Web 依赖审计和 gitleaks 均恢复为 0。
- 初始化 down 在已有 Page 时会提前删除 Namespace 种子并触发 FK：已移除冗余的提前
  seed DELETE，含真实页面的隔离库 `down → up` 通过。
- 全 API 修复生产部署：`2e14c32` 已完成五镜像构建、Migration gate、Doctor 与
  API/Worker/Web/AI Kernel 滚动替换；缺失的 `WEB_BIND/WEB_PORT` 已持久化回部署
  环境文件，正式域名和 `127.0.0.1:4444` 绑定验证通过。
- 149 个 OpenAPI operation 的动态审计：隔离全栈已逐个命中 handler，并完成核心、
  治理、BulkReview、Import/AI 配置与双会话协作成功工作流；故意不可达模型地址按
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
- 生产 Compose/OCI 验收：远端生产机已完成 `0196ec6` 六个本地业务镜像构建、迁移
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
- Web production high 漏洞：Next 已升级到 `16.3.8`，PostCSS `8.5.25`、
  Sharp `0.35.5` 与 `source-map-js` `1.2.2` 已固定；production audit、类型、Lint
  与构建通过。
- Go 可达漏洞：gRPC `1.83.1`、OpenTelemetry `1.45.0` 等升级后，
  `govulncheck` 为 0。
- PostgreSQL-only 搜索架构：Meilisearch Adapter、生产服务、Worker 投影提交、
  混合/语义查询与前端工作台均已实现并完成真实实例联调；仅保留目标环境容量
  与召回质量验收。
- 最新初始化 Schema 与真实依赖链：PostgreSQL 16 空库 `up → down → up` 通过，
  API/Worker/Web/Redis/MinIO/Meilisearch 联调和 Doctor 均已完成。
- 可重建投影与异步恢复：全量重建 0 失败，一致性状态 30/30，旧
  `claim.changed` dead 事件兼容回放成功，当前 Outbox 为 0 backlog/0 dead。
- 多格式导入基础链路：文本、JSON、CSV、图片 OCR 与旧扫描 PDF 本地 OCR 均通过真实
  解析；文本、PNG、JSON、CSV Job 经完整 Worker 管线生成 Proposal，浏览器重开后仍
  可找回。当前复杂 PDF 双轨链路的真实多模态验收见问题 8。
- Revision 冷热分层：非当前快照经领域服务写入 MinIO 并切换 cold tier，历史读取
  端点完成哈希校验和透明回源。账户归档页的数量、容量、候选和手动操作已限定为
  当前账户拥有的 Wiki；跨 Wiki 共用快照保留给后台 Worker，手动操作在行锁内复核范围。
  隔离 PostgreSQL 17/RustFS 验证双账号、协作 Wiki、空账户、共享/无引用快照、
  当前版本保护与冷历史回源/回滚，浏览器验证账户缓存隔离与移动端布局；
  此账户隔离修复尚未提交或部署。
- 个人中心模型配置范围与状态：读取、保存和连接测试均限定为当前账户拥有的私人
  Wiki，协作 Wiki 选择不改变目标；个人资料和模型配置缓存按账户隔离，未保存密钥时
  正确显示“尚未配置”。模型最大输入 Token 与来源 Chunk 字符数均允许
  `12,800～20,000,000`，默认值为 `128,000` / `32,000`。
  `make check`、全量 Go 测试、隔离 PostgreSQL 17 E2E 与真实浏览器验证通过，
  覆盖跨账户缓存、两端值、越界拒绝及留空密钥保留；实际供应商上下文容量需按模型确认。
  此配置修复尚未提交或部署。
- 工作台入口与后端权限不一致：导入中心及新建提案按账户所属 Wiki 过滤，
  创建、专题、审核、应用、审计和管理入口按实际角色过滤；侧栏、页头、页脚、
  CmdK 与页面内链接一致。受限 URL 保留页面及权限提示，禁止挂载相应工作台；
  提案详情 403 提供提示。真实浏览器验证多角色、Wiki 切换、分享阅读/编辑、
  受限工作台零请求及移动端布局通过；本次修复尚未提交或部署。

- 组件定制入口难用、缺少效果示例：组件中心已补五套带示例数据的模板、
  字段表单与即时示意；草稿/预览/插入共用配置，编辑器可选择自定义已发布组件。
  隔离 PostgreSQL 17 与真实 Chromium 验证创建、版本冻结、动态事实、实际页面
  发布阅读、复杂 Schema 无损保留、失败重试、协作 Wiki 权限与移动端布局。
  当前仍只有两种可信渲染器，示例是配置模板；任意自定义布局或脚本执行未开放。

- 导入失败只展示 `stage/code`，无法判断具体原因：已补 `ImportFailure` v1 的原因、
  建议、重试语义及安全诊断，并同步到 Job/Run/Stage；旧抽取门禁失败按各次运行的
  不可变提取记录补充提示注入标记、模型、质量分数与复用信息，不改历史数据。
  队列、阶段、历史和请求错误使用可读提示，JSON 默认折叠；未记录的阶段异常及时收尾，
  取消与新一轮运行不会被晚到错误覆盖。隔离 PostgreSQL/真实 API/生成客户端、
  全量 Go 测试、`make check` 和 320/390/960px 浏览器验收通过。
  疑似提示注入仍需核查来源，角色设定等资料可能被误判；此次诊断改进不消除模型误判。

## 参考

- [安全基线](security.md)
- [开发与部署指南](../Deploy.md)
- [可观测性](observability.md)
- [搜索 Adapter ADR](adr/0006-search-adapter.md)
