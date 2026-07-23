# 当前实现状态

> 更新时间：2026-07-24  
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
| M6 AI 与导入 | HTML/PDF 安全获取、解析与抽取流水线、Prompt 版本、用量记录、幂等任务、证据约束 Proposal |
| M7 平台化 | PostgreSQL FTS/Meilisearch Adapter、OIDC 登录、服务端 Session、网关安全、OTel、备份恢复、数据一致性 Doctor、部署流水线 |
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
- 已实现通用 OIDC Authorization Code + PKCE 登录、服务端 Session Cookie、当前账户查询和退出。
- 首次 OIDC 登录自动创建 `human` Actor；当前不提供本地用户名密码注册或密码表。

### 运维与质量

- 数据库在预发布阶段使用唯一 `000001_initial_schema` 初始化版本。
- CI 覆盖 Go/Web lint、类型检查、单元与 PostgreSQL 集成测试、构建、契约、生成物漂移、迁移、部署配置和安全扫描。
- 提供 OTel、Prometheus 指标、备份恢复、数据一致性 Doctor、Projection/Search 重建和部署 runbook。
- PostgreSQL 集成测试必须 `-p 1` 串行，避免共享测试库的 Reset 相互干扰。

## 数据库状态

- 唯一迁移版本：`000001_initial_schema`。
- 初始化包含当前全部表、函数、触发器、约束、索引和固定种子。
- 已验证全新空库 `up`、完整 `down`、再次 `up`。
- 项目首次生产上线后必须冻结版本 1，并恢复只增不改的增量迁移策略。
