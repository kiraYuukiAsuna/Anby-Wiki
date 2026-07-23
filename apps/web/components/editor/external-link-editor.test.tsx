// 外部链接编辑器测试（M2-T04）：合法/非法 URL（Zod 仅允许 http/https）、
// display_text 与 url 分离（留空回退为 url）。
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ExternalLinkEditor } from "./external-link-editor";

function renderEditor(
  overrides: Partial<Parameters<typeof ExternalLinkEditor>[0]> = {},
) {
  const props = {
    open: true,
    onOpenChange: vi.fn(),
    onSubmit: vi.fn(),
    ...overrides,
  };
  render(<ExternalLinkEditor {...props} />);
  return props;
}

function fillUrl(url: string) {
  fireEvent.change(screen.getByLabelText("链接地址"), {
    target: { value: url },
  });
}

describe("ExternalLinkEditor", () => {
  beforeEach(() => vi.clearAllMocks());

  it("合法 https URL + display_text 分离回调", () => {
    const props = renderEditor();
    fillUrl("https://example.com/page");
    fireEvent.change(screen.getByLabelText("显示文本"), {
      target: { value: "参考资料" },
    });
    fireEvent.click(screen.getByRole("button", { name: "插入链接" }));
    expect(props.onSubmit).toHaveBeenCalledWith(
      "https://example.com/page",
      "参考资料",
    );
    expect(props.onOpenChange).toHaveBeenCalledWith(false);
  });

  it("display_text 留空时回退为 url 本身", () => {
    const props = renderEditor();
    fillUrl("https://example.com");
    fireEvent.click(screen.getByRole("button", { name: "插入链接" }));
    expect(props.onSubmit).toHaveBeenCalledWith(
      "https://example.com",
      "https://example.com",
    );
  });

  it.each(["not-a-url", "ftp://example.com/x", "javascript:alert(1)"])(
    "非法/非 http(s) URL 被拒绝：%s",
    (bad) => {
      const props = renderEditor();
      fillUrl(bad);
      fireEvent.click(screen.getByRole("button", { name: "插入链接" }));
      expect(props.onSubmit).not.toHaveBeenCalled();
      expect(screen.getByRole("alert")).toHaveTextContent("http/https");
    },
  );

  it("打开时注入默认显示文本（选中文本）", () => {
    renderEditor({ defaultDisplayText: "选中的文字" });
    expect(screen.getByLabelText("显示文本")).toHaveValue("选中的文字");
  });
});
