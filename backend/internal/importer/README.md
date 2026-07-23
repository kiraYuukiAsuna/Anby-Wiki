# AI 来源导入（M6）

本包编排 `获取 → 解析 → 抽取 → 匹配/冲突分类 → Proposal → Review`，但不直接写
Page、Knowledge、Evidence 或 Governance 的权威表。所有正式写入均调用相应领域服务；
模型只能产生候选，不能发布 Revision/Claim，也不能伪造 Citation。

## 运行时

API 的 `POST /api/v1/import-jobs` 只创建队列项。`cmd/worker` 在显式设置
`AI_IMPORT_ENABLED=true` 后领取 `source_import`：

- `AI_PROVIDER=openai-compatible`
- `AI_BASE_URL`：支持 `/chat/completions` 和 JSON Schema structured output 的 API 根地址
- `AI_API_KEY`：只允许经环境变量注入
- `AI_MODEL`：抽取模型 ID
- `S3_ENDPOINT` / `S3_REGION` / `S3_BUCKET` / `S3_ACCESS_KEY` / `S3_SECRET_KEY`

Worker 通过 `FOR UPDATE SKIP LOCKED` 原子领取任务；每次运行有独立幂等键。优雅退出时
停止领取新任务，并给已领取任务一个有界完成窗口。相同 SourceVersion 已有成功任务时，
后续任务跳过抽取、匹配、Compose 与 Review，并复用原 Proposal。

HTTP 产品入口同时接收受控公网 URL 与 multipart 文件上传。上传先由 API 经 Evidence
服务写入私有对象存储，Job config 只保存对象键与内容哈希、不保存正文；Worker 读取后
再次校验。两条路径共享 MIME/magic、10 MiB、恶意签名、哈希和证据固化门禁。

## 安全边界

- URL 仅允许 HTTP(S) 80/443，逐次校验 DNS 与重定向；Dial 时再次拒绝内网、回环、
  link-local、CGNAT、benchmark 等地址，防 DNS rebinding。
- 原始资产和 Source 在解析前持久化；解析失败不丢失原始证据。
- HTML/PDF 只产出稳定 Chunk、页码/章节/字符范围，不执行来源内脚本或指令。
- 抽取结果先经权威 JSON Schema，再逐条核对 `source_version_id`、Chunk ID、原文引用和
  字符范围；检测到 Prompt Injection 或质量低于阈值即停止，不创建 Citation/Proposal。
- 实体歧义只进入人工 Review；不自动合并或创建重复实体。Claim 会区分新增、支持、
  矛盾、替代，并对人工验证 Claim 提升风险。
- Compose 后的 Operation 再经 Operation v1 Schema；Apply 仍需 M5 权限、审核、冲突和
  原子事务门禁。
- `error_json` 只保存阶段与稳定错误码，不保存来源全文、Prompt、密钥或供应商响应体。

## 验证

核心回归位于 `pipeline_test.go`、`golden_test.go`、`acquisition_test.go`、
`extraction_test.go`、`matching_test.go`、`classifier_test.go`、`composer_test.go`。
PostgreSQL 用例必须按仓库约定设置 `TEST_DATABASE_URL` 并使用 `-p 1` 串行执行。
