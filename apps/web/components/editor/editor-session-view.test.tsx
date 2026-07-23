// 编辑会话视图测试（M2-T03）：加载与 dirty 跟踪、发布成功、409 stale_revision
// 保留内容、Zod 校验失败阻断、401 登录引导、草稿落后提示。
// BlockEditor（BlockNote）与 lib/api 均 mock，聚焦会话逻辑。
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { parseDocument, type Document } from "@/lib/ast/schema";
import { draftKey, useEditorSession } from "@/lib/editor-session";

import { EditorSessionView } from "./editor-session-view";

// ---- mocks（vi.hoisted：vi.mock 工厂在模块导入期执行，不能引用顶层 const） ----

const mocks = vi.hoisted(() => ({
  publishRevision: vi.fn(),
  getPageByID: vi.fn(),
  diffRevisions: vi.fn(),
  push: vi.fn(),
  replace: vi.fn(),
  toast: {
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  },
  editedAst: null as Document | null,
  collaborationOptions: vi.fn(),
  collaborationConnect: vi.fn(),
  collaborationSyncAst: vi.fn(),
  collaborationClose: vi.fn(),
  collaborationDocumentId: null as string | null,
}));

vi.mock("@/lib/api", () => ({
  pagesApi: () => ({ publishRevision: mocks.publishRevision }),
  readingApi: () => ({ getPageByID: mocks.getPageByID }),
  historyApi: () => ({ diffRevisions: mocks.diffRevisions }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push, replace: mocks.replace }),
}));

vi.mock("sonner", () => ({ toast: mocks.toast }));

vi.mock("@/lib/collaboration/client", () => ({
  CollaborationClient: class {
    constructor(options: unknown) {
      mocks.collaborationOptions(options);
    }

    connect() {
      mocks.collaborationConnect();
    }

    syncAst(ast: Document) {
      mocks.collaborationSyncAst(ast);
    }

    getDocumentId() {
      return mocks.collaborationDocumentId;
    }

    close() {
      mocks.collaborationClose();
    }
  },
}));

// BlockNote 替换为最小桩：一个按钮模拟一次编辑（onChange 回传 AST）。
vi.mock("@/components/editor/block-editor", () => ({
  BlockEditor: ({ onChange }: { onChange?: (ast: Document) => void }) => (
    <button
      data-testid="mock-edit"
      onClick={() => onChange?.(mocks.editedAst as Document)}
    >
      模拟编辑
    </button>
  ),
}));

// ---- fixtures --------------------------------------------------------------

const PAGE_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01";
const REV_A = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02";
const REV_B = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a03";
const REV_C = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a05";

const SERVER_AST = parseDocument({
  type: "document",
  schema_version: 1,
  children: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a09",
      type: "paragraph",
      content: [{ type: "text", text: "server" }],
    },
  ],
});

const EDITED_AST = parseDocument({
  type: "document",
  schema_version: 1,
  children: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a09",
      type: "paragraph",
      content: [{ type: "text", text: "edited" }],
    },
  ],
});
mocks.editedAst = EDITED_AST;

const LATEST_AST = parseDocument({
  type: "document",
  schema_version: 1,
  children: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a09",
      type: "paragraph",
      content: [{ type: "text", text: "latest server" }],
    },
  ],
});

function mockLatestPage() {
  mocks.getPageByID.mockResolvedValue({
    page: { currentRevisionId: REV_C },
    content: { astJson: LATEST_AST },
  });
}

function conflictError(code: string): ResponseError {
  return new ResponseError(
    new Response(JSON.stringify({ code, message: "x", request_id: "r" }), {
      status: 409,
      headers: { "Content-Type": "application/json" },
    }),
    "409",
  );
}

function unauthorizedError(): ResponseError {
  return new ResponseError(new Response(null, { status: 401 }), "401");
}

function renderView() {
  return render(
    <EditorSessionView
      pageId={PAGE_ID}
      displayTitle="测试页面"
      baseRevisionId={REV_B}
      initialAst={SERVER_AST}
    />,
  );
}

/** 编辑一次 → 打开发布对话框 → 填摘要 → 确认发布。 */
async function editAndConfirmPublish() {
  fireEvent.click(screen.getByTestId("mock-edit"));
  fireEvent.click(screen.getByRole("button", { name: "发布…" }));
  fireEvent.change(await screen.findByLabelText("修改摘要"), {
    target: { value: "更新正文" },
  });
  fireEvent.click(screen.getByRole("button", { name: "确认发布" }));
}

describe("EditorSessionView", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.clearAllMocks();
    mocks.collaborationDocumentId = null;
    useEditorSession.setState({
      pageId: null,
      baseRevisionId: null,
      serverAst: null,
      ast: null,
      dirty: false,
      summary: "",
      isMinor: false,
      publishing: false,
      conflict: null,
      rebasedFromConflict: false,
    });
  });

  it("加载后建立会话；编辑后标 dirty 并显示未发布改动", async () => {
    renderView();
    // 初始：无改动。
    expect(await screen.findByText(/无改动/)).toBeInTheDocument();
    expect(useEditorSession.getState().pageId).toBe(PAGE_ID);
    expect(useEditorSession.getState().dirty).toBe(false);

    fireEvent.click(screen.getByTestId("mock-edit"));
    expect(useEditorSession.getState().dirty).toBe(true);
    expect(useEditorSession.getState().ast).toEqual(EDITED_AST);
    expect(mocks.collaborationSyncAst).toHaveBeenCalledWith(EDITED_AST);
    expect(screen.getByText(/有未发布改动/)).toBeInTheDocument();
  });

  it("接收协作工作副本并显示连接状态", async () => {
    renderView();
    await screen.findByText(/无改动/);
    const options = mocks.collaborationOptions.mock.calls[0][0] as {
      onAst: (ast: Document) => void;
      onStatus: (status: string) => void;
    };

    act(() => {
      options.onStatus("online");
      options.onAst(EDITED_AST);
    });

    expect(screen.getByText("协作在线")).toBeInTheDocument();
    expect(useEditorSession.getState().ast).toEqual(EDITED_AST);
    expect(useEditorSession.getState().dirty).toBe(true);
  });

  it("无改动时点发布只提示，不开对话框", async () => {
    renderView();
    await screen.findByText(/无改动/);
    fireEvent.click(screen.getByRole("button", { name: "发布…" }));
    expect(mocks.toast.info).toHaveBeenCalledWith("内容没有改动，无需发布");
    expect(screen.queryByText("发布修改")).not.toBeInTheDocument();
  });

  it("协作发布成功：携带工作副本与基线，清草稿并跳阅读页", async () => {
    const documentId = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a06";
    mocks.collaborationDocumentId = documentId;
    mocks.publishRevision.mockResolvedValue({});
    renderView();
    await editAndConfirmPublish();

    await waitFor(() =>
      expect(mocks.publishRevision).toHaveBeenCalledWith({
        id: PAGE_ID,
        publishRevisionRequest: {
          expectedRevisionId: REV_B,
          workingDocumentId: documentId,
          ast: EDITED_AST as unknown as { [key: string]: unknown },
          summary: "更新正文",
          isMinor: false,
        },
      }),
    );
    await waitFor(() =>
      expect(mocks.push).toHaveBeenCalledWith(`/pages/${PAGE_ID}`),
    );
    expect(mocks.toast.success).toHaveBeenCalledWith("发布成功");
    // 草稿已清除。
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_B))).toBeNull();
  });

  it("409 stale_revision：拉取 Current、显示常驻冲突条并保留编辑内容", async () => {
    mocks.publishRevision.mockRejectedValue(conflictError("stale_revision"));
    mockLatestPage();
    renderView();
    await editAndConfirmPublish();

    await waitFor(() =>
      expect(mocks.toast.warning).toHaveBeenCalledWith(
        "页面已被他人更新",
        expect.objectContaining({
          description: expect.stringContaining("保留"),
        }),
      ),
    );
    // 内容保留（不清空），不跳转，发布状态复位可重试。
    expect(useEditorSession.getState().ast).toEqual(EDITED_AST);
    expect(useEditorSession.getState().dirty).toBe(true);
    expect(useEditorSession.getState().publishing).toBe(false);
    expect(mocks.push).not.toHaveBeenCalled();
    expect(mocks.getPageByID).toHaveBeenCalledWith({ id: PAGE_ID });
    expect(useEditorSession.getState().conflict).toEqual({
      baseRevisionId: REV_B,
      currentRevisionId: REV_C,
      serverAst: LATEST_AST,
    });
    expect(screen.getByRole("alert")).toHaveTextContent("页面已被他人更新");
    expect(
      screen.getByRole("button", { name: "以最新版为基继续编辑" }),
    ).toBeInTheDocument();
    // 本地草稿也仍在（恢复辅助）。
    expect(
      window.localStorage.getItem(draftKey(PAGE_ID, REV_B)),
    ).not.toBeNull();
  });

  it("冲突后 rebase：保留本地内容，以 Current 为 expected 再发布并提示成功", async () => {
    mocks.publishRevision
      .mockRejectedValueOnce(conflictError("stale_revision"))
      .mockResolvedValueOnce({});
    mockLatestPage();
    renderView();
    await editAndConfirmPublish();
    await screen.findByRole("alert");

    fireEvent.click(
      screen.getByRole("button", { name: "以最新版为基继续编辑" }),
    );
    expect(useEditorSession.getState().conflict).toBeNull();
    expect(useEditorSession.getState().baseRevisionId).toBe(REV_C);
    expect(useEditorSession.getState().ast).toEqual(EDITED_AST);

    fireEvent.click(screen.getByRole("button", { name: "发布…" }));
    fireEvent.click(await screen.findByRole("button", { name: "确认发布" }));
    await waitFor(() => expect(mocks.publishRevision).toHaveBeenCalledTimes(2));
    expect(mocks.publishRevision.mock.calls[1][0]).toMatchObject({
      publishRevisionRequest: { expectedRevisionId: REV_C, ast: EDITED_AST },
    });
    await waitFor(() =>
      expect(mocks.toast.success).toHaveBeenCalledWith(
        "已基于最新版本发布",
      ),
    );
  });

  it("冲突后放弃修改：加载 Current、清草稿并解除 dirty", async () => {
    mocks.publishRevision.mockRejectedValue(conflictError("stale_revision"));
    mockLatestPage();
    renderView();
    await editAndConfirmPublish();
    await screen.findByRole("alert");

    fireEvent.click(screen.getByRole("button", { name: "放弃我的修改" }));
    const state = useEditorSession.getState();
    expect(state.conflict).toBeNull();
    expect(state.ast).toEqual(LATEST_AST);
    expect(state.baseRevisionId).toBe(REV_C);
    expect(state.dirty).toBe(false);
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_B))).toBeNull();
  });

  it("冲突条查看 Base/Current 结构差异", async () => {
    mocks.publishRevision.mockRejectedValue(conflictError("stale_revision"));
    mocks.diffRevisions.mockResolvedValue({ changes: [] });
    mockLatestPage();
    renderView();
    await editAndConfirmPublish();
    await screen.findByRole("alert");

    fireEvent.click(screen.getByRole("button", { name: "查看差异" }));
    await waitFor(() =>
      expect(mocks.diffRevisions).toHaveBeenCalledWith({
        id: PAGE_ID,
        from: REV_B,
        to: REV_C,
      }),
    );
  });

  it("网络错误：toast 错误，内容保留可重试", async () => {
    mocks.publishRevision.mockRejectedValue(new Error("network down"));
    renderView();
    await editAndConfirmPublish();

    await waitFor(() =>
      expect(mocks.toast.error).toHaveBeenCalledWith(
        "网络错误，发布未完成",
        expect.anything(),
      ),
    );
    expect(useEditorSession.getState().ast).toEqual(EDITED_AST);
    expect(useEditorSession.getState().publishing).toBe(false);
  });

  it("Zod 校验失败：阻断发布，不调用 API", async () => {
    renderView();
    await screen.findByText(/无改动/);
    // 直接注入非法 AST（绕过编辑器，模拟会话内容损坏）；act 包裹确保重渲染后再点击。
    act(() =>
      useEditorSession.setState({
        ast: { type: "bogus" } as unknown as Document,
        dirty: true,
      }),
    );
    await screen.findByText(/有未发布改动/);
    fireEvent.click(screen.getByRole("button", { name: "发布…" }));
    fireEvent.click(await screen.findByRole("button", { name: "确认发布" }));
    await waitFor(() =>
      expect(mocks.toast.error).toHaveBeenCalledWith(
        "内容校验失败，无法发布",
        expect.anything(),
      ),
    );
    expect(mocks.publishRevision).not.toHaveBeenCalled();
  });

  it("401：保留编辑内容并引导登录", async () => {
    mocks.publishRevision.mockRejectedValue(unauthorizedError());
    renderView();
    await editAndConfirmPublish();
    await waitFor(() =>
      expect(mocks.toast.error).toHaveBeenCalledWith(
        "请先登录后再发布",
        expect.objectContaining({
          description: expect.stringContaining("保留"),
        }),
      ),
    );
    expect(mocks.publishRevision).toHaveBeenCalledOnce();
    expect(mocks.push).toHaveBeenCalledWith("/api/v1/auth/login");
    expect(useEditorSession.getState().ast).toEqual(EDITED_AST);
  });

  it("草稿 base 落后于服务端 current：toast 提示并保留草稿内容", async () => {
    // 旧 base（REV_A）的草稿；服务端 current 已是 REV_B。
    window.localStorage.setItem(
      draftKey(PAGE_ID, REV_A),
      JSON.stringify({ ast: EDITED_AST, savedAt: new Date().toISOString() }),
    );
    renderView();
    await waitFor(() =>
      expect(mocks.toast.warning).toHaveBeenCalledWith(
        "本地草稿基于旧版本，已保留草稿内容",
        expect.anything(),
      ),
    );
    const state = useEditorSession.getState();
    expect(state.ast).toEqual(EDITED_AST);
    expect(state.baseRevisionId).toBe(REV_B);
    expect(state.dirty).toBe(true);
    expect(state.conflict).toMatchObject({
      baseRevisionId: REV_A,
      currentRevisionId: REV_B,
    });
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });
});
