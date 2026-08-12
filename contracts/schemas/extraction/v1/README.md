# Extraction Candidates v1

`candidates.schema.json` 是 M6 模型结构化抽取的跨语言权威契约。Entity 与 Claim
候选都必须携带 SourceChunk ID、原文 quotation 和字符范围；只有通过 Schema、
Chunk 归属与 quotation 子串复核的候选才能进入匹配/Proposal Composer。
Entity 值使用 `entity_candidate_id` 引用同批候选；只有输入明确给出已有 Wiki Entity
UUID 时才允许 `entity_id`。主体和值引用都会在证据校验和跨批合并时同步校验、重映射。

模型输出视为不可信输入。本项目尚处预发布阶段，v1 在冻结前允许直接替换；冻结后删除
字段、改变既有语义或新增必填字段必须发布新版本。
