import { useEffect, useRef, useState } from "react";
import { ContextMenu, DropdownMenu } from "radix-ui";
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Check, Eraser, MoreHorizontal, Trash2 } from "lucide-react";
import { NodeViewContent, NodeViewWrapper, type ReactNodeViewProps } from "@tiptap/react";
import type { Node as PMNode } from "@tiptap/pm/model";
import { Selection } from "@tiptap/pm/state";
import { closeHistory } from "@tiptap/pm/history";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { newBlockId } from "@/lib/editor/ids";
import { toAst } from "@/lib/editor/toAst";

type TableAction = "rowBefore" | "rowAfter" | "columnBefore" | "columnAfter" | "deleteRow" | "deleteColumn" | "clearCell" | "deleteTable";
type CellLocation = { row: number; column: number; rowOffset: number; cellOffset: number; cell: PMNode };

function locateCell(table: PMNode, id: string): CellLocation | undefined {
  let found: CellLocation | undefined;
  table.forEach((row, rowOffset, rowIndex) => row.forEach((cell, cellOffset, column) => {
    if (cell.attrs.id === id) found = { row: rowIndex, column, rowOffset, cellOffset, cell };
  }));
  return found;
}

function canDeleteColumn(table: PMNode, column: number) {
  let allowed = true;
  table.forEach(row => { if (column < row.childCount && row.childCount <= 1) allowed = false; });
  return allowed;
}

function editTable(props: ReactNodeViewProps, cellId: string, action: TableAction): boolean {
  if (!props.editor.isEditable) return false;
  const pos = props.getPos();
  if (pos === undefined) return false;
  const { state, view } = props.editor;
  const table = state.doc.nodeAt(pos);
  if (!table || table.type.name !== "astTable" || table.attrs.id !== props.node.attrs.id) return false;
  const target = locateCell(table, cellId);
  if (action !== "deleteTable" && !target) return false;
  const tr = closeHistory(state.tr);
  const paragraph = () => state.schema.nodes.paragraph.create({ id: newBlockId() });
  const cell = () => state.schema.nodes.astTableCell.create({ id: newBlockId() }, paragraph());
  let focusRow = target?.row ?? 0;
  let focusColumn = target?.column ?? 0;
  if (action === "deleteTable") {
    tr.delete(pos, pos + table.nodeSize);
  } else if (target) {
    const row = table.child(target.row);
    const rowPos = pos + 1 + target.rowOffset;
    const cellPos = rowPos + 1 + target.cellOffset;
    switch (action) {
      case "rowBefore":
      case "rowAfter": {
        const after = action === "rowAfter";
        const newRow = state.schema.nodes.astTableRow.create({ id: newBlockId() }, Array.from({ length: row.childCount }, cell));
        tr.insert(rowPos + (after ? row.nodeSize : 0), newRow);
        focusRow += after ? 1 : 0;
        break;
      }
      case "columnBefore":
      case "columnAfter": {
        focusColumn += action === "columnAfter" ? 1 : 0;
        table.forEach((item, offset) => {
          let cellOffset = 0;
          for (let index = 0; index < Math.min(focusColumn, item.childCount); index++) cellOffset += item.child(index).nodeSize;
          tr.insert(tr.mapping.map(pos + 2 + offset + cellOffset), cell());
        });
        break;
      }
      case "deleteRow":
        if (table.childCount <= 1) return false;
        tr.delete(rowPos, rowPos + row.nodeSize);
        break;
      case "deleteColumn":
        if (!canDeleteColumn(table, target.column)) return false;
        table.forEach((item, offset) => {
          if (target.column >= item.childCount) return;
          let cellOffset = 0;
          for (let index = 0; index < target.column; index++) cellOffset += item.child(index).nodeSize;
          const start = pos + 2 + offset + cellOffset;
          tr.delete(tr.mapping.map(start), tr.mapping.map(start + item.child(target.column).nodeSize));
        });
        break;
      case "clearCell":
        tr.replaceWith(cellPos + 1, cellPos + target.cell.nodeSize - 1, paragraph());
        break;
    }
  }
  let focusPos = Math.min(pos, tr.doc.content.size);
  if (action !== "deleteTable") {
    const next = tr.doc.nodeAt(pos)!;
    focusRow = Math.min(focusRow, next.childCount - 1);
    const row = next.child(focusRow);
    focusColumn = Math.min(focusColumn, row.childCount - 1);
    focusPos = pos + 3;
    for (let index = 0; index < focusRow; index++) focusPos += next.child(index).nodeSize;
    for (let index = 0; index < focusColumn; index++) focusPos += row.child(index).nodeSize;
  }
  tr.setSelection(Selection.near(tr.doc.resolve(focusPos))).scrollIntoView();
  try {
    toAst(tr.doc.toJSON());
    view.dispatch(tr);
    return true;
  } catch {
    toast.error("表格修改失败，原内容已保留。");
    return false;
  }
}

export function TableView(props: ReactNodeViewProps & { onUpdateAttrs: (attrs: Record<string, unknown>) => boolean }) {
  const { editor, node } = props;
  const [activeCell, setActiveCell] = useState("");
  const [toolbarOpen, setToolbarOpen] = useState(false);
  const [contextOpen, setContextOpen] = useState(false);
  const menuOpen = toolbarOpen || contextOpen;
  const actionRan = useRef(false);
  const keyboardMenu = useRef(false);
  const [editable, setEditable] = useState(editor.isEditable);
  const cellId = activeCell || node.firstChild?.firstChild?.attrs.id || "";
  const target = locateCell(node, cellId);

  useEffect(() => {
    const selectedCell = () => {
      const { $head } = editor.state.selection;
      for (let depth = $head.depth; depth > 0; depth--) {
        if ($head.node(depth).type.name !== "astTableCell") continue;
        return depth >= 2 && $head.node(depth - 2).attrs.id === node.attrs.id ? $head.node(depth).attrs.id as string : undefined;
      }
    };
    const trackSelection = () => { if (menuOpen) return; const id = selectedCell(); if (id) setActiveCell(id); };
    const trackEditable = () => setEditable(editor.isEditable);
    const openKeyboardMenu = (event: KeyboardEvent) => {
      if (!editor.isEditable || event.isComposing || !(event.key === "ContextMenu" || (event.shiftKey && event.key === "F10")) ||
        (event.target as Element).closest("input,textarea,button,select")) return;
      const id = selectedCell();
      if (!id) return;
      event.preventDefault(); event.stopPropagation();
      setActiveCell(id); keyboardMenu.current = true; setToolbarOpen(true);
    };
    trackSelection();
    editor.on("selectionUpdate", trackSelection);
    editor.on("update", trackEditable);
    const dom = editor.view.dom;
    dom.addEventListener("keydown", openKeyboardMenu);
    return () => {
      editor.off("selectionUpdate", trackSelection);
      editor.off("update", trackEditable);
      dom.removeEventListener("keydown", openKeyboardMenu);
    };
  }, [editor, node.attrs.id, menuOpen]);

  const rememberCell = (element: EventTarget) => {
    if (!(element instanceof Element)) return false;
    const cell = element.closest<HTMLElement>(".ast-table-cell");
    if (!cell || cell.closest<HTMLElement>('[data-editor-block-type="astTable"]')?.dataset.id !== node.attrs.id) return false;
    if (!cell.dataset.id) return false;
    setActiveCell(cell.dataset.id);
    return true;
  };
  const run = (action: TableAction) => { actionRan.current = editTable(props, cellId, action); };
  const applyAttrs = (attrs: Record<string, unknown>) => {
    if (!editor.isEditable) return false;
    editor.view.dispatch(closeHistory(editor.state.tr));
    return props.onUpdateAttrs(attrs);
  };
  const update = (attrs: Record<string, unknown>) => { actionRan.current = applyAttrs(attrs); };
  const restoreFocus = (event: Event, context = false) => {
    if (actionRan.current || context || keyboardMenu.current) {
      event.preventDefault();
      if (!editor.isDestroyed) editor.view.focus();
    }
    actionRan.current = false;
    keyboardMenu.current = false;
  };
  const items = (Menu: typeof ContextMenu | typeof DropdownMenu) => <>
    <Menu.Label className="ast-table-menu__label">{target ? `第 ${target.row + 1} 行，第 ${target.column + 1} 列` : "请选择单元格"}</Menu.Label>
    {([
      ["rowBefore", "在上方插入行", ArrowUp], ["rowAfter", "在下方插入行", ArrowDown],
      ["columnBefore", "在左侧插入列", ArrowLeft], ["columnAfter", "在右侧插入列", ArrowRight],
    ] as const).map(([action, label, Icon]) => <Menu.Item key={action} className="ast-table-menu__item" disabled={!target} onSelect={() => run(action)}><Icon aria-hidden />{label}</Menu.Item>)}
    <Menu.Separator className="ast-table-menu__separator" />
    <Menu.CheckboxItem className="ast-table-menu__item" checked={Boolean(node.attrs.hasColumnHeader)} onCheckedChange={checked => update({ hasColumnHeader: checked })}>
      <span className="size-4"><Menu.ItemIndicator><Check aria-hidden /></Menu.ItemIndicator></span>首行作为表头
    </Menu.CheckboxItem>
    <Menu.CheckboxItem className="ast-table-menu__item" checked={Boolean(node.attrs.hasRowHeader)} onCheckedChange={checked => update({ hasRowHeader: checked })}>
      <span className="size-4"><Menu.ItemIndicator><Check aria-hidden /></Menu.ItemIndicator></span>首列作为表头
    </Menu.CheckboxItem>
    <Menu.Separator className="ast-table-menu__separator" />
    <Menu.Item className="ast-table-menu__item" disabled={!target} onSelect={() => run("clearCell")}><Eraser aria-hidden />清空单元格</Menu.Item>
    <Menu.Item className="ast-table-menu__item text-destructive" disabled={!target || node.childCount <= 1} onSelect={() => run("deleteRow")}><Trash2 aria-hidden />删除当前行</Menu.Item>
    <Menu.Item className="ast-table-menu__item text-destructive" disabled={!target || !canDeleteColumn(node, target.column)} onSelect={() => run("deleteColumn")}><Trash2 aria-hidden />删除当前列</Menu.Item>
    <Menu.Separator className="ast-table-menu__separator" />
    <Menu.Item className="ast-table-menu__item text-destructive" onSelect={() => run("deleteTable")}><Trash2 aria-hidden />删除表格</Menu.Item>
  </>;
  return <NodeViewWrapper as="section" className="ast-editable-block ast-table-editor" data-editor-block-type="astTable" data-id={node.attrs.id}
    data-column-header={node.attrs.hasColumnHeader || undefined} data-row-header={node.attrs.hasRowHeader || undefined}>
    {editable ? <div className="ast-table-toolbar" contentEditable={false} onKeyDown={event => event.stopPropagation()}>
      <input key={node.attrs.caption} aria-label="表格标题" placeholder="表格标题（可选）" defaultValue={node.attrs.caption}
        onBlur={event => { if (event.target.value !== node.attrs.caption && !applyAttrs({ caption: event.target.value })) event.target.value = node.attrs.caption; }} />
      <DropdownMenu.Root open={toolbarOpen} onOpenChange={setToolbarOpen} modal={false}>
        <DropdownMenu.Trigger asChild><Button type="button" variant="ghost" size="sm" aria-label="表格操作"><MoreHorizontal aria-hidden />表格操作</Button></DropdownMenu.Trigger>
        <DropdownMenu.Portal><DropdownMenu.Content className="ast-table-menu" sideOffset={4} align="end" collisionPadding={8} onCloseAutoFocus={restoreFocus} onKeyDown={event => event.stopPropagation()}>{items(DropdownMenu)}</DropdownMenu.Content></DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div> : node.attrs.caption ? <div className="text-sm text-muted-foreground">{node.attrs.caption}</div> : null}
    <ContextMenu.Root modal={false} open={contextOpen} onOpenChange={setContextOpen}>
      <ContextMenu.Trigger asChild disabled={!editable}>
        <div className="ast-table-region" onPointerDown={event => { if (editable && rememberCell(event.target)) event.stopPropagation(); }}
          onContextMenu={event => { if (!editable) return; if (rememberCell(event.target)) event.stopPropagation(); else event.preventDefault(); }}>
          <NodeViewContent className="ast-editable-block__content" />
        </div>
      </ContextMenu.Trigger>
      {editable ? <ContextMenu.Portal><ContextMenu.Content className="ast-table-menu" collisionPadding={8} onCloseAutoFocus={event => restoreFocus(event, true)} onKeyDown={event => event.stopPropagation()}>{items(ContextMenu)}</ContextMenu.Content></ContextMenu.Portal> : null}
    </ContextMenu.Root>
  </NodeViewWrapper>;
}
