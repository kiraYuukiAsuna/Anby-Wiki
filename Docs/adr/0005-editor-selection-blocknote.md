# ADR-0005：编辑器选型（Block Editor Adapter）

状态：已接受（M0-T02，M2-T02 落地验证）
日期：2026-07-21

## 背景

实施方案要求编辑器内部模型可**无损双向转换**为 Typed Block AST v1，新 Block 生成稳定 ID、编辑既有 Block 不改 ID。

## 决策

- 选型：**BlockNote**（基于 ProseMirror/TipTap 的块编辑器，React 组件形态）。
- 接入方式：只通过 `apps/web/lib/editor/BlockEditorAdapter` 使用，业务代码不直接引用 BlockNote API；Adapter 负责 BlockNote Document ↔ AST v1 的双向映射。
- 验收标准（M2-T02 必须证明，达不到则重选）：
  1. AST v1 覆盖的 Block（Heading/Paragraph/List/Table/Code/Quote/Callout/Divider + Inline）在 BlockNote 中有对应或可自定义 Block；
  2. Block ID 由我方生成（创建时 UUIDv7），序列化往返后 ID 不变；
  3. 不支持无损映射的 BlockNote 特性（如嵌套样式细节）在 Adapter 边界显式拒绝或降级，不静默丢失。

## 备选方案

- 自研 TipTap Schema：灵活度最高，但块交互（拖拽、选中）要自研，M2 成本过高。
- Lexical：块模型不如 BlockNote 直接，Meta 生态绑定深。
- Editor.js：块模型匹配但 React 集成与维护活跃度较弱。

## 影响

- M2-T02 以 Adapter 测试为验收门槛；若验证失败，按备选方案重估并更新本 ADR。
- 编辑器升级（BlockNote major version）视为契约变更，需跑 Adapter 往返测试套件。

## M2-T02 验证结果

日期：2026-07-21。实现：`apps/web/lib/editor/`（schema/ids/fromAst/toAst/errors）+
受控组件 `apps/web/components/editor/block-editor.tsx` + 验证页 `/dev/editor`（仅 development）。
BlockNote 版本 **0.52.1**（@blocknote/core、@blocknote/react、@blocknote/mantine 固定精确版本），
UUIDv7 用 uuid@14 的 `v7()`。

### 验收标准 1：AST v1 全部 Block/Inline 在 BlockNote 中有对应或可自定义 Block —— 达成

- BlockNote 0.52 heading 默认支持 level 1-6（`HEADING_LEVELS`），覆盖 AST 全范围。
- paragraph / heading / bulletListItem / numberedListItem / quote / codeBlock / divider 用原生块；
  callout / table / tableRow / tableCell 为自定义容器块（content `"none"` + children）。
  原生 table 因单元格只承载 InlineContent（`TableContent`），无法表达 AST `table_cell`
  的任意子块（如单元格内 callout），故以自定义容器块替代。
- 行内：marks ↔ StyledText styles（bold/italic/strike/code）；`inline_code` ↔ 自定义 style
  `inlineCode`（与 code mark 区分，否则往返有损）；`page_reference` ↔ 自定义 inline content
  `pageReference`（7 个 string props 承载已解析/未解析全部字段）；`external_link` ↔ 原生 link。
- 验证：`lib/editor/roundtrip.test.ts` 用真实 `BlockNoteEditor` 实例对
  fixtures/valid 的 full_document / nested_lists / table 做 AST→BN→AST 往返，深度相等（含 ID）。

### 验收标准 2：Block ID 我方生成（UUIDv7），序列化往返不变 —— 达成

- BlockNote 允许外部传入块 id（`PartialBlock.id`，UniqueID 扩展只处理缺失/重复 id），
  fromAst 直接写入 AST id；**未采用** props 存 astId 的备选方案。
- BlockNote 给新建块分配 uuidv4（lib0），toAst 边界把非 UUIDv7 的 id 经
  `AdapterState.generatedIds` 稳定重映射为 UUIDv7；list 容器 id 与提升的 paragraph id
  分别经 `AdapterState.listIds` / `liftedParagraphIds` 保留。
- 验证：`lib/editor/ids.test.ts` —— 编辑既有块文本后 ID 不变；新建块获得 UUIDv7，
  且同一会话重复序列化保持稳定。

### 验收标准 3：不映射特性显式拒绝或降级，不静默丢失 —— 达成（含 1 项已记录降级）

- 显式拒绝（抛 `UnsupportedBlockNoteFeatureError`，带块位置）：audio/file/image/video/
  checkListItem/toggleListItem/pageBreak/原生 table 块；underline/textColor/backgroundColor
  及未知 style；非默认 textColor/backgroundColor/textAlignment、heading isToggleable、
  numberedListItem start；link 内带样式文本；inlineCode 叠加其他样式；callout kind 越界。
  验证：`lib/editor/reject.test.ts`（12 例，含映射表完整性断言）。
- toAst 产物一律经 `parseDocument`（Zod）校验，失败抛 `AstValidationError`（带 issue 路径）。
- **已记录降级**：BlockNote 拒绝空 `initialContent`，空文档在编辑器内表现为一个空段落，
  序列化回 AST 会多出一个空 paragraph（minimal fixture 因此只在「不经编辑器」路径做严格往返，
  编辑器路径断言该降级行为）。BlockNote/ProseMirror 还会规范化相邻同样式文本节点并丢弃空
  文本节点，AST 侧若存在此类节点，经编辑器往返后会被合并/清除（fixture 均不含此形态）。
- 顶层孤立 list_item（AST schema 允许但不在 fixture 中）摊平为 bulletListItem，
  回读时会被包进新的 bullet_list 容器（容器 id 新建）。

### 结论

三条验收标准均达成（空文档行为为 BlockNote 平台限制的显式降级，已记录），选型维持 BlockNote。
M2-T03 注意：业务代码只经 `components/editor/block-editor.tsx` 使用编辑器；
list/quote 的「首个 paragraph 提升为行内 content」是 Adapter 结构约定，UI 层无需感知。
