import { changedTextRanges, type IndexedBlock, type TextRange, type VisualChange } from "./visual-diff";

export function blockElements(root: HTMLElement): Map<string, HTMLElement> {
  return new Map(Array.from(root.querySelectorAll<HTMLElement>("[data-history-block-id]"), el => [el.dataset.historyBlockId!, el]));
}

function ownTextNodes(element: HTMLElement): Text[] {
  const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
  const nodes: Text[] = [];
  while (walker.nextNode()) {
    const node = walker.currentNode as Text;
    if (node.parentElement?.closest("[data-history-block-id]") !== element) continue;
    // Live claim values and citation numbering are not immutable revision text.
    if (node.parentElement?.closest(".claim-reference, [data-claim-ref], [data-citation-ref]")) continue;
    nodes.push(node);
  }
  return nodes;
}

function markText(nodes: Text[], ranges: TextRange[], side: string) {
  let offset = 0;
  for (const node of nodes) {
    const value = node.data, start = offset;
    offset += value.length;
    const matches = ranges.filter(([a, b]) => a < offset && b > start);
    if (!matches.length) continue;
    const fragment = document.createDocumentFragment();
    let cursor = 0;
    for (const [a, b] of matches) {
      const from = Math.max(0, a - start), to = Math.min(value.length, b - start);
      fragment.append(value.slice(cursor, from));
      const mark = document.createElement("mark");
      mark.dataset.historyText = side;
      mark.textContent = value.slice(from, to);
      fragment.append(mark);
      cursor = to;
    }
    fragment.append(value.slice(cursor));
    node.replaceWith(fragment);
  }
}

export function decorateComparison(roots: [HTMLElement, HTMLElement], html: [string, string], changes: VisualChange[], indexes: [Map<string, IndexedBlock>, Map<string, IndexedBlock>]) {
  const sides = ["before", "after"];
  const maps = roots.map((root, i) => {
    // Only HTML returned by the fixed backend reading renderer enters this boundary.
    root.innerHTML = html[i];
    const prefix = `history-${sides[i]}-`;
    for (const el of root.querySelectorAll<HTMLElement>("[id]")) el.id = prefix + el.id;
    for (const link of root.querySelectorAll<HTMLAnchorElement>('a[href^="#"]')) link.setAttribute("href", `#${prefix}${link.getAttribute("href")!.slice(1)}`);
    for (const el of root.querySelectorAll<HTMLElement>("a, button, iframe, input")) el.tabIndex = -1;
    for (const button of root.querySelectorAll<HTMLButtonElement>("[data-page-button]")) button.disabled = true;
    return blockElements(root);
  });
  for (const change of changes) {
    maps.forEach((map, i) => {
      const el = map.get(change.id);
      if (!el) return;
      el.dataset.historyChange = change.types.join(" ");
      el.title = `${change.label} · ${change.types.map(type => ({ added: "新增", removed: "删除", changed: "修改", moved: "移动" })[type]).join("、")}`;
      if (el instanceof HTMLDetailsElement) el.open = true;
      for (const ancestor of indexes[i].get(change.id)?.ancestors ?? []) {
        const container = map.get(ancestor);
        if (container instanceof HTMLDetailsElement) container.open = true;
      }
    });
    if (!change.types.includes("changed")) continue;
    const old = maps[0].get(change.id), next = maps[1].get(change.id);
    const block = indexes[1].get(change.id)?.block;
    if (!old || !next || !block || !["paragraph", "heading", "toggle", "todo", "code", "preformatted"].includes(block.type)) continue;
    const oldNodes = ownTextNodes(old), nextNodes = ownTextNodes(next);
    const ranges = changedTextRanges(oldNodes.map(n => n.data).join(""), nextNodes.map(n => n.data).join(""));
    markText(oldNodes, ranges.before, "before");
    markText(nextNodes, ranges.after, "after");
  }
}

export function filterComparison(root: HTMLElement, index: Map<string, IndexedBlock>, changes: VisualChange[], onlyChanges: boolean) {
  const keep = new Set<string>(), whole = new Set<string>();
  for (const change of changes) {
    const entry = index.get(change.id);
    if (!entry) continue;
    whole.add(change.id);
    keep.add(change.id);
    for (const id of entry.ancestors) {
      keep.add(id);
      // Keep table geometry and column context instead of collapsing unmatched cells.
      if (["table", "column_list"].includes(index.get(id)!.block.type)) whole.add(id);
    }
  }
  for (const [id, el] of blockElements(root)) {
    const entry = index.get(id);
    el.toggleAttribute("data-history-hidden", onlyChanges && !!entry && entry.block.type !== "heading" && !keep.has(id) && !entry.ancestors.some(parent => whole.has(parent)));
  }
}
