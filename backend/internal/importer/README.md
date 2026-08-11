# AI 来源导入（M6）

本包编排 `获取 → 解析 → 抽取 → 匹配/冲突分类 → Proposal → Review`，但不直接写
Page、Knowledge、Evidence 或 Governance 的权威表。所有正式写入均调用相应领域服务；
模型只能产生候选，不能发布 Revision/Claim，也不能伪造 Citation。

## 运行时

API 的 `POST /api/v1/import-jobs` 只创建队列项。Provider、API 根地址、模型、输出模式、
模型最大输入 Token、超时、尝试次数和密钥由管理员在 `/admin/ai` 配置；API Key 使用部署主密钥加密写入
`wiki_site.settings_json`，不再通过 Worker 环境变量注入。配置缺失、禁用或不可解密时，
Worker 不领取 `source_import`，已有任务保持 queued。

模型调用经私网 Semantic Kernel Sidecar：OpenAI-compatible 可使用原生 strict JSON
Schema，DeepSeek 使用 JSON Object；Sidecar 对 JSON 和权威 Schema 失败执行有限纠正重试，
Go Gateway 再执行最终独立校验。Sidecar 不访问数据库、对象存储或用户会话。

Worker 会从管理员提供的最大输入 Token 中预留 Prompt/Schema 空间，再按估算 Token 和
单批最多 6 个 Chunk 分批调用，最多并行处理 3 批；较大上下文仍受独立的输出安全批量
上限约束。每批候选分别完成 Schema 与逐字证据核验后，以服务端生成的候选 ID 去重合并。
若供应商返回 `output_truncated` 或 `invalid_structured_output`，只二分重试对应批次；
已完成批次以及获取、安全扫描、解析恢复点均不会重跑。模型结构化调用默认最多尝试 3 次。

部署环境只保留 `AI_CONFIG_MASTER_KEY`、`AI_KERNEL_URL`、
`AI_KERNEL_INTERNAL_TOKEN` 以及对象存储配置：

- `S3_ENDPOINT` / `S3_REGION` / `S3_BUCKET` / `S3_ACCESS_KEY` / `S3_SECRET_KEY`

Worker 通过 `FOR UPDATE SKIP LOCKED` 原子领取任务；每次运行有独立幂等键。优雅退出时
停止领取新任务，并给已领取任务一个有界完成窗口。相同 SourceVersion 已有成功任务时，
后续任务跳过抽取、匹配、Compose 与 Review，并复用原 Proposal。
Worker 被强制终止或部署替换后，超过完整任务超时仍处于 running 的遗留 Run 会标记为
`worker_interrupted` 并自动重新排队，防止任务永久卡死。
同一任务在解析成功后会把不可变 SourceVersion 作为恢复点；后续失败重试复用已通过
安全扫描的资产、SourceVersion 与 Chunk，把获取和解析阶段记为 skipped，直接从抽取继续，
不会重复创建来源或再次执行安全扫描。上传恢复点还必须与 Job 中固定的内容哈希一致。

HTTP 产品入口同时接收受控公网 URL 与 multipart 文件上传。支持 HTML、纯文本、
JSON、CSV、PDF、PNG 与 JPEG：公网 JSON 作为 API 快照，上传的 JSON/CSV 作为
数据库记录导出，图片进入 OCR。上传先由 API 经 Evidence 服务写入私有对象存储，
Job config 只保存对象键与内容哈希、不保存正文；Worker 读取后再次校验。两条路径共享
MIME/magic、10 MiB、恶意签名、哈希和证据固化门禁。

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

`source-extraction-v3` Prompt 同时提供当前固定 Entity type / Claim property 词表，
明确要求模型生成临时 `candidate_id`、禁止猜测持久化 Entity ID，并在来源包含明确主体
但没有受支持 Claim 时仍生成 Entity 候选。合法但空的候选集合会在 Extract 阶段以
`no_candidates_extracted` 停止，不再到 Compose 阶段才显示无 Proposal。模型提供的
引文必须逐字存在；服务端会重新推导 rune 范围以纠正模型常见的 Unicode/字节计数偏差。
引文重复出现时选择离模型提示位置最近的精确匹配，最近距离并列才拒绝；模糊匹配、翻译
或改写文本始终不能成为证据。
当模型复制了正确的逐字引文但错指 Chunk UUID 时，导入领域层只会在该引文能唯一定位到同一
SourceVersion 的某个 Chunk 时纠正引用；仍无法核验的证据与候选会被丢弃并按保留比例降低
质量分，而不再让单条坏证据否决整批有效候选。没有任何可核验证据时仍严格失败。
模型回显错误 SourceVersion 时会使用请求中的权威 ID 纠正；重复的临时候选 ID、悬空或
歧义 Claim 引用以及非法有效期只淘汰受影响的 Claim，不再否决同批其他可核验候选。
来源标题或上传文件名会作为主体发现上下文，但不能单独构成证据；需求、规格和技术报告
中的候选仍必须由 Chunk 内逐字引文支持。

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
