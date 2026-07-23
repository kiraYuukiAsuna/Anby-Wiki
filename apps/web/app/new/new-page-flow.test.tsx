// /new 创建页面流程测试（M2-T03）：创建成功跳编辑页、标题冲突跳既有页面、
// 401 登录引导、无 title 时的标题输入表单。pagesApi 与路由均 mock。
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { NewPageFlow } from "./new-page-flow";

const mocks = vi.hoisted(() => ({
  createPage: vi.fn(),
  push: vi.fn(),
  replace: vi.fn(),
  search: "?title=新条目&namespace=main",
  toast: {
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}));

vi.mock("@/lib/api", () => ({
  pagesApi: () => ({ createPage: mocks.createPage }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push, replace: mocks.replace }),
  useSearchParams: () => new URLSearchParams(mocks.search),
}));

vi.mock("sonner", () => ({ toast: mocks.toast }));

const NEW_PAGE_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a05";

function conflict409(): ResponseError {
  return new ResponseError(
    new Response(
      JSON.stringify({ code: "conflict", message: "x", request_id: "r" }),
      { status: 409, headers: { "Content-Type": "application/json" } },
    ),
    "409",
  );
}

function unauthorized401(): ResponseError {
  return new ResponseError(new Response(null, { status: 401 }), "401");
}

describe("NewPageFlow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.search = "?title=新条目&namespace=main";
  });

  it("创建成功：调用 createPage 并跳转 /pages/[id]/edit", async () => {
    mocks.createPage.mockResolvedValue({ id: NEW_PAGE_ID, displayTitle: "新条目" });
    render(<NewPageFlow />);

    await waitFor(() =>
      expect(mocks.createPage).toHaveBeenCalledWith({
        createPageRequest: { namespace: "main", title: "新条目" },
      }),
    );
    await waitFor(() =>
      expect(mocks.replace).toHaveBeenCalledWith(`/pages/${NEW_PAGE_ID}/edit`),
    );
  });

  it("标题冲突（409）：toast 提示并跳既有页面阅读页", async () => {
    mocks.createPage.mockRejectedValue(conflict409());
    render(<NewPageFlow />);

    await waitFor(() =>
      expect(mocks.replace).toHaveBeenCalledWith(
        `/wiki/${encodeURIComponent("新条目")}`,
      ),
    );
    expect(mocks.toast.info).toHaveBeenCalledWith(
      "页面已存在",
      expect.anything(),
    );
  });

  it("服务端错误：停留本页并可重试", async () => {
    mocks.createPage.mockRejectedValue(new Error("boom"));
    render(<NewPageFlow />);

    await screen.findByText("创建失败");
    expect(mocks.replace).not.toHaveBeenCalled();

    mocks.createPage.mockResolvedValue({ id: NEW_PAGE_ID, displayTitle: "新条目" });
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await waitFor(() =>
      expect(mocks.replace).toHaveBeenCalledWith(`/pages/${NEW_PAGE_ID}/edit`),
    );
    expect(mocks.createPage).toHaveBeenCalledTimes(2);
  });

  it("401：引导 OIDC 登录", async () => {
    mocks.createPage.mockRejectedValue(unauthorized401());
    render(<NewPageFlow />);
    expect(await screen.findByText("需要登录")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "前往登录" }),
    ).toHaveAttribute("href", "/api/v1/auth/login");
    expect(mocks.createPage).toHaveBeenCalledOnce();
  });

  it("无 title 参数：渲染标题输入表单，提交后带参数跳转", async () => {
    mocks.search = "";
    render(<NewPageFlow />);
    const input = await screen.findByLabelText("页面标题");
    fireEvent.change(input, { target: { value: "另一个条目" } });
    fireEvent.click(screen.getByRole("button", { name: "创建并编辑" }));
    expect(mocks.push).toHaveBeenCalledWith(
      `/new?title=${encodeURIComponent("另一个条目")}&namespace=main`,
    );
    expect(mocks.createPage).not.toHaveBeenCalled();
  });
});
