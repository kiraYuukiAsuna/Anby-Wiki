"use client";

import Link from "next/link";
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
  ResponseError,
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
import { compactId } from "@/lib/display-id";

const rollbackConfirmation = z.literal("ROLLBACK");

type PendingAction = "submit" | "apply" | "rollback" | null;

export function ProposalActions({ proposal }: { proposal: Proposal }) {
  const router = useRouter();
  const [pending, setPending] = useState<PendingAction>(null);
  const [rollbackOpen, setRollbackOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const canRollback =
    proposal.status === "applied" &&
    proposal.changeBatchStatus === "applied" &&
    Boolean(proposal.changeBatchId);

  const submit = async () => {
    setPending("submit");
    try {
      const result = await governanceApi().submitProposal({ id: proposal.id });
      toast.success(
        result.reviewTask ? "提案已进入人工审核队列" : "提案已通过策略审核",
      );
      router.refresh();
    } catch (error) {
      showGovernanceError(error, "提交提案失败");
    } finally {
      setPending(null);
    }
  };

  const apply = async () => {
    setPending("apply");
    try {
      const result = await governanceApi().applyProposal({ id: proposal.id });
      toast.success(result.idempotent ? "该提案已经生效" : "提案已原子生效", {
        description: `ChangeBatch ${compactId(result.changeBatchId)} 已写入审计账本${result.revisionIds.length ? `，发布 ${result.revisionIds.length} 个页面 Revision` : ""}。`,
      });
      router.refresh();
    } catch (error) {
      showGovernanceError(error, "应用提案失败");
    } finally {
      setPending(null);
    }
  };

  const rollback = async () => {
    const parsed = rollbackConfirmation.safeParse(confirmation);
    if (!parsed.success || !proposal.changeBatchId) {
      toast.error("请输入 ROLLBACK 以确认整批补偿");
      return;
    }
    setPending("rollback");
    try {
      const result = await governanceApi().rollbackChangeBatch({
        id: proposal.changeBatchId,
      });
      toast.success(result.idempotent ? "该批次已回滚" : "ChangeBatch 已完整补偿", {
        description: rollbackSummary(result),
      });
      setRollbackOpen(false);
      setConfirmation("");
      router.refresh();
    } catch (error) {
      showGovernanceError(error, "整批回滚失败");
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
            <p className="text-sm font-semibold">治理操作台</p>
            <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
              {actionDescription(proposal)}
            </p>
            {proposal.changeBatchId ? (
              <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground">
                ChangeBatch {proposal.changeBatchId}
              </p>
            ) : null}
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {proposal.status === "draft" ? (
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
              <Link href="/governance/review">
                查看审核队列
                <ArrowRight aria-hidden />
              </Link>
            </Button>
          ) : null}

          {proposal.status === "approved" ? (
            <>
              <Button variant="outline" asChild>
                <Link href="/governance/apply">打开待应用队列<ArrowRight aria-hidden /></Link>
              </Button>
              <Button disabled={pending !== null} onClick={() => void apply()}>
                {pending === "apply" ? (
                  <LoaderCircle aria-hidden className="animate-spin" />
                ) : (
                  <CheckCircle2 aria-hidden />
                )}
                原子应用
              </Button>
            </>
          ) : null}

          {canRollback ? (
            <Button
              variant="destructive"
              disabled={pending !== null}
              onClick={() => setRollbackOpen(true)}
            >
              <RotateCcw aria-hidden />
              回滚整批变更
            </Button>
          ) : null}

          {proposal.status === "rolled_back" ? (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-muted px-3 py-1.5 text-xs font-medium text-muted-foreground">
              <CheckCircle2 aria-hidden className="size-3.5" />
              已完成补偿
            </span>
          ) : null}
        </div>
      </div>

      {proposal.status === "applied" && !canRollback ? (
        <div className="border-t border-amber-500/25 bg-amber-500/5 px-4 py-2.5 text-xs text-amber-800 dark:text-amber-300">
          当前响应没有可用的 applied ChangeBatch，已禁用回滚入口以避免错误操作。
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
            <DialogTitle>回滚整个 ChangeBatch？</DialogTitle>
            <DialogDescription>
              系统会在一个事务中追加补偿 Revision/Claim，并逆向恢复页面生命周期、
              实体合并、来源关系和 Collection 成员。任何对象已有后续修改时，整批操作都会拒绝。
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-xl border border-destructive/20 bg-destructive/5 p-3 text-xs leading-5 text-muted-foreground">
            这是高风险治理操作，不会删除 Revision、Snapshot 或审计历史。输入
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
              确认整批回滚
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
      return "Operation 已冻结在草稿中；提交后进入风险策略与人工审核。";
    case "submitted":
    case "in_review":
      return "提案正在审核队列中，权威状态尚未发生改变。";
    case "approved":
      return "审核证据已满足；全部有序 Operation 将写入同一个 ChangeBatch，要么全部生效，要么全部回滚。";
    case "applied":
      return "权威变更已生效，可从这里持久访问其 ChangeBatch 与补偿入口。";
    case "rolled_back":
      return "权威状态已通过追加式补偿恢复，原始历史与审计账本仍保留。";
    case "conflicted":
      return "Current 已偏离基线，请先完成冲突决议。";
    case "rejected":
      return "提案已被拒绝，不会进入权威写入路径。";
    default:
      return `当前状态：${proposal.status}`;
  }
}

function rollbackSummary(result: RollbackChangeBatchResult): string {
  const facts = [
    result.revisionIds.length
      ? `${result.revisionIds.length} 个补偿 Revision`
      : "",
    result.compensationClaimIds.length
      ? `${result.compensationClaimIds.length} 个 Claim 补偿`
      : "",
    result.deletedPageIds.length
      ? `${result.deletedPageIds.length} 个新建页面已软删除`
      : "",
    result.deletedEntityIds.length
      ? `${result.deletedEntityIds.length} 个新建实体已软删除`
      : "",
    result.entityMergeIds.length
      ? `${result.entityMergeIds.length} 个实体合并已恢复`
      : "",
    result.collectionIds.length
      ? `${result.collectionIds.length} 个 Collection 已恢复`
      : "",
    result.removedClaimSourceCount
      ? `${result.removedClaimSourceCount} 个来源关系已移除`
      : "",
  ].filter(Boolean);
  return facts.join("；") || "该批次没有需要追加的补偿对象。";
}

function showGovernanceError(error: unknown, fallback: string) {
  if (error instanceof ResponseError) {
    if (error.response.status === 401) {
      toast.error("请先登录再执行治理操作");
      return;
    }
    if (error.response.status === 403) {
      toast.error("当前账号没有执行该操作的权限");
      return;
    }
    if (error.response.status === 409) {
      toast.error(fallback, {
        description: "对象状态或基线已经变化，请刷新后检查最新审计记录。",
      });
      return;
    }
    toast.error(`${fallback}（HTTP ${error.response.status}）`);
    return;
  }
  toast.error(fallback, { description: "网络连接异常，请稍后重试。" });
}
