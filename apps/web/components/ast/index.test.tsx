// AST 渲染器组件测试：v1 全部 block/inline 形态至少一例。
// 数据直接用 contracts/schemas/ast/v1/fixtures/valid/ 的权威 fixture（组件保持纯，无需 mock fetch）。
import * as fs from "node:fs";
import * as path from "node:path";

import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AstDocument, InlineNodeView } from "./index";
import { parseDocument, type ExternalLinkNode } from "@/lib/ast/schema";

// 与 lib/ast/schema.test.ts 同一约定：vitest 以 apps/web 为 cwd 运行。
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
const nestedLists = readFixture("nested_lists.json");
const tableDocument = readFixture("table.json");
const knowledgeReferences = readFixture("knowledge_references.json");

describe("AstDocument（full_document fixture）", () => {
  it("渲染空文档不报错", () => {
    const { container } = render(
      <AstDocument document={parseDocument(minimalDocument)} />,
    );
    expect(container.querySelector("[data-ast-document]")).toBeInTheDocument();
  });

  it("heading 渲染为带锚点 id 的标题，bold mark 生效", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    const heading = screen.getByRole("heading", { level: 1, name: "示例页面" });
    expect(heading).toHaveAttribute(
      "id",
      "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
    );
    expect(within(heading).getByText("示例页面").tagName).toBe("STRONG");
  });

  it("text marks：bold+italic 组合、strikethrough、code mark", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    const combined = screen.getByText("多种");
    // marks 按数组顺序包裹：bold 在内（strong），italic 在外（em）。
    expect(combined.tagName).toBe("STRONG");
    expect(combined.parentElement?.tagName).toBe("EM");
    expect(screen.getByText("引用内容").closest("s")).not.toBeNull();
    expect(screen.getByText("引用内容").closest("code")).not.toBeNull();
  });

  it("inline_code 渲染为 code", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    expect(screen.getByText("inline_code").tagName).toBe("CODE");
  });

  it("已解析 page_reference 渲染为指向 /pages/{id}#{heading} 的链接", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    const link = screen.getByRole("link", { name: "已解析页面" });
    expect(link).toHaveAttribute(
      "href",
      "/pages/0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01#0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b02",
    );
  });

  it("未解析 page_reference 渲染为带「页面不存在」提示的不可点文本", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    const ref = screen.getByText("wei-lai-de-ye-mian");
    expect(ref.tagName).toBe("SPAN");
    expect(ref).toHaveAttribute("data-unresolved-ref", "wei-lai-de-ye-mian");
    expect(ref).toHaveAttribute(
      "title",
      "页面不存在：wei-lai-de-ye-mian",
    );
    expect(ref).toHaveClass("cursor-not-allowed");
  });

  it("external_link 新窗口打开并带 rel=noopener noreferrer", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    const link = screen.getByRole("link", { name: /外部链接/ });
    expect(link).toHaveAttribute("href", "https://example.com/spec");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("external_link 危险协议在 schema 拒绝且渲染层降级为文本", () => {
    const unsafeDocument = structuredClone(fullDocument);
    const paragraph = unsafeDocument.children.find(
      (block: { type: string }) => block.type === "paragraph",
    );
    const external = paragraph.content.find(
      (node: { type: string }) => node.type === "external_link",
    );
    external.url = "javascript:alert(1)";
    expect(() => parseDocument(unsafeDocument)).toThrow();

    render(
      <InlineNodeView
        node={{
          type: "external_link",
          url: "data:text/html,unsafe",
          display_text: "不安全外链",
        } as ExternalLinkNode}
      />,
    );
    const fallback = screen.getByText("不安全外链");
    expect(fallback.tagName).toBe("SPAN");
    expect(fallback).toHaveAttribute("data-unsafe-external-link");
    expect(screen.queryByRole("link", { name: "不安全外链" })).toBeNull();
  });

  it("bullet_list / ordered_list 渲染为 ul/ol + li", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    const lists = screen.getAllByRole("list");
    expect(lists.some((el) => el.tagName === "UL")).toBe(true);
    expect(lists.some((el) => el.tagName === "OL")).toBe(true);
    expect(screen.getByText("第一项").closest("li")).not.toBeNull();
    expect(screen.getByText("步骤一").closest("li")).not.toBeNull();
  });

  it("table 渲染为表格单元格", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    expect(screen.getByRole("cell", { name: "单元格" })).toBeInTheDocument();
  });

  it("code block 渲染 pre>code 并显示语言标签", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    expect(screen.getByText("go")).toBeInTheDocument();
    expect(
      screen.getByText(/func main\(\)/).closest("pre"),
    ).not.toBeNull();
  });

  it("quote 渲染为 blockquote", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    expect(screen.getByText("引用内容").closest("blockquote")).not.toBeNull();
  });

  it("callout 渲染为带 kind 标签的 note", () => {
    render(<AstDocument document={parseDocument(fullDocument)} />);
    const note = screen.getByRole("note", { name: "警告" });
    expect(within(note).getByText("注意！")).toBeInTheDocument();
  });

  it("divider 渲染为 hr", () => {
    const { container } = render(
      <AstDocument document={parseDocument(fullDocument)} />,
    );
    expect(container.querySelector("hr")).toBeInTheDocument();
  });
});

describe("AstDocument（其余 fixture）", () => {
  it("table fixture：多行单元格与单元格内 callout", () => {
    render(<AstDocument document={parseDocument(tableDocument)} />);
    expect(screen.getAllByRole("cell")).toHaveLength(4);
    expect(
      screen.getByRole("link", { name: "相关页面" }),
    ).toHaveAttribute("href", "/pages/0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01");
    expect(screen.getByRole("note", { name: "信息" })).toBeInTheDocument();
  });

  it("nested_lists fixture：嵌套列表渲染为嵌套 ul/ol", () => {
    render(<AstDocument document={parseDocument(nestedLists)} />);
    const outerItem = screen.getByText("外层第一项").closest("li");
    expect(outerItem).not.toBeNull();
    expect(
      within(outerItem as HTMLElement).getByText("嵌套子项").closest("ol"),
    ).not.toBeNull();
  });

  it("knowledge_references fixture：Entity/Claim/Citation 渲染并按出现顺序编号", () => {
    const { container } = render(
      <AstDocument document={parseDocument(knowledgeReferences)} />,
    );
    const entity = screen.getByRole("link", { name: "安比·德玛拉" });
    expect(entity).toHaveAttribute(
      "href",
      "/entities/0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d01",
    );
    expect(entity).toHaveAttribute("data-entity-ref");

    const claim = screen.getByRole("link", { name: "12 月 20 日" });
    expect(claim).toHaveAttribute(
      "data-claim-ref",
      "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d02",
    );
    expect(claim).toHaveAttribute(
      "href",
      "/claims/0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d02",
    );
    const citations = container.querySelectorAll("[data-citation-ref]");
    expect(citations).toHaveLength(2);
    expect(citations[0]).toHaveTextContent("[1]");
    expect(citations[0].querySelector("a")).toHaveAttribute(
      "href",
      "/citations/0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d03",
    );
    expect(citations[0]).toHaveAttribute(
      "title",
      "《绝区零官方设定集》第 42 页",
    );
    expect(citations[1]).toHaveTextContent("[2]");
    expect(citations[1]).toHaveAttribute(
      "title",
      "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d04",
    );
  });

  it("知识引用展示文本按 React 文本与属性转义，不形成注入节点", () => {
    const document = parseDocument({
      type: "document",
      schema_version: 1,
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01",
          type: "paragraph",
          content: [
            {
              type: "entity_reference",
              entity_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d01",
              display_text: "<img src=x onerror=alert(1)>",
            },
            {
              type: "citation_reference",
              citation_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d02",
              display_text: "\"><script>alert(1)</script>",
            },
          ],
        },
      ],
    });
    const { container } = render(<AstDocument document={document} />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(screen.getByText("<img src=x onerror=alert(1)>")).toBeInTheDocument();
    expect(container.querySelector("sup")).toHaveAttribute(
      "title",
      "\"><script>alert(1)</script>",
    );
  });
});
