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
  CalloutKind,
  Document,
  InlineNode,
  Mark,
} from "@/lib/ast/schema";
import { parseDocument } from "@/lib/ast/schema";

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
  strike: "strikethrough",
  code: "code",
};

/** marks 输出顺序固定，保证 AST→BN→AST 深度相等。 */
const CANONICAL_MARK_ORDER: readonly Mark[] = [
  "bold",
  "italic",
  "strikethrough",
  "code",
];

function activeStyles(styles: unknown): string[] {
  if (!styles || typeof styles !== "object") return [];
  return Object.entries(styles as Record<string, unknown>)
    .filter(([, value]) => Boolean(value))
    .map(([key]) => key);
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
    const mark = MARK_BY_STYLE[key];
    if (!mark) {
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: 不支持的文本样式 "${key}"（AST v1 仅支持 bold/italic/strikethrough/code）`,
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
      assertSupportedProps(block, ["level"], where);
      const level = Number(block.props?.level);
      if (!Number.isInteger(level) || level < 1 || level > 6) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: heading level=${JSON.stringify(block.props?.level)} 超出 AST 范围 1-6`,
        );
      }
      return {
        id,
        type: "heading",
        level,
        content: inlineContentToAst(block.content, where),
      };
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
      const parts = Array.isArray(block.content) ? block.content : [];
      let text = "";
      for (const part of parts) {
        if (part?.type !== "text") {
          throw new UnsupportedBlockNoteFeatureError(
            `${where}: codeBlock 内含非文本内容（${String(part?.type)}）`,
          );
        }
        const active = activeStyles(part.styles);
        if (active.length > 0) {
          throw new UnsupportedBlockNoteFeatureError(
            `${where}: codeBlock 内文本样式（${active.join(", ")}）无法映射到 AST code`,
          );
        }
        text += String(part.text ?? "");
      }
      const language = String(block.props?.language ?? "");
      const node: AstBlock = { id, type: "code", content: text };
      if (language) (node as { language?: string }).language = language;
      return node;
    }
    case "divider":
      assertSupportedProps(block, [], where);
      return { id, type: "divider" };
    case "callout": {
      assertSupportedProps(block, ["kind"], where);
      const kind = String(block.props?.kind ?? "");
      if (!["info", "warning", "danger"].includes(kind)) {
        throw new UnsupportedBlockNoteFeatureError(
          `${where}: callout kind="${kind}" 不在 AST 枚举（info/warning/danger）内`,
        );
      }
      return {
        id,
        type: "callout",
        kind: kind as CalloutKind,
        children: childrenToAst(block.children, state, `${where}.children`),
      };
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
    case "table":
      assertSupportedProps(block, [], where);
      return {
        id,
        type: "table",
        children: childrenToAst(
          block.children,
          state,
          `${where}.children`,
        ) as Extract<AstBlock, { type: "table_row" }>[],
      };
    case "tableRow":
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
    case "tableCell":
      assertSupportedProps(block, [], where);
      return {
        id,
        type: "table_cell",
        children: childrenToAst(block.children, state, `${where}.children`),
      };
    default:
      throw new UnsupportedBlockNoteFeatureError(
        `${where}: block 类型 "${block.type}" 不在 AST v1 映射表中 ` +
          `（audio/file/image/video/checkListItem/toggleListItem/pageBreak/原生 table 等特性不支持）`,
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
