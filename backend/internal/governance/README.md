# governance — Proposal 安全写入边界（M5）

本包是 AI 与批量变更进入权威 Page/Knowledge 数据的唯一治理入口。模型或导入流程
只能创建版本化 `ProposalOperation`，不能直接发布 Revision、修改 Claim 或写投影。

## 生命周期

```text
draft → submitted → approved → applying → applied → rolled_back
                   ↘ in_review → approved/rejected
                   ↘ conflicted
```

- `Service` 幂等创建 Proposal，并只允许 draft 追加严格有序 Operation；
- `Operation v1` 先按 JSON Schema draft 2020-12 校验，包含 Base、Target、
  Expected Hash、Evidence、Risk、Payload；
- `RiskEvaluator` 只自动批准无歧义的低风险引用改向。删除、普通正文改动、跨域外链、
  批量改名及覆盖人工验证 Claim 都进入人工审核；
- `ConflictService` 检查 Revision、Block hash、Claim state。无关块变化可三方应用，
  目标变化生成 `MergeConflict` 并禁止覆盖；
- `ApplyService` 在单事务内创建 ChangeBatch，调用 Page/Knowledge 领域服务写入，
  同步产生 Audit/Outbox 并推进 Proposal；重复 Apply 返回原 ChangeBatch；
- `RollbackService` 生成补偿 Revision 或 Claim 状态/替代关系，旧版本保持不可变；
- `AuthorizationService` 叠加 Actor 类型、Role 与 PageProtection。AI/import/anonymous
  即使误授角色也不能直接编辑、审核、Apply 或回滚；system 保留恢复通道。

## Patch 与预览

`PagePatchEngine` 和 `KnowledgePatchEngine` 都不直接拼 SQL 修改权威状态。页面 Patch 是
纯函数，按序支持 Block 与引用的插入/删除/移动/替换/改向；Knowledge Patch 只调用
`knowledge.Service` 的事务方法。`PreviewService` 只读 Base/Current，在内存生成
Proposed、双 Diff、证据与影响统计，不创建 Revision/Claim/Audit/Outbox。

## 契约与测试

- 权威 Schema：`contracts/schemas/proposal-operation/v1/operation.schema.json`；
- Go 内嵌副本：`schema/operation.schema.json`；
- 防漂移：`make contract-schema-check`；
- 集成测试必须设置 `TEST_DATABASE_URL` 并 `-p 1` 串行。`safety_test.go`、
  `apply_test.go`、`conflict_test.go`、`rollback` 相关用例覆盖伪造批准、越权、陈旧基线、
  重复请求、提交前故障与补偿回滚。
