import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { LoginForm } from "./login-form";

const mocks = vi.hoisted(() => ({
  devLogin: vi.fn(),
  replace: vi.fn(),
  search: "?next=/new",
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/lib/api", () => ({
  authApi: () => ({ devLogin: mocks.devLogin }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mocks.replace }),
  useSearchParams: () => new URLSearchParams(mocks.search),
}));

vi.mock("sonner", () => ({ toast: mocks.toast }));

describe("LoginForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.search = "?next=/new";
  });

  it("用引导令牌建立会话并返回站内 next", async () => {
    mocks.devLogin.mockResolvedValue({
      actorId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      displayName: "Alice",
    });
    render(<LoginForm />);

    fireEvent.change(screen.getByLabelText("引导令牌"), {
      target: { value: "bootstrap-secret" },
    });
    fireEvent.change(screen.getByLabelText("显示名（可选）"), {
      target: { value: "Alice" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() =>
      expect(mocks.devLogin).toHaveBeenCalledWith({
        devLoginRequest: {
          token: "bootstrap-secret",
          displayName: "Alice",
        },
      }),
    );
    expect(mocks.toast.success).toHaveBeenCalledWith("已登录为 Alice");
    expect(mocks.replace).toHaveBeenCalledWith("/new");
  });

  it("拒绝外部 next，成功后回到首页", async () => {
    mocks.search = "?next=//evil.example";
    mocks.devLogin.mockResolvedValue({
      actorId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      displayName: "Bootstrap user",
    });
    render(<LoginForm />);

    fireEvent.change(screen.getByLabelText("引导令牌"), {
      target: { value: "bootstrap-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/"));
  });

  it("令牌无效时保留表单并提示", async () => {
    mocks.devLogin.mockRejectedValue(
      new ResponseError(new Response(null, { status: 401 }), "401"),
    );
    render(<LoginForm />);

    fireEvent.change(screen.getByLabelText("引导令牌"), {
      target: { value: "wrong-token" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() =>
      expect(mocks.toast.error).toHaveBeenCalledWith("引导令牌无效"),
    );
    expect(mocks.replace).not.toHaveBeenCalled();
    expect(screen.getByLabelText("引导令牌")).toHaveValue("wrong-token");
  });
});
