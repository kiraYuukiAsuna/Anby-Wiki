import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CreatePageLink } from "./create-page-link";
import WikiTitleNotFound from "./not-found";

describe("/wiki/[title] not-found", () => {
  it("渲染 404 说明与「创建此页面」真实入口", () => {
    render(<WikiTitleNotFound />);
    expect(
      screen.getByRole("heading", { name: "页面不存在" }),
    ).toBeInTheDocument();
    const create = screen.getByRole("link", { name: "创建此页面" });
    // 地址栏无 /wiki/ 前缀时退化为 /new（无预填标题）。
    expect(create).toHaveAttribute("href", "/new");
    expect(screen.getByText(/斜杠/)).toBeInTheDocument();
  });
});

describe("CreatePageLink", () => {
  it("从地址栏恢复标题并预填 /new?title=...", async () => {
    window.history.pushState({}, "", "/wiki/%E6%96%B0%E6%9D%A1%E7%9B%AE");
    render(<CreatePageLink />);
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "创建此页面" })).toHaveAttribute(
        "href",
        `/new?title=${encodeURIComponent("新条目")}&namespace=main`,
      ),
    );
    window.history.pushState({}, "", "/");
  });

  it("含斜杠的标题不预填（v1 不支持）", async () => {
    window.history.pushState({}, "", "/wiki/a%2Fb");
    render(<CreatePageLink />);
    // effect 执行后仍为 /new。
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "创建此页面" })).toHaveAttribute(
        "href",
        "/new",
      ),
    );
    window.history.pushState({}, "", "/");
  });
});
