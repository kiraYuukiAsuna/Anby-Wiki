import { useCallback, useEffect, useRef } from "react";
import { DropdownMenu } from "radix-ui";
import { ArrowLeft, ArrowRight, Columns2, MoreHorizontal, Plus } from "lucide-react";
import { NodeViewContent, NodeViewWrapper, useEditorState, type ReactNodeViewProps } from "@tiptap/react";
import type { Editor } from "@tiptap/core";
import { Fragment, type Node as PMNode } from "@tiptap/pm/model";
import { Selection } from "@tiptap/pm/state";
import { closeHistory } from "@tiptap/pm/history";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { newBlockId } from "@/lib/editor/ids";
import { toAst } from "@/lib/editor/toAst";

type Action = "addBefore" | "addAfter" | "moveLeft" | "moveRight" | "mergeLeft" | "mergeRight" | "equal" | "unwrap";

export function insertColumns(editor: Editor) {
  if (!editor.isEditable) return false;
  const firstId = newBlockId();
  return editor.chain().focus().insertContent({ type: "columnList", content: [firstId, newBlockId()].map(id => ({
    type: "column", content: [{ type: "paragraph", attrs: { id } }],
  })) }).command(({ tr }) => {
    tr.doc.descendants((node, pos) => { if (node.attrs.id === firstId) tr.setSelection(Selection.near(tr.doc.resolve(pos + 1))); });
    return true;
  }).run();
}

function location(props: ReactNodeViewProps) {
  if (props.editor.isDestroyed) return;
  const pos = props.getPos();
  if (pos === undefined) return;
  const current = props.editor.state.doc.nodeAt(pos);
  if (!current || current.type !== props.node.type || current.attrs.id !== props.node.attrs.id) return;
  if (current.type.name === "columnList") return { pos, node: current, index: current.childCount - 1 };
  const $pos = props.editor.state.doc.resolve(pos);
  if (!$pos.depth || $pos.parent.type.name !== "columnList") return;
  return { pos: $pos.before(), node: $pos.parent, index: $pos.index() };
}

function editColumns(props: ReactNodeViewProps, action: Action) {
  if (!props.editor.isEditable) return false;
  const target = location(props);
  if (!target) return false;
  const { pos, node, index } = target;
  const columns: PMNode[] = [];
  node.forEach(column => columns.push(column));
  const { state, view } = props.editor;
  let focusId = columns[index].attrs.id;
  const paragraph = () => state.schema.nodes.paragraph.create({ id: newBlockId() });
  if (action === "addBefore" || action === "addAfter") {
    if (columns.length >= 5) return false;
    const added = state.schema.nodes.column.create({ id: newBlockId(), widthRatio: 1 }, paragraph());
    columns.splice(index + (action === "addAfter" ? 1 : 0), 0, added);
    focusId = added.attrs.id;
  } else if (action === "moveLeft" || action === "moveRight") {
    const other = index + (action === "moveLeft" ? -1 : 1);
    if (other < 0 || other >= columns.length) return false;
    [columns[index], columns[other]] = [columns[other], columns[index]];
  } else if (action === "mergeLeft" || action === "mergeRight") {
    const other = index + (action === "mergeLeft" ? -1 : 1);
    if (other < 0 || other >= columns.length) return false;
    const content = other < index ? columns[other].content.append(columns[index].content) : columns[index].content.append(columns[other].content);
    columns[other] = columns[other].copy(content);
    focusId = columns[other].attrs.id;
    columns.splice(index, 1);
  } else if (action === "equal") {
    columns.forEach((column, i) => { columns[i] = column.type.create({ ...column.attrs, widthRatio: 1 }, column.content); });
  }
  try {
    const tr = closeHistory(state.tr);
    if (action === "unwrap" || columns.length === 1) {
      let content = Fragment.empty;
      columns.forEach(column => { content = content.append(column.content); });
      tr.replaceWith(pos, pos + node.nodeSize, content.size ? content : paragraph());
      tr.setSelection(Selection.near(tr.doc.resolve(pos)));
    } else {
      tr.replaceWith(pos, pos + node.nodeSize, node.copy(Fragment.fromArray(columns)));
      tr.doc.descendants((child, start) => {
        if (child.attrs.id === focusId) tr.setSelection(Selection.near(tr.doc.resolve(start + 1)));
      });
    }
    toAst(tr.doc.toJSON());
    view.dispatch(tr.scrollIntoView());
    view.focus();
    return true;
  } catch { toast.error("分栏修改失败，原内容已保留。"); return false; }
}

function resizeColumns(props: ReactNodeViewProps, expected: PMNode, left: number) {
  const target = location(props);
  if (!props.editor.isEditable || !target || target.node !== expected || target.index >= target.node.childCount - 1) return false;
  const { pos, node, index } = target;
  const first = node.child(index), second = node.child(index + 1);
  const total = first.attrs.widthRatio + second.attrs.widthRatio;
  const next = Math.max(Math.max(0.1, total - 10), Math.min(Math.min(10, total - 0.1), left));
  if (Math.abs(next - first.attrs.widthRatio) < 0.001) return false;
  let start = pos + 1;
  for (let i = 0; i < index; i++) start += node.child(i).nodeSize;
  const tr = closeHistory(props.editor.state.tr)
    .setNodeMarkup(start, undefined, { ...first.attrs, widthRatio: next })
    .setNodeMarkup(start + first.nodeSize, undefined, { ...second.attrs, widthRatio: total - next });
  try { toAst(tr.doc.toJSON()); props.editor.view.dispatch(tr); return true; }
  catch { toast.error("宽度修改失败，原内容已保留。"); return false; }
}

export function ColumnsView(props: ReactNodeViewProps) {
  const editable = useEditorState({ editor: props.editor, selector: ({ editor }) => editor.isEditable });
  return <NodeViewWrapper className="ast-columns" data-editor-block-type="columnList" data-id={props.node.attrs.id}>
    {editable ? <div className="ast-columns-toolbar" contentEditable={false}>
      <span><Columns2 size={15} aria-hidden />{props.node.childCount} 栏</span>
      <div className="flex flex-wrap gap-1">
        <Button type="button" size="sm" variant="ghost" disabled={props.node.childCount >= 5} onClick={() => editColumns(props, "addAfter")}><Plus aria-hidden />增加一栏</Button>
        <Button type="button" size="sm" variant="ghost" onClick={() => editColumns(props, "equal")}>平均分配</Button>
        <Button type="button" size="sm" variant="ghost" onClick={() => editColumns(props, "unwrap")}>取消分栏</Button>
      </div>
    </div> : null}
    <NodeViewContent className="ast-columns-content" />
  </NodeViewWrapper>;
}

type Drag = { node: PMNode; x: number; pixels: number; initial: number; total: number; next: number; left: HTMLElement; right: HTMLElement; leftStyle: string; rightStyle: string };

export function ColumnView(props: ReactNodeViewProps) {
  const { doc, editable } = useEditorState({ editor: props.editor, selector: ({ editor }) => ({ doc: editor.state.doc, editable: editor.isEditable }) });
  const drag = useRef<Drag | null>(null);
  const actionRan = useRef(false);
  const target = location(props);
  const index = target?.index ?? 0;
  const count = target?.node.childCount ?? 0;
  const next = target && index + 1 < count ? target.node.child(index + 1) : undefined;
  const ratio = props.node.attrs.widthRatio;
  const total = ratio + (next?.attrs.widthRatio ?? 0);
  const resetPreview = useCallback(() => {
    const value = drag.current;
    if (!value) return;
    value.left.style.flexGrow = value.leftStyle;
    value.right.style.flexGrow = value.rightStyle;
    if (!props.editor.isDestroyed) props.editor.state.doc.descendants(node => {
      if (node.attrs.id === value.left.dataset.id) value.left.style.flexGrow = String(node.attrs.widthRatio);
      if (node.attrs.id === value.right.dataset.id) value.right.style.flexGrow = String(node.attrs.widthRatio);
    });
    drag.current = null;
  }, [props.editor]);
  useEffect(() => { resetPreview(); }, [doc, editable, resetPreview]);
  useEffect(() => () => resetPreview(), [resetPreview]);
  const run = (action: Action) => { actionRan.current = editColumns(props, action); };
  const menuItem = (action: Action, label: string, disabled: boolean) => <DropdownMenu.Item className="ast-table-menu__item" disabled={disabled} onSelect={() => run(action)}>{label}</DropdownMenu.Item>;
  return <NodeViewWrapper className="ast-column" data-editor-block-type="column" data-id={props.node.attrs.id} style={{ flexGrow: ratio }}>
    {editable ? <div className="ast-column-toolbar" contentEditable={false}>
      <span>第 {index + 1} 栏</span>
      <DropdownMenu.Root modal={false}><DropdownMenu.Trigger asChild><Button type="button" size="icon-sm" variant="ghost" aria-label={`第 ${index + 1} 栏操作`}><MoreHorizontal aria-hidden /></Button></DropdownMenu.Trigger>
        <DropdownMenu.Portal><DropdownMenu.Content className="ast-table-menu" align="end" sideOffset={4} collisionPadding={8} onKeyDown={event => event.stopPropagation()} onCloseAutoFocus={event => {
          if (actionRan.current) { event.preventDefault(); if (!props.editor.isDestroyed) props.editor.view.focus(); actionRan.current = false; }
        }}>
          {menuItem("addBefore", "在左侧插入栏", count >= 5)}
          {menuItem("addAfter", "在右侧插入栏", count >= 5)}
          {menuItem("moveLeft", "向左移动", index === 0)}
          {menuItem("moveRight", "向右移动", index === count - 1)}
          <DropdownMenu.Separator className="my-1 border-t" />
          {menuItem("mergeLeft", "合并到左栏", index === 0)}
          {menuItem("mergeRight", "合并到右栏", index === count - 1)}
        </DropdownMenu.Content></DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div> : null}
    <NodeViewContent className="ast-column-content" />
    {editable && next ? <div className="ast-column-resize" contentEditable={false} role="separator" aria-orientation="vertical" tabIndex={0}
      aria-label={`调整第 ${index + 1}、${index + 2} 栏宽度`} aria-valuenow={Math.round(ratio / total * 100)} aria-valuemin={Math.ceil(Math.max(0.1, total - 10) / total * 100)} aria-valuemax={Math.floor(Math.min(10, total - 0.1) / total * 100)}
      title="拖动调整栏宽，方向键微调，双击均分"
      onPointerDown={event => {
        if (event.button !== 0 || !target) return;
        event.preventDefault(); event.stopPropagation();
        const left = event.currentTarget.closest<HTMLElement>(".ast-column")!;
        const right = left.parentElement?.nextElementSibling?.firstElementChild as HTMLElement | null;
        if (!right?.classList.contains("ast-column")) return;
        drag.current = { node: target.node, x: event.clientX, pixels: left.getBoundingClientRect().width + right.getBoundingClientRect().width, initial: ratio, total, next: ratio, left, right, leftStyle: left.style.flexGrow, rightStyle: right.style.flexGrow };
        event.currentTarget.focus();
        event.currentTarget.setPointerCapture(event.pointerId);
      }}
      onPointerMove={event => {
        const value = drag.current;
        if (!value) return;
        const width = value.initial + (event.clientX - value.x) / value.pixels * value.total;
        value.next = Math.max(Math.max(0.1, value.total - 10), Math.min(Math.min(10, value.total - 0.1), width));
        value.left.style.flexGrow = String(value.next);
        value.right.style.flexGrow = String(value.total - value.next);
      }}
      onPointerUp={event => {
        const value = drag.current;
        resetPreview();
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
        if (value) resizeColumns(props, value.node, value.next);
      }}
      onPointerCancel={resetPreview} onLostPointerCapture={resetPreview}
      onDoubleClick={() => target && resizeColumns(props, target.node, total / 2)}
      onKeyDown={event => {
        if (event.key === "Escape") { resetPreview(); return; }
        if (!target || !["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key) || event.nativeEvent.isComposing) return;
        event.preventDefault(); event.stopPropagation();
        const width = event.key === "Home" ? 0.1 : event.key === "End" ? total - 0.1 : ratio + (event.key === "ArrowRight" ? 1 : -1) * total * (event.shiftKey ? 0.1 : 0.02);
        resizeColumns(props, target.node, width);
      }}><span /><ArrowLeft size={10} aria-hidden /><ArrowRight size={10} aria-hidden /></div> : null}
  </NodeViewWrapper>;
}
