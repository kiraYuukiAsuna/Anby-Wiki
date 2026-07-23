// 引用节点（pageReference / external link）的插入与编辑 helper（M2-T04）。
//
// 业务组件只经这些函数触碰 BlockNote 编辑器（ADR-0005 边界：BlockNote API
// 不出 lib/editor 与 components/editor/block-editor.tsx）。自定义 inline content
// 的 props 结构与 fromAst/toAst 的映射保持一致（未写出的 key 一律显式置空，
// 保证 toAst 逐字段读取时不产生歧义）。
import { normalizeTitle } from "@/lib/ast/title";

/** helper 依赖的最小编辑器接口（由 BlockEditor 传入 BlockNoteEditor 实例）。 */
export interface InlineContentEditor {
  insertInlineContent(
    content: unknown,
    options?: { updateSelection?: boolean },
  ): void;
  getSelectedText(): string;
  prosemirrorState: {
    selection: unknown;
    tr: {
      setNodeAttribute(pos: number, name: string, value: unknown): unknown;
    };
  };
  prosemirrorView: {
    dispatch(tr: unknown): void;
  };
}

/** pageReference props 全集（与 lib/editor/schema.ts 的 propSchema 一致）。 */
interface PageReferenceProps {
  targetPageId: string;
  targetHeadingBlockId: string;
  displayText: string;
  resolutionStatus: string;
  targetNamespace: string;
  normalizedTitle: string;
  expectedEntityType: string;
}

function pageReferenceInlineContent(props: PageReferenceProps) {
  return { type: "pageReference", props };
}

/**
 * 在光标处插入已解析页面引用（AST page_reference 已解析形态：
 * target_page_id + display_text，两者分离，display_text 可独立编辑）。
 */
export function insertResolvedPageReference(
  editor: InlineContentEditor,
  targetPageId: string,
  displayText: string,
): void {
  editor.insertInlineContent([
    pageReferenceInlineContent({
      targetPageId,
      targetHeadingBlockId: "",
      displayText,
      resolutionStatus: "",
      targetNamespace: "",
      normalizedTitle: "",
      expectedEntityType: "",
    }),
  ]);
}

/**
 * 在光标处插入未解析页面引用（AST page_reference 未解析形态：
 * resolution_status=unresolved + target_namespace + normalized_title）。
 * rawTitle 为用户输入原文，这里完成与后端一致的规范化（NFC/空白折叠/小写）。
 */
export function insertUnresolvedPageReference(
  editor: InlineContentEditor,
  rawTitle: string,
  targetNamespace = "main",
): void {
  editor.insertInlineContent([
    pageReferenceInlineContent({
      targetPageId: "",
      targetHeadingBlockId: "",
      displayText: "",
      resolutionStatus: "unresolved",
      targetNamespace,
      normalizedTitle: normalizeTitle(rawTitle),
      expectedEntityType: "",
    }),
  ]);
}

/**
 * 在光标处插入外部链接（AST external_link：url + display_text 分离）。
 * 若有选中文本，插入会替换选区。
 */
export function insertExternalLink(
  editor: InlineContentEditor,
  url: string,
  displayText: string,
): void {
  editor.insertInlineContent([
    { type: "link", href: url, content: displayText },
  ]);
}

/** 当前选区选中的 pageReference 节点信息；未选中返回 null。 */
export interface SelectedPageReference {
  /** ProseMirror 文档中节点的位置（NodeSelection.from）。 */
  pos: number;
  /** 已解析形态（resolutionStatus 非 "unresolved"）。 */
  resolved: boolean;
  targetPageId: string;
  displayText: string;
  normalizedTitle: string;
}

interface NodeSelectionLike {
  node?: {
    type: { name: string };
    attrs: Record<string, unknown>;
  };
  from?: number;
}

/**
 * 读取当前选区：若选中的是 pageReference 原子节点（点击引用产生的
 * NodeSelection），返回其位置与 props；否则返回 null。
 */
export function getSelectedPageReference(
  editor: InlineContentEditor,
): SelectedPageReference | null {
  const sel = editor.prosemirrorState.selection as NodeSelectionLike;
  if (!sel?.node || sel.node.type.name !== "pageReference") return null;
  const attrs = sel.node.attrs;
  const unresolved = String(attrs.resolutionStatus ?? "") === "unresolved";
  return {
    pos: typeof sel.from === "number" ? sel.from : -1,
    resolved: !unresolved,
    targetPageId: String(attrs.targetPageId ?? ""),
    displayText: String(attrs.displayText ?? ""),
    normalizedTitle: String(attrs.normalizedTitle ?? ""),
  };
}

/**
 * 更新选中 pageReference 的 display_text（只改显示文本，不动 target_page_id）。
 * 未解析形态没有 display_text 字段（显示即 normalized_title），返回 false。
 * 选区不是 pageReference 时同样返回 false。
 */
export function updateSelectedPageReferenceDisplayText(
  editor: InlineContentEditor,
  displayText: string,
): boolean {
  const selected = getSelectedPageReference(editor);
  if (!selected || !selected.resolved || selected.pos < 0) return false;
  const tr = editor.prosemirrorState.tr.setNodeAttribute(
    selected.pos,
    "displayText",
    displayText,
  );
  editor.prosemirrorView.dispatch(tr);
  return true;
}
