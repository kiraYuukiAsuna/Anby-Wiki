import { useRef, useState } from "react";
import { Pencil } from "lucide-react";
import { NodeViewWrapper, useEditorState, type ReactNodeViewProps } from "@tiptap/react";
import { closeHistory } from "@tiptap/pm/history";
import { toast } from "sonner";
import { BlockView } from "@/components/ast";
import { Button } from "@/components/ui/button";
import { PageButtonInsertDialog } from "@/components/editor/page-button-insert-dialog";
import { HTMLBlockInsertDialog } from "@/components/editor/html-block-insert-dialog";
import { MediaBlockInsertDialog, type MediaBlock } from "@/components/editor/media-block-insert-dialog";
import type { Block } from "@/lib/ast/schema";
import { fromAst } from "@/lib/editor/fromAst";
import { toAst } from "@/lib/editor/toAst";

type EditableBlock = Extract<Block, { type: "button" | "html" }> | MediaBlock;
const names = { button: "按钮", html: "HTML", image: "图片", video: "视频", audio: "音频", file: "文件", pdf: "PDF" };

export function EditableAtomView(props: ReactNodeViewProps & { onUpdateAttrs: (attrs: Record<string, unknown>) => boolean }) {
  const editable = useEditorState({ editor: props.editor, selector: ({ editor }) => editor.isEditable });
  const block = toAst({ type: "doc", content: [props.node.toJSON()] }).children[0] as EditableBlock;
  const [draft, setDraft] = useState<EditableBlock | null>(null);
  const baseline = useRef("");
  const trigger = useRef<HTMLButtonElement>(null);
  const close = (open: boolean) => {
    if (!open) { setDraft(null); requestAnimationFrame(() => trigger.current?.focus()); }
  };
  const save = (value: EditableBlock) => {
    const pos = props.getPos();
    if (!props.editor.isEditable || pos === undefined) return false;
    const current = props.editor.state.doc.nodeAt(pos);
    if (!current || JSON.stringify(current.toJSON()) !== baseline.current) {
      toast.error("此块已更新，请重新打开编辑，避免覆盖新内容。");
      return false;
    }
    const attrs = fromAst({ schema_version: 1, type: "document", children: [value] }).content?.[0].attrs;
    if (!attrs) return false;
    props.editor.view.dispatch(closeHistory(props.editor.state.tr));
    return props.onUpdateAttrs(attrs);
  };
  return <NodeViewWrapper as="section" className={`ast-editable-block ast-${block.type}`} data-editor-block-type={props.node.type.name} data-id={block.id} contentEditable={false}>
    {editable && <div className="flex justify-end"><Button ref={trigger} type="button" variant="ghost" size="sm" onClick={() => { baseline.current = JSON.stringify(props.node.toJSON()); setDraft(block); }}><Pencil aria-hidden />编辑{block.type === "html" || block.type === "pdf" ? " " : ""}{names[block.type]}</Button></div>}
    <div className="ast-editor-preview" onClickCapture={event => {
      if ((event.target as Element).closest("a,button")) { event.preventDefault(); event.stopPropagation(); }
    }}><BlockView block={block} /></div>
    {editable && draft && (draft.type === "button" ? <PageButtonInsertDialog open onOpenChange={close} initialValue={draft} onInsert={value => save({ id: draft.id, type: "button", ...value })} />
      : draft.type === "html" ? <HTMLBlockInsertDialog open onOpenChange={close} initialValue={draft} onInsert={value => save({ id: draft.id, type: "html", ...value })} />
      : <MediaBlockInsertDialog open onOpenChange={close} initialBlock={draft} onInsert={save} />)}
  </NodeViewWrapper>;
}
