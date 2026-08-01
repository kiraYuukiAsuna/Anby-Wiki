"use client";

import {
  Archive,
  CheckCircle2,
  Clock3,
  Database,
  Flame,
  HardDrive,
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  ShieldCheck,
  Snowflake,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRMutation from "swr/mutation";
import { z } from "zod";

import {
  ResponseError,
  type RevisionStorageStats,
  type SnapshotArchiveResult,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { governanceApi } from "@/lib/api";
import { LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const archiveLimitSchema = z.coerce
  .number()
  .int("批次大小必须是整数")
  .min(1, "批次大小至少为 1")
  .max(500, "批次大小不能超过 500");

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let amount = value / 1024;
  let unit = units[0];
  for (let index = 1; index < units.length && amount >= 1024; index += 1) {
    amount /= 1024;
    unit = units[index];
  }
  return `${amount >= 10 ? amount.toFixed(1) : amount.toFixed(2)} ${unit}`;
}

function formatDuration(seconds: number): string {
  const days = Math.round(seconds / 86_400);
  if (days >= 365 && days % 365 === 0) return `${days / 365} 年`;
  if (days >= 30 && days % 30 === 0) return `${days / 30} 个月`;
  return `${days} 天`;
}

function Metric({
  icon: Icon,
  label,
  value,
  detail,
  tone = "default",
}: {
  icon: typeof Database;
  label: string;
  value: string;
  detail: string;
  tone?: "default" | "hot" | "cold";
}) {
  return (
    <div className="rounded-2xl border bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)]">
      <span
        className={cn(
          "flex size-9 items-center justify-center rounded-xl",
          tone === "hot" && "bg-orange-100 text-orange-700",
          tone === "cold" && "bg-sky-100 text-sky-700",
          tone === "default" && "bg-primary/9 text-primary",
        )}
      >
        <Icon className="size-4" aria-hidden />
      </span>
      <p className="mt-4 text-3xl font-semibold tracking-[-0.035em]">{value}</p>
      <p className="mt-1 text-sm font-medium">{label}</p>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">{detail}</p>
    </div>
  );
}

function ArchiveDialog({
  stats,
  onComplete,
}: {
  stats: RevisionStorageStats;
  onComplete: (result: SnapshotArchiveResult) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [limit, setLimit] = useState(String(stats.defaultBatchSize));
  const [validationError, setValidationError] = useState("");
  const archive = useSWRMutation(
    "governance:revision-storage:archive",
    async () => {
      const parsed = archiveLimitSchema.safeParse(limit);
      if (!parsed.success) {
        setValidationError(parsed.error.issues[0]?.message ?? "批次大小无效");
        throw new Error("validation");
      }
      setValidationError("");
      return governanceApi().archiveRevisionSnapshots({
        archiveRevisionSnapshotsRequest: { limit: parsed.data },
      });
    },
  );

  const run = async () => {
    try {
      const result = await archive.trigger();
      if (!result) return;
      await onComplete(result);
      setOpen(false);
      setConfirmed(false);
      toast.success("归档批次已完成", {
        description: `已迁移 ${result.archived} 个快照（${formatBytes(result.archivedBytes)}），跳过 ${result.skipped} 个。`,
      });
    } catch (error) {
      if (error instanceof Error && error.message === "validation") return;
      if (error instanceof ResponseError && error.response.status === 503) {
        toast.error("冷存储尚不可用", {
          description: "请先检查 API 的 S3 配置与对象存储健康状态。",
        });
      } else {
        toast.error("归档批次执行失败", {
          description: "数据库不会切换到缺失或校验失败的冷对象，可安全重试。",
        });
      }
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setConfirmed(false);
          setValidationError("");
        }
      }}
    >
      <DialogTrigger asChild>
        <Button disabled={!stats.archiveAvailable || stats.eligibleSnapshots === 0}>
          <Archive aria-hidden />
          执行归档批次
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>迁移到冷存储</DialogTitle>
          <DialogDescription>
            系统会先上传并校验内容寻址对象，再原子切换数据库中的物理层标记。
            当前版本不会成为候选，历史读取仍会透明回源。
          </DialogDescription>
        </DialogHeader>

        <div className="rounded-xl border bg-muted/35 p-4">
          <div className="flex items-center justify-between gap-4 text-sm">
            <span className="text-muted-foreground">当前到期候选</span>
            <strong>{stats.eligibleSnapshots.toLocaleString("zh-CN")} 个</strong>
          </div>
          <label className="mt-4 block text-xs font-medium" htmlFor="archive-limit">
            本批最多处理
          </label>
          <Input
            id="archive-limit"
            className="mt-2"
            type="number"
            min={1}
            max={500}
            value={limit}
            onChange={(event) => setLimit(event.target.value)}
            aria-invalid={Boolean(validationError)}
          />
          {validationError ? (
            <p className="mt-1.5 text-xs text-destructive">{validationError}</p>
          ) : null}
        </div>

        <label className="flex cursor-pointer items-start gap-3 rounded-xl border p-3 text-xs leading-5">
          <Checkbox
            className="mt-0.5"
            checked={confirmed}
            onCheckedChange={(value) => setConfirmed(value === true)}
          />
          <span>
            我理解这只迁移不可变快照的物理存放位置；对象校验失败时批次会停止，
            不会删除数据库内容。
          </span>
        </label>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => setOpen(false)}
            disabled={archive.isMutating}
          >
            取消
          </Button>
          <Button
            onClick={() => void run()}
            disabled={!confirmed || archive.isMutating}
          >
            {archive.isMutating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : (
              <Archive aria-hidden />
            )}
            {archive.isMutating ? "正在归档…" : "确认执行"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function RevisionStorageWorkspace() {
  const session = useSession();
  const storage = useSWR(
    session.isAuthenticated ? "governance:revision-storage" : null,
    () => governanceApi().getRevisionStorageStats(),
    { shouldRetryOnError: false, revalidateOnFocus: true },
  );

  const totalSnapshots =
    (storage.data?.hotSnapshots ?? 0) + (storage.data?.coldSnapshots ?? 0);
  const coldPercent = useMemo(() => {
    if (!storage.data || totalSnapshots === 0) return 0;
    return Math.round((storage.data.coldSnapshots / totalSnapshots) * 100);
  }, [storage.data, totalSnapshots]);

  if (session.isLoading) {
    return (
      <div className="flex min-h-72 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在确认管理员权限…
      </div>
    );
  }
  if (!session.isAuthenticated) {
    return (
      <div className="rounded-3xl border border-dashed bg-muted/25 p-10 text-center">
        <LockKeyhole className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后查看存储治理</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          该页面展示站点级存储容量和运维动作，只向管理员开放。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }
  if (
    storage.error instanceof ResponseError &&
    storage.error.response.status === 403
  ) {
    return (
      <div className="rounded-3xl border border-amber-200 bg-amber-50/65 p-8">
        <ShieldCheck className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">
          需要管理员权限
        </h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-amber-900/70">
          Revision 历史仍可正常阅读；容量统计、保留期和手动归档仅供 admin
          角色操作。
        </p>
      </div>
    );
  }
  if (storage.error) {
    return (
      <div className="rounded-3xl border border-destructive/20 bg-destructive/5 p-8">
        <TriangleAlert className="size-7 text-destructive" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">暂时无法读取存储状态</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          请确认 API 与数据库可用后重试。
        </p>
        <Button
          className="mt-5"
          variant="outline"
          onClick={() => void storage.mutate()}
        >
          <RefreshCw aria-hidden />
          重试
        </Button>
      </div>
    );
  }
  if (!storage.data) {
    return (
      <div className="grid gap-3 sm:grid-cols-3">
        {[0, 1, 2].map((item) => (
          <div
            key={item}
            className="h-44 animate-pulse rounded-2xl border bg-muted/30"
          />
        ))}
      </div>
    );
  }

  const stats = storage.data;
  return (
    <div className="space-y-8">
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Metric
          icon={Flame}
          tone="hot"
          label="热快照"
          value={stats.hotSnapshots.toLocaleString("zh-CN")}
          detail={`${formatBytes(stats.hotBytes)} · 当前版本与近期历史`}
        />
        <Metric
          icon={Snowflake}
          tone="cold"
          label="冷快照"
          value={stats.coldSnapshots.toLocaleString("zh-CN")}
          detail={`${formatBytes(stats.coldBytes)} · 按需校验并透明回源`}
        />
        <Metric
          icon={Clock3}
          label="归档保留期"
          value={formatDuration(stats.retentionSeconds)}
          detail="超过此年龄且不是当前版本时成为候选"
        />
        <Metric
          icon={Archive}
          label="当前到期候选"
          value={stats.eligibleSnapshots.toLocaleString("zh-CN")}
          detail={`默认每批最多 ${stats.defaultBatchSize} 个`}
        />
      </section>

      <section className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="rounded-3xl border bg-card p-6">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <p className="text-xs font-semibold tracking-[0.14em] text-primary uppercase">
                Physical tier distribution
              </p>
              <h2 className="mt-2 text-xl font-semibold tracking-tight">
                不可变历史的物理分布
              </h2>
            </div>
            <span
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium",
                stats.archiveAvailable
                  ? "bg-emerald-100 text-emerald-800"
                  : "bg-amber-100 text-amber-800",
              )}
            >
              {stats.archiveAvailable ? (
                <CheckCircle2 className="size-3" aria-hidden />
              ) : (
                <TriangleAlert className="size-3" aria-hidden />
              )}
              {stats.archiveAvailable ? "对象存储已装配" : "对象存储未装配"}
            </span>
          </div>

          <div className="mt-8">
            <div className="flex h-4 overflow-hidden rounded-full bg-orange-200/70">
              <div
                className="bg-sky-500 transition-[width]"
                style={{ width: `${coldPercent}%` }}
                aria-hidden
              />
            </div>
            <div className="mt-3 flex flex-wrap justify-between gap-2 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-1.5">
                <span className="size-2 rounded-full bg-orange-400" />
                Hot {100 - coldPercent}%
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="size-2 rounded-full bg-sky-500" />
                Cold {coldPercent}%
              </span>
            </div>
          </div>

          <dl className="mt-8 grid gap-3 border-t pt-5 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-xs text-muted-foreground">最早热快照</dt>
              <dd className="mt-1 font-medium">
                {stats.oldestHotCreatedAt
                  ? DATE_FORMATTER.format(stats.oldestHotCreatedAt)
                  : "暂无热快照"}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">逻辑快照总量</dt>
              <dd className="mt-1 font-medium">
                {totalSnapshots.toLocaleString("zh-CN")} 个 ·{" "}
                {formatBytes(stats.hotBytes + stats.coldBytes)}
              </dd>
            </div>
          </dl>
        </div>

        <aside className="rounded-3xl border bg-muted/25 p-6">
          <HardDrive className="size-5 text-primary" aria-hidden />
          <h2 className="mt-4 text-lg font-semibold">安全归档</h2>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            上传、Head 校验、数据库行锁与当前版本复核依次完成。历史详情、Diff
            与回滚会自动读取冷对象，并重新验证大小、SHA-256 和 AST Schema。
          </p>
          {!stats.archiveAvailable ? (
            <p className="mt-4 rounded-xl border border-amber-200 bg-amber-50 p-3 text-xs leading-5 text-amber-900">
              当前 API 未装配 S3。阅读热版本不受影响；请在运维环境补齐对象存储配置。
            </p>
          ) : null}
          <div className="mt-5 flex flex-col gap-2">
            <ArchiveDialog
              stats={stats}
              onComplete={async () => {
                await storage.mutate();
              }}
            />
            <Button
              variant="outline"
              disabled={storage.isValidating}
              onClick={() => void storage.mutate()}
            >
              <RefreshCw
                className={cn(storage.isValidating && "animate-spin")}
                aria-hidden
              />
              刷新统计
            </Button>
          </div>
        </aside>
      </section>
    </div>
  );
}
