import * as fs from "node:fs";
import * as path from "node:path";

import { describe, expect, it } from "vitest";

import { documentSchema, parseDocument } from "./schema";

// vitest 以 apps/web 为 cwd 运行，fixtures 与 Go 测试共享同一目录。
const fixturesRoot = path.resolve(
  process.cwd(),
  "../../contracts/schemas/ast/v1/fixtures",
);

function listFixtures(kind: "valid" | "invalid"): string[] {
  const names = fs
    .readdirSync(path.join(fixturesRoot, kind))
    .filter((f) => f.endsWith(".json"))
    .sort();
  if (names.length === 0) {
    throw new Error(`fixtures/${kind} 目录为空`);
  }
  return names;
}

function readFixture(kind: "valid" | "invalid", name: string): unknown {
  return JSON.parse(
    fs.readFileSync(path.join(fixturesRoot, kind, name), "utf8"),
  );
}

describe("AST v1 共享 fixtures", () => {
  for (const name of listFixtures("valid")) {
    it(`valid: ${name}`, () => {
      expect(() => parseDocument(readFixture("valid", name))).not.toThrow();
    });
  }
  for (const name of listFixtures("invalid")) {
    it(`invalid: ${name}`, () => {
      expect(() => parseDocument(readFixture("invalid", name))).toThrow();
    });
  }
});

describe("AST v1 细粒度校验", () => {
  const id = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e01";

  it("拒绝缺 id 的 Block", () => {
    expect(
      documentSchema.safeParse({
        type: "document",
        schema_version: 1,
        children: [{ type: "paragraph", content: [] }],
      }).success,
    ).toBe(false);
  });

  it("拒绝非法 heading level", () => {
    for (const level of [0, 7, 2.5, "2"]) {
      expect(
        documentSchema.safeParse({
          type: "document",
          schema_version: 1,
          children: [{ id, type: "heading", level, content: [] }],
        }).success,
      ).toBe(false);
    }
  });

  it("拒绝 list 直挂 paragraph（容器规则）", () => {
    expect(
      documentSchema.safeParse({
        type: "document",
        schema_version: 1,
        children: [
          {
            id,
            type: "bullet_list",
            children: [{ id, type: "paragraph", content: [] }],
          },
        ],
      }).success,
    ).toBe(false);
  });

  it("拒绝 divider 带内容", () => {
    expect(
      documentSchema.safeParse({
        type: "document",
        schema_version: 1,
        children: [{ id, type: "divider", children: [] }],
      }).success,
    ).toBe(false);
  });

  it("拒绝缺 resolution_status 的未解析引用", () => {
    expect(
      documentSchema.safeParse({
        type: "document",
        schema_version: 1,
        children: [
          {
            id,
            type: "paragraph",
            content: [
              {
                type: "page_reference",
                target_namespace: "Main",
                normalized_title: "x",
              },
            ],
          },
        ],
      }).success,
    ).toBe(false);
  });

  it("拒绝非法 uuid 的 Block id", () => {
    expect(
      documentSchema.safeParse({
        type: "document",
        schema_version: 1,
        children: [{ id: "not-a-uuid", type: "divider" }],
      }).success,
    ).toBe(false);
  });

  it("拒绝已解析与未解析形态混合的引用", () => {
    expect(
      documentSchema.safeParse({
        type: "document",
        schema_version: 1,
        children: [
          {
            id,
            type: "paragraph",
            content: [
              {
                type: "page_reference",
                target_page_id: id,
                display_text: "x",
                resolution_status: "unresolved",
                target_namespace: "Main",
                normalized_title: "x",
              },
            ],
          },
        ],
      }).success,
    ).toBe(false);
  });

  it("parseDocument 返回推导类型文档", () => {
    const doc = parseDocument(readFixture("valid", "full_document.json"));
    expect(doc.type).toBe("document");
    expect(doc.schema_version).toBe(1);
    expect(doc.children.length).toBeGreaterThan(0);
    expect(doc.children[0]?.type).toBe("heading");
  });
});
