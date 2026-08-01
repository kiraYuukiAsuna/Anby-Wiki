// BlockNote schema 与 AST v1 映射表（M2-T02 / ADR-0005）。
//
// 映射表（AST v1 block → BlockNote block）：
//   heading       → heading（BlockNote 0.52 heading 默认支持 level 1-6，覆盖 AST 全范围）
//   paragraph     → paragraph
//   bullet_list   → 连续 bulletListItem 序列（容器 ID 经 AdapterState.listIds 保留）
//   ordered_list  → 连续 numberedListItem 序列（同上）
//   list_item     → bulletListItem / numberedListItem（首个 paragraph 子块提升为行内 content）
//   table         → table（自定义容器块，children = tableRow）
//   table_row     → tableRow（自定义容器块，children = tableCell）
//   table_cell    → tableCell（自定义容器块，children = 任意块）
//   code          → codeBlock（plain content ↔ content 字符串，language prop ↔ language）
//   quote         → quote（首个 paragraph 子块提升为行内 content，其余子块保持嵌套）
//   callout       → callout（自定义容器块，kind prop）
//   component     → component（自定义原子块，稳定引用与 display_config 存 props）
//   divider       → divider
//
// 行内映射：
//   text.marks            → StyledText styles（bold/italic/strike/code）
//   inline_code           → 自定义 style "inlineCode"（与 code mark 区分，保证往返无损）
//   page_reference        → 自定义 inline content "pageReference"（全部字段存 props）
//   external_link         → link（href 承载 url，纯文本 content 承载 display_text）
//   entity_reference      → 自定义 inline content "entityReference"
//   claim_reference       → 自定义 inline content "claimReference"
//   citation_reference    → 自定义 inline content "citationReference"
//
// 显式不映射（toAst 遇到即抛 UnsupportedBlockNoteFeatureError，不静默丢弃）：
//   - BlockNote 块类型：audio / file / image / video / checkListItem /
//     toggleListItem / pageBreak（不进入 editorSchema，外来文档出现即拒绝）；
//   - BlockNote 原生 table 块（content model 为 InlineContent 单元格，无法承载
//     AST table_cell 的任意子块，故用自定义容器块替代，原生 table 同样被拒绝）；
//   - 文本样式：underline / textColor / backgroundColor 及任何未知 style；
//   - 块 props：textColor / backgroundColor（非 "default"）、textAlignment（非 "left"）、
//     heading 的 isToggleable、numberedListItem 的 start；
//   - link 内带样式的文本（external_link.display_text 是纯文本，无法承载样式）。
import {
  BlockNoteSchema,
  createBlockSpec,
  createInlineContentSpec,
  createStyleSpec,
  defaultBlockSpecs,
  defaultInlineContentSpecs,
  defaultStyleSpecs,
} from "@blocknote/core";

import type { Block as AstBlock } from "@/lib/ast/schema";

/** AST v1 全部 block 类型（判别值）。 */
export const AST_BLOCK_TYPES = [
  "heading",
  "paragraph",
  "bullet_list",
  "ordered_list",
  "list_item",
  "table",
  "table_row",
  "table_cell",
  "code",
  "quote",
  "callout",
  "component",
  "image",
  "video",
  "dataset_view",
  "embed",
  "divider",
] as const satisfies readonly AstBlock["type"][];

export type AstBlockType = (typeof AST_BLOCK_TYPES)[number];

/** AST block 类型 → BlockNote 承载方式。 */
export const AST_TO_BLOCKNOTE_BLOCK: Record<AstBlockType, string> = {
  heading: "heading",
  paragraph: "paragraph",
  bullet_list: "bulletListItem (连续分组)",
  ordered_list: "numberedListItem (连续分组)",
  list_item: "bulletListItem | numberedListItem",
  table: "table (自定义容器)",
  table_row: "tableRow (自定义容器)",
  table_cell: "tableCell (自定义容器)",
  code: "codeBlock",
  quote: "quote",
  callout: "callout (自定义容器)",
  component: "component (自定义原子块)",
  image: "imageAsset (自定义原子块)",
  video: "videoAsset (自定义原子块)",
  dataset_view: "datasetView (自定义原子块)",
  embed: "embed (自定义原子块)",
  divider: "divider",
};

/** 显式不映射的 BlockNote 默认块类型（不进入 editorSchema）。 */
export const UNMAPPED_BLOCKNOTE_BLOCKS = [
  "audio",
  "file",
  "checkListItem",
  "toggleListItem",
  "pageBreak",
] as const;

/** 显式不映射的 BlockNote 文本样式（toAst 遇到即拒绝）。 */
export const UNMAPPED_BLOCKNOTE_STYLES = [
  "underline",
  "textColor",
  "backgroundColor",
] as const;

// ---- 自定义块：callout / table / tableRow / tableCell（content "none" 容器） ----

const CALLOUT_LABELS: Record<string, string> = {
  info: "信息",
  warning: "警告",
  danger: "危险",
};

const calloutSpec = createBlockSpec(
  {
    type: "callout",
    propSchema: {
      kind: {
        default: "info",
        values: ["info", "warning", "danger"],
      },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = `ast-callout ast-callout--${block.props.kind}`;
      dom.textContent = CALLOUT_LABELS[block.props.kind] ?? block.props.kind;
      return { dom };
    },
  },
);

const componentSpec = createBlockSpec(
  {
    type: "component",
    propSchema: {
      componentId: { default: "" },
      componentVersion: { default: 1 },
      entityId: { default: "" },
      displayConfig: { default: "{}" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-component";
      dom.dataset.componentId = block.props.componentId;
      dom.dataset.entityId = block.props.entityId;
      dom.textContent = `组件 v${block.props.componentVersion}`;
      return { dom };
    },
  },
);

const imageAssetSpec = createBlockSpec(
  {
    type: "imageAsset",
    propSchema: {
      assetRevisionId: { default: "" },
      altText: { default: "" },
      caption: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-image";
      dom.dataset.assetRevisionId = block.props.assetRevisionId;
      dom.textContent = block.props.caption || block.props.altText || "图片";
      return { dom };
    },
  },
);

const videoAssetSpec = createBlockSpec(
  {
    type: "videoAsset",
    propSchema: {
      assetRevisionId: { default: "" },
      posterAssetRevisionId: { default: "" },
      title: { default: "" },
      caption: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-video";
      dom.dataset.assetRevisionId = block.props.assetRevisionId;
      dom.textContent = block.props.caption || block.props.title || "视频";
      return { dom };
    },
  },
);

const datasetViewSpec = createBlockSpec(
  {
    type: "datasetView",
    propSchema: { datasetViewId: { default: "" } },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-dataset-view";
      dom.dataset.datasetViewId = block.props.datasetViewId;
      dom.textContent = "可查询数据视图";
      return { dom };
    },
  },
);

const embedSpec = createBlockSpec(
  {
    type: "embed",
    propSchema: {
      url: { default: "" },
      title: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-embed";
      dom.textContent = block.props.title || block.props.url || "外部嵌入";
      return { dom };
    },
  },
);

/** 生成 content "none" 的容器块 spec（子块由 BlockNote 自动渲染在块下方）。 */
function containerBlockSpec<T extends string>(type: T, className: string) {
  return createBlockSpec(
    { type, propSchema: {}, content: "none" },
    {
      render: () => {
        const dom = window.document.createElement("div");
        dom.className = className;
        return { dom };
      },
    },
  );
}

// ---- 自定义行内内容：pageReference（原子节点，全部字段存 props） ----

const pageReferenceSpec = createInlineContentSpec(
  {
    type: "pageReference",
    content: "none",
    propSchema: {
      targetPageId: { default: "" },
      targetHeadingBlockId: { default: "" },
      displayText: { default: "" },
      resolutionStatus: { default: "" },
      targetNamespace: { default: "" },
      normalizedTitle: { default: "" },
      expectedEntityType: { default: "" },
    },
  },
  {
    render: (inlineContent) => {
      const props = inlineContent.props;
      const unresolved = props.resolutionStatus === "unresolved";
      const dom = window.document.createElement("span");
      dom.className = unresolved
        ? "ast-page-ref ast-page-ref--unresolved"
        : "ast-page-ref";
      dom.textContent = unresolved ? props.normalizedTitle : props.displayText;
      return { dom };
    },
  },
);

const pageAnchorReferenceSpec = createInlineContentSpec(
  {
    type: "pageAnchorReference",
    content: "none",
    propSchema: {
      targetPageId: { default: "" },
      targetHeadingBlockId: { default: "" },
      displayText: { default: "" },
    },
  },
  {
    render: (inlineContent) => {
      const dom = window.document.createElement("span");
      dom.className = "ast-page-anchor-ref";
      dom.textContent = inlineContent.props.displayText;
      return { dom };
    },
  },
);

const entityReferenceSpec = createInlineContentSpec(
  {
    type: "entityReference",
    content: "none",
    propSchema: {
      entityId: { default: "" },
      displayText: { default: "" },
    },
  },
  {
    render: (inlineContent) => {
      const dom = window.document.createElement("span");
      dom.className = "ast-entity-ref";
      dom.dataset.entityRef = inlineContent.props.entityId;
      dom.textContent = inlineContent.props.displayText;
      return { dom };
    },
  },
);

const claimReferenceSpec = createInlineContentSpec(
  {
    type: "claimReference",
    content: "none",
    propSchema: {
      claimId: { default: "" },
      displayText: { default: "" },
    },
  },
  {
    render: (inlineContent) => {
      const dom = window.document.createElement("span");
      dom.className = "ast-claim-ref";
      dom.dataset.claimRef = inlineContent.props.claimId;
      dom.textContent = inlineContent.props.displayText;
      return { dom };
    },
  },
);

const citationReferenceSpec = createInlineContentSpec(
  {
    type: "citationReference",
    content: "none",
    propSchema: {
      citationId: { default: "" },
      displayText: { default: "" },
    },
  },
  {
    render: (inlineContent) => {
      const dom = window.document.createElement("sup");
      dom.className = "ast-citation-ref";
      dom.dataset.citationRef = inlineContent.props.citationId;
      dom.title = inlineContent.props.displayText;
      dom.textContent = "[引用]";
      return { dom };
    },
  },
);

const mathSpec = createInlineContentSpec(
  {
    type: "math",
    content: "none",
    propSchema: { expression: { default: "" } },
  },
  {
    render: (inlineContent) => {
      const dom = window.document.createElement("code");
      dom.className = "ast-math";
      dom.textContent = inlineContent.props.expression;
      return { dom };
    },
  },
);

const mentionSpec = createInlineContentSpec(
  {
    type: "mention",
    content: "none",
    propSchema: {
      actorId: { default: "" },
      displayText: { default: "" },
    },
  },
  {
    render: (inlineContent) => {
      const dom = window.document.createElement("span");
      dom.className = "ast-mention";
      dom.dataset.actorId = inlineContent.props.actorId;
      dom.textContent = `@${inlineContent.props.displayText}`;
      return { dom };
    },
  },
);

// ---- 自定义样式：inlineCode（与 code mark 区分，AST inline_code 专用） ----

const inlineCodeStyleSpec = createStyleSpec(
  { type: "inlineCode", propSchema: "boolean" },
  {
    render: () => {
      const dom = window.document.createElement("code");
      dom.className = "ast-inline-code";
      return { dom, contentDOM: dom };
    },
  },
);

/**
 * 编辑器 schema：只包含可无损映射到 AST v1 的块/行内/样式。
 * 原生 table、checkListItem 等不映射特性一律不进入 schema。
 */
export const editorSchema = BlockNoteSchema.create({
  blockSpecs: {
    paragraph: defaultBlockSpecs.paragraph,
    heading: defaultBlockSpecs.heading,
    bulletListItem: defaultBlockSpecs.bulletListItem,
    numberedListItem: defaultBlockSpecs.numberedListItem,
    quote: defaultBlockSpecs.quote,
    codeBlock: defaultBlockSpecs.codeBlock,
    divider: defaultBlockSpecs.divider,
    callout: calloutSpec(),
    component: componentSpec(),
    imageAsset: imageAssetSpec(),
    videoAsset: videoAssetSpec(),
    datasetView: datasetViewSpec(),
    embed: embedSpec(),
    table: containerBlockSpec("table", "ast-table")(),
    tableRow: containerBlockSpec("tableRow", "ast-table-row")(),
    tableCell: containerBlockSpec("tableCell", "ast-table-cell")(),
  },
  inlineContentSpecs: {
    ...defaultInlineContentSpecs,
    pageReference: pageReferenceSpec,
    pageAnchorReference: pageAnchorReferenceSpec,
    entityReference: entityReferenceSpec,
    claimReference: claimReferenceSpec,
    citationReference: citationReferenceSpec,
    math: mathSpec,
    mention: mentionSpec,
  },
  styleSpecs: {
    ...defaultStyleSpecs,
    inlineCode: inlineCodeStyleSpec,
  },
});

export type EditorSchema = typeof editorSchema;

/**
 * BlockNote 块的最小结构视图。Adapter 面向运行时数据工作（逐字段校验 +
 * 显式拒绝未知类型），不强依赖 BlockNote 的泛型类型推导。
 */
export interface BNBlock {
  id: string;
  type: string;
  props: Record<string, unknown>;
  content: unknown;
  children: BNBlock[];
}

/** fromAst 产出的 PartialBlock 最小结构（编辑器接受 id 外部传入）。 */
export interface BNPartialBlock {
  id?: string;
  type?: string;
  props?: Record<string, unknown>;
  content?: unknown;
  children?: BNPartialBlock[];
}
