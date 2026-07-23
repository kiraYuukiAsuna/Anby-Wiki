# ProposalOperation v1

operation.schema.json 是 Proposal 有序原子操作的跨语言权威契约（JSON Schema
draft 2020-12）。Go 运行时在 backend/internal/governance/schema/ 内嵌字节级副本，
两者由 scripts/check-operation-schema-sync.sh 和 Go 契约测试共同防漂移。

所有操作必须携带：

- base：基础 Revision 或知识状态版本；
- target：稳定 Page/Block/Node/Entity/Claim/Citation ID；
- expected_hash：目标旧值的 SHA-256，创建类操作可显式为 null；
- evidence：Citation、SourceChunk 或人工说明；
- risk：风险级别与可解释原因；
- payload：新值或操作参数。

schema_version=1 的操作类型只做 additive 演进；删除字段、改变既有语义或新增必填
字段必须新建 v2。
