// ID 稳定性测试（ADR-0005 验收标准 2）：
// 编辑既有 Block 不改 ID；新建 Block 获得 UUIDv7 且在同一会话内稳定。
import { BlockNoteEditor } from "@blocknote/core";
import { describe, expect, it } from "vitest";

import { parseDocument, type Document } from "@/lib/ast/schema";

import { fromAst } from "./fromAst";
import { createAdapterState, isUuidV7, newBlockId } from "./ids";
import { editorSchema, type BNBlock } from "./schema";
import { toAst } from "./toAst";

function sampleDoc(): Document {
  return parseDocument({
    type: "document",
    schema_version: 1,
    children: [
      {
        id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
        type: "paragraph",
        content: [{ type: "text", text: "第一段" }],
      },
      {
        id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02",
        type: "heading",
        level: 2,
        content: [{ type: "text", text: "标题" }],
      },
    ],
  });
}

function createEditor(doc: Document) {
  const state = createAdapterState();
  const editor = BlockNoteEditor.create({
    schema: editorSchema,
    initialContent: fromAst(doc, state) as never,
  });
  return { editor, state };
}

describe("Block ID 稳定性", () => {
  it("newBlockId 生成合法 UUIDv7", () => {
    const id = newBlockId();
    expect(isUuidV7(id)).toBe(true);
    expect(id).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
  });

  it("编辑既有块文本后 ID 不变", () => {
    const doc = sampleDoc();
    const { editor, state } = createEditor(doc);
    try {
      editor.updateBlock("0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01", {
        content: "改过的文本",
      } as never);
      const back = toAst(editor.document as unknown as BNBlock[], state);
      expect(back.children.map((b) => b.id)).toEqual(
        doc.children.map((b) => b.id),
      );
      expect(back.children[0]).toMatchObject({
        type: "paragraph",
        content: [{ type: "text", text: "改过的文本" }],
      });
    } finally {
      editor._tiptapEditor.destroy();
    }
  });

  it("新建块获得 UUIDv7，且重复序列化保持稳定", () => {
    const doc = sampleDoc();
    const { editor, state } = createEditor(doc);
    try {
      const [inserted] = editor.insertBlocks(
        [{ type: "paragraph", content: "新段落" } as never],
        "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02",
        "after",
      );
      // BlockNote 内部分配的是 uuidv4，不是 UUIDv7。
      expect(isUuidV7(inserted.id)).toBe(false);

      const first = toAst(editor.document as unknown as BNBlock[], state);
      const newId = first.children[2].id;
      expect(isUuidV7(newId)).toBe(true);
      expect(doc.children.map((b) => b.id)).not.toContain(newId);
      // 既有块 ID 不变。
      expect(first.children.slice(0, 2).map((b) => b.id)).toEqual(
        doc.children.map((b) => b.id),
      );

      // 再次编辑后重复序列化：新块 ID 稳定（经 AdapterState 映射）。
      editor.updateBlock(inserted.id, { content: "新段落 v2" } as never);
      const second = toAst(editor.document as unknown as BNBlock[], state);
      expect(second.children[2].id).toBe(newId);
    } finally {
      editor._tiptapEditor.destroy();
    }
  });
});
