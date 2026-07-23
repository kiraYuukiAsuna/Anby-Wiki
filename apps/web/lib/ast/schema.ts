// Typed Block AST v1 的 Zod 绑定。
//
// 权威契约是 contracts/schemas/ast/v1/ast.schema.json（JSON Schema draft 2020-12），
// 本文件与其保持语义等价：strictObject 对应 additionalProperties: false，
// literal/enum 对应 const/enum，z.uuid() 对应 format: uuid。
// 扩展规则见 contracts/schemas/ast/v1/README.md（v1 只 additive 演进）。
//
// 注意：Block 是互递归判别联合（容器 Block 的 children 又回到 Block），
// TypeScript 无法对互递归 schema 做类型推导，因此 Block 各成员接口手工声明
// （必须与 JSON Schema 保持同步），递归处用 z.lazy 引用显式标注为
// z.ZodType<Block> 的 blockSchema；非递归的 Inline 节点类型仍由 z.infer 推导。
import { z } from "zod";

import { httpUrlSchema } from "@/lib/http-url";

/** Block ID：UUIDv7，标准 36 字符小写连字符形式（ADR-0008）。 */
const idSchema = z.uuid();

export const markSchema = z.enum(["bold", "italic", "strikethrough", "code"]);

export const textNodeSchema = z.strictObject({
  type: z.literal("text"),
  text: z.string(),
  marks: z.array(markSchema).optional(),
});

export const inlineCodeNodeSchema = z.strictObject({
  type: z.literal("inline_code"),
  text: z.string(),
});

/** 已解析页面引用：target_page_id + display_text，可选章节锚点（设计 §5.1、§9）。 */
export const resolvedPageReferenceSchema = z.strictObject({
  type: z.literal("page_reference"),
  target_page_id: idSchema,
  target_heading_block_id: idSchema.optional(),
  display_text: z.string(),
});

/** 未解析页面引用：resolution_status=unresolved + target_namespace + normalized_title（设计 §5.2）。 */
export const unresolvedPageReferenceSchema = z.strictObject({
  type: z.literal("page_reference"),
  resolution_status: z.literal("unresolved"),
  target_namespace: z.string(),
  normalized_title: z.string(),
  expected_entity_type: z.string().optional(),
});

/** 页面引用：已解析与未解析两种互斥形态（对应 JSON Schema 的 oneOf）。 */
export const pageReferenceNodeSchema = z.union([
  resolvedPageReferenceSchema,
  unresolvedPageReferenceSchema,
]);

/** 外部链接：v1 直接保存 URL；external_resource_id 规范化属于 M3。 */
export const externalLinkNodeSchema = z.strictObject({
  type: z.literal("external_link"),
  url: httpUrlSchema,
  display_text: z.string(),
});

/** 知识实体引用：稳定 Entity ID + 当前文档中的展示文本（设计 §5.3）。 */
export const entityReferenceNodeSchema = z.strictObject({
  type: z.literal("entity_reference"),
  entity_id: idSchema,
  display_text: z.string(),
});

/** Claim 引用：稳定 Claim ID + 当前文档中的展示文本（设计 §5.4）。 */
export const claimReferenceNodeSchema = z.strictObject({
  type: z.literal("claim_reference"),
  claim_id: idSchema,
  display_text: z.string(),
});

/** Citation 引用：稳定 Citation ID；display_text 作为可选悬浮提示。 */
export const citationReferenceNodeSchema = z.strictObject({
  type: z.literal("citation_reference"),
  citation_id: idSchema,
  display_text: z.string().optional(),
});

/** 行内节点联合；v1 仅以新增判别分支的方式 additive 演进。 */
export const inlineNodeSchema = z.union([
  textNodeSchema,
  inlineCodeNodeSchema,
  pageReferenceNodeSchema,
  externalLinkNodeSchema,
  entityReferenceNodeSchema,
  claimReferenceNodeSchema,
  citationReferenceNodeSchema,
]);

export type Mark = z.infer<typeof markSchema>;
export type TextNode = z.infer<typeof textNodeSchema>;
export type InlineCodeNode = z.infer<typeof inlineCodeNodeSchema>;
export type ResolvedPageReference = z.infer<typeof resolvedPageReferenceSchema>;
export type UnresolvedPageReference = z.infer<
  typeof unresolvedPageReferenceSchema
>;
export type PageReferenceNode = z.infer<typeof pageReferenceNodeSchema>;
export type ExternalLinkNode = z.infer<typeof externalLinkNodeSchema>;
export type EntityReferenceNode = z.infer<typeof entityReferenceNodeSchema>;
export type ClaimReferenceNode = z.infer<typeof claimReferenceNodeSchema>;
export type CitationReferenceNode = z.infer<
  typeof citationReferenceNodeSchema
>;
export type InlineNode = z.infer<typeof inlineNodeSchema>;

// ---- Block 成员接口（手工声明，对应 ast.schema.json 的 $defs；互递归无法推导）----

export interface HeadingBlock {
  id: string;
  type: "heading";
  level: number;
  content: InlineNode[];
}

export interface ParagraphBlock {
  id: string;
  type: "paragraph";
  content: InlineNode[];
}

/** list_item：children 为任意 Block（嵌套 list 仍必须经 list_item 包裹）。 */
export interface ListItemBlock {
  id: string;
  type: "list_item";
  children: Block[];
}

/** 容器规则：list 的 children 只能是 list_item。 */
export interface BulletListBlock {
  id: string;
  type: "bullet_list";
  children: ListItemBlock[];
}

export interface OrderedListBlock {
  id: string;
  type: "ordered_list";
  children: ListItemBlock[];
}

export interface TableCellBlock {
  id: string;
  type: "table_cell";
  children: Block[];
}

/** 容器规则：table → table_row → table_cell。 */
export interface TableRowBlock {
  id: string;
  type: "table_row";
  children: TableCellBlock[];
}

export interface TableBlock {
  id: string;
  type: "table";
  children: TableRowBlock[];
}

export interface CodeBlock {
  id: string;
  type: "code";
  content: string;
  language?: string;
}

export interface QuoteBlock {
  id: string;
  type: "quote";
  children: Block[];
}

export type CalloutKind = z.infer<typeof calloutKindSchema>;

export interface CalloutBlock {
  id: string;
  type: "callout";
  kind: CalloutKind;
  children: Block[];
}

/** 冻结组件版本实例；Claim 值由渲染层按 Entity 动态解析，不复制进 AST。 */
export interface ComponentBlock {
  id: string;
  type: "component";
  component_id: string;
  component_version: number;
  entity_id: string;
  display_config: Record<string, unknown>;
}

/** divider 无内容：不允许 content/children 等任何额外字段。 */
export interface DividerBlock {
  id: string;
  type: "divider";
}

/** Block 判别联合：按 type 判别。新增 Block 类型属于 additive 变更。 */
export type Block =
  | HeadingBlock
  | ParagraphBlock
  | BulletListBlock
  | OrderedListBlock
  | ListItemBlock
  | TableBlock
  | TableRowBlock
  | TableCellBlock
  | CodeBlock
  | QuoteBlock
  | CalloutBlock
  | ComponentBlock
  | DividerBlock;

// ---- Block schema（与上面的接口一一对应）----

export const headingBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("heading"),
  level: z.number().int().min(1).max(6),
  content: z.array(inlineNodeSchema),
});

export const paragraphBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("paragraph"),
  content: z.array(inlineNodeSchema),
});

export const listItemBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("list_item"),
  children: z.array(z.lazy(() => blockSchema)),
});

export const bulletListBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("bullet_list"),
  children: z.array(listItemBlockSchema),
});

export const orderedListBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("ordered_list"),
  children: z.array(listItemBlockSchema),
});

export const tableCellBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("table_cell"),
  children: z.array(z.lazy(() => blockSchema)),
});

export const tableRowBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("table_row"),
  children: z.array(tableCellBlockSchema),
});

export const tableBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("table"),
  children: z.array(tableRowBlockSchema),
});

export const codeBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("code"),
  content: z.string(),
  language: z.string().optional(),
});

export const quoteBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("quote"),
  children: z.array(z.lazy(() => blockSchema)),
});

export const calloutKindSchema = z.enum(["info", "warning", "danger"]);

export const calloutBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("callout"),
  kind: calloutKindSchema,
  children: z.array(z.lazy(() => blockSchema)),
});

export const componentBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("component"),
  component_id: idSchema,
  component_version: z.number().int().min(1),
  entity_id: idSchema,
  display_config: z.record(z.string(), z.unknown()),
});

export const dividerBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("divider"),
});

export const blockSchema: z.ZodType<Block> = z.discriminatedUnion("type", [
  headingBlockSchema,
  paragraphBlockSchema,
  bulletListBlockSchema,
  orderedListBlockSchema,
  listItemBlockSchema,
  tableBlockSchema,
  tableRowBlockSchema,
  tableCellBlockSchema,
  codeBlockSchema,
  quoteBlockSchema,
  calloutBlockSchema,
  componentBlockSchema,
  dividerBlockSchema,
]);

export const documentSchema = z.strictObject({
  type: z.literal("document"),
  schema_version: z.literal(1),
  children: z.array(blockSchema),
});

export type Document = z.infer<typeof documentSchema>;

/** 校验并解析 AST v1 文档，非法输入抛出 ZodError。 */
export function parseDocument(input: unknown): Document {
  return documentSchema.parse(input);
}
