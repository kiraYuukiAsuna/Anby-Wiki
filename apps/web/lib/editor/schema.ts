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

import {
  breadcrumbItemSchema,
  buttonActionSchema,
  type Block as AstBlock,
} from "@/lib/ast/schema";
import {
  booleanProp,
  buttonControl,
  checkboxField,
  createActionRow,
  createEditableBlockShell,
  createFieldGrid,
  createPreview,
  ignoreEditableBlockMutation,
  isPlainObject,
  jsonTextareaField,
  nonEmpty,
  numberField,
  numberProp,
  optionalUUID,
  patchBlockProps,
  requiredHTTPURL,
  requiredUUID,
  selectField,
  stringProp,
  textField,
  type EditableBlockEditor,
} from "@/lib/editor/editable-block-controls";

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

// ---- 自定义块：直接编辑 props，子块仍交给 BlockNote 渲染 ----

const CALLOUT_LABELS: Record<string, string> = {
  info: "信息",
  warning: "警告",
  danger: "危险",
};

const CALLOUT_KIND_OPTIONS = [
  { value: "info", label: "信息" },
  { value: "warning", label: "警告" },
  { value: "danger", label: "危险" },
];

const COLOR_OPTIONS = [
  { value: "default", label: "默认" },
  { value: "gray", label: "灰" },
  { value: "brown", label: "棕" },
  { value: "orange", label: "橙" },
  { value: "yellow", label: "黄" },
  { value: "green", label: "绿" },
  { value: "blue", label: "蓝" },
  { value: "purple", label: "紫" },
  { value: "pink", label: "粉" },
  { value: "red", label: "红" },
];

const PREFORMATTED_KIND_OPTIONS = [
  { value: "ascii_diagram", label: "ASCII 图" },
  { value: "fixed_table", label: "固定宽表格" },
  { value: "protocol_trace", label: "协议 Trace" },
  { value: "formula_text", label: "公式文本" },
  { value: "plain", label: "纯文本" },
];

const EMBED_PROVIDER_OPTIONS = [
  { value: "none", label: "普通链接" },
  { value: "youtube", label: "YouTube" },
  { value: "vimeo", label: "Vimeo" },
  { value: "figma", label: "Figma" },
];

const breadcrumbItemsSchema = breadcrumbItemSchema.array().min(1).max(32);
const buttonActionsSchema = buttonActionSchema.array().min(1).max(10);

function isEditableBlockRender(
  context: unknown,
  editor: EditableBlockEditor,
): boolean {
  return (
    isPlainObject(context) &&
    context.renderType === "nodeView" &&
    editor.isEditable !== false &&
    typeof editor.updateBlock === "function"
  );
}

type PartialEditorBlock = {
  id?: string;
  type?: string;
  props?: Record<string, unknown>;
  content?: unknown;
  children?: PartialEditorBlock[];
};

function newEditorBlockId(): string {
  const crypto = globalThis.crypto;
  if (typeof crypto?.randomUUID === "function") return crypto.randomUUID();
  return `local-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function emptyParagraph(text = ""): PartialEditorBlock {
  return { id: newEditorBlockId(), type: "paragraph", content: text };
}

function emptyTableCell(text = ""): PartialEditorBlock {
  return {
    id: newEditorBlockId(),
    type: "astTableCell",
    children: [emptyParagraph(text)],
  };
}

function patchBlockChildren(
  editor: EditableBlockEditor,
  blockId: string,
  children: PartialEditorBlock[],
): void {
  if (editor.isEditable === false || !editor.updateBlock) return;
  editor.updateBlock(blockId, { children });
}

function childBlocks(block: { children?: unknown[] }): PartialEditorBlock[] {
  return Array.isArray(block.children)
    ? (block.children as PartialEditorBlock[])
    : [];
}

function addTableRow(block: { children?: unknown[] }): PartialEditorBlock[] {
  const rows = childBlocks(block);
  const firstRow = rows[0];
  const columnCount = Math.max(childBlocks(firstRow ?? {}).length, 1);
  return [
    ...rows,
    {
      id: newEditorBlockId(),
      type: "astTableRow",
      children: Array.from({ length: columnCount }, () => emptyTableCell("单元格")),
    },
  ];
}

function addTableColumn(block: { children?: unknown[] }): PartialEditorBlock[] {
  const rows = childBlocks(block);
  if (rows.length === 0) {
    return [
      {
        id: newEditorBlockId(),
        type: "astTableRow",
        children: [emptyTableCell("单元格")],
      },
    ];
  }
  return rows.map((row) => ({
    ...row,
    children: [...childBlocks(row), emptyTableCell("单元格")],
  }));
}

function removeLastTableRow(block: { children?: unknown[] }): PartialEditorBlock[] {
  const rows = childBlocks(block);
  return rows.length > 1 ? rows.slice(0, -1) : rows;
}

function removeLastTableColumn(block: { children?: unknown[] }): PartialEditorBlock[] {
  return childBlocks(block).map((row) => {
    const cells = childBlocks(row);
    return { ...row, children: cells.length > 1 ? cells.slice(0, -1) : cells };
  });
}

function findParentBlock(
  blocks: unknown[] | undefined,
  childId: string,
  parentType: string,
): PartialEditorBlock | null {
  if (!Array.isArray(blocks)) return null;
  for (const block of blocks as PartialEditorBlock[]) {
    const children = childBlocks(block);
    if (block.type === parentType && children.some((child) => child.id === childId)) {
      return block;
    }
    const nested = findParentBlock(children, childId, parentType);
    if (nested) return nested;
  }
  return null;
}

function addColumnSibling(editor: EditableBlockEditor, columnId: string): void {
  const parent = findParentBlock(editor.document, columnId, "columnList");
  if (!parent?.id) return;
  const columns = childBlocks(parent);
  if (columns.length >= 5) return;
  patchBlockChildren(editor, parent.id, [
    ...columns,
    {
      id: newEditorBlockId(),
      type: "column",
      props: { widthRatio: 1 },
      children: [emptyParagraph("新列")],
    },
  ]);
}

function removeColumn(editor: EditableBlockEditor, columnId: string): void {
  const parent = findParentBlock(editor.document, columnId, "columnList");
  if (!parent?.id) return;
  const columns = childBlocks(parent);
  if (columns.length <= 2) return;
  patchBlockChildren(
    editor,
    parent.id,
    columns.filter((column) => column.id !== columnId),
  );
}

function parsedJSONArray(value: string, fallback: Record<string, unknown>[]) {
  try {
    const parsed = JSON.parse(value) as unknown;
    return Array.isArray(parsed) && parsed.every(isPlainObject)
      ? (parsed as Record<string, unknown>[])
      : fallback;
  } catch {
    return fallback;
  }
}

function writeJSONProps(
  editor: EditableBlockEditor,
  blockId: string,
  propName: string,
  value: unknown,
): void {
  patchBlockProps(editor, blockId, { [propName]: JSON.stringify(value) });
}

const DEFAULT_INSERT_BLOCK_ACTION = {
  type: "insert_blocks",
  target_page_id: "00000000-0000-7000-8000-000000000000",
  blocks: [
    {
      id: "00000000-0000-7000-8000-000000000001",
      type: "paragraph",
      content: [{ type: "text", text: "插入内容" }],
    },
  ],
};

const BUTTON_ACTION_OPTIONS = [
  { value: "notification", label: "通知" },
  { value: "insert_blocks", label: "插入内容块" },
  { value: "webhook", label: "Webhook" },
  { value: "update_dataset_record", label: "更新数据记录" },
];

function buttonActions(value: string): Record<string, unknown>[] {
  return parsedJSONArray(value, [
    { type: "notification", message: "操作完成", tone: "success" },
  ]);
}

function defaultButtonAction(type: string): Record<string, unknown> {
  switch (type) {
    case "insert_blocks":
      return structuredClone(DEFAULT_INSERT_BLOCK_ACTION);
    case "webhook":
      return { type, url: "https://example.com/webhook", payload: {} };
    case "update_dataset_record":
      return {
        type,
        record_id: "00000000-0000-7000-8000-000000000000",
        values: {},
      };
    default:
      return { type: "notification", message: "操作完成", tone: "success" };
  }
}

function updateButtonAction(
  editor: EditableBlockEditor,
  blockId: string,
  actions: Record<string, unknown>[],
  index: number,
  patch: Record<string, unknown>,
): void {
  const next = actions.map((action, actionIndex) =>
    actionIndex === index ? { ...action, ...patch } : action,
  );
  writeJSONProps(editor, blockId, "actionsJson", next);
}

function renderButtonActionEditor(
  editor: EditableBlockEditor,
  blockId: string,
  actions: Record<string, unknown>[],
  action: Record<string, unknown>,
  index: number,
): HTMLElement {
  const panel = createEditableBlockShell({
    type: "buttonAction",
    title: `动作 ${index + 1}`,
    summary: stringProp(action, "type", "notification"),
  });
  panel.classList.add("ast-editable-block--nested");
  const actionType = stringProp(action, "type", "notification");
  const fields: HTMLElement[] = [
    selectField({
      label: "动作类型",
      value: actionType,
      options: BUTTON_ACTION_OPTIONS,
      onCommit: (value) => {
        const next = actions.map((candidate, actionIndex) =>
          actionIndex === index ? defaultButtonAction(value) : candidate,
        );
        writeJSONProps(editor, blockId, "actionsJson", next);
      },
    }),
  ];
  if (actionType === "notification") {
    fields.push(
      textField({
        label: "通知内容",
        value: stringProp(action, "message"),
        validate: nonEmpty("通知内容不能为空"),
        onCommit: (value) => updateButtonAction(editor, blockId, actions, index, { message: value }),
      }),
      selectField({
        label: "语气",
        value: stringProp(action, "tone", "default"),
        options: [
          { value: "default", label: "默认" },
          { value: "success", label: "成功" },
          { value: "info", label: "信息" },
          { value: "warning", label: "警告" },
          { value: "error", label: "错误" },
        ],
        onCommit: (value) => updateButtonAction(editor, blockId, actions, index, { tone: value }),
      }),
    );
  } else if (actionType === "insert_blocks") {
    fields.push(
      textField({
        label: "目标页面 ID",
        value: stringProp(action, "target_page_id"),
        validate: requiredUUID,
        onCommit: (value) => updateButtonAction(editor, blockId, actions, index, { target_page_id: value.trim() }),
      }),
      jsonTextareaField({
        label: "插入块 JSON",
        value: JSON.stringify(action.blocks ?? [], null, 2),
        fallback: [{ id: newEditorBlockId(), type: "paragraph", content: [{ type: "text", text: "插入内容" }] }],
        rows: 5,
        validate: (parsed) => Array.isArray(parsed) && parsed.length > 0 ? null : "至少需要一个块",
        onCommit: (_jsonText, parsed) => updateButtonAction(editor, blockId, actions, index, { blocks: parsed }),
      }),
    );
  } else if (actionType === "webhook") {
    fields.push(
      textField({
        label: "HTTPS URL",
        value: stringProp(action, "url"),
        validate: (value) => value.startsWith("https://") ? requiredHTTPURL(value) : "Webhook 必须使用 HTTPS",
        onCommit: (value) => updateButtonAction(editor, blockId, actions, index, { url: value.trim() }),
      }),
      jsonTextareaField({
        label: "Payload JSON",
        value: JSON.stringify(action.payload ?? {}, null, 2),
        fallback: {},
        rows: 4,
        validate: (parsed) => isPlainObject(parsed) ? null : "Payload 必须是 JSON object",
        onCommit: (_jsonText, parsed) => updateButtonAction(editor, blockId, actions, index, { payload: parsed }),
      }),
    );
  } else {
    fields.push(
      textField({
        label: "Record ID",
        value: stringProp(action, "record_id"),
        validate: requiredUUID,
        onCommit: (value) => updateButtonAction(editor, blockId, actions, index, { record_id: value.trim() }),
      }),
      textField({
        label: "Entity ID（可选）",
        value: stringProp(action, "entity_id"),
        validate: optionalUUID,
        onCommit: (value) => updateButtonAction(editor, blockId, actions, index, { entity_id: value.trim() || null }),
      }),
      jsonTextareaField({
        label: "Values JSON",
        value: JSON.stringify(action.values ?? {}, null, 2),
        fallback: {},
        rows: 4,
        validate: (parsed) => isPlainObject(parsed) ? null : "Values 必须是 JSON object",
        onCommit: (_jsonText, parsed) => updateButtonAction(editor, blockId, actions, index, { values: parsed }),
      }),
    );
  }
  panel.append(
    createFieldGrid(...fields),
    createActionRow(
      buttonControl({
        label: "删除动作",
        variant: "danger",
        disabled: actions.length <= 1,
        onClick: () => writeJSONProps(editor, blockId, "actionsJson", actions.filter((_, actionIndex) => actionIndex !== index)),
      }),
    ),
  );
  return panel;
}

function staticTextBlock(className: string, text: string): HTMLElement {
  const dom = window.document.createElement("div");
  dom.className = className;
  dom.textContent = text;
  return dom;
}

function renderStaticCallout(block: { props: Record<string, unknown> }) {
  const kind = stringProp(block.props, "kind", "info");
  const icon = stringProp(block.props, "icon");
  const dom = staticTextBlock(
    `ast-callout ast-callout--${kind}`,
    `${icon}${CALLOUT_LABELS[kind] ?? kind}`,
  );
  dom.dataset.color = stringProp(block.props, "color", "default");
  return dom;
}

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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        return { dom: renderStaticCallout(block) };
      }
      const kind = stringProp(block.props, "kind", "info");
      const root = createEditableBlockShell({
        type: "callout",
        title: "提示框",
        summary: CALLOUT_LABELS[kind] ?? kind,
        className: `ast-callout ast-callout--${kind}`,
      });
      root.dataset.color = stringProp(block.props, "color", "default");
      root.appendChild(
        createFieldGrid(
          selectField({
            label: "类型",
            value: kind,
            options: CALLOUT_KIND_OPTIONS,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { kind: value }),
          }),
          textField({
            label: "图标",
            value: stringProp(block.props, "icon"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { icon: value.trim() }),
          }),
          selectField({
            label: "颜色",
            value: stringProp(block.props, "color", "default"),
            options: COLOR_OPTIONS,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { color: value }),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
    },
  },
);

function renderStaticPreformatted(block: { props: Record<string, unknown> }) {
  const figure = window.document.createElement("figure");
  figure.className = "ast-preformatted";
  figure.dataset.kind = stringProp(block.props, "kind", "plain");
  const captionText = stringProp(block.props, "caption");
  if (captionText) {
    const caption = window.document.createElement("figcaption");
    caption.textContent = captionText;
    figure.appendChild(caption);
  }
  const pre = window.document.createElement("pre");
  const code = window.document.createElement("code");
  code.textContent = stringProp(block.props, "text");
  pre.appendChild(code);
  figure.appendChild(pre);
  return figure;
}

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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        return { dom: renderStaticPreformatted(block) };
      }
      const root = createEditableBlockShell({
        type: "preformatted",
        title: "固定宽文本",
        summary: stringProp(block.props, "kind", "plain"),
        className: "ast-preformatted",
      });
      root.append(
        createFieldGrid(
          selectField({
            label: "类型",
            value: stringProp(block.props, "kind", "plain"),
            options: PREFORMATTED_KIND_OPTIONS,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { kind: value }),
          }),
          textField({
            label: "说明",
            value: stringProp(block.props, "caption"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { caption: value }),
          }),
        ),
        textField({
          label: "内容",
          value: stringProp(block.props, "text"),
          multiline: true,
          rows: 6,
          monospace: true,
          onCommit: (value) =>
            patchBlockProps(editableEditor, block.id, { text: value }),
        }),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
    },
  },
);

function renderStaticEquation(block: { props: Record<string, unknown> }) {
  const expression = stringProp(block.props, "expression");
  const caption = stringProp(block.props, "caption");
  const dom = staticTextBlock(
    "ast-equation",
    caption ? `${expression} · ${caption}` : expression,
  );
  dom.dataset.mathExpression = expression;
  return dom;
}

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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        return { dom: renderStaticEquation(block) };
      }
      const root = createEditableBlockShell({
        type: "equation",
        title: "数学公式",
        summary: stringProp(block.props, "expression") || "LaTeX",
        className: "ast-equation",
      });
      root.append(
        textField({
          label: "表达式",
          value: stringProp(block.props, "expression"),
          monospace: true,
          validate: nonEmpty("公式不能为空"),
          onCommit: (value) =>
            patchBlockProps(editableEditor, block.id, { expression: value }),
        }),
        textField({
          label: "说明",
          value: stringProp(block.props, "caption"),
          onCommit: (value) =>
            patchBlockProps(editableEditor, block.id, { caption: value }),
        }),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
    },
  },
);

function renderStaticTable(block: { props: Record<string, unknown> }) {
  const dom = staticTextBlock("ast-table", stringProp(block.props, "caption"));
  dom.dataset.hasColumnHeader = String(
    booleanProp(block.props, "hasColumnHeader"),
  );
  dom.dataset.hasRowHeader = String(booleanProp(block.props, "hasRowHeader"));
  return dom;
}

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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        return { dom: renderStaticTable(block) };
      }
      const root = createEditableBlockShell({
        type: "astTable",
        title: "语义表格",
        summary: "标题与表头设置",
        className: "ast-table",
      });
      const rowCount = childBlocks(block).length;
      const columnCount = Math.max(childBlocks(childBlocks(block)[0] ?? {}).length, 0);
      root.append(
        textField({
          label: "表格标题",
          value: stringProp(block.props, "caption"),
          onCommit: (value) =>
            patchBlockProps(editableEditor, block.id, { caption: value }),
        }),
        createFieldGrid(
          checkboxField({
            label: "第一行为列头",
            checked: booleanProp(block.props, "hasColumnHeader"),
            onCommit: (checked) =>
              patchBlockProps(editableEditor, block.id, {
                hasColumnHeader: checked,
              }),
          }),
          checkboxField({
            label: "第一列为行头",
            checked: booleanProp(block.props, "hasRowHeader"),
            onCommit: (checked) =>
              patchBlockProps(editableEditor, block.id, { hasRowHeader: checked }),
          }),
        ),
        createActionRow(
          buttonControl({
            label: "添加行",
            onClick: () => patchBlockChildren(editableEditor, block.id, addTableRow(block)),
          }),
          buttonControl({
            label: "添加列",
            onClick: () => patchBlockChildren(editableEditor, block.id, addTableColumn(block)),
          }),
          buttonControl({
            label: "删除末行",
            disabled: rowCount <= 1,
            variant: "danger",
            onClick: () => patchBlockChildren(editableEditor, block.id, removeLastTableRow(block)),
          }),
          buttonControl({
            label: "删除末列",
            disabled: columnCount <= 1,
            variant: "danger",
            onClick: () => patchBlockChildren(editableEditor, block.id, removeLastTableColumn(block)),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
    },
  },
);

function breadcrumbPreview(itemsJson: string): string {
  const parsed = breadcrumbItemsSchema.safeParse(JSON.parse(itemsJson));
  if (!parsed.success) return "页面路径";
  return parsed.data.map((item) => item.display_text).join(" / ");
}

function renderStaticBreadcrumb(block: { props: Record<string, unknown> }) {
  const dom = window.document.createElement("div");
  dom.className = "ast-breadcrumb";
  try {
    dom.textContent = breadcrumbPreview(
      stringProp(block.props, "itemsJson", "[]"),
    );
  } catch {
    dom.textContent = "页面路径";
  }
  return dom;
}

const breadcrumbSpec = createBlockSpec(
  {
    type: "breadcrumb",
    propSchema: { itemsJson: { default: "[]" } },
    content: "none",
  },
  {
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        return { dom: renderStaticBreadcrumb(block) };
      }
      const root = createEditableBlockShell({
        type: "breadcrumb",
        title: "页面路径",
        summary: "JSON 列表",
        className: "ast-breadcrumb",
      });
      root.append(
        createPreview(renderStaticBreadcrumb(block).textContent ?? "页面路径"),
        jsonTextareaField({
          label: "路径项 JSON",
          value: stringProp(block.props, "itemsJson", "[]"),
          fallback: [{ display_text: "当前页面" }],
          rows: 5,
          validate: (parsed) =>
            breadcrumbItemsSchema.safeParse(parsed).success
              ? null
              : "需要 1-32 个 { display_text, target_page_id? }",
          onCommit: (jsonText) =>
            patchBlockProps(editableEditor, block.id, { itemsJson: jsonText }),
        }),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        return {
          dom: staticTextBlock(
            "ast-column",
            `列宽 ${numberProp(block.props, "widthRatio", 1)}`,
          ),
        };
      }
      const root = createEditableBlockShell({
        type: "column",
        title: "分栏列",
        summary: `列宽 ${numberProp(block.props, "widthRatio", 1)}`,
        className: "ast-column",
      });
      const parent = findParentBlock(editableEditor.document, block.id, "columnList");
      const siblingCount = parent ? childBlocks(parent).length : 0;
      root.append(
        numberField({
          label: "宽度比例",
          value: numberProp(block.props, "widthRatio", 1),
          min: 0.1,
          max: 10,
          step: 0.1,
          onCommit: (value) =>
            patchBlockProps(editableEditor, block.id, { widthRatio: value }),
        }),
        createActionRow(
          buttonControl({
            label: "添加列",
            disabled: siblingCount >= 5,
            onClick: () => addColumnSibling(editableEditor, block.id),
          }),
          buttonControl({
            label: "删除本列",
            disabled: siblingCount <= 2,
            variant: "danger",
            onClick: () => removeColumn(editableEditor, block.id),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
    },
  },
);

function renderStaticButton(block: { props: Record<string, unknown> }) {
  const button = window.document.createElement("button");
  button.className = "wiki-button";
  button.type = "button";
  button.disabled = true;
  button.textContent = stringProp(block.props, "label", "运行操作");
  return button;
}

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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        return { dom: renderStaticButton(block) };
      }
      const root = createEditableBlockShell({
        type: "pageButton",
        title: "页面按钮",
        summary: stringProp(block.props, "label", "运行操作"),
      });
      const actions = buttonActions(stringProp(block.props, "actionsJson", "[]"));
      root.append(
        renderStaticButton(block),
        createFieldGrid(
          textField({
            label: "按钮名称",
            value: stringProp(block.props, "label", "运行操作"),
            validate: nonEmpty("按钮名称不能为空"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { label: value.trim() }),
          }),
          textField({
            label: "确认提示",
            value: stringProp(block.props, "confirmation"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { confirmation: value }),
          }),
        ),
        createActionRow(
          buttonControl({
            label: "添加通知动作",
            disabled: actions.length >= 10,
            onClick: () => writeJSONProps(editableEditor, block.id, "actionsJson", [...actions, defaultButtonAction("notification")]),
          }),
          buttonControl({
            label: "添加插入动作",
            disabled: actions.length >= 10,
            onClick: () => writeJSONProps(editableEditor, block.id, "actionsJson", [...actions, defaultButtonAction("insert_blocks")]),
          }),
          buttonControl({
            label: "添加 Webhook",
            disabled: actions.length >= 10,
            onClick: () => writeJSONProps(editableEditor, block.id, "actionsJson", [...actions, defaultButtonAction("webhook")]),
          }),
        ),
        ...actions.map((action, index) =>
          renderButtonActionEditor(editableEditor, block.id, actions, action, index),
        ),
        jsonTextareaField({
          label: "高级：动作 JSON",
          value: stringProp(block.props, "actionsJson", "[]"),
          fallback: [defaultButtonAction("notification")],
          rows: 6,
          validate: (parsed) =>
            buttonActionsSchema.safeParse(parsed).success
              ? null
              : "需要 1-10 个合法按钮动作",
          onCommit: (jsonText) =>
            patchBlockProps(editableEditor, block.id, { actionsJson: jsonText }),
        }),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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

function renderStaticHTMLBlock(block: { props: Record<string, unknown> }) {
  const figure = window.document.createElement("figure");
  figure.className = "html-block";
  const iframe = window.document.createElement("iframe");
  iframe.setAttribute("sandbox", "");
  iframe.setAttribute("referrerpolicy", "no-referrer");
  iframe.loading = "lazy";
  iframe.height = String(numberProp(block.props, "height", 240));
  iframe.title = stringProp(block.props, "caption") || "HTML content";
  iframe.srcdoc = isolatedHTMLDocument(stringProp(block.props, "html"));
  figure.appendChild(iframe);
  const captionText = stringProp(block.props, "caption");
  if (captionText) {
    const caption = window.document.createElement("figcaption");
    caption.textContent = captionText;
    figure.appendChild(caption);
  }
  return figure;
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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        return { dom: renderStaticHTMLBlock(block) };
      }
      const root = createEditableBlockShell({
        type: "htmlBlock",
        title: "HTML 块",
        summary: stringProp(block.props, "caption") || "隔离预览",
      });
      root.append(
        createFieldGrid(
          numberField({
            label: "高度（px）",
            value: numberProp(block.props, "height", 240),
            min: 120,
            max: 1200,
            step: 1,
            integer: true,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { height: value }),
          }),
          textField({
            label: "标题",
            value: stringProp(block.props, "caption"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { caption: value }),
          }),
        ),
        textField({
          label: "HTML",
          value: stringProp(block.props, "html"),
          multiline: true,
          rows: 8,
          monospace: true,
          validate: (value) =>
            value.length <= 200000 ? null : "HTML 不能超过 200000 字符",
          onCommit: (value) =>
            patchBlockProps(editableEditor, block.id, { html: value }),
        }),
        renderStaticHTMLBlock(block),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        const dom = staticTextBlock(
          "ast-component",
          `组件 v${numberProp(block.props, "componentVersion", 1)}`,
        );
        dom.dataset.componentId = stringProp(block.props, "componentId");
        dom.dataset.entityId = stringProp(block.props, "entityId");
        return { dom };
      }
      const root = createEditableBlockShell({
        type: "component",
        title: "组件实例",
        summary: `v${numberProp(block.props, "componentVersion", 1)}`,
        className: "ast-component",
      });
      root.dataset.componentId = stringProp(block.props, "componentId");
      root.dataset.entityId = stringProp(block.props, "entityId");
      root.append(
        createFieldGrid(
          textField({
            label: "Component ID",
            value: stringProp(block.props, "componentId"),
            validate: requiredUUID,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, {
                componentId: value.trim(),
              }),
          }),
          numberField({
            label: "版本",
            value: numberProp(block.props, "componentVersion", 1),
            min: 1,
            step: 1,
            integer: true,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, {
                componentVersion: value,
              }),
          }),
          textField({
            label: "Entity ID",
            value: stringProp(block.props, "entityId"),
            validate: requiredUUID,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { entityId: value.trim() }),
          }),
        ),
        jsonTextareaField({
          label: "显示配置 JSON",
          value: stringProp(block.props, "displayConfig", "{}"),
          fallback: {},
          rows: 6,
          validate: (parsed) =>
            isPlainObject(parsed) ? null : "显示配置必须是 JSON object",
          onCommit: (jsonText) =>
            patchBlockProps(editableEditor, block.id, { displayConfig: jsonText }),
        }),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
    },
  },
);

function assetSummary(props: Record<string, unknown>, fallback: string): string {
  return (
    stringProp(props, "caption") ||
    stringProp(props, "title") ||
    stringProp(props, "name") ||
    stringProp(props, "altText") ||
    fallback
  );
}

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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        const dom = staticTextBlock("ast-image", assetSummary(block.props, "图片"));
        dom.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
        return { dom };
      }
      const root = createEditableBlockShell({
        type: "imageAsset",
        title: "图片",
        summary: assetSummary(block.props, "图片"),
        className: "ast-image",
      });
      root.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
      root.append(
        createFieldGrid(
          textField({
            label: "AssetRevision ID",
            value: stringProp(block.props, "assetRevisionId"),
            validate: requiredUUID,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, {
                assetRevisionId: value.trim(),
              }),
          }),
          textField({
            label: "Alt text",
            value: stringProp(block.props, "altText"),
            validate: nonEmpty("Alt text 不能为空"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { altText: value }),
          }),
          textField({
            label: "Caption",
            value: stringProp(block.props, "caption"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { caption: value }),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        const dom = staticTextBlock("ast-video", assetSummary(block.props, "视频"));
        dom.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
        return { dom };
      }
      const root = createEditableBlockShell({
        type: "videoAsset",
        title: "视频",
        summary: assetSummary(block.props, "视频"),
        className: "ast-video",
      });
      root.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
      root.append(
        createFieldGrid(
          textField({
            label: "AssetRevision ID",
            value: stringProp(block.props, "assetRevisionId"),
            validate: requiredUUID,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, {
                assetRevisionId: value.trim(),
              }),
          }),
          textField({
            label: "Poster AssetRevision ID",
            value: stringProp(block.props, "posterAssetRevisionId"),
            validate: optionalUUID,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, {
                posterAssetRevisionId: value.trim(),
              }),
          }),
          textField({
            label: "标题",
            value: stringProp(block.props, "title"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { title: value }),
          }),
          textField({
            label: "Caption",
            value: stringProp(block.props, "caption"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { caption: value }),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        const dom = staticTextBlock(
          "ast-audio",
          [
            stringProp(block.props, "title"),
            stringProp(block.props, "artist"),
            stringProp(block.props, "album"),
          ]
            .filter(Boolean)
            .join(" · "),
        );
        dom.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
        return { dom };
      }
      const root = createEditableBlockShell({
        type: "audioAsset",
        title: "音频",
        summary: assetSummary(block.props, "音频"),
        className: "ast-audio",
      });
      root.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
      root.append(
        createFieldGrid(
          textField({
            label: "AssetRevision ID",
            value: stringProp(block.props, "assetRevisionId"),
            validate: requiredUUID,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, {
                assetRevisionId: value.trim(),
              }),
          }),
          textField({
            label: "标题",
            value: stringProp(block.props, "title"),
            validate: nonEmpty("标题不能为空"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { title: value }),
          }),
          textField({
            label: "艺术家",
            value: stringProp(block.props, "artist"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { artist: value }),
          }),
          textField({
            label: "专辑",
            value: stringProp(block.props, "album"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { album: value }),
          }),
          numberField({
            label: "时长秒数",
            value: numberProp(block.props, "durationSeconds", 0),
            min: 0,
            step: 0.1,
            emptyValue: 0,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { durationSeconds: value }),
          }),
          textField({
            label: "Caption",
            value: stringProp(block.props, "caption"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { caption: value }),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        const dom = staticTextBlock("ast-file", assetSummary(block.props, "文件"));
        dom.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
        return { dom };
      }
      const root = createEditableBlockShell({
        type: "fileAsset",
        title: "文件",
        summary: assetSummary(block.props, "文件"),
        className: "ast-file",
      });
      root.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
      root.append(
        createFieldGrid(
          textField({
            label: "AssetRevision ID",
            value: stringProp(block.props, "assetRevisionId"),
            validate: requiredUUID,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, {
                assetRevisionId: value.trim(),
              }),
          }),
          textField({
            label: "文件名",
            value: stringProp(block.props, "name"),
            validate: nonEmpty("文件名不能为空"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { name: value }),
          }),
          textField({
            label: "Caption",
            value: stringProp(block.props, "caption"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { caption: value }),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      if (!isEditableBlockRender(this, editableEditor)) {
        const dom = staticTextBlock("ast-pdf", assetSummary(block.props, "PDF"));
        dom.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
        return { dom };
      }
      const root = createEditableBlockShell({
        type: "pdfAsset",
        title: "PDF",
        summary: assetSummary(block.props, "PDF"),
        className: "ast-pdf",
      });
      root.dataset.assetRevisionId = stringProp(block.props, "assetRevisionId");
      root.append(
        createFieldGrid(
          textField({
            label: "AssetRevision ID",
            value: stringProp(block.props, "assetRevisionId"),
            validate: requiredUUID,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, {
                assetRevisionId: value.trim(),
              }),
          }),
          textField({
            label: "标题",
            value: stringProp(block.props, "title"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { title: value }),
          }),
          numberField({
            label: "起始页",
            value: numberProp(block.props, "startPage", 1),
            min: 1,
            step: 1,
            integer: true,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { startPage: value }),
          }),
          numberField({
            label: "阅读器高度",
            value: numberProp(block.props, "height", 640),
            min: 320,
            max: 1200,
            step: 1,
            integer: true,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { height: value }),
          }),
          textField({
            label: "Caption",
            value: stringProp(block.props, "caption"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { caption: value }),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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
      dom.textContent = "旧结构化视图已停用";
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
    render(this: unknown, block, editor) {
      const editableEditor = editor as unknown as EditableBlockEditor;
      const summary =
        stringProp(block.props, "title") ||
        stringProp(block.props, "url") ||
        "外部嵌入";
      if (!isEditableBlockRender(this, editableEditor)) {
        return { dom: staticTextBlock("ast-embed", summary) };
      }
      const root = createEditableBlockShell({
        type: "embed",
        title: "外部嵌入",
        summary,
        className: "ast-embed",
      });
      root.appendChild(
        createFieldGrid(
          textField({
            label: "URL",
            value: stringProp(block.props, "url"),
            validate: requiredHTTPURL,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { url: value.trim() }),
          }),
          selectField({
            label: "Provider",
            value: stringProp(block.props, "provider", "none"),
            options: EMBED_PROVIDER_OPTIONS,
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { provider: value }),
          }),
          textField({
            label: "标题",
            value: stringProp(block.props, "title"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { title: value }),
          }),
          textField({
            label: "Caption",
            value: stringProp(block.props, "caption"),
            onCommit: (value) =>
              patchBlockProps(editableEditor, block.id, { caption: value }),
          }),
        ),
      );
      return { dom: root, ignoreMutation: ignoreEditableBlockMutation(root) };
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

// ---- 自定义行内内容：原地编辑 props；导出/复制仍使用静态 atom ----

type InlineUpdate = (update: { type: string; props: Record<string, unknown> }) => void;

function isInlineNodeView(context: unknown): boolean {
  return isPlainObject(context) && context.renderType === "nodeView";
}

function inlineAtomShell(className: string, text: string): HTMLElement {
  const dom = window.document.createElement("span");
  dom.className = className;
  dom.textContent = text;
  return dom;
}

function inlineTextInput(options: {
  label: string;
  value: string;
  onCommit: (value: string) => void;
  validate?: (value: string) => string | null;
  type?: string;
}): HTMLElement {
  const field = textField({
    label: options.label,
    value: options.value,
    validate: options.validate,
    onCommit: options.onCommit,
  });
  const input = field.querySelector("input");
  if (input && options.type) input.type = options.type;
  return field;
}

function inlineEditorShell(options: {
  className: string;
  title: string;
  text: string;
  fields: HTMLElement[];
}): HTMLElement {
  const root = window.document.createElement("span");
  root.className = `ast-inline-editor ${options.className}`;
  root.contentEditable = "false";
  const label = window.document.createElement("span");
  label.className = "ast-inline-editor__label";
  label.textContent = options.title;
  const preview = window.document.createElement("span");
  preview.className = "ast-inline-editor__preview";
  preview.textContent = options.text;
  const panel = window.document.createElement("span");
  panel.className = "ast-inline-editor__panel";
  panel.append(...options.fields);
  root.append(label, preview, panel);
  return root;
}

function editableInlineAtom(options: {
  context: unknown;
  type: string;
  props: Record<string, unknown>;
  update: InlineUpdate;
  className: string;
  title: string;
  text: string;
  fields: (commit: (props: Record<string, unknown>) => void) => HTMLElement[];
  decorateStatic?: (dom: HTMLElement) => void;
}) {
  if (!isInlineNodeView(options.context)) {
    const dom = inlineAtomShell(options.className, options.text);
    options.decorateStatic?.(dom);
    return { dom };
  }
  const commit = (props: Record<string, unknown>) =>
    options.update({ type: options.type, props: { ...options.props, ...props } });
  const dom = inlineEditorShell({
    className: options.className,
    title: options.title,
    text: options.text,
    fields: options.fields(commit),
  });
  options.decorateStatic?.(dom);
  return { dom };
}

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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      const unresolved = props.resolutionStatus === "unresolved";
      const text = unresolved ? props.normalizedTitle : props.displayText;
      return editableInlineAtom({
        context: this,
        type: "pageReference",
        props,
        update: updateInlineContent as InlineUpdate,
        className: unresolved ? "ast-page-ref ast-page-ref--unresolved" : "ast-page-ref",
        title: unresolved ? "未解析页面" : "页面引用",
        text,
        fields: (commit) =>
          unresolved
            ? [
                inlineTextInput({ label: "页面标题", value: stringProp(props, "normalizedTitle"), validate: nonEmpty("页面标题不能为空"), onCommit: (value) => commit({ normalizedTitle: value.trim() }) }),
                inlineTextInput({ label: "命名空间", value: stringProp(props, "targetNamespace", "main"), onCommit: (value) => commit({ targetNamespace: value.trim() }) }),
                inlineTextInput({ label: "期望实体类型", value: stringProp(props, "expectedEntityType"), onCommit: (value) => commit({ expectedEntityType: value.trim() }) }),
              ]
            : [
                inlineTextInput({ label: "显示文本", value: stringProp(props, "displayText"), validate: nonEmpty("显示文本不能为空"), onCommit: (value) => commit({ displayText: value }) }),
                inlineTextInput({ label: "Page ID", value: stringProp(props, "targetPageId"), validate: requiredUUID, onCommit: (value) => commit({ targetPageId: value.trim() }) }),
                inlineTextInput({ label: "Heading Block ID", value: stringProp(props, "targetHeadingBlockId"), validate: optionalUUID, onCommit: (value) => commit({ targetHeadingBlockId: value.trim() }) }),
              ],
      });
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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      return editableInlineAtom({
        context: this,
        type: "pageAnchorReference",
        props,
        update: updateInlineContent as InlineUpdate,
        className: "ast-page-anchor-ref",
        title: "章节引用",
        text: props.displayText,
        fields: (commit) => [
          inlineTextInput({ label: "显示文本", value: props.displayText, validate: nonEmpty("显示文本不能为空"), onCommit: (value) => commit({ displayText: value }) }),
          inlineTextInput({ label: "Page ID", value: props.targetPageId, validate: requiredUUID, onCommit: (value) => commit({ targetPageId: value.trim() }) }),
          inlineTextInput({ label: "Heading Block ID", value: props.targetHeadingBlockId, validate: requiredUUID, onCommit: (value) => commit({ targetHeadingBlockId: value.trim() }) }),
        ],
      });
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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      return editableInlineAtom({
        context: this,
        type: "entityReference",
        props,
        update: updateInlineContent as InlineUpdate,
        className: "ast-entity-ref",
        title: "实体引用",
        text: props.displayText,
        decorateStatic: (dom) => { dom.dataset.entityRef = props.entityId; },
        fields: (commit) => [
          inlineTextInput({ label: "显示文本", value: props.displayText, validate: nonEmpty("显示文本不能为空"), onCommit: (value) => commit({ displayText: value }) }),
          inlineTextInput({ label: "Entity ID", value: props.entityId, validate: requiredUUID, onCommit: (value) => commit({ entityId: value.trim() }) }),
        ],
      });
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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      return editableInlineAtom({
        context: this,
        type: "claimReference",
        props,
        update: updateInlineContent as InlineUpdate,
        className: "ast-claim-ref",
        title: "Claim 引用",
        text: props.displayText,
        decorateStatic: (dom) => { dom.dataset.claimRef = props.claimId; },
        fields: (commit) => [
          inlineTextInput({ label: "显示文本", value: props.displayText, validate: nonEmpty("显示文本不能为空"), onCommit: (value) => commit({ displayText: value }) }),
          inlineTextInput({ label: "Claim ID", value: props.claimId, validate: requiredUUID, onCommit: (value) => commit({ claimId: value.trim() }) }),
        ],
      });
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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      return editableInlineAtom({
        context: this,
        type: "citationReference",
        props,
        update: updateInlineContent as InlineUpdate,
        className: "ast-citation-ref",
        title: "Citation 引用",
        text: props.displayText || "[引用]",
        decorateStatic: (dom) => {
          dom.dataset.citationRef = props.citationId;
          dom.title = props.displayText;
        },
        fields: (commit) => [
          inlineTextInput({ label: "显示文本", value: props.displayText, onCommit: (value) => commit({ displayText: value }) }),
          inlineTextInput({ label: "Citation ID", value: props.citationId, validate: requiredUUID, onCommit: (value) => commit({ citationId: value.trim() }) }),
        ],
      });
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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      return editableInlineAtom({
        context: this,
        type: "math",
        props,
        update: updateInlineContent as InlineUpdate,
        className: "ast-math",
        title: "行内公式",
        text: props.expression,
        fields: (commit) => [
          inlineTextInput({ label: "表达式", value: props.expression, validate: nonEmpty("公式不能为空"), onCommit: (value) => commit({ expression: value }) }),
        ],
      });
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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      return editableInlineAtom({
        context: this,
        type: "mention",
        props,
        update: updateInlineContent as InlineUpdate,
        className: "ast-mention",
        title: "提及",
        text: `@${props.displayText}`,
        decorateStatic: (dom) => { dom.dataset.actorId = props.actorId; },
        fields: (commit) => [
          inlineTextInput({ label: "显示名称", value: props.displayText, validate: nonEmpty("显示名称不能为空"), onCommit: (value) => commit({ displayText: value }) }),
          inlineTextInput({ label: "Actor ID", value: props.actorId, validate: requiredUUID, onCommit: (value) => commit({ actorId: value.trim() }) }),
        ],
      });
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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      return editableInlineAtom({
        context: this,
        type: "dateMention",
        props,
        update: updateInlineContent as InlineUpdate,
        className: "ast-date-mention",
        title: "日期",
        text: props.displayText,
        decorateStatic: (dom) => {
          dom.dataset.endDate = props.endDate;
          if (dom instanceof HTMLTimeElement) dom.dateTime = props.date;
        },
        fields: (commit) => [
          inlineTextInput({ label: "开始日期", value: props.date, type: "date", validate: nonEmpty("开始日期不能为空"), onCommit: (value) => commit({ date: value }) }),
          inlineTextInput({ label: "结束日期", value: props.endDate, type: "date", onCommit: (value) => commit({ endDate: value }) }),
          inlineTextInput({ label: "显示文本", value: props.displayText, validate: nonEmpty("显示文本不能为空"), onCommit: (value) => commit({ displayText: value }) }),
        ],
      });
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
    render(inlineContent, updateInlineContent) {
      const props = inlineContent.props;
      return editableInlineAtom({
        context: this,
        type: "databaseMention",
        props,
        update: updateInlineContent as InlineUpdate,
        className: "ast-database-mention",
        title: "Dataset 提及",
        text: props.displayText,
        decorateStatic: (dom) => { dom.dataset.datasetId = props.datasetId; },
        fields: (commit) => [
          inlineTextInput({ label: "显示文本", value: props.displayText, validate: nonEmpty("显示文本不能为空"), onCommit: (value) => commit({ displayText: value }) }),
          inlineTextInput({ label: "Dataset ID", value: props.datasetId, validate: requiredUUID, onCommit: (value) => commit({ datasetId: value.trim() }) }),
        ],
      });
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
