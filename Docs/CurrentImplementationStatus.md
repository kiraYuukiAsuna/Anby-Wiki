# 当前实现状态

> 更新时间：2026-10-21
> 产品与能力审计依据：[整体设计方案](WikiDesignOnePage.md)

## 总体结论

`WikiDesignOnePage.md` 定义的模块化单体 API、独立 Worker、权威数据、治理写入、
可重建投影、知识与证据、导入、搜索及平台化主链路已落到代码与 HTTP 契约。适合
用户操作或查看的主要后端能力已在 Web 中提供持久、可重新发现的入口；导入队列、
治理任务、审计活动和后台工具不再只存在于一次性流程页面。协作领域已经具备在线
Yjs update、持久恢复、普通发布与 AI 合并的 sequence CAS、同标签页断线 update
重发、每 100 个已确认 sequence 自动 snapshot/compact 及 Block 级 Presence；
跨标签页离线恢复与多 API 实例实时广播尚未闭环，不能概括为全部协作设计能力均已完成。

当前代码已通过编译、契约、依赖安全和部署静态门禁，并在本机隔离环境完成
PostgreSQL、Redis、RustFS、Meilisearch、API、Linux Worker 与 Next.js Web 的真实
联调。提交 `18944a7` 已在远端生产拓扑完成六个 OCI target 构建、迁移 gate、Doctor、
滚动替换和健康检查，并通过复杂 PDF/扫描件、提示注入、Provider 故障恢复、上传幂等、
501 页边界、在途 Worker restart、18 页关联文档、12 页混合方向/尺寸文档、中英双语
跨页表格、恶意附件隔离、密码加密拒绝及 128/298 MiB 上传边界验收。另一个完全隔离的
远端数据库/bucket/index/Redis DB 已执行
149/149 OpenAPI operation handler 探针、核心成功工作流、治理、批量审核、导入、
AI 配置和双会话协作 E2E。“实现完成”仍不等于“生产发布就绪”：目标规模容量、正式域名
安全边界、账号恢复、备份恢复与人工可访问性等仍须验收。详见
[待解决问题](OutstandingIssues.md)。

## 2026-10-21 私人 Wiki 多用户隔离

- 统一 Wiki 已替换为账号级私人 Wiki。注册事务原子创建 `wiki_site`、七个
  Namespace、默认 ChangeTag、内置信息框、human Actor、admin 角色和首个 Session；
  Session 与 CLI Principal 都携带 `wiki_id`、Wiki 名称和主 Namespace ID。
- 除注册、登录、Session 查询、退出和 CLI 授权码兑换外，所有 `/api/v1` 路由都要求
  已认证 Principal 和显式 Wiki。Page、Entity、Claim、Source、Citation、Component、
  Dataset、Collection、Governance、Audit、AI 配置和 ImportJob 均按当前 Wiki 查询；
  外部 UUID 命中其他 Wiki 时统一按 404 处理。
- `source`、`component`、`change_tag` 和 `import_job` 已补齐 `wiki_id`；导入 Worker
  从 Job 解析 Wiki，并以该 Wiki 选择 AI 运行时配置。数据库用复合外键禁止把 human
  Actor 的角色授予其他 Wiki。
- Web 在无 Session Cookie 时先跳转登录页，失效会话只显示登录提示，不渲染 Wiki
  页眉、搜索、导航或正文。登录后页眉展示当前 Wiki 名称，人工 Proposal 模板使用
  Session 返回的真实 Wiki/主 Namespace ID。
- 本变更按预发布策略直接改写唯一初始化 Schema，不迁移旧统一 Wiki 数据。生产部署
  必须先备份再重建空库，并重新注册账号、签发 CLI Token、配置每个 Wiki 的 AI 设置。

## 2026-10-08 宿主机部署入口

- API/Worker 提供 10 分钟启动宽限和 12 分钟部署等待，覆盖首次启动的
  Meilisearch 语义模型下载；健康探针成功后立即继续。

- `scripts/deploy-production.sh` 提供干净工作区检查、ff-only 拉取、提交 SHA 镜像标签、
  顺序构建、迁移闸门、Doctor、应用更新与失败回滚。
- 宿主机 Nginx 模板覆盖 HTTPS、ACME 验证、HSTS、流式响应与协作 WebSocket；
  Certbot 证书可沿用服务器账户，续期由系统 timer 和 reload hook 管理。
- Web 与 API 分别只发布宿主机回环端口。Nginx 将 `/api/*` 和健康检查直送 Go API，
  并关闭 API 请求缓冲；其余页面请求送 Web。Next rewrite 只作为开发和兼容路径，
  避免生产大文件受其默认 10 MiB 请求缓冲截断。
- PostgreSQL、RustFS 与 Meilisearch 持久数据统一到根目录 `data/`；配置、首个管理员
  凭据及配置备份位于被 Git/Docker 排除的 `.env` / `Secret/`。首次管理员通过现有
  领域 API 初始化后关闭公开注册，未引入直接 SQL 权威写入。
- dotenv 按数据读取，不执行 Shell；本入口用于 `anbywiki.momiya.cloud` 独立实例，
  不自动迁移其他实例的数据。既有生产发布验收项继续见 `OutstandingIssues.md`。
- 生产对象存储使用固定 digest 的 RustFS 1.0.1；一次性 AWS CLI 工具以 path-style
  初始化私有 bucket，不依赖已不可拉取的 MinIO/mc 镜像。已有 MinIO 目录不能直接
  当作 RustFS 数据目录使用，必须通过 S3 导出/恢复。
- 上线检查补齐 Next.js 16.3.8、sharp 0.35.5、source-map-js 1.2.2、golang.org/x/text 0.41.0 与
  OpenTelemetry Go 1.45.0 的安全修复；首次部署失败重试不视为可回滚的已验收版本。

## 设计能力覆盖与已知边界

| 领域 | 当前实现 |
| --- | --- |
| 页面核心 | Wiki/Namespace、Page、不可变 Revision/ContentSnapshot、稳定 Block ID、历史、Diff、回滚、改名、PageRedirect、BlockRedirect、页面保护 |
| 知识图谱 | Entity/Property/Claim、标签/别名/主标签、Page 主 Entity 绑定、Claim 验证、Entity 合并与回滚、联邦 Wiki/Entity 映射、图谱查询 |
| 证据与媒体 | Source/Version/Chunk/Citation、Asset/AssetRevision、引用校验、相同证据 Citation 幂等复用、按页面聚合并可展开到 Block/Node 的反向使用、按全文首次出现统一编号的 References 投影、逐处正文回链、可审计来源与媒体目录 |
| 结构化内容 | Dataset/View/Record、Component/Version、内置 Entity/Claim 信息框（按页面语言→`und`→任意主标签降级、同属性多值聚合、Entity 可跳转、验证摘要与类型感知排序）、静态与动态 Collection、成员维护、页面反向归属查询与投影；Entity/Claim/Citation 反向使用、页面反链和 Component 依赖在 Web 中按来源页面聚合，并可展开到具体 Block/Node；列表返回真实总位置/页面/区块数并支持游标续页 |
| 治理 | ProposalOperation v1 全部 24 种 Operation（含可原子应用和补偿回滚的 Page 主 Entity 绑定）、预分配 Page/Entity ID 的同批依赖、Operation 集合事务冻结、Wiki 级多目标 Proposal、跨页面 Revision/Block、Claim 及 Page/Entity 身份冲突检测、并发唯一索引冲突到 HTTP 409 的领域映射、预览、风险、ReviewTask、面向 applier/admin 的跨 Actor 待原子应用队列、带分类失败原因和终态跳过语义的批量审核、ChangeBatch、整批补偿回滚、审计事件、ChangeTag、AI Trust 档案/策略、事实一致性；AI Trust 仅作用于预置的 `ai/import` Actor，当前用户导入仍以 human Actor 创建 Proposal |
| 导入与 AI | URL/HTML/文本/PDF/PNG/JPEG/JSON/CSV 获取，基于 DOM 主正文且排除导航/侧栏/页眉/页脚的 HTML 解析，复杂 PDF 的原生文本+逐页视觉双轨解析，表格/图表/图片结构化 Chunk 与页内区域定位，独立图片中英 OCR、来源标题/作者/发布者/日期/DOI 等安全元数据推导、结构化数据规范化、七阶段进度、幂等 Job、同 Job 多 Run/多不可变 ImportPlan 版本、解析成功后原子写入且通过 ImportJob API 可见的不可变 SourceVersion 恢复点与无重复来源的失败重试、无需 AI 配置且只为 HTML/纯文本/JSON/CSV 固化不可变来源/分片的 `source_only` 模式、Worker 中断任务自动恢复、管理员可配置模型最大输入 Token、来源 Chunk 字符数与中间计划自动确认（默认开启，最终 Proposal 审核不变）、按输入预算与输出安全上限并行分批抽取/规划、单个持久 Chunk 内的临时语义窗口二分及证据回映射、截断或结构不合法窗口重试及跨批去重、轻量模型规划 Schema 与服务端机械字段补全、确定性 ImportPlan 合并、近义内容整合和结构清理、用户标题/内容块上限的跨窗口全局执行、现有页异常扩张硬门禁、对照原始 Chunk 与用户指定范围的生成后保真审计和证据化章节修复、保真分窗的三次语义纠正、并发真因保留及坏修复隔离与覆盖率回退、服务端五维质量评分及默认 0.70 硬门槛、Semantic Kernel 默认三次结构化调用与纠正重试、精确引文的跨 Chunk 安全纠偏、纯空白差异原文回填及坏候选/坏 Claim 隔离、Entity 缩写/别名归并、Claim 方向/类型/非自引用门禁、`instance_of` 明确分类证据门禁与批内单值属性确定性消歧、作品/RFC 的发布组织、文档编号/类别/状态、更新与废止专用属性、仅由页面路由授权图谱写入、来源概况与智能多页面 create/update/link/ignore 路由、强制创建单页、更新指定页面与用户导入要求、证据约束 Typed Block 生成/补丁及正文 Entity 引用、确定性 H2 层级与标准 See also、主 Entity 绑定及信息框、显式且有证据的页面关联与反链投影、规划结果可视化、单 ImportJob 页面+Entity+Claim 复合 Proposal 与同一 ChangeBatch 原子应用、可持久查询的导入队列 |
| 投影与搜索 | Outbox 租约/重试/死信、链接/目录/锚点/章节/渲染/知识使用/References/相关推荐/组件依赖/图谱投影、可解释的链接+Collection+Entity 相关度、PostgreSQL fallback、Meilisearch 关键词/混合/语义检索 |
| 规模与归档 | 章节懒加载、服务端可信 HTML 渲染、Revision 热冷分层与 S3 回源、Projection/Search 重建、容量基准命令 |
| 协作 | Yjs WorkingDocument、持久增量 update、重连游标、普通发布与 AI 合并的 sequence CAS、未确认 update 幂等重发、自动 snapshot/compact、Block 级 Presence、三方合并与人工冲突决议；跨标签页离线恢复和多 API 实例广播仍待实现 |
| 平台 | 每账号私人 Wiki、本地账号/Session/RBAC、一次性授权码与可撤销/可清理 Agent CLI Bearer Token、162 个 OpenAPI operation 和协作 WebSocket 的 JSON + 人类命令双模式 CLI、Redis 限流、安全头、OTel/Prometheus、备份恢复、Doctor、生产部署清单 |

## 关键不变量

- Page、Revision、Entity、Claim、Source、Citation、Proposal 等权威状态只经领域
  Service 写入；Handler、Worker 和投影 Builder 不绕过服务拼接权威写入。
- Revision 与 ContentSnapshot 发布后不可变；链接、渲染、搜索、图谱和反向使用
  投影可删除并由 Outbox 重建。
- Proposal 的客户端合并只提供交互预览；服务端会按权威 AST 重新计算并拒绝不一致
  结果。24 种 Operation 的 Apply/rollback 都有领域语义。
- Page、History、Knowledge、Evidence、Component、Collection、Dataset、Governance
  和 Projection 的 UUID 读取路径校验当前 Wiki，避免跨 Wiki ID 枚举泄漏。
- Claim 值按 Property 的完整 JSON Schema 校验；Entity label/alias 的语言、长度和
  alias type 同时有服务层与数据库约束；当前内置 Entity 属性禁止 subject=target，
  导入过滤、领域服务和数据库 CHECK 三层兜底。

## 2026-10-10 复杂 PDF 双轨解析

- 导入 Parser 已改为 Adapter 路由。HTML、文本、JSON、CSV 与独立图片保持确定性解析；
  PDF 由 Poppler 同时提取原生页文本并渲染页面，私有 AI Kernel 使用管理员配置的
  多模态模型按 `document-page-analysis-v1` Schema 识别正文、表格、图表和图片。
- 文本型 PDF 以原生页文本生成精确 `page` SourceChunk，视觉输出只补充非文本元素；
  扫描页使用视觉转写。Chunk 固化 `text/table/chart/figure/page` 类型、章节路径、
  页范围、页内区域与类型化 JSON，Extraction、ImportPlan 和 fidelity 均接收这些字段。
- SourceVersion 固化 parse track、Adapter、MIME、Provider、Prompt/模型版本、解析器
  指纹和警告；相同原文件可在解析器身份变化后生成新版本。完整解析 manifest 写入对象存储并由
  `extracted_asset_id` 关联，原文件继续由 `raw_asset_id` 关联。
- 多模态调用复用既有 Prompt Registry、Gateway 限流/重试、双重 Schema 校验和
  `ai_request_usage` 审计。管理端连通性测试同时验证文本结构化输出和图片输入。
- 上传上限提升为 300 MiB，PDF 上限为 500 页，页分析并发为 3，Import Worker
  单任务恢复窗口为 2 小时。任一页面分析失败时 Parse stage 整体失败，不发布部分版本。
- 本次按预发布策略直接修改唯一初始化 Schema，不兼容旧 SourceVersion/SourceChunk
  行；部署验证必须使用重建后的空库。

## 2026-10-11 复杂文档生产验收

- Prompt Registry 会在代码请求版本高于当前活动版本时原子创建并激活新版本，同时
  保留更高的运维版本。生产 `document-page-analysis-v1` 已从 version 1 升级到
  version 2，Schema 越界 locator 先容错再由 Go 裁剪或省略。
- 事实抽取低于质量门槛但未检测到提示注入时，只丢弃不可信 Entity/Claim enrichment，
  继续由不可变原文 Chunk 生成 ImportPlan；提示注入仍以 `quality_gate` 硬失败。
- 真实三页论文和混合扫描 PDF 已完成七阶段导入并拒绝测试 Proposal；纯扫描件低于
  0.70 时进入 `action_required`，扫描提示注入不会生成 Proposal。
- AI Kernel 不可用会稳定分类为 `extraction_provider_failed`。恢复后重试复用同一
  SourceVersion，fetch/parse 显式标记 `skipped`；错误 Job 不隐藏已完成的解析证据。
- Worker 使用 `2h5m` stop grace 覆盖两小时导入窗口。生产复杂 PDF 在 parse 阶段执行
  Compose restart，等待 55 秒完成单一 Run 后健康重启，无 `worker_interrupted`。
- 截断 PDF 和 501 页 PDF 分别以 `parse_failed`、`pdf_page_limit_exceeded` 原子失败；
  上传幂等冲突返回 409 且不修改当前 Asset Revision。完整证据见
  [商业验收报告](CommercialAcceptance-2026-10-11.md)。
- 三份各 3 页且互相引用的架构、运维、事故文档已分别按正序和逆序合并验收。两种顺序
  均稳定生成 26 个 Chunk、覆盖 9 页和 19 个页内区域，并只规划三个指定页面；低于
  0.70 的结果保持在 `action_required`，未生成 Proposal。
- 同一正序与逆序 dossier 拼接为 18 页后，仍只规划三个指定页面；生产结果为 50 个
  Chunk、36 个页内区域并完整覆盖第 1–18 页，质量 0.690，最终取消且无 Proposal。
- 同一关联语料经重复、重排、0/90/180/270 度旋转及 A4/Letter 混合尺寸后，12 页
  生产结果为 28 个 Chunk、18 个页内区域，仍只规划三个指定页面；质量 0.690 后取消。
- 6 页中英双语语料把 36 行表格拆到两页，并分别按正常和扰乱页序导入。两次均得到
  6 page、2 table、1 chart，完整保留 `M-01` 至 `M-36` 和中文原文，只规划一个
  指定页面。带 33 个 EmbeddedFile 的版本不会把附件内的提示注入载荷写入 Chunk。
- 可见正文中的反提示注入措辞会在 Extraction 触发 `quality_gate`；只保留附件内攻击
  载荷后，同一页面内容完成规划。密码加密 PDF 则以 `parse_failed` 原子失败且不创建
  SourceVersion/Proposal。
- 顺序扰动曾暴露模型把 `operationalizes`、`remains authoritative` 误判为 `updates`
  的问题。关系候选现在必须由直接方向证据支持；同 Chunk 的 “It updates/obsoletes”
  只有在 240 字符内存在唯一前置作品主语且中间无其他作品时才保留。不可变 Extraction
  不回写，复用时重新执行确定性守卫。
- ImportPlan `related_to` 只表示面向已存在 Page 的 link route；同时创建的新页面之间
  的候选 Claim 不以该字段表达。
- multipart 超过 1 MiB 后写入权限为 `0700` 的宿主机临时目录；MIME、签名和摘要
  流式校验，Evidence 直传对象存储，Worker 只保留一份按 `Head` 精确分配的内容。
  单 API 进程默认只接纳一个上传，第二个并发请求返回结构化 503。
- 128 MiB 与 298 MiB 存储前 422 的 API anon 峰值降为 75.9/80.0 MiB，临时文件
  响应后均清零。298 MiB 完整 Worker Job 在 20 秒内七阶段成功、质量 1.0；
  API/Worker 峰值为 461.2/774.0 MiB，30 秒后回落至 150.3/312.8 MiB，无重启或 OOM。
- 12 次 32 MiB EmbeddedFile PDF 顺序预检共扫描约 384 MiB，单次 0.360–0.422 秒；
  每分钟三个额度耗尽时返回带 `Retry-After` 的 429，等待后恢复。每次 422 后临时目录
  都为 0，API/Worker 峰值 184.2/25.74 MiB，结束后 API 回落到 25.63 MiB，权威计数
  不变且无 OOM、重启或 Doctor issue。
- 最终生产版本为 `18944a7d0c30`，CI `38173395650`
  的 8 个 Job、Migration gate、全服务健康检查和只读 Doctor 均通过；active ImportJob、
  开放导入 Proposal、Outbox backlog/dead 与 Projection error 均为 0。

## 2026-10-21 全量元素回归修复

- 页面按钮的 `insert_blocks` Operation 现在显式发送必填的
  `expected_hash: null`，继续只提交 Proposal；OpenAPI 3.1 契约同步使用
  `string | null`。
- AST canonical JSON 在 JS 安全数值范围内支持有限小数，Go 与 Web 都输出无指数的
  最短十进制形式；音频时长、分栏比例和按钮 Dataset 值不再被旧整数约束误拒绝。
- 编辑器媒体弹窗补齐 Image/Video，并对主资源和可选 Video poster 的 AssetRevision
  MIME 做服务端元数据校验。
- HTML 与纯文本 Parser 会把标题、列表、引用、代码、固定宽度内容、公式、Todo、
  Toggle、Callout 和 Table 固化为 SourceChunk 结构 Hint。`source-import-plan-v10`
  明确保留这些服务端派生结构；规划收敛会用精确证据确定性恢复对应 Typed Block，
  删除低于既有 0.90 grounding 门槛的模型降级块，不放宽最终 0.70 质量门禁。

## 2026-10-20 页面布局与受控交互块

- Typed Block AST v1 新增 `breadcrumb`、`column_list` / `column`、`button` 与
  `html`。Breadcrumb 的稳定页面项进入反链、Doctor 与搜索投影；多列保存 0.1-10
  的宽度比例，并在 720px 以下确定性改为纵向堆叠。
- 页面按钮支持插入内容块、修改 DatasetRecord、站内通知和 Webhook。读者执行前必须
  显式确认；插块只创建并提交 Proposal，不直接发布 Revision；记录修改继续经 Dataset
  领域服务；Webhook 的 URL/payload 只能来自当前不可变 Revision，出口只允许公网
  HTTPS，并向接收端传递一次执行 ID 作为 `Idempotency-Key`。
- HTML Block 支持粘贴或上传最多 200 KB 的 HTML，并在固定高度的空 `sandbox`
  `srcdoc` iframe 中展示。内嵌 CSP 使用 `default-src 'none'`，只允许内联样式与
  `data:` 图片/字体，不授予脚本、外部网络、表单、同源或弹窗能力。
- 这些导航、动作和隔离 HTML 块只能由人工编辑/API 明确创建；Importer 继续排除来源
  导航、表单与按钮，也不会把不可信来源自动提升为可执行动作。

## 2026-10-20 媒体与外部内容块

- Typed Block AST v1 新增 `audio`、`file`、`pdf`、`bookmark` 与 `link_preview`；
  Audio/File/PDF 均固定不可变 AssetRevision，分别提供音频播放器与标题/艺术家/专辑/
  时长元数据、正文下载卡片和可配置起始页/高度的 PDF 阅读器。
- 资产目录增加 Audio/PDF 筛选，不可变版本详情可直接播放音频或阅读 PDF。Doctor 会
  校验三类新资产引用，搜索投影纳入媒体标题、caption、书签和链接预览文本。
- Bookmark 与 LinkPreview 在作者插入时通过登录态 API 安全抓取最多 512 KiB HTML，
  拒绝私网、回环、保留地址和不安全重定向，只冻结净化后的最终 URL、标题、描述和
  站点名；阅读请求不向第三方回源。
- Embed 的交互供应商白名单为 YouTube、Vimeo、Figma。provider 与 URL 结构同时匹配
  才输出受限 sandbox iframe，并由 CSP `frame-src` 精确允许；其他地址保持安全外链
  降级，不执行任意第三方脚本。
- Importer 仍只把 PDF/音视频作为来源或资产处理，不会依据不可信网页自动创建
  Bookmark、LinkPreview 或交互式 Embed；这些正文块必须由人工编辑/API 明确插入。

## 2026-10-20 行内富文本与 Mention

- AST `text` 支持 `underline` mark，以及 gray/brown/red/orange/yellow/green/blue/
  purple/pink 九色白名单内的 `text_color` 和 `background_color`。编辑器可无损往返
  BlockNote 内置样式；任意 CSS 颜色继续在发布前显式拒绝。
- Person 与 Page 分别保存稳定 Actor ID 和 Page ID；新增 `date_mention` 支持单日或
  日期区间，新增 `database_mention` 保存稳定 Dataset ID。编辑器提供统一提及入口，
  Page 继续使用页面搜索选择器，Database 使用站点 Dataset 目录。
- 服务端 Renderer v12 输出语义化 `<u>`、受限颜色 `data-*`、`<time>` 和 Dataset
  站内链接；React 阅读兜底、搜索正文、目录纯文本均同步支持。Doctor 会把不存在的
  Actor 或 Dataset 引用报告为错误。

## 2026-10-20 Dataset 多视图与交互

- DatasetView 新增 Board、Timeline、Calendar、List、Gallery、Form 与 Dashboard；
  保留 Table、Cards 和 Chart。Board 支持稳定字段分组、子分组、桌面拖放和移动端组选单，
  移动记录继续经 Dataset 领域服务执行完整 Schema 校验。
- 已部署数据库继续使用原有 `table/cards/chart` CHECK 约束；服务端把新增判别值规范化
  保存到 `config.view_mode` 并在读取时恢复，避免单一初始化迁移无法升级存量生产库。
- 视图查询支持最多三层的 AND/OR 条件树、空值/反向/数字/日期操作符、最多五字段排序链，
  以及两级分组的展开状态、空组策略、组名排序和固定组选项。旧 `filter` / `sort` /
  `group_by` 配置保持兼容。
- Timeline 与 Calendar 使用 JSON Schema `string/date` 字段；Gallery 可选择安全
  http/https 媒体封面、标题、描述和卡片尺寸；List 是独立的紧凑扫描视图。
- Form 保存字段顺序、公开状态、提交按钮和成功提示。公开端点只暴露表单允许字段，
  提交仍在 Dataset 服务内按完整 Schema 校验，并受 API 全局限流保护。
- Dashboard 最多组合 12 个同 Dataset 的非 Dashboard 保存视图，支持一至三列布局和
  运行时全局筛选；筛选通过参数化查询下推 PostgreSQL，不复制权威记录。

## 2026-09-21 导入更新精炼与扩张门禁

- HTML 获取接受完整文档及以 `<main>`、`<article>`、`<section>`、`<div>`、`<p>`、
  `<pre>` 等正文元素开头的合法 fragment；解析仍优先读取 DOM 主正文并排除导航、
  侧栏、页眉、页脚、目录和隐藏节点，防止站点容器文本进入不可变 SourceChunk 并
  诱发无关页面规划。
- `source-import-plan-v9` 与 `source-import-plan-fidelity-v6` 将用户范围、排除项和
  块数限制明确为跨窗口全局约束，并禁止用导航标签、孤立代码或示例值支撑宽泛正文。
- Typed Block AST 与编辑器支持可折叠 Heading、Todo、Toggle、自定义图标/颜色 Callout、
  带 caption/行列头的 Table、带 language/caption 的 Code、块级 Equation，以及按
  ASCII 图、固定宽度表、协议轨迹和公式文本分类的 `preformatted`。
- ImportPlan 可由精确 SourceChunk evidence 生成上述基础内容块；单路由计划会确定性
  补齐 `preformatted`，并把逐字固定宽度覆盖率纳入 fidelity 硬门禁。
- 服务端在跨窗口合并时归并近义段落；单页面显式块数上限按指令主题覆盖与引文直接
  支撑度选择代表块，并以最终块对正向要求的覆盖率约束 fidelity。未写明上限的现有页
  更新超过相对当前页规模的预算时直接失败质量门禁，不会静默截断不同事实或生成不可
  审核的大型 Proposal。
- Extraction 保留每批最多 6 个 Chunk 的输出保护；ImportPlan 在来源适配模型输入预算时
  使用一个全局窗口，只有超预算或结构化输出失败时才自适应二分，避免独立窗口重复规划
  同一章节。页面标题有唯一精确 Entity 标签时优先于别名候选，同名跨类型候选保持歧义且
  不产生权威图谱写入；用户明确点名的既有页面关联可在模型漏项时由服务端依据原文引文
  补成 link route。
- 用户明确点名的函数/方法标识符（如 `JSON.parse()`、`Response.json()`）必须全部
  出现在最终计划中，代表块选择优先补齐尚未覆盖的标识符。
- Grounding 只按正文块计算，标题不再提供无条件满分；任一正文块低于直接支撑底线时，
  即使平均分或综合分足够也不能通过质量门禁。
- `instance_of` 的分类证据必须由分类短语直接连接候选主体和值标签，避免把来源中的
  “JSON 基于 JavaScript 对象语法”等关联描述误写成分类关系。
- 保真模型连续三次返回自相矛盾的 `complete/coverage` 元数据时，服务端保守撤销覆盖率
  增益并继续隔离该窗口，不再让已通过独立证据校验的正文因可选审计元数据而整项失败。
- 回归测试覆盖 36 个近义 JSON Web 使用段落收敛为一个标题和三个互补内容块，以及
  无上限的 30 块异常更新被质量门禁拒绝。

## 2026-09-20 导入计划版本与确认流程

- 一个 ImportJob 现在可包含多个不可变 ImportPlan 版本；重新规划只更新当前规划输入、
  新增 ImportRun 和 ImportPlan，不创建新的 Job，历史版本通过 revision/parent_plan_id
  保留。
- Job 显式保存 current_plan_id 与 confirmed_plan_id。手工确认必须提交精确 plan_id，
  防止并发重新规划时确认错误版本；重复确认同一版本保持幂等。
- 管理员可在 `/admin/ai` 控制是否自动确认通过质量门禁的中间计划，默认开启。该开关
  不绕过质量门禁，也不批准或应用最终 Proposal；最终内容仍进入治理审核。
- Web 支持智能多页面、强制创建单页和更新指定页面，任务详情展示五维质量、Run 历史
  和 Plan 版本摘要，并可在同一 Job 内修改规划输入后重新排队。
- Go 全量测试、Web TypeScript/ESLint、契约副本检查和初始化 Schema
  `up → down → up` 已通过；三次公共表数量为 `85 → 1 → 85`。

## 2026-08-17 协作链路复核

- 在线 Yjs update 会按客户端幂等键和服务端单调 sequence 持久化，重连可按游标补发；
  AI Proposal 合并使用 `expected_sequence` CAS，服务端重新计算权威合并 AST。
- 普通 WorkingDocument 发布现要求 `working_document_id + expected_sequence`，
  在 Page/WorkingDocument 行锁事务内同时检查 Revision 基线与 latest sequence；
  前端只从同一 Y.Doc 物化 AST，并在本地 update 获得服务端回显前阻止发布。
- 本地 Yjs update 保留稳定幂等 ID，服务端回显前持续排队；同一标签页短暂断线时，
  客户端先恢复远端 update，再合并并重发未确认 update。浏览器关闭或重载后的离线
  恢复仍依赖 localStorage AST 草稿，不等价于持久 Yjs 操作日志。
- Presence 已接入 BlockEditor 选区、10 秒心跳、服务端排除发送者广播及 30 秒过期，
  编辑页显示远端 Actor 和稳定 Block ID；尚未提供字符级光标装饰和 Actor 名称解析。
- 客户端在 100 个已确认 durable sequence 且无 pending update 时上传最多 16 MiB 的
  Yjs state；服务端经 `SaveSnapshot(..., compact=true)` 同事务保存并删除 covered
  update，再广播 `snapshot_saved`。远端 E2E 已证明旧游标只恢复 snapshot、不再恢复
  被压缩的 update。
- 进程内 Hub 意味着多 API 副本部署前仍需补跨实例广播或连接粘性。
- 因此当前可确认在线增量同步、同标签页断线合并、Block Presence、普通发布/AI CAS、
  Revision 换基和自动压缩；不能把跨标签页离线 CRDT 恢复、字符级协作光标或多副本
  实时广播视为已验收能力。
- 当前 Internal Beta 明确采用单 API 实例，并把浏览器重启后的恢复定义为 AST 草稿
  加人工冲突处理；跨实例广播、持久离线 update 和字符级光标是非阻断后续能力。
- Go 全量测试、Vet/Build、Web TypeScript/ESLint/生产构建、OpenAPI 校验和契约检查
  已通过。BlockEditor 改为 client-only 动态加载后，浏览器验证 `/dev/editor` 从
  SSR `window is not defined` 500 恢复为 200，插入标题后编辑内容与 AST 同步更新。
- 远端 `c84233c` 双用户 E2E 通过正式注册/Session、WebSocket 和 Page API，验证
  Presence 到达对端、update 双端回显、重复 update 保持同一 sequence、陈旧 sequence
  发布返回 409、最新 sequence 发布成功、snapshot compact covered update，以及旧游标
  重连只恢复 snapshot。

## 2026-08-17 远端生产拓扑联调

- `/home/Dev/Anby-Wiki` 已快进到 `c84233c`，生产部署机顺序构建
  `ai-kernel/api/worker/web/migrate` 五个本地镜像；Web 镜像内 Next.js production
  build 和 Linux Go binaries 构建通过。
- 初始化迁移保持版本 1，Migration gate 报告 `current=1 expected=1 compatible=1..1
  dirty=false`；显式清理 6 条过期 Session 后，只读 Doctor 为 0
  info/warning/error/critical。
- PostgreSQL、Redis、MinIO、Meilisearch、Semantic Kernel、API、Worker 和 Web
  均完成滚动替换并保持 healthy；Web `/healthz`、`/readyz` 与首页探针通过。
- 常驻 Worker Prometheus 显示 Outbox backlog/pending/claimed/retrying/dead 均为 0，
  Projection error/stale 均为 0；部署后十分钟内 API/Worker/Web/AI Kernel 无 error、
  panic 或 fatal 日志。
- 部署环境文件原先缺少 AI 基础设施密钥且数据库密码与持久卷不一致；本次从仍健康的
  旧容器恢复相同值到权限 `0600` 的部署环境文件，全程未输出密钥。该环境文件已备份。

## 2026-08-18 正式域名路由恢复

- `anbywiki.kirayuukiasuna.cloud` 的宿主机 Nginx upstream 曾误指向
  `127.0.0.1:3000`，该端口属于 `homepage-web-1`；Anby Wiki 容器始终 healthy，
  正确端口为 4444。
- 站点配置已备份并改为 `proxy_pass http://127.0.0.1:4444`，`nginx -t` 通过后
  平滑 reload。Web 端口进一步收紧为 `127.0.0.1:4444`，不再直接绑定公网。
- 部署环境现设置 `SESSION_COOKIE_SECURE=true` 和正式
  `COLLABORATION_ORIGIN_PATTERNS`；API/Web 重建后保持 healthy。
- 正式 HTTPS 域名的首页、`/healthz`、`/readyz` 均为 200，未登录 Session 为 401；
  HTTPS/WSS 双用户 E2E 再次覆盖 Presence、更新、发布 CAS 和 snapshot/compact。

## 2026-08-18 全 API 修复生产部署

- 远端仓库快进到 `2e14c32`，五个业务镜像顺序构建成功；Migration gate 为
  `current=1 expected=1 compatible=1..1 dirty=false`，部署前后 Doctor 均为 0 issues。
- AI Kernel、API、Worker 已按顺序切换并健康。首次 Web 切换发现部署环境文件没有
  持久化 `WEB_BIND/WEB_PORT`，Compose 回退到已被其他站点占用的 `0.0.0.0:3000`，
  因端口冲突停止；没有回滚或修改权威数据。
- 环境文件备份后补回 `WEB_BIND=127.0.0.1`、`WEB_PORT=4444`，Web 单独重建并健康。
  正式域名 `/healthz` 返回版本 `2e14c32`，`/readyz`、首页和标题探针通过。
- API/Worker/Web/AI Kernel 均使用 `2e14c32` 镜像；Outbox backlog/dead、
  Projection error/stale 均为 0，启动后没有 error、panic 或 fatal 日志。

## 2026-08-18 Agent CLI 生产部署

- 部署前短暂停止 Web/API/Worker 写入，生成 PostgreSQL custom-format 备份、31 张
  权威表的确定性快照及 SHA-256 清单；备份耗时 5 秒，包内校验全部通过。完整恢复
  演练和对象存储恢复验收仍按 `OutstandingIssues.md` 保留。
- 生产数据库已处于 migration version 1，初始化迁移不会重放。本次在备份后以单事务
  additive DDL 补充 `cli_authorization_code`、`cli_access_token` 及其索引/约束；
  migration 保持 `current=1 dirty=false`，变更前后 31 张权威表逐行哈希完全一致。
- 远端源码和 `ai-kernel/api/worker/web/cli/migrate` 六个镜像均为 `f56876a`。
  Migration gate 通过，发布前后 Doctor 均为 0 issue，四个常驻业务容器全部 healthy。
- 正式 HTTPS 域名首页和 `/settings/cli` 返回 200，页面包含 Agent CLI 管理界面；
  匿名和无效 Bearer 均返回带 request ID 的结构化 401。生产 CLI 镜像报告版本
  `f56876a`，列出 149 个 operation，通用 `getHealthz` 调用及缺失 body 的请求校验通过。
- 发布后 Worker 指标可读，API/Worker/Web/AI Kernel 没有 error、panic 或 fatal
  日志。正式域名的登录态授权码创建、兑换和撤销仍需管理员人工登录完成一次冒烟；
  同一闭环已在隔离生产等价拓扑完成自动化 E2E。

## 2026-08-19 Agent CLI 全量接口验证

- 新增 `TestAllOperationsReachHTTPTransport`：从嵌入 OpenAPI 自动生成满足必填约束的
  path/query/header、JSON body 与 multipart fixture，162/162 个 operation 均通过
  `App.Execute("operation.call")` 到达 HTTP transport；Bearer、方法、URL、文件字段和
  响应 metadata 同时校验。
- 新增隔离全栈 `TestAllOperationsAgainstAPI`。首个管理员签发的 CLI Token 对 149 个
  operation 逐个真实调用，状态分布为 31×200、7×201、2×204、3×400、2×401、
  4×403、94×404、2×409、4×422；所有 JSON 都通过对应响应 Schema，0 个 5xx。
- CLI action 成功路径覆盖 `version`、operation 清单/过滤/描述/调用、
  `auth.exchange/status/logout`、`config.show` 和 `collaboration.run`。真实协作覆盖
  WorkingDocument recovery、opaque update、Presence、snapshot/compact；真实文件链路
  覆盖 Asset multipart、二进制 base64 逐字节还原和 Import upload multipart。
- 测试发现并修复两处协议问题：本地输入/契约错误曾被误分为 exit 1
  `request_failed`，现统一为 exit 2 `validation_failed/operation_not_found`，且非法
  logout 参数不会删除本地 Token；`nullable + enum` 的 OpenAPI 兼容转换曾遗漏
  `null` 枚举值，现可正确校验 Proposal 的空 `change_batch_status`。
- 隔离资源清理时曾因加载生产 `.env` 覆盖同名 `MEILI_INDEX`，误删可重建的生产搜索
  index；PostgreSQL 权威数据与对象存储未受影响。已立即全量重建 72/72 页面，
  Meilisearch 恢复 72 文档/72 embedding，Projection consistency 为 792/792、
  0 issue，正式搜索 API 为 200，Doctor 为 0 error/critical。两套隔离 database、
  bucket、index、Redis DB、container 和 SSH tunnel 均已清理。
- 修复提交 `0196ec6` 已完成六镜像构建和滚动发布；Migration gate 为
  `current=1 expected=1 compatible=1..1 dirty=false`，发布 Doctor 为 0
  error/critical。API/Worker/Web/AI Kernel 全部 healthy，生产 CLI 报告
  `version=0196ec6`、149 operation，缺少必填 body 返回 exit 2；发布后四个服务无
  error/panic/fatal 日志，正式健康与搜索调用均为 200。

## Web 信息架构

Web 使用桌面优先的现代百科 Shell：可浏览最近发布条目、专题与实体的百科首页、全部页面目录、固定全局导航、视口内滚动的命令面板、搜索、上下文侧栏、
目录与站点页脚。正文保留维基式可扫描排版，同时使用 shadcn/Radix、Lucide、
Sonner、Geist 和响应式卡片提升工具型页面的可读性。

阅读页由文档目录投影生成带层级编号的 `Contents`，并把 References、Related
topics 与 Related outlines 纳入同一目录；正文后按标准顺序展示可逐处回链的
References、可解释的 Related topics 与由页面所属 Collections 映射的 Related
outlines。References/Related 是 Current Revision
的可重建投影，Collection 归属和 Page 主 Entity 绑定仍来自权威领域数据。

持久入口包括：

- `/wiki/[...title]`、`/pages/[id]`、编辑、历史、Revision 详情、Diff 与页面工具；
  catch-all 路由原生支持标题中的 `/`，404 创建入口会完整保留标题；
- `/entities`、Entity/Claim/Citation 详情、知识图谱与联邦管理；
- `/sources`、`/assets`、Asset Revision、`/datasets`、`/components`、
  `/collections`；
- `/imports` 及每个 Job 的阶段、运行记录和进度详情；
- `/governance` 下的 Proposal 创建/详情、审核队列、待原子应用队列、批量审核、审计活动、页面保护、
  AI Trust、事实一致性与 Revision 存储；
- `/admin/ai` 的加密 Provider 密钥、模型、输出模式、超时、重试和连通性测试；
- `/explore` 的搜索工作台、反向链接和 `/explore/graph` 图谱工作台。

服务端数据统一通过 `apps/web/lib/api.ts` 创建的生成客户端访问，SWR 是唯一服务端
数据客户端缓存；Zustand 只保存本地交互状态。

## 2026-08-16 导入提案身份冲突修复

- Apply 在打开权威写事务前，按 Page 的 `wiki + namespace + normalized_title`
  及 Entity 的 `wiki + canonical_key` 检测审核等待期间产生的身份占用；冲突与
  Proposal 的 `conflicted` 状态在同一事务记录，不再执行注定回滚的写入。
- 唯一索引仍保留最终并发兜底；即使另一个 ChangeBatch 在预检后抢先提交，页面
  标题、Entity canonical key 和重复绑定错误也会映射为 HTTP 409，而不是 500。
- 身份冲突不会自动合并或改写已冻结 Operation。批量审核把它记录为
  `identity_conflict` 并终态跳过；页面/Revision、状态、校验、权限及未知错误分别
  保留独立分类，避免无意义重试。
- Web 会解析契约 Error 响应并展示真实原因；身份冲突详情明确要求基于 Current
  重新导入，同时禁用对该类语义冲突无效的 Current/Proposed 决议按钮。

## 搜索、渲染与存储

- `SearchAdapter` 保持产品无关接口。开发可用 PostgreSQL FTS；生产 Compose 默认
  `SEARCH_BACKEND=meilisearch`，内置固定版本 Meilisearch 服务。
- Worker 在 PostgreSQL 投影事务完成后提交 Meilisearch 文档并等待 task 终态；
  Page 行锁与 Current Revision 复核阻止旧 Revision 覆盖新索引。
- Meilisearch 支持关键词、facet/filter、高亮、混合与语义查询；本地
  HuggingFace embedder 默认不向第三方发送正文。
- 后端从已校验 AST 生成转义后的可信 HTML 和章节投影；阅读页可按章节懒加载，
  目录通过 MutationObserver 感知新章节并跟踪当前滚动位置。分章节渲染仍使用
  全文首次出现的 Citation 顺序，避免懒加载导致同一引用编号漂移。
- Revision snapshot 可由 Worker 归档到 S3，读取 API 对热/冷存储透明回源；
  归档状态、容量统计和手动归档在治理页面可见。

## 安全与依赖

- Web 使用 Next.js `16.2.12`；实际运行树固定到 PostCSS `8.5.25` 和 Sharp
  `0.35.3`。shadcn CLI 位于 devDependencies。
- 新披露的 `brace-expansion` 漏洞通过 `5.0.9` 修复；安装期幂等适配器只让
  遗留 `minimatch@3` 读取新版函数导出，未知版本会直接失败，避免“审计通过但
  ESLint 损坏”。
- 2026-08 新披露的 Nano ID 零长度生成器拒绝服务通过 `nanoid@3.3.18` override
  固定；开发工具链同时锁定已修复的 `fast-uri@3.1.5`、`hono@4.13.1`、
  `ip-address@10.5.0`、`js-yaml@4.3.1` 与 `undici@7.29.0`。
- 完整 `npm audit --audit-level=high` 与 production-only audit 均为 0。
- Go 构建基线升级到 `1.26.10`，`golang.org/x/net` 升级到 `0.60.0`，修复
  `net/http`、`html/template`、TLS、HTTP/2、MIME 与 IDNA 可达漏洞；gRPC 为
  `1.83.1`，OpenTelemetry 家族为 `1.45.0`；同时关闭 `GO-2026-6809`
  中间证书信任漏洞，`govulncheck` 报告 0 个可达漏洞。
- gitleaks 扫描全部 tracked 与未忽略 untracked 源文件，包括环境模板；忽略的本机
  `0600` 部署环境文件不再被 `dir` 模式误当成仓库内容。

## 运维与发布拓扑

- 预发布阶段只维护 `000001_initial_schema.up/down.sql`。
- 开发环境不使用 Docker；`scripts/dev.sh` 连接自行提供的 PostgreSQL、Redis、
  MinIO，并可按配置连接外部 Meilisearch。
- 生产 Compose 包含 PostgreSQL、Redis、RustFS、Meilisearch、Semantic Kernel、API、
  Worker、Web、CLI tools image、migrate 和 doctor；不包含 Nginx、OIDC 或 TLS 终结。
- 数据与应用服务继承只读根文件系统、非 root、capability drop 和
  `no-new-privileges` 策略。应用镜像在部署机本地按 `RELEASE_ID` 构建。
- `.gitattributes` 固定 Shell 与部署门禁文件为 LF，使 Windows+WSL 与 Linux CI
  执行同一套脚本。

## 2026-08-12 百科文章结构增量验证

- OpenAPI、领域服务、Worker 投影、导入 Composer 与 Web 阅读/编辑入口已完成
  References、Related topics、Related outlines、主 Entity 信息框和标准 See also 的
  跨层闭环；生成客户端由契约重新生成，未手改生成物。
- Go 全量测试、Web TypeScript 与 ESLint 已通过；渲染回归覆盖 Citation 全文统一
  编号、正文到 References 跳转以及多处引用回链。
- PostgreSQL 17 隔离空库执行初始化 Schema `up → down → up`，三次公共表数量为
  `83 → 1 → 83`；down 后仅保留 `schema_migrations`。
- 以上是本地实现与静态/隔离数据库验证；生产现有页面仍需部署后执行 Projection
  全量重建，并以真实导入页面完成浏览器和内容质量验收。

## 2026-07-31 验证快照

已通过：

- `go test ./...`、`go vet ./...`、`go build ./...`、`go mod verify`；
- CRLF/LF 等价的 gofmt 检查；
- `govulncheck@latest ./...` 与 CI 固定的 `govulncheck@v1.1.4 ./...`；
- `npm ci`/`npm install` 安装期适配、TypeScript、ESLint、Next production build；
- 完整 `npm audit` 与 production audit；
- OpenAPI Generator validate、客户端重新生成、Schema 嵌入副本一致性；
- Shell 语法、迁移文件规范、部署静态规则、Compose YAML 解析；
- 隔离空库执行唯一初始化迁移 `up → down → up`：每次 up 均得到 81 张表（含
  `schema_migrations`），down 后仅保留迁移元数据表；
- PostgreSQL 16、Redis、MinIO、Meilisearch、API、Linux Worker 与 Web 真实启动；
  页面发布/历史/Diff/回滚/改名/重定向、不可变快照、链接解析与反向链接、资产
  上传去重、Dataset/View、Component、Collection、知识图谱和治理 Apply 均完成
  API 联调；
- Meilisearch 关键词、语义与混合查询均使用真实索引和本地 HuggingFace embedder；
- 文本、PNG OCR、JSON 与 CSV 导入均经真实队列、对象存储、Worker、AI Gateway、
  Schema 校验、匹配与 Proposal 生成成功；PNG 使用真实 Tesseract，扫描 PDF 的
  Poppler + Tesseract 回退通过运行测试。重新打开浏览器后仍可从 `/imports` 找回
  Job、七阶段进度、运行记录、证据定位与 Proposal 链接；
- 全量 Projection 重建共扫描 4 页：3 页成功重建、1 个未发布页按设计跳过、
  0 失败；随后一致性检查 30/30 状态一致，Outbox backlog/dead、Projection
  error/stale 均为 0；
- 一条旧版缺少 `subject_entity_id` 的 `claim.changed` dead 事件通过兼容解码器
  回放成功；修复后的新 Claim Proposal Apply 两条事件均首轮完成；
- 非当前 Revision 经领域服务真实归档到 MinIO，API 从 cold tier 校验哈希后透明
  回源，AST 内容保持一致；
- 144/144 个 OpenAPI operationId 均由 Web 生成客户端调用路径引用，并由
  `make web-api-coverage` 验证调用文件可反向追踪到页面或全局 layout；Chrome
  headless 覆盖 34 个桌面路由、关键成功/空状态，以及真实 390×844 设备度量；
  窄屏 `html/body scrollWidth === clientWidth === 390`，斜杠标题的分段与编码
  URL 均可访问；
- 隔离远端全栈动态命中 144/144 个 OpenAPI operation handler，除故意指向
  `.invalid` 模型地址的 AI 连通性检查按契约返回 502 外，没有非预期 5xx；页面、
  Asset、Source/Citation、Dataset、Component、Collection、Entity/Claim/联邦、
  Proposal/Review/Apply、BulkReview、审计/保护/回滚、Import cancel/retry/upload、
  AI 配置脱敏、Revision 归档和双会话协作成功工作流均通过；
- Doctor 健康退出（0 error/critical；仅报告两条刻意未附 Citation 的测试 Claim
  证据警告）；
- gitleaks v8.28.0。

仍须在发布/目标环境补充：

- PostgreSQL/对象存储备份恢复演练和 RPO/RTO 记录；
- 10 万页面目标硬件容量、搜索语义质量和长时间队列/SLO 观察；
- 正式域名下的证书续期失败告警、代理恢复演练、CSRF、账号恢复/MFA 与完整键盘、
  读屏、对比度人工验收。

仓库现有 Go 单元测试、162-operation handler 探针和高风险成功工作流仍不能穷举
全部 ProposalOperation 组合、外部模型供应商行为或容量/安全/人工可访问性；本轮真实
系统演练不能替代发布环境验收。

## 2026-08-18 Web/API/CLI 可操作性审计

- 权威 OpenAPI 当前共有 162 个 operation。160 个 Web-owned operation 全部通过生成
  客户端调用，调用文件均可沿 TypeScript import 图反向到达 48 个页面/global layout
  owner；`exchangeCLIAuthCode` 与 `revokeCurrentCLIToken` 是明确的 CLI transport。
- 69 个写操作分别落在注册/登录、页面编辑与工具、导入、资产、来源、Dataset、
  Component、Collection、Entity、联邦和治理工作区，均有表单、按钮或明确命令入口。
  动态详情页都有上游列表、搜索、审计、引用或关联对象链接。
- 正式域名批量验证 26 个非动态页面和 10 个真实数据动态页面，均返回 200；全局命令
  面板可发现主要知识、共建、治理和管理工作区。
- `/metrics` 是 Prometheus 抓取端点，不属于用户操作；协作 WebSocket 是编辑器内部
  传输协议，由 `/pages/[id]/edit` 使用并经 HTTPS/WSS E2E 覆盖。
- 新增 `scripts/check-web-api-coverage.mjs` 并纳入 `make check`。以后新增 operation
  没有前端调用，或调用只存在于未挂载组件中，提交门禁会失败。

## 2026-08-18 API 语义与工作流审计

- `TestAPIContractE2E` 从权威 OpenAPI 动态读取并命中 155/155 个 operation；所有
  handler 禁止非预期 5xx，只有配置为测试 `.invalid` 地址的 `testAIConfig`
  允许契约声明的 502/504 依赖失败。
- 成功工作流覆盖 Page/Revision/Diff/rollback/rename/redirect/BlockRedirect、
  section/anchor/projection、Asset、Source/Chunk/Citation、Dataset、Component、
  Manual/Rule Collection、Entity/Claim、联邦、Entity merge/rollback、Proposal
  preview/review/apply、BulkReview 全状态机、ChangeTag/audit/protection、
  ChangeBatch rollback、Import cancel/retry/upload、AI 配置与 Revision archive。
- 正式 Session + WebSocket 双用户测试继续覆盖 Presence、update 广播、幂等重放、
  stale sequence 409、当前 sequence 发布和 snapshot/compact 恢复。
- 修复 `create_entity` 的 Schema/运行时解码漂移：`language` 和 `description` 现在
  可通过严格 payload 解码，不再让 Schema 合法 Proposal 在 Apply 预检时误报 422。
- Parse stage 完成或恢复跳过时，Worker 现在在同一事务完成 stage 并写入
  `ImportJob.source_version_id`。即使后续模型调用失败，失败任务 API 仍可导航到
  已持久化 SourceVersion/Chunk，并从同一 checkpoint 重试。
- AI Trust 的更新成功路径依赖预先存在的 `ai/import` Actor；公开注册只创建 human，
  当前用户发起的导入 Proposal 也以 human 为作者，因此该策略尚未进入普通导入闭环，
  详见 [待解决问题](OutstandingIssues.md)。

## Agent CLI

- `backend/cmd/anby-wiki` 提供 Go CLI；不带子命令或使用 `--input` 时保持供 Agent
  使用的单一 JSON envelope，传入子命令时提供面向人工的普通命令行参数封装，退出码
  仍区分成功、输入/契约错误和远端错误。
- CLI 从嵌入的权威 OpenAPI 读取 162 个 operationId，支持清单、搜索、Schema 描述和
  通用调用；所有 HTTP operation 均可通过 kebab-case 命令名或 `call <operation>`
  调用，发送前校验 path/query/header、JSON/multipart body，收到 JSON 后按状态码校验
  响应。文件上传读取本地路径，二进制响应统一转为 base64 JSON。
- Yjs 协作 WebSocket 不属于 OpenAPI，另由 `collaboration.run` 覆盖恢复、
  durable update、Presence 和 snapshot/compact，opaque bytes 均通过 base64。
- `/settings/cli` 使用浏览器 Session 生成十分钟一次性授权码，CLI 兑换只显示一次的
  Bearer Token。数据库只保存 SHA-256；后台显示前缀、状态、到期和最近使用时间并可撤销。
- CLI Token 映射到签发 Actor，不复制角色；每次请求实时读取 Actor/账号状态并继续经过
  现有 RBAC、PageProtection、Proposal Review/Apply 与领域服务边界。
- 详细协议与示例见 [Agent CLI](AgentCLI.md)。
- 隔离 PostgreSQL/Redis/Meilisearch 上的真实 E2E 已覆盖授权码兑换、Bearer Session、
  禁止 Bearer 递归签发、后台撤销、CLI 创建 Page、WorkingDocument 恢复和 CLI 自撤销。
- 初始化 Schema 在包含真实 Page 的隔离库上完成 `down → up`；同时修复了旧 down
  脚本在删除 Page 前提前删除 Namespace 种子导致的 FK 失败。

## 数据库状态

- 唯一迁移版本：`000001_initial_schema`。
- 初始化文件包含当前全部表、函数、触发器、约束、索引和固定种子；up/down 配对、
  命名与静态检查通过。
- 当前表和约束已在 PostgreSQL 17 隔离库重新执行 `up → down → up`；
  up 后 85 张表、down 后仅 `schema_migrations`，再次 up 结果一致。
- 首次生产上线后必须冻结版本 1，并恢复只增不改的增量迁移策略。
