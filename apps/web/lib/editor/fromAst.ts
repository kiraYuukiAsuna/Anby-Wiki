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
  underline: "underline",
  strikethrough: "strike",
  code: "code",
};

function inlineNodeToBN(node: InlineNode): Record<string, unknown> {
  switch (node.type) {
    case "text": {
      const styles: Record<string, boolean | string> = {};
      for (const mark of node.marks ?? []) {
        styles[STYLE_BY_MARK[mark]] = true;
      }
      if (node.text_color) {
        styles.textColor = node.text_color;
      }
      if (node.background_color) {
        styles.backgroundColor = node.background_color;
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
    case "page_anchor_reference":
      return {
        type: "pageAnchorReference",
        props: {
          targetPageId: node.target_page_id,
          targetHeadingBlockId: node.target_heading_block_id,
          displayText: node.display_text,
        },
      };
    case "math":
      return {
        type: "math",
        props: { expression: node.expression },
      };
    case "mention":
      return {
        type: "mention",
        props: { actorId: node.actor_id, displayText: node.display_text },
      };
    case "date_mention":
      return {
        type: "dateMention",
        props: {
          date: node.date,
          endDate: node.end_date ?? "",
          displayText: node.display_text,
        },
      };
    case "database_mention":
      return {
        type: "databaseMention",
        props: {
          datasetId: node.dataset_id,
          displayText: node.display_text,
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
        props: {
          level: block.level,
          isToggleable: block.is_toggleable ?? false,
        },
        content: inlinesToBN(block.content),
        children: blocksToBN(block.children ?? [], state),
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
      if (block.caption) {
        state.codeCaptions.set(block.id, block.caption);
      }
      return {
        id: block.id,
        type: "codeBlock",
        props: { language: block.language ?? "" },
        content: [{ type: "text", text: block.content, styles: {} }],
      };
    case "preformatted":
      return {
        id: block.id,
        type: "preformatted",
        props: {
          kind: block.kind,
          caption: block.caption ?? "",
          text: block.content,
        },
      };
    case "equation":
      return {
        id: block.id,
        type: "equation",
        props: {
          expression: block.expression,
          caption: block.caption ?? "",
        },
      };
    case "todo":
      return {
        id: block.id,
        type: "checkListItem",
        props: { checked: block.checked },
        content: inlinesToBN(block.content),
        children: blocksToBN(block.children ?? [], state),
      };
    case "toggle":
      return {
        id: block.id,
        type: "toggleListItem",
        content: inlinesToBN(block.content),
        children: blocksToBN(block.children ?? [], state),
      };
    case "breadcrumb":
      return {
        id: block.id,
        type: "breadcrumb",
        props: { itemsJson: JSON.stringify(block.items) },
      };
    case "column_list":
      return {
        id: block.id,
        type: "columnList",
        children: blocksToBN(block.children, state),
      };
    case "column":
      return {
        id: block.id,
        type: "column",
        props: { widthRatio: block.width_ratio },
        children: blocksToBN(block.children, state),
      };
    case "button":
      return {
        id: block.id,
        type: "pageButton",
        props: {
          label: block.label,
          confirmation: block.confirmation ?? "",
          actionsJson: JSON.stringify(block.actions),
        },
      };
    case "html":
      return {
        id: block.id,
        type: "htmlBlock",
        props: {
          html: block.html,
          height: block.height,
          caption: block.caption ?? "",
        },
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
        props: {
          kind: block.kind,
          icon: block.icon ?? "",
          color: block.color ?? "default",
        },
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
    case "image":
      return {
        id: block.id,
        type: "imageAsset",
        props: {
          assetRevisionId: block.asset_revision_id,
          altText: block.alt_text,
          caption: block.caption ?? "",
        },
      };
    case "video":
      return {
        id: block.id,
        type: "videoAsset",
        props: {
          assetRevisionId: block.asset_revision_id,
          posterAssetRevisionId: block.poster_asset_revision_id ?? "",
          title: block.title ?? "",
          caption: block.caption ?? "",
        },
      };
    case "audio":
      return {
        id: block.id,
        type: "audioAsset",
        props: {
          assetRevisionId: block.asset_revision_id,
          title: block.title,
          artist: block.artist ?? "",
          album: block.album ?? "",
          durationSeconds: block.duration_seconds ?? 0,
          caption: block.caption ?? "",
        },
      };
    case "file":
      return {
        id: block.id,
        type: "fileAsset",
        props: {
          assetRevisionId: block.asset_revision_id,
          name: block.name,
          caption: block.caption ?? "",
        },
      };
    case "pdf":
      return {
        id: block.id,
        type: "pdfAsset",
        props: {
          assetRevisionId: block.asset_revision_id,
          title: block.title ?? "",
          startPage: block.start_page ?? 1,
          height: block.height,
          caption: block.caption ?? "",
        },
      };
    case "bookmark":
      return {
        id: block.id,
        type: "bookmark",
        props: {
          url: block.url,
          title: block.title,
          description: block.description ?? "",
          siteName: block.site_name ?? "",
          caption: block.caption ?? "",
        },
      };
    case "link_preview":
      return {
        id: block.id,
        type: "linkPreview",
        props: {
          url: block.url,
          title: block.title,
          description: block.description ?? "",
          siteName: block.site_name ?? "",
          caption: "",
        },
      };
    case "dataset_view":
      return {
        id: block.id,
        type: "datasetView",
        props: { datasetViewId: block.dataset_view_id },
      };
    case "embed":
      return {
        id: block.id,
        type: "embed",
        props: {
          url: block.url,
          title: block.title ?? "",
          provider: block.provider ?? "none",
          caption: block.caption ?? "",
        },
      };
    case "table":
      return {
        id: block.id,
        type: "astTable",
        props: {
          caption: block.caption ?? "",
          hasColumnHeader: block.has_column_header ?? false,
          hasRowHeader: block.has_row_header ?? false,
        },
        children: blocksToBN(block.children, state),
      };
    case "table_row":
      return { id: block.id, type: "astTableRow", children: blocksToBN(block.children, state) };
    case "table_cell":
      return { id: block.id, type: "astTableCell", children: blocksToBN(block.children, state) };
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
