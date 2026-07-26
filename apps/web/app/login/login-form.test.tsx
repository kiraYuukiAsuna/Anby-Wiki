import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { LoginForm } from "./login-form";

const mocks = vi.hoisted(() => ({
  login: vi.fn(),
  replace: vi.fn(),
  search: "?next=/new",
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/lib/api", () => ({
  authApi: () => ({ login: mocks.login }),
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

  it("用账号密码建立会话并返回站内 next", async () => {
    mocks.login.mockResolvedValue({
      actorId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      displayName: "Alice",
    });
    render(<LoginForm />);

    fireEvent.change(screen.getByLabelText("用户名或邮箱"), {
      target: { value: "alice@example.com" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "correct horse battery staple" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() =>
      expect(mocks.login).toHaveBeenCalledWith({
        loginRequest: {
          identifier: "alice@example.com",
          password: "correct horse battery staple",
        },
      }),
    );
    expect(mocks.toast.success).toHaveBeenCalledWith("已登录为 Alice");
    expect(mocks.replace).toHaveBeenCalledWith("/new");
  });

  it("拒绝外部 next，成功后回到首页", async () => {
    mocks.search = "?next=//evil.example";
    mocks.login.mockResolvedValue({
      actorId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      displayName: "Alice",
    });
    render(<LoginForm />);

    fireEvent.change(screen.getByLabelText("用户名或邮箱"), {
      target: { value: "alice" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "correct horse battery staple" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/"));
  });

  it("密码无效时保留表单并提示", async () => {
    mocks.login.mockRejectedValue(
      new ResponseError(new Response(null, { status: 401 }), "401"),
    );
    render(<LoginForm />);

    fireEvent.change(screen.getByLabelText("用户名或邮箱"), {
      target: { value: "alice" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "wrong-password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() =>
      expect(mocks.toast.error).toHaveBeenCalledWith("用户名、邮箱或密码错误"),
    );
    expect(mocks.replace).not.toHaveBeenCalled();
    expect(screen.getByLabelText("密码")).toHaveValue("wrong-password");
  });
});
