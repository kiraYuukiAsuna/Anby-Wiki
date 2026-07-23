# ADR-0015：AI 合并采用客户端辅助 Yjs CAS

状态：已接受（M8-T06）
日期：2026-07-23

## 背景

Governance 已能按 Base/Current/Proposed 检测语义冲突并持久化 MergeConflict，但 Go
协作服务只把 Yjs update 作为 opaque binary 存储，不能安全地把合并 AST 转成 Yjs
update。仅发布 AI Revision 后修改 WorkingDocument base 会让仍含人工编辑的工作副本
在下一次发布时覆盖 AI 内容。

## 决策

- 不引入 Node/Yjs sidecar；Yjs 物化和 delta 生成继续留在既有浏览器 Adapter。
- 客户端基于已恢复到 server sequence `S` 的 Y.Doc 物化 Current Working AST，调用
  Governance 三方合并，得到 Merged AST 或显式 MergeConflict。
- 无冲突时，客户端在克隆 Y.Doc 上把 Merged AST reconcile 为 Yjs delta，并连同
  `working_document_id`、`expected_sequence=S` 和幂等 update ID 提交。
- 服务端在单事务锁定 WorkingDocument；仅当 active 且 `latest_sequence=S` 时接纳
  delta、推进 sequence，并完成 Proposal/ChangeBatch/Audit 状态变更。
- sequence 不一致返回冲突，客户端必须恢复最新 update、重新执行三方合并；不得把
  旧 delta 强行追加。
- 冲突时不生成 delta，Base/Current/Proposed 值写入现有 MergeConflict，由 M8-T07
  显式解决。
- AI Actor 仍不能直接写正式内容；最终 Revision 发布继续经过人工身份和 Page
  Service。合并到 WorkingDocument 不等于正式发布。

## 影响

- M8-T06 需要一个“预览三方合并”和一个“CAS 接纳 Yjs delta”的治理边界。
- CollaborationClient 需要暴露当前 durable sequence，并能在克隆文档上生成 delta。
- M8-T08 必须覆盖预览后有人类 update 到达、CAS 拒绝、恢复后重试并最终收敛。
- Yjs update 内容不写日志；服务端只记录 Proposal、WorkingDocument、sequence 和
  ChangeBatch 标识。
