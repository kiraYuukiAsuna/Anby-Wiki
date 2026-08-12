"use client";

import Link from "next/link";
import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  Clock3,
  FileText,
  Inbox,
  LoaderCircle,
  RefreshCw,
  XCircle,
} from "lucide-react";
import { useState } from "react";
import useSWRInfinite from "swr/infinite";

import type {
  ImportJob,
  ImportJobListPage,
  ImportJobStatusEnum,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { importsApi } from "@/lib/api";
import { LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 20;

type Filter = "all" | ImportJobStatusEnum;

const FILTERS: Array<{ value: Filter; label: string }> = [
  { value: "all", label: "全部" },
  { value: "running", label: "进行中" },
  { value: "queued", label: "排队中" },
  { value: "succeeded", label: "已完成" },
  { value: "failed", label: "失败" },
  { value: "cancelled", label: "已取消" },
];

const STATUS_META: Record<
  ImportJobStatusEnum,
  {
    label: string;
    className: string;
    icon: typeof Clock3;
  }
> = {
  queued: {
    label: "排队中",
    className: "bg-amber-50 text-amber-700 ring-amber-200",
    icon: Clock3,
  },
  running: {
    label: "处理中",
    className: "bg-sky-50 text-sky-700 ring-sky-200",
    icon: LoaderCircle,
  },
  succeeded: {
    label: "已完成",
    className: "bg-emerald-50 text-emerald-700 ring-emerald-200",
    icon: CheckCircle2,
  },
  failed: {
    label: "失败",
    className: "bg-rose-50 text-rose-700 ring-rose-200",
    icon: AlertTriangle,
  },
  cancelled: {
    label: "已取消",
    className: "bg-slate-100 text-slate-600 ring-slate-200",
    icon: XCircle,
  },
};

const STAGE_LABEL: Record<ImportJob["currentStage"], string> = {
  queued: "等待 Worker",
  fetch: "获取来源",
  parse: "解析内容",
  extract: "抽取知识",
  plan: "规划页面",
  match: "匹配实体",
  compose: "生成提议",
  review: "进入审核",
  complete: "处理完成",
};

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function recordValue(value: unknown): Record<string, unknown> | null {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function sourceSummary(job: ImportJob): { title: string; detail: string } {
  const config = recordValue(job.config);
  const source = recordValue(config?.source);
  const configuredTitle =
    typeof config?.title === "string" ? config.title.trim() : "";
  const filename =
    typeof source?.filename === "string" ? source.filename.trim() : "";
  const url = typeof source?.url === "string" ? source.url : "";

  if (configuredTitle) {
    return {
      title: configuredTitle,
      detail: filename || url || "来源导入",
    };
  }
  if (filename) {
    return { title: filename, detail: "上传文件" };
  }
  if (url) {
    try {
      return { title: new URL(url).hostname, detail: url };
    } catch {
      return { title: "网页来源", detail: url };
    }
  }
  return { title: "来源导入", detail: job.jobType };
}

function ImportJobRow({ job }: { job: ImportJob }) {
  const status = STATUS_META[job.status];
  const StatusIcon = status.icon;
  const source = sourceSummary(job);
  const active = job.status === "queued" || job.status === "running";

  return (
    <li>
      <Link
        href={`/imports/${job.id}`}
        className="group block rounded-2xl border border-border/80 bg-card p-4 shadow-[0_1px_0_rgb(15_23_42/0.03)] transition hover:-translate-y-0.5 hover:border-primary/20 hover:shadow-[0_14px_35px_rgb(15_23_42/0.08)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <div className="flex items-start gap-3">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/6 text-primary">
            <FileText className="size-4.5" aria-hidden />
          </span>
          <span className="min-w-0 flex-1">
            <span className="flex flex-wrap items-start justify-between gap-2">
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold text-foreground">
                  {source.title}
                </span>
                <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                  {source.detail}
                </span>
              </span>
              <span
                className={cn(
                  "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-1 text-[11px] font-medium ring-1 ring-inset",
                  status.className,
                )}
              >
                <StatusIcon
                  className={cn(
                    "size-3",
                    job.status === "running" && "animate-spin",
                  )}
                  aria-hidden
                />
                {status.label}
              </span>
            </span>
            <span className="mt-3 flex items-center gap-3 text-xs text-muted-foreground">
              <span>{STAGE_LABEL[job.currentStage]}</span>
              <span aria-hidden>·</span>
              <time dateTime={job.createdAt.toISOString()}>
                {DATE_FORMATTER.format(job.createdAt)}
              </time>
              <ArrowRight
                className="ml-auto size-3.5 transition-transform group-hover:translate-x-0.5"
                aria-hidden
              />
            </span>
            {active ? (
              <span className="mt-3 block h-1.5 overflow-hidden rounded-full bg-muted">
                <span
                  className="block h-full rounded-full bg-primary transition-[width]"
                  style={{ width: `${job.progress}%` }}
                />
              </span>
            ) : null}
          </span>
        </div>
      </Link>
    </li>
  );
}

export function ImportJobList() {
  const [filter, setFilter] = useState<Filter>("all");
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const { data, error, isLoading, isValidating, mutate, size, setSize } =
    useSWRInfinite<ImportJobListPage>(
      (pageIndex, previousPage) => {
        if (!isAuthenticated) return null;
        if (pageIndex > 0 && !previousPage?.nextCursor) return null;
        return [
          "import-jobs",
          filter,
          pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
        ] as const;
      },
      ([, selectedFilter, cursor]) =>
        importsApi().listImportJobs({
          cursor: typeof cursor === "string" && cursor ? cursor : undefined,
          pageSize: PAGE_SIZE,
          status:
            selectedFilter === "all"
              ? undefined
              : (selectedFilter as ImportJobStatusEnum),
        }),
      {
        refreshInterval: (pages) =>
          pages?.some((page) =>
            page.items.some(
              (job) => job.status === "queued" || job.status === "running",
            ),
          )
            ? 2500
            : 0,
        revalidateFirstPage: true,
      },
    );

  const jobs = data?.flatMap((page) => page.items) ?? [];
  const pageCount = data?.length ?? 0;
  const lastPage = pageCount > 0 ? data?.[pageCount - 1] : undefined;
  const reachedEnd = Boolean(data && !lastPage?.nextCursor);
  const loadingMore =
    isValidating && pageCount > 0 && size > pageCount;

  if (sessionLoading) {
    return (
      <div className="flex min-h-48 items-center justify-center rounded-2xl border border-dashed">
        <LoaderCircle
          className="size-5 animate-spin text-muted-foreground"
          aria-label="正在读取账户"
        />
      </div>
    );
  }

  if (!isAuthenticated) {
    return (
      <div className="rounded-2xl border border-dashed bg-muted/25 px-6 py-10 text-center">
        <Inbox className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h3 className="mt-4 font-semibold">登录后恢复你的导入队列</h3>
        <p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-muted-foreground">
          所有历史任务、失败原因、重试记录和待审核提议都会保存在这里。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>登录并查看</Link>
        </Button>
      </div>
    );
  }

  return (
    <section aria-labelledby="import-queue-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-xs font-semibold tracking-[0.16em] text-primary uppercase">
            Your queue
          </p>
          <h2 id="import-queue-title" className="mt-1 text-xl font-semibold">
            导入队列
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            任务永久保留，可随时回来继续处理。
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={isValidating}
          onClick={() => void mutate()}
          aria-label="刷新导入队列"
        >
          <RefreshCw
            className={cn("size-3.5", isValidating && "animate-spin")}
            aria-hidden
          />
          刷新
        </Button>
      </div>

      <div
        className="mt-5 flex gap-1 overflow-x-auto rounded-xl bg-muted/70 p-1"
        aria-label="按状态筛选导入任务"
      >
        {FILTERS.map((item) => (
          <button
            key={item.value}
            type="button"
            aria-pressed={filter === item.value}
            onClick={() => setFilter(item.value)}
            className={cn(
              "shrink-0 rounded-lg px-3 py-1.5 text-xs font-medium text-muted-foreground transition",
              filter === item.value &&
                "bg-background text-foreground shadow-sm ring-1 ring-border/70",
            )}
          >
            {item.label}
          </button>
        ))}
      </div>

      {error ? (
        <div className="mt-4 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm">
          <p className="font-medium text-destructive">导入队列暂时无法读取</p>
          <p className="mt-1 text-muted-foreground">请检查服务状态后重试。</p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="mt-4"
            onClick={() => void mutate()}
          >
            重新加载
          </Button>
        </div>
      ) : null}

      {isLoading ? (
        <div className="mt-4 space-y-3" aria-label="正在加载导入任务">
          {[0, 1, 2].map((item) => (
            <div
              key={item}
              className="h-28 animate-pulse rounded-2xl border bg-muted/35"
            />
          ))}
        </div>
      ) : null}

      {!isLoading && !error && jobs.length === 0 ? (
        <div className="mt-4 rounded-2xl border border-dashed px-6 py-10 text-center">
          <Inbox className="mx-auto size-7 text-muted-foreground" aria-hidden />
          <p className="mt-3 text-sm font-medium">这个筛选条件下还没有任务</p>
          <p className="mt-1 text-xs text-muted-foreground">
            从右侧提交网页或文件，第一条任务会立即出现在这里。
          </p>
        </div>
      ) : null}

      {jobs.length > 0 ? (
        <ul className="mt-4 space-y-3">
          {jobs.map((job) => (
            <ImportJobRow key={job.id} job={job} />
          ))}
        </ul>
      ) : null}

      {jobs.length > 0 && !reachedEnd ? (
        <Button
          type="button"
          variant="outline"
          className="mt-4 w-full"
          disabled={loadingMore}
          onClick={() => void setSize(size + 1)}
        >
          {loadingMore ? "正在加载…" : "加载更早的任务"}
        </Button>
      ) : null}
    </section>
  );
}
