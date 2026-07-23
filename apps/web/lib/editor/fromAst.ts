// AST v1 → BlockNote document（M2-T02 / ADR-0005）。
//
// ID 策略：AST block id 直接写入 BlockNote PartialBlock.id（BlockNote 允许
// 外部传入 id，UniqueID 扩展只处理缺失/重复 id，不会改写），因此既有块
// 序列化往返后 ID 不变。
//
// 结构适配规则（与 toAst 互逆）：
//   - bullet_list / ordered_list 摊平为连续 list item 块；list 容器 id 记入
//     state.listIds（key = list_item id），供 toAst 还原。
//   - quote / list_item 的首个 paragraph 子块提升为 BlockNote 块的行内
//     content，其 AST id 记入 state.liftedParagraphIds（key = 容器块 id）。
import type {
  Block,
  Document,
  InlineNode,
  Mark,
} from "@/lib/ast/schema";
import { parseDocument } from "@/lib/ast/schema";

import { createAdapterState, type AdapterState } from "./ids";
import type { BNPartialBlock } from "./schema";

const STYLE_BY_MARK: Record<Mark, string> = {
  bold: "bold",
  italic: "italic",
  strikethrough: "strike",
  code: "code",
};

function inlineNodeToBN(node: InlineNode): Record<string, unknown> {
  switch (node.type) {
    case "text": {
      const styles: Record<string, boolean> = {};
      for (const mark of node.marks ?? []) {
        styles[STYLE_BY_MARK[mark]] = true;
      }
      return { type: "text", text: node.text, styles };
    }
    case "inline_code":
      // 专用自定义 style，避免与 code mark 混淆（保证往返无损）。
      return { type: "text", text: node.text, styles: { inlineCode: true } };
    case "external_link":
      return {
        type: "link",
        href: node.url,
        content: [{ type: "text", text: node.display_text, styles: {} }],
      };
    case "entity_reference":
      return {
        type: "entityReference",
        props: { entityId: node.entity_id, displayText: node.display_text },
      };
    case "claim_reference":
      return {
        type: "claimReference",
        props: { claimId: node.claim_id, displayText: node.display_text },
      };
    case "citation_reference":
      return {
        type: "citationReference",
        props: {
          citationId: node.citation_id,
          displayText: node.display_text ?? "",
        },
      };
    case "page_reference":
      if ("resolution_status" in node) {
        return {
          type: "pageReference",
          props: {
            targetPageId: "",
            targetHeadingBlockId: "",
            displayText: "",
            resolutionStatus: "unresolved",
            targetNamespace: node.target_namespace,
            normalizedTitle: node.normalized_title,
            expectedEntityType: node.expected_entity_type ?? "",
          },
        };
      }
      return {
        type: "pageReference",
        props: {
          targetPageId: node.target_page_id,
          targetHeadingBlockId: node.target_heading_block_id ?? "",
          displayText: node.display_text,
          resolutionStatus: "",
          targetNamespace: "",
          normalizedTitle: "",
          expectedEntityType: "",
        },
      };
  }
}

function inlinesToBN(nodes: InlineNode[]): Record<string, unknown>[] {
  return nodes.map(inlineNodeToBN);
}

/**
 * 把容器（quote / list_item）的首个 paragraph 子块提升为行内 content，
 * paragraph 的 AST id 记入 state（key = 容器块 id），其余子块保持嵌套。
 */
function liftFirstParagraph(
  children: Block[],
  containerId: string,
  state: AdapterState,
): { content: Record<string, unknown>[]; rest: Block[] } {
  const [first, ...rest] = children;
  if (first && first.type === "paragraph") {
    state.liftedParagraphIds.set(containerId, first.id);
    return { content: inlinesToBN(first.content), rest };
  }
  return { content: [], rest: children };
}

function listItemToBN(
  item: Extract<Block, { type: "list_item" }>,
  bnType: "bulletListItem" | "numberedListItem",
  state: AdapterState,
): BNPartialBlock {
  const { content, rest } = liftFirstParagraph(item.children, item.id, state);
  return {
    id: item.id,
    type: bnType,
    content,
    children: blocksToBN(rest, state),
  };
}

function blockToBN(block: Block, state: AdapterState): BNPartialBlock {
  switch (block.type) {
    case "heading":
      return {
        id: block.id,
        type: "heading",
        props: { level: block.level },
        content: inlinesToBN(block.content),
      };
    case "paragraph":
      return {
        id: block.id,
        type: "paragraph",
        content: inlinesToBN(block.content),
      };
    case "list_item":
      // list_item 正常只出现在 list 内（由 blocksToBN 处理）；
      // 顶层孤立的 list_item（schema 允许）降级为 bulletListItem，
      // toAst 时会被重新包进新的 bullet_list 容器。
      return listItemToBN(block, "bulletListItem", state);
    case "code":
      return {
        id: block.id,
        type: "codeBlock",
        props: { language: block.language ?? "" },
        content: [{ type: "text", text: block.content, styles: {} }],
      };
    case "quote": {
      const { content, rest } = liftFirstParagraph(
        block.children,
        block.id,
        state,
      );
      return {
        id: block.id,
        type: "quote",
        content,
        children: blocksToBN(rest, state),
      };
    }
    case "callout":
      return {
        id: block.id,
        type: "callout",
        props: { kind: block.kind },
        children: blocksToBN(block.children, state),
      };
    case "component":
      return {
        id: block.id,
        type: "component",
        props: {
          componentId: block.component_id,
          componentVersion: block.component_version,
          entityId: block.entity_id,
          displayConfig: JSON.stringify(block.display_config),
        },
      };
    case "table":
      return { id: block.id, type: "table", children: blocksToBN(block.children, state) };
    case "table_row":
      return { id: block.id, type: "tableRow", children: blocksToBN(block.children, state) };
    case "table_cell":
      return { id: block.id, type: "tableCell", children: blocksToBN(block.children, state) };
    case "divider":
      return { id: block.id, type: "divider" };
    case "bullet_list":
    case "ordered_list":
      // 由 blocksToBN 摊平处理，不会到达这里。
      throw new Error(`fromAst: 内部错误，${block.type} 应在数组层处理`);
  }
}

function blocksToBN(blocks: Block[], state: AdapterState): BNPartialBlock[] {
  const out: BNPartialBlock[] = [];
  for (const block of blocks) {
    if (block.type === "bullet_list" || block.type === "ordered_list") {
      const bnType =
        block.type === "bullet_list" ? "bulletListItem" : "numberedListItem";
      for (const item of block.children) {
        state.listIds.set(item.id, block.id);
        out.push(listItemToBN(item, bnType, state));
      }
    } else {
      out.push(blockToBN(block, state));
    }
  }
  return out;
}

/**
 * AST v1 Document → BlockNote PartialBlock[]（可直接作为 initialContent）。
 * 输入先经 parseDocument 校验；state 必须与后续 toAst 共享以保证 ID 稳定。
 */
export function fromAst(
  input: Document | unknown,
  state: AdapterState = createAdapterState(),
): BNPartialBlock[] {
  const doc = parseDocument(input);
  return blocksToBN(doc.children, state);
}
