// 双向往返测试（ADR-0005 验收标准 1、2）：
// contracts/schemas/ast/v1/fixtures/valid 全部 fixture 经
// AST → fromAst → BlockNote 编辑器实例 → editor.document → toAst → AST
// 必须深度相等（含全部 Block ID）。
import * as fs from "node:fs";
import * as path from "node:path";

import { BlockNoteEditor } from "@blocknote/core";
import { describe, expect, it } from "vitest";

import { parseDocument } from "@/lib/ast/schema";

import { fromAst } from "./fromAst";
import { createAdapterState } from "./ids";
import { editorSchema, type BNBlock, type BNPartialBlock } from "./schema";
import { toAst } from "./toAst";

// 与 components/ast/index.test.tsx 同一约定：vitest 以 apps/web 为 cwd 运行。
const FIXTURE_DIR = path.resolve(
  process.cwd(),
  "../../contracts/schemas/ast/v1/fixtures/valid",
);

const FIXTURES = [
  "full_document.json",
  "nested_lists.json",
  "table.json",
  "knowledge_references.json",
];

function readFixture(name: string) {
  return JSON.parse(fs.readFileSync(path.join(FIXTURE_DIR, name), "utf8"));
}

describe("AST ↔ BlockNote 往返（真实编辑器实例）", () => {
  for (const name of FIXTURES) {
    it(`${name} 往返后深度相等`, () => {
      const doc = parseDocument(readFixture(name));
      const state = createAdapterState();
      const initialContent = fromAst(doc, state);

      const editor = BlockNoteEditor.create({
        schema: editorSchema,
        initialContent: initialContent as never,
      });
      try {
        const back = toAst(
          editor.document as unknown as BNBlock[],
          state,
        );
        expect(back).toEqual(doc);
      } finally {
        editor._tiptapEditor.destroy();
      }
    });
  }

  it("fromAst → toAst（不经编辑器）同样深度相等", () => {
    const normalize = (blocks: BNPartialBlock[]): BNBlock[] =>
      blocks.map((b) => ({
        id: b.id ?? "",
        type: b.type ?? "",
        props: b.props ?? {},
        content: b.content,
        children: normalize(b.children ?? []),
      }));
    for (const name of [...FIXTURES, "minimal.json"]) {
      const doc = parseDocument(readFixture(name));
      const state = createAdapterState();
      expect(toAst(normalize(fromAst(doc, state)), state)).toEqual(doc);
    }
  });

  it("ComponentBlock 保留冻结版本、Entity 与 display_config", () => {
    const doc = parseDocument({
      type: "document",
      schema_version: 1,
      children: [
        {
          id: "019f92d5-92c9-7e57-a49a-54aadbc3e121",
          type: "component",
          component_id: "019f92d5-92c9-7e57-a49a-54aadbc3e122",
          component_version: 2,
          entity_id: "019f92d5-92c9-7e57-a49a-54aadbc3e123",
          display_config: {
            title: "角色信息",
            property_keys: ["developer", "release_date"],
          },
        },
      ],
    });
    const state = createAdapterState();
    const initial = fromAst(doc, state);
    const normalized = initial.map((block) => ({
      id: block.id ?? "",
      type: block.type ?? "",
      props: block.props ?? {},
      content: block.content,
      children: [],
    })) as BNBlock[];
    expect(toAst(normalized, state)).toEqual(doc);
  });

  it("minimal.json（空文档）：BlockNote 不接受空 initialContent，编辑器内表现为一个空段落（已记录的降级）", () => {
    const doc = parseDocument(readFixture("minimal.json"));
    const state = createAdapterState();
    const initialContent = fromAst(doc, state);
    expect(initialContent).toEqual([]);
    // 与 BlockEditor 组件行为一致：空文档不传 initialContent，
    // BlockNote 默认创建一个空段落，序列化回 AST 即一个空 paragraph。
    const editor = BlockNoteEditor.create({ schema: editorSchema });
    try {
      const back = toAst(editor.document as unknown as BNBlock[], state);
      expect(back.children).toHaveLength(1);
      expect(back.children[0]).toMatchObject({ type: "paragraph", content: [] });
    } finally {
      editor._tiptapEditor.destroy();
    }
  });
});
