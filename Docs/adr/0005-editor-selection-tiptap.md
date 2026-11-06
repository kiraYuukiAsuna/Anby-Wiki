# ADR-0005：编辑器选型（Tiptap + Block Editor Adapter）

状态：已接受（M0-T02；2026-11-06 实现修订）
日期：2026-07-21
修订：2026-11-06

## 背景

实施方案要求编辑器内部模型可**无损双向转换**为 Typed Block AST v1，新 Block 生成稳定 ID、编辑既有 Block 不改 ID。编辑器还必须支持 Anby 自定义块和行内原子节点的原地编辑，避免把业务字段封装成不可编辑的静态控件。

## 决策

- 选型：**Tiptap 3 + ProseMirror Schema + React NodeView**。
- 接入方式：业务代码只通过 `apps/web/components/editor/block-editor.tsx` 使用编辑器；`apps/web/lib/editor/` 负责 Tiptap JSON Document ↔ AST v1 的双向映射。
- 验收标准：
  1. AST v1 覆盖的 Block 和 Inline 在 Tiptap Schema 中有对应节点、mark 或可编辑 NodeView；
  2. Block ID 由我方生成 UUIDv7，序列化往返后 ID 不变；
  3. 不支持无损映射的编辑器特性在 Adapter 边界显式拒绝或降级，不静默丢失。

## 备选方案

- 预置块编辑器组件：基础交互成本低，但自定义块直接编辑和 AST 结构控制不足。
- Lexical：块模型需要额外约束，生态与项目现有 ProseMirror/Tiptap 依赖不一致。
- Editor.js：块模型匹配但 React 集成与维护活跃度较弱。

## 影响

- 编辑器升级或 Schema 变化视为契约变更，需人工核对 AST 往返语义并完成类型检查、Lint、构建和 `/dev/editor` 冒烟。
- 业务组件不得直接依赖 Tiptap 节点结构；跨边界只传 Typed Block AST v1。
- 阅读态、导出、AI、Diff、Proposal 和 Renderer 不理解编辑器私有结构。

## 当前实现验证

日期：2026-11-06。实现：`apps/web/lib/editor/`（schema/ids/fromAst/toAst/errors）+ 受控组件 `apps/web/components/editor/block-editor.tsx` + 验证页 `/dev/editor`（仅 development）。Tiptap 依赖固定为 3.31.3，UUIDv7 使用 uuid@14 的 `v7()`。

### 验收标准 1：AST v1 Block/Inline 均有映射 —— 达成

- paragraph / heading / bulletList / orderedList / listItem / blockquote / codeBlock / horizontalRule 使用 Tiptap 原生或基础节点。
- callout、todo、toggle、preformatted、equation、breadcrumb、column、pageButton、HTML、component、媒体、外部内容和 AST table 使用自定义 NodeView。
- Page/Page Anchor、Entity、Claim、Citation、Math、Mention 与 Date Mention 使用行内原子节点；普通 link、underline、strike、code、受限颜色等使用 mark。
- AST table 的 row/cell 是容器节点，单元格可以继续承载任意 block 子树。

### 验收标准 2：Block ID 我方生成且往返稳定 —— 达成

- `fromAst` 直接把 AST Block ID 写入 Tiptap 节点属性。
- `BlockIds` 扩展为缺少 ID 的节点补 UUIDv7。
- `toAst` 对尚未写回的临时节点 ID 使用 `AdapterState.generatedIds` 稳定映射为 UUIDv7。
- 列表和容器节点都直接持有 AST Block ID，不再需要列表容器或提升段落 sidecar 映射。

### 验收标准 3：不映射特性显式拒绝或降级，不静默丢失 —— 达成

- Tiptap Schema 不暴露不能进入 AST v1 的节点类型。
- `toAst` 对未知行内节点、未知 mark、越界颜色和非法 JSON 属性抛出明确错误。
- `toAst` 产物一律经 `parseDocument`（Zod）校验，失败抛 `AstValidationError`（带 issue 路径）。
- 空文档在编辑器内表现为一个空段落；相邻同样式文本节点和空文本节点可能被 ProseMirror 规范化，这是编辑器层已记录降级。

### 结论

三条验收标准均达成，选型维持 Tiptap + Block Editor Adapter。业务代码只经 `components/editor/block-editor.tsx` 使用编辑器；Adapter 结构约定不泄漏到 UI 层。
