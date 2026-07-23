import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ResponseError } from "../../../contracts/generated/typescript";

import { AccountMenu } from "./account-menu";

const mocks = vi.hoisted(() => ({
  getSession: vi.fn(),
  logout: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/lib/api", () => ({
  authApi: () => ({
    getSession: mocks.getSession,
    logout: mocks.logout,
  }),
}));
vi.mock("sonner", () => ({ toast: mocks.toast }));

function renderAccount() {
  return render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <AccountMenu />
    </SWRConfig>,
  );
}

describe("AccountMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("SWR session 成功时展示账户并可退出", async () => {
    mocks.getSession.mockResolvedValue({
      actorId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      actorType: "human",
      displayName: "Alice",
      method: "session",
    });
    mocks.logout.mockResolvedValue(undefined);
    renderAccount();

    expect(await screen.findByText("Alice")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "退出" }));
    await waitFor(() => expect(mocks.logout).toHaveBeenCalledOnce());
    expect(mocks.toast.success).toHaveBeenCalledWith("已退出登录");
  });

  it("session 返回 401 时展示登录入口且不重试", async () => {
    mocks.getSession.mockRejectedValue(
      new ResponseError(new Response(null, { status: 401 }), "401"),
    );
    renderAccount();

    expect(await screen.findByRole("link", { name: "登录" })).toHaveAttribute(
      "href",
      "/api/v1/auth/login",
    );
    expect(mocks.getSession).toHaveBeenCalledOnce();
  });
});
