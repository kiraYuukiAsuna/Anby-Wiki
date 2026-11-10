"use client";

import { WorkspaceLink } from "@/components/workspace-link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import {
  ArrowRight,
  CheckCircle2,
  LoaderCircle,
  RotateCcw,
  Send,
  ShieldCheck,
} from "lucide-react";
import { toast } from "sonner";
import { z } from "zod";
import {
  type Proposal,
  type RollbackChangeBatchResult,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { governanceApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { workspaceAccessNotice } from "@/lib/workspace-access";
import { compactId } from "@/lib/display-id";
import { PROPOSAL_STATUS_LABELS } from "@/lib/display-labels";
import {
  isIdentityConflictMessage,
  readGovernanceError,
} from "@/lib/governance-error";

const rollbackConfirmation = z.literal("ROLLBACK");

type PendingAction = "submit" | "apply" | "rollback" | null;

export function ProposalActions({ proposal }: { proposal: Proposal }) {
  const router = useRouter();
  const { session } = useSession();
  const canApply = !workspaceAccessNotice("/governance/apply", session);
  const [pending, setPending] = useState<PendingAction>(null);
  const [rollbackOpen, setRollbackOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const hasAppliedBatch =
    proposal.status === "applied" &&
    proposal.changeBatchStatus === "applied" &&
    Boolean(proposal.changeBatchId);
  const canRollback = canApply && hasAppliedBatch;

  const submit = async () => {
    setPending("submit");
    try {
      const result = await governanceApi().submitProposal({ id: proposal.id });
      toast.success(
        result.reviewTask ? "修改建议已进入待审核列表" : "修改建议已通过策略审核",
      );
      router.refresh();
    } catch (error) {
      await showGovernanceError(error, "提交修改建议失败");
    } finally {
      setPending(null);
    }
  };

  const apply = async () => {
    setPending("apply");
    try {
      const result = await governanceApi().applyProposal({ id: proposal.id });
      toast.success(result.idempotent ? "该修改建议已经生效" : "修改建议已生效", {
        description: `变更批次 ${compactId(result.changeBatchId)} 已写入操作记录${result.revisionIds.length ? `，发布 ${result.revisionIds.length} 个页面版本` : ""}。`,
      });
      router.refresh();
    } catch (error) {
      await showGovernanceError(error, "修改未能生效");
    } finally {
      setPending(null);
    }
  };

  const rollback = async () => {
    const parsed = rollbackConfirmation.safeParse(confirmation);
    if (!parsed.success || !proposal.changeBatchId) {
      toast.error("请输入 ROLLBACK，确认撤销整批修改。");
      return;
    }
    setPending("rollback");
    try {
      const result = await governanceApi().rollbackChangeBatch({
        id: proposal.changeBatchId,
      });
      toast.success(result.idempotent ? "本批修改已撤销，无需重复操作。" : "本批修改已撤销。", {
        description: rollbackSummary(result),
      });
      setRollbackOpen(false);
      setConfirmation("");
      router.refresh();
    } catch (error) {
      await showGovernanceError(error, "整批撤销失败");
    } finally {
      setPending(null);
    }
  };

  return (
    <section className="mb-6 overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div className="flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <div className="mt-0.5 rounded-xl bg-primary/10 p-2 text-primary">
            <ShieldCheck aria-hidden className="size-4" />
          </div>
          <div className="min-w-0">
            <p className="text-sm font-semibold">处理修改建议</p>
            <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
              {actionDescription(proposal)}
            </p>
            {proposal.changeBatchId ? (
              <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground">
                变更批次 {proposal.changeBatchId}
              </p>
            ) : null}
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {proposal.status === "draft" && proposal.createdBy === session?.actorId ? (
            <Button disabled={pending !== null} onClick={() => void submit()}>
              {pending === "submit" ? (
                <LoaderCircle aria-hidden className="animate-spin" />
              ) : (
                <Send aria-hidden />
              )}
              提交审核
            </Button>
          ) : null}

          {proposal.status === "submitted" ||
          proposal.status === "in_review" ? (
            <Button variant="outline" asChild>
              <WorkspaceLink href="/governance/review">
                查看待审核列表
                <ArrowRight aria-hidden />
              </WorkspaceLink>
            </Button>
          ) : null}

          {proposal.status === "approved" ? (
            <>
              <Button variant="outline" asChild>
                <WorkspaceLink href="/governance/apply">打开待生效列表<ArrowRight aria-hidden /></WorkspaceLink>
              </Button>
              {canApply ? (
                <Button disabled={pending !== null} onClick={() => void apply()}>
                  {pending === "apply" ? (
                    <LoaderCircle aria-hidden className="animate-spin" />
                  ) : (
                    <CheckCircle2 aria-hidden />
                  )}
                  使本批修改生效
                </Button>
              ) : null}
            </>
          ) : null}

          {canRollback ? (
            <Button
              variant="destructive"
              disabled={pending !== null}
              onClick={() => setRollbackOpen(true)}
            >
              <RotateCcw aria-hidden />
              撤销整批修改
            </Button>
          ) : null}

          {proposal.status === "rolled_back" ? (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-muted px-3 py-1.5 text-xs font-medium text-muted-foreground">
              <CheckCircle2 aria-hidden className="size-3.5" />
              已撤销修改
            </span>
          ) : null}
        </div>
      </div>

      {proposal.status === "applied" && !hasAppliedBatch ? (
        <div className="border-t border-amber-500/25 bg-amber-500/5 px-4 py-2.5 text-xs text-amber-800 dark:text-amber-300">
          未找到已生效的变更批次，暂时无法撤销。
        </div>
      ) : null}

      <Dialog
        open={rollbackOpen}
        onOpenChange={(open) => {
          if (pending === "rollback") return;
          setRollbackOpen(open);
          if (!open) setConfirmation("");
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>撤销整批修改？</DialogTitle>
            <DialogDescription>
              撤销会恢复本批修改前的页面状态、事实、实体合并、来源关系和合集成员。若其中任何内容已有后续修改，整批撤销将停止，避免覆盖新内容。
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-xl border border-destructive/20 bg-destructive/5 p-3 text-xs leading-5 text-muted-foreground">
            这会撤销整批已生效的修改，历史版本和操作记录仍会保留。输入
            <span className="mx-1 font-mono font-semibold text-foreground">
              ROLLBACK
            </span>
            继续。
          </div>
          <div className="grid gap-2">
            <Label htmlFor="rollback-confirmation">确认文本</Label>
            <Input
              id="rollback-confirmation"
              autoComplete="off"
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              placeholder="ROLLBACK"
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={pending === "rollback"}
              onClick={() => setRollbackOpen(false)}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={
                pending === "rollback" ||
                !rollbackConfirmation.safeParse(confirmation).success
              }
              onClick={() => void rollback()}
            >
              {pending === "rollback" ? (
                <LoaderCircle aria-hidden className="animate-spin" />
              ) : (
                <RotateCcw aria-hidden />
              )}
              确认整批撤销
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

function actionDescription(proposal: Proposal): string {
  switch (proposal.status) {
    case "draft":
      return "修改内容已保存为草稿，提交后将进行风险检查和人工审核。";
    case "submitted":
    case "in_review":
      return "修改建议正在等待审核，现有内容尚未改变。";
    case "approved":
      return "审核已通过，可以让本批修改一并生效。任一项失败，整批都不会生效。";
    case "applied":
      return "修改已生效，可以查看变更记录或撤销本批修改。";
    case "rolled_back":
      return "本批修改已撤销，历史版本和操作记录仍然保留。";
    case "conflicted":
      return "相关内容已有新版本，请先解决冲突。";
    case "rejected":
      return "修改建议已拒绝，现有内容不会改变。";
    default:
      return `当前状态：${PROPOSAL_STATUS_LABELS[proposal.status]}`;
  }
}

function rollbackSummary(result: RollbackChangeBatchResult): string {
  const facts = [
    result.revisionIds.length
      ? `${result.revisionIds.length} 个恢复版本`
      : "",
    result.compensationClaimIds.length
      ? `${result.compensationClaimIds.length} 条事实恢复记录`
      : "",
    result.deletedPageIds.length
      ? `${result.deletedPageIds.length} 个新建页面已移除，历史记录保留`
      : "",
    result.deletedEntityIds.length
      ? `${result.deletedEntityIds.length} 个新建实体已移除，历史记录保留`
      : "",
    result.entityMergeIds.length
      ? `${result.entityMergeIds.length} 个实体合并已恢复`
      : "",
    result.collectionIds.length
      ? `${result.collectionIds.length} 个合集已恢复`
      : "",
    result.removedClaimSourceCount
      ? `${result.removedClaimSourceCount} 个来源关系已移除`
      : "",
  ].filter(Boolean);
  return facts.join("；") || "本批没有需要撤销的修改。";
}

async function showGovernanceError(error: unknown, fallback: string) {
  const detail = await readGovernanceError(error);
  if (detail) {
    if (detail.status === 401) {
      toast.error("请先登录，再处理修改建议。");
      return;
    }
    if (detail.status === 403) {
      toast.error("当前账号没有执行该操作的权限");
      return;
    }
    if (detail.status === 409) {
      const identityConflict = isIdentityConflictMessage(detail.message);
      toast.error(identityConflict ? "修改建议已过期，未执行任何写入" : fallback, {
        description:
          detail.message ??
          "相关内容已变化，请刷新后查看最新操作记录。",
      });
      return;
    }
    toast.error(`${fallback}（HTTP ${detail.status}）`, {
      description: detail.message ?? undefined,
    });
    return;
  }
  toast.error(fallback, { description: "网络连接异常，请稍后重试。" });
}
