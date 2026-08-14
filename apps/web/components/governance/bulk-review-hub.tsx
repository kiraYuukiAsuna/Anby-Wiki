"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  LoaderCircle,
  Plus,
  ShieldCheck,
} from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";
import {
  ListBulkReviewBatchesStatusEnum,
  type BulkReviewBatchSummary,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { governanceApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";
import { compactId } from "@/lib/display-id";
import { cn } from "@/lib/utils";

const createBatchSchema = z.object({
  proposalIds: z.array(z.string().uuid()).min(1, "至少选择一个待审提案"),
  samplePercent: z.coerce.number().int().min(1).max(100),
  waveSize: z.coerce.number().int().min(1).max(1000),
  forceFull: z.boolean(),
});

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
  const progress = Math.min(100, Math.round((finished / batch.itemCount) * 100));

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
  const router = useRouter();
  const [status, setStatus] = useState<StatusFilter>("all");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [samplePercent, setSamplePercent] = useState("20");
  const [waveSize, setWaveSize] = useState("25");
  const [forceFull, setForceFull] = useState(false);
  const [creating, setCreating] = useState(false);

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
    mutate: mutateBatches,
  } = useSWR(["governance:bulk-review-batches", status], () =>
    governanceApi().listBulkReviewBatches(listRequest),
  );
  const {
    data: tasks,
    error: taskError,
    isLoading: tasksLoading,
    mutate: mutateTasks,
  } = useSWR("governance:bulk-review-candidates", () =>
    governanceApi().listReviewTasks({ pageSize: 100 }),
  );

  const allSelected = useMemo(
    () =>
      Boolean(tasks?.items.length) &&
      tasks?.items.every((task) => selected.has(task.proposalId)),
    [selected, tasks],
  );

  const toggleAll = (checked: boolean) => {
    setSelected(
      checked
        ? new Set(tasks?.items.map((task) => task.proposalId) ?? [])
        : new Set(),
    );
  };

  const createBatch = async () => {
    const parsed = createBatchSchema.safeParse({
      proposalIds: [...selected],
      samplePercent,
      waveSize,
      forceFull,
    });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查批次参数");
      return;
    }
    setCreating(true);
    try {
      const created = await governanceApi().createBulkReviewBatch({
        createBulkReviewBatchRequest: {
          proposalIds: new Set(parsed.data.proposalIds),
          samplePercent: parsed.data.samplePercent,
          waveSize: parsed.data.waveSize,
          forceFull: parsed.data.forceFull,
        },
      });
      await Promise.all([mutateBatches(), mutateTasks()]);
      toast.success("批量评审批次已创建");
      router.push(`/governance/bulk/${created.id}`);
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("请先登录后创建批次");
        router.push(LOGIN_PATH);
      } else {
        toast.error("批次创建失败", {
          description: "请确认所选提案仍在待审状态且拥有审核权限。",
        });
      }
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="grid gap-8 py-8 xl:grid-cols-[minmax(0,1fr)_23rem]">
      <section>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold">持续中的批次</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              批次不会因离开页面而丢失，可随时恢复处理。
            </p>
          </div>
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
            <p className="mt-1 text-xs text-muted-foreground">
              从右侧待审提案创建第一个可恢复的批次。
            </p>
          </div>
        )}
      </section>

      <aside className="xl:sticky xl:top-24 xl:self-start">
        <div className="rounded-2xl border border-primary/15 bg-card shadow-[0_20px_55px_-38px_color-mix(in_oklch,var(--primary),transparent_45%)]">
          <div className="border-b border-border/75 p-5">
            <span className="flex size-9 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <Plus className="size-4" aria-hidden />
            </span>
            <h2 className="mt-4 font-semibold">创建评审批次</h2>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              选中的成员、抽样结果和 wave 会在创建时冻结。
            </p>
          </div>

          <div className="p-5">
            <div className="flex items-center justify-between">
              <label className="flex items-center gap-2 text-xs font-medium">
                <Checkbox
                  checked={allSelected}
                  onCheckedChange={(value) => toggleAll(value === true)}
                  aria-label="选择全部待审提案"
                />
                待审提案
              </label>
              <span className="text-xs text-muted-foreground">
                已选 {selected.size}/{tasks?.items.length ?? 0}
              </span>
            </div>
            <div className="mt-3 max-h-52 space-y-1.5 overflow-y-auto rounded-xl border bg-muted/20 p-2">
              {taskError ? (
                <p className="p-3 text-xs text-destructive">待审队列加载失败。</p>
              ) : tasksLoading && !tasks ? (
                <p className="p-3 text-xs text-muted-foreground">正在读取…</p>
              ) : tasks?.items.length ? (
                tasks.items.map((task) => (
                  <label
                    key={task.id}
                    className="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-2 hover:bg-background"
                  >
                    <Checkbox
                      checked={selected.has(task.proposalId)}
                      onCheckedChange={(value) =>
                        setSelected((current) => {
                          const next = new Set(current);
                          if (value === true) next.add(task.proposalId);
                          else next.delete(task.proposalId);
                          return next;
                        })
                      }
                      aria-label={`选择提案 ${task.proposalId}`}
                    />
                    <span className="min-w-0">
                      <span className="block truncate font-mono text-xs">
                        {compactId(task.proposalId)}
                      </span>
                      <span className="block text-[11px] text-muted-foreground">
                        {new Intl.DateTimeFormat("zh-CN", {
                          month: "short",
                          day: "numeric",
                          hour: "2-digit",
                          minute: "2-digit",
                        }).format(task.createdAt)}
                      </span>
                    </span>
                  </label>
                ))
              ) : (
                <p className="p-3 text-xs text-muted-foreground">没有可加入批次的待审提案。</p>
              )}
            </div>

            <div className="mt-5 grid grid-cols-2 gap-3">
              <label className="text-xs font-medium">
                抽样比例
                <span className="mt-1 flex items-center">
                  <Input
                    type="number"
                    min={1}
                    max={100}
                    value={samplePercent}
                    onChange={(event) => setSamplePercent(event.target.value)}
                    className="rounded-r-none"
                  />
                  <span className="flex h-8 items-center rounded-r-lg border border-l-0 px-2 text-muted-foreground">
                    %
                  </span>
                </span>
              </label>
              <label className="text-xs font-medium">
                每个 wave
                <Input
                  type="number"
                  min={1}
                  max={1000}
                  value={waveSize}
                  onChange={(event) => setWaveSize(event.target.value)}
                  className="mt-1"
                />
              </label>
            </div>
            <label className="mt-4 flex cursor-pointer gap-3 rounded-xl border border-border/75 p-3">
              <Checkbox
                checked={forceFull}
                onCheckedChange={(value) => setForceFull(value === true)}
                aria-label="强制全量审核"
              />
              <span>
                <span className="block text-xs font-medium">强制全量审核</span>
                <span className="mt-1 block text-[11px] leading-4 text-muted-foreground">
                  高危提案会自动切换为全量；这里可主动提高审查强度。
                </span>
              </span>
            </label>
            <Button
              className="mt-5 w-full"
              disabled={creating || !selected.size}
              onClick={() => void createBatch()}
            >
              {creating ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <ShieldCheck aria-hidden />
              )}
              冻结并开始评审
            </Button>
          </div>
        </div>
      </aside>
    </div>
  );
}
