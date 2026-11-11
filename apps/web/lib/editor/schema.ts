import React, { useState } from "react";
import { ColumnsView, ColumnView } from "@/lib/editor/columns";
import { CalloutView } from "@/lib/editor/callout";
import { EditableAtomView } from "@/lib/editor/editable-atom";
import { TableView } from "@/lib/editor/table";
import { ChevronRight } from "lucide-react";
import { toast } from "sonner";
import { assetsApi } from "@/lib/api";
import { ResourcePicker, type ResourceKind, type MediaKind, type ResourceChoice } from "@/components/editor/resource-picker";
import { AnchorPicker, BreadcrumbFields } from "@/components/editor/reference-fields";
import { ComponentInsertDialog } from "@/components/editor/component-insert-button";
import { Button } from "@/components/ui/button";
import type { ComponentSelection } from "@/components/components/component-configure";
import { BlockView } from "@/components/ast";
import { toAst } from "@/lib/editor/toAst";
import { Extension, Mark, Node, mergeAttributes, type JSONContent } from "@tiptap/core";
import { ReactNodeViewRenderer, NodeViewContent, NodeViewWrapper, useEditorState, type ReactNodeViewProps } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import { SuggestionPluginKey } from "@tiptap/suggestion";
import { closeHistory } from "@tiptap/pm/history";
import { Plugin, TextSelection } from "@tiptap/pm/state";
import { Mapping } from "@tiptap/pm/transform";
import type { EditorView } from "@tiptap/pm/view";

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
  "paragraph", "heading", "sectionHeading", "bulletList", "orderedList", "listItem", "blockquote", "codeBlock", "horizontalRule",
  "preformatted", "equation", "todoBlock", "toggleBlock", "breadcrumb", "columnList", "column", "pageButton", "htmlBlock",
  "callout", "componentBlock", "imageAsset", "videoAsset", "audioAsset", "fileAsset", "pdfAsset", "bookmark", "linkPreview",
  "embedBlock", "astTable", "astTableRow", "astTableCell",
];

const FIELD_STYLE = "ast-editable-block__field";
const CONTROL_STYLE = "ast-editable-block__control";

function textValue(value: unknown): string {
  return typeof value === "string" ? value : value == null ? "" : String(value);
}

function numberValue(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function updateAttrs(props: ReactNodeViewProps, attrs: Record<string, unknown>) {
  const pos = props.getPos();
  if (pos === undefined || !props.editor.isEditable) return false;
  try {
    const node = props.editor.state.doc.nodeAt(pos);
    if (!node || node.type !== props.node.type) return false;
    const tr = props.editor.state.tr.setNodeMarkup(pos, undefined, { ...node.attrs, ...attrs });
    toAst(tr.doc.toJSON());
    props.editor.view.dispatch(tr);
    return true;
  } catch {
    toast.error("输入格式不正确，原内容已保留。");
    return false;
  }
}

function updateAttr(props: ReactNodeViewProps, key: string, value: unknown) {
  return updateAttrs(props, { [key]: value });
}

const resourceAttrs: Record<string, ResourceKind> = { targetPageId: "page", targetHeadingBlockId: "heading", entityId: "entity", claimId: "claim", citationId: "citation", actorId: "member", assetRevisionId: "asset", posterAssetRevisionId: "asset" };
function ResourceInput(props: ReactNodeViewProps & { attrName: string; label: string }) {
  const attr = props.attrName;
  const attrs = props.node.attrs;
  if (props.node.type.name === "pageAnchorReference") return React.createElement(AnchorPicker, {
    pageId: textValue(attrs.targetPageId), headingId: textValue(attrs.targetHeadingBlockId),
    onChange: choice => updateAttrs(props, { targetPageId: choice.pageId, targetHeadingBlockId: choice.id, ...(choice.label ? { displayText: choice.label } : {}) }),
  });
  const kind = resourceAttrs[attr];
  const mediaKind = (attr === "posterAssetRevisionId" ? "image" : props.node.type.name.replace("Asset", "")) as MediaKind;
  const select = async (choice: ResourceChoice) => {
    const changes: Record<string, unknown> = { [attr]: choice.id };
    if (kind === "asset" && choice.id) {
      try {
        const revision = await assetsApi().getAssetRevision({ revisionId: choice.id });
        if ((mediaKind === "pdf" && revision.mimeType !== "application/pdf") || (["image", "audio", "video"].includes(mediaKind) && !revision.mimeType.startsWith(`${mediaKind}/`))) {
          toast.error("文件类型不匹配，请重新选择。"); return false;
        }
      } catch { toast.error("无法读取文件版本，请重试。"); return false; }
    }
    if (attr === "targetPageId") Object.assign(changes, { resolutionStatus: "", normalizedTitle: "", targetHeadingBlockId: "" });
    if (Object.hasOwn(attrs, "displayText") && choice.label) changes.displayText = choice.label;
    return updateAttrs(props, changes);
  };
  return React.createElement(Field, { label: props.label }, React.createElement(ResourcePicker, {
    kind, value: textValue(attrs[attr]), label: props.label, mediaKind: kind === "asset" ? mediaKind : undefined,
    pageId: textValue(attrs.targetPageId), clearable: attr === "posterAssetRevisionId" || attr === "targetHeadingBlockId",
    valueLabel: ["entity", "claim", "citation", "member"].includes(kind) ? textValue(attrs.displayText) : undefined,
    onChange: select,
  }));
}

function ComponentBlockView(props: ReactNodeViewProps) {
  const [open, setOpen] = useState(false);
  const attrs = props.node.attrs;
  const selection: ComponentSelection = { componentId: textValue(attrs.componentId), componentVersion: numberValue(attrs.componentVersion, 1), entityId: textValue(attrs.entityId), displayConfig: JSON.parse(textValue(attrs.displayConfig) || "{}") };
  return React.createElement(BlockShell, { ...props, title: "知识组件", summary: `v${selection.componentVersion}`, className: "ast-component" },
    React.createElement(Button, { type: "button", variant: "outline", onClick: () => setOpen(true) }, "选择组件与实体"),
    React.createElement(ComponentInsertDialog, { open, onOpenChange: setOpen, initialSelection: selection, onInsert: value => updateAttrs(props, { ...value, displayConfig: JSON.stringify(value.displayConfig) }) }),
  );
}

function Field({ label, children }: { label: string; children?: React.ReactNode }) {
  return React.createElement("label", { className: FIELD_STYLE },
    React.createElement("span", { className: "ast-editable-block__label" }, label),
    children,
  );
}

function TextInput(props: ReactNodeViewProps & { attrName: string; label: string; multiline?: boolean; type?: string }) {
  const value = textValue(props.node.attrs[props.attrName]);
  if (resourceAttrs[props.attrName]) return React.createElement(ResourceInput, props);
  const common = {
    className: `${CONTROL_STYLE}${props.multiline ? " ast-editable-block__textarea" : ""}`,
    key: value,
    defaultValue: value,
    disabled: !props.editor.isEditable,
    contentEditable: false,
    onBlur: (event: React.FocusEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      if (!updateAttr(props, props.attrName, event.currentTarget.value)) event.currentTarget.value = value;
    },
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
      key: value,
      defaultValue: String(value),
      disabled: !props.editor.isEditable,
      min: props.min,
      max: props.max,
      step: props.step,
      onBlur: (event: React.FocusEvent<HTMLInputElement>) => {
        const next = Number(event.currentTarget.value);
        if (!event.currentTarget.value.trim() || !Number.isFinite(next) || !updateAttr(props, props.attrName, next)) event.currentTarget.value = String(value);
      },
      onKeyDown: (event: React.KeyboardEvent<HTMLInputElement>) => event.stopPropagation(),
    }),
  );
}

function BlockShell(props: ReactNodeViewProps & { title: string; summary?: string; className?: string; children?: React.ReactNode; content?: boolean }) {
  return React.createElement(NodeViewWrapper, {
    as: "section",
    className: ["ast-editable-block", props.className].filter(Boolean).join(" "),
    "data-editor-block-type": props.node.type.name,
    "data-id": props.node.attrs.id,
    "data-column-header": props.node.attrs.hasColumnHeader || undefined,
    "data-row-header": props.node.attrs.hasRowHeader || undefined,
  },
    props.node.isAtom ? React.createElement("div", {
      className: "ast-editor-preview", contentEditable: false,
      onClickCapture: (event: React.MouseEvent) => {
        if ((event.target as Element).closest("a,button")) { event.preventDefault(); event.stopPropagation(); }
      },
    }, React.createElement(BlockView, { block: toAst({ type: "doc", content: [props.node.toJSON()] }).children[0] })) : null,
    props.editor.isEditable && React.Children.count(props.children) > 0 ? React.createElement("details", null,
      React.createElement("summary", { className: "ast-editable-block__summary-trigger", contentEditable: false },
        React.createElement("div", { className: "ast-editable-block__header" },
          React.createElement("span", { className: "ast-editable-block__title" }, props.title),
          props.summary ? React.createElement("span", { className: "ast-editable-block__summary" }, props.summary) : null,
          React.createElement("span", { className: "ast-editable-block__hint" }, "点击编辑"),
        ),
      ),
      React.createElement("div", { className: "ast-editable-block__grid", contentEditable: false }, props.children),
    ) : null,
    props.node.type.name === "astTable" && props.node.attrs.caption ? React.createElement("div", { className: "text-sm text-muted-foreground", contentEditable: false }, props.node.attrs.caption) : null,
    props.content ? React.createElement(NodeViewContent, { className: "ast-editable-block__content" }) : null,
  );
}

function LeafView(props: ReactNodeViewProps & { title: string; summary?: string; className?: string; fields: React.ReactNode[] }) {
  return React.createElement(BlockShell, { ...props, title: props.title, summary: props.summary, className: props.className }, ...props.fields);
}

function PreformattedView(props: ReactNodeViewProps) {
  return React.createElement(LeafView, { ...props, title: "等宽文本", summary: textValue(props.node.attrs.kind), className: "ast-preformatted", fields: [
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
    tabIndex: 0,
    "aria-label": `${props.label}：${text}`,
  },
    React.createElement("span", { className: "ast-inline-editor__preview" }, text,
      props.editor.isEditable ? React.createElement("span", { className: "ast-inline-editor__panel" },
        props.fields.map((field) => React.createElement(TextInput, { ...props, key: field.attr, attrName: field.attr, label: field.label, type: field.type })),
      ) : null,
    ),
  );
}

function handleBlockKeyDown(view: EditorView, event: KeyboardEvent): boolean {
  if (!["Enter", "Backspace"].includes(event.key) || event.metaKey || event.ctrlKey || event.altKey ||
      event.isComposing || view.composing || !view.editable ||
      (event.target as Element).closest("button,input,textarea,select") ||
      SuggestionPluginKey.getState(view.state)?.active) return false;
  const { selection } = view.state;
  const { $from, $to } = selection;
  if (!(selection instanceof TextSelection) || $from.depth < 2 || !$from.sameParent($to) ||
      $from.parent.type.name !== "paragraph") return false;
  const block = $from.node(-1);
  const todo = block.type.name === "todoBlock";
  const item = todo || block.type.name === "toggleBlock";
  const summary = $from.index(-1) === 0;
  if (event.shiftKey) {
    if (event.key !== "Enter" || !summary || !(block.type.name === "toggleBlock" ||
        (block.type.name === "sectionHeading" && block.attrs.isToggleable))) return false;
    const tr = view.state.tr.deleteSelection();
    const cursor = tr.selection.$from;
    const remaining = cursor.parent.content.cut(cursor.parentOffset);
    tr.delete(cursor.pos, cursor.end());
    const bodyPos = tr.selection.$from.after();
    const body = tr.doc.nodeAt(bodyPos);
    if (body?.type.name === "paragraph" && body.content.size === 0) {
      tr.insert(bodyPos + 1, remaining);
    } else {
      tr.insert(bodyPos, view.state.schema.nodes.paragraph.create(null, remaining));
    }
    tr.setSelection(TextSelection.create(tr.doc, bodyPos + 1));
    tr.ensureMarks(view.state.storedMarks ?? $from.marks());
    const dom = view.nodeDOM($from.before(-1));
    if (dom instanceof Element) dom.querySelector<HTMLButtonElement>(':scope > .ast-outline > button[aria-expanded="false"]')?.click();
    view.dispatch(tr.scrollIntoView());
    return true;
  }
  if (event.key === "Backspace") {
    if (!["todoBlock", "toggleBlock", "sectionHeading", "callout"].includes(block.type.name) ||
        !summary || !selection.empty || $from.parentOffset !== 0) return false;
    const paragraph = $from.parent.type.create({ ...$from.parent.attrs, id: block.attrs.id }, $from.parent.content);
    const start = $from.before(-1);
    const tr = view.state.tr.replaceWith(start, $from.after(-1), block.content.replaceChild(0, paragraph));
    tr.setSelection(TextSelection.create(tr.doc, start + 1));
    view.dispatch(tr.scrollIntoView());
    return true;
  }
  if (block.type.name === "callout") {
    if (!selection.empty || $from.parent.content.size !== 0 || $from.index(-1) !== block.childCount - 1) return false;
    const tr = view.state.tr;
    const after = $from.after(-1);
    if (block.childCount === 1) tr.replaceWith($from.before(-1), after, $from.parent);
    else tr.delete($from.before(), $from.after()).insert(tr.mapping.map(after), $from.parent);
    tr.setSelection(TextSelection.create(tr.doc, tr.mapping.map(after) - 1));
    view.dispatch(tr.scrollIntoView());
    return true;
  }
  if (!item) {
    if (block.type.name !== "sectionHeading") return false;
    if (summary) {
      const dom = view.nodeDOM($from.before(-1));
      if (!(dom instanceof Element) || !dom.querySelector(':scope > .ast-outline[data-open="false"]')) return false;
    } else if (!selection.empty || $from.parent.content.size !== 0 || $from.index(-1) !== block.childCount - 1) {
      return false;
    }
  }

  const tr = view.state.tr.deleteSelection();
  const cursor = tr.selection.$from;
  if (!item && !summary) {
    const after = cursor.after(-1);
    tr.delete(cursor.before(), cursor.after());
    const outside = tr.mapping.map(after);
    tr.insert(outside, cursor.parent).setSelection(TextSelection.create(tr.doc, outside + 1));
  } else {
    // Keep existing children with their block, including children hidden by collapse.
    const paragraph = view.state.schema.nodes.paragraph.create(null, cursor.parent.content.cut(cursor.parentOffset));
    tr.delete(cursor.pos, cursor.end());
    const after = tr.selection.$from.after(-1);
    const sibling = item ? block.type.create({ ...block.attrs, id: null, ...(todo ? { checked: false } : {}) }, paragraph) : paragraph;
    tr.insert(after, sibling).setSelection(TextSelection.create(tr.doc, after + (item ? 2 : 1)));
    tr.ensureMarks(view.state.storedMarks ?? $from.marks());
  }
  view.dispatch(tr.scrollIntoView());
  return true;
}

const BlockKeys = Extension.create({
  name: "anbyBlockKeys",
  addProseMirrorPlugins: () => [new Plugin({ props: { handleKeyDown: handleBlockKeyDown } })],
});

function OutlineView(props: ReactNodeViewProps) {
  const [open, setOpen] = useState(true);
  const heading = props.node.type.name === "sectionHeading";
  const toggleable = !heading || props.node.attrs.isToggleable;
  const toggle = () => {
    const pos = props.getPos();
    const { selection } = props.editor.state;
    if (open && pos !== undefined && props.node.firstChild) {
      const summaryEnd = pos + props.node.firstChild.nodeSize;
      // Move a body selection before hiding it so the next key edits visible content.
      if (selection.from > pos && selection.to > summaryEnd && selection.to < pos + props.node.nodeSize) {
        props.editor.commands.setTextSelection(summaryEnd);
      }
    }
    setOpen(!open);
  };
  return React.createElement(NodeViewWrapper, {
    className: `ast-outline${heading ? " ast-outline--heading" : ""}`,
    "data-level": props.node.attrs.level,
    "data-open": open,
    "data-id": props.node.attrs.id,
    "data-editor-block-type": props.node.type.name,
  },
    toggleable ? React.createElement("button", {
      type: "button", className: "ast-outline__toggle", contentEditable: false,
      "aria-label": open ? "折叠内容" : "展开内容", "aria-expanded": open,
      onMouseDown: (event: React.MouseEvent) => event.preventDefault(), onClick: toggle,
    }, React.createElement(ChevronRight, { size: 16, style: { transform: open ? "rotate(90deg)" : undefined } })) : null,
    React.createElement(NodeViewContent, { className: "ast-outline__content" }),
  );
}

function TodoView(props: ReactNodeViewProps) {
  return React.createElement(NodeViewWrapper, { className: "ast-todo", "data-checked": props.node.attrs.checked, "data-id": props.node.attrs.id },
    React.createElement("input", { type: "checkbox", contentEditable: false, "aria-label": "完成事项", checked: props.node.attrs.checked, disabled: !props.editor.isEditable, onChange: (event: React.ChangeEvent<HTMLInputElement>) => updateAttr(props, "checked", event.target.checked) }),
    React.createElement(NodeViewContent, { className: "ast-todo__content" }),
  );
}

function BreadcrumbView(props: ReactNodeViewProps) {
  const editable = useEditorState({ editor: props.editor, selector: ({ editor }) => editor.isEditable });
  return React.createElement(NodeViewWrapper, { contentEditable: false, "data-editor-block-type": "breadcrumb", "data-id": props.node.attrs.id },
    React.createElement(BreadcrumbFields, { value: JSON.parse(props.node.attrs.itemsJson), editable, onChange: items => {
      props.editor.view.dispatch(closeHistory(props.editor.state.tr));
      return updateAttr(props, "itemsJson", JSON.stringify(items));
    } }),
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
      appendTransaction: (transactions, oldState, newState) => {
        const tr = newState.tr;
        let changed = false;
        const mapping = new Mapping();
        for (const transaction of transactions) mapping.appendMapping(transaction.mapping);
        const owners = new Map<string, number>();
        oldState.doc.descendants((node, pos) => {
          if (!node.attrs.id) return;
          const mapped = mapping.mapResult(pos);
          if (!mapped.deleted && newState.doc.nodeAt(mapped.pos)?.attrs.id === node.attrs.id) owners.set(node.attrs.id, mapped.pos);
        });
        const seen = new Set<string>();
        newState.doc.descendants((node, pos, parent, index) => {
          if (!blockTypesWithId.includes(node.type.name)) return;
          if (index === 0 && node.type.name === "paragraph" && parent && ["todoBlock", "toggleBlock", "sectionHeading"].includes(parent.type.name)) return;
          let id = node.attrs.id;
          if (!id || seen.has(id) || (owners.has(id) && owners.get(id) !== pos)) {
            id = newBlockId();
            tr.setNodeMarkup(pos, undefined, { ...node.attrs, id });
            changed = true;
          }
          seen.add(id);
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
    isolating: name !== "todoBlock",
    addAttributes: () => idAttrs(attrs),
    parseHTML: () => [{ tag: `[data-node-type="${name}"]` }],
    renderHTML: ({ HTMLAttributes }) => ["div", mergeAttributes(HTMLAttributes, { "data-node-type": name }), 0],
    addNodeView: () => ReactNodeViewRenderer(view),
  });
}

const InlineCode = Mark.create({
  name: "inlineCode",
  code: true,
  excludes: "",
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
    marks: "",
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
  BlockKeys,
  InlineCode,
  TextColor,
  BackgroundColor,
  containerNode("callout", "block*", (props) => React.createElement(CalloutView, { ...props, onUpdateAttrs: attrs => updateAttrs(props, attrs) }), { kind: { default: "info" }, icon: { default: "" }, color: { default: "default" } }),
  atomNode("preformatted", { kind: { default: "plain" }, caption: { default: "" }, text: { default: "" } }, PreformattedView),
  atomNode("equation", { expression: { default: "" }, caption: { default: "" } }, EquationView),
  containerNode("todoBlock", "paragraph block*", TodoView, { checked: { default: false } }),
  containerNode("toggleBlock", "paragraph block*", OutlineView, { hasChildren: { default: true } }),
  containerNode("sectionHeading", "paragraph block*", OutlineView, { level: { default: 1 }, isToggleable: { default: null }, hasChildren: { default: false } }),
  atomNode("breadcrumb", { itemsJson: { default: "[]" } }, BreadcrumbView),
  containerNode("columnList", "column{2,5}", ColumnsView),
  containerNode("column", "block*", ColumnView, { widthRatio: { default: 1 } }),
  atomNode("pageButton", { label: { default: "运行操作" }, confirmation: { default: "" }, actionsJson: { default: "[]" } }, (props) => React.createElement(EditableAtomView, { ...props, onUpdateAttrs: attrs => updateAttrs(props, attrs) })),
  atomNode("htmlBlock", { html: { default: "" }, height: { default: 240 }, caption: { default: "" } }, (props) => React.createElement(EditableAtomView, { ...props, onUpdateAttrs: attrs => updateAttrs(props, attrs) })),
  containerNode("astTable", "astTableRow+", (props) => React.createElement(TableView, { ...props, onUpdateAttrs: (attrs) => updateAttrs(props, attrs) }), { caption: { default: "" }, hasColumnHeader: { default: false }, hasRowHeader: { default: false } }),
  containerNode("astTableRow", "astTableCell+", () => React.createElement(NodeViewWrapper, { as: "div", className: "ast-table-row" }, React.createElement(NodeViewContent, null))),
  containerNode("astTableCell", "block*", (props) => React.createElement(NodeViewWrapper, { as: "div", className: "ast-table-cell", "data-id": props.node.attrs.id }, React.createElement(NodeViewContent, null))),
  atomNode("componentBlock", { componentId: { default: "" }, componentVersion: { default: 1 }, entityId: { default: "" }, displayConfig: { default: "{}" } }, ComponentBlockView),
  atomNode("imageAsset", { assetRevisionId: { default: "" }, altText: { default: "" }, caption: { default: "" } }, (props) => React.createElement(EditableAtomView, { ...props, onUpdateAttrs: attrs => updateAttrs(props, attrs) })),
  atomNode("videoAsset", { assetRevisionId: { default: "" }, posterAssetRevisionId: { default: "" }, title: { default: "" }, caption: { default: "" } }, (props) => React.createElement(EditableAtomView, { ...props, onUpdateAttrs: attrs => updateAttrs(props, attrs) })),
  atomNode("audioAsset", { assetRevisionId: { default: "" }, title: { default: "" }, artist: { default: "" }, album: { default: "" }, durationSeconds: { default: 0 }, caption: { default: "" } }, (props) => React.createElement(EditableAtomView, { ...props, onUpdateAttrs: attrs => updateAttrs(props, attrs) })),
  atomNode("fileAsset", { assetRevisionId: { default: "" }, name: { default: "" }, caption: { default: "" } }, (props) => React.createElement(EditableAtomView, { ...props, onUpdateAttrs: attrs => updateAttrs(props, attrs) })),
  atomNode("pdfAsset", { assetRevisionId: { default: "" }, title: { default: "" }, startPage: { default: 1 }, height: { default: 640 }, caption: { default: "" } }, (props) => React.createElement(EditableAtomView, { ...props, onUpdateAttrs: attrs => updateAttrs(props, attrs) })),
  atomNode("bookmark", { url: { default: "" }, title: { default: "" }, description: { default: "" }, siteName: { default: "" }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "书签", summaryAttr: "title", className: "ast-bookmark", fields: [{ attr: "url", label: "链接地址" }, { attr: "title", label: "标题" }, { attr: "siteName", label: "站点" }, { attr: "description", label: "描述", multiline: true }, { attr: "caption", label: "说明" }] })),
  atomNode("linkPreview", { url: { default: "" }, title: { default: "" }, description: { default: "" }, siteName: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "链接预览", summaryAttr: "title", className: "ast-link-preview", fields: [{ attr: "url", label: "链接地址" }, { attr: "title", label: "标题" }, { attr: "siteName", label: "站点" }, { attr: "description", label: "描述", multiline: true }] })),
  atomNode("embedBlock", { url: { default: "" }, title: { default: "" }, provider: { default: "none" }, caption: { default: "" } }, (props) => React.createElement(SimpleAttrsView, { ...props, title: "嵌入内容", summaryAttr: "title", className: "ast-embed", fields: [{ attr: "url", label: "链接地址" }, { attr: "provider", label: "提供方" }, { attr: "title", label: "标题" }, { attr: "caption", label: "说明" }] })),
  inlineAtomNode("pageReference", { targetPageId: { default: "" }, targetHeadingBlockId: { default: "" }, displayText: { default: "" }, resolutionStatus: { default: "" }, targetNamespace: { default: "" }, normalizedTitle: { default: "" }, expectedEntityType: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "页面", className: "ast-page-ref", textAttrs: ["displayText", "normalizedTitle"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "targetPageId", label: "页面" }, { attr: "normalizedTitle", label: "待创建页面标题" }] })),
  inlineAtomNode("pageAnchorReference", { targetPageId: { default: "" }, targetHeadingBlockId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "章节链接", className: "ast-page-anchor-ref", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "targetPageId", label: "页面" }] })),
  inlineAtomNode("entityReference", { entityId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "实体", className: "ast-entity-ref", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "entityId", label: "实体" }] })),
  inlineAtomNode("claimReference", { claimId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "事实", className: "ast-claim-ref", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "claimId", label: "事实" }] })),
  inlineAtomNode("citationReference", { citationId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "引用", className: "ast-citation-ref", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示文本" }, { attr: "citationId", label: "引文" }] })),
  inlineAtomNode("mathInline", { expression: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "公式", className: "ast-math", textAttrs: ["expression"], fields: [{ attr: "expression", label: "表达式" }] })),
  inlineAtomNode("mention", { actorId: { default: "" }, displayText: { default: "" } }, (props) => React.createElement(InlineAtomView, { ...props, label: "提及", className: "ast-mention", textAttrs: ["displayText"], fields: [{ attr: "displayText", label: "显示名称" }, { attr: "actorId", label: "人员" }] })),
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
