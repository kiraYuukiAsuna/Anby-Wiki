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

export const markSchema = z.enum([
  "bold",
  "italic",
  "underline",
  "strikethrough",
  "code",
]);

export const inlineColorSchema = z.enum([
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

export const textNodeSchema = z.strictObject({
  type: z.literal("text"),
  text: z.string(),
  marks: z.array(markSchema).optional(),
  text_color: inlineColorSchema.optional(),
  background_color: inlineColorSchema.optional(),
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

/** 显式章节引用：Page 与 Heading Block 均为稳定 ID。 */
export const pageAnchorReferenceNodeSchema = z.strictObject({
  type: z.literal("page_anchor_reference"),
  target_page_id: idSchema,
  target_heading_block_id: idSchema,
  display_text: z.string(),
});

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

/** 行内数学表达式保留 LaTeX 源文本；渲染层不得当作 HTML。 */
export const mathNodeSchema = z.strictObject({
  type: z.literal("math"),
  expression: z.string(),
});

/** 对稳定 Actor 的行内提及。 */
export const mentionNodeSchema = z.strictObject({
  type: z.literal("mention"),
  actor_id: idSchema,
  display_text: z.string(),
});

/** 日期或日期区间提及。 */
export const dateMentionNodeSchema = z.strictObject({
  type: z.literal("date_mention"),
  date: z.iso.date(),
  end_date: z.iso.date().optional(),
  display_text: z.string(),
});

/** 对稳定 Dataset 的行内提及。 */
export const databaseMentionNodeSchema = z.strictObject({
  type: z.literal("database_mention"),
  dataset_id: idSchema,
  display_text: z.string(),
});

/** 行内节点联合；v1 仅以新增判别分支的方式 additive 演进。 */
export const inlineNodeSchema = z.union([
  textNodeSchema,
  inlineCodeNodeSchema,
  pageReferenceNodeSchema,
  pageAnchorReferenceNodeSchema,
  externalLinkNodeSchema,
  entityReferenceNodeSchema,
  claimReferenceNodeSchema,
  citationReferenceNodeSchema,
  mathNodeSchema,
  mentionNodeSchema,
  dateMentionNodeSchema,
  databaseMentionNodeSchema,
]);

export type Mark = z.infer<typeof markSchema>;
export type InlineColor = z.infer<typeof inlineColorSchema>;
export type TextNode = z.infer<typeof textNodeSchema>;
export type InlineCodeNode = z.infer<typeof inlineCodeNodeSchema>;
export type ResolvedPageReference = z.infer<typeof resolvedPageReferenceSchema>;
export type UnresolvedPageReference = z.infer<
  typeof unresolvedPageReferenceSchema
>;
export type PageReferenceNode = z.infer<typeof pageReferenceNodeSchema>;
export type PageAnchorReferenceNode = z.infer<
  typeof pageAnchorReferenceNodeSchema
>;
export type ExternalLinkNode = z.infer<typeof externalLinkNodeSchema>;
export type EntityReferenceNode = z.infer<typeof entityReferenceNodeSchema>;
export type ClaimReferenceNode = z.infer<typeof claimReferenceNodeSchema>;
export type CitationReferenceNode = z.infer<
  typeof citationReferenceNodeSchema
>;
export type MathNode = z.infer<typeof mathNodeSchema>;
export type MentionNode = z.infer<typeof mentionNodeSchema>;
export type DateMentionNode = z.infer<typeof dateMentionNodeSchema>;
export type DatabaseMentionNode = z.infer<typeof databaseMentionNodeSchema>;
export type InlineNode = z.infer<typeof inlineNodeSchema>;

// ---- Block 成员接口（手工声明，对应 ast.schema.json 的 $defs；互递归无法推导）----

export interface HeadingBlock {
  id: string;
  type: "heading";
  level: number;
  is_toggleable?: boolean;
  content: InlineNode[];
  children?: Block[];
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
  caption?: string;
  has_column_header?: boolean;
  has_row_header?: boolean;
  children: TableRowBlock[];
}

export interface CodeBlock {
  id: string;
  type: "code";
  content: string;
  language?: string;
  caption?: string;
}

export type PreformattedKind = z.infer<typeof preformattedKindSchema>;

export interface PreformattedBlock {
  id: string;
  type: "preformatted";
  kind: PreformattedKind;
  content: string;
  caption?: string;
}

export interface EquationBlock {
  id: string;
  type: "equation";
  expression: string;
  caption?: string;
}

export interface TodoBlock {
  id: string;
  type: "todo";
  checked: boolean;
  content: InlineNode[];
  children?: Block[];
}

export interface ToggleBlock {
  id: string;
  type: "toggle";
  content: InlineNode[];
  children?: Block[];
}

export interface BreadcrumbItem {
  target_page_id?: string;
  display_text: string;
}

export interface BreadcrumbBlock {
  id: string;
  type: "breadcrumb";
  items: BreadcrumbItem[];
}

export interface ColumnBlock {
  id: string;
  type: "column";
  width_ratio: number;
  children: Block[];
}

export interface ColumnListBlock {
  id: string;
  type: "column_list";
  children: ColumnBlock[];
}

export type ButtonAction =
  | {
      type: "insert_blocks";
      target_page_id: string;
      blocks: Block[];
    }
  | {
      type: "update_dataset_record";
      record_id: string;
      entity_id?: string | null;
      values: Record<string, unknown>;
    }
  | {
      type: "notification";
      message: string;
      tone?: "default" | "success" | "info" | "warning" | "error";
    }
  | {
      type: "webhook";
      url: string;
      payload: Record<string, unknown>;
    };

export interface ButtonBlock {
  id: string;
  type: "button";
  label: string;
  confirmation?: string;
  actions: ButtonAction[];
}

export interface HTMLBlock {
  id: string;
  type: "html";
  html: string;
  height: number;
  caption?: string;
}

export interface QuoteBlock {
  id: string;
  type: "quote";
  children: Block[];
}

export type CalloutKind = z.infer<typeof calloutKindSchema>;
export type CalloutColor = z.infer<typeof calloutColorSchema>;

export interface CalloutBlock {
  id: string;
  type: "callout";
  kind: CalloutKind;
  icon?: string;
  color?: CalloutColor;
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

/** 图片固定到不可变 AssetRevision。 */
export interface ImageBlock {
  id: string;
  type: "image";
  asset_revision_id: string;
  alt_text: string;
  caption?: string;
}

/** 视频与可选封面均固定到不可变 AssetRevision。 */
export interface VideoBlock {
  id: string;
  type: "video";
  asset_revision_id: string;
  poster_asset_revision_id?: string;
  title?: string;
  caption?: string;
}

export interface AudioBlock {
  id: string;
  type: "audio";
  asset_revision_id: string;
  title: string;
  artist?: string;
  album?: string;
  duration_seconds?: number;
  caption?: string;
}

export interface FileBlock {
  id: string;
  type: "file";
  asset_revision_id: string;
  name: string;
  caption?: string;
}

export interface PDFBlock {
  id: string;
  type: "pdf";
  asset_revision_id: string;
  title?: string;
  start_page?: number;
  height: number;
  caption?: string;
}

export interface BookmarkBlock {
  id: string;
  type: "bookmark";
  url: string;
  title: string;
  description?: string;
  site_name?: string;
  caption?: string;
}

export interface LinkPreviewBlock {
  id: string;
  type: "link_preview";
  url: string;
  title: string;
  description?: string;
  site_name?: string;
}

/** 可查询数据只保存 DatasetView 稳定 ID，不复制记录。 */
export interface DatasetViewBlock {
  id: string;
  type: "dataset_view";
  dataset_view_id: string;
}

/** 外部嵌入以安全链接卡片渲染，不执行第三方脚本。 */
export interface EmbedBlock {
  id: string;
  type: "embed";
  url: string;
  title?: string;
  provider?: "youtube" | "vimeo" | "figma";
  caption?: string;
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
  | PreformattedBlock
  | EquationBlock
  | TodoBlock
  | ToggleBlock
  | BreadcrumbBlock
  | ColumnListBlock
  | ColumnBlock
  | ButtonBlock
  | HTMLBlock
  | QuoteBlock
  | CalloutBlock
  | ComponentBlock
  | ImageBlock
  | VideoBlock
  | AudioBlock
  | FileBlock
  | PDFBlock
  | BookmarkBlock
  | LinkPreviewBlock
  | DatasetViewBlock
  | EmbedBlock
  | DividerBlock;

// ---- Block schema（与上面的接口一一对应）----

export const headingBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("heading"),
  level: z.number().int().min(1).max(6),
  is_toggleable: z.boolean().optional(),
  content: z.array(inlineNodeSchema),
  children: z.array(z.lazy(() => blockSchema)).optional(),
}).superRefine((value, context) => {
  if (value.children !== undefined && value.is_toggleable !== true) {
    context.addIssue({
      code: "custom",
      path: ["children"],
      message: "heading children 要求 is_toggleable=true",
    });
  }
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
  caption: z.string().optional(),
  has_column_header: z.boolean().optional(),
  has_row_header: z.boolean().optional(),
  children: z.array(tableRowBlockSchema),
});

export const codeBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("code"),
  content: z.string(),
  language: z.string().optional(),
  caption: z.string().optional(),
});

export const preformattedKindSchema = z.enum([
  "ascii_diagram",
  "fixed_table",
  "protocol_trace",
  "formula_text",
  "plain",
]);

export const preformattedBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("preformatted"),
  kind: preformattedKindSchema,
  content: z.string(),
  caption: z.string().optional(),
});

export const equationBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("equation"),
  expression: z.string().min(1),
  caption: z.string().optional(),
});

export const todoBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("todo"),
  checked: z.boolean(),
  content: z.array(inlineNodeSchema),
  children: z.array(z.lazy(() => blockSchema)).optional(),
});

export const toggleBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("toggle"),
  content: z.array(inlineNodeSchema),
  children: z.array(z.lazy(() => blockSchema)).optional(),
});

export const breadcrumbItemSchema = z.strictObject({
  target_page_id: idSchema.optional(),
  display_text: z.string().min(1).max(255),
});

export const breadcrumbBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("breadcrumb"),
  items: z.array(breadcrumbItemSchema).min(1).max(32),
});

export const columnBlockSchema: z.ZodType<ColumnBlock> = z.strictObject({
  id: idSchema,
  type: z.literal("column"),
  width_ratio: z.number().min(0.1).max(10),
  children: z.array(z.lazy(() => blockSchema)),
});

export const columnListBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("column_list"),
  children: z.array(columnBlockSchema).min(2).max(5),
});

const buttonInsertBlocksActionSchema = z.strictObject({
  type: z.literal("insert_blocks"),
  target_page_id: idSchema,
  blocks: z.array(z.lazy(() => blockSchema)).min(1).max(20),
});

const buttonUpdateRecordActionSchema = z.strictObject({
  type: z.literal("update_dataset_record"),
  record_id: idSchema,
  entity_id: idSchema.nullable().optional(),
  values: z.record(z.string(), z.unknown()),
});

const buttonNotificationActionSchema = z.strictObject({
  type: z.literal("notification"),
  message: z.string().min(1).max(500),
  tone: z.enum(["default", "success", "info", "warning", "error"]).optional(),
});

const buttonWebhookActionSchema = z.strictObject({
  type: z.literal("webhook"),
  url: httpUrlSchema.refine((value) => value.startsWith("https://"), {
    message: "Webhook URL 必须使用 https",
  }),
  payload: z.record(z.string(), z.unknown()),
});

export const buttonActionSchema: z.ZodType<ButtonAction> = z.union([
  buttonInsertBlocksActionSchema,
  buttonUpdateRecordActionSchema,
  buttonNotificationActionSchema,
  buttonWebhookActionSchema,
]);

export const buttonBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("button"),
  label: z.string().min(1).max(120),
  confirmation: z.string().min(1).max(500).optional(),
  actions: z.array(buttonActionSchema).min(1).max(10),
});

export const htmlBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("html"),
  html: z.string().max(200000),
  height: z.number().int().min(120).max(1200),
  caption: z.string().max(1000).optional(),
});

export const quoteBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("quote"),
  children: z.array(z.lazy(() => blockSchema)),
});

export const calloutKindSchema = z.enum(["info", "warning", "danger"]);
export const calloutColorSchema = z.enum([
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
]);

export const calloutBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("callout"),
  kind: calloutKindSchema,
  icon: z.string().min(1).max(64).optional(),
  color: calloutColorSchema.optional(),
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

export const imageBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("image"),
  asset_revision_id: idSchema,
  alt_text: z.string().min(1),
  caption: z.string().optional(),
});

export const videoBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("video"),
  asset_revision_id: idSchema,
  poster_asset_revision_id: idSchema.optional(),
  title: z.string().optional(),
  caption: z.string().optional(),
});

export const audioBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("audio"),
  asset_revision_id: idSchema,
  title: z.string().min(1).max(500),
  artist: z.string().max(500).optional(),
  album: z.string().max(500).optional(),
  duration_seconds: z.number().min(0).optional(),
  caption: z.string().max(1000).optional(),
});

export const fileBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("file"),
  asset_revision_id: idSchema,
  name: z.string().min(1).max(500),
  caption: z.string().max(1000).optional(),
});

export const pdfBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("pdf"),
  asset_revision_id: idSchema,
  title: z.string().max(500).optional(),
  start_page: z.number().int().min(1).optional(),
  height: z.number().int().min(320).max(1200),
  caption: z.string().max(1000).optional(),
});

export const bookmarkBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("bookmark"),
  url: httpUrlSchema,
  title: z.string().min(1).max(500),
  description: z.string().max(1000).optional(),
  site_name: z.string().max(255).optional(),
  caption: z.string().max(1000).optional(),
});

export const linkPreviewBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("link_preview"),
  url: httpUrlSchema,
  title: z.string().min(1).max(500),
  description: z.string().max(1000).optional(),
  site_name: z.string().max(255).optional(),
});

export const datasetViewBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("dataset_view"),
  dataset_view_id: idSchema,
});

export const embedBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("embed"),
  url: httpUrlSchema,
  title: z.string().optional(),
  provider: z.enum(["youtube", "vimeo", "figma"]).optional(),
  caption: z.string().max(1000).optional(),
});

export const dividerBlockSchema = z.strictObject({
  id: idSchema,
  type: z.literal("divider"),
});

export const blockSchema: z.ZodType<Block> = z.union([
  headingBlockSchema,
  paragraphBlockSchema,
  bulletListBlockSchema,
  orderedListBlockSchema,
  listItemBlockSchema,
  tableBlockSchema,
  tableRowBlockSchema,
  tableCellBlockSchema,
  codeBlockSchema,
  preformattedBlockSchema,
  equationBlockSchema,
  todoBlockSchema,
  toggleBlockSchema,
  breadcrumbBlockSchema,
  columnListBlockSchema,
  columnBlockSchema,
  buttonBlockSchema,
  htmlBlockSchema,
  quoteBlockSchema,
  calloutBlockSchema,
  componentBlockSchema,
  imageBlockSchema,
  videoBlockSchema,
  audioBlockSchema,
  fileBlockSchema,
  pdfBlockSchema,
  bookmarkBlockSchema,
  linkPreviewBlockSchema,
  datasetViewBlockSchema,
  embedBlockSchema,
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
