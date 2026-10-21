// BlockNote schema 与 AST v1 映射表（M2-T02 / ADR-0005）。
//
// 映射表（AST v1 block → BlockNote block）：
//   heading       → heading（BlockNote 0.52 heading 默认支持 level 1-6，覆盖 AST 全范围）
//   paragraph     → paragraph
//   bullet_list   → 连续 bulletListItem 序列（容器 ID 经 AdapterState.listIds 保留）
//   ordered_list  → 连续 numberedListItem 序列（同上）
//   list_item     → bulletListItem / numberedListItem（首个 paragraph 子块提升为行内 content）
//   table         → astTable（自定义容器块，children = astTableRow）
//   table_row     → astTableRow（自定义容器块，children = astTableCell）
//   table_cell    → astTableCell（自定义容器块，children = 任意块）
//   code          → codeBlock（plain content ↔ content 字符串，language prop ↔ language）
//   preformatted  → preformatted（固定宽度原子块，text/kind/caption 存 props）
//   equation      → equation（自定义原子块，LaTeX expression）
//   todo          → checkListItem（checked + inline content + children）
//   toggle        → toggleListItem（inline summary + children）
//   breadcrumb    → breadcrumb（页面路径 JSON 存 props）
//   column_list   → columnList（自定义容器，children = column）
//   column        → column（自定义容器，widthRatio prop）
//   button        → pageButton（声明式动作 JSON 存 props，编辑器内不执行）
//   html          → htmlBlock（sandbox iframe，不允许脚本/网络/表单）
//   audio/file/pdf → AssetRevision 原子块
//   bookmark/link_preview → 冻结的外链预览原子块
//   quote         → quote（首个 paragraph 子块提升为行内 content，其余子块保持嵌套）
//   callout       → callout（自定义容器块，kind prop）
//   component     → component（自定义原子块，稳定引用与 display_config 存 props）
//   divider       → divider
//
// 行内映射：
//   text                  → StyledText styles（marks + text/background color）
//   inline_code           → 自定义 style "inlineCode"（与 code mark 区分，保证往返无损）
//   page_reference        → 自定义 inline content "pageReference"（全部字段存 props）
//   external_link         → link（href 承载 url，纯文本 content 承载 display_text）
//   entity_reference      → 自定义 inline content "entityReference"
//   claim_reference       → 自定义 inline content "claimReference"
//   citation_reference    → 自定义 inline content "citationReference"
//   mention/date_mention/database_mention → 对应自定义原子 inline content
//
// 显式不映射（toAst 遇到即抛 UnsupportedBlockNoteFeatureError，不静默丢弃）：
//   - BlockNote 块类型：audio / file / image / video / pageBreak
//     （不进入 editorSchema，外来文档出现即拒绝）；
//   - BlockNote 原生 table 块（content model 为 InlineContent 单元格，无法承载
//     AST table_cell 的任意子块，故用自定义容器块替代，原生 table 同样被拒绝）；
//   - 文本样式：白名单调色板之外的 textColor / backgroundColor 及任何未知 style；
//   - 块 props：textColor / backgroundColor（非 "default"）、textAlignment（非 "left"）、
//     numberedListItem 的 start；
//   - link 内带样式的文本（external_link.display_text 是纯文本，无法承载样式）。
import {
  BlockNoteSchema,
  createBlockSpec,
  createHeadingBlockSpec,
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
  "preformatted",
  "equation",
  "todo",
  "toggle",
  "breadcrumb",
  "column_list",
  "column",
  "button",
  "html",
  "quote",
  "callout",
  "component",
  "image",
  "video",
  "audio",
  "file",
  "pdf",
  "bookmark",
  "link_preview",
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
  table: "astTable (自定义容器)",
  table_row: "astTableRow (自定义容器)",
  table_cell: "astTableCell (自定义容器)",
  code: "codeBlock",
  preformatted: "preformatted",
  equation: "equation (自定义原子块)",
  todo: "checkListItem",
  toggle: "toggleListItem",
  breadcrumb: "breadcrumb (自定义原子块)",
  column_list: "columnList (自定义容器)",
  column: "column (自定义容器)",
  button: "pageButton (自定义原子块)",
  html: "htmlBlock (自定义原子块)",
  quote: "quote",
  callout: "callout (自定义容器)",
  component: "component (自定义原子块)",
  image: "imageAsset (自定义原子块)",
  video: "videoAsset (自定义原子块)",
  audio: "audioAsset (自定义原子块)",
  file: "fileAsset (自定义原子块)",
  pdf: "pdfAsset (自定义原子块)",
  bookmark: "bookmark (自定义原子块)",
  link_preview: "linkPreview (自定义原子块)",
  dataset_view: "datasetView (自定义原子块)",
  embed: "embed (自定义原子块)",
  divider: "divider",
};

/** 显式不映射的 BlockNote 默认块类型（不进入 editorSchema）。 */
export const UNMAPPED_BLOCKNOTE_BLOCKS = [
  "audio",
  "file",
  "table",
  "pageBreak",
] as const;

// ---- 自定义块：callout / astTable*（content "none" 容器） ----

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
      icon: { default: "" },
      color: {
        default: "default",
        values: [
          "default",
          "gray",
          "brown",
          "orange",
          "yellow",
          "green",
          "blue",
          "purple",
          "pink",
          "red",
        ],
      },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = `ast-callout ast-callout--${block.props.kind}`;
      dom.dataset.color = block.props.color;
      dom.textContent = `${block.props.icon || ""}${CALLOUT_LABELS[block.props.kind] ?? block.props.kind}`;
      return { dom };
    },
  },
);

const preformattedSpec = createBlockSpec(
  {
    type: "preformatted",
    propSchema: {
      kind: {
        default: "plain",
        values: [
          "ascii_diagram",
          "fixed_table",
          "protocol_trace",
          "formula_text",
          "plain",
        ],
      },
      caption: { default: "" },
      text: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const figure = window.document.createElement("figure");
      figure.className = "ast-preformatted";
      figure.dataset.kind = block.props.kind;
      if (block.props.caption) {
        const caption = window.document.createElement("figcaption");
        caption.textContent = block.props.caption;
        figure.appendChild(caption);
      }
      const pre = window.document.createElement("pre");
      const code = window.document.createElement("code");
      code.textContent = block.props.text;
      pre.appendChild(code);
      figure.appendChild(pre);
      return { dom: figure };
    },
  },
);

const equationSpec = createBlockSpec(
  {
    type: "equation",
    propSchema: {
      expression: { default: "" },
      caption: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-equation";
      dom.dataset.mathExpression = block.props.expression;
      dom.textContent = block.props.caption
        ? `${block.props.expression} · ${block.props.caption}`
        : block.props.expression;
      return { dom };
    },
  },
);

const tableSpec = createBlockSpec(
  {
    type: "astTable",
    propSchema: {
      caption: { default: "" },
      hasColumnHeader: { default: false },
      hasRowHeader: { default: false },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-table";
      dom.dataset.hasColumnHeader = String(block.props.hasColumnHeader);
      dom.dataset.hasRowHeader = String(block.props.hasRowHeader);
      dom.textContent = block.props.caption;
      return { dom };
    },
  },
);

const breadcrumbSpec = createBlockSpec(
  {
    type: "breadcrumb",
    propSchema: { itemsJson: { default: "[]" } },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-breadcrumb";
      try {
        const items = JSON.parse(block.props.itemsJson) as {
          display_text?: string;
        }[];
        dom.textContent = items
          .map((item) => String(item.display_text ?? ""))
          .filter(Boolean)
          .join(" / ");
      } catch {
        dom.textContent = "页面路径";
      }
      return { dom };
    },
  },
);

const columnSpec = createBlockSpec(
  {
    type: "column",
    propSchema: { widthRatio: { default: 1 } },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-column";
      dom.textContent = `列宽 ${block.props.widthRatio}`;
      return { dom };
    },
  },
);

const pageButtonSpec = createBlockSpec(
  {
    type: "pageButton",
    propSchema: {
      label: { default: "运行操作" },
      confirmation: { default: "" },
      actionsJson: { default: "[]" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("button");
      dom.className = "wiki-button";
      dom.type = "button";
      dom.disabled = true;
      dom.textContent = block.props.label;
      return { dom };
    },
  },
);

function isolatedHTMLDocument(content: string): string {
  const policy =
    "default-src 'none'; img-src data:; font-src data:; style-src 'unsafe-inline'; " +
    "form-action 'none'; base-uri 'none'";
  return (
    '<!doctype html><html><head><meta charset="utf-8">' +
    `<meta http-equiv="Content-Security-Policy" content="${policy}">` +
    '<meta name="viewport" content="width=device-width,initial-scale=1">' +
    "<style>html{color-scheme:light}body{margin:0;padding:12px;font:14px/1.5 system-ui,sans-serif;" +
    "overflow-wrap:anywhere}img,video,canvas,svg{max-width:100%;height:auto}</style>" +
    `</head><body>${content}</body></html>`
  );
}

const htmlBlockSpec = createBlockSpec(
  {
    type: "htmlBlock",
    propSchema: {
      html: { default: "" },
      height: { default: 240 },
      caption: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const figure = window.document.createElement("figure");
      figure.className = "html-block";
      const iframe = window.document.createElement("iframe");
      iframe.setAttribute("sandbox", "");
      iframe.setAttribute("referrerpolicy", "no-referrer");
      iframe.loading = "lazy";
      iframe.height = String(block.props.height);
      iframe.title = block.props.caption || "HTML content";
      iframe.srcdoc = isolatedHTMLDocument(block.props.html);
      figure.appendChild(iframe);
      if (block.props.caption) {
        const caption = window.document.createElement("figcaption");
        caption.textContent = block.props.caption;
        figure.appendChild(caption);
      }
      return { dom: figure };
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

const audioAssetSpec = createBlockSpec(
  {
    type: "audioAsset",
    propSchema: {
      assetRevisionId: { default: "" },
      title: { default: "" },
      artist: { default: "" },
      album: { default: "" },
      durationSeconds: { default: 0 },
      caption: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-audio";
      dom.dataset.assetRevisionId = block.props.assetRevisionId;
      dom.textContent = [block.props.title, block.props.artist, block.props.album]
        .filter(Boolean)
        .join(" · ");
      return { dom };
    },
  },
);

const fileAssetSpec = createBlockSpec(
  {
    type: "fileAsset",
    propSchema: {
      assetRevisionId: { default: "" },
      name: { default: "" },
      caption: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-file";
      dom.dataset.assetRevisionId = block.props.assetRevisionId;
      dom.textContent = block.props.name || "文件";
      return { dom };
    },
  },
);

const pdfAssetSpec = createBlockSpec(
  {
    type: "pdfAsset",
    propSchema: {
      assetRevisionId: { default: "" },
      title: { default: "" },
      startPage: { default: 1 },
      height: { default: 640 },
      caption: { default: "" },
    },
    content: "none",
  },
  {
    render: (block) => {
      const dom = window.document.createElement("div");
      dom.className = "ast-pdf";
      dom.dataset.assetRevisionId = block.props.assetRevisionId;
      dom.textContent = block.props.title || "PDF";
      return { dom };
    },
  },
);

function externalPreviewSpec(
  type: "bookmark" | "linkPreview",
  className: string,
) {
  return createBlockSpec(
    {
      type,
      propSchema: {
        url: { default: "" },
        title: { default: "" },
        description: { default: "" },
        siteName: { default: "" },
        caption: { default: "" },
      },
      content: "none",
    },
    {
      render: (block) => {
        const dom = window.document.createElement("div");
        dom.className = className;
        dom.textContent = `${block.props.siteName || "Link"} · ${block.props.title || block.props.url}`;
        return { dom };
      },
    },
  );
}

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
      provider: {
        default: "none",
        values: ["none", "youtube", "vimeo", "figma"],
      },
      caption: { default: "" },
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

const dateMentionSpec = createInlineContentSpec(
  {
    type: "dateMention",
    content: "none",
    propSchema: {
      date: { default: "" },
      endDate: { default: "" },
      displayText: { default: "" },
    },
  },
  {
    render: (inlineContent) => {
      const dom = window.document.createElement("time");
      dom.className = "ast-date-mention";
      dom.dateTime = inlineContent.props.date;
      dom.dataset.endDate = inlineContent.props.endDate;
      dom.textContent = inlineContent.props.displayText;
      return { dom };
    },
  },
);

const databaseMentionSpec = createInlineContentSpec(
  {
    type: "databaseMention",
    content: "none",
    propSchema: {
      datasetId: { default: "" },
      displayText: { default: "" },
    },
  },
  {
    render: (inlineContent) => {
      const dom = window.document.createElement("span");
      dom.className = "ast-database-mention";
      dom.dataset.datasetId = inlineContent.props.datasetId;
      dom.textContent = inlineContent.props.displayText;
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
    heading: createHeadingBlockSpec({ allowToggleHeadings: true }),
    bulletListItem: defaultBlockSpecs.bulletListItem,
    numberedListItem: defaultBlockSpecs.numberedListItem,
    checkListItem: defaultBlockSpecs.checkListItem,
    toggleListItem: defaultBlockSpecs.toggleListItem,
    quote: defaultBlockSpecs.quote,
    codeBlock: defaultBlockSpecs.codeBlock,
    preformatted: preformattedSpec(),
    equation: equationSpec(),
    breadcrumb: breadcrumbSpec(),
    columnList: containerBlockSpec("columnList", "ast-column-list")(),
    column: columnSpec(),
    pageButton: pageButtonSpec(),
    htmlBlock: htmlBlockSpec(),
    divider: defaultBlockSpecs.divider,
    callout: calloutSpec(),
    component: componentSpec(),
    imageAsset: imageAssetSpec(),
    videoAsset: videoAssetSpec(),
    audioAsset: audioAssetSpec(),
    fileAsset: fileAssetSpec(),
    pdfAsset: pdfAssetSpec(),
    bookmark: externalPreviewSpec("bookmark", "ast-bookmark")(),
    linkPreview: externalPreviewSpec("linkPreview", "ast-link-preview")(),
    datasetView: datasetViewSpec(),
    embed: embedSpec(),
    // Do not use BlockNote's reserved `table` type here. Its React extensions
    // would treat this AST container as a native table and crash on selection.
    astTable: tableSpec(),
    astTableRow: containerBlockSpec("astTableRow", "ast-table-row")(),
    astTableCell: containerBlockSpec("astTableCell", "ast-table-cell")(),
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
    dateMention: dateMentionSpec,
    databaseMention: databaseMentionSpec,
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
