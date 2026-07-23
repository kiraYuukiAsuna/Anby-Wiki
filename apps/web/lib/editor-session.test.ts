// 编辑会话 store 测试（M2-T03）：草稿恢复（fresh/stale/none）、dirty 跟踪、
// 丢弃与清除草稿、localStorage key 规则。草稿只是恢复辅助，绝不自动发布。
import { beforeEach, describe, expect, it } from "vitest";

import { parseDocument, type Document } from "@/lib/ast/schema";

import { draftKey, useEditorSession } from "./editor-session";

const PAGE_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01";
const REV_A = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02";
const REV_B = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a03";

function doc(text: string): Document {
  return parseDocument({
    type: "document",
    schema_version: 1,
    children: [
      {
        id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a09",
        type: "paragraph",
        content: [{ type: "text", text }],
      },
    ],
  });
}

function seedDraft(
  pageId: string,
  baseRevisionId: string | null,
  ast: Document,
): void {
  window.localStorage.setItem(
    draftKey(pageId, baseRevisionId),
    JSON.stringify({ ast, savedAt: new Date().toISOString() }),
  );
}

describe("useEditorSession", () => {
  beforeEach(() => {
    window.localStorage.clear();
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

  it("无草稿：以服务端内容开始，不 dirty", () => {
    const server = doc("server");
    const restore = useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: server });
    const state = useEditorSession.getState();
    expect(restore).toBe("none");
    expect(state.ast).toEqual(server);
    expect(state.dirty).toBe(false);
    expect(state.baseRevisionId).toBe(REV_A);
  });

  it("setAst 标 dirty 并按 pageId+baseRevisionId 持久化草稿", () => {
    useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: doc("s") });
    const edited = doc("edited");
    useEditorSession.getState().setAst(edited);

    const state = useEditorSession.getState();
    expect(state.dirty).toBe(true);
    const raw = window.localStorage.getItem(draftKey(PAGE_ID, REV_A));
    expect(raw).not.toBeNull();
    expect(parseDocument(JSON.parse(raw as string).ast)).toEqual(edited);
  });

  it("recordConflict 幂等；rebase 保留本地内容并把草稿迁到 Current", () => {
    useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: doc("base") });
    useEditorSession.getState().setAst(doc("mine"));
    useEditorSession.getState().recordConflict({
      currentRevisionId: REV_B,
      serverAst: doc("current"),
    });
    useEditorSession.getState().recordConflict({
      currentRevisionId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4aff",
      serverAst: doc("ignored"),
    });
    expect(useEditorSession.getState().conflict?.currentRevisionId).toBe(REV_B);

    useEditorSession.getState().rebaseToLatest();
    const state = useEditorSession.getState();
    expect(state.conflict).toBeNull();
    expect(state.baseRevisionId).toBe(REV_B);
    expect(state.ast).toEqual(doc("mine"));
    expect(state.dirty).toBe(true);
    expect(state.rebasedFromConflict).toBe(true);
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_A))).toBeNull();
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_B))).not.toBeNull();
  });

  it("resetToServer 丢弃本地内容并加载冲突记录的 Current", () => {
    useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: doc("base") });
    useEditorSession.getState().setAst(doc("mine"));
    useEditorSession.getState().recordConflict({
      currentRevisionId: REV_B,
      serverAst: doc("current"),
    });
    useEditorSession.getState().resetToServer();

    const state = useEditorSession.getState();
    expect(state.conflict).toBeNull();
    expect(state.baseRevisionId).toBe(REV_B);
    expect(state.ast).toEqual(doc("current"));
    expect(state.dirty).toBe(false);
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_A))).toBeNull();
  });

  it("fresh 草稿（base 一致）：恢复草稿内容并标 dirty", () => {
    seedDraft(PAGE_ID, REV_A, doc("draft"));
    const restore = useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: doc("s") });
    const state = useEditorSession.getState();
    expect(restore).toBe("fresh");
    expect(state.ast).toEqual(doc("draft"));
    expect(state.dirty).toBe(true);
  });

  it("stale 草稿（base 落后）：保留内容并把 base 重置到服务端 current", () => {
    seedDraft(PAGE_ID, REV_A, doc("old-draft"));
    const restore = useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_B, ast: doc("s") });
    const state = useEditorSession.getState();
    expect(restore).toBe("stale");
    // 内容保留（绝不因版本落后而清空），base 已重置为服务端 current。
    expect(state.ast).toEqual(doc("old-draft"));
    expect(state.baseRevisionId).toBe(REV_B);
    expect(state.dirty).toBe(true);
    expect(state.conflict).toEqual({
      baseRevisionId: REV_A,
      currentRevisionId: REV_B,
      serverAst: doc("s"),
    });
    // 草稿归一到当前 base 的 key，旧 key 被清掉。
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_A))).toBeNull();
    expect(
      window.localStorage.getItem(draftKey(PAGE_ID, REV_B)),
    ).not.toBeNull();
  });

  it("首发布页面（baseRevisionId=null）的草稿用 none 段 key", () => {
    useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: null, ast: doc("s") });
    useEditorSession.getState().setAst(doc("first"));
    expect(
      window.localStorage.getItem(draftKey(PAGE_ID, null)),
    ).not.toBeNull();
  });

  it("损坏的草稿按无草稿处理，不阻断编辑", () => {
    window.localStorage.setItem(draftKey(PAGE_ID, REV_A), "{not-json");
    const restore = useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: doc("s") });
    expect(restore).toBe("none");
    expect(useEditorSession.getState().dirty).toBe(false);
  });

  it("discardDraft 回退到服务端内容并清除全部草稿", () => {
    seedDraft(PAGE_ID, REV_A, doc("draft"));
    useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: doc("s") });
    useEditorSession.getState().discardDraft();
    const state = useEditorSession.getState();
    expect(state.ast).toEqual(doc("s"));
    expect(state.dirty).toBe(false);
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_A))).toBeNull();
  });

  it("clearDraft 清除本页全部草稿（含其他 base 的残留）", () => {
    seedDraft(PAGE_ID, REV_A, doc("a"));
    seedDraft(PAGE_ID, REV_B, doc("b"));
    useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: doc("s") });
    useEditorSession.getState().clearDraft();
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_A))).toBeNull();
    expect(window.localStorage.getItem(draftKey(PAGE_ID, REV_B))).toBeNull();
  });

  it("不影响其他页面的草稿", () => {
    const OTHER = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a08";
    seedDraft(OTHER, REV_A, doc("other"));
    useEditorSession
      .getState()
      .startSession({ pageId: PAGE_ID, baseRevisionId: REV_A, ast: doc("s") });
    useEditorSession.getState().clearDraft();
    expect(
      window.localStorage.getItem(draftKey(OTHER, REV_A)),
    ).not.toBeNull();
  });
});
