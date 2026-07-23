/**
 * 编辑会话状态（M2-T03）。
 *
 * Zustand 只存未提交的编辑会话（当前 AST、dirty、摘要），服务端数据
 * （页面当前内容）经服务端组件注入为会话初始值，不在此双份缓存。
 *
 * 本地草稿（localStorage）只是崩溃/误关恢复辅助，绝不自动发布：
 * - key 含 pageId + baseRevisionId：`anbywiki.editor-draft.<pageId>.<baseRevisionId|"none">`；
 * - 进入编辑页时若草稿 base 落后服务端 current，保留草稿内容并把 base
 *   重置到服务端 current，同时记录 conflict 状态（M2-T06），
 *   由编辑页冲突条提供「查看差异 / 以最新版为基继续编辑 / 放弃我的修改」。
 *
 * 冲突状态机（M2-T06 乐观锁冲突体验）：
 * - 记录：发布 409 stale_revision（视图层拉取服务端最新后 recordConflict），
 *   或 startSession 恢复落后草稿（base 已重置为服务端 current，conflict.base
 *   记草稿原 base）。recordConflict 幂等：已有冲突时不覆盖（不重复拉取）。
 * - 解除：rebaseToLatest（base:=current，本地内容与 dirty 保留）或
 *   resetToServer（丢弃本地内容，加载服务端 current）。
 * - 发布：视图层在 conflict 未解除时阻止打开发布框；rebase 后发布带
 *   expected=current，杜绝 stale 草稿未经确认直接覆盖 Current。
 */
"use client";

import { create } from "zustand";

import { parseDocument, type Document } from "@/lib/ast/schema";

const DRAFT_KEY_PREFIX = "anbywiki.editor-draft.";
const NO_REVISION = "none";

export function draftKey(pageId: string, baseRevisionId: string | null): string {
  return `${DRAFT_KEY_PREFIX}${pageId}.${baseRevisionId ?? NO_REVISION}`;
}

interface DraftPayload {
  ast: unknown;
  savedAt: string;
}

/** 草稿恢复结果：无草稿 / 基于当前版本 / 基于旧版本（已 rebase 到当前）。 */
export type DraftRestore = "none" | "fresh" | "stale";

function listDraftKeys(pageId: string): string[] {
  const prefix = `${DRAFT_KEY_PREFIX}${pageId}.`;
  const keys: string[] = [];
  for (let i = 0; i < window.localStorage.length; i += 1) {
    const key = window.localStorage.key(i);
    if (key?.startsWith(prefix)) keys.push(key);
  }
  return keys;
}

function readDraftAst(key: string): Document | null {
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return null;
    const payload = JSON.parse(raw) as DraftPayload;
    return parseDocument(payload.ast);
  } catch {
    // 损坏的草稿直接忽略（不阻断正常编辑）。
    return null;
  }
}

function removeOtherDrafts(pageId: string, keepKey: string | null): void {
  for (const key of listDraftKeys(pageId)) {
    if (key !== keepKey) window.localStorage.removeItem(key);
  }
}

/** 乐观锁冲突状态（M2-T06）：发布 409 或恢复落后草稿时记录。 */
export interface EditorConflict {
  /** 我基于的版本（发布被拒时的 expected_revision_id；草稿场景为草稿的原 base）。 */
  baseRevisionId: string | null;
  /** 服务端当前最新版本。 */
  currentRevisionId: string | null;
  /** 服务端最新内容（「放弃我的修改」回退目标与差异参考）。 */
  serverAst: Document;
}

export interface EditorSessionState {
  /** 会话页面 ID；null 表示会话未开始。 */
  pageId: string | null;
  /** 发布基线（乐观锁 expected_revision_id）；首发布为 null。 */
  baseRevisionId: string | null;
  /** 进入会话时的服务端内容（用于丢弃草稿时回退）。 */
  serverAst: Document | null;
  /** 当前编辑内容。 */
  ast: Document | null;
  /** 相对进入会话时的内容是否有改动（含恢复了草稿的情况）。 */
  dirty: boolean;
  /** 修改摘要（发布对话框）。 */
  summary: string;
  /** 小修改标志（发布对话框）。 */
  isMinor: boolean;
  /** 发布请求进行中（防重复提交）。 */
  publishing: boolean;
  /** 乐观锁冲突状态；null 表示无冲突。 */
  conflict: EditorConflict | null;
  /** rebaseToLatest 后待提示：发布成功时 toast「已基于最新版本发布」。 */
  rebasedFromConflict: boolean;

  /**
   * 开启/重置编辑会话。服务端内容作为初始值注入；
   * 若存在本地草稿则恢复草稿内容并标记 dirty；
   * 草稿 base 落后时记录 conflict（base=草稿原 base，current=服务端 current）。
   */
  startSession: (input: {
    pageId: string;
    baseRevisionId: string | null;
    ast: Document;
  }) => DraftRestore;
  /** 编辑器 onChange 入口：更新 AST、标 dirty 并持久化草稿。 */
  setAst: (ast: Document) => void;
  setSummary: (summary: string) => void;
  setIsMinor: (isMinor: boolean) => void;
  setPublishing: (publishing: boolean) => void;
  /**
   * 记录乐观锁冲突（发布 409 后由视图层拉取服务端最新内容后调用）。
   * 幂等：已有冲突记录时不覆盖（重复 409 不改变首次记录的 base/current）。
   */
  recordConflict: (input: {
    currentRevisionId: string | null;
    serverAst: Document;
  }) => void;
  /**
   * 「以最新版为基继续编辑」：base 更新为 current，本地 AST 与 dirty 保留，
   * 清除冲突并标记 rebasedFromConflict（用户自行参考差异合并）。
   */
  rebaseToLatest: () => void;
  /** 「放弃我的修改」：丢弃本地 AST 与草稿，加载服务端 current 重新开始。 */
  resetToServer: () => void;
  /** 发布成功提示已消费：清除 rebasedFromConflict 标记。 */
  clearRebasedFlag: () => void;
  /** 丢弃本地草稿，回退到服务端内容。 */
  discardDraft: () => void;
  /** 发布成功后调用：清除本页全部草稿。 */
  clearDraft: () => void;
}

export const useEditorSession = create<EditorSessionState>((set, get) => ({
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

  startSession: ({ pageId, baseRevisionId, ast }) => {
    const currentKey = draftKey(pageId, baseRevisionId);

    // 优先精确匹配当前 base 的草稿；否则取任一旧 base 草稿（视为落后）。
    let restore: DraftRestore = "none";
    let draftAst: Document | null = null;
    // stale 草稿的原 base（key 后缀），用于记录 conflict；undefined 表示未恢复草稿。
    let staleDraftBase: string | null | undefined;
    const keys = listDraftKeys(pageId);
    if (keys.includes(currentKey)) {
      draftAst = readDraftAst(currentKey);
      if (draftAst) restore = "fresh";
    } else if (keys.length > 0) {
      for (const key of keys) {
        draftAst = readDraftAst(key);
        if (draftAst) {
          restore = "stale";
          const suffix = key.slice(`${DRAFT_KEY_PREFIX}${pageId}.`.length);
          staleDraftBase = suffix === NO_REVISION ? null : suffix;
          break;
        }
      }
    }

    // 同一页面只保留一份草稿：恢复后把草稿归一到当前 base 的 key。
    removeOtherDrafts(pageId, restore === "none" ? null : currentKey);
    if (draftAst) {
      window.localStorage.setItem(
        currentKey,
        JSON.stringify({ ast: draftAst, savedAt: new Date().toISOString() }),
      );
    }

    set({
      pageId,
      baseRevisionId,
      serverAst: ast,
      ast: draftAst ?? ast,
      dirty: draftAst !== null,
      summary: "",
      isMinor: false,
      publishing: false,
      // stale 草稿升级为冲突流（M2-T06）：记录草稿原 base 与服务端 current。
      conflict:
        restore === "stale"
          ? {
              baseRevisionId: staleDraftBase ?? null,
              currentRevisionId: baseRevisionId,
              serverAst: ast,
            }
          : null,
      rebasedFromConflict: false,
    });
    return restore;
  },

  setAst: (ast) => {
    const { pageId, baseRevisionId } = get();
    set({ ast, dirty: true });
    if (pageId) {
      window.localStorage.setItem(
        draftKey(pageId, baseRevisionId),
        JSON.stringify({ ast, savedAt: new Date().toISOString() }),
      );
    }
  },

  setSummary: (summary) => set({ summary }),
  setIsMinor: (isMinor) => set({ isMinor }),
  setPublishing: (publishing) => set({ publishing }),

  recordConflict: ({ currentRevisionId, serverAst }) => {
    // 幂等：已有冲突记录时保持首次记录（重复 409 不重复拉取/覆盖）。
    if (get().conflict) return;
    set({
      conflict: {
        baseRevisionId: get().baseRevisionId,
        currentRevisionId,
        serverAst,
      },
    });
  },

  rebaseToLatest: () => {
    const { pageId, ast, conflict } = get();
    if (!conflict) return;
    // base 切到服务端 current；本地内容与 dirty 保留（不丢工作）。
    // serverAst 同步更新为最新，使后续 discardDraft 回退到最新版。
    set({
      baseRevisionId: conflict.currentRevisionId,
      serverAst: conflict.serverAst,
      conflict: null,
      rebasedFromConflict: true,
    });
    // 草稿归一到新 base 的 key（继续编辑时 setAst 也会写新 key）。
    if (pageId) {
      removeOtherDrafts(pageId, null);
      if (ast) {
        window.localStorage.setItem(
          draftKey(pageId, conflict.currentRevisionId),
          JSON.stringify({ ast, savedAt: new Date().toISOString() }),
        );
      }
    }
  },

  resetToServer: () => {
    const { pageId, conflict } = get();
    if (!conflict) return;
    if (pageId) removeOtherDrafts(pageId, null);
    set({
      baseRevisionId: conflict.currentRevisionId,
      serverAst: conflict.serverAst,
      ast: conflict.serverAst,
      dirty: false,
      conflict: null,
      rebasedFromConflict: false,
    });
  },

  clearRebasedFlag: () => set({ rebasedFromConflict: false }),

  discardDraft: () => {
    const { pageId, serverAst } = get();
    if (pageId) removeOtherDrafts(pageId, null);
    set({ ast: serverAst, dirty: false, conflict: null });
  },

  clearDraft: () => {
    const { pageId } = get();
    if (pageId) removeOtherDrafts(pageId, null);
  },
}));
