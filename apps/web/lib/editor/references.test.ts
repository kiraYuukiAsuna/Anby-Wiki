// 引用插入/编辑 helper 测试（M2-T04）：
// 真实编辑器实例（jsdom）插入 pageReference / link 后经 toAst 断言 AST 形态；
// display_text 编辑只动显示文本、不改 target_page_id；
// 两种 page_reference 形态与 external_link 的 toAst→fromAst 往返无损。
import { BlockNoteEditor } from "@blocknote/core";
import { NodeSelection } from "prosemirror-state";
import { describe, expect, it } from "vitest";

import type { Document } from "@/lib/ast/schema";

import { fromAst } from "./fromAst";
import { createAdapterState } from "./ids";
import {
  getSelectedPageReference,
  insertExternalLink,
  insertResolvedPageReference,
  insertUnresolvedPageReference,
  updateSelectedPageReferenceDisplayText,
} from "./references";
import { editorSchema, type BNBlock } from "./schema";
import { toAst } from "./toAst";

const TARGET_PAGE_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01";

function createEditor(): BlockNoteEditor<never, never, never> {
  // 空文档：BlockNote 默认创建一个空段落。
  return BlockNoteEditor.create({ schema: editorSchema }) as never;
}

function docOf(editor: BlockNoteEditor<never, never, never>): Document {
  return toAst(editor.document as unknown as BNBlock[], createAdapterState());
}

describe("引用插入 helper", () => {
  it("insertResolvedPageReference 产出已解析 page_reference（target_page_id + display_text）", () => {
    const editor = createEditor();
    try {
      insertResolvedPageReference(editor, TARGET_PAGE_ID, "安比·德玛拉");
      const doc = docOf(editor);
      expect(doc.children[0]).toMatchObject({
        type: "paragraph",
        content: [
          {
            type: "page_reference",
            target_page_id: TARGET_PAGE_ID,
            display_text: "安比·德玛拉",
          },
        ],
      });
    } finally {
      editor._tiptapEditor.destroy();
    }
  });

  it("insertUnresolvedPageReference 产出未解析 page_reference（规范化标题）", () => {
    const editor = createEditor();
    try {
      insertUnresolvedPageReference(editor, "  Billy   KID ");
      const doc = docOf(editor);
      expect(doc.children[0]).toMatchObject({
        type: "paragraph",
        content: [
          {
            type: "page_reference",
            resolution_status: "unresolved",
            target_namespace: "main",
            normalized_title: "billy kid",
          },
        ],
      });
      // 未解析形态不携带 display_text / target_page_id。
      const node = (doc.children[0] as { content: Record<string, unknown>[] })
        .content[0];
      expect(node).not.toHaveProperty("display_text");
      expect(node).not.toHaveProperty("target_page_id");
    } finally {
      editor._tiptapEditor.destroy();
    }
  });

  it("insertExternalLink 产出 external_link（url 与 display_text 分离）", () => {
    const editor = createEditor();
    try {
      insertExternalLink(editor, "https://example.com/ref", "参考资料");
      const doc = docOf(editor);
      expect(doc.children[0]).toMatchObject({
        type: "paragraph",
        content: [
          {
            type: "external_link",
            url: "https://example.com/ref",
            display_text: "参考资料",
          },
        ],
      });
    } finally {
      editor._tiptapEditor.destroy();
    }
  });
});

describe("display_text 编辑", () => {
  /** 在文档中找到 pageReference 节点位置并选中它。 */
  function selectPageReference(editor: BlockNoteEditor<never, never, never>) {
    let pos = -1;
    editor.prosemirrorState.doc.descendants((node, nodePos) => {
      if (node.type.name === "pageReference") {
        pos = nodePos;
        return false;
      }
      return true;
    });
    expect(pos).toBeGreaterThanOrEqual(0);
    const sel = NodeSelection.create(editor.prosemirrorState.doc, pos);
    editor.prosemirrorView.dispatch(
      editor.prosemirrorState.tr.setSelection(sel),
    );
    return pos;
  }

  it("updateSelectedPageReferenceDisplayText 只改 display_text，不改 target_page_id", () => {
    const editor = createEditor();
    try {
      insertResolvedPageReference(editor, TARGET_PAGE_ID, "原标题");
      selectPageReference(editor);

      const selected = getSelectedPageReference(editor);
      expect(selected).toMatchObject({
        resolved: true,
        targetPageId: TARGET_PAGE_ID,
        displayText: "原标题",
      });

      expect(
        updateSelectedPageReferenceDisplayText(editor, "自定义显示文本"),
      ).toBe(true);

      const doc = docOf(editor);
      const node = (
        doc.children[0] as { content: Record<string, unknown>[] }
      ).content[0];
      expect(node).toMatchObject({
        type: "page_reference",
        target_page_id: TARGET_PAGE_ID,
        display_text: "自定义显示文本",
      });
    } finally {
      editor._tiptapEditor.destroy();
    }
  });

  it("未选中 pageReference 时不做任何修改", () => {
    const editor = createEditor();
    try {
      insertResolvedPageReference(editor, TARGET_PAGE_ID, "原标题");
      // 插入后选区是普通文本选区。
      expect(getSelectedPageReference(editor)).toBeNull();
      expect(
        updateSelectedPageReferenceDisplayText(editor, "不会被写入"),
      ).toBe(false);
      const doc = docOf(editor);
      expect(doc.children[0]).toMatchObject({
        content: [{ display_text: "原标题" }],
      });
    } finally {
      editor._tiptapEditor.destroy();
    }
  });
});

describe("引用节点往返", () => {
  it("两种 page_reference 形态 + external_link 经 toAst→fromAst→toAst 深度相等", () => {
    const editor = createEditor();
    try {
      insertResolvedPageReference(editor, TARGET_PAGE_ID, "已解析");
      insertUnresolvedPageReference(editor, "Ghost Page");
      insertExternalLink(editor, "https://example.com", "外链");
      const first = docOf(editor);

      // AST → fromAst → 新编辑器 → toAst，必须深度相等（Block ID 除外：此处比内容）。
      const state = createAdapterState();
      const roundTripped = fromAst(first, state);
      const editor2 = BlockNoteEditor.create({
        schema: editorSchema,
        initialContent: roundTripped as never,
      });
      try {
        const second = toAst(
          editor2.document as unknown as BNBlock[],
          state,
        );
        expect(second).toEqual(first);
      } finally {
        editor2._tiptapEditor.destroy();
      }
    } finally {
      editor._tiptapEditor.destroy();
    }
  });
});
