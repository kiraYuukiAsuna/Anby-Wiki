# AI 来源导入（M6）

本包编排 `获取 → 解析 → 抽取 → 匹配/冲突分类 → Proposal → Review`，但不直接写
Page、Knowledge、Evidence 或 Governance 的权威表。所有正式写入均调用相应领域服务；
模型只能产生候选，不能发布 Revision/Claim，也不能伪造 Citation。

## 运行时

API 的 `POST /api/v1/import-jobs` 只创建队列项。`cmd/worker` 在显式设置
`AI_IMPORT_ENABLED=true` 后领取 `source_import`：

- `AI_PROVIDER=openai-compatible`：供应商必须支持 `/chat/completions` 的严格
  `response_format=json_schema`
- `AI_PROVIDER=deepseek`：使用 DeepSeek `response_format=json_object`，并在本地执行同一份
  权威 JSON Schema 校验
- `AI_BASE_URL`：供应商 API 根地址；DeepSeek 使用 `https://api.deepseek.com`
- `AI_API_KEY`：只允许经环境变量注入
- `AI_MODEL`：抽取模型 ID
- `S3_ENDPOINT` / `S3_REGION` / `S3_BUCKET` / `S3_ACCESS_KEY` / `S3_SECRET_KEY`

Worker 通过 `FOR UPDATE SKIP LOCKED` 原子领取任务；每次运行有独立幂等键。优雅退出时
停止领取新任务，并给已领取任务一个有界完成窗口。相同 SourceVersion 已有成功任务时，
后续任务跳过抽取、匹配、Compose 与 Review，并复用原 Proposal。

HTTP 产品入口同时接收受控公网 URL 与 multipart 文件上传。上传先由 API 经 Evidence
服务写入私有对象存储，Job config 只保存对象键与内容哈希、不保存正文；Worker 读取后
再次校验。两条路径共享 MIME/magic、10 MiB、恶意签名、哈希和证据固化门禁。

Worker 使用 Poppler `pdftotext` 从标准输入读取 PDF、从标准输出取得 UTF-8 文本，
不创建临时来源文件，也不向子进程传递 Worker 环境变量。页间换页符用于保留页码定位，
解压后的文本限制为 32 MiB。当前只支持带可提取文本层的 PDF；纯图片扫描件返回
`pdf_ocr_required`，尚不执行 OCR。

DeepSeek 的 JSON Object 模式只保证返回合法 JSON，因此 Adapter 会把与 Gateway
本地校验完全相同的权威 JSON Schema 注入 system message，禁止额外包装层；OpenAI
compatible Adapter 继续使用供应商原生 strict JSON Schema。DeepSeek 抽取使用
`temperature=0`，避免同一来源在重试之间产生随机空结果。

`source-extraction-v2` Prompt 同时提供当前固定 Entity type / Claim property 词表，
明确要求模型生成临时 `candidate_id`、禁止猜测持久化 Entity ID，并在来源包含明确主体
但没有受支持 Claim 时仍生成 Entity 候选。合法但空的候选集合会在 Extract 阶段以
`no_candidates_extracted` 停止，不再到 Compose 阶段才显示无 Proposal。模型提供的
引文必须逐字存在；服务端会重新推导 rune 范围以纠正模型常见的 Unicode/字节计数偏差。
引文重复出现时选择离模型提示位置最近的精确匹配，最近距离并列才拒绝；模糊匹配、翻译
或改写文本始终不能成为证据。
来源标题或上传文件名会作为主体发现上下文，但不能单独构成证据；需求、规格和技术报告
中的候选仍必须由 Chunk 内逐字引文支持。

## 安全边界

- URL 仅允许 HTTP(S) 80/443，逐次校验 DNS 与重定向；Dial 时再次拒绝内网、回环、
  link-local、CGNAT、benchmark 等地址，防 DNS rebinding。
- 原始资产和 Source 在解析前持久化；解析失败不丢失原始证据。
- HTML/PDF 只产出稳定 Chunk、页码/章节/字符范围，不执行来源内脚本或指令；PDF
  解析进程在只读、非 root、capabilities 全移除的 Worker 容器内运行。
- 抽取结果先经权威 JSON Schema，再逐条核对 `source_version_id`、Chunk ID、原文引用和
  字符范围；检测到 Prompt Injection 或质量低于阈值即停止，不创建 Citation/Proposal。
- 实体歧义只进入人工 Review；不自动合并或创建重复实体。Claim 会区分新增、支持、
  矛盾、替代，并对人工验证 Claim 提升风险。
- Compose 后的 Operation 再经 Operation v1 Schema；Apply 仍需 M5 权限、审核、冲突和
  原子事务门禁。
- `error_json` 只保存阶段与稳定错误码，不保存来源全文、Prompt、密钥或供应商响应体；
  PDF 与抽取失败会区分组件缺失、需 OCR、文本过大、供应商失败、超时、Schema
  不合规和证据不合规。
