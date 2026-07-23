// 历史列表测试（M2-T05）：行渲染（徽章/占位摘要/时间）、SWR「加载更多」翻页、
// 复选两版跳 diff、「与上一版对比」快捷链接。
// lib/api（historyApi）、next/navigation、sonner 均 mock，聚焦列表交互逻辑。
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Revision } from "../../../../contracts/generated/typescript";

import { RevisionList } from "./revision-list";
import { shortId } from "./utils";

// ---- mocks（vi.hoisted：vi.mock 工厂在模块导入期执行，不能引用顶层 const） ----

const mocks = vi.hoisted(() => ({
  listRevisions: vi.fn(),
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
  historyApi: () => ({
    listRevisions: mocks.listRevisions,
    rollbackPage: mocks.rollbackPage,
  }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push }),
}));

vi.mock("sonner", () => ({ toast: mocks.toast }));

// ---- fixtures --------------------------------------------------------------

const PAGE_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01";
const ACTOR_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a04";
// 注意：fixture UUID 前 8 位必须互不相同（短 id 展示取前 8 位）。
const REV_NEW = "11111111-c3d4-7e5f-8a9b-0c1d2e3f4a10";
const REV_OLD = "22222222-c3d4-7e5f-8a9b-0c1d2e3f4a11";
const REV_PAGE2 = "33333333-c3d4-7e5f-8a9b-0c1d2e3f4a12";

function makeRevision(id: string, overrides: Partial<Revision> = {}): Revision {
  return {
    id,
    pageId: PAGE_ID,
    parentRevisionId: "",
    contentSnapshotId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a20",
    actorId: ACTOR_ID,
    summary: "",
    isMinor: false,
    visibility: "public",
    contentHash: "ab".repeat(32),
    schemaVersion: 1,
    createdAt: new Date("2026-07-01T10:00:00Z"),
    ...overrides,
  };
}

const revNew = makeRevision(REV_NEW, { summary: "第二版" });
const revOld = makeRevision(REV_OLD, { isMinor: true });
const revPage2 = makeRevision(REV_PAGE2, { summary: "更早的版本" });

function renderList(nextCursor: string | null = null) {
  return render(
    <RevisionList
      pageId={PAGE_ID}
      currentRevisionId={REV_NEW}
      initialPage={{ items: [revNew, revOld], nextCursor: nextCursor ?? "" }}
    />,
  );
}

describe("RevisionList", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.clearAllMocks();
  });

  it("渲染行：短 id、时间、actor 短 id、摘要与徽章", () => {
    renderList();
    expect(screen.getByText(`版本 ${shortId(REV_NEW)}`)).toBeInTheDocument();
    expect(screen.getByText("第二版")).toBeInTheDocument();
    expect(screen.getByText("当前版本")).toBeInTheDocument();
    expect(screen.getByText("小修改")).toBeInTheDocument();
    // 无摘要时占位
    expect(screen.getByText("（无摘要）")).toBeInTheDocument();
    // actor 短 id 两行各一个
    expect(
      screen.getAllByText(`编辑者 ${shortId(ACTOR_ID)}`),
    ).toHaveLength(2);
    // 本地化时间（zh-CN medium+short）
    expect(screen.getAllByText(/2026/)).not.toHaveLength(0);
  });

  it("「加载更多」按 next_cursor 翻页并追加；无更多时按钮消失", async () => {
    mocks.listRevisions.mockResolvedValue({
      items: [revPage2],
      next_cursor: null,
      nextCursor: null,
    });
    renderList("cursor-2");

    fireEvent.click(screen.getByRole("button", { name: "加载更多" }));

    await screen.findByText(`版本 ${shortId(REV_PAGE2)}`);
    expect(mocks.listRevisions).toHaveBeenCalledWith({
      id: PAGE_ID,
      cursor: "cursor-2",
      pageSize: 20,
    });
    expect(
      screen.queryByRole("button", { name: "加载更多" }),
    ).not.toBeInTheDocument();
  });

  it("复选两个版本后「对比所选」跳转 diff（from=旧版, to=新版）", () => {
    renderList();
    fireEvent.click(
      screen.getByRole("checkbox", { name: `选择版本 ${shortId(REV_NEW)}` }),
    );
    fireEvent.click(
      screen.getByRole("checkbox", { name: `选择版本 ${shortId(REV_OLD)}` }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "对比所选（2/2）" }),
    );
    expect(mocks.push).toHaveBeenCalledWith(
      `/pages/${PAGE_ID}/diff?from=${REV_OLD}&to=${REV_NEW}`,
    );
  });

  it("「与上一版对比」快捷链接指向相邻旧版", () => {
    renderList();
    const link = screen.getByRole("link", { name: "与上一版对比" });
    expect(link).toHaveAttribute(
      "href",
      `/pages/${PAGE_ID}/diff?from=${REV_OLD}&to=${REV_NEW}`,
    );
    // 最旧的一行没有「与上一版对比」
    expect(
      screen.getAllByRole("link", { name: "与上一版对比" }),
    ).toHaveLength(1);
  });

  it("空历史显示占位说明", () => {
    render(
      <RevisionList
        pageId={PAGE_ID}
        currentRevisionId={null}
        initialPage={{ items: [], nextCursor: "" }}
      />,
    );
    expect(screen.getByText("本页面尚未发布任何版本。")).toBeInTheDocument();
  });
});
