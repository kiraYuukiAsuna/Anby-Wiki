# ADR-0014：WorkingDocument CRDT 采用 Yjs

状态：已接受（M8-T01）
日期：2026-07-23

## 背景

M8 需要支持字符编辑、Block 增删/移动、属性修改、离线合并和多客户端收敛，同时
保持 Typed Block AST v1 是正式内容契约。CRDT 状态只属于 WorkingDocument，不能
替代不可变 Revision/ContentSnapshot，也不能把自动收敛误当成语义冲突裁决。

现有编辑器是 BlockNote/ProseMirror，AST Adapter 已保证稳定 UUIDv7 Block ID 和
AST v1 无损往返。BlockNote 已声明 Yjs/y-prosemirror 协作生态兼容。

## 决策

- WorkingDocument CRDT 采用 **Yjs 13.6.27**，依赖固定精确版本。
- AST 对象映射为 `Y.Map`，有序集合映射为 `Y.Array`，可协作正文映射为 `Y.Text`。
- `id`、`type`、引用目标、URL、枚举等结构字段保持标量；CRDT 内部 item ID 不得
  替代 AST UUIDv7 Block ID。
- 块移动由 Adapter 物化后执行删除/插入，必须保留 AST Block ID；最终物化时统一
  检查结构和 ID。
- 网络和存储使用 Yjs binary update/state vector；服务端另分配单调 sequence，
  sequence 只用于持久化游标和断线补发，不改变 CRDT 因果关系。
- 每次发布前必须将 Yjs 状态物化为 JSON，并通过现有 `parseDocument` 和 Go
  `ast.ValidateJSON`；验证失败不得创建 Revision。
- 正式发布仍调用 Page Service 并检查 WorkingDocument `base_revision_id` 对应当前
  Revision。CRDT update、snapshot 和 sequence 不进入正式版本历史。
- CRDT 只解决操作层收敛。Base/Current/Proposed 的语义冲突继续使用三方合并和
  `MergeConflict`，不得自动裁决。

## 验证结果

`apps/web/lib/collaboration/yjs-ast.test.ts` 覆盖：

- 全部代表性 AST v1 fixture 无损物化；
- 两个离线副本并发字符编辑后收敛；
- Block 增加、删除、移动及属性修改保持稳定 ID；
- 重复和乱序 binary update 幂等收敛；
- 非法工作副本在物化边界被 AST Schema 拒绝。

## 备选方案

- Automerge：数据模型清晰且自带同步协议，但当前编辑器生态需要新增独立
  ProseMirror/BlockNote 桥接，包体和集成面更大。
- 直接使用 ProseMirror transaction 日志：在线协作成熟，但离线因果合并、状态向量
  和跨服务恢复需要自行补齐。
- 自研 OT/CRDT：无法在 M8 范围内证明正确性和长期维护成本，不采用。

## 影响

- M8-T02 持久化 Yjs update/snapshot 原始字节、hash、sequence 和 codec version。
- M8-T03 WebSocket 协议以 state vector、binary update 和 server sequence 为核心；
  Presence 是易失数据，不写入权威表。
- M8-T04 只能经现有 `BlockEditor`/AST Adapter 边界接入，不允许业务组件直接依赖
  Yjs 或 BlockNote 内部结构。
- Yjs major 升级或 CRDT 根结构变化必须新增 codec version，并提供旧快照恢复测试。
