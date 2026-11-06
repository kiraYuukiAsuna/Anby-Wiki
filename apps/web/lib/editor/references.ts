// 引用节点（pageReference / external link）的插入 helper。
//
// 业务组件只经这些函数触碰编辑器插入结构；Tiptap 节点 attrs 与
// lib/editor/schema.ts、fromAst/toAst 的映射保持一致。
import type { Editor } from "@tiptap/core";

import { normalizeTitle } from "@/lib/ast/title";

export type InlineContentEditor = Editor;

/** pageReference attrs 全集（与 lib/editor/schema.ts 的 attrs 一致）。 */
interface PageReferenceAttrs {
  targetPageId: string;
  targetHeadingBlockId: string;
  displayText: string;
  resolutionStatus: string;
  targetNamespace: string;
  normalizedTitle: string;
  expectedEntityType: string;
}

function pageReferenceInlineContent(attrs: PageReferenceAttrs) {
  return { type: "pageReference", attrs };
}

/** 插入已解析页面引用：target_page_id + display_text 分离。 */
export function insertResolvedPageReference(
  editor: InlineContentEditor,
  targetPageId: string,
  displayText: string,
): void {
  editor.chain().focus().insertContent(pageReferenceInlineContent({
    targetPageId,
    targetHeadingBlockId: "",
    displayText,
    resolutionStatus: "",
    targetNamespace: "",
    normalizedTitle: "",
    expectedEntityType: "",
  })).run();
}

/** 插入未解析页面引用：resolution_status=unresolved + namespace + normalized title。 */
export function insertUnresolvedPageReference(
  editor: InlineContentEditor,
  rawTitle: string,
  targetNamespace = "main",
): void {
  editor.chain().focus().insertContent(pageReferenceInlineContent({
    targetPageId: "",
    targetHeadingBlockId: "",
    displayText: "",
    resolutionStatus: "unresolved",
    targetNamespace,
    normalizedTitle: normalizeTitle(rawTitle),
    expectedEntityType: "",
  })).run();
}

/** 插入外部链接（AST external_link：url + display_text 分离）。 */
export function insertExternalLink(
  editor: InlineContentEditor,
  url: string,
  displayText: string,
): void {
  editor.chain().focus().insertContent({
    type: "text",
    text: displayText,
    marks: [{ type: "link", attrs: { href: url } }],
  }).run();
}

/** 插入稳定 Actor 引用。 */
export function insertPersonMention(
  editor: InlineContentEditor,
  actorId: string,
  displayText: string,
): void {
  editor.chain().focus().insertContent({
    type: "mention",
    attrs: { actorId, displayText },
  }).run();
}

/** 插入日期或日期区间。 */
export function insertDateMention(
  editor: InlineContentEditor,
  date: string,
  endDate: string,
  displayText: string,
): void {
  editor.chain().focus().insertContent({
    type: "dateMention",
    attrs: { date, endDate, displayText },
  }).run();
}
