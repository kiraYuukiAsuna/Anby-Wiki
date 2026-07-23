// TOC 提取函数测试。
import * as fs from "node:fs";
import * as path from "node:path";

import { describe, expect, it } from "vitest";

import { parseDocument } from "./schema";
import { extractToc, inlineToPlainText } from "./toc";

// 与 schema.test.ts 同一约定：vitest 以 apps/web 为 cwd 运行。
function readFixture(name: string) {
  return JSON.parse(
    fs.readFileSync(
      path.resolve(
        process.cwd(),
        "../../contracts/schemas/ast/v1/fixtures/valid",
        name,
      ),
      "utf8",
    ),
  );
}

const fullDocument = readFixture("full_document.json");
const minimalDocument = readFixture("minimal.json");

describe("extractToc", () => {
  it("空文档返回空目录", () => {
    expect(extractToc(parseDocument(minimalDocument))).toEqual([]);
  });

  it("提取 heading 的 id/level/纯文本", () => {
    const toc = extractToc(parseDocument(fullDocument));
    expect(toc).toEqual([
      {
        id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
        level: 1,
        text: "示例页面",
      },
    ]);
  });

  it("嵌套在 quote/callout 内的 heading 也会被提取，保持文档顺序", () => {
    const doc = parseDocument({
      type: "document",
      schema_version: 1,
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e01",
          type: "quote",
          children: [
            {
              id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e02",
              type: "heading",
              level: 3,
              content: [{ type: "text", text: "引用里的标题" }],
            },
          ],
        },
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e03",
          type: "heading",
          level: 2,
          content: [
            { type: "text", text: "参见 " },
            {
              type: "page_reference",
              target_page_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
              display_text: "另一页",
            },
          ],
        },
      ],
    });
    expect(extractToc(doc)).toEqual([
      {
        id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e02",
        level: 3,
        text: "引用里的标题",
      },
      {
        id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e03",
        level: 2,
        text: "参见 另一页",
      },
    ]);
  });
});

describe("inlineToPlainText", () => {
  it("覆盖全部 inline 形态", () => {
    expect(
      inlineToPlainText([
        { type: "text", text: "a", marks: ["bold"] },
        { type: "inline_code", text: "b" },
        {
          type: "page_reference",
          target_page_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
          display_text: "c",
        },
        {
          type: "page_reference",
          resolution_status: "unresolved",
          target_namespace: "Main",
          normalized_title: "d",
        },
        {
          type: "external_link",
          url: "https://example.com",
          display_text: "e",
        },
      ]),
    ).toBe("abcde");
  });
});
