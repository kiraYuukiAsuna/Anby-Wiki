// 回滚确认流测试（M2-T05）：确认对话框默认摘要、成功 toast + 跳转阅读页、
// 409 stale_revision 提示刷新重试、401 引导登录。
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  ResponseError,
  type Revision,
} from "../../../../contracts/generated/typescript";

import { RollbackButton } from "./rollback-button";
import { shortId } from "./utils";

// ---- mocks -----------------------------------------------------------------

const mocks = vi.hoisted(() => ({
  rollbackPage: vi.fn(),
  push: vi.fn(),
  toast: {
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
}));

vi.mock("@/lib/api", () => ({
  historyApi: () => ({ rollbackPage: mocks.rollbackPage }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push }),
}));

vi.mock("sonner", () => ({ toast: mocks.toast }));

// ---- fixtures --------------------------------------------------------------

const PAGE_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01";
const ACTOR_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a04";
const TARGET_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a11";

const revision: Revision = {
  id: TARGET_ID,
  pageId: PAGE_ID,
  parentRevisionId: "",
  contentSnapshotId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a20",
  actorId: ACTOR_ID,
  summary: "旧版",
  isMinor: false,
  visibility: "public",
  contentHash: "ab".repeat(32),
  schemaVersion: 1,
  createdAt: new Date("2026-07-01T10:00:00Z"),
};

function conflictError(code: string): ResponseError {
  return new ResponseError(
    new Response(JSON.stringify({ code, message: "x", request_id: "r" }), {
      status: 409,
      headers: { "Content-Type": "application/json" },
    }),
    "409",
  );
}

async function openDialogAndConfirm() {
  fireEvent.click(screen.getByRole("button", { name: "回滚" }));
  const summary = await screen.findByLabelText("修改摘要");
  fireEvent.click(screen.getByRole("button", { name: "确认回滚" }));
  return summary;
}

describe("RollbackButton", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("确认对话框展示目标版本与默认摘要；成功后 toast 并跳转阅读页", async () => {
    mocks.rollbackPage.mockResolvedValue({});
    render(<RollbackButton pageId={PAGE_ID} revision={revision} />);

    const summary = await openDialogAndConfirm();
    // 默认摘要「回滚到 {短id}」
    expect(summary).toHaveValue(`回滚到 ${shortId(TARGET_ID)}`);

    await waitFor(() =>
      expect(mocks.rollbackPage).toHaveBeenCalledWith({
        id: PAGE_ID,
        rollbackRequest: {
          targetRevisionId: TARGET_ID,
          summary: `回滚到 ${shortId(TARGET_ID)}`,
        },
      }),
    );
    await waitFor(() =>
      expect(mocks.push).toHaveBeenCalledWith(`/pages/${PAGE_ID}`),
    );
    expect(mocks.toast.success).toHaveBeenCalled();
  });

  it("409 stale_revision：toast 提示刷新重试，不跳转", async () => {
    mocks.rollbackPage.mockRejectedValue(conflictError("stale_revision"));
    render(<RollbackButton pageId={PAGE_ID} revision={revision} />);

    await openDialogAndConfirm();

    await waitFor(() =>
      expect(mocks.toast.warning).toHaveBeenCalledWith(
        "页面在你操作期间已有新版本",
        expect.objectContaining({
          description: expect.stringContaining("刷新"),
        }),
      ),
    );
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("401：toast 引导登录", async () => {
    mocks.rollbackPage.mockRejectedValue(
      new ResponseError(new Response(null, { status: 401 }), "401"),
    );
    render(<RollbackButton pageId={PAGE_ID} revision={revision} />);

    await openDialogAndConfirm();

    await waitFor(() =>
      expect(mocks.toast.error).toHaveBeenCalledWith("请先登录后再回滚"),
    );
    expect(mocks.rollbackPage).toHaveBeenCalledOnce();
    expect(mocks.push).toHaveBeenCalledWith("/api/v1/auth/login");
  });
});
