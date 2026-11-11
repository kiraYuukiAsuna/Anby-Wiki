import { useEffect, useRef, useState } from "react";
import type { Editor } from "@tiptap/core";
import type { Node as PMNode } from "@tiptap/pm/model";
import { NodeSelection } from "@tiptap/pm/state";
import { Code2, Columns2, Heading, List, Pilcrow, Quote, Square, Table2, ChevronRight } from "lucide-react";
import { Tooltip } from "radix-ui";
import { Button } from "@/components/ui/button";

const names: Record<string, string> = {
  paragraph: "正文", heading: "标题", sectionHeading: "标题", bulletList: "无序列表", orderedList: "有序列表", listItem: "列表项",
  blockquote: "引用", codeBlock: "代码", horizontalRule: "分割线", preformatted: "等宽文本", equation: "公式", todoBlock: "待办",
  toggleBlock: "折叠块", breadcrumb: "页面路径", columnList: "分栏", column: "栏", pageButton: "页面按钮", htmlBlock: "HTML",
  callout: "提示框", componentBlock: "知识组件", imageAsset: "图片", videoAsset: "视频", audioAsset: "音频", fileAsset: "文件",
  pdfAsset: "PDF", bookmark: "书签", linkPreview: "链接预览", embedBlock: "嵌入内容", astTable: "表格", astTableRow: "表格行", astTableCell: "单元格",
};
const icons = { paragraph: Pilcrow, heading: Heading, sectionHeading: Heading, bulletList: List, orderedList: List, listItem: List,
  blockquote: Quote, codeBlock: Code2, htmlBlock: Code2, toggleBlock: ChevronRight, columnList: Columns2, column: Columns2,
  astTable: Table2, astTableRow: Table2, astTableCell: Table2 };
type BlockPosition = { node: PMNode; pos: number };
type Target = { id: string; type: string; label: string; path: string; top: number; left: number; width: number; height: number };

function findBlock(editor: Editor, id: string): BlockPosition | null {
  let found: BlockPosition | null = null;
  editor.state.doc.descendants((node, pos) => {
    if (found) return false;
    if (node.isBlock && node.attrs.id === id) { found = { node, pos }; return false; }
  });
  return found;
}

function selectedBlock(editor: Editor): BlockPosition | null {
  const selection = editor.state.selection;
  if (selection instanceof NodeSelection && selection.node.isBlock && selection.node.attrs.id) return { node: selection.node, pos: selection.from };
  for (let depth = selection.$head.depth; depth > 0; depth--) {
    const node = selection.$head.node(depth);
    if (node.isBlock && node.attrs.id) return { node, pos: selection.$head.before(depth) };
  }
  return null;
}

export function BlockGutter({ editor }: { editor: Editor }) {
  const anchor = useRef<HTMLDivElement>(null);
  const [targets, setTargets] = useState<{ active: Target | null; hover: Target | null }>({ active: null, hover: null });

  useEffect(() => {
    const host = anchor.current?.parentElement;
    if (!host) return;
    const root = editor.view.dom;
    let hoveredId: string | null = null;
    let frame = 0;
    const measure = (block: BlockPosition | null): Target | null => {
      if (!block) return null;
      const dom = editor.view.nodeDOM(block.pos);
      if (!(dom instanceof HTMLElement) || !dom.getClientRects().length) return null;
      const box = dom.getBoundingClientRect(), base = host.getBoundingClientRect();
      if (!box.height) return null;
      const label = names[block.node.type.name] ?? "内容块";
      const position = editor.state.doc.resolve(block.pos), parents: string[] = [];
      for (let depth = 1; depth <= position.depth; depth++) {
        const parent = position.node(depth);
        if (parent.attrs.id) parents.push(names[parent.type.name] ?? "内容块");
      }
      return { id: block.node.attrs.id, type: block.node.type.name, label, path: [...parents, label].join(" › "),
        top: box.top - base.top, left: box.left - base.left, width: box.width, height: box.height };
    };
    const update = () => {
      frame = 0;
      if (editor.isDestroyed) return;
      const focused = document.activeElement;
      const field = focused instanceof HTMLElement && root.contains(focused) && focused.matches("input,textarea,select")
        ? focused.closest<HTMLElement>("[data-id]")?.dataset.id : undefined;
      setTargets({
        active: editor.isEditable && host.contains(focused) ? measure(field ? findBlock(editor, field) : selectedBlock(editor)) : null,
        hover: editor.isEditable && hoveredId ? measure(findBlock(editor, hoveredId)) : null,
      });
    };
    const schedule = () => { if (!frame) frame = requestAnimationFrame(update); };
    const hover = (event: PointerEvent) => {
      if (event.pointerType === "touch") { hoveredId = null; schedule(); return; }
      const element = event.target instanceof Element ? event.target.closest<HTMLElement>("[data-id]") : null;
      hoveredId = element && root.contains(element) ? element.dataset.id ?? null : null;
      schedule();
    };
    const leave = () => { hoveredId = null; schedule(); };
    const resize = new ResizeObserver(schedule);
    resize.observe(root);
    editor.on("transaction", schedule);
    editor.on("update", schedule);
    editor.on("selectionUpdate", leave);
    root.addEventListener("pointerover", hover);
    root.addEventListener("load", schedule, true);
    host.addEventListener("pointerleave", leave);
    host.addEventListener("focusin", schedule);
    host.addEventListener("focusout", schedule);
    window.addEventListener("scroll", schedule, true);
    window.addEventListener("resize", schedule);
    schedule();
    return () => {
      cancelAnimationFrame(frame);
      resize.disconnect();
      editor.off("transaction", schedule);
      editor.off("update", schedule);
      editor.off("selectionUpdate", leave);
      root.removeEventListener("pointerover", hover);
      root.removeEventListener("load", schedule, true);
      host.removeEventListener("pointerleave", leave);
      host.removeEventListener("focusin", schedule);
      host.removeEventListener("focusout", schedule);
      window.removeEventListener("scroll", schedule, true);
      window.removeEventListener("resize", schedule);
    };
  }, [editor]);

  const target = targets.hover ?? targets.active;
  const Icon = target ? icons[target.type as keyof typeof icons] ?? Square : Square;
  const select = () => {
    if (!target || editor.isDestroyed || !editor.isEditable) return;
    // Resolve the ID again: collaboration may have moved or deleted this block.
    const block = findBlock(editor, target.id);
    if (block && NodeSelection.isSelectable(block.node)) editor.chain().focus().setNodeSelection(block.pos).run();
  };
  const outline = (block: Target, active: boolean) => <div key={active ? "active" : "hover"} aria-hidden
    data-block-indicator={active ? "active" : "hover"} data-block-id={block.id}
    className={`anby-block-indicator${active ? " anby-block-indicator--active" : ""}`}
    style={{ top: block.top, left: block.left, width: block.width, height: block.height }} />;
  return <div ref={anchor} className="anby-block-gutter-layer">
    {targets.active && outline(targets.active, true)}
    {targets.hover && targets.hover.id !== targets.active?.id && outline(targets.hover, false)}
    {target && <Tooltip.Provider delayDuration={150} disableHoverableContent><Tooltip.Root>
      <Tooltip.Trigger asChild><Button type="button" variant="ghost" size="icon-xs" className="anby-block-gutter"
        style={{ top: target.top }} data-block-id={target.id} data-block-path={target.path}
        aria-label={`选中${target.label}块`} onMouseDown={event => event.preventDefault()} onClick={select}>
        <Icon aria-hidden className="size-3.5" />
      </Button></Tooltip.Trigger>
      <Tooltip.Portal><Tooltip.Content side="right" align="start" sideOffset={6} className="anby-block-gutter-tooltip">
        {target.path}<span>点击选中整块</span>
      </Tooltip.Content></Tooltip.Portal>
    </Tooltip.Root></Tooltip.Provider>}
  </div>;
}
