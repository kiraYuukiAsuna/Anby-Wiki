import type { DocumentDiff, BlockChange } from "../../../../contracts/generated/typescript";
import type { Block, Document } from "../ast/schema";

export type IndexedBlock = { block: Block; parent: string; ancestors: string[] };
export type VisualChange = { id: string; types: BlockChange["type"][]; label: string; excerpt: string; fields: string[] };
export const changeLabels = { added: "新增", removed: "删除", changed: "修改", moved: "移动" };
const blockLabels: Record<Block["type"], string> = {
  paragraph: "正文", heading: "标题", bullet_list: "无序列表", ordered_list: "有序列表", list_item: "列表项",
  table: "表格", table_row: "表格行", table_cell: "单元格", code: "代码", preformatted: "等宽文本", equation: "公式",
  todo: "待办", toggle: "折叠块", breadcrumb: "页面路径", column_list: "分栏", column: "栏", button: "页面按钮",
  html: "HTML", quote: "引用", callout: "提示框", component: "知识组件", image: "图片", video: "视频", audio: "音频",
  file: "文件", pdf: "PDF", bookmark: "书签", link_preview: "链接预览", embed: "嵌入内容", divider: "分割线",
};

export function indexDocument(document: Document): Map<string, IndexedBlock> {
  const index = new Map<string, IndexedBlock>();
  const visit = (blocks: Block[], ancestors: string[]) => {
    for (const block of blocks) {
      index.set(block.id, { block, parent: ancestors.at(-1) ?? "", ancestors });
      if ("children" in block) visit(block.children ?? [], [...ancestors, block.id]);
    }
  };
  visit(document.children, []);
  return index;
}

// Unique block IDs reduce sibling LCS to an O(n log n) increasing subsequence.
function stationaryIDs(before: string[], after: string[]): Set<string> {
  const positions = new Map(before.map((id, i) => [id, i]));
  const tails: number[] = [], previous = new Array<number>(after.length).fill(-1);
  after.forEach((id, i) => {
    const value = positions.get(id)!;
    let lo = 0, hi = tails.length;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (positions.get(after[tails[mid]])! < value) lo = mid + 1;
      else hi = mid;
    }
    previous[i] = lo > 0 ? tails[lo - 1] : -1;
    tails[lo] = i;
  });
  const result = new Set<string>();
  for (let i = tails.at(-1) ?? -1; i >= 0; i = previous[i]) result.add(after[i]);
  return result;
}

function excerpt(block: Block): string {
  if ("content" in block) {
    if (typeof block.content === "string") return block.content.slice(0, 70);
    return block.content.map(node => "text" in node ? node.text : "display_text" in node ? node.display_text : "").join("").slice(0, 70);
  }
  for (const key of ["caption", "title", "label", "name", "alt_text"] as const) {
    if (key in block && typeof block[key as keyof Block] === "string") return String(block[key as keyof Block]).slice(0, 70);
  }
  if ("children" in block) return (block.children ?? []).map(excerpt).filter(Boolean).join(" · ").slice(0, 70);
  return "";
}

export function buildVisualChanges(before: Map<string, IndexedBlock>, after: Map<string, IndexedBlock>, diff: DocumentDiff): VisualChange[] {
  const changes = new Map<string, Set<BlockChange["type"]>>();
  const fields = new Map<string, string[]>();
  const add = (id: string, type: BlockChange["type"]) => {
    if (!changes.has(id)) changes.set(id, new Set());
    changes.get(id)!.add(type);
  };
  for (const change of diff.changes) {
    if (change.type === "moved") continue;
    const index = change.type === "removed" ? before : after;
    const other = change.type === "removed" ? after : before;
    const entry = index.get(change.blockId);
    if (!entry) continue;
    if ((change.type === "added" || change.type === "removed") && entry.ancestors.some(id => !other.has(id))) continue;
    add(change.blockId, change.type);
    if (change.fields) fields.set(change.blockId, change.fields.map(field => field.field));
  }
  const oldSiblings = new Map<string, string[]>(), newSiblings = new Map<string, string[]>();
  for (const [id, entry] of before) {
    if (after.get(id)?.parent !== entry.parent) continue;
    if (!oldSiblings.has(entry.parent)) oldSiblings.set(entry.parent, []);
    oldSiblings.get(entry.parent)!.push(id);
  }
  for (const [id, entry] of after) {
    const old = before.get(id);
    if (!old) continue;
    if (old.parent !== entry.parent) { add(id, "moved"); continue; }
    if (!newSiblings.has(entry.parent)) newSiblings.set(entry.parent, []);
    newSiblings.get(entry.parent)!.push(id);
  }
  for (const [parent, ids] of newSiblings) {
    const stationary = stationaryIDs(oldSiblings.get(parent) ?? [], ids);
    for (const id of ids) if (!stationary.has(id)) add(id, "moved");
  }
  const order = [...new Set([...after.keys(), ...before.keys()])];
  return order.flatMap(id => {
    const types = changes.get(id);
    if (!types) return [];
    const entry = after.get(id) ?? before.get(id)!;
    // A changed leaf in a table still names its surrounding table context.
    const context = entry.ancestors.map(parent => (after.get(parent) ?? before.get(parent))?.block.type);
    const label = context.includes("table_cell") ? "表格 · 单元格" : blockLabels[entry.block.type];
    return [{ id, types: [...types], label, excerpt: excerpt(entry.block), fields: fields.get(id) ?? [] }];
  });
}

export type TextRange = [number, number];
export function changedTextRanges(before: string, after: string): { before: TextRange[]; after: TextRange[] } {
  const segmenter = new Intl.Segmenter("zh", { granularity: "grapheme" });
  const split = (s: string) => Array.from(segmenter.segment(s), item => item.segment);
  const a = split(before), b = split(after);
  let start = 0, endA = a.length, endB = b.length;
  while (start < endA && start < endB && a[start] === b[start]) start++;
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) { endA--; endB--; }
  const removed = new Set<number>(), added = new Set<number>();
  const n = endA - start, m = endB - start;
  // shortcut: bounded inline LCS; huge replacements use one range until a worker diff is needed.
  if (n * m > 250_000) {
    for (let i = start; i < endA; i++) removed.add(i);
    for (let j = start; j < endB; j++) added.add(j);
  } else {
    const dp = new Uint32Array((n + 1) * (m + 1));
    for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) {
      dp[i * (m + 1) + j] = a[start + i] === b[start + j] ? 1 + dp[(i + 1) * (m + 1) + j + 1] : Math.max(dp[(i + 1) * (m + 1) + j], dp[i * (m + 1) + j + 1]);
    }
    let i = 0, j = 0;
    while (i < n || j < m) {
      if (i < n && j < m && a[start + i] === b[start + j]) { i++; j++; }
      else if (i < n && (j === m || dp[(i + 1) * (m + 1) + j] >= dp[i * (m + 1) + j + 1])) removed.add(start + i++);
      else added.add(start + j++);
    }
  }
  const ranges = (tokens: string[], changed: Set<number>): TextRange[] => {
    const result: TextRange[] = [];
    let offset = 0;
    tokens.forEach((token, i) => {
      if (changed.has(i)) {
        const last = result.at(-1);
        if (last && last[1] === offset) last[1] += token.length;
        else result.push([offset, offset + token.length]);
      }
      offset += token.length;
    });
    return result;
  };
  return { before: ranges(a, removed), after: ranges(b, added) };
}
