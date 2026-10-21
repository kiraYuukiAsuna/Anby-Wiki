// BlockNote document → AST v1 Document（M2-T02 / ADR-0005）。
//
// 输出经 parseDocument（Zod）校验，失败抛 AstValidationError（带 issue 路径）。
// 无法无损映射的 BlockNote 特性抛 UnsupportedBlockNoteFeatureError，不静默丢弃。
//
// ID 策略：
//   - UUIDv7 的 BlockNote 块 id（来自 fromAst / 上一轮 toAst）原样保留 → 编辑既有块不改 ID；
//   - 其他 id（BlockNote 给新建块分配的 uuidv4）经 state.generatedIds 稳定分配 UUIDv7；
//   - list 容器 id 经 state.listIds 还原，新建分组分配新 UUIDv7；
//   - quote / list_item 行内 content 还原为 paragraph 子块时，优先取
//     state.liftedParagraphIds 中 fromAst 记录的 id，否则分配新 UUIDv7。
import { ZodError } from "zod";

import type {
  Block as AstBlock,
  ButtonAction,
  CalloutColor,
  CalloutKind,
  Document,
  InlineColor,
  InlineNode,
  Mark,
  PreformattedKind,
} from "@/lib/ast/schema";
import {
  breadcrumbItemSchema,
  buttonActionSchema,
  parseDocument,
} from "@/lib/ast/schema";

import { AstValidationError, UnsupportedBlockNoteFeatureError } from "./errors";
import {
  createAdapterState,
  newBlockId,
  resolveBlockId,
  type AdapterState,
} from "./ids";
import type { BNBlock } from "./schema";

// ---- 行内转换 ----

const MARK_BY_STYLE: Record<string, Mark> = {
  bold: "bold",
  italic: "italic",
  underline: "underline",
  strike: "strikethrough",
  code: "code",
};

/** marks 输出顺序固定，保证 AST→BN→AST 深度相等。 */
const CANONICAL_MARK_ORDER: readonly Mark[] = [
  "bold",
  "italic",
  "underline",
  "strikethrough",
  "code",
];

const INLINE_COLORS = new Set<InlineColor>([
  "gray",
  "brown",
  "red",
  "orange",
  "yellow",
  "green",
  "blue",
  "purple",
  "pink",
]);

function activeStyles(styles: unknown): string[] {
  if (!styles || typeof styles !== "object") return [];
  return Object.entries(styles as Record<string, unknown>)
    .filter(
      ([key, value]) =>
        (key === "textColor" || key === "backgroundColor")
          ? typeof value === "string" && value !== "" && value !== "default"
          : Boolean(value),
    )
    .map(([key]) => key);
}

function inlineColor(
  value: unknown,
  style: "textColor" | "backgroundColor",
  where: string,
): InlineColor | undefined {
  if (value == null || value === "" || value === "default") return undefined;
  if (typeof value !== "string" || !INLINE_COLORS.has(value as InlineColor)) {
    throw new UnsupportedBlockNoteFeatureError(
      `${where}: ${style}="${String(value)}" 不在 AST v1 调色板中`,
    );
  }
  return value as InlineColor;
}

function plainBlockText(block: BNBlock, where: string): string {
  const parts = Array.isArray(block.content) ? block.content : [];
  let text = "";
  for (const part of parts) {
    if (part?.type !== "text") {
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: ${block.type} 内含非文本内容（${String(part?.type)}）`,
      );
    }
    const active = activeStyles(part.styles);
    if (active.length > 0) {
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: ${block.type} 内文本样式（${active.join(", ")}）无法映射到固定宽度文本`,
      );
    }
    text += String(part.text ?? "");
  }
  return text;
}

function parseJSONProp(block: BNBlock, key: string, where: string): unknown {
  try {
    return JSON.parse(String(block.props?.[key] ?? ""));
  } catch {
    throw new UnsupportedBlockNoteFeatureError(
      `${where}: ${block.type}.${key} 必须是合法 JSON`,
    );
  }
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function styledTextToInlineNodes(item: any, where: string): InlineNode[] {
  const active = activeStyles(item.styles);
  if (active.includes("inlineCode")) {
    if (active.length > 1) {
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: inlineCode 样式不能与其他样式（${active.join(", ")}）叠加，` +
          `AST inline_code 无法承载`,
      );
    }
    return [{ type: "inline_code", text: String(item.text ?? "") }];
  }
  const marks: Mark[] = [];
  for (const key of active) {
    if (key === "textColor" || key === "backgroundColor") continue;
    const mark = MARK_BY_STYLE[key];
    if (!mark) {
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: 不支持的文本样式 "${key}"`,
      );
    }
    marks.push(mark);
  }
  marks.sort(
    (a, b) =>
      CANONICAL_MARK_ORDER.indexOf(a) - CANONICAL_MARK_ORDER.indexOf(b),
  );
  const node: InlineNode = { type: "text", text: String(item.text ?? "") };
  if (marks.length > 0) {
    (node as { marks?: Mark[] }).marks = marks;
  }
  const textColor = inlineColor(item.styles?.textColor, "textColor", where);
  const backgroundColor = inlineColor(
    item.styles?.backgroundColor,
    "backgroundColor",
    where,
  );
  if (textColor) {
    (node as { text_color?: InlineColor }).text_color = textColor;
  }
  if (backgroundColor) {
    (node as { background_color?: InlineColor }).background_color =
      backgroundColor;
  }
  return [node];
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function linkToInlineNode(item: any, where: string): InlineNode {
  const content = Array.isArray(item.content) ? item.content : [];
  let displayText = "";
  for (const part of content) {
    if (part?.type !== "text") {
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: link 内嵌非文本内容（${String(part?.type)}）无法映射到 external_link`,
      );
    }
    const active = activeStyles(part.styles);
    if (active.length > 0) {
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: link 内文本样式（${active.join(", ")}）无法映射到 external_link.display_text`,
      );
    }
    displayText += String(part.text ?? "");
  }
  return {
    type: "external_link",
    url: String(item.href ?? ""),
    display_text: displayText,
  };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function pageReferenceToInlineNode(item: any, where: string): InlineNode {
  const props = (item.props ?? {}) as Record<string, unknown>;
  const str = (key: string) => String(props[key] ?? "");
  if (str("resolutionStatus") === "unresolved") {
    const node: InlineNode = {
      type: "page_reference",
      resolution_status: "unresolved",
      target_namespace: str("targetNamespace"),
      normalized_title: str("normalizedTitle"),
    };
    if (str("expectedEntityType")) {
      (node as { expected_entity_type?: string }).expected_entity_type =
        str("expectedEntityType");
    }
    return node;
  }
  if (str("resolutionStatus")) {
    throw new UnsupportedBlockNoteFeatureError(
      `${where}: pageReference 的 resolutionStatus="${str("resolutionStatus")}" 未知`,
    );
  }
  const node: InlineNode = {
    type: "page_reference",
    target_page_id: str("targetPageId"),
    display_text: str("displayText"),
  };
  if (str("targetHeadingBlockId")) {
    (node as { target_heading_block_id?: string }).target_heading_block_id =
      str("targetHeadingBlockId");
  }
  return node;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function knowledgeReferenceToInlineNode(item: any): InlineNode {
  const props = (item.props ?? {}) as Record<string, unknown>;
  const str = (key: string) => String(props[key] ?? "");
  switch (item.type) {
    case "entityReference":
      return {
        type: "entity_reference",
        entity_id: str("entityId"),
        display_text: str("displayText"),
      };
    case "claimReference":
      return {
        type: "claim_reference",
        claim_id: str("claimId"),
        display_text: str("displayText"),
      };
    case "citationReference": {
      const node: InlineNode = {
        type: "citation_reference",
        citation_id: str("citationId"),
      };
      if (str("displayText")) {
        (node as { display_text?: string }).display_text = str("displayText");
      }
      return node;
    }
    default:
      throw new UnsupportedBlockNoteFeatureError(
        `无法识别的知识引用类型 "${String(item.type)}"`,
      );
  }
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function extendedReferenceToInlineNode(item: any): InlineNode {
  const props = (item.props ?? {}) as Record<string, unknown>;
  const str = (key: string) => String(props[key] ?? "");
  switch (item.type) {
    case "pageAnchorReference":
      return {
        type: "page_anchor_reference",
        target_page_id: str("targetPageId"),
        target_heading_block_id: str("targetHeadingBlockId"),
        display_text: str("displayText"),
      };
    case "math":
      return { type: "math", expression: str("expression") };
    case "mention":
      return {
        type: "mention",
        actor_id: str("actorId"),
        display_text: str("displayText"),
      };
    case "dateMention": {
      const node: InlineNode = {
        type: "date_mention",
        date: str("date"),
        display_text: str("displayText"),
      };
      if (str("endDate")) {
        (node as { end_date?: string }).end_date = str("endDate");
      }
      return node;
    }
    case "databaseMention":
      return {
        type: "database_mention",
        dataset_id: str("datasetId"),
        display_text: str("displayText"),
      };
    default:
      throw new UnsupportedBlockNoteFeatureError(
        `无法识别的扩展行内类型 "${String(item.type)}"`,
      );
  }
}

function inlineContentToAst(content: unknown, where: string): InlineNode[] {
  if (content == null) return [];
  if (typeof content === "string") {
    return content ? [{ type: "text", text: content }] : [];
  }
  if (!Array.isArray(content)) {
    throw new UnsupportedBlockNoteFeatureError(
      `${where}: 无法识别的行内 content 形态`,
    );
  }
  const out: InlineNode[] = [];
  for (const item of content) {
    switch (item?.type) {
      case "text":
        out.push(...styledTextToInlineNodes(item, where));
        break;
      case "link":
        out.push(linkToInlineNode(item, where));
        break;
      case "pageReference":
        out.push(pageReferenceToInlineNode(item, where));
        break;
      case "entityReference":
      case "claimReference":
      case "citationReference":
        out.push(knowledgeReferenceToInlineNode(item));
        break;
      case "pageAnchorReference":
      case "math":
      case "mention":
      case "dateMention":
      case "databaseMention":
        out.push(extendedReferenceToInlineNode(item));
        break;
      default:
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: 不支持的行内内容类型 "${String(item?.type)}"`,
        );
    }
  }
  return out;
}

// ---- 块转换 ----

/** 已映射 prop 之外的 prop 必须等于 BlockNote 默认值，否则显式拒绝。 */
const BLOCKNOTE_PROP_DEFAULTS: Record<string, unknown> = {
  textColor: "default",
  backgroundColor: "default",
  textAlignment: "left",
  isToggleable: false,
  start: undefined,
};

function assertSupportedProps(
  block: BNBlock,
  handledProps: readonly string[],
  where: string,
): void {
  for (const [key, value] of Object.entries(block.props ?? {})) {
    if (handledProps.includes(key)) continue;
    if (value == null) continue;
    if (key in BLOCKNOTE_PROP_DEFAULTS && value === BLOCKNOTE_PROP_DEFAULTS[key]) {
      continue;
    }
    throw new UnsupportedBlockNoteFeatureError(
      `${where}: block "${block.type}" 的 prop "${key}"=${JSON.stringify(value)} ` +
        `无法映射到 AST v1`,
    );
  }
}

/**
 * 把块（quote / list item）的行内 content 还原为 paragraph 子块。
 * content 为空则不产生子块；paragraph id 优先取 fromAst 记录，否则新建。
 */
function liftedParagraph(
  block: BNBlock,
  state: AdapterState,
  where: string,
): AstBlock[] {
  const content = inlineContentToAst(block.content, where);
  if (content.length === 0) return [];
  let id = state.liftedParagraphIds.get(block.id);
  if (!id) {
    id = newBlockId();
    state.liftedParagraphIds.set(block.id, id);
  }
  return [{ id, type: "paragraph", content }];
}

function childrenToAst(
  children: BNBlock[],
  state: AdapterState,
  where: string,
): AstBlock[] {
  return blocksToAst(children ?? [], state, where);
}

function listItemToAst(
  block: BNBlock,
  state: AdapterState,
  where: string,
): AstBlock {
  assertSupportedProps(block, [], where);
  return {
    id: resolveBlockId(block.id, state),
    type: "list_item",
    children: [
      ...liftedParagraph(block, state, where),
      ...childrenToAst(block.children, state, `${where}.children`),
    ],
  };
}

function singleBlockToAst(
  block: BNBlock,
  state: AdapterState,
  where: string,
): AstBlock {
  const id = resolveBlockId(block.id, state);
  switch (block.type) {
    case "paragraph":
      assertSupportedProps(block, [], where);
      return {
        id,
        type: "paragraph",
        content: inlineContentToAst(block.content, where),
      };
    case "heading": {
      assertSupportedProps(block, ["level", "isToggleable"], where);
      const level = Number(block.props?.level);
      if (!Number.isInteger(level) || level < 1 || level > 6) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: heading level=${JSON.stringify(block.props?.level)} 超出 AST 范围 1-6`,
        );
      }
      const isToggleable = Boolean(block.props?.isToggleable);
      if (!isToggleable && block.children.length > 0) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: 非折叠 heading 不能包含 children`,
        );
      }
      const node: AstBlock = {
        id,
        type: "heading",
        level,
        content: inlineContentToAst(block.content, where),
      };
      if (isToggleable) {
        (node as Extract<AstBlock, { type: "heading" }>).is_toggleable = true;
        const children = childrenToAst(
          block.children,
          state,
          `${where}.children`,
        );
        if (children.length > 0) {
          (node as Extract<AstBlock, { type: "heading" }>).children = children;
        }
      }
      return node;
    }
    case "checkListItem": {
      assertSupportedProps(block, ["checked"], where);
      const checked = block.props?.checked;
      if (typeof checked !== "boolean") {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: checkListItem.checked 必须是 boolean`,
        );
      }
      const node: AstBlock = {
        id,
        type: "todo",
        checked,
        content: inlineContentToAst(block.content, where),
      };
      const children = childrenToAst(
        block.children,
        state,
        `${where}.children`,
      );
      if (children.length > 0) node.children = children;
      return node;
    }
    case "toggleListItem": {
      assertSupportedProps(block, [], where);
      const node: AstBlock = {
        id,
        type: "toggle",
        content: inlineContentToAst(block.content, where),
      };
      const children = childrenToAst(
        block.children,
        state,
        `${where}.children`,
      );
      if (children.length > 0) node.children = children;
      return node;
    }
    case "breadcrumb": {
      assertSupportedProps(block, ["itemsJson"], where);
      const parsed = breadcrumbItemSchema
        .array()
        .min(1)
        .max(32)
        .safeParse(parseJSONProp(block, "itemsJson", where));
      if (!parsed.success) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: breadcrumb.itemsJson 不符合页面路径结构`,
        );
      }
      return { id, type: "breadcrumb", items: parsed.data };
    }
    case "columnList": {
      assertSupportedProps(block, [], where);
      const children = childrenToAst(
        block.children,
        state,
        `${where}.children`,
      );
      if (
        children.length < 2 ||
        children.length > 5 ||
        children.some((child) => child.type !== "column")
      ) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: columnList 必须包含 2-5 个 column`,
        );
      }
      return {
        id,
        type: "column_list",
        children: children as Extract<AstBlock, { type: "column" }>[],
      };
    }
    case "column": {
      assertSupportedProps(block, ["widthRatio"], where);
      const widthRatio = Number(block.props?.widthRatio);
      if (!Number.isFinite(widthRatio) || widthRatio < 0.1 || widthRatio > 10) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: column.widthRatio 必须在 0.1-10 之间`,
        );
      }
      return {
        id,
        type: "column",
        width_ratio: widthRatio,
        children: childrenToAst(block.children, state, `${where}.children`),
      };
    }
    case "pageButton": {
      assertSupportedProps(
        block,
        ["label", "confirmation", "actionsJson"],
        where,
      );
      const label = String(block.props?.label ?? "");
      const parsed = buttonActionSchema
        .array()
        .min(1)
        .max(10)
        .safeParse(parseJSONProp(block, "actionsJson", where));
      if (!label || !parsed.success) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: pageButton 需要 label 和 1-10 个合法动作`,
        );
      }
      const node: AstBlock = {
        id,
        type: "button",
        label,
        actions: parsed.data as ButtonAction[],
      };
      const confirmation = String(block.props?.confirmation ?? "");
      if (confirmation) node.confirmation = confirmation;
      return node;
    }
    case "htmlBlock": {
      assertSupportedProps(block, ["html", "height", "caption"], where);
      const height = Number(block.props?.height);
      const html = String(block.props?.html ?? "");
      if (
        !Number.isInteger(height) ||
        height < 120 ||
        height > 1200 ||
        html.length > 200000
      ) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: htmlBlock 的高度或内容长度非法`,
        );
      }
      const node: AstBlock = { id, type: "html", html, height };
      const caption = String(block.props?.caption ?? "");
      if (caption) node.caption = caption;
      return node;
    }
    case "quote":
      assertSupportedProps(block, [], where);
      return {
        id,
        type: "quote",
        children: [
          ...liftedParagraph(block, state, where),
          ...childrenToAst(block.children, state, `${where}.children`),
        ],
      };
    case "codeBlock": {
      assertSupportedProps(block, ["language"], where);
      const text = plainBlockText(block, where);
      const language = String(block.props?.language ?? "");
      const node: AstBlock = { id, type: "code", content: text };
      if (language) (node as { language?: string }).language = language;
      const caption = state.codeCaptions.get(block.id);
      if (caption) (node as { caption?: string }).caption = caption;
      return node;
    }
    case "preformatted": {
      assertSupportedProps(block, ["kind", "caption", "text"], where);
      const kind = String(block.props?.kind ?? "");
      if (
        ![
          "ascii_diagram",
          "fixed_table",
          "protocol_trace",
          "formula_text",
          "plain",
        ].includes(kind)
      ) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: preformatted kind="${kind}" 非法`,
        );
      }
      const node: AstBlock = {
        id,
        type: "preformatted",
        kind: kind as PreformattedKind,
        content: String(block.props?.text ?? ""),
      };
      const caption = String(block.props?.caption ?? "");
      if (caption) node.caption = caption;
      return node;
    }
    case "equation": {
      assertSupportedProps(block, ["expression", "caption"], where);
      const expression = String(block.props?.expression ?? "");
      if (!expression) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: equation.expression 不能为空`,
        );
      }
      const node: AstBlock = { id, type: "equation", expression };
      const caption = String(block.props?.caption ?? "");
      if (caption) node.caption = caption;
      return node;
    }
    case "divider":
      assertSupportedProps(block, [], where);
      return { id, type: "divider" };
    case "callout": {
      assertSupportedProps(block, ["kind", "icon", "color"], where);
      const kind = String(block.props?.kind ?? "");
      if (!["info", "warning", "danger"].includes(kind)) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: callout kind="${kind}" 不在 AST 枚举（info/warning/danger）内`,
        );
      }
      const color = String(block.props?.color ?? "default");
      if (
        ![
          "default",
          "gray",
          "brown",
          "orange",
          "yellow",
          "green",
          "blue",
          "purple",
          "pink",
          "red",
        ].includes(color)
      ) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: callout color="${color}" 非法`,
        );
      }
      const node: AstBlock = {
        id,
        type: "callout",
        kind: kind as CalloutKind,
        children: childrenToAst(block.children, state, `${where}.children`),
      };
      const icon = String(block.props?.icon ?? "");
      if (icon) node.icon = icon;
      if (color !== "default") node.color = color as CalloutColor;
      return node;
    }
    case "component": {
      assertSupportedProps(
        block,
        ["componentId", "componentVersion", "entityId", "displayConfig"],
        where,
      );
      const componentVersion = Number(block.props?.componentVersion);
      let displayConfig: unknown;
      try {
        displayConfig = JSON.parse(String(block.props?.displayConfig ?? "{}"));
      } catch {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: component displayConfig 必须是合法 JSON object`,
        );
      }
      if (
        !Number.isInteger(componentVersion) ||
        componentVersion < 1 ||
        displayConfig === null ||
        Array.isArray(displayConfig) ||
        typeof displayConfig !== "object"
      ) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: component version/displayConfig 非法`,
        );
      }
      return {
        id,
        type: "component",
        component_id: String(block.props?.componentId ?? ""),
        component_version: componentVersion,
        entity_id: String(block.props?.entityId ?? ""),
        display_config: displayConfig as Record<string, unknown>,
      };
    }
    case "imageAsset": {
      assertSupportedProps(
        block,
        ["assetRevisionId", "altText", "caption"],
        where,
      );
      const node: AstBlock = {
        id,
        type: "image",
        asset_revision_id: String(block.props?.assetRevisionId ?? ""),
        alt_text: String(block.props?.altText ?? ""),
      };
      const caption = String(block.props?.caption ?? "");
      if (caption) {
        (node as { caption?: string }).caption = caption;
      }
      return node;
    }
    case "videoAsset": {
      assertSupportedProps(
        block,
        ["assetRevisionId", "posterAssetRevisionId", "title", "caption"],
        where,
      );
      const node: AstBlock = {
        id,
        type: "video",
        asset_revision_id: String(block.props?.assetRevisionId ?? ""),
      };
      const poster = String(block.props?.posterAssetRevisionId ?? "");
      const title = String(block.props?.title ?? "");
      const caption = String(block.props?.caption ?? "");
      if (poster) {
        (node as { poster_asset_revision_id?: string }).poster_asset_revision_id =
          poster;
      }
      if (title) (node as { title?: string }).title = title;
      if (caption) (node as { caption?: string }).caption = caption;
      return node;
    }
    case "audioAsset": {
      assertSupportedProps(
        block,
        [
          "assetRevisionId",
          "title",
          "artist",
          "album",
          "durationSeconds",
          "caption",
        ],
        where,
      );
      const title = String(block.props?.title ?? "");
      const duration = Number(block.props?.durationSeconds ?? 0);
      if (!title || !Number.isFinite(duration) || duration < 0) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: audioAsset 的标题或时长非法`,
        );
      }
      const node: AstBlock = {
        id,
        type: "audio",
        asset_revision_id: String(block.props?.assetRevisionId ?? ""),
        title,
      };
      const artist = String(block.props?.artist ?? "");
      const album = String(block.props?.album ?? "");
      const caption = String(block.props?.caption ?? "");
      if (artist) node.artist = artist;
      if (album) node.album = album;
      if (duration > 0) node.duration_seconds = duration;
      if (caption) node.caption = caption;
      return node;
    }
    case "fileAsset": {
      assertSupportedProps(
        block,
        ["assetRevisionId", "name", "caption"],
        where,
      );
      const node: AstBlock = {
        id,
        type: "file",
        asset_revision_id: String(block.props?.assetRevisionId ?? ""),
        name: String(block.props?.name ?? ""),
      };
      const caption = String(block.props?.caption ?? "");
      if (caption) node.caption = caption;
      return node;
    }
    case "pdfAsset": {
      assertSupportedProps(
        block,
        ["assetRevisionId", "title", "startPage", "height", "caption"],
        where,
      );
      const startPage = Number(block.props?.startPage ?? 1);
      const height = Number(block.props?.height);
      if (
        !Number.isInteger(startPage) ||
        startPage < 1 ||
        !Number.isInteger(height) ||
        height < 320 ||
        height > 1200
      ) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: pdfAsset 的页码或高度非法`,
        );
      }
      const node: AstBlock = {
        id,
        type: "pdf",
        asset_revision_id: String(block.props?.assetRevisionId ?? ""),
        height,
      };
      const title = String(block.props?.title ?? "");
      const caption = String(block.props?.caption ?? "");
      if (title) node.title = title;
      if (startPage !== 1) node.start_page = startPage;
      if (caption) node.caption = caption;
      return node;
    }
    case "bookmark":
    case "linkPreview": {
      assertSupportedProps(
        block,
        ["url", "title", "description", "siteName", "caption"],
        where,
      );
      const isBookmark = block.type === "bookmark";
      const url = String(block.props?.url ?? "");
      const title = String(block.props?.title ?? "");
      const description = String(block.props?.description ?? "");
      const siteName = String(block.props?.siteName ?? "");
      const caption = String(block.props?.caption ?? "");
      if (isBookmark) {
        return {
          id,
          type: "bookmark",
          url,
          title,
          ...(description ? { description } : {}),
          ...(siteName ? { site_name: siteName } : {}),
          ...(caption ? { caption } : {}),
        };
      }
      return {
        id,
        type: "link_preview",
        url,
        title,
        ...(description ? { description } : {}),
        ...(siteName ? { site_name: siteName } : {}),
      };
    }
    case "datasetView":
      assertSupportedProps(block, ["datasetViewId"], where);
      return {
        id,
        type: "dataset_view",
        dataset_view_id: String(block.props?.datasetViewId ?? ""),
      };
    case "embed": {
      assertSupportedProps(
        block,
        ["url", "title", "provider", "caption"],
        where,
      );
      const node: AstBlock = {
        id,
        type: "embed",
        url: String(block.props?.url ?? ""),
      };
      const title = String(block.props?.title ?? "");
      const provider = String(block.props?.provider ?? "none");
      const caption = String(block.props?.caption ?? "");
      if (!["none", "youtube", "vimeo", "figma"].includes(provider)) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: embed provider="${provider}" 非法`,
        );
      }
      if (title) (node as { title?: string }).title = title;
      if (provider !== "none") {
        node.provider = provider as "youtube" | "vimeo" | "figma";
      }
      if (caption) node.caption = caption;
      return node;
    }
    case "astTable": {
      assertSupportedProps(
        block,
        ["caption", "hasColumnHeader", "hasRowHeader"],
        where,
      );
      const node: AstBlock = {
        id,
        type: "table",
        children: childrenToAst(
          block.children,
          state,
          `${where}.children`,
        ) as Extract<AstBlock, { type: "table_row" }>[],
      };
      const caption = String(block.props?.caption ?? "");
      if (caption) node.caption = caption;
      if (Boolean(block.props?.hasColumnHeader)) node.has_column_header = true;
      if (Boolean(block.props?.hasRowHeader)) node.has_row_header = true;
      return node;
    }
    case "astTableRow":
      assertSupportedProps(block, [], where);
      return {
        id,
        type: "table_row",
        children: childrenToAst(
          block.children,
          state,
          `${where}.children`,
        ) as Extract<AstBlock, { type: "table_cell" }>[],
      };
    case "astTableCell":
      assertSupportedProps(block, [], where);
      return {
        id,
        type: "table_cell",
        children: childrenToAst(block.children, state, `${where}.children`),
      };
    default:
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: block 类型 "${block.type}" 不在 AST v1 映射表中 ` +
          `（audio/file/pageBreak/原生 table 等特性不支持）`,
      );
  }
}

function blocksToAst(
  blocks: BNBlock[],
  state: AdapterState,
  where: string,
): AstBlock[] {
  const out: AstBlock[] = [];
  let i = 0;
  while (i < blocks.length) {
    const block = blocks[i];
    if (block.type === "bulletListItem" || block.type === "numberedListItem") {
      // 连续同类型 list item 折叠回 list 容器；容器 id 经 state 还原或新建。
      const itemType = block.type;
      const run: BNBlock[] = [];
      while (i < blocks.length && blocks[i].type === itemType) {
        run.push(blocks[i]);
        i++;
      }
      const listId =
        run
          .map((item) => state.listIds.get(item.id))
          .find((existing) => existing != null) ?? newBlockId();
      for (const item of run) {
        state.listIds.set(item.id, listId);
      }
      out.push({
        id: listId,
        type: itemType === "bulletListItem" ? "bullet_list" : "ordered_list",
        children: run.map((item, j) =>
          listItemToAst(item, state, `${where}[${i - run.length + j}]`),
        ) as Extract<AstBlock, { type: "list_item" }>[],
      });
    } else {
      out.push(singleBlockToAst(block, state, `${where}[${i}]`));
      i++;
    }
  }
  return out;
}

function formatIssuePath(path: PropertyKey[]): string {
  let out = "";
  for (const seg of path) {
    out += typeof seg === "number" ? `[${seg}]` : out ? `.${String(seg)}` : String(seg);
  }
  return out || "(root)";
}

/**
 * BlockNote document（editor.document）→ AST v1 Document。
 * state 必须与 fromAst 共享以保证 ID 稳定；缺省为一次性状态
 * （此时新建块的 id 在多次调用间不保证稳定）。
 */
export function toAst(
  blocks: readonly BNBlock[],
  state: AdapterState = createAdapterState(),
): Document {
  const raw = {
    type: "document",
    schema_version: 1,
    children: blocksToAst([...blocks], state, "children"),
  };
  try {
    return parseDocument(raw);
  } catch (error) {
    if (error instanceof ZodError) {
      const detail = error.issues
        .map((issue) => `  - ${formatIssuePath(issue.path)}: ${issue.message}`)
        .join("\n");
      throw new AstValidationError(
        `toAst 产物未通过 AST v1 校验：\n${detail}`,
      );
    }
    throw error;
  }
}
