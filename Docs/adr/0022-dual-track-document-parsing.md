# ADR-0022：复杂文档采用文本与视觉双轨解析

- 状态：Accepted
- 日期：2026-10-10

## 背景

原导入解析器以 `pdftotext` 为主，无文本层时回退到 Tesseract。它能保留页码和 OCR
区域，但复杂 PDF 中的多栏阅读顺序、表格、图表和图片语义会退化为扁平文本或直接丢失。
ImportPlan 因此无法可靠使用图表数据，也无法区分原生文本、表格和视觉解释。

## 决策

1. `Parser` 改为 Adapter 路由。HTML、文本、JSON、CSV 和独立图片继续走确定性轨道；
   PDF 走原生文本与逐页视觉分析双轨。
2. PDF 每页由 Poppler 同时提取原生文本并渲染 JPEG。私有 AI Kernel 使用管理员配置的
   多模态模型，按权威 JSON Schema 输出 `text/table/chart/figure` 元素、层级路径、
   归一化区域、结构化数据、置信度和警告。
3. 有文本层的页面以原生页文本生成 `page` Chunk，视觉输出只补充表格、图表和图片；
   无可靠文本层时使用视觉转写。两条轨道统一写入不可变 SourceChunk。
4. SourceVersion 固化 parse track、Adapter 名称/版本、MIME、Provider、Prompt 版本、
   模型和解析警告。
   `(source, content hash, parser fingerprint)` 是解析制品身份；解析器、Prompt、模型或
   Chunk 上限变化均可为相同原文件生成新版本。
5. SourceChunk 固化 `chunk_type`、定位信息、可核验 `text_content` 和类型化
   `content_json`。完整解析 manifest 另存对象存储并由
   `source_version.extracted_asset_id` 引用。
6. 视觉调用复用 AI Gateway、Prompt Registry、Schema 双重校验、重试、并发限制和
   `ai_request_usage` 审计。Provider 密钥不进入 ImportJob、日志或解析制品。
7. 上传上限提升到 300 MiB，PDF 上限为 500 页，页视觉分析并发为 3。整页解析失败时
   整个 Parse stage 失败，不静默发布部分 SourceVersion。

## 后果

- 表格、图表和图片成为可定位、可审计、可进入 Extraction/ImportPlan 的来源证据。
- 文本型 PDF 不因视觉模型改写而失去逐字引用能力。
- PDF 解析要求管理员配置支持图片输入和结构化 JSON 的模型，成本与耗时显著高于普通
  文本解析；管理端连接测试同时验证文本与图片输入。
- 本次不保留旧 SourceVersion/SourceChunk Schema 兼容。预发布环境必须从更新后的
  `000001_initial_schema` 重建数据库，不能把旧解析行直接视为新契约数据。
