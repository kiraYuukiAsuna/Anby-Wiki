# AI 来源导入（M6）

本包编排 `获取 → 解析 → 抽取 → 来源规划 → 匹配/冲突分类 → Proposal → Review`，但不直接写
Page、Knowledge、Evidence 或 Governance 的权威表。所有正式写入均调用相应领域服务；
模型只能产生候选，不能发布 Revision/Claim，也不能伪造 Citation。

## 运行时

API 的 `POST /api/v1/import-jobs` 只创建队列项。Provider、API 根地址、模型、输出模式、
模型最大输入 Token、来源 Chunk 字符数、超时、尝试次数和密钥由管理员在 `/admin/ai` 配置；默认
上下文上限为 128000 Token，默认持久 Chunk 上限为 32000 Unicode 字符。API Key 使用部署主密钥加密写入
`wiki_site.settings_json`，不再通过 Worker 环境变量注入。配置缺失、禁用或不可解密时，
Worker 不领取 `source_import`，已有任务保持 queued。

模型调用经私网 Semantic Kernel Sidecar：OpenAI-compatible 可使用原生 strict JSON
Schema，DeepSeek 使用 JSON Object；Sidecar 对 JSON 和权威 Schema 失败执行有限纠正重试，
Go Gateway 再执行最终独立校验。Sidecar 不访问数据库、对象存储或用户会话。

Worker 会从管理员提供的最大输入 Token 中预留 Prompt/Schema 空间，再按估算 Token 和
单批最多 6 个 Chunk 分批调用，最多并行处理 3 批；较大上下文仍受独立的输出安全批量
上限约束。每批候选分别完成 Schema 与逐字证据核验后，以服务端生成的候选 ID 去重合并。
若供应商返回 `output_truncated` 或 `invalid_structured_output`，只二分重试对应模型窗口；
即使持久 Chunk 本身只有一个，仍可在不创建新 SourceChunk 的前提下按语义边界临时二分，核验后的
证据会映射回原始 Chunk ID 和全局字符位置。获取、安全扫描和解析恢复点不会重跑；模型结构化调用
默认最多尝试 3 次。

部署环境只保留 `AI_CONFIG_MASTER_KEY`、`AI_KERNEL_URL`、
`AI_KERNEL_INTERNAL_TOKEN` 以及对象存储配置：

- `S3_ENDPOINT` / `S3_REGION` / `S3_BUCKET` / `S3_ACCESS_KEY` / `S3_SECRET_KEY`

Worker 通过 `FOR UPDATE SKIP LOCKED` 原子领取任务；每次运行有独立幂等键。优雅退出时
停止领取新任务，并给已领取任务一个有界完成窗口。相同 SourceVersion 的事实抽取可复用
不可变 Extraction；页面规划按 ImportJob 与导入要求独立生成，不能因为
来源相同而复用另一任务的 Proposal。
Worker 被强制终止或部署替换后，超过完整任务超时仍处于 running 的遗留 Run 会标记为
`worker_interrupted` 并自动重新排队，防止任务永久卡死。
同一任务在解析成功后会把不可变 SourceVersion 作为恢复点；后续失败重试复用已通过
安全扫描的资产、SourceVersion 与 Chunk，把获取和解析阶段记为 skipped，直接从抽取继续，
不会重复创建来源或再次执行安全扫描。完成或跳过 Parse stage 与写入
`ImportJob.source_version_id` 在同一事务提交，因此后续模型失败时 API 仍能展示并导航
已固化的 SourceVersion/Chunk；上传恢复点还必须与 Job 中固定的内容哈希一致。

HTTP 产品入口同时接收受控公网 URL 与 multipart 文件上传。支持 HTML、纯文本、
JSON、CSV、PDF、PNG 与 JPEG：公网 JSON 作为 API 快照，上传的 JSON/CSV 作为
数据库记录导出，图片进入 OCR。上传先由 API 经 Evidence 服务写入私有对象存储，
Job config 只保存对象键与内容哈希、不保存正文；Worker 读取后再次校验。两条路径共享
MIME/magic、10 MiB、恶意签名、哈希和证据固化门禁。

来源固化前会在最多 256 KiB 的有界窗口中推导结构化元数据：HTML `title`/meta、
JSON 常用字段、Markdown front matter 与 RFC 头可提供标题、作者、发布者、日期、
DOI/标识符和语言；URL host、文件名与 MIME 作为补充。只把这些短字段写入 Source
metadata，不记录正文、Prompt 或密钥；请求显式标题优先于自动推导结果。

Worker 使用 Poppler `pdftotext` 从标准输入读取 PDF、从标准输出取得 UTF-8 文本，
不创建临时来源文件，也不向子进程传递 Worker 环境变量。页间换页符用于保留页码定位，
解压后的文本限制为 32 MiB。没有文本层的 PDF 由 Poppler 逐页、限尺寸栅格化后交给
Tesseract；最多处理 20 页，单页临时图片随页删除，原 PDF 始终只经标准输入传递。
PNG/JPEG 使用同一 OCR 路径，限制 4000 万像素。OCR 的行级像素区域、原图尺寸、引擎、
语言与置信度一并固化进 SourceChunk locator，Citation 可以还原到原始图像区域。

DeepSeek 的 JSON Object 模式只保证返回合法 JSON，因此 Semantic Kernel 会使用与
Gateway 相同的权威 JSON Schema 预校验并纠正失败输出；OpenAI compatible 可使用供应商
原生 strict JSON Schema。抽取固定 `temperature=0`，DeepSeek 同时关闭 thinking，避免
推理正文污染 JSON 结果。

`source-extraction-v7` Prompt 同时提供当前固定 Entity type / Claim property 词表，
明确要求模型生成临时 `candidate_id`、用 `entity_candidate_id` 表达同批 Entity 值、
禁止猜测持久化 Entity ID，并为 Entity 关系规定严格的 subject/value 类型和方向；
其中作品/RFC 可使用 `issued_by`、`document_identifier`、`document_category`、
`document_status`、`updates` 与 `obsoletes`，不再把发布组织错误压成 `part_of`。
参考文献、致谢、样板与联系信息中的孤立名称不会仅因出现而成为候选。跨批合并还会归并
缩写/全名与 label/alias 命中的同一 Entity，并确定性拒绝方向、类型不成立或主体等于
Entity 值的自引用 Claim；相同约束在逐字证据核验、稳定 ID 分类与 Knowledge 写入边界
重复兜底，旧 Extraction 复用时也会重新执行。`instance_of` 仅保留引文中明确表达分类
关系的候选，不再把相邻技术术语当作通用关联边；同一主体的单值属性若在一个模型批次中
出现多个值，分类器按“支持现有事实优先，其次置信度、证据覆盖、稳定 ID”只保留一个，
Composer/Apply 前置校验与 Knowledge 写入约束继续防御不可能应用的 Proposal。
事实候选允许为空：来源随后仍会进入 `source-import-plan-v6`，由不受固定 Entity/Claim
词表限制的页面规划判断百科价值。模型提供的
引文必须逐字存在；服务端会重新推导 rune 范围以纠正模型常见的 Unicode/字节计数偏差。
引文重复出现时选择离模型提示位置最近的精确匹配，最近距离并列才拒绝；模糊匹配、翻译
或改写文本始终不能成为证据。
当模型复制了正确的逐字引文但错指 Chunk UUID 时，导入领域层只会在该引文能唯一定位到同一
SourceVersion 的某个 Chunk 时纠正引用；仍无法核验的证据与候选会被丢弃并按保留比例降低
质量分，而不再让单条坏证据否决整批有效候选。没有任何可核验证据时仍严格失败。
模型回显错误 SourceVersion 时会使用请求中的权威 ID 纠正；重复的临时候选 ID、悬空或
歧义 Claim 引用以及非法有效期只淘汰受影响的 Claim，不再否决同批其他可核验候选。
淘汰比例导致整体质量分低于门槛时，仅在整体分仍达到半门槛、保留候选平均置信度达到
完整门槛且每个候选都有已核验证据时继续；Prompt Injection 始终直接拒绝。
来源标题或上传文件名会作为主体发现上下文，但不能单独构成证据；需求、规格和技术报告
中的候选仍必须由 Chunk 内逐字引文支持。

页面计划完成后，只有与 create/update 路由标题直接匹配的 Entity 才进入匹配；来源主题概况
只用于发现和写作，不授权图谱写入。有效 Claim 可以从已选主体引入其 Entity 值依赖。
参考文献里的其余候选留在不可变抽取证据中，
但不会无差别变成 Proposal 写操作。匹配后，服务端生成并固化在 Extraction 中的 Entity
`candidate_id` 直接作为新 Entity
的最终预分配 UUID。一次 ImportJob 只合成一个复合 Proposal：所有 `create_entity`
Operation 排在 Claim Operation 之前，Claim 的主体和 Entity 值在 Compose 时解析为
已存在或预分配的稳定 Entity UUID。整组 Operation 在一个治理事务中冻结，审核后再由
一个 ChangeBatch 原子应用；任一 Entity、Claim、Citation 绑定、Audit 或 Outbox 写入
失败都会回滚整批，不会产生只创建部分 Entity 的中间状态。
单个 Claim 若使用未登记属性、或值结构与属性类型不匹配，只淘汰该 Claim；同批其余
可核验 Entity/Claim 继续进入治理。新实体 canonical key 使用 `type_key:label`，避免
不同类型的同名实体在整批应用时互相冲突。

`source-import-plan-v6` 在事实抽取之后执行来源理解与页面路由。它先按标题、来源名、
Entity/alias 召回已有 Page 及可替换 Block，再允许一份来源同时生成多个 `create`、
`update`、`link` 与 `ignore` 路由。`link.related_to` 显式选择同批 create/update
页面并携带原文证据，随后编译为稳定 `page_reference`，由投影生成反链；每个新写或改写
Block 都必须绑定可逐字核验的 SourceChunk evidence；update/replace 只能引用服务端给出的
Page/Block ID。模型只输出页面语义、正文和 `chunk_id + quotation`；SourceVersion、字符范围、页码、
空集合、Block 模式和质量分均由服务端补齐，避免让模型重复计算机械字段。输出过长、结构错误或局部
证据失败时按临时模型窗口自适应二分；窗口仍有语义错误时，服务端携带校验反馈最多尝试三次。
对 RFC 等硬换行文本，
只允许将字母、标点和大小写全部一致的纯空白差异回填为不可变 Chunk 中的原始逐字引文；
独立窗口并行规划后直接经过确定性的路由/段落合并、无内容标题与非正文区段清理，不再用一次
大输出 LLM 调用重写整份计划，避免可选的“收敛”步骤成为新的单点失败。
作者、编辑、贡献者、出版者或发布机构若只出现在文档头、署名、联系方式或
元数据中，规划与合并阶段都不为其生成独立页面；只有来源提供独立人物或机构
内容时才保留路由。Composer 调用 Citation 领域服务后再按稳定 ID 去重，同一段不可变
证据在重试、多窗口或多路由中只会建立一个 Citation 记录。

确定性合并后由 `source-import-plan-fidelity-v4` 按原始 Chunk 分窗比较完整页面计划，逐项检查定义、
约束、禁止事项、条件、例外、先后顺序、数量、互操作和安全要求。模型只能返回带精确 Chunk
引文的遗漏段落；模型同样只给 `chunk_id + quotation`，服务端重新定位并把修复插入已有路由和章节；不能借审计新建页面或猜测
章节，目标页面语言也不得用于翻译 evidence 引文。单个保真分窗经过三次纠正仍含坏修复时，
只丢弃无法核验的修复并撤销其覆盖率增益，已经独立核验的页面计划和同窗修复不会被连带丢弃；
回退后的覆盖率仍必须通过质量门槛。最终 `quality_score` 不再采用模型自评分，而由服务端按原文保真度 35%、证据支撑 25%、
文章结构 20%、去重精炼 10% 和路由置信度 10% 计算。保真度低于 0.70、证据或结构存在硬伤，
或综合分低于管理员阈值（默认 0.70）时，计划停在 Plan 阶段，不生成 Proposal；达到门槛才进入
匹配和审核。这借鉴了 VeronicaWIKI 的生成后保真审计，但修复仍遵守 Anby 的不可变证据模型，
不会把无法定位的“遗漏原句”作为兜底正文堆入页面。

Composer 把页面路由与 Entity/Claim 决策合成为一个以 Wiki 为目标的复合 Proposal；正文中
实际采用的 Entity 会编译为稳定 `entity_reference`，标题证据继续保留在 Operation 上但不
渲染多余的行内 Citation。创建页预分配 Page ID，更新页带 Revision 与 Block hash 基线，审核时可按页面查看全部
Operation。Apply 先做跨页面 Revision/Block 与 Claim 冲突检测，再在单一事务中创建/更新
多个页面和知识对象；任一步失败整批回滚。成功响应返回全部 `revision_ids`，ChangeBatch
回滚仍按审计账本对整批追加补偿。

文章结构不交给模型自由发挥：服务端把正文 heading 归一为从 H2 开始且不跳级，删除
模型生成的 References/See also/Further reading/External links 等尾部样板；有关联时
确定性生成标准 H2 `参见`/`See also` 与 PageReference 列表。路由能唯一匹配主 Entity
时，Composer 同批写入 `set_page_entity_binding` 并在文首插入内置
`article-infobox` Component；更新页会复用现有标准章节与 Block ID，避免重复尾部和
信息框。References 本身由已核验 Citation 的 Current Revision 投影生成，不复制模型
书目文本，也不把页面关联伪装成 Citation。

## 安全边界

- URL 仅允许 HTTP(S) 80/443，逐次校验 DNS 与重定向；Dial 时再次拒绝内网、回环、
  link-local、CGNAT、benchmark 等地址，防 DNS rebinding。
- 原始资产和 Source 在解析前持久化；解析失败不丢失原始证据。
- HTML/PDF/图片/JSON/CSV 只产出稳定 Chunk、页码/章节/字符范围或图片区域，不执行
  来源内脚本或指令；Poppler 与 Tesseract 在只读、非 root、capabilities 全移除的
  Worker 容器内运行。
- 抽取结果先经权威 JSON Schema，再逐条核对 `source_version_id`、Chunk ID、原文引用和
  字符范围；检测到 Prompt Injection 或质量低于阈值即停止，不创建 Citation/Proposal。
- 实体歧义只进入人工 Review；不自动合并或创建重复实体。Claim 会区分新增、支持、
  矛盾、替代，并对人工验证 Claim 提升风险。
- Compose 后的 Operation 再经 Operation v1 Schema；Apply 仍需 M5 权限、审核、冲突和
  原子事务门禁。
- `error_json` 只保存阶段与稳定错误码，不保存来源全文、Prompt、密钥或供应商响应体；
  PDF/OCR 与抽取失败会区分组件缺失、页数或像素超限、未识别文本、供应商失败、超时、
  Schema 不合规和证据不合规。
