// 受控 Block 编辑器包装组件（M2-T02，供 M2-T03 编辑页面使用）。
//
// 业务代码只允许通过这个组件使用 BlockNote（ADR-0005）：
// 输入初始 AST v1 Document，编辑时经 onChange 回传最新 AST。
// AdapterState 随组件实例存活，保证会话内 Block ID 稳定。
//
// M2-T04 引用与自定义块插入：⌘K / ⇧⌘K 保留为快捷入口；完整插入入口通过
// BlockNote slash menu 原地提供。插入/更新引用全部经 lib/editor/references.ts helper。
"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { TextSelection } from "@tiptap/pm/state";

import { BlockNoteView } from "@blocknote/mantine";
import { filterSuggestionItems, insertOrUpdateBlockForSlashMenu } from "@blocknote/core/extensions";
import {
  getDefaultReactSlashMenuItems,
  SuggestionMenuController,
  useCreateBlockNote,
  type DefaultReactSuggestionItem,
} from "@blocknote/react";
import {
  AtSign,
  CodeXml,
  Columns3,
  FileAudio,
  FileText,
  Link2,
  MousePointerClick,
  Navigation,
  PanelsTopLeft,
  Table2,
} from "lucide-react";
import { toast } from "sonner";

import "@blocknote/core/style.css";
import "@blocknote/mantine/style.css";

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
import { PresenceExtension, editorPosition, resolveEditorPosition, type EditorPeer } from "@/lib/editor/presence";
import type { EditorCursor } from "@/lib/collaboration/presence";
import {
  insertDateMention,
  insertExternalLink,
  insertPersonMention,
  insertResolvedPageReference,
  insertUnresolvedPageReference,
} from "@/lib/editor/references";
import { editorSchema, type BNBlock } from "@/lib/editor/schema";
import { toAst } from "@/lib/editor/toAst";
import type { LinkPreviewMetadata } from "../../../../contracts/generated/typescript";

export interface BlockEditorProps {
  /** 初始文档（AST v1）。组件非受控于该 prop：后续变更不会重置编辑器。 */
  initialAst: Document;
  /** 远端协作内容，应用时保留本地字符选区。 */
  remoteAst?: Document;
  remoteCursor?: EditorCursor;
  /** 每次编辑后回传序列化后的 AST v1 Document。 */
  onChange?: (ast: Document) => void;
  /** 稳定 AST Block ID + UTF-16 字符选区，不发送正文。 */
  onPresenceChange?: (cursor: EditorCursor) => void;
  peers?: EditorPeer[];
  /** 传给 BlockNoteView 的 editable。 */
  editable?: boolean;
  /** 既有页面 ID；用于发现主 Entity 并插入可信信息框。 */
  pageId?: string;
  /** 页面层注入的第三方预览解析器，避免编辑器直接持有服务端数据边界。 */
  resolveLinkPreview?: (url: string) => Promise<LinkPreviewMetadata>;
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
  // AdapterState 随组件实例存活（lazy useState），保证会话内 Block ID 稳定。
  const [state] = useState<AdapterState>(() => createAdapterState());
  const applyingRemote = useRef(false);

  // initialContent 只在编辑器创建时读取一次；空文档传 undefined，
  // BlockNote 会默认创建一个空段落（已在 ADR-0005 验证记录中说明）。
  const initialContent = useMemo(() => {
    const blocks = fromAst(initialAst, state);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return blocks.length > 0 ? (blocks as any) : undefined;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const editor = useCreateBlockNote({
    schema: editorSchema,
    initialContent,
    extensions: [PresenceExtension({ adapter: state })],
  });

  useEffect(() => {
    editor.getExtension(PresenceExtension)?.setPeers(peers ?? []);
  }, [editor, peers]);

  useEffect(() => {
    if (!remoteAst) return;
    const current = toAst(editor.document as unknown as BNBlock[], state);
    if (JSON.stringify(current) === JSON.stringify(remoteAst)) return;
    const selection = editor.prosemirrorState.selection;
    const anchor = remoteCursor?.anchor ?? editorPosition(selection.$anchor, state);
    const head = remoteCursor?.head ?? editorPosition(selection.$head, state);
    applyingRemote.current = true;
    try {
      const blocks = fromAst(remoteAst, state);
      editor.replaceBlocks(editor.document, blocks as never);
      if (anchor && head) {
        editor.transact((transaction) => {
          const from = resolveEditorPosition(transaction.doc, anchor, state);
          const to = resolveEditorPosition(transaction.doc, head, state);
          if (from !== null && to !== null) transaction.setSelection(TextSelection.create(transaction.doc, from, to));
        });
      }
    } finally {
      applyingRemote.current = false;
    }
    const restored = editor.prosemirrorState.selection;
    const restoredAnchor = editorPosition(restored.$anchor, state);
    const restoredHead = editorPosition(restored.$head, state);
    if (restoredAnchor && restoredHead) onPresenceChange?.({ anchor: restoredAnchor, head: restoredHead });
  }, [editor, remoteAst, remoteCursor, state, onPresenceChange]);

  // ---- M2-T04 引用编辑 UI 状态 ----
  const [pickerOpen, setPickerOpen] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  const [buttonOpen, setButtonOpen] = useState(false);
  const [htmlOpen, setHTMLOpen] = useState(false);
  const [mediaOpen, setMediaOpen] = useState(false);
  const [externalContentOpen, setExternalContentOpen] = useState(false);
  const [mentionOpen, setMentionOpen] = useState(false);
  const [componentOpen, setComponentOpen] = useState(false);
  // 打开外链编辑器时的默认显示文本（选中文本）。
  const [linkDefaultText, setLinkDefaultText] = useState("");

  // 跟踪选区：行内原子节点已改为原地编辑；这里仅同步 Presence。
  useEffect(() => {
    if (!editable) return;
    const syncSelection = () => {
      if (applyingRemote.current) return;
      try {
        const selection = editor.prosemirrorState.selection;
        const anchor = editorPosition(selection.$anchor, state);
        const head = editorPosition(selection.$head, state);
        if (anchor && head) onPresenceChange?.({ anchor, head });
      } catch {
        // Some atomic node selections do not expose a text cursor.
      }
    };
    const unsubscribe = editor.onSelectionChange(syncSelection);
    syncSelection();
    return () => {
      unsubscribe();
    };
  }, [editor, editable, onPresenceChange, state]);

  const openLinkEditor = () => {
    setLinkDefaultText(editor.getSelectedText());
    setLinkOpen(true);
  };

  /** Slash menu / dialog 共用插入：空段落替换，非空块后插入。 */
  const insertTemplate = (block: Record<string, unknown>) => {
    insertOrUpdateBlockForSlashMenu(editor, block as never);
  };

  const insertAstBlock = (block: Block) => {
    const [mapped] = fromAst(
      { type: "document", schema_version: 1, children: [block] },
      state,
    );
    if (mapped) insertTemplate(mapped as Record<string, unknown>);
  };


  const customSlashItems: DefaultReactSuggestionItem[] = [
    {
      title: "页面引用",
      subtext: "插入站内页面引用",
      aliases: ["page", "ref", "页面", "引用"],
      group: "Anby",
      icon: <FileText className="size-4" />,
      onItemClick: () => setPickerOpen(true),
    },
    {
      title: "提及",
      subtext: "插入人员或日期提及",
      aliases: ["mention", "date", "提及", "日期"],
      group: "Anby",
      icon: <AtSign className="size-4" />,
      onItemClick: () => setMentionOpen(true),
    },
    {
      title: "外链",
      subtext: "插入外部链接",
      aliases: ["link", "url", "外链"],
      group: "Anby",
      icon: <Link2 className="size-4" />,
      onItemClick: openLinkEditor,
    },
    {
      title: "语义表格",
      subtext: "插入可嵌套内容的 AST 表格",
      aliases: ["table", "表格"],
      group: "Anby",
      icon: <Table2 className="size-4" />,
      onItemClick: () => insertTemplate({
        type: "astTable",
        children: [
          {
            type: "astTableRow",
            children: [
              { type: "astTableCell", children: [{ type: "paragraph", content: "单元格 1" }] },
              { type: "astTableCell", children: [{ type: "paragraph", content: "单元格 2" }] },
            ],
          },
        ],
      }),
    },
    {
      title: "页面路径",
      subtext: "插入 Breadcrumb",
      aliases: ["breadcrumb", "path", "路径"],
      group: "Anby",
      icon: <Navigation className="size-4" />,
      onItemClick: () => insertTemplate({ type: "breadcrumb", props: { itemsJson: JSON.stringify([{ display_text: "当前页面" }]) } }),
    },
    {
      title: "分栏",
      subtext: "插入双栏布局",
      aliases: ["columns", "layout", "分栏"],
      group: "Anby",
      icon: <Columns3 className="size-4" />,
      onItemClick: () => insertTemplate({
        type: "columnList",
        children: [
          { type: "column", props: { widthRatio: 1 }, children: [{ type: "paragraph", content: "左列" }] },
          { type: "column", props: { widthRatio: 1 }, children: [{ type: "paragraph", content: "右列" }] },
        ],
      }),
    },
    {
      title: "页面按钮",
      subtext: "插入声明式动作按钮",
      aliases: ["button", "action", "按钮"],
      group: "Anby",
      icon: <MousePointerClick className="size-4" />,
      onItemClick: () => setButtonOpen(true),
    },
    {
      title: "HTML 块",
      subtext: "插入隔离 HTML 预览",
      aliases: ["html", "iframe"],
      group: "Anby",
      icon: <CodeXml className="size-4" />,
      onItemClick: () => setHTMLOpen(true),
    },
    {
      title: "媒体",
      subtext: "插入图片、视频、音频、文件或 PDF",
      aliases: ["media", "asset", "image", "pdf", "媒体"],
      group: "Anby",
      icon: <FileAudio className="size-4" />,
      onItemClick: () => setMediaOpen(true),
    },
    {
      title: "外部内容",
      subtext: "插入书签、链接预览或 Embed",
      aliases: ["bookmark", "embed", "preview", "外部内容"],
      group: "Anby",
      icon: <PanelsTopLeft className="size-4" />,
      onItemClick: () => setExternalContentOpen(true),
    },
    {
      title: "组件",
      subtext: "插入已发布信息组件",
      aliases: ["component", "infobox", "组件"],
      group: "Anby",
      icon: <PanelsTopLeft className="size-4" />,
      onItemClick: () => setComponentOpen(true),
    },
  ];

  const getSlashMenuItems = async (query: string) =>
    filterSuggestionItems(
      [...customSlashItems, ...getDefaultReactSlashMenuItems(editor)],
      query,
    );

  // 快捷键：⌘/Ctrl+K 页面引用，⌘/Ctrl+Shift+K 外链。
  useEffect(() => {
    if (!editable) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.metaKey || event.ctrlKey)) return;
      if (event.key.toLowerCase() !== "k") return;
      event.preventDefault();
      if (event.shiftKey) {
        openLinkEditor();
      } else {
        setPickerOpen(true);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor, editable]);

  return (
    <div className="min-w-0" data-block-editor>
      <BlockNoteView
        editor={editor}
        editable={editable}
        theme="light"
        slashMenu={false}
        onChange={() => {
          if (applyingRemote.current) return;
          onChange?.(toAst(editor.document as unknown as BNBlock[], state));
        }}
      >
        {editable ? (
          <SuggestionMenuController
            triggerCharacter="/"
            getItems={getSlashMenuItems}
            shouldOpen={(menuState) =>
              !menuState.selection.$from.parent.type.isInGroup("tableContent")
            }
          />
        ) : null}
      </BlockNoteView>

      {editable ? (
        <>
          <PageReferencePicker
            open={pickerOpen}
            onOpenChange={setPickerOpen}
            onInsertResolved={(targetPageId, displayText) =>
              insertResolvedPageReference(editor, targetPageId, displayText)
            }
            onInsertUnresolved={(rawTitle) =>
              insertUnresolvedPageReference(editor, rawTitle)
            }
          />
          <ExternalLinkEditor
            open={linkOpen}
            onOpenChange={setLinkOpen}
            defaultDisplayText={linkDefaultText}
            onSubmit={(url, displayText) =>
              insertExternalLink(editor, url, displayText)
            }
          />
          <PageButtonInsertDialog
            open={buttonOpen}
            onOpenChange={setButtonOpen}
            currentPageId={pageId}
            onInsert={(value) =>
              insertTemplate({
                type: "pageButton",
                props: {
                  label: value.label,
                  confirmation: value.confirmation ?? "",
                  actionsJson: JSON.stringify(value.actions),
                },
              })
            }
          />
          <HTMLBlockInsertDialog
            open={htmlOpen}
            onOpenChange={setHTMLOpen}
            onInsert={(value) =>
              insertTemplate({
                type: "htmlBlock",
                props: {
                  html: value.html,
                  height: value.height,
                  caption: value.caption ?? "",
                },
              })
            }
          />
          <MediaBlockInsertDialog
            open={mediaOpen}
            onOpenChange={setMediaOpen}
            onInsert={insertAstBlock}
          />
          <ExternalContentInsertDialog
            open={externalContentOpen}
            onOpenChange={setExternalContentOpen}
            resolvePreview={resolveLinkPreview}
            onInsert={insertAstBlock}
          />
          <InlineMentionInsertDialog
            open={mentionOpen}
            onOpenChange={setMentionOpen}
            onInsertPerson={(actorId, displayText) =>
              insertPersonMention(editor, actorId, displayText)
            }
            onInsertDate={(date, endDate, displayText) =>
              insertDateMention(editor, date, endDate, displayText)
            }
          />
          <ComponentInsertDialog
            open={componentOpen}
            onOpenChange={setComponentOpen}
            pageId={pageId}
            onInsert={(selection) => {
              insertAstBlock({
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
