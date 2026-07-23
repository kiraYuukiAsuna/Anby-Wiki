// BlockEditor 引用编辑集成测试（M2-T04，mock api 层）：
// 编辑器内渲染两种 page_reference 形态（已解析蓝色 / 未解析红色虚线）；
// 经工具栏打开 cmdk 选择器，搜索（mock pages/search）→ 选中既有页面 →
// 插入已解析引用，onChange 回传的 AST（经 toAst）含目标 ID 与显示文本分离的节点。
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { parseDocument, type Document } from "@/lib/ast/schema";

import { BlockEditor } from "./block-editor";

const searchPagesMock = vi.fn();

vi.mock("@/lib/api", () => ({
  searchApi: () => ({ searchPages: searchPagesMock }),
}));

const RESOLVED_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01";

const DOC_WITH_REFS = parseDocument({
  type: "document",
  schema_version: 1,
  children: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      type: "paragraph",
      content: [
        {
          type: "page_reference",
          target_page_id: RESOLVED_ID,
          display_text: "已解析页面",
        },
        { type: "text", text: " 与 " },
        {
          type: "page_reference",
          resolution_status: "unresolved",
          target_namespace: "main",
          normalized_title: "ghost page",
        },
      ],
    },
  ],
});

const EMPTY_DOC = parseDocument({
  type: "document",
  schema_version: 1,
  children: [],
});

describe("BlockEditor 引用渲染", () => {
  it("已解析引用渲染 .ast-page-ref，未解析引用渲染 .ast-page-ref--unresolved", () => {
    const { container } = render(<BlockEditor initialAst={DOC_WITH_REFS} />);
    const resolved = container.querySelector(
      ".ast-page-ref:not(.ast-page-ref--unresolved)",
    );
    expect(resolved?.textContent).toBe("已解析页面");
    const unresolved = container.querySelector(".ast-page-ref--unresolved");
    expect(unresolved?.textContent).toBe("ghost page");
  });

  it("editable 时渲染引用工具栏（页面引用 / 外链）", () => {
    render(<BlockEditor initialAst={EMPTY_DOC} />);
    expect(
      screen.getByRole("button", { name: /页面引用/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /外链/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "插入标题" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "插入列表" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "插入表格" })).toBeInTheDocument();
  });

  it("常用结构按钮插入 heading/list/table 并经 Adapter 输出 AST", async () => {
    let latest: Document | null = null;
    render(
      <BlockEditor initialAst={EMPTY_DOC} onChange={(ast) => (latest = ast)} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "插入标题" }));
    fireEvent.click(screen.getByRole("button", { name: "插入列表" }));
    fireEvent.click(screen.getByRole("button", { name: "插入表格" }));

    await waitFor(() => expect(latest).not.toBeNull());
    expect(latest!.children.map((block) => block.type).sort()).toEqual([
      "bullet_list",
      "heading",
      "paragraph",
      "table",
    ]);
    expect(latest!.children.find((block) => block.type === "heading")).toMatchObject({
      type: "heading",
      level: 2,
      content: [{ type: "text", text: "章节标题" }],
    });
    expect(
      latest!.children.find((block) => block.type === "bullet_list"),
    ).toMatchObject({ type: "bullet_list" });
    expect(latest!.children.find((block) => block.type === "table")).toMatchObject({
      type: "table",
      children: [{ type: "table_row" }],
    });
  });
});

describe("BlockEditor 经选择器插入引用", () => {
  beforeEach(() => {
    searchPagesMock.mockReset();
    searchPagesMock.mockResolvedValue({
      items: [
        {
          id: RESOLVED_ID,
          displayTitle: "Anby Demara",
          namespace: "main",
          matchedOn: "title",
        },
      ],
    });
  });

  it("搜索 → 选中既有页面 → 插入已解析引用，AST 节点形态正确", async () => {
    let latest: Document | null = null;
    // 独立 SWR 缓存，避免相同 key 在 dedupe 窗口内被去重。
    render(
      <SWRConfig value={{ provider: () => new Map() }}>
        <BlockEditor initialAst={EMPTY_DOC} onChange={(ast) => (latest = ast)} />
      </SWRConfig>,
    );

    fireEvent.click(screen.getByRole("button", { name: /页面引用/ }));
    fireEvent.change(screen.getByPlaceholderText("输入页面标题搜索…"), {
      target: { value: "anby" },
    });
    fireEvent.click(await screen.findByText("Anby Demara"));
    // 显示文本默认页面标题，改成自定义文本后插入。
    fireEvent.change(screen.getByLabelText("显示文本"), {
      target: { value: "安比" },
    });
    fireEvent.click(screen.getByRole("button", { name: "插入引用" }));

    await waitFor(() => expect(latest).not.toBeNull());
    const paragraph = latest!.children[0];
    expect(paragraph).toMatchObject({
      type: "paragraph",
      content: [
        {
          type: "page_reference",
          target_page_id: RESOLVED_ID,
          display_text: "安比",
        },
      ],
    });
  });

  it("创建未解析引用：AST 为 unresolved 形态（规范化标题）", async () => {
    let latest: Document | null = null;
    // 独立 SWR 缓存，避免相同 key 在 dedupe 窗口内被去重。
    render(
      <SWRConfig value={{ provider: () => new Map() }}>
        <BlockEditor initialAst={EMPTY_DOC} onChange={(ast) => (latest = ast)} />
      </SWRConfig>,
    );

    fireEvent.click(screen.getByRole("button", { name: /页面引用/ }));
    fireEvent.change(screen.getByPlaceholderText("输入页面标题搜索…"), {
      target: { value: "Ghost Page" },
    });
    fireEvent.click(
      await screen.findByText("创建未解析引用：Ghost Page"),
    );

    await waitFor(() => expect(latest).not.toBeNull());
    expect(latest!.children[0]).toMatchObject({
      type: "paragraph",
      content: [
        {
          type: "page_reference",
          resolution_status: "unresolved",
          target_namespace: "main",
          normalized_title: "ghost page",
        },
      ],
    });
  });

  it("插入外链：AST external_link 的 url 与 display_text 分离", async () => {
    let latest: Document | null = null;
    // 独立 SWR 缓存，避免相同 key 在 dedupe 窗口内被去重。
    render(
      <SWRConfig value={{ provider: () => new Map() }}>
        <BlockEditor initialAst={EMPTY_DOC} onChange={(ast) => (latest = ast)} />
      </SWRConfig>,
    );

    fireEvent.click(screen.getByRole("button", { name: /外链/ }));
    fireEvent.change(await screen.findByLabelText("链接地址"), {
      target: { value: "https://example.com/ref" },
    });
    fireEvent.change(screen.getByLabelText("显示文本"), {
      target: { value: "参考资料" },
    });
    fireEvent.click(screen.getByRole("button", { name: "插入链接" }));

    await waitFor(() => expect(latest).not.toBeNull());
    expect(latest!.children[0]).toMatchObject({
      type: "paragraph",
      content: [
        {
          type: "external_link",
          url: "https://example.com/ref",
          display_text: "参考资料",
        },
      ],
    });
  });
});
