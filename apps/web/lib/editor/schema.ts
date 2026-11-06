import React from "react";
import { Extension, Mark, Node, mergeAttributes, type JSONContent } from "@tiptap/core";
import { ReactNodeViewRenderer, NodeViewContent, NodeViewWrapper, type ReactNodeViewProps } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import { Plugin } from "@tiptap/pm/state";

import type { Block as AstBlock } from "@/lib/ast/schema";
import { newBlockId } from "@/lib/editor/ids";

export type EditorDoc = JSONContent;
export type EditorNode = JSONContent;

export const AST_BLOCK_TYPES = [
  "heading", "paragraph", "bullet_list", "ordered_list", "list_item", "table", "table_row", "table_cell",
  "code", "preformatted", "equation", "todo", "toggle", "breadcrumb", "column_list", "column", "button", "html",
  "quote", "callout", "component", "image", "video", "audio", "file", "pdf", "bookmark", "link_preview", "embed", "divider",
] as const satisfies readonly AstBlock["type"][];

export type AstBlockType = (typeof AST_BLOCK_TYPES)[number];

export const AST_TO_EDITOR_BLOCK: Record<AstBlockType, string> = {
  heading: "heading",
  paragraph: "paragraph",
  bullet_list: "bulletList",
  ordered_list: "orderedList",
  list_item: "listItem",
  table: "astTable",
  table_row: "astTableRow",
  table_cell: "astTableCell",
  code: "codeBlock",
  preformatted: "preformatted",
  equation: "equation",
  todo: "todoBlock",
  toggle: "toggleBlock",
  breadcrumb: "breadcrumb",
  column_list: "columnList",
  column: "column",
  button: "pageButton",
  html: "htmlBlock",
  quote: "blockquote",
  callout: "callout",
  component: "componentBlock",
  image: "imageAsset",
  video: "videoAsset",
  audio: "audioAsset",
  file: "fileAsset",
  pdf: "pdfAsset",
  bookmark: "bookmark",
  link_preview: "linkPreview",
  embed: "embedBlock",
  divider: "horizontalRule",
};

const blockTypesWithId = [
  "paragraph", "heading", "bulletList", "orderedList", "listItem", "blockquote", "codeBlock", "horizontalRule",
  "preformatted", "equation", "todoBlock", "toggleBlock", "breadcrumb", "columnList", "column", "pageButton", "htmlBlock",
  "callout", "componentBlock", "imageAsset", "videoAsset", "audioAsset", "fileAsset", "pdfAsset", "bookmark", "linkPreview",
  "embedBlock", "astTable", "astTableRow", "astTableCell",
];

const CALLOUT_LABELS: Record<string, string> = { info: "信息", warning: "警告", danger: "危险" };
const FIELD_STYLE = "ast-editable-block__field";
const CONTROL_STYLE = "ast-editable-block__control";

function textValue(value: unknown): string {
  return typeof value === "string" ? value : value == null ? "" : String(value);
}

function numberValue(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function updateAttr(props: ReactNodeViewProps, key: string, value: unknown) {
  props.updateAttributes({ [key]: value });
}

function Field({ label, children }: { label: string; children?: React.ReactNode }) {
  return React.createElement("label", { className: FIELD_STYLE },
    React.createElement("span", { className: "ast-editable-block__label" }, label),
    children,
  );
}

function TextInput(props: ReactNodeViewProps & { attrName: string; label: string; multiline?: boolean; type?: string }) {
  const value = textValue(props.node.attrs[props.attrName]);
  const common = {
    className: `${CONTROL_STYLE}${props.multiline ? " ast-editable-block__textarea" : ""}`,
    defaultValue: value,
    contentEditable: false,
    onBlur: (event: React.FocusEvent<HTMLInputElement | HTMLTextAreaElement>) => updateAttr(props, props.attrName, event.currentTarget.value),
    onKeyDown: (event: React.KeyboardEvent<HTMLInputElement | HTMLTextAreaElement>) => event.stopPropagation(),
  };
  return React.createElement(Field, { label: props.label },
    props.multiline
      ? React.createElement("textarea", { ...common, rows: 4 })
      : React.createElement("input", { ...common, type: props.type ?? "text" }),
  );
}

function NumberInput(props: ReactNodeViewProps & { attrName: string; label: string; min?: number; max?: number; step?: number; fallback: number }) {
  const value = numberValue(props.node.attrs[props.attrName], props.fallback);
  return React.createElement(Field, { label: props.label },
    React.createElement("input", {
      className: CONTROL_STYLE,
      contentEditable: false,
      type: "number",
      defaultValue: String(value),
      min: props.min,
      max: props.max,
      step: props.step,
      onBlur: (event: React.FocusEvent<HTMLInputElement>) => {
        const next = Number(event.currentTarget.value);
        if (Number.isFinite(next)) updateAttr(props, props.attrName, next);
      },
      onKeyDown: (event: React.KeyboardEvent<HTMLInputElement>) => event.stopPropagation(),
    }),
  );
}

function SelectInput(props: ReactNodeViewProps & { attrName: string; label: string; options: { value: string; label: string }[] }) {
  const value = textValue(props.node.attrs[props.attrName]);
  return React.createElement(Field, { label: props.label },
    React.createElement("select", {
      className: CONTROL_STYLE,
      contentEditable: false,
      value,
      onChange: (event: React.ChangeEvent<HTMLSelectElement>) => updateAttr(props, props.attrName, event.currentTarget.value),
      onKeyDown: (event: React.KeyboardEvent<HTMLSelectElement>) => event.stopPropagation(),
    }, props.options.map((option) => React.createElement("option", { key: option.value, value: option.value }, option.label))),
  );
}

function CheckboxInput(props: ReactNodeViewProps & { attrName: string; label: string }) {
  return React.createElement("label", { className: "ast-editable-block__checkbox-row", contentEditable: false },
    React.createElement("input", {
      type: "checkbox",
      checked: Boolean(props.node.attrs[props.attrName]),
      onChange: (event: React.ChangeEvent<HTMLInputElement>) => updateAttr(props, props.attrName, event.currentTarget.checked),
    }),
    React.createElement("span", null, props.label),
  );
}

function BlockShell(props: ReactNodeViewProps & { title: string; summary?: string; className?: string; children?: React.ReactNode; content?: boolean }) {
  return React.createElement(NodeViewWrapper, {
    as: "section",
    className: ["ast-editable-block", props.className].filter(Boolean).join(" "),
    "data-editor-block-type": props.node.type.name,
  },
    React.createElement("details", null,
      React.createElement("summary", { className: "ast-editable-block__summary-trigger", contentEditable: false },
        React.createElement("div", { className: "ast-editable-block__header" },
          React.createElement("span", { className: "ast-editable-block__title" }, props.title),
          props.summary ? React.createElement("span", { className: "ast-editable-block__summary" }, props.summary) : null,
          React.createElement("span", { className: "ast-editable-block__hint" }, "点击编辑"),
        ),
      ),
      React.createElement("div", { className: "ast-editable-block__grid", contentEditable: false }, props.children),
    ),
    props.content ? React.createElement(NodeViewContent, { className: "ast-editable-block__content" }) : null,
  );
}

function CalloutView(props: ReactNodeViewProps) {
  const kind = textValue(props.node.attrs.kind) || "info";
  return React.createElement(BlockShell, { ...props, title: "提示框", summary: CALLOUT_LABELS[kind] ?? kind, className: `ast-callout ast-callout--${kind}`, content: true },
    React.createElement(SelectInput, { ...props, label: "类型", attrName: "kind", options: [
      { value: "info", label: "信息" }, { value: "warning", label: "警告" }, { value: "danger", label: "危险" },
    ] }),
    React.createElement(TextInput, { ...props, label: "图标", attrName: "icon" }),
    React.createElement(TextInput, { ...props, label: "颜色", attrName: "color" }),
  );
}

function LeafView(props: ReactNodeViewProps & { title: string; summary?: string; className?: string; fields: React.ReactNode[] }) {
  return React.createElement(BlockShell, { ...props, title: props.title, summary: props.summary, className: props.className }, ...props.fields);
}

function PreformattedView(props: ReactNodeViewProps) {
  return React.createElement(LeafView, { ...props, title: "固定宽文本", summary: textValue(props.node.attrs.kind), className: "ast-preformatted", fields: [
    React.createElement(TextInput, { ...props, key: "caption", label: "说明", attrName: "caption" }),
    React.createElement(TextInput, { ...props, key: "text", label: "内容", attrName: "text", multiline: true }),
  ] });
}

function EquationView(props: ReactNodeViewProps) {
  return React.createElement(LeafView, { ...props, title: "公式", summary: textValue(props.node.attrs.expression), className: "ast-equation", fields: [
    React.createElement(TextInput, { ...props, key: "expression", label: "表达式", attrName: "expression" }),
    React.createElement(TextInput, { ...props, key: "caption", label: "说明", attrName: "caption" }),
  ] });
}

function SimpleAttrsView(props: ReactNodeViewProps & { title: string; summaryAttr?: string; fields: { attr: string; label: string; multiline?: boolean; number?: boolean; fallback?: number }[]; className?: string }) {
  const summary = props.summaryAttr ? textValue(props.node.attrs[props.summaryAttr]) : undefined;
  return React.createElement(LeafView, { ...props, title: props.title, summary, className: props.className, fields: props.fields.map((field) => field.number
    ? React.createElement(NumberInput, { ...props, key: field.attr, attrName: field.attr, label: field.label, fallback: field.fallback ?? 0 })
    : React.createElement(TextInput, { ...props, key: field.attr, attrName: field.attr, label: field.label, multiline: field.multiline })) });
}

function InlineAtomView(props: ReactNodeViewProps & { label: string; className: string; textAttrs: string[]; fields: { attr: string; label: string; type?: string }[] }) {
  const text = props.textAttrs.map((attr) => textValue(props.node.attrs[attr])).filter(Boolean).join(" ");
  return React.createElement(NodeViewWrapper, {
    as: "span",
    className: `ast-inline-editor ${props.className}`,
    contentEditable: false,
  },
    React.createElement("span", { className: "ast-inline-editor__label" }, props.label),
    React.createElement("span", { className: "ast-inline-editor__preview" }, text),
    React.createElement("span", { className: "ast-inline-editor__panel" },
      props.fields.map((field) => React.createElement(TextInput, { ...props, key: field.attr, attrName: field.attr, label: field.label, type: field.type })),
    ),
  );
}

function idAttrs(extra: Record<string, unknown> = {}) {
  return {
    id: { default: null, parseHTML: (el: HTMLElement) => el.getAttribute("data-id"), renderHTML: (attrs: Record<string, unknown>) => attrs.id ? { "data-id": attrs.id } : {} },
    ...extra,
  };
}

const BlockIds = Extension.create({
  name: "anbyBlockIds",
  addGlobalAttributes() {
    return [
      { types: blockTypesWithId, attributes: idAttrs() },
      { types: ["codeBlock"], attributes: { caption: { default: "" } } },
    ];
  },
  addProseMirrorPlugins() {
    return [new Plugin({
      appendTransaction: (_transactions, _oldState, newState) => {
        const tr = newState.tr;
        let changed = false;
        newState.doc.descendants((node, pos) => {
          if (blockTypesWithId.includes(node.type.name) && !node.attrs.id) {
            tr.setNodeMarkup(pos, undefined, { ...node.attrs, id: newBlockId() });
            changed = true;
          }
        });
        return changed ? tr : null;
      },
    })];
  },
});

function atomNode(name: string, attrs: Record<string, unknown>, view: React.ComponentType<ReactNodeViewProps>) {
  return Node.create({
    name,
    group: "block",
    atom: true,
    draggable: true,
    addAttributes: () => idAttrs(attrs),
    parseHTML: () => [{ tag: `[data-node-type="${name}"]` }],
    renderHTML: ({ HTMLAttributes }) => ["div", mergeAttributes(HTMLAttributes, { "data-node-type": name })],
    addNodeView: () => ReactNodeViewRenderer(view),
  });
}

function containerNode(name: string, content: string, view: React.ComponentType<ReactNodeViewProps>, attrs: Record<string, unknown> = {}) {
  return Node.create({
    name,
    group: "block",
    content,
    isolating: true,
    addAttributes: () => idAttrs(attrs),
    parseHTML: () => [{ tag: `[data-node-type="${name}"]` }],
    renderHTML: ({ HTMLAttributes }) => ["div", mergeAttributes(HTMLAttributes, { "data-node-type": name }), 0],
    addNodeView: () => ReactNodeViewRenderer(view),
  });
}

const InlineCode = Mark.create({
  name: "inlineCode",
  code: true,
  parseHTML: () => [{ tag: "code[data-inline-code]" }],
  renderHTML: ({ HTMLAttributes }) => ["code", mergeAttributes(HTMLAttributes, { "data-inline-code": "", class: "ast-inline-code" }), 0],
});

const TextColor = Mark.create({
  name: "textColor",
  addAttributes: () => ({ color: { default: "", renderHTML: (attrs) => attrs.color ? { "data-text-color": attrs.color } : {} } }),
  parseHTML: () => [{ tag: "span[data-text-color]", getAttrs: (el) => ({ color: (el as HTMLElement).dataset.textColor }) }],
  renderHTML: ({ HTMLAttributes }) => ["span", HTMLAttributes, 0],
});

const BackgroundColor = Mark.create({
  name: "backgroundColor",
  addAttributes: () => ({ color: { default: "", renderHTML: (attrs) => attrs.color ? { "data-background-color": attrs.color } : {} } }),
  parseHTML: () => [{ tag: "span[data-background-color]", getAttrs: (el) => ({ color: (el as HTMLElement).dataset.backgroundColor }) }],
  renderHTML: ({ HTMLAttributes }) => ["span", HTMLAttributes, 0],
});

function inlineAtomNode(name: string, attrs: Record<string, unknown>, view: React.ComponentType<ReactNodeViewProps>) {
  return Node.create({
    name,
    group: "inline",
    inline: true,
    atom: true,
    selectable: true,
    addAttributes: () => attrs,
    parseHTML: () => [{ tag: `[data-inline-type="${name}"]` }],
    renderHTML: ({ HTMLAttributes }) => ["span", mergeAttributes(HTMLAttributes, { "data-inline-type": name })],
    addNodeView: () => ReactNodeViewRenderer(view),
  });
}

export const editorExtensions = [
  StarterKit.configure({
    link: false,
    horizontalRule: { HTMLAttributes: { class: "wiki-divider" } },
    heading: { levels: [1, 2, 3, 4, 5, 6] },
  }),
  Link.configure({ openOnClick: false }),
  BlockIds,
  InlineCode,
  TextColor,
  BackgroundColor,
  containerNode("callout", "block*", CalloutView, { kind: { default: "info" }, icon: { default: "" }, color: { default: "default" } }),
  atomNode("preformatted", { kind: { default: "plain" }, caption: { default: "" }, text: { default: "" } }, PreformattedView),
  atomNode("equation", { expression: { default: "" }, caption: { default: "" } }, EquationView),
  containerNode("todoBlock", "block*", (props) => React.createElement(BlockShell, { ...props, title: "待办", summary: props.node.attrs.checked ? "已完成" : "未完成", content: true }, React.createElement(CheckboxInput, { ...props, label: "完成", attrName: "checked" })), { checked: { default: false } }),
  containerNode("toggleBlock", "block*", (props) => React.createElement(BlockShell, { ...props, title: "折叠块", summary: "详情", content: true }, React.createElement(TextInput, { ...props, label: "摘要", attrName: "summaryText" })), { summaryText: { default: "详情" }, summaryJson: { default: "[]" } }),
  atomNode("breadcrumb", { itemsJson: { default: "[]" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "页面路径", summaryAttr: "itemsJson", fields: [{ attr: "itemsJson", label: "路径项 JSON", multiline: true }] })),
  containerNode("columnList", "column+", (props) => React.createElement(BlockShell, { ...props, title: "分栏", content: true })),
  containerNode("column", "block*", (props) => React.createElement(BlockShell, { ...props, title: "列", summary: `宽度 ${props.node.attrs.widthRatio}`, content: true }, React.createElement(NumberInput, { ...props, label: "宽度比例", attrName: "widthRatio", fallback: 1, step: 0.1 })), { widthRatio: { default: 1 } }),
  atomNode("pageButton", { label: { default: "运行操作" }, confirmation: { default: "" }, actionsJson: { default: "[]" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "页面按钮", summaryAttr: "label", fields: [{ attr: "label", label: "按钮名称" }, { attr: "confirmation", label: "确认提示" }, { attr: "actionsJson", label: "动作 JSON", multiline: true }] })),
  atomNode("htmlBlock", { html: { default: "" }, height: { default: 240 }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "HTML", summaryAttr: "caption", fields: [{ attr: "height", label: "高度", number: true, fallback: 240 }, { attr: "caption", label: "标题" }, { attr: "html", label: "HTML", multiline: true }] })),
  containerNode("astTable", "astTableRow+", (props) => React.createElement(BlockShell, { ...props, title: "语义表格", summary: props.node.attrs.caption, content: true }, React.createElement(TextInput, { ...props, label: "标题", attrName: "caption" }), React.createElement(CheckboxInput, { ...props, label: "第一行为列头", attrName: "hasColumnHeader" }), React.createElement(CheckboxInput, { ...props, label: "第一列为行头", attrName: "hasRowHeader" })), { caption: { default: "" }, hasColumnHeader: { default: false }, hasRowHeader: { default: false } }),
  containerNode("astTableRow", "astTableCell+", () => React.createElement(NodeViewWrapper, { as: "div", className: "ast-table-row" }, React.createElement(NodeViewContent, null))),
  containerNode("astTableCell", "block*", () => React.createElement(NodeViewWrapper, { as: "div", className: "ast-table-cell" }, React.createElement(NodeViewContent, null))),
  atomNode("componentBlock", { componentId: { default: "" }, componentVersion: { default: 1 }, entityId: { default: "" }, displayConfig: { default: "{}" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "组件", summaryAttr: "componentId", className: "ast-component", fields: [{ attr: "componentId", label: "Component ID" }, { attr: "componentVersion", label: "版本", number: true, fallback: 1 }, { attr: "entityId", label: "Entity ID" }, { attr: "displayConfig", label: "显示配置", multiline: true }] })),
  atomNode("imageAsset", { assetRevisionId: { default: "" }, altText: { default: "" }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "图片", summaryAttr: "caption", className: "ast-image", fields: [{ attr: "assetRevisionId", label: "AssetRevision ID" }, { attr: "altText", label: "Alt text" }, { attr: "caption", label: "Caption" }] })),
  atomNode("videoAsset", { assetRevisionId: { default: "" }, posterAssetRevisionId: { default: "" }, title: { default: "" }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "视频", summaryAttr: "title", className: "ast-video", fields: [{ attr: "assetRevisionId", label: "AssetRevision ID" }, { attr: "posterAssetRevisionId", label: "Poster" }, { attr: "title", label: "标题" }, { attr: "caption", label: "Caption" }] })),
  atomNode("audioAsset", { assetRevisionId: { default: "" }, title: { default: "" }, artist: { default: "" }, album: { default: "" }, durationSeconds: { default: 0 }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "音频", summaryAttr: "title", className: "ast-audio", fields: [{ attr: "assetRevisionId", label: "AssetRevision ID" }, { attr: "title", label: "标题" }, { attr: "artist", label: "艺术家" }, { attr: "album", label: "专辑" }, { attr: "durationSeconds", label: "时长", number: true, fallback: 0 }, { attr: "caption", label: "Caption" }] })),
  atomNode("fileAsset", { assetRevisionId: { default: "" }, name: { default: "" }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "文件", summaryAttr: "name", className: "ast-file", fields: [{ attr: "assetRevisionId", label: "AssetRevision ID" }, { attr: "name", label: "文件名" }, { attr: "caption", label: "Caption" }] })),
  atomNode("pdfAsset", { assetRevisionId: { default: "" }, title: { default: "" }, startPage: { default: 1 }, height: { default: 640 }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "PDF", summaryAttr: "title", className: "ast-pdf", fields: [{ attr: "assetRevisionId", label: "AssetRevision ID" }, { attr: "title", label: "标题" }, { attr: "startPage", label: "起始页", number: true, fallback: 1 }, { attr: "height", label: "高度", number: true, fallback: 640 }, { attr: "caption", label: "Caption" }] })),
  atomNode("bookmark", { url: { default: "" }, title: { default: "" }, description: { default: "" }, siteName: { default: "" }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "Bookmark", summaryAttr: "title", className: "ast-bookmark", fields: [{ attr: "url", label: "URL" }, { attr: "title", label: "标题" }, { attr: "siteName", label: "站点" }, { attr: "description", label: "描述", multiline: true }, { attr: "caption", label: "Caption" }] })),
  atomNode("linkPreview", { url: { default: "" }, title: { default: "" }, description: { default: "" }, siteName: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "链接预览", summaryAttr: "title", className: "ast-link-preview", fields: [{ attr: "url", label: "URL" }, { attr: "title", label: "标题" }, { attr: "siteName", label: "站点" }, { attr: "description", label: "描述", multiline: true }] })),
  atomNode("embedBlock", { url: { default: "" }, title: { default: "" }, provider: { default: "none" }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "Embed", summaryAttr: "title", className: "ast-embed", fields: [{ attr: "url", label: "URL" }, { attr: "provider", label: "Provider" }, { attr: "title", label: "标题" }, { attr: "caption", label: "Caption" }] })),
  inlineAtomNode("pageReference", { targetPageId: { default: "" }, targetHeadingBlockId: { default: "" }, displayText: { default: "" }, resolutionStatus: { default: "" }, targetNamespace: { default: "" }, normalizedTitle: { default: "" }, expectedEntityType: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "页面", className: "ast-page-ref", textAttrs: ["displayText", "normalizedTitle"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "targetPageId", label: "Page ID" }, { attr: "normalizedTitle", label: "未解析标题" }] })),
  inlineAtomNode("pageAnchorReference", { targetPageId: { default: "" }, targetHeadingBlockId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "锚点", className: "ast-page-anchor-ref", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "targetPageId", label: "Page ID" }, { attr: "targetHeadingBlockId", label: "Heading ID" }] })),
  inlineAtomNode("entityReference", { entityId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "实体", className: "ast-entity-ref", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "entityId", label: "Entity ID" }] })),
  inlineAtomNode("claimReference", { claimId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "Claim", className: "ast-claim-ref", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "claimId", label: "Claim ID" }] })),
  inlineAtomNode("citationReference", { citationId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "引用", className: "ast-citation-ref", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "citationId", label: "Citation ID" }] })),
  inlineAtomNode("mathInline", { expression: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "公式", className: "ast-math", textAttrs: ["expression"], fields: [{ attr: "expression", label: "表达式" }] })),
  inlineAtomNode("mention", { actorId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "提及", className: "ast-mention", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示名称" }, { attr: "actorId", label: "Actor ID" }] })),
  inlineAtomNode("dateMention", { date: { default: "" }, endDate: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "日期", className: "ast-date-mention", textAttrs: ["displayText"], fields: [{ attr: "date", label: "开始日期", type: "date" }, { attr: "endDate", label: "结束日期", type: "date" }, { attr: "displayText", label: "显示文本" }] })),
];

export interface PMBlock {
  type?: string;
  attrs?: Record<string, unknown>;
  content?: PMBlock[];
  text?: string;
  marks?: { type: string; attrs?: Record<string, unknown> }[];
}

export interface PMDocument extends PMBlock {
  type: "doc";
  content?: PMBlock[];
}
