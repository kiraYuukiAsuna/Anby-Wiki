// 受控 Tiptap Block 编辑器包装组件。
//
// 业务代码只允许通过这个组件使用编辑器：输入初始 AST v1 Document，编辑时经
// onChange 回传最新 AST。AdapterState 随组件实例存活，保证会话内 Block ID 稳定。
"use client";

import { useEffect, useMemo, useRef, useState } from "react";

import type { Editor, JSONContent } from "@tiptap/core";
import { Extension } from "@tiptap/core";
import { EditorContent, useEditor } from "@tiptap/react";
import { Suggestion, type SuggestionKeyDownProps, type SuggestionProps } from "@tiptap/suggestion";
import { TextSelection } from "@tiptap/pm/state";
import { toast } from "sonner";

import { ExternalLinkEditor } from "@/components/editor/external-link-editor";
import { ExternalContentInsertDialog } from "@/components/editor/external-content-insert-dialog";
import { HTMLBlockInsertDialog } from "@/components/editor/html-block-insert-dialog";
import { InlineMentionInsertDialog } from "@/components/editor/inline-mention-insert-dialog";
import { MediaBlockInsertDialog } from "@/components/editor/media-block-insert-dialog";
import { ComponentInsertDialog } from "@/components/editor/component-insert-button";
import { PageReferencePicker } from "@/components/editor/page-reference-picker";
import { PageButtonInsertDialog } from "@/components/editor/page-button-insert-dialog";
import { clientUUID } from "@/lib/client-uuid";
import type { Block, Document } from "@/lib/ast/schema";
import { fromAst } from "@/lib/editor/fromAst";
import { createAdapterState, type AdapterState } from "@/lib/editor/ids";
import { PresenceExtension, editorPosition, resolveEditorPosition, setPresencePeers, type EditorPeer } from "@/lib/editor/presence";
import type { EditorCursor } from "@/lib/collaboration/presence";
import {
  insertDateMention,
  insertExternalLink,
  insertPersonMention,
  insertResolvedPageReference,
  insertUnresolvedPageReference,
} from "@/lib/editor/references";
import { editorExtensions, type PMDocument } from "@/lib/editor/schema";
import { toAst } from "@/lib/editor/toAst";
import type { LinkPreviewMetadata } from "../../../../contracts/generated/typescript";

export interface BlockEditorProps {
  initialAst: Document;
  remoteAst?: Document;
  remoteCursor?: EditorCursor;
  onChange?: (ast: Document) => void;
  onPresenceChange?: (cursor: EditorCursor) => void;
  peers?: EditorPeer[];
  editable?: boolean;
  pageId?: string;
  resolveLinkPreview?: (url: string) => Promise<LinkPreviewMetadata>;
}

type SlashItem = {
  title: string;
  subtext: string;
  aliases: string[];
  group: string;
  icon: string;
  run: (editor: Editor) => void;
};

function itemMatches(item: SlashItem, query: string): boolean {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  return [item.title, item.subtext, ...item.aliases].some((value) =>
    value.toLowerCase().includes(normalized),
  );
}

function renderSlashItems(
  root: HTMLElement,
  props: SuggestionProps<SlashItem, SlashItem>,
  selectedIndex: number,
): void {
  root.innerHTML = "";
  root.className = "anby-slash-menu";
  const group = document.createElement("div");
  group.className = "anby-slash-menu__group";
  group.textContent = "Anby";
  root.append(group);
  for (let index = 0; index < props.items.length; index += 1) {
    const item = props.items[index];
    const button = document.createElement("button");
    button.type = "button";
    button.className = index === selectedIndex
      ? "anby-slash-menu__item anby-slash-menu__item--active"
      : "anby-slash-menu__item";
    button.addEventListener("mousedown", (event) => event.preventDefault());
    button.addEventListener("click", () => props.command(item));
    const icon = document.createElement("span");
    icon.className = "anby-slash-menu__icon";
    icon.textContent = item.icon;
    const text = document.createElement("span");
    const title = document.createElement("strong");
    title.textContent = item.title;
    const subtext = document.createElement("small");
    subtext.textContent = item.subtext;
    text.append(title, subtext);
    button.append(icon, text);
    root.append(button);
  }
}

function slashMenuExtension(items: SlashItem[]) {
  return Extension.create({
    name: "anbySlashMenu",
    addProseMirrorPlugins() {
      return [Suggestion<SlashItem, SlashItem>({
        editor: this.editor,
        char: "/",
        items: ({ query }) => items.filter((item) => itemMatches(item, query)),
        command: ({ editor, range, props }) => {
          editor.chain().focus().deleteRange(range).run();
          props.run(editor);
        },
        render: () => {
          let root: HTMLElement | null = null;
          let unmount: (() => void) | undefined;
          let selectedIndex = 0;
          let current: SuggestionProps<SlashItem, SlashItem> | null = null;
          const update = (props: SuggestionProps<SlashItem, SlashItem>) => {
            current = props;
            selectedIndex = Math.min(selectedIndex, Math.max(props.items.length - 1, 0));
            if (root) renderSlashItems(root, props, selectedIndex);
          };
          return {
            onStart: (props) => {
              selectedIndex = 0;
              root = document.createElement("div");
              update(props);
              unmount = props.mount(root);
            },
            onUpdate: update,
            onKeyDown: ({ event }: SuggestionKeyDownProps) => {
              if (!current || current.items.length === 0) return false;
              if (event.key === "ArrowDown") {
                selectedIndex = (selectedIndex + 1) % current.items.length;
                if (root) renderSlashItems(root, current, selectedIndex);
                return true;
              }
              if (event.key === "ArrowUp") {
                selectedIndex = (selectedIndex - 1 + current.items.length) % current.items.length;
                if (root) renderSlashItems(root, current, selectedIndex);
                return true;
              }
              if (event.key === "Enter") {
                current.command(current.items[selectedIndex]);
                return true;
              }
              return false;
            },
            onExit: () => {
              unmount?.();
              root = null;
              current = null;
            },
          };
        },
      })];
    },
  });
}

function singleMappedBlock(block: Block, state: AdapterState): JSONContent | null {
  const doc = fromAst({ type: "document", schema_version: 1, children: [block] }, state);
  return doc.content?.[0] ?? null;
}

export function BlockEditor({
  initialAst,
  remoteAst,
  remoteCursor,
  onChange,
  onPresenceChange,
  peers,
  editable = true,
  pageId,
  resolveLinkPreview,
}: BlockEditorProps) {
  const [state] = useState<AdapterState>(() => createAdapterState());
  const applyingRemote = useRef(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  const [buttonOpen, setButtonOpen] = useState(false);
  const [htmlOpen, setHTMLOpen] = useState(false);
  const [mediaOpen, setMediaOpen] = useState(false);
  const [externalContentOpen, setExternalContentOpen] = useState(false);
  const [mentionOpen, setMentionOpen] = useState(false);
  const [componentOpen, setComponentOpen] = useState(false);
  const [linkDefaultText, setLinkDefaultText] = useState("");

  const initialContent = useMemo(() => fromAst(initialAst, state), [initialAst, state]);

  const insertTemplate = (editor: Editor, block: JSONContent) => {
    editor.chain().focus().insertContent(block).run();
  };

  const insertAstBlock = (editor: Editor, block: Block) => {
    const mapped = singleMappedBlock(block, state);
    if (mapped) insertTemplate(editor, mapped);
  };

  const slashItems: SlashItem[] = useMemo(() => [
    { title: "页面引用", subtext: "插入站内页面引用", aliases: ["page", "ref", "页面", "引用"], group: "Anby", icon: "↗", run: () => setPickerOpen(true) },
    { title: "提及", subtext: "插入人员或日期提及", aliases: ["mention", "date", "提及", "日期"], group: "Anby", icon: "@", run: () => setMentionOpen(true) },
    { title: "外链", subtext: "插入外部链接", aliases: ["link", "url", "外链"], group: "Anby", icon: "🔗", run: (editor: Editor) => { setLinkDefaultText(editor.state.doc.textBetween(editor.state.selection.from, editor.state.selection.to)); setLinkOpen(true); } },
    { title: "语义表格", subtext: "插入可嵌套内容的 AST 表格", aliases: ["table", "表格"], group: "Anby", icon: "▦", run: (editor: Editor) => insertTemplate(editor, { type: "astTable", content: [{ type: "astTableRow", content: [{ type: "astTableCell", content: [{ type: "paragraph", content: [{ type: "text", text: "单元格 1" }] }] }, { type: "astTableCell", content: [{ type: "paragraph", content: [{ type: "text", text: "单元格 2" }] }] }] }] }) },
    { title: "页面路径", subtext: "插入 Breadcrumb", aliases: ["breadcrumb", "path", "路径"], group: "Anby", icon: "⌁", run: (editor: Editor) => insertTemplate(editor, { type: "breadcrumb", attrs: { itemsJson: JSON.stringify([{ display_text: "当前页面" }]) } }) },
    { title: "分栏", subtext: "插入双栏布局", aliases: ["columns", "layout", "分栏"], group: "Anby", icon: "▥", run: (editor: Editor) => insertTemplate(editor, { type: "columnList", content: [{ type: "column", attrs: { widthRatio: 1 }, content: [{ type: "paragraph", content: [{ type: "text", text: "左列" }] }] }, { type: "column", attrs: { widthRatio: 1 }, content: [{ type: "paragraph", content: [{ type: "text", text: "右列" }] }] }] }) },
    { title: "页面按钮", subtext: "插入声明式动作按钮", aliases: ["button", "action", "按钮"], group: "Anby", icon: "◉", run: () => setButtonOpen(true) },
    { title: "HTML 块", subtext: "插入隔离 HTML 预览", aliases: ["html", "iframe"], group: "Anby", icon: "<>", run: () => setHTMLOpen(true) },
    { title: "媒体", subtext: "插入图片、视频、音频、文件或 PDF", aliases: ["media", "asset", "image", "pdf", "媒体"], group: "Anby", icon: "▣", run: () => setMediaOpen(true) },
    { title: "外部内容", subtext: "插入书签、链接预览或 Embed", aliases: ["bookmark", "embed", "preview", "外部内容"], group: "Anby", icon: "▤", run: () => setExternalContentOpen(true) },
    { title: "组件", subtext: "插入已发布信息组件", aliases: ["component", "infobox", "组件"], group: "Anby", icon: "▧", run: () => setComponentOpen(true) },
  ], []);

  const presenceExtension = useMemo(
    () => PresenceExtension.configure({ adapter: state }),
    [state],
  );

  const editor = useEditor({
    extensions: [
      ...editorExtensions,
      presenceExtension,
      slashMenuExtension(slashItems),
    ],
    content: initialContent,
    editable,
    immediatelyRender: false,
    onUpdate: ({ editor }) => {
      if (applyingRemote.current) return;
      onChange?.(toAst(editor.getJSON() as PMDocument, state));
    },
    onSelectionUpdate: ({ editor }) => {
      if (applyingRemote.current) return;
      try {
        const selection = editor.state.selection;
        const anchor = editorPosition(selection.$anchor, state);
        const head = editorPosition(selection.$head, state);
        if (anchor && head) onPresenceChange?.({ anchor, head });
      } catch {
        // Some atomic node selections do not expose a text cursor.
      }
    },
  }, [state]);

  useEffect(() => {
    editor?.setEditable(editable);
  }, [editor, editable]);

  useEffect(() => {
    setPresencePeers(state, peers ?? []);
    if (editor && !editor.isDestroyed) editor.view.dispatch(editor.state.tr);
  }, [editor, peers, state]);

  useEffect(() => {
    if (!editor || !remoteAst) return;
    const current = toAst(editor.getJSON() as PMDocument, state);
    if (JSON.stringify(current) === JSON.stringify(remoteAst)) return;
    const selection = editor.state.selection;
    const anchor = remoteCursor?.anchor ?? editorPosition(selection.$anchor, state);
    const head = remoteCursor?.head ?? editorPosition(selection.$head, state);
    applyingRemote.current = true;
    try {
      editor.commands.setContent(fromAst(remoteAst, state), { emitUpdate: false });
      if (anchor && head) {
        const from = resolveEditorPosition(editor.state.doc, anchor, state);
        const to = resolveEditorPosition(editor.state.doc, head, state);
        if (from !== null && to !== null) {
          editor.view.dispatch(editor.state.tr.setSelection(TextSelection.create(editor.state.doc, from, to)));
        }
      }
    } finally {
      applyingRemote.current = false;
    }
  }, [editor, remoteAst, remoteCursor, state]);

  useEffect(() => {
    if (!editor || !editable) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== "k") return;
      event.preventDefault();
      if (event.shiftKey) {
        setLinkDefaultText(editor.state.doc.textBetween(editor.state.selection.from, editor.state.selection.to));
        setLinkOpen(true);
      } else {
        setPickerOpen(true);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [editor, editable]);

  if (!editor) return <div className="min-h-80 animate-pulse bg-muted" aria-label="编辑器加载中" />;

  return (
    <div className="min-w-0" data-block-editor>
      <EditorContent editor={editor} className="anby-tiptap-editor" />
      {editable ? (
        <>
          <PageReferencePicker
            open={pickerOpen}
            onOpenChange={setPickerOpen}
            onInsertResolved={(targetPageId, displayText) => insertResolvedPageReference(editor, targetPageId, displayText)}
            onInsertUnresolved={(rawTitle) => insertUnresolvedPageReference(editor, rawTitle)}
          />
          <ExternalLinkEditor
            open={linkOpen}
            onOpenChange={setLinkOpen}
            defaultDisplayText={linkDefaultText}
            onSubmit={(url, displayText) => insertExternalLink(editor, url, displayText)}
          />
          <PageButtonInsertDialog
            open={buttonOpen}
            onOpenChange={setButtonOpen}
            currentPageId={pageId}
            onInsert={(value) => insertTemplate(editor, { type: "pageButton", attrs: { label: value.label, confirmation: value.confirmation ?? "", actionsJson: JSON.stringify(value.actions) } })}
          />
          <HTMLBlockInsertDialog
            open={htmlOpen}
            onOpenChange={setHTMLOpen}
            onInsert={(value) => insertTemplate(editor, { type: "htmlBlock", attrs: { html: value.html, height: value.height, caption: value.caption ?? "" } })}
          />
          <MediaBlockInsertDialog open={mediaOpen} onOpenChange={setMediaOpen} onInsert={(block) => insertAstBlock(editor, block)} />
          <ExternalContentInsertDialog
            open={externalContentOpen}
            onOpenChange={setExternalContentOpen}
            resolvePreview={resolveLinkPreview}
            onInsert={(block) => insertAstBlock(editor, block)}
          />
          <InlineMentionInsertDialog
            open={mentionOpen}
            onOpenChange={setMentionOpen}
            onInsertPerson={(actorId, displayText) => insertPersonMention(editor, actorId, displayText)}
            onInsertDate={(date, endDate, displayText) => insertDateMention(editor, date, endDate, displayText)}
          />
          <ComponentInsertDialog
            open={componentOpen}
            onOpenChange={setComponentOpen}
            pageId={pageId}
            onInsert={(selection) => {
              insertAstBlock(editor, {
                type: "component",
                id: clientUUID(),
                component_id: selection.componentId,
                component_version: selection.componentVersion,
                entity_id: selection.entityId,
                display_config: selection.displayConfig,
              });
              toast.success("已插入组件");
            }}
          />
        </>
      ) : null}
    </div>
  );
}
