// 从 AST v1 文档提取目录（TOC）：收集 heading block 的锚点与纯文本标题。
import type { Block, Document, InlineNode } from "./schema";

/** 目录条目：id 即 heading block 的锚点 id（渲染时 id={block.id}）。 */
export interface TocEntry {
  id: string;
  level: number;
  text: string;
}

/** 把 inline 节点序列拍平为纯文本（未解析引用显示其规范化标题）。 */
export function inlineToPlainText(nodes: InlineNode[]): string {
  return nodes
    .map((node) => {
      switch (node.type) {
        case "text":
        case "inline_code":
          return node.text;
        case "page_reference":
          return "resolution_status" in node
            ? node.normalized_title
            : node.display_text;
        case "external_link":
          return node.display_text;
      }
    })
    .join("");
}

function collectHeadings(blocks: Block[], out: TocEntry[]): void {
  for (const block of blocks) {
    if (block.type === "heading") {
      out.push({
        id: block.id,
        level: block.level,
        text: inlineToPlainText(block.content),
      });
      continue;
    }
    if ("children" in block) {
      collectHeadings(block.children, out);
    }
  }
}

/** 提取文档中全部 heading（含嵌套在 list/quote/callout/table 内的），保持文档顺序。 */
export function extractToc(doc: Document): TocEntry[] {
  const out: TocEntry[] = [];
  collectHeadings(doc.children, out);
  return out;
}
