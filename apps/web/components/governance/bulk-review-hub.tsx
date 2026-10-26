"use client";

import Link from "next/link";
import {
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  LoaderCircle,
} from "lucide-react";
import { useState } from "react";
import useSWR from "swr";
import {
  ListBulkReviewBatchesStatusEnum,
  type BulkReviewBatchSummary,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { governanceApi } from "@/lib/api";
import { compactId } from "@/lib/display-id";
import { cn } from "@/lib/utils";

const statusMeta = {
  reviewing: { label: "抽样审核", tone: "bg-amber-500/10 text-amber-700" },
  ready: { label: "等待应用", tone: "bg-blue-500/10 text-blue-700" },
  applying: { label: "正在应用", tone: "bg-violet-500/10 text-violet-700" },
  paused: { label: "已暂停", tone: "bg-rose-500/10 text-rose-700" },
  completed: { label: "已完成", tone: "bg-emerald-500/10 text-emerald-700" },
} as const;

type BatchStatus = keyof typeof statusMeta;
type StatusFilter = "all" | BatchStatus;

function BatchCard({ batch }: { batch: BulkReviewBatchSummary }) {
  const meta = statusMeta[batch.status];
  const finished = batch.appliedCount + batch.rejectedCount;
  const progress = Math.min(100, Math.round((finished / Math.max(1, batch.itemCount)) * 100));

  return (
    <Link
      href={`/governance/bulk/${batch.id}`}
      className="group block rounded-2xl border border-border/75 bg-card p-5 transition hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-[0_18px_45px_-28px_color-mix(in_oklch,var(--primary),transparent_50%)]"
    >
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <span className={cn("rounded-full px-2.5 py-1 text-xs font-medium", meta.tone)}>
              {meta.label}
            </span>
            <span className="text-xs text-muted-foreground">
              {batch.samplingMode === "full"
                ? "全量审核"
                : `${batch.samplePercent}% 风险抽样`}
            </span>
          </div>
          <p className="mt-3 font-mono text-sm font-semibold">
            {compactId(batch.id)}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            {new Intl.DateTimeFormat("zh-CN", {
              dateStyle: "medium",
              timeStyle: "short",
            }).format(batch.createdAt)}
            · {batch.itemCount} 个提案 · wave {batch.waveSize}
          </p>
        </div>
        <ArrowRight
          className="size-4 text-muted-foreground transition-transform group-hover:translate-x-1 group-hover:text-primary"
          aria-hidden
        />
      </div>
      <div className="mt-5 h-1.5 overflow-hidden rounded-full bg-muted">
        <div
          className="h-full rounded-full bg-primary transition-[width]"
          style={{ width: `${progress}%` }}
        />
      </div>
      <div className="mt-3 grid grid-cols-4 gap-2 text-xs">
        <span>
          <strong className="block text-sm text-foreground">{batch.pendingDecisions}</strong>
          <span className="text-muted-foreground">待决策</span>
        </span>
        <span>
          <strong className="block text-sm text-foreground">{batch.approvedCount}</strong>
          <span className="text-muted-foreground">已批准</span>
        </span>
        <span>
          <strong className="block text-sm text-foreground">{batch.appliedCount}</strong>
          <span className="text-muted-foreground">已应用</span>
        </span>
        <span>
          <strong className={cn("block text-sm", batch.failedCount ? "text-destructive" : "text-foreground")}>
            {batch.failedCount}
          </strong>
          <span className="text-muted-foreground">失败</span>
        </span>
      </div>
    </Link>
  );
}

export function BulkReviewHub() {
  const [status, setStatus] = useState<StatusFilter>("all");

  const listRequest =
    status === "all"
      ? { pageSize: 100 }
      : {
          pageSize: 100,
          status: ListBulkReviewBatchesStatusEnum[
            status === "reviewing"
              ? "Reviewing"
              : status === "ready"
                ? "Ready"
                : status === "applying"
                  ? "Applying"
                  : status === "paused"
                    ? "Paused"
                    : "Completed"
          ],
        };
  const {
    data: batches,
    error: batchError,
    isLoading: batchesLoading,
  } = useSWR(["governance:bulk-review-batches", status], () =>
    governanceApi().listBulkReviewBatches(listRequest),
  );

  return (
    <section className="py-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">批量审核批次</h2>
        <div className="flex flex-wrap gap-1 rounded-xl border bg-muted/35 p-1">
          {(["all", "reviewing", "ready", "paused", "completed"] as const).map(
            (value) => (
              <Button
                key={value}
                type="button"
                size="sm"
                variant={status === value ? "secondary" : "ghost"}
                onClick={() => setStatus(value)}
              >
                {value === "all" ? "全部" : statusMeta[value].label}
              </Button>
            ),
          )}
        </div>
      </div>

      {batchError ? (
        <div className="mt-5 flex items-center gap-3 rounded-2xl border border-destructive/25 bg-destructive/5 p-5 text-sm text-destructive">
          <CircleAlert className="size-4" aria-hidden />
          批次列表加载失败，请确认已登录并具有审核权限。
        </div>
      ) : batchesLoading && !batches ? (
        <div className="mt-5 flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircle className="size-4 animate-spin" aria-hidden />
          正在读取批次…
        </div>
      ) : batches?.items.length ? (
        <div className="mt-5 grid gap-4 md:grid-cols-2">
          {batches.items.map((batch) => (
            <BatchCard key={batch.id} batch={batch} />
          ))}
        </div>
      ) : (
        <div className="mt-5 rounded-2xl border border-dashed bg-muted/20 p-10 text-center">
          <CheckCircle2 className="mx-auto size-6 text-muted-foreground" aria-hidden />
          <p className="mt-3 text-sm font-medium">这个视图里还没有批次</p>
        </div>
      )}
    </section>
  );
}
