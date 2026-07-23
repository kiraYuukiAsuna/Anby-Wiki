# 现代 Wiki 实施方案（AI Agent Coding）

> 依据：[WikiDesignOnePage.md](./WikiDesignOnePage.md)  
> 文档状态：实施基线 v1  
> 目标：将整体设计拆成可由 AI Coding Agent 独立执行、验证和合并的里程碑与 Task。

## 1. 交付目标与实施策略

首个可发布版本不是一次性实现设计稿中的所有对象，而是逐步完成四个可运行闭环：

1. **人工内容闭环**：创建页面 → 编辑 AST → 发布 Revision → 阅读 → 查看历史 → 回滚。
2. **数据派生闭环**：发布 Revision → Outbox → Projection → Render/Search → 失败重试与重建。
3. **知识治理闭环**：Entity/Claim/Source/Citation → Proposal → Review → Apply → Audit/Rollback。
4. **AI 导入闭环**：导入来源 → 解析与抽取 → 生成 Proposal → 人工/策略审核 → 正式发布。

P0 完成后交付一个可内部使用的单站点 Wiki Beta；P1 增加多人协作和自动化增强；P2 再处理规模化与高级知识能力。

### 1.1 实施原则

- 采用**模块化单体 + 独立 Worker**，先稳定领域边界和事务，再根据负载拆服务。
- 每个里程碑都必须产生可演示的纵向能力，不按数据库表横向铺设空壳。
- 权威写入只能经过领域服务；API、Worker、Agent 均不得直接拼 SQL 修改权威状态。
- Revision、ContentSnapshot 发布后不可变；Projection 必须可丢弃、可重建。
- 业务 AI 在 Proposal/Review 能力完成前只用于辅助开发，不接入正式数据写入路径。
- 所有异步任务必须幂等，并携带来源版本；旧任务不得覆盖新 Revision 的投影。
- 所有 Task 都同时交付实现、测试、必要迁移、契约和最小运维说明。

## 2. 固定技术基线

当前仓库没有代码。以下技术栈作为项目固定基线，M0 的 ADR 只补充具体工具和工程约定，不再重新选择前后端框架或数据库。

| 层次 | 固定基线 | 说明 |
|---|---|---|
| 代码仓库 | Go + TypeScript 单仓库 | 前后端通过 OpenAPI/JSON Schema 共享契约，不跨语言共享运行时代码 |
| Web | React + Next.js + TypeScript | 阅读页、编辑器、审核台、管理台 |
| 样式与组件 | Tailwind CSS 4 + shadcn/ui + Radix UI | shadcn/ui 作为项目组件基线，Radix 提供无障碍交互原语 |
| 图标与字体 | Lucide + Geist | 不并行引入第二套通用图标或字体体系 |
| UI 增强 | Sonner + cmdk | Toast 统一用 Sonner；命令面板和可搜索选择器统一用 cmdk |
| 状态与数据 | Zustand + SWR | Zustand 管本地交互状态，SWR 管远端数据，禁止双份缓存同一服务端实体 |
| 前端校验 | Zod | 表单、编辑器输入和客户端边界运行时校验 |
| 网关 | Nginx | TLS 终止、反向代理、上传限制、压缩和基础安全头 |
| API/Worker | Golang 模块化单体 + 独立 Worker | API 负责同步领域操作，Worker 负责 Outbox、Projection、导入、渲染和搜索同步 |
| 数据库 | PostgreSQL | 唯一关系数据库；不建设 MySQL 兼容层 |
| 数据访问 | Go Repository + 显式 SQL Migration | 复杂事务、约束和索引必须可审查，不为多数据库增加抽象 |
| 缓存与任务 | Redis | 缓存、限流、短期协调和异步任务队列 |
| 对象存储 | S3 兼容存储 | 原始来源、PDF、图片和抓取产物 |
| 搜索 | Search Adapter；P0 后段接独立搜索引擎 | 禁止业务代码直接依赖具体搜索产品 |
| 编辑器 | Block Editor Adapter；M0 选型验证 | 编辑器内部模型必须可无损转换为 Typed Block AST |
| 多人协作 | CRDT Adapter；P1 接入 | CRDT 状态不作为正式版本历史 |
| 接口规范 | OpenAPI 3.1 | HTTP API 的唯一权威契约；错误模型、分页和幂等头统一定义 |
| 客户端生成 | OpenAPI Generator 或 Kiota 二选一 | M0 选定一种并固定版本，只维护一条生成链路，生成物禁止手改 |
| 测试 | Go/前端单元 + PostgreSQL 集成 + OpenAPI Contract + E2E | 不使用纯 Mock 代替关键事务测试 |
| 可观测性 | 结构化日志、Trace、Metrics | `request_id/job_id/page_id/revision_id/change_batch_id` 可串联 |

建议目录边界：

```text
apps/
  web/                       # Next.js、Tailwind、shadcn/ui、页面与前端状态
backend/
  cmd/api/                   # Golang HTTP API 入口
  cmd/worker/                # Golang 异步 Worker 入口
  internal/                  # Page、Knowledge、Evidence、Governance 领域模块
  migrations/                # PostgreSQL 显式迁移
  testkit/                   # Fixture、Factory、集成测试工具
contracts/
  openapi/                   # OpenAPI 3.1 权威接口定义
  schemas/                   # AST、事件、ProposalOperation JSON Schema
  generated/typescript/      # OpenAPI Generator/Kiota 生成的前端客户端
infra/
  nginx/                     # 网关配置
  local/                     # PostgreSQL、Redis、对象存储和搜索本地依赖
  deploy/                    # 部署与监控模板
Docs/adr/               # 重要技术决策
Docs/tasks/             # 可直接交给 Coding Agent 的 Task Packet
```

前端实现约束：SWR 是服务端数据的唯一客户端缓存入口；Zustand 只保存编辑器会话、面板开关和未提交交互状态。所有 API 调用必须经过生成客户端，不在页面或 Store 中手写 URL/DTO。shadcn/ui 组件代码可以按项目需要调整，但交互行为优先复用 Radix；Toast、图标、字体和搜索选择器分别固定为 Sonner、Lucide、Geist 和 cmdk。

## 3. 路线图总览

| 里程碑 | 结果 | 范围 | 完成标志 |
|---|---|---|---|
| M0 工程地基 | 可持续开发的空系统 | 工程、契约、CI、本地环境、ADR | 新环境一条命令启动，CI 全绿 |
| M1 文档内核 | Page/Revision/AST 发布闭环 | 站点、页面、版本、渲染、回滚 | 页面可创建、发布、改名、读取、回滚 |
| M2 人工编辑体验 | 普通用户可维护内容 | 编辑器、链接、历史、冲突提示 | 浏览器中完成页面全生命周期 |
| M3 事件与投影 | 可重建的派生数据链路 | Outbox、Projection、RenderedPage、链接关系 | 投影可重试、重放、按页重建 |
| M4 知识与证据 | 事实和来源可追溯 | Entity、Claim、Source、Citation、Usage | 页面事实可追到来源 Chunk |
| M5 Proposal 治理 | AI 安全写入边界 | Operation、Review、Risk、Conflict、Batch、Audit | 所有机器变更均经 Proposal 生效 |
| M6 AI 导入 | 来源到百科更新闭环 | 抓取、解析、抽取、消歧、提案、评测 | 真实样例可生成并审核发布 Proposal |
| M7 P0 Beta | 可部署、可运营的内部版本 | 搜索、安全、性能、备份、观测、发布 | P0 验收和恢复演练通过 |
| M8 P1 实时协作 | 多人及 AI 并发可控 | WorkingDocument、CRDT、三方合并 | 多端协作、离线恢复、冲突处理通过 |
| M9 P1 自动化增强 | 内容维护效率提升 | Component、Collection、链接检查、实体合并 | P1 功能形成稳定后台作业 |

主依赖关系：

```text
M0 → M1 ─┬→ M2 ───────────────┐
         ├→ M3 ───────────────┤
         └→ M4 → M5 → M6 ─────┤
                               ▼
                              M7 ─┬→ M8
                                  └→ M9
```

M2、M3、M4 可在 M1 后由不同 Agent 并行推进，但同一时间只允许一个 Task 修改同一组数据库迁移或共享 AST Schema。

## 4. Task 划分规则

每个 Task 是一次可独立审查的变更，目标为一个 Agent 在一个聚焦上下文中完成。不要将“完成某个里程碑”直接交给单个 Agent。

依赖列中省略里程碑前缀的 `Txx` 均指当前里程碑；生成正式 Task Packet 时必须展开为完整 Task ID。路线图以 Agent Work Unit 而非日历估期，一个 Task 默认不超过一次聚焦实现与一次修订；超出时应继续拆分。

### 4.1 合理 Task 的边界

- 一个主要业务结果，例如“发布 Revision 的原子事务”，而不是“实现整个 Page 模块”。
- 尽量只跨一个领域；纵向功能允许同时修改 Schema、Service、API 和测试。
- 有明确输入契约和输出契约；不能依赖 Agent 自行猜测产品规则。
- 有自动化验收命令，并至少包含一个失败路径或并发路径测试。
- 数据库迁移、公共 Schema、事件格式等高冲突文件由串行 Task 管理。
- 大 Task 先拆“契约/迁移”“领域实现”“UI/E2E”，而不是让多个 Agent 同时改同一功能。

### 4.2 每个 Task Packet 必填内容

```text
Task ID / 标题
业务目标与非目标
对应设计稿章节、相关 ADR
前置依赖与允许修改的模块
输入/输出契约、数据不变量
实现要求与禁止事项
验收场景（Given / When / Then）
必须执行的测试命令
迁移、回滚、观测和文档要求
交付文件清单
```

### 4.3 通用 Definition of Done

- 类型检查、Lint、单元测试、相关集成测试通过。
- 新增 API/事件/Operation 均有版本化 Schema 和契约测试。
- 新迁移在空库升级、已有库升级、回滚策略三种场景下有说明。
- 权威写入路径有事务测试；异步路径有幂等和重试测试。
- 用户输入与渲染输出经过校验、授权与安全处理。
- 日志不记录来源全文、Prompt 密钥、Token 或个人敏感信息。
- 文档、Fixture 和验收命令随代码提交；不留下无负责人 TODO。

## 5. P0 里程碑与 Task

### M0：工程地基与架构定稿

**目标**：建立所有后续 Agent 共用的规则、目录、工具链与可运行环境。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M0-T01 | 范围与需求追踪矩阵 | 将设计稿 P0 对象、14 条不变量映射到里程碑和测试 ID；明确 P0 非目标 | 无 |
| M0-T02 | 技术 ADR | 固化既定技术栈；只确认 Go 工程结构、SQL/迁移工具、队列、对象存储、编辑器、搜索、客户端生成器与 ID 策略 | T01 |
| M0-T03 | 仓库脚手架 | 创建 Next.js/TypeScript Web、Go API/Worker、Contracts 和 Infra；接入 Tailwind CSS 4、shadcn/ui、Radix、Lucide、Geist、Sonner、cmdk、Zustand、SWR、Zod | T02 |
| M0-T04 | 本地基础设施 | Nginx、PostgreSQL、Redis、对象存储的可复现本地环境；反向代理、健康检查和初始化可重复执行 | T02 |
| M0-T05 | CI 与质量门禁 | Go 格式/静态检查/测试/构建、前端类型/Lint/测试/构建、OpenAPI 校验、生成物漂移和迁移校验 | T03、T04 |
| M0-T06 | OpenAPI 与服务基础契约 | OpenAPI 3.1、标准错误/分页/幂等模型、请求 ID、配置校验、健康接口；选定 OpenAPI Generator 或 Kiota 并验证 Next.js 调用 Go API | T03 |
| M0-T07 | Agent 开发约定 | 增加仓库级 Agent 指令、Task 模板、提交/分支/迁移所有权和交接清单 | T01-T06 |

**里程碑验收**：从全新环境执行约定的 bootstrap 命令后，Web/API/Worker 启动，依赖就绪，CI 完整通过；任何 Agent 可只读仓库文档定位模块边界与测试命令。

### M1：文档内核与不可变版本

**目标**：首先完成不依赖 LLM 的最小 Wiki 纵向闭环。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M1-T01 | Typed Block AST v1 | 语言无关 JSON Schema 以及 Go/Zod 实现；覆盖 Document、Heading、Paragraph、List、Table、Code、Quote、Callout、Divider 和基础 Inline，规定稳定 ID 与 Schema version | M0 |
| M1-T02 | AST 工具集 | Go 实现遍历、按 ID 定位、内容哈希、稳定序列化、纯函数 Patch 和结构 Diff；共享 Fixture 验证 Go/Zod 往返一致性 | T01 |
| M1-T03 | 文档核心迁移 | `wiki_site/namespace/actor/page/page_alias/page_redirect/revision/content_snapshot/audit_event/outbox_event`；唯一索引、外键及不可变约束 | M0、T01 |
| M1-T04 | Page 领域服务 | 标题规范化、创建、查询、改名、别名解析、重定向和循环检测；Page ID 在改名后保持不变 | T03 |
| M1-T05 | Revision 发布事务 | 校验 AST 和 `expected_revision_id`，原子写入 Snapshot/Revision/current pointer/Audit/Outbox；并发发布仅一个成功 | T02-T04 |
| M1-T06 | 基础 Renderer 与阅读 API | AST 输出安全 HTML；按标题/别名/ID 读取当前 Revision；重定向和不存在页面行为确定 | T01、T04、T05 |
| M1-T07 | 历史、Diff 与回滚 | Revision 列表、两版结构 Diff；回滚通过创建新 Revision 完成，旧 Snapshot 不变 | T02、T05 |
| M1-T08 | 文档内核集成测试 | 创建→两次发布→改名→旧标题访问→回滚；覆盖非法 AST、陈旧基线、重定向环和无效 Actor | T04-T07 |

**里程碑验收**：API 测试中完成页面全生命周期；直接更新已发布 Revision/Snapshot 会失败；回滚后产生新的 Revision；发布事务失败时不留下半成品或孤立 current pointer。

### M2：人工编辑与阅读产品闭环

**目标**：让非开发者通过浏览器维护页面，同时保持 AST 为唯一正文格式。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M2-T01 | UI 基线、阅读页壳与路由 | 用 Tailwind CSS 4、shadcn/ui、Radix、Lucide 和 Geist 建立响应式应用壳；实现标题路由、目录、内部链接、重定向、404 和 Revision 信息 | M1 |
| M2-T02 | Block Editor Adapter | 编辑器模型与 AST v1 双向转换；新 Block 生成稳定 ID，编辑既有 Block 不改 ID | M1-T01、T02 |
| M2-T03 | 编辑与发布页面 | SWR 读取服务端版本、Zustand 保存编辑会话、Zod 校验、Sonner 反馈；支持基础 Block、修改摘要、发布错误恢复，本地草稿不成为权威版本 | T02、M1-T05 |
| M2-T04 | 引用节点编辑 | 用 cmdk 完成页面搜索选择；创建已解析/未解析 PageReference、ExternalLink 及显示文本，目标 ID 与显示文字分离 | T02、M1-T04 |
| M2-T05 | 历史与回滚界面 | 展示结构 Diff、Revision 元信息和回滚确认；回滚后跳转到新 Revision | M1-T07 |
| M2-T06 | 乐观锁冲突体验 | 两个浏览器基于同一 Revision 发布时，后提交者保留本地内容并看到 Base/Current 提示 | T03、M1-T05 |
| M2-T07 | 浏览器 E2E | 覆盖创建、编辑、链接、发布、改名、冲突、历史和回滚的主路径 | T01-T06 |

**里程碑验收**：不调用内部 API 或手写 JSON，即可在浏览器创建并维护一篇含标题、段落、列表、表格和内部链接的页面；双窗口并发不会静默覆盖。

### M3：Outbox、Projection 与渲染链路

**目标**：将阅读和关系查询从权威写入链路解耦，并证明所有投影可安全重建。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M3-T01 | Outbox 消费框架 | 领取、续租、完成、失败、指数退避、死信和幂等键；Worker 崩溃重启不丢事件 | M1-T03、T05 |
| M3-T02 | Projection 通用框架 | `projection_state`、Builder 接口、版本防护、按 Page 重建、全量重放；旧 Revision 任务不能覆盖新结果 | T01 |
| M3-T03 | 页面链接与目录投影 | `page_link_projection/document_outline_projection/page_anchor`；解析 AST，支持已解析与未解析引用 | T02、M1-T01 |
| M3-T04 | 未解析链接 Resolver | 新建/改名页面后解析候选；无歧义时更新投影，有歧义时保持 unresolved，不直接改 AST | T03、M1-T04 |
| M3-T05 | RenderedPage 投影 | Worker 生成带 renderer version/hash 的安全 HTML；阅读路径优先投影，缺失时受控降级 | T02、M1-T06 |
| M3-T06 | 外链使用投影 | `external_resource/external_link_usage` 和 URL 规范化；URL 状态更新不创建页面 Revision | T02、M2-T04 |
| M3-T07 | 运维与一致性工具 | 投影积压/延迟/失败指标，按页重建命令，一致性抽检；故障注入测试覆盖乱序和重复消息 | T01-T06 |

**里程碑验收**：删除某页全部投影后可从当前 Revision 完整重建；重复、延迟、乱序事件不会产生重复关系或旧渲染；Worker 故障不回滚已经发布的 Revision。

### M4：Entity、Claim、Source 与 Citation

**目标**：完成文档、知识和证据三平面的连接，使事实可独立验证和引用。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M4-T01 | Knowledge Schema | `entity_type/entity/entity_label/entity_alias/property/claim/claim_source/page_entity_binding` 迁移、索引与状态枚举 | M1 |
| M4-T02 | Entity 服务 | 创建、标签/别名、规范化搜索、Page 绑定；同名候选不自动合并 | T01 |
| M4-T03 | Property/Claim 服务 | 值类型 Schema、多值/目标类型校验、业务状态与验证状态分离、Supersede 链 | T01、T02 |
| M4-T04 | Evidence Schema 与存储 | `asset/asset_revision/source/source_version/source_chunk/citation`，复用 M3 的 ExternalResource；对象存储哈希去重和元数据 | M1、M3-T06 |
| M4-T05 | Source/Citation 服务 | 创建来源版本、Chunk 定位、Quotation hash、Citation；Citation 必须指向有效版本和位置 | T04 |
| M4-T06 | AST 知识引用 | EntityReference、ClaimReference、CitationReference Schema、编辑命令和 Renderer；引用稳定 ID | M1-T01、T02、T03、T05 |
| M4-T07 | 知识/证据投影 | `entity_mention_projection/citation_usage/claim_usage`；提供反向查询 API 并携带 Revision | M3-T02、T06 |
| M4-T08 | 知识与来源界面 | Entity/Claim 详情、验证状态、来源定位、页面使用位置；没有权限时只读 | T02-T07 |
| M4-T09 | 端到端证据测试 | 页面 ClaimReference → Claim → Citation → SourceChunk 全链路；Supersede 后旧事实仍可审计 | T01-T08 |

**里程碑验收**：页面中的结构化事实可定位到具体 SourceVersion/Chunk；修改 Claim 不会篡改旧 Citation；反向查询不扫描 AST JSON；同名 Entity 不会被无依据合并。

### M5：Proposal、Review 与变更治理

**目标**：建立所有 AI/批量修改共用的安全写入协议，再允许接入模型。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M5-T01 | Proposal Schema 与状态机 | `proposal/proposal_operation/review_task/merge_conflict/change_batch/import_job`；合法状态转换、序号和幂等键 | M1、M4 |
| M5-T02 | Operation Contract v1 | 在 JSON Schema/OpenAPI 3.1 中定义首批页面、引用、Entity、Claim、Citation 操作的版本化 Discriminated Union；生成 Go/TypeScript 类型，每种操作含 Base、Target、Expected Hash、Evidence | T01 |
| M5-T03 | 页面 Patch Engine | Insert/Delete/Move/Replace Block 和引用操作；纯函数应用、Target 不存在/已修改时拒绝 | T02、M1-T02 |
| M5-T04 | Knowledge Patch Engine | CreateEntity/CreateClaim/SupersedeClaim/AddClaimSource；复用 M4 校验，不绕过 Claim 状态机 | T02、M4-T03、T05 |
| M5-T05 | Diff 与预览 API/UI | 在不写权威数据的情况下展示 Base/Current/Proposed、来源和影响范围 | T03、T04、M2 |
| M5-T06 | 风险策略与审核队列 | 风险规则、人工审核、拒绝原因；高风险操作不可自动通过，策略决策可解释和审计 | T01、T02 |
| M5-T07 | 冲突检测与解决 | Revision/Block hash/Claim state 三层检测；生成 MergeConflict，禁止语义冲突自动覆盖 | T03、T04、T05 |
| M5-T08 | Proposal 原子应用 | 经批准后原子创建 Revision/Claim/ChangeBatch/Audit/Outbox；重复 Apply 返回同一结果 | T03-T07 |
| M5-T09 | 批量回滚 | 按 ChangeBatch 生成补偿变更；页面回滚创建新 Revision，Claim 使用 Supersede/状态转换 | T08 |
| M5-T10 | 权限与页面保护 | Role/ActorRole/PageProtection；创建、编辑、审核、批量应用分权，AI Actor 无直接发布权限 | T06、T08 |
| M5-T11 | 治理安全测试 | 伪造批准、重复应用、陈旧 Proposal、越权、部分事务失败、批量回滚等对抗场景 | T01-T10 |

**里程碑验收**：禁用 Proposal Apply 服务后，AI Actor 没有任何可修改 Revision、Claim 或 Projection 的路径；陈旧或高风险 Proposal 必须进入冲突/审核；批量操作能完整追踪并以新版本方式回滚。

### M6：AI 来源导入与持续更新闭环

**目标**：让 LLM 只负责理解和提出结构化变更，领域服务负责全部验证与正式写入。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M6-T01 | AI Gateway 与 Prompt Registry | 模型供应商适配、结构化输出、超时/重试/限流、模型与 Prompt 版本、用量记录；业务层不依赖供应商 DTO | M0、M5 |
| M6-T02 | 导入任务编排 | ImportJob 状态机、Job/Run 幂等键、取消、重试、阶段性错误；重复提交同一版本不重复抽取 | M3-T01、M5-T01 |
| M6-T03 | 来源获取安全层 | 首版支持用户上传和受控 URL；SSRF 防护、类型/大小限制、哈希、重定向策略、恶意文件隔离 | M4-T04、T05 |
| M6-T04 | 文本解析与 Chunking | HTML/PDF 文本抽取、页码/章节/字符范围定位、稳定 Chunk hash；解析失败保留原始来源和诊断 | T02、T03 |
| M6-T05 | 抽取 Schema 与评测集 | Entity/Claim/Citation 候选 Schema，包含证据定位和置信信息；建立人工标注的网页/PDF Golden Set | T01、T04 |
| M6-T06 | 实体匹配与消歧 | 使用类型、别名、页面绑定和上下文召回候选；歧义输出 Review，不自行创建重复 Entity | T05、M4-T02 |
| M6-T07 | Claim 去重与冲突识别 | 比较规范化值、有效时间、来源和验证状态；区分新增、支持、矛盾、替代候选 | T05、M4-T03 |
| M6-T08 | Proposal Composer | 将候选转换为 Operation v1，附 Base、Expected Hash、Citation 和风险因子；输出再次经过领域 Schema 校验 | T05-T07、M5-T02 |
| M6-T09 | 导入进度与审核体验 | 展示获取、解析、抽取、匹配、提案阶段；按来源/页面/风险分组审核，可定位原文 Chunk | T02、T08、M5-T05、T06 |
| M6-T10 | AI 安全与质量门禁 | Prompt Injection 样例、无证据断言、伪造 Citation、超长输入、模型超时、重复运行；质量低于阈值不自动通过 | T01-T09 |

**里程碑验收**：给定 Golden Set 中的来源，系统能生成有证据定位的 Proposal，经人工批准后形成 Revision/Claim/Projection；重复导入同一 SourceVersion 不重复发布；模型输出不能绕过 Schema、权限、冲突和风险策略。

### M7：P0 Beta 硬化与发布

**目标**：把功能闭环提升为可部署、可恢复、可观测的内部 Beta。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M7-T01 | 搜索投影与 Adapter | 标题、别名、正文、Entity、字段过滤和高亮；索引由 Outbox 更新，可全量重建 | M3、M4 |
| M7-T02 | 身份认证与权限收口 | 经 Nginx/Go API 接入实际身份系统，完成 Actor 映射、会话安全和权限缓存失效；匿名/人工/AI/System 边界清晰 | M5-T10 |
| M7-T03 | Web 与网关安全硬化 | Next.js XSS/CSP/CSRF、Nginx 安全头/上传限制、文件隔离、URL 获取策略、速率限制、Secret 管理和依赖扫描 | M2、M6-T03 |
| M7-T04 | 可观测性与告警 | API 延迟/错误、发布失败、队列积压、投影延迟、导入耗时/失败、模型用量；Trace 串联全链路 | M3、M6 |
| M7-T05 | 性能与容量基线 | 构造十万级页面/Revision/投影数据，压测阅读、发布、反链、审核和导入；建立容量预算和慢查询基线 | T01、T04 |
| M7-T06 | 备份、恢复与重建演练 | PostgreSQL 和对象存储恢复；Search/Projection 从权威数据重建；记录 RPO/RTO 实测值 | M3、M4、T01 |
| M7-T07 | 数据一致性巡检 | 检查 current pointer、不可变内容、悬空引用、投影版本、Claim/Citation 链、Outbox 卡死；支持安全修复或生成报告 | M1-M6 |
| M7-T08 | 部署与迁移流水线 | Nginx、Next.js、Go API/Worker 的环境配置、Migration gate、滚动发布、兼容窗口、回滚流程和 Seed | M0、T04-T07 |
| M7-T09 | P0 需求与不变量验收 | 对照 M0 追踪矩阵执行功能、安全、故障和恢复测试；所有例外有明确 ADR | T01-T08 |
| M7-T10 | Beta 发布与观察期 | 小范围数据导入、运行手册、值班/故障分级、反馈入口；观察期内不并行引入 P1 架构改动 | T09 |

**里程碑验收**：P0 追踪矩阵全部通过；在测试环境完成数据库恢复和投影全量重建；十万级数据下达到 ADR 中定义的 SLO；上线后能从日志/指标定位任一发布或导入链路。

## 6. P1 里程碑与 Task

### M8：WorkingDocument、CRDT 与语义冲突

**进入条件**：P0 Beta 稳定，AST v1 和发布 API 不再频繁变化。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M8-T01 | CRDT 选型与映射 ADR | 验证字符、Block 增删/移动、属性修改、稳定 ID、离线合并及 AST 无损转换 | M7 |
| M8-T02 | WorkingDocument 存储 | `working_document/working_document_update`、递增 sequence、快照、压缩和恢复 | T01 |
| M8-T03 | 实时同步服务 | 鉴权 WebSocket、房间生命周期、更新广播、Presence/光标；断线重连不丢更新 | T02 |
| M8-T04 | 协作编辑器接入 | 多端编辑、在线状态、自动保存和发布；CRDT 内容发布前必须通过 AST Schema | T03、M2-T02 |
| M8-T05 | 发布与 WorkingDocument 换基 | 发布检查 base revision；成功后换基，失败时保留工作副本并提示新 Current | T04、M1-T05 |
| M8-T06 | AI 三方合并 | Base/Current/Proposed 块级合并；无冲突合并到工作副本或新 Proposal，有冲突显式记录 | T05、M5-T07 |
| M8-T07 | MergeConflict 解决界面 | 展示三方值、逐冲突决议、重新校验后 Apply；解决记录进入 Audit | T06 |
| M8-T08 | 协作可靠性测试 | 多客户端、离线恢复、乱序/重复 update、服务重启、大文档、删除后编辑和权限撤销 | T02-T07 |

**里程碑验收**：三个客户端可同时编辑并在断网重连后收敛；正式发布仍是一条单父 Revision 主线；CRDT 自动合并不被当作语义冲突的自动裁决。

### M9：内容自动化增强

M9 的子域相对独立，可在冻结各自 Schema 后并行开发。

| ID | Task | 交付与验收 | 依赖 |
|---|---|---|---|
| M9-T01 | 稳定锚点与迁移 | PageAnchorAlias、BlockRedirect、章节改名/迁移后的旧链接解析 | M3、M8 |
| M9-T02 | Component 基础 | Component/Version Schema、Renderer Registry、Props 校验、版本冻结 | M4、M7 |
| M9-T03 | 信息框与依赖 | 从 Entity/Claim 渲染信息框，ComponentDependency/ClaimUsage 触发精准重渲染 | T02、M4-T07 |
| M9-T04 | Manual/Rule Collection | Collection/Membership、规则校验、来源 Revision、列表查询和页面展示 | M4、M3 |
| M9-T05 | 外链健康检查 | 调度、退避、Canonical/Redirect/失效状态；只更新 ExternalResource，改目标时生成 Proposal | M3-T06、M5 |
| M9-T06 | Entity 合并 | merged 映射、引用可解析、标签/Claim 迁移规则、引用修复 Proposal 和回滚策略 | M4、M5 |
| M9-T07 | 批量风险审核 | 按 ChangeBatch 聚合、抽样/全量规则、部分拒绝策略、灰度 Apply 和暂停 | M5、M6 |
| M9-T08 | P1 回归与容量测试 | Component/Collection/Link/Entity 后台任务的幂等、重建、积压和性能测试 | T01-T07 |

**里程碑验收**：章节和 Entity 变更保留旧 ID 的解析路径；Claim 或组件升级只重渲染受影响页面；批量 AI 任务可暂停、审核、灰度应用和审计。

## 7. P2 规划方式

P2 不应现在拆成代码级 Task。它依赖 P0/P1 的真实负载、数据质量和用户行为，现阶段只保留 Epic 与启动条件：

| Epic | 启动条件 | 首要验证 |
|---|---|---|
| Dynamic Collection / Dataset | Rule Collection 无法满足已确认用例 | 查询模型、更新一致性、权限和大数据量分页 |
| 高级 Claim Qualifier | 真实事实模型需要复杂限定词 | Schema 演进、索引、冲突与展示 |
| AI 信任等级/事实一致性 | 有足够审核结果作为评测数据 | 校准率、误合并成本、策略可解释性 |
| 语义搜索和图查询 | 关键词/字段搜索的召回不足有数据证明 | Embedding 版本、重建成本、权限过滤 |
| 跨 Wiki Federation | 存在两个真实 Wiki 的共享实体需求 | ID/命名空间、同步冲突和来源归属 |
| 章节懒加载/冷热 Revision | 容量基线显示数据库或阅读瓶颈 | 一致性、归档恢复和历史查询成本 |

每个 P2 Epic 先创建 Discovery Task，输出 ADR、原型、容量实验和可量化成功指标，再拆实施 Task。

## 8. 并行开发与合并编排

### 8.1 可并行工作流

M1 完成后，可形成三条主线：

- **文档产品线**：M2 阅读/编辑 UI。
- **基础设施线**：M3 Outbox/Projection/Render。
- **知识模型线**：M4 Entity/Claim/Source/Citation。

M5 的 Contract/State Machine 可在 M4 后半段开始，但 Apply 不得在 M1、M3、M4 的事务和投影接口稳定前合并。M6 必须等待 M5 安全边界完成。M7 是统一集成与发布门禁，不与大规模功能开发混合。

### 8.2 高冲突资源所有权

以下资源同一时间只分配给一个 Agent：

- 数据库主 Schema 与迁移序号。
- Typed Block AST 根 Schema 和 Schema version。
- ProposalOperation Union 与事件 Envelope。
- 发布事务、Proposal Apply 事务。
- 仓库根配置、CI、Go Module 和前端依赖锁文件。

其他 Agent 通过已合并契约开发 Adapter、UI、Builder 或测试 Fixture，不在自己的 Task 中顺手修改公共契约。

### 8.3 推荐合并顺序

```text
契约/ADR
  → Migration
    → Repository/Domain Service
      → API/Worker
        → UI
          → E2E/容量/故障测试
```

契约变更必须向后兼容一个部署窗口：先让消费者兼容新旧格式，再发布生产者，最后清理旧格式。

## 9. 测试与验收体系

### 9.1 测试层次

| 层次 | 重点 |
|---|---|
| Schema/Property Test | JSON Schema、Go、Zod 的 AST 往返一致，ID 唯一、Operation 可判别、值类型校验 |
| Domain Unit Test | 状态机、权限、风险、标题规范化、Patch、冲突规则 |
| PostgreSQL Integration | 发布原子性、不可变约束、唯一索引、并发锁、Outbox |
| Worker Integration | 重复、乱序、失败、重试、版本防护、重建 |
| Contract Test | OpenAPI 3.1、生成客户端、事件、AI 结构化输出和版本兼容 |
| Browser E2E | 阅读、编辑、审核、冲突、回滚、导入 |
| Golden/Evaluation | 来源抽取、实体匹配、Claim 冲突、Citation 定位 |
| Security Test | XSS、SSRF、越权、恶意文件、Prompt Injection |
| Load/Chaos Test | 十万级数据、队列积压、Worker 中断、恢复和重放 |

### 9.2 长期不变量对应的必测门禁

至少将以下测试作为发布阻断项：

1. `current_revision_id` 必须属于当前 Page。
2. 已发布 Revision/Snapshot 的 Update/Delete 被拒绝。
3. 删除 Projection 后能从权威数据得到等价结果。
4. 旧 Revision 的异步任务不能覆盖新投影。
5. AI Actor 不能直接发布或修改 Claim。
6. 页面/章节/Entity 改名后稳定 ID 不变且旧地址可解析。
7. Claim 可绑定 Citation、验证状态与有效时间。
8. 人工验证 Claim 被 AI 修改时一定进入高风险审核。
9. 陈旧 Base 的人工/AI 修改不能静默覆盖 Current。
10. ChangeBatch 可完整审计，并通过补偿版本完成回滚。

## 10. 首批执行顺序

建议先按依赖批次建立并启动下列 Task Packet：

```text
M0-T01 需求追踪矩阵
  → M0-T02 技术 ADR
    → [M0-T03 仓库脚手架 || M0-T04 本地基础设施]
      → [M0-T05 CI 与质量门禁 || M0-T06 OpenAPI 与服务基础契约]
        → M0-T07 Agent 开发约定
          → [M1-T01 Typed Block AST v1 || M1-T03 文档核心迁移]
            → [M1-T02 AST 工具集 || M1-T04 Page 领域服务]
              → M1-T05 Revision 发布事务
```

M1-T01 与 M1-T03 在 ID、AST 存储和 Schema version ADR 确认后可以并行；M1-T05 是首个关键一致性门禁，不宜和 Page Schema 修改并行。

## 11. P0 明确非目标

以下能力保留扩展点，但不得为了“以后可能需要”阻塞 P0：

- 微服务拆分、跨地域多活、多主写入。
- 跨 Wiki Entity Federation。
- 完整 Wikitext 兼容层和任意 HTML 导入保真。
- CRDT 正式版本历史；CRDT 仅属于 P1 WorkingDocument。
- 无人工监督的高风险 AI 自动发布。
- 通用图数据库、向量数据库和复杂语义检索。
- Dynamic Collection、Dataset、高级 Claim Qualifier。
- 百万级容量优化；P0 先以十万级数据验证架构和瓶颈。

## 12. 里程碑完成判定

里程碑只有在以下条件全部满足时才可关闭：

- 表格中的 Task 全部合并并通过各自验收，不以“代码已生成”代替完成。
- 里程碑 E2E 场景在干净环境可重复运行。
- 数据迁移、回滚/补偿、告警和运行手册已验证。
- 需求追踪矩阵与不变量测试已更新。
- 没有绕开领域服务的临时写入路径。
- 下一里程碑依赖的 Schema、API、事件和 Adapter 已冻结并版本化。

这套拆分使 Coding Agent 可以在受控上下文内持续交付，同时确保系统真正沿着“结构化文档 + 知识与证据 + Proposal 治理 + 可重建投影”的设计主线演进。
