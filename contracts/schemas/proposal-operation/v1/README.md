# ProposalOperation v1

operation.schema.json 是 Proposal 有序原子操作的跨语言权威契约（JSON Schema
draft 2020-12）。Go 运行时在 backend/internal/governance/schema/ 内嵌字节级副本，
两者由 `scripts/check-contracts.sh` 防漂移。

所有操作必须携带：

- base：基础 Revision 或知识状态版本；
- target：稳定 Page/Block/Node/Entity/Claim/Citation ID；
- expected_hash：目标旧值的 SHA-256，创建类操作可显式为 null；
- evidence：Citation、SourceChunk 或人工说明；
- risk：风险级别与可解释原因；
- payload：新值或操作参数。

`create_entity` 必须在 target 同时携带 `wiki_id` 和预分配的最终 `entity_id`。因此同一
Proposal 中排在后面的 `create_claim` 可以直接引用稳定 ID，Apply 不需要临时候选映射。

`add_collection_membership` 的 `sort_key` 必填。Collection 目标提案仍须显式提供
`source_revision_id`；Wiki 目标提案可为当前 Wiki 的页面省略该字段，Apply 在本批次
所有页面发布后绑定其当前 Revision。Wiki 提案仅支持现有手动专题的页面成员，
不允许跨 Wiki 或写入规则/动态集合。重复加入保留既有成员元数据且不产生新增审计，
因此回滚不会误删先前存在的归属。

项目尚处预发布阶段，schema_version=1 在冻结前允许直接替换；冻结后删除字段、改变
既有语义或新增必填字段必须新建 v2。
