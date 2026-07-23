import * as fs from "node:fs";
import * as path from "node:path";

import { describe, expect, it } from "vitest";
import * as Y from "yjs";

import { parseDocument, type Document } from "@/lib/ast/schema";

import {
  applyYjsState,
  createYjsAstDocument,
  encodeYjsState,
  getYjsAstRoot,
  materializeYjsAst,
  moveYjsArrayItem,
  syncYjsAst,
} from "./yjs-ast";

const FIXTURE_DIR = path.resolve(
  process.cwd(),
  "../../contracts/schemas/ast/v1/fixtures/valid",
);

function fixture(name: string): Document {
  return parseDocument(
    JSON.parse(fs.readFileSync(path.join(FIXTURE_DIR, name), "utf8")),
  );
}

function children(ydoc: Y.Doc): Y.Array<unknown> {
  return getYjsAstRoot(ydoc).get("children") as Y.Array<unknown>;
}

function block(ydoc: Y.Doc, index: number): Y.Map<unknown> {
  return children(ydoc).get(index) as Y.Map<unknown>;
}

function inlineText(ydoc: Y.Doc, blockIndex = 0): Y.Text {
  const content = block(ydoc, blockIndex).get("content") as Y.Array<unknown>;
  return (content.get(0) as Y.Map<unknown>).get("text") as Y.Text;
}

function sync(left: Y.Doc, right: Y.Doc): void {
  applyYjsState(left, encodeYjsState(right));
  applyYjsState(right, encodeYjsState(left));
}

describe("Yjs AST v1 mapping", () => {
  for (const name of [
    "minimal.json",
    "full_document.json",
    "nested_lists.json",
    "table.json",
    "knowledge_references.json",
  ]) {
    it(`${name} maps without loss`, () => {
      const ast = fixture(name);
      expect(materializeYjsAst(createYjsAstDocument(ast))).toEqual(ast);
    });
  }

  it("merges offline character edits and converges", () => {
    const ast = parseDocument({
      type: "document",
      schema_version: 1,
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d01",
          type: "paragraph",
          content: [{ type: "text", text: "AB" }],
        },
      ],
    });
    const left = createYjsAstDocument(ast);
    const right = new Y.Doc();
    applyYjsState(right, encodeYjsState(left));

    inlineText(left).insert(1, "left");
    inlineText(right).insert(1, "right");
    sync(left, right);

    const leftResult = materializeYjsAst(left);
    const rightResult = materializeYjsAst(right);
    expect(leftResult).toEqual(rightResult);
    const text = (
      leftResult.children[0] as { content: Array<{ text: string }> }
    ).content[0].text;
    expect(text).toContain("left");
    expect(text).toContain("right");
    expect(leftResult.children[0].id).toBe(ast.children[0].id);
  });

  it("converges three clients after duplicate and out-of-order delivery", () => {
    const ast = parseDocument({
      type: "document",
      schema_version: 1,
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d71",
          type: "paragraph",
          content: [{ type: "text", text: "ABC" }],
        },
      ],
    });
    const first = createYjsAstDocument(ast);
    const second = new Y.Doc();
    const third = new Y.Doc();
    const initial = encodeYjsState(first);
    applyYjsState(second, initial);
    applyYjsState(third, initial);
    inlineText(first).insert(1, "one");
    inlineText(second).insert(2, "two");
    inlineText(third).insert(3, "three");
    const firstUpdate = encodeYjsState(first);
    const secondUpdate = encodeYjsState(second);
    const thirdUpdate = encodeYjsState(third);

    for (const target of [first, second, third]) {
      applyYjsState(target, thirdUpdate);
      applyYjsState(target, firstUpdate);
      applyYjsState(target, secondUpdate);
      applyYjsState(target, firstUpdate);
    }

    const expected = materializeYjsAst(first);
    expect(materializeYjsAst(second)).toEqual(expected);
    expect(materializeYjsAst(third)).toEqual(expected);
    const text = (
      expected.children[0] as { content: Array<{ text: string }> }
    ).content[0].text;
    expect(text).toContain("one");
    expect(text).toContain("two");
    expect(text).toContain("three");
  });

  it("reconciles editor AST changes into mergeable Y.Text operations", () => {
    const ast = parseDocument({
      type: "document",
      schema_version: 1,
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d21",
          type: "paragraph",
          content: [{ type: "text", text: "AB" }],
        },
      ],
    });
    const left = createYjsAstDocument(ast);
    const right = new Y.Doc();
    applyYjsState(right, encodeYjsState(left));
    const leftAst = structuredClone(ast);
    const rightAst = structuredClone(ast);
    leftAst.children[0] = {
      ...leftAst.children[0],
      type: "paragraph",
      content: [{ type: "text", text: "A-left-B" }],
    };
    rightAst.children[0] = {
      ...rightAst.children[0],
      type: "paragraph",
      content: [{ type: "text", text: "A-right-B" }],
    };

    syncYjsAst(left, leftAst, "editor");
    syncYjsAst(right, rightAst, "editor");
    sync(left, right);

    expect(materializeYjsAst(left)).toEqual(materializeYjsAst(right));
    const text = (
      materializeYjsAst(left).children[0] as {
        content: Array<{ text: string }>;
      }
    ).content[0].text;
    expect(text).toContain("left");
    expect(text).toContain("right");
  });

  it("preserves IDs across block add, delete, move, and property edits", () => {
    const ast = parseDocument({
      type: "document",
      schema_version: 1,
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d11",
          type: "heading",
          level: 1,
          content: [{ type: "text", text: "Heading" }],
        },
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d12",
          type: "divider",
        },
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d13",
          type: "divider",
        },
      ],
    });
    const ydoc = createYjsAstDocument(ast);
    block(ydoc, 0).set("level", 2);
    children(ydoc).delete(1, 1);

    const added = new Y.Map<unknown>();
    added.set("id", "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d14");
    added.set("type", "divider");
    children(ydoc).insert(1, [added]);
    moveYjsArrayItem(children(ydoc), 0, 2);

    const result = materializeYjsAst(ydoc);
    expect(result.children.map((item) => item.id)).toEqual([
      "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d14",
      "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d13",
      "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d11",
    ]);
    expect(result.children[2]).toMatchObject({ type: "heading", level: 2 });
  });

  it("applies duplicate and out-of-order updates idempotently", () => {
    const source = createYjsAstDocument(fixture("full_document.json"));
    const target = new Y.Doc();
    const initial = encodeYjsState(source);
    applyYjsState(target, initial);

    inlineText(source).insert(0, "A");
    const first = encodeYjsState(source, Y.encodeStateVector(target));
    inlineText(source).insert(1, "B");
    const second = encodeYjsState(source, Y.encodeStateVector(target));

    applyYjsState(target, second);
    applyYjsState(target, first);
    applyYjsState(target, second);
    sync(source, target);

    expect(materializeYjsAst(target)).toEqual(materializeYjsAst(source));
  });

  it("rejects a working copy that cannot materialize as AST v1", () => {
    const ydoc = createYjsAstDocument(fixture("full_document.json"));
    block(ydoc, 0).delete("id");
    expect(() => materializeYjsAst(ydoc)).toThrow();
  });
});
