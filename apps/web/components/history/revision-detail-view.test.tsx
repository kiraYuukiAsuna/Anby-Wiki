// 单版详情视图测试（M2-T05）：Revision 元信息、AST 只读渲染（复用 components/ast）、
// 回滚入口与当前版本徽章。
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  Revision,
  RevisionDetail,
} from "../../../../contracts/generated/typescript";

import { RevisionDetailView } from "./revision-detail-view";
import { shortId } from "./utils";

// ---- mocks（RollbackButton 依赖） ------------------------------------------

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
// 注意：REV_ID/PARENT_ID 前 8 位须与 ACTOR_ID 及彼此不同（短 id 展示取前 8 位）。
const REV_ID = "11111111-c3d4-7e5f-8a9b-0c1d2e3f4a11";
const PARENT_ID = "22222222-c3d4-7e5f-8a9b-0c1d2e3f4a12";

const revision: Revision = {
  id: REV_ID,
  pageId: PAGE_ID,
  parentRevisionId: PARENT_ID,
  contentSnapshotId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a20",
  actorId: ACTOR_ID,
  summary: "第二版",
  isMinor: false,
  visibility: "public",
  contentHash: "ab".repeat(32),
  schemaVersion: 1,
  createdAt: new Date("2026-07-01T10:00:00Z"),
};

const detail: RevisionDetail = {
  revision,
  astJson: {
    type: "document",
    schema_version: 1,
    children: [
      {
        id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a09",
        type: "paragraph",
        content: [{ type: "text", text: "历史正文内容" }],
      },
    ],
  },
};

describe("RevisionDetailView", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.clearAllMocks();
  });

  it("渲染元信息、AST 内容与回滚入口", () => {
    render(
      <RevisionDetailView
        pageId={PAGE_ID}
        detail={detail}
        currentRevisionId={REV_ID}
      />,
    );
    expect(screen.getByText(`版本 ${shortId(REV_ID)}`)).toBeInTheDocument();
    expect(screen.getByText("当前版本")).toBeInTheDocument();
    expect(screen.getByText("第二版")).toBeInTheDocument();
    expect(screen.getByText(shortId(ACTOR_ID))).toBeInTheDocument();
    expect(screen.getByText(shortId(PARENT_ID))).toBeInTheDocument();
    // AST 只读渲染
    expect(screen.getByText("历史正文内容")).toBeInTheDocument();
    // 回滚入口
    expect(screen.getByRole("button", { name: "回滚" })).toBeInTheDocument();
  });

  it("无摘要时显示占位；非当前版本不显示徽章", () => {
    render(
      <RevisionDetailView
        pageId={PAGE_ID}
        detail={{ ...detail, revision: { ...revision, summary: "" } }}
        currentRevisionId={null}
      />,
    );
    expect(screen.getByText("（无摘要）")).toBeInTheDocument();
    expect(screen.queryByText("当前版本")).not.toBeInTheDocument();
  });
});
