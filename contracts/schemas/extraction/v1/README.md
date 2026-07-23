# Extraction Candidates v1

`candidates.schema.json` 是 M6 模型结构化抽取的跨语言权威契约。Entity 与 Claim
候选都必须携带 SourceChunk ID、原文 quotation 和字符范围；只有通过 Schema、
Chunk 归属与 quotation 子串复核的候选才能进入匹配/Proposal Composer。

模型输出视为不可信输入。新增可选字段可在 v1 additive 演进；删除字段、改变既有语义
或新增必填字段必须发布新版本。
