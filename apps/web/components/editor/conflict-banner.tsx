// 乐观锁冲突常驻条（M2-T06）：编辑页顶部常驻（区别于一次性 toast）。
//
// 触发：发布 409 stale_revision（recordConflict），或 stale 草稿恢复（startSession）。
// 展示：「页面已被他人更新：你基于 {base 短id}，当前最新 {current 短id}」+ 三个动作：
// - 查看差异：复用 M2-T05 DiffView 展示 Base（我基于的版本）vs Current（服务端最新），
//   经 historyApi diff?from=base&to=current 拉取；本地未发布修改不混入 diff，仅以文案说明。
// - 以最新版为基继续编辑（rebase）：base:=current，本地编辑内容保留，用户自行对照合并。
// - 放弃我的修改：丢弃本地 AST 与草稿，加载服务端 current（onReset 重挂载编辑器）。
"use client";

import { useState } from "react";
import { toast } from "sonner";
import type { DocumentDiff } from "../../../../contracts/generated/typescript";

import { DiffView } from "@/components/history/diff-view";
import { shortId } from "@/components/history/utils";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { historyApi } from "@/lib/api";
import { useEditorSession } from "@/lib/editor-session";

export interface ConflictBannerProps {
  pageId: string;
  /** 「放弃我的修改」后重挂载 BlockEditor（其 initialAst 只在挂载时读取一次）。 */
  onReset: () => void;
}

export function ConflictBanner({ pageId, onReset }: ConflictBannerProps) {
  const session = useEditorSession();
  const [diffOpen, setDiffOpen] = useState(false);
  const [diff, setDiff] = useState<DocumentDiff | null>(null);
  const [diffLoading, setDiffLoading] = useState(false);
  const [diffFailed, setDiffFailed] = useState(false);

  const conflict = session.conflict;
  if (!conflict) return null;

  const baseLabel = conflict.baseRevisionId
    ? shortId(conflict.baseRevisionId)
    : "无基线";
  const currentLabel = conflict.currentRevisionId
    ? shortId(conflict.currentRevisionId)
    : "无";

  // Base vs Current 结构 Diff；结果在冲突存续期间缓存一次（同一 from/to 幂等）。
  const openDiff = async () => {
    setDiffOpen(true);
    if (!conflict.baseRevisionId || !conflict.currentRevisionId) return;
    if (diff || diffLoading) return;
    setDiffLoading(true);
    setDiffFailed(false);
    try {
      const result = await historyApi().diffRevisions({
        id: pageId,
        from: conflict.baseRevisionId,
        to: conflict.currentRevisionId,
      });
      setDiff(result);
    } catch {
      setDiffFailed(true);
    } finally {
      setDiffLoading(false);
    }
  };

  const onRebase = () => {
    session.rebaseToLatest();
    setDiff(null);
    toast.info("已切换到以最新版本为基", {
      description: "你的编辑内容已保留；请对照差异自行合并后再发布。",
    });
  };

  const onDiscard = () => {
    session.resetToServer();
    setDiff(null);
    onReset();
    toast.info("已放弃本地修改", {
      description: "已加载服务端最新版本，可重新开始编辑。",
    });
  };

  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-amber-500/50 bg-amber-500/10 px-4 py-3"
    >
      <p className="min-w-0 flex-1 text-sm">
        <span className="font-medium">页面已被他人更新：</span>
        你基于{" "}
        <code className="font-mono" title={conflict.baseRevisionId ?? undefined}>
          {baseLabel}
        </code>
        ，当前最新{" "}
        <code
          className="font-mono"
          title={conflict.currentRevisionId ?? undefined}
        >
          {currentLabel}
        </code>
        。你的编辑内容已保留。
      </p>
      <div className="flex shrink-0 items-center gap-2">
        <Button variant="outline" size="sm" onClick={() => void openDiff()}>
          查看差异
        </Button>
        <Button variant="outline" size="sm" onClick={onRebase}>
          以最新版为基继续编辑
        </Button>
        <Button variant="outline" size="sm" onClick={onDiscard}>
          放弃我的修改
        </Button>
      </div>

      <Dialog open={diffOpen} onOpenChange={setDiffOpen}>
        <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              版本差异：{baseLabel} → {currentLabel}
            </DialogTitle>
            <DialogDescription>
              下方为你基于的版本与服务端最新版本的结构差异；
              你的未发布修改不在此对比中，请自行对照合并。
            </DialogDescription>
          </DialogHeader>
          {!conflict.baseRevisionId || !conflict.currentRevisionId ? (
            <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              缺少基线或最新版本，无法对比。
            </p>
          ) : diffLoading ? (
            <p className="p-6 text-center text-sm text-muted-foreground">
              正在加载差异…
            </p>
          ) : diffFailed ? (
            <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              差异加载失败，请关闭后重试。
            </p>
          ) : diff ? (
            <DiffView diff={diff} />
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
