import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ResponseError } from "../../../contracts/generated/typescript";

import { RenamePageButton } from "./rename-page-button";

const mocks = vi.hoisted(() => ({
  renamePage: vi.fn(),
  push: vi.fn(),
  refresh: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/lib/api", () => ({
  pagesApi: () => ({ renamePage: mocks.renamePage }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push, refresh: mocks.refresh }),
}));
vi.mock("sonner", () => ({ toast: mocks.toast }));

const PAGE_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01";

describe("RenamePageButton", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("经生成客户端改名并跳到新标题路由", async () => {
    mocks.renamePage.mockResolvedValue({ displayTitle: "新标题" });
    render(<RenamePageButton pageId={PAGE_ID} currentTitle="旧标题" />);

    fireEvent.click(screen.getByRole("button", { name: "改名" }));
    fireEvent.change(screen.getByLabelText("新标题"), {
      target: { value: "新标题" },
    });
    fireEvent.click(screen.getByRole("button", { name: "确认改名" }));

    await waitFor(() =>
      expect(mocks.renamePage).toHaveBeenCalledWith({
        id: PAGE_ID,
        renamePageRequest: { title: "新标题" },
      }),
    );
    expect(mocks.push).toHaveBeenCalledWith(
      `/wiki/${encodeURIComponent("新标题")}`,
    );
    expect(mocks.refresh).toHaveBeenCalled();
  });

  it("401 时引导登录", async () => {
    mocks.renamePage.mockRejectedValue(
      new ResponseError(new Response(null, { status: 401 }), "401"),
    );
    render(<RenamePageButton pageId={PAGE_ID} currentTitle="旧标题" />);
    fireEvent.click(screen.getByRole("button", { name: "改名" }));
    fireEvent.change(screen.getByLabelText("新标题"), {
      target: { value: "新标题" },
    });
    fireEvent.click(screen.getByRole("button", { name: "确认改名" }));
    await waitFor(() =>
      expect(mocks.push).toHaveBeenCalledWith("/api/v1/auth/login"),
    );
    expect(mocks.toast.error).toHaveBeenCalledWith("请先登录后再改名");
  });
});
