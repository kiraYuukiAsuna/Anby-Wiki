import type { JSONContent } from "@tiptap/core";

import type { Block, Document, InlineNode, Mark } from "@/lib/ast/schema";
import { parseDocument } from "@/lib/ast/schema";
import { createAdapterState, type AdapterState } from "./ids";

const MARK_TO_TIPTAP: Record<Mark, string> = {
  bold: "bold",
  italic: "italic",
  underline: "underline",
  strikethrough: "strike",
  code: "code",
};

type PMMark = { type: string; attrs?: Record<string, unknown> };

function textNode(text: string, marks: PMMark[] = []): JSONContent | null {
  if (!text) return null;
  return marks.length > 0 ? { type: "text", text, marks } : { type: "text", text };
}

function inlineToTiptap(node: InlineNode): JSONContent[] {
  switch (node.type) {
    case "text": {
      const marks: PMMark[] = (node.marks ?? []).map((mark) => ({ type: MARK_TO_TIPTAP[mark] }));
      if (node.text_color) marks.push({ type: "textColor", attrs: { color: node.text_color } });
      if (node.background_color) {
        marks.push({ type: "backgroundColor", attrs: { color: node.background_color } });
      }
      const out = textNode(node.text, marks);
      return out ? [out] : [];
    }
    case "inline_code": {
      const out = textNode(node.text, [{ type: "inlineCode" }]);
      return out ? [out] : [];
    }
    case "external_link": {
      const out = textNode(node.display_text, [{ type: "link", attrs: { href: node.url } }]);
      return out ? [out] : [];
    }
    case "page_reference":
      if ("resolution_status" in node) {
        return [{
          type: "pageReference",
          attrs: {
            targetPageId: "",
            targetHeadingBlockId: "",
            displayText: "",
            resolutionStatus: "unresolved",
            targetNamespace: node.target_namespace,
            normalizedTitle: node.normalized_title,
            expectedEntityType: node.expected_entity_type ?? "",
          },
        }];
      }
      return [{
        type: "pageReference",
        attrs: {
          targetPageId: node.target_page_id,
          targetHeadingBlockId: node.target_heading_block_id ?? "",
          displayText: node.display_text,
          resolutionStatus: "",
          targetNamespace: "",
          normalizedTitle: "",
          expectedEntityType: "",
        },
      }];
    case "page_anchor_reference":
      return [{ type: "pageAnchorReference", attrs: {
        targetPageId: node.target_page_id,
        targetHeadingBlockId: node.target_heading_block_id,
        displayText: node.display_text,
      } }];
    case "entity_reference":
      return [{ type: "entityReference", attrs: { entityId: node.entity_id, displayText: node.display_text } }];
    case "claim_reference":
      return [{ type: "claimReference", attrs: { claimId: node.claim_id, displayText: node.display_text } }];
    case "citation_reference":
      return [{ type: "citationReference", attrs: { citationId: node.citation_id, displayText: node.display_text ?? "" } }];
    case "math":
      return [{ type: "mathInline", attrs: { expression: node.expression } }];
    case "mention":
      return [{ type: "mention", attrs: { actorId: node.actor_id, displayText: node.display_text } }];
    case "date_mention":
      return [{ type: "dateMention", attrs: {
        date: node.date,
        endDate: node.end_date ?? "",
        displayText: node.display_text,
      } }];
  }
}

function inlinesToTiptap(nodes: InlineNode[]): JSONContent[] {
  return nodes.flatMap(inlineToTiptap);
}

function paragraphFromInline(id: string | undefined, content: InlineNode[]): JSONContent {
  return { type: "paragraph", attrs: { id }, content: inlinesToTiptap(content) };
}

function textBlock(type: string, id: string, content: InlineNode[], attrs: Record<string, unknown> = {}): JSONContent {
  return { type, attrs: { id, ...attrs }, content: inlinesToTiptap(content) };
}

function textContent(value: string): JSONContent[] {
  const node = textNode(value);
  return node ? [node] : [];
}

function blocksToTiptap(blocks: Block[], state: AdapterState): JSONContent[] {
  const out: JSONContent[] = [];
  for (const block of blocks) {
    out.push(...blockToTiptap(block, state));
  }
  return out;
}

function listItemToTiptap(block: Extract<Block, { type: "list_item" }>, state: AdapterState): JSONContent {
  return { type: "listItem", attrs: { id: block.id }, content: blocksToTiptap(block.children, state) };
}

function blockToTiptap(block: Block, state: AdapterState): JSONContent[] {
  switch (block.type) {
    case "heading": {
      if (block.children !== undefined || block.is_toggleable !== undefined) return [{
        type: "sectionHeading",
        attrs: { id: block.id, level: block.level, isToggleable: block.is_toggleable ?? null, hasChildren: block.children !== undefined },
        content: [paragraphFromInline(undefined, block.content), ...blocksToTiptap(block.children ?? [], state)],
      }];
      return [textBlock("heading", block.id, block.content, { level: block.level })];
    }
    case "paragraph":
      return [paragraphFromInline(block.id, block.content)];
    case "bullet_list":
      return [{ type: "bulletList", attrs: { id: block.id }, content: block.children.map((item) => listItemToTiptap(item, state)) }];
    case "ordered_list":
      return [{ type: "orderedList", attrs: { id: block.id }, content: block.children.map((item) => listItemToTiptap(item, state)) }];
    case "list_item":
      return [listItemToTiptap(block, state)];
    case "todo":
      return [{
        type: "todoBlock",
        attrs: { id: block.id, checked: block.checked },
        content: [paragraphFromInline(undefined, block.content), ...blocksToTiptap(block.children ?? [], state)],
      }];
    case "toggle":
      return [{
        type: "toggleBlock",
        attrs: { id: block.id, hasChildren: block.children !== undefined },
        content: [paragraphFromInline(undefined, block.content), ...blocksToTiptap(block.children ?? [], state)],
      }];
    case "quote":
      return [{ type: "blockquote", attrs: { id: block.id }, content: blocksToTiptap(block.children, state) }];
    case "callout":
      return [{
        type: "callout",
        attrs: { id: block.id, kind: block.kind, icon: block.icon ?? "", color: block.color ?? "default" },
        content: blocksToTiptap(block.children, state),
      }];
    case "code":
      return [{ type: "codeBlock", attrs: { id: block.id, language: block.language ?? "", caption: block.caption ?? "" }, content: textContent(block.content) }];
    case "preformatted":
      return [{ type: "preformatted", attrs: { id: block.id, kind: block.kind, caption: block.caption ?? "", text: block.content } }];
    case "equation":
      return [{ type: "equation", attrs: { id: block.id, expression: block.expression, caption: block.caption ?? "" } }];
    case "breadcrumb":
      return [{ type: "breadcrumb", attrs: { id: block.id, itemsJson: JSON.stringify(block.items) } }];
    case "column_list":
      return [{ type: "columnList", attrs: { id: block.id }, content: blocksToTiptap(block.children, state) }];
    case "column":
      return [{ type: "column", attrs: { id: block.id, widthRatio: block.width_ratio }, content: blocksToTiptap(block.children, state) }];
    case "button":
      return [{ type: "pageButton", attrs: { id: block.id, label: block.label, confirmation: block.confirmation ?? "", actionsJson: JSON.stringify(block.actions) } }];
    case "html":
      return [{ type: "htmlBlock", attrs: { id: block.id, html: block.html, height: block.height, caption: block.caption ?? "" } }];
    case "component":
      return [{ type: "componentBlock", attrs: { id: block.id, componentId: block.component_id, componentVersion: block.component_version, entityId: block.entity_id, displayConfig: JSON.stringify(block.display_config) } }];
    case "image":
      return [{ type: "imageAsset", attrs: { id: block.id, assetRevisionId: block.asset_revision_id, altText: block.alt_text, caption: block.caption ?? "" } }];
    case "video":
      return [{ type: "videoAsset", attrs: { id: block.id, assetRevisionId: block.asset_revision_id, posterAssetRevisionId: block.poster_asset_revision_id ?? "", title: block.title ?? "", caption: block.caption ?? "" } }];
    case "audio":
      return [{ type: "audioAsset", attrs: { id: block.id, assetRevisionId: block.asset_revision_id, title: block.title, artist: block.artist ?? "", album: block.album ?? "", durationSeconds: block.duration_seconds ?? 0, caption: block.caption ?? "" } }];
    case "file":
      return [{ type: "fileAsset", attrs: { id: block.id, assetRevisionId: block.asset_revision_id, name: block.name, caption: block.caption ?? "" } }];
    case "pdf":
      return [{ type: "pdfAsset", attrs: { id: block.id, assetRevisionId: block.asset_revision_id, title: block.title ?? "", startPage: block.start_page ?? 1, height: block.height, caption: block.caption ?? "" } }];
    case "bookmark":
      return [{ type: "bookmark", attrs: { id: block.id, url: block.url, title: block.title, description: block.description ?? "", siteName: block.site_name ?? "", caption: block.caption ?? "" } }];
    case "link_preview":
      return [{ type: "linkPreview", attrs: { id: block.id, url: block.url, title: block.title, description: block.description ?? "", siteName: block.site_name ?? "" } }];
    case "embed":
      return [{ type: "embedBlock", attrs: { id: block.id, url: block.url, title: block.title ?? "", provider: block.provider ?? "none", caption: block.caption ?? "" } }];
    case "table":
      return [{
        type: "astTable",
        attrs: { id: block.id, caption: block.caption ?? "", hasColumnHeader: block.has_column_header ?? false, hasRowHeader: block.has_row_header ?? false },
        content: blocksToTiptap(block.children, state),
      }];
    case "table_row":
      return [{ type: "astTableRow", attrs: { id: block.id }, content: blocksToTiptap(block.children, state) }];
    case "table_cell":
      return [{ type: "astTableCell", attrs: { id: block.id }, content: blocksToTiptap(block.children, state) }];
    case "divider":
      return [{ type: "horizontalRule", attrs: { id: block.id } }];
  }
}

export function fromAst(
  input: Document | unknown,
  state: AdapterState = createAdapterState(),
): JSONContent {
  void state;
  const doc = parseDocument(input);
  return {
    type: "doc",
    content: doc.children.length > 0 ? blocksToTiptap(doc.children, state) : [{ type: "paragraph" }],
  };
}
