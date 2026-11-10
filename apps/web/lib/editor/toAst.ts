import { ZodError } from "zod";

import {
  buttonActionSchema,
  breadcrumbItemSchema,
  parseDocument,
  type Block as AstBlock,
  type ButtonAction,
  type CalloutColor,
  type CalloutKind,
  type Document,
  type InlineColor,
  type InlineNode,
  type Mark,
  type PreformattedKind,
} from "@/lib/ast/schema";
import { AstValidationError, UnsupportedEditorFeatureError } from "./errors";
import { createAdapterState, resolveBlockId, type AdapterState } from "./ids";
import type { PMBlock, PMDocument } from "./schema";

const MARK_BY_TIPTAP: Record<string, Mark> = {
  bold: "bold",
  italic: "italic",
  underline: "underline",
  strike: "strikethrough",
  code: "code",
  inlineCode: "code",
};

const INLINE_COLORS: Record<string, true> = { gray: true, brown: true, red: true, orange: true, yellow: true, green: true, blue: true, purple: true, pink: true };

function blockId(node: PMBlock, state: AdapterState, fallbackKey: string): string {
  const id = node.attrs?.id;
  if (typeof id === "string" && id) return resolveBlockId(id, state);
  return resolveBlockId(`missing:${fallbackKey}:${node.type ?? "block"}`, state);
}

function str(node: PMBlock, key: string): string {
  const value = node.attrs?.[key];
  return typeof value === "string" ? value : "";
}

function num(node: PMBlock, key: string, fallback = 0): number {
  const value = node.attrs?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function bool(node: PMBlock, key: string): boolean {
  return node.attrs?.[key] === true;
}

function textOf(node: PMBlock): string {
  if (node.type === "text") return node.text ?? "";
  return (node.content ?? []).map(textOf).join("");
}

function colorValue(value: unknown, where: string): InlineColor | undefined {
  if (!value) return undefined;
  if (typeof value !== "string" || !INLINE_COLORS[value]) {
    throw new UnsupportedEditorFeatureError(`${where}: 不支持的颜色 ${String(value)}`);
  }
  return value as InlineColor;
}

function marksToInlineText(node: PMBlock, where: string): InlineNode[] {
  const text = node.text ?? "";
  if (!text) return [];
  const marks = node.marks ?? [];
  const link = marks.find((mark) => mark.type === "link");
  if (link) {
    if (marks.length > 1) throw new UnsupportedEditorFeatureError("链接暂不支持叠加文字格式，请先移除链接或取消格式");
    return [{ type: "external_link", url: String(link.attrs?.href ?? ""), display_text: text }];
  }
  if (marks.length === 1 && marks[0].type === "inlineCode") {
    return [{ type: "inline_code", text }];
  }
  const out: InlineNode = { type: "text", text };
  const markNames: Mark[] = [];
  for (const mark of marks) {
    const mapped = MARK_BY_TIPTAP[mark.type];
    if (mapped && !markNames.includes(mapped)) markNames.push(mapped);
    if (!mapped && mark.type !== "textColor" && mark.type !== "backgroundColor") throw new UnsupportedEditorFeatureError(`未知文字格式 ${mark.type}`);
    if (mark.type === "textColor") out.text_color = colorValue(mark.attrs?.color, where);
    if (mark.type === "backgroundColor") out.background_color = colorValue(mark.attrs?.color, where);
  }
  if (markNames.length > 0) out.marks = markNames;
  return [out];
}

function inlineAtom(node: PMBlock): InlineNode {
  switch (node.type) {
    case "pageReference":
      if (str(node, "resolutionStatus") === "unresolved") {
        const out: InlineNode = {
          type: "page_reference",
          resolution_status: "unresolved",
          target_namespace: str(node, "targetNamespace"),
          normalized_title: str(node, "normalizedTitle"),
        };
        const expected = str(node, "expectedEntityType");
        if (expected) (out as { expected_entity_type?: string }).expected_entity_type = expected;
        return out;
      }
      return {
        type: "page_reference",
        target_page_id: str(node, "targetPageId"),
        ...(str(node, "targetHeadingBlockId") ? { target_heading_block_id: str(node, "targetHeadingBlockId") } : {}),
        display_text: str(node, "displayText"),
      };
    case "pageAnchorReference":
      return {
        type: "page_anchor_reference",
        target_page_id: str(node, "targetPageId"),
        target_heading_block_id: str(node, "targetHeadingBlockId"),
        display_text: str(node, "displayText"),
      };
    case "entityReference":
      return { type: "entity_reference", entity_id: str(node, "entityId"), display_text: str(node, "displayText") };
    case "claimReference":
      return { type: "claim_reference", claim_id: str(node, "claimId"), display_text: str(node, "displayText") };
    case "citationReference": {
      const out: InlineNode = { type: "citation_reference", citation_id: str(node, "citationId") };
      if (str(node, "displayText")) out.display_text = str(node, "displayText");
      return out;
    }
    case "mathInline":
      return { type: "math", expression: str(node, "expression") };
    case "mention":
      return { type: "mention", actor_id: str(node, "actorId"), display_text: str(node, "displayText") };
    case "dateMention": {
      const out: InlineNode = { type: "date_mention", date: str(node, "date"), display_text: str(node, "displayText") };
      if (str(node, "endDate")) out.end_date = str(node, "endDate");
      return out;
    }
    default:
      throw new UnsupportedEditorFeatureError(`未知行内节点 ${String(node.type)}`);
  }
}

function inlineContent(nodes: PMBlock[] | undefined, where: string): InlineNode[] {
  const out: InlineNode[] = [];
  for (const node of nodes ?? []) {
    if (node.type === "text") out.push(...marksToInlineText(node, where));
    else if (node.type === "hardBreak") out.push({ type: "text", text: "\n" });
    else {
      if (node.marks?.length) throw new UnsupportedEditorFeatureError("引用节点不支持叠加文字格式");
      out.push(inlineAtom(node));
    }
  }
  return out;
}

function childBlocks(node: PMBlock, where: string, state: AdapterState): AstBlock[] {
  return blocksToAst(node.content ?? [], `${where}.content`, state);
}

function firstParagraphContent(node: PMBlock, where: string, state: AdapterState): { content: InlineNode[]; rest: AstBlock[] } {
  const [first, ...rest] = node.content ?? [];
  if (first?.type === "paragraph") {
    return { content: inlineContent(first.content, `${where}.content[0]`), rest: blocksToAst(rest, `${where}.content`, state) };
  }
  return { content: [], rest: childBlocks(node, where, state) };
}

function parseJSON(value: string): unknown {
  try { return JSON.parse(value); } catch { throw new UnsupportedEditorFeatureError("属性必须是有效的 JSON，已保留原内容"); }
}

function singleBlock(node: PMBlock, where: string, state: AdapterState): AstBlock | null {
  const id = blockId(node, state, where);
  switch (node.type) {
    case "paragraph":
      return { id, type: "paragraph", content: inlineContent(node.content, where) };
    case "heading":
      return { id, type: "heading", level: num(node, "level", 1), content: inlineContent(node.content, where) };
    case "sectionHeading": {
      const lifted = firstParagraphContent(node, where, state);
      return { id, type: "heading", level: num(node, "level", 1), content: lifted.content,
        ...(typeof node.attrs?.isToggleable === "boolean" ? { is_toggleable: bool(node, "isToggleable") } : {}),
        ...(lifted.rest.length || bool(node, "hasChildren") ? { children: lifted.rest } : {}),
      };
    }
    case "blockquote":
      return { id, type: "quote", children: childBlocks(node, where, state) };
    case "bulletList":
      return { id, type: "bullet_list", children: (node.content ?? []).filter((child) => child.type === "listItem").map((child, index) => singleBlock(child, `${where}.content[${index}]`, state)!).filter(Boolean) as Extract<AstBlock, { type: "list_item" }>[] };
    case "orderedList":
      return { id, type: "ordered_list", children: (node.content ?? []).filter((child) => child.type === "listItem").map((child, index) => singleBlock(child, `${where}.content[${index}]`, state)!).filter(Boolean) as Extract<AstBlock, { type: "list_item" }>[] };
    case "listItem":
      return { id, type: "list_item", children: childBlocks(node, where, state) };
    case "todoBlock": {
      const lifted = firstParagraphContent(node, where, state);
      const out: AstBlock = { id, type: "todo", checked: bool(node, "checked"), content: lifted.content };
      if (lifted.rest.length > 0) out.children = lifted.rest;
      return out;
    }
    case "toggleBlock": {
      const lifted = firstParagraphContent(node, where, state);
      return { id, type: "toggle", content: lifted.content, ...(lifted.rest.length || bool(node, "hasChildren") ? { children: lifted.rest } : {}) };
    }
    case "callout":
      return { id, type: "callout", kind: str(node, "kind") as CalloutKind, ...(str(node, "icon") ? { icon: str(node, "icon") } : {}), ...(str(node, "color") && str(node, "color") !== "default" ? { color: str(node, "color") as CalloutColor } : {}), children: childBlocks(node, where, state) };
    case "codeBlock":
      return { id, type: "code", content: textOf(node), ...(str(node, "language") ? { language: str(node, "language") } : {}), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "preformatted":
      return { id, type: "preformatted", kind: str(node, "kind") as PreformattedKind, content: str(node, "text"), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "equation":
      return { id, type: "equation", expression: str(node, "expression"), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "breadcrumb":
      return { id, type: "breadcrumb", items: breadcrumbItemSchema.array().parse(parseJSON(str(node, "itemsJson"))) };
    case "columnList":
      return { id, type: "column_list", children: childBlocks(node, where, state) as Extract<AstBlock, { type: "column" }>[] };
    case "column":
      return { id, type: "column", width_ratio: num(node, "widthRatio", 1), children: childBlocks(node, where, state) };
    case "pageButton":
      return { id, type: "button", label: str(node, "label"), ...(str(node, "confirmation") ? { confirmation: str(node, "confirmation") } : {}), actions: buttonActionSchema.array().parse(parseJSON(str(node, "actionsJson"))) as ButtonAction[] };
    case "htmlBlock":
      return { id, type: "html", html: str(node, "html"), height: num(node, "height", 240), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "componentBlock":
      return { id, type: "component", component_id: str(node, "componentId"), component_version: num(node, "componentVersion", 1), entity_id: str(node, "entityId"), display_config: parseJSON(str(node, "displayConfig")) as Record<string, unknown> };
    case "imageAsset":
      return { id, type: "image", asset_revision_id: str(node, "assetRevisionId"), alt_text: str(node, "altText"), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "videoAsset":
      return { id, type: "video", asset_revision_id: str(node, "assetRevisionId"), ...(str(node, "posterAssetRevisionId") ? { poster_asset_revision_id: str(node, "posterAssetRevisionId") } : {}), ...(str(node, "title") ? { title: str(node, "title") } : {}), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "audioAsset":
      return { id, type: "audio", asset_revision_id: str(node, "assetRevisionId"), title: str(node, "title"), ...(str(node, "artist") ? { artist: str(node, "artist") } : {}), ...(str(node, "album") ? { album: str(node, "album") } : {}), ...(num(node, "durationSeconds", 0) > 0 ? { duration_seconds: num(node, "durationSeconds", 0) } : {}), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "fileAsset":
      return { id, type: "file", asset_revision_id: str(node, "assetRevisionId"), name: str(node, "name"), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "pdfAsset":
      return { id, type: "pdf", asset_revision_id: str(node, "assetRevisionId"), ...(str(node, "title") ? { title: str(node, "title") } : {}), ...(num(node, "startPage", 1) !== 1 ? { start_page: num(node, "startPage", 1) } : {}), height: num(node, "height", 640), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "bookmark":
      return { id, type: "bookmark", url: str(node, "url"), title: str(node, "title"), ...(str(node, "description") ? { description: str(node, "description") } : {}), ...(str(node, "siteName") ? { site_name: str(node, "siteName") } : {}), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "linkPreview":
      return { id, type: "link_preview", url: str(node, "url"), title: str(node, "title"), ...(str(node, "description") ? { description: str(node, "description") } : {}), ...(str(node, "siteName") ? { site_name: str(node, "siteName") } : {}) };
    case "embedBlock":
      return { id, type: "embed", url: str(node, "url"), ...(str(node, "title") ? { title: str(node, "title") } : {}), ...(str(node, "provider") && str(node, "provider") !== "none" ? { provider: str(node, "provider") as "youtube" | "vimeo" | "figma" } : {}), ...(str(node, "caption") ? { caption: str(node, "caption") } : {}) };
    case "astTable":
      return { id, type: "table", ...(str(node, "caption") ? { caption: str(node, "caption") } : {}), ...(bool(node, "hasColumnHeader") ? { has_column_header: true } : {}), ...(bool(node, "hasRowHeader") ? { has_row_header: true } : {}), children: childBlocks(node, where, state) as Extract<AstBlock, { type: "table_row" }>[] };
    case "astTableRow":
      return { id, type: "table_row", children: childBlocks(node, where, state) as Extract<AstBlock, { type: "table_cell" }>[] };
    case "astTableCell":
      return { id, type: "table_cell", children: childBlocks(node, where, state) };
    case "horizontalRule":
      return { id, type: "divider" };
    default:
      throw new UnsupportedEditorFeatureError(`未知块类型 ${String(node.type)}`);
  }
}

function blocksToAst(nodes: PMBlock[], where: string, state: AdapterState): AstBlock[] {
  const out: AstBlock[] = [];
  for (let index = 0; index < nodes.length; index += 1) {
    const block = singleBlock(nodes[index], `${where}[${index}]`, state);
    if (block) out.push(block);
  }
  return out;
}

function formatIssuePath(path: PropertyKey[]): string {
  let out = "";
  for (const seg of path) out += typeof seg === "number" ? `[${seg}]` : out ? `.${String(seg)}` : String(seg);
  return out || "(root)";
}

export function toAst(
  doc: PMDocument | PMBlock,
  state: AdapterState = createAdapterState(),
): Document {
  const pending = { generatedIds: new Map(state.generatedIds) };
  const raw = {
    type: "document",
    schema_version: 1,
    children: blocksToAst(doc.content ?? [], "children", pending),
  };
  try {
    const parsed = parseDocument(raw);
    state.generatedIds = pending.generatedIds;
    return parsed;
  } catch (error) {
    if (error instanceof ZodError) {
      const detail = error.issues.map((issue) => `  - ${formatIssuePath(issue.path)}: ${issue.message}`).join("\n");
      throw new AstValidationError(`内容格式校验失败：\n${detail}`);
    }
    throw error;
  }
}
