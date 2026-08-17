// 受控 Block 编辑器包装组件（M2-T02，供 M2-T03 编辑页面使用）。
//
// 业务代码只允许通过这个组件使用 BlockNote（ADR-0005）：
// 输入初始 AST v1 Document，编辑时经 onChange 回传最新 AST。
// AdapterState 随组件实例存活，保证会话内 Block ID 稳定。
//
// M2-T04 引用编辑：工具栏与快捷键（⌘K 页面引用、⇧⌘K 外链）触发插入；
// 点击既有 pageReference（NodeSelection）时显示 display_text 编辑面板
// （只改显示文本，不改 target_page_id）。插入/更新全部经
// lib/editor/references.ts 的 helper，不在这里直接拼 BlockNote 结构。
"use client";

import { useEffect, useMemo, useState } from "react";

import { BlockNoteView } from "@blocknote/mantine";
import { useCreateBlockNote } from "@blocknote/react";
import {
  BookMarked,
  BookOpenCheck,
  FileText,
  Globe2,
  Heading2,
  Link2,
  List,
  Table2,
} from "lucide-react";
import { toast } from "sonner";

import "@blocknote/core/style.css";
import "@blocknote/mantine/style.css";

import { ExternalLinkEditor } from "@/components/editor/external-link-editor";
import {
  InfoboxInsertButton,
  type InfoboxSelection,
} from "@/components/editor/infobox-insert-button";
import { PageReferencePicker } from "@/components/editor/page-reference-picker";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Document } from "@/lib/ast/schema";
import { fromAst } from "@/lib/editor/fromAst";
import { createAdapterState, type AdapterState } from "@/lib/editor/ids";
import {
  getSelectedPageReference,
  insertExternalLink,
  insertResolvedPageReference,
  insertUnresolvedPageReference,
  updateSelectedPageReferenceDisplayText,
  type SelectedPageReference,
} from "@/lib/editor/references";
import { editorSchema, type BNBlock } from "@/lib/editor/schema";
import { toAst } from "@/lib/editor/toAst";

export interface BlockEditorProps {
  /** 初始文档（AST v1）。组件非受控于该 prop：后续变更不会重置编辑器。 */
  initialAst: Document;
  /** 每次编辑后回传序列化后的 AST v1 Document。 */
  onChange?: (ast: Document) => void;
  /** 协作 Presence 只发送稳定 Block ID 和选区类型，不发送正文。 */
  onPresenceChange?: (cursor: Record<string, unknown>) => void;
  /** 传给 BlockNoteView 的 editable。 */
  editable?: boolean;
  /** 既有页面 ID；用于发现主 Entity 并插入可信信息框。 */
  pageId?: string;
}

export function BlockEditor({
  initialAst,
  onChange,
  onPresenceChange,
  editable = true,
  pageId,
}: BlockEditorProps) {
  // AdapterState 随组件实例存活（lazy useState），保证会话内 Block ID 稳定。
  const [state] = useState<AdapterState>(() => createAdapterState());

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
  });

  // ---- M2-T04 引用编辑 UI 状态 ----
  const [pickerOpen, setPickerOpen] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  // 打开外链编辑器时的默认显示文本（选中文本）。
  const [linkDefaultText, setLinkDefaultText] = useState("");
  // 当前选中的 pageReference 节点（点击引用产生的 NodeSelection）。
  const [selectedRef, setSelectedRef] = useState<SelectedPageReference | null>(
    null,
  );
  const [refDisplayText, setRefDisplayText] = useState("");

  // 跟踪选区：选中 pageReference 原子节点时展示 display_text 编辑面板。
  useEffect(() => {
    if (!editable) return;
    const syncSelection = () => {
      const selected = getSelectedPageReference(editor);
      setSelectedRef(selected);
      if (selected) setRefDisplayText(selected.displayText);
      try {
        const cursor = editor.getTextCursorPosition();
        onPresenceChange?.({
          block_id: cursor.block.id,
          selection: selected ? "reference" : "text",
        });
      } catch {
        // Some atomic node selections do not expose a text cursor.
      }
    };
    const unsubscribe = editor.onSelectionChange(syncSelection);
    syncSelection();
    return () => {
      unsubscribe();
    };
  }, [editor, editable, onPresenceChange]);

  const openLinkEditor = () => {
    setLinkDefaultText(editor.getSelectedText());
    setLinkOpen(true);
  };

  /** 常用结构块的显式入口，避免非技术用户依赖 slash command。 */
  const insertTemplate = (block: Record<string, unknown>) => {
    const cursor = editor.getTextCursorPosition();
    editor.insertBlocks([block] as never, cursor.block, "after");
  };

  const insertEndSection = (title: string, placeholder: string) => {
    const lastBlock = editor.document.at(-1);
    if (!lastBlock) return;
    editor.insertBlocks(
      [
        { type: "heading", props: { level: 2 }, content: title },
        { type: "bulletListItem", content: placeholder },
      ] as never,
      lastBlock,
      "after",
    );
  };

  const insertInfobox = (selection: InfoboxSelection) => {
    const blocks = editor.document as unknown as BNBlock[];
    const includesComponent = (items: BNBlock[]): boolean =>
      items.some(
        (block) =>
          (block.type === "component" &&
            block.props.componentId === selection.componentId) ||
          includesComponent(block.children),
      );
    if (includesComponent(blocks)) {
      toast.info("页面中已经包含文章信息框");
      return;
    }
    const firstBlock = editor.document[0];
    if (!firstBlock) return;
    editor.insertBlocks(
      [
        {
          type: "component",
          props: {
            componentId: selection.componentId,
            componentVersion: selection.componentVersion,
            entityId: selection.entityId,
            displayConfig: JSON.stringify({
              title: "",
              language: selection.language,
              property_keys: [],
            }),
          },
        },
      ] as never,
      firstBlock,
      "before",
    );
    toast.success("已在文首插入主实体信息框");
  };

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
    <div data-block-editor>
      {editable ? (
        <div className="mb-2 flex flex-wrap items-center gap-2" data-editor-toolbar>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setPickerOpen(true)}
          >
            <FileText className="size-4" />
            页面引用
            <kbd className="text-xs text-muted-foreground">⌘K</kbd>
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={openLinkEditor}
          >
            <Link2 className="size-4" />
            外链
            <kbd className="text-xs text-muted-foreground">⇧⌘K</kbd>
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              insertTemplate({
                type: "heading",
                props: { level: 2 },
                content: "章节标题",
              })
            }
          >
            <Heading2 className="size-4" />
            插入标题
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              insertTemplate({ type: "bulletListItem", content: "列表项" })
            }
          >
            <List className="size-4" />
            插入列表
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              insertTemplate({
                type: "astTable",
                children: [
                  {
                    type: "astTableRow",
                    children: [
                      {
                        type: "astTableCell",
                        children: [{ type: "paragraph", content: "单元格 1" }],
                      },
                      {
                        type: "astTableCell",
                        children: [{ type: "paragraph", content: "单元格 2" }],
                      },
                    ],
                  },
                ],
              })
            }
          >
            <Table2 className="size-4" />
            插入表格
          </Button>
          {pageId ? (
            <InfoboxInsertButton pageId={pageId} onInsert={insertInfobox} />
          ) : null}
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => insertEndSection("参见", "插入相关页面引用")}
          >
            <BookOpenCheck className="size-4" />
            参见
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => insertEndSection("拓展阅读", "补充书目或来源")}
          >
            <BookMarked className="size-4" />
            拓展阅读
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => insertEndSection("外部链接", "补充外部资源")}
          >
            <Globe2 className="size-4" />
            外部链接
          </Button>
        </div>
      ) : null}

      {editable && selectedRef ? (
        <div
          className="mb-2 flex items-center gap-2 rounded-md border border-border p-2"
          data-page-ref-panel
        >
          {selectedRef.resolved ? (
            <>
              <span className="text-xs text-muted-foreground">
                引用显示文本
              </span>
              <Input
                value={refDisplayText}
                onChange={(event) => setRefDisplayText(event.target.value)}
                className="h-8 max-w-64"
                aria-label="引用显示文本"
              />
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() =>
                  updateSelectedPageReferenceDisplayText(editor, refDisplayText)
                }
              >
                应用
              </Button>
              <span className="text-xs text-muted-foreground">
                仅修改显示文本，引用目标不变
              </span>
            </>
          ) : (
            <span className="text-xs text-muted-foreground">
              未解析引用：{selectedRef.normalizedTitle}
              （目标页面创建后自动解析）
            </span>
          )}
        </div>
      ) : null}

      <BlockNoteView
        editor={editor}
        editable={editable}
        theme="light"
        onChange={() => {
          onChange?.(toAst(editor.document as unknown as BNBlock[], state));
        }}
      />

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
        </>
      ) : null}
    </div>
  );
}
