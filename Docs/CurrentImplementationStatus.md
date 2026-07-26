# 当前实现状态

> 更新时间：2026-07-27
> 依据：[整体设计方案](WikiDesignOnePage.md) 与 [实施方案](WikiImplementationPlan.md)

## 总体结论

实施方案 M0～M9 的研发内容均已完成。当前代码包含 Go API、异步 Worker、Next.js Web、
PostgreSQL 权威数据、Outbox 投影、治理审核、协作编辑和 P1 扩展能力。

“研发完成”不等于“生产发布就绪”。生产发布仍受
[待解决问题](OutstandingIssues.md) 中的依赖安全和环境输入阻塞。

## 阶段汇总

| 阶段 | 已实现内容 |
|---|---|
| M0 基线 | 仓库结构、架构边界、OpenAPI/JSON Schema 契约、生成客户端、CI 与 ADR 机制 |
| M1 文档核心 | Wiki/Namespace、Page/Revision/ContentSnapshot、发布事务、历史、改名、别名、重定向、补偿回滚 |
| M2 Web 编辑 | Next.js 页面生命周期、BlockNote 编辑器、AST 往返、创建/编辑/发布/历史 Diff/回滚 |
| M3 投影与 Worker | PostgreSQL Outbox、租约/重试/死信、链接/大纲/锚点/外链/渲染投影、按页及全量重建 |
| M4 知识与证据 | Entity/Property/Claim、标签与别名、Source/Version/Chunk/Citation、知识引用及反向使用投影 |
| M5 治理 | Proposal/Operation、风险分级、ReviewTask、三方冲突、ChangeBatch、审计、权限与补偿回滚 |
| M6 AI 与导入 | HTML/PDF 安全获取、解析与抽取流水线、OpenAI strict JSON Schema 与 DeepSeek JSON Object Adapter、Prompt 版本、用量记录、幂等任务、证据约束 Proposal |
| M7 平台化 | PostgreSQL FTS Adapter、本地账号与服务端 Session、应用层限流与安全头、OTel、备份恢复、数据一致性 Doctor、部署流水线 |
| M8 协作编辑 | Yjs AST 映射、WorkingDocument、WebSocket 增量同步、Presence、原子发布换基、AI 三方合并、人工冲突决议 |
| M9 P1 扩展 | 稳定章节锚点、BlockRedirect、Component/Version/信息框、Collection、外链健康检查、Entity 合并、批量风险审核、可靠性与容量 smoke |

## 当前核心能力

### 内容与协作

- Typed Block AST v1 是正文权威格式，Block 使用稳定 UUIDv7。
- Revision 与 ContentSnapshot 发布后不可变，发布和 WorkingDocument 换基在单事务内完成。
- Yjs update 只追加保存，支持快照压缩、断线恢复、慢客户端保护和 sequence CAS。
- 人工与 AI 的陈旧基线修改不能静默覆盖 Current Revision。

### 知识、证据与治理

- Entity/Claim 与 Source/Citation 形成可追踪证据链。
- AI Actor 只能生成 Proposal，不能直接发布正文或修改正式 Claim。
- Proposal 支持风险审核、冲突检测、ChangeBatch 审计及补偿回滚。
- Entity 合并保留旧 ID 映射，并由 Worker 幂等生成引用修复 Proposal。
- BulkReview 支持风险强制全量、确定性抽样、部分拒绝、暂停恢复和分 wave Apply。

### 投影与自动化

- 链接、目录、锚点、渲染、搜索、知识使用、外链使用和组件依赖均为可重建投影。
- Worker 使用租约、退避、重试和死信机制消费 Outbox。
- 外链检查包含 URL、Redirect 和实际 Dial 三层 SSRF 防护，并使用 lease token fencing。
- Claim 变化通过 `claim_usage` 与 `component_dependency` 精确定位需重渲染页面。

### Web、API 与认证

- OpenAPI 3.1 是 HTTP 契约源，Web 只通过生成的 TypeScript 客户端访问 API。
- Web 提供页面、编辑、历史、搜索、知识、治理、Collection 和协作入口。
- 正式本地账号提供 `POST /api/v1/auth/register` 和 `POST /api/v1/auth/login`：
  用户名、邮箱全站唯一且不区分大小写，密码只保存 Argon2id 哈希。
- 每个账号绑定独立 `human` Actor；首个注册账号在串行事务中获得管理员角色，
  后续账号默认获得编辑者角色。`AUTH_REGISTRATION_ENABLED=false` 可关闭公开注册。
- 登录支持用户名或邮箱，成功后签发 HttpOnly 服务端 Session Cookie；数据库只保存
  随机会话令牌的 SHA-256，并在每次请求重新检查 Actor 状态。
- 邮箱验证、忘记密码、MFA 与登录设备管理尚未实现，继续列为商业发布阻塞项。

### 运维与质量

- 数据库在预发布阶段使用唯一 `000001_initial_schema` 初始化版本。
- 开发环境由 `scripts/dev.sh` 直接启动 API、Worker 与 Web；生产 Compose
  自带 PostgreSQL、Redis、MinIO 和应用进程，不包含 Nginx、Meilisearch 或 OIDC。
- 生产部署的普通配置与机密统一从仓库外 `DEPLOY_ENV_FILE` 注入；部署脚本校验
  必填机密非空以及环境文件不允许 group/world 读取（ADR-0017）。
- Anby Wiki 自有业务镜像不发布到 registry；生产部署机从受保护源码本地构建
  `anby-wiki-<target>:<RELEASE_ID>` 并直接运行，回滚复用保留的旧本地镜像（ADR-0018）。
- CI 覆盖 Go/Web lint、类型检查、构建、契约、生成物漂移、迁移、部署配置和安全扫描。
- 提供 OTel、Prometheus 指标、备份恢复、数据一致性 Doctor、Projection/Search 重建和部署 runbook。
- 为降低小项目维护成本，仓库当前不维护自动化单元、集成或浏览器测试套件；
  关键变更依赖静态检查、构建、契约漂移检查与人工联调。

## 数据库状态

- 唯一迁移版本：`000001_initial_schema`。
- 初始化包含当前全部表、函数、触发器、约束、索引和固定种子。
- 已验证全新空库 `up`、完整 `down`、再次 `up`。
- 项目首次生产上线后必须冻结版本 1，并恢复只增不改的增量迁移策略。
