"use client";

import Link from "next/link";
import {
  Activity,
  Check,
  ChevronRight,
  CircleAlert,
  CirclePause,
  CirclePlay,
  Clock3,
  LoaderCircle,
  Play,
  RotateCcw,
  ShieldCheck,
  X,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";
import type {
  BulkReviewBatch,
  BulkReviewItem,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { governanceApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";
import { cn } from "@/lib/utils";

const decisionSchema = z.string().trim().min(1, "请记录审核依据").max(1000);

const statusMeta = {
  reviewing: { label: "抽样审核中", tone: "bg-amber-500/10 text-amber-700" },
  ready: { label: "等待下一 wave", tone: "bg-blue-500/10 text-blue-700" },
  applying: { label: "应用中", tone: "bg-violet-500/10 text-violet-700" },
  paused: { label: "已暂停", tone: "bg-rose-500/10 text-rose-700" },
  completed: { label: "全部完成", tone: "bg-emerald-500/10 text-emerald-700" },
} as const;

const decisionMeta = {
  pending: { label: "待决策", tone: "text-amber-700" },
  approved: { label: "已批准", tone: "text-emerald-700" },
  rejected: { label: "已拒绝", tone: "text-rose-700" },
} as const;

const applyMeta = {
  pending: "等待应用",
  applied: "已应用",
  failed: "应用失败",
  skipped: "已跳过",
} as const;

function BatchOverview({ batch }: { batch: BulkReviewBatch }) {
  const reviewed = batch.items.filter((item) => item.decision !== "pending").length;
  const applied = batch.items.filter((item) => item.applyStatus === "applied").length;
  const progress =
    batch.status === "reviewing"
      ? Math.round((reviewed / batch.items.length) * 100)
      : Math.round((applied / Math.max(1, batch.items.filter((item) => item.decision === "approved").length)) * 100);

  return (
    <div className="rounded-2xl border bg-card p-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs font-medium text-muted-foreground">
            {batch.status === "reviewing" ? "审核进度" : "应用进度"}
          </p>
          <p className="mt-2 text-3xl font-semibold tracking-[-0.04em]">{progress}%</p>
        </div>
        <span className={cn("rounded-full px-2.5 py-1 text-xs font-medium", statusMeta[batch.status].tone)}>
          {statusMeta[batch.status].label}
        </span>
      </div>
      <div className="mt-4 h-2 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full bg-primary" style={{ width: `${progress}%` }} />
      </div>
      <dl className="mt-5 grid grid-cols-2 gap-4 text-xs">
        <div>
          <dt className="text-muted-foreground">审核模式</dt>
          <dd className="mt-1 font-medium">
            {batch.samplingMode === "full" ? "全量审核" : `${batch.samplePercent}% 抽样`}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">固定 wave</dt>
          <dd className="mt-1 font-medium">{batch.waveSize} 个提案 / wave</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">当前 wave</dt>
          <dd className="mt-1 font-medium">{batch.currentWave || "尚未开始"}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">创建时间</dt>
          <dd className="mt-1 font-medium">
            {new Intl.DateTimeFormat("zh-CN", {
              month: "short",
              day: "numeric",
              hour: "2-digit",
              minute: "2-digit",
            }).format(batch.createdAt)}
          </dd>
        </div>
      </dl>
    </div>
  );
}

function ProposalRow({
  item,
  reason,
  busy,
  onReasonChange,
  onDecision,
}: {
  item: BulkReviewItem;
  reason: string;
  busy: boolean;
  onReasonChange: (value: string) => void;
  onDecision: (approve: boolean) => void;
}) {
  const needsDecision = item.selectedForReview && item.decision === "pending";

  return (
    <li className="rounded-2xl border border-border/75 bg-card p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-muted font-mono text-xs">
            {item.position}
          </span>
          <div className="min-w-0">
            <Link
              href={`/governance/proposals/${item.proposalId}`}
              className="group inline-flex max-w-full items-center gap-1 font-mono text-sm font-semibold hover:text-primary"
            >
              <span className="truncate">{item.proposalId}</span>
              <ChevronRight className="size-3.5 shrink-0 transition-transform group-hover:translate-x-0.5" aria-hidden />
            </Link>
            <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>wave {item.wave}</span>
              <span>{item.selectedForReview ? "抽中审核" : "抽样通过后自动批准"}</span>
              <span className={decisionMeta[item.decision].tone}>{decisionMeta[item.decision].label}</span>
              <span>{applyMeta[item.applyStatus]}</span>
            </div>
          </div>
        </div>
        {item.applyStatus === "applied" ? (
          <Check className="size-4 text-emerald-600" aria-label="已应用" />
        ) : item.applyStatus === "failed" ? (
          <CircleAlert className="size-4 text-destructive" aria-label="应用失败" />
        ) : null}
      </div>

      {needsDecision ? (
        <div className="mt-4 rounded-xl border bg-muted/20 p-3">
          <label className="text-xs font-medium" htmlFor={`bulk-reason-${item.proposalId}`}>
            审核依据
          </label>
          <Textarea
            id={`bulk-reason-${item.proposalId}`}
            value={reason}
            onChange={(event) => onReasonChange(event.target.value)}
            placeholder="说明证据质量、变更风险与判断依据"
            className="mt-2 min-h-20 bg-background"
            maxLength={1000}
          />
          <div className="mt-3 flex justify-end gap-2">
            <Button variant="destructive" disabled={busy} onClick={() => onDecision(false)}>
              <X aria-hidden />
              拒绝
            </Button>
            <Button disabled={busy} onClick={() => onDecision(true)}>
              <Check aria-hidden />
              批准
            </Button>
          </div>
        </div>
      ) : item.decisionReason ? (
        <p className="mt-3 rounded-lg bg-muted/45 px-3 py-2 text-xs leading-5 text-muted-foreground">
          {item.decisionReason}
        </p>
      ) : null}
    </li>
  );
}

export function BulkReviewWorkspace({ id }: { id: string }) {
  const router = useRouter();
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const [busyAction, setBusyAction] = useState<string>();
  const {
    data: batch,
    error,
    isLoading,
    mutate: mutateBatch,
  } = useSWR(["governance:bulk-review", id], () =>
    governanceApi().getBulkReviewBatch({ id }),
  );
  const { data: audit, mutate: mutateAudit } = useSWR(
    ["governance:bulk-review-audit", id],
    () => governanceApi().listBulkReviewAuditEvents({ id }),
  );

  const pendingSampleCount = useMemo(
    () =>
      batch?.items.filter(
        (item) => item.selectedForReview && item.decision === "pending",
      ).length ?? 0,
    [batch],
  );

  const handleError = (actionError: unknown, fallback: string) => {
    if (isUnauthorized(actionError)) {
      toast.error("请先登录后继续处理");
      router.push(LOGIN_PATH);
    } else {
      toast.error(fallback, {
        description: "批次可能已被其他审核者更新，请刷新状态后重试。",
      });
    }
  };

  const refresh = async () => {
    await Promise.all([mutateBatch(), mutateAudit()]);
  };

  const decide = async (item: BulkReviewItem, approve: boolean) => {
    const parsed = decisionSchema.safeParse(reasons[item.proposalId] ?? "");
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请填写审核依据");
      return;
    }
    setBusyAction(item.proposalId);
    try {
      await governanceApi().decideBulkReviewProposal({
        id,
        proposalId: item.proposalId,
        bulkReviewDecisionRequest: {
          approve,
          reason: parsed.data,
        },
      });
      await refresh();
      toast.success(approve ? "提案已批准" : "提案已拒绝");
    } catch (actionError) {
      handleError(actionError, "决策提交失败");
    } finally {
      setBusyAction(undefined);
    }
  };

  const runAction = async (
    key: string,
    action: () => Promise<unknown>,
    success: string,
  ) => {
    setBusyAction(key);
    try {
      await action();
      await refresh();
      toast.success(success);
    } catch (actionError) {
      handleError(actionError, "操作未完成");
    } finally {
      setBusyAction(undefined);
    }
  };

  if (error) {
    return (
      <div className="rounded-2xl border border-destructive/25 bg-destructive/5 p-8 text-center">
        <CircleAlert className="mx-auto size-6 text-destructive" aria-hidden />
        <h1 className="mt-3 font-semibold">无法读取这个评审批次</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          它可能不存在、属于其他 Wiki，或当前账号没有审核权限。
        </p>
      </div>
    );
  }
  if (isLoading || !batch) {
    return (
      <div className="flex items-center gap-2 py-16 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在恢复批次状态…
      </div>
    );
  }

  return (
    <>
      <header className="grid gap-6 border-b border-border/75 pb-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
        <div>
          <p className="text-xs font-semibold tracking-[0.18em] text-primary uppercase">
            Frozen review batch
          </p>
          <h1 className="mt-3 text-3xl font-semibold tracking-[-0.04em]">
            批次 {batch.id.slice(0, 8)}
          </h1>
          <p className="mt-3 font-mono text-xs text-muted-foreground">{batch.id}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {batch.status === "reviewing" ? (
            <Button
              disabled={Boolean(busyAction) || pendingSampleCount > 0}
              onClick={() =>
                void runAction(
                  "finalize",
                  () => governanceApi().finalizeBulkReviewBatch({ id }),
                  "抽样审核已冻结，批次可以应用",
                )
              }
            >
              <ShieldCheck aria-hidden />
              完成审核
            </Button>
          ) : null}
          {batch.status === "ready" ? (
            <>
              <Button
                variant="outline"
                disabled={Boolean(busyAction)}
                onClick={() =>
                  void runAction(
                    "pause",
                    () => governanceApi().pauseBulkReviewBatch({ id }),
                    "批次已暂停",
                  )
                }
              >
                <CirclePause aria-hidden />
                暂停
              </Button>
              <Button
                disabled={Boolean(busyAction)}
                onClick={() =>
                  void runAction(
                    "apply",
                    () => governanceApi().applyNextBulkReviewWave({ id }),
                    "下一 wave 已处理",
                  )
                }
              >
                <Play aria-hidden />
                应用下一 wave
              </Button>
            </>
          ) : null}
          {batch.status === "paused" ? (
            <Button
              disabled={Boolean(busyAction)}
              onClick={() =>
                void runAction(
                  "resume",
                  () => governanceApi().resumeBulkReviewBatch({ id }),
                  "批次已恢复",
                )
              }
            >
              <CirclePlay aria-hidden />
              恢复批次
            </Button>
          ) : null}
          {busyAction && !batch.items.some((item) => item.proposalId === busyAction) ? (
            <span className="flex items-center gap-2 text-xs text-muted-foreground">
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
              正在提交…
            </span>
          ) : null}
        </div>
      </header>

      {batch.status === "reviewing" && pendingSampleCount > 0 ? (
        <div className="mt-6 flex items-start gap-3 rounded-2xl border border-amber-500/20 bg-amber-500/5 p-4 text-sm">
          <Clock3 className="mt-0.5 size-4 shrink-0 text-amber-700" aria-hidden />
          <p>
            还需对 <strong>{pendingSampleCount}</strong>{" "}
            个抽中提案作出明确决策，之后才能冻结审核结果。
          </p>
        </div>
      ) : null}

      <div className="grid gap-8 py-8 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <section>
          <div className="flex items-end justify-between gap-4">
            <div>
              <h2 className="text-lg font-semibold">冻结的提案集合</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                共 {batch.items.length} 项；顺序与 wave 在批次创建后保持不变。
              </p>
            </div>
          </div>
          <ol className="mt-5 space-y-3">
            {batch.items.map((item) => (
              <ProposalRow
                key={item.proposalId}
                item={item}
                reason={reasons[item.proposalId] ?? ""}
                busy={Boolean(busyAction)}
                onReasonChange={(value) =>
                  setReasons((current) => ({
                    ...current,
                    [item.proposalId]: value,
                  }))
                }
                onDecision={(approve) => void decide(item, approve)}
              />
            ))}
          </ol>
        </section>

        <aside className="space-y-4 xl:sticky xl:top-24 xl:self-start">
          <BatchOverview batch={batch} />
          <div className="rounded-2xl border bg-card p-5">
            <div className="flex items-center gap-2">
              <Activity className="size-4 text-primary" aria-hidden />
              <h2 className="text-sm font-semibold">批次审计</h2>
            </div>
            <ol className="mt-4 space-y-4 border-l border-border pl-4">
              {audit?.items.length ? (
                [...audit.items].reverse().map((event) => (
                  <li key={event.id} className="relative text-xs">
                    <span className="absolute top-1 -left-[1.18rem] size-2 rounded-full border-2 border-background bg-primary" />
                    <p className="font-medium">{event.eventType.replaceAll("_", " ")}</p>
                    <p className="mt-1 text-muted-foreground">
                      {new Intl.DateTimeFormat("zh-CN", {
                        month: "short",
                        day: "numeric",
                        hour: "2-digit",
                        minute: "2-digit",
                      }).format(event.createdAt)}
                      {event.wave ? ` · wave ${event.wave}` : ""}
                    </p>
                  </li>
                ))
              ) : (
                <li className="text-xs text-muted-foreground">正在读取审计记录…</li>
              )}
            </ol>
            <Button variant="ghost" size="sm" className="mt-4 w-full" onClick={() => void refresh()}>
              <RotateCcw aria-hidden />
              刷新状态
            </Button>
          </div>
        </aside>
      </div>
    </>
  );
}
