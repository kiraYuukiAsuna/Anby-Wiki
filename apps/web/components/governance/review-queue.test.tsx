import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  ResponseError,
  type ReviewTaskList,
} from "../../../../contracts/generated/typescript";

import { ReviewQueue } from "./review-queue";

const mocks = vi.hoisted(() => ({
  listReviewTasks: vi.fn(),
  decideReviewTask: vi.fn(),
  push: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/lib/api", () => ({
  governanceApi: () => ({
    listReviewTasks: mocks.listReviewTasks,
    decideReviewTask: mocks.decideReviewTask,
  }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push }),
}));
vi.mock("sonner", () => ({ toast: mocks.toast }));

const initialData: ReviewTaskList = {
  items: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a10",
      proposalId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a11",
      status: "pending",
      reviewerId: "",
      decisionReason: "",
      createdAt: new Date("2026-07-22T00:00:00Z"),
      reviewedAt: new Date(0),
    },
  ],
};

describe("ReviewQueue", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.listReviewTasks.mockResolvedValue(initialData);
    mocks.decideReviewTask.mockResolvedValue({});
  });

  it("要求审核理由并通过 session 提交决定", async () => {
    render(<ReviewQueue initialData={initialData} />);

    fireEvent.change(screen.getByLabelText("审核理由"), {
      target: { value: "来源充分且影响范围可控" },
    });
    fireEvent.click(screen.getByRole("button", { name: "批准" }));

    await waitFor(() =>
      expect(mocks.decideReviewTask).toHaveBeenCalledWith({
        id: initialData.items[0].id,
        reviewDecisionRequest: {
          approve: true,
          reason: "来源充分且影响范围可控",
        },
      }),
    );
    expect(mocks.toast.success).toHaveBeenCalledWith("提案已批准");
  });

  it("401 时引导登录", async () => {
    mocks.decideReviewTask.mockRejectedValue(
      new ResponseError(new Response(null, { status: 401 }), "401"),
    );
    render(<ReviewQueue initialData={initialData} />);
    fireEvent.change(screen.getByLabelText("审核理由"), {
      target: { value: "拒绝理由" },
    });
    fireEvent.click(screen.getByRole("button", { name: "拒绝" }));

    await waitFor(() =>
      expect(mocks.push).toHaveBeenCalledWith("/api/v1/auth/login"),
    );
    expect(mocks.decideReviewTask).toHaveBeenCalledOnce();
  });
});
