// BlockEditor 受控包装组件冒烟测试：初始 AST 渲染进编辑器，
// 自定义块/行内（callout、pageReference、inlineCode）出现在 DOM 中。
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { parseDocument } from "@/lib/ast/schema";

import { BlockEditor } from "./block-editor";

const DOC = parseDocument({
  type: "document",
  schema_version: 1,
  children: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      type: "paragraph",
      content: [
        { type: "text", text: "含 " },
        { type: "inline_code", text: "code-x" },
        {
          type: "page_reference",
          target_page_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
          display_text: "已解析页面",
        },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02",
      type: "callout",
      kind: "danger",
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a03",
          type: "paragraph",
          content: [{ type: "text", text: "危险提示" }],
        },
      ],
    },
  ],
});

describe("BlockEditor", () => {
  it("渲染初始 AST，自定义块/行内出现在 DOM 中", () => {
    const { container } = render(<BlockEditor initialAst={DOC} />);
    expect(container.querySelector("[data-block-editor]")).toBeInTheDocument();
    expect(container.querySelector(".bn-editor")).toBeInTheDocument();
    expect(container.querySelector(".ast-inline-code")?.textContent).toBe(
      "code-x",
    );
    expect(container.querySelector(".ast-page-ref")?.textContent).toBe(
      "已解析页面",
    );
    expect(
      container.querySelector(".ast-callout--danger"),
    ).toBeInTheDocument();
  });
});
