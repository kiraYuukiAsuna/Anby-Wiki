"use client";

import {
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  FileWarning,
  Inbox,
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  ScanSearch,
  ShieldAlert,
  TriangleAlert,
  Waypoints,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";

import {
  ResponseError,
  type FactConsistencyIssue,
  type FactConsistencyIssueListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { knowledgeApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 30;
type StatusFilter = "open" | "resolved" | "all";
type IssueTypeFilter =
  | ""
  | "single_value_conflict"
  | "multiple_preferred_values"
  | "verified_without_support"
  | "merged_entity_target"
  | "evidence_disagreement";

const ISSUE_TYPES: ReadonlyArray<{
  value: IssueTypeFilter;
  label: string;
}> = [
  { value: "", label: "全部问题类型" },
  { value: "single_value_conflict", label: "单值属性冲突" },
  { value: "multiple_preferred_values", label: "多个首选值" },
  { value: "verified_without_support", label: "已验证但无支持证据" },
  { value: "merged_entity_target", label: "指向已合并 Entity" },
  { value: "evidence_disagreement", label: "证据支持与反驳并存" },
];

const ISSUE_META: Record<
  FactConsistencyIssue["issueType"],
  { title: string; detail: string }
> = {
  single_value_conflict: {
    title: "单值属性存在多个已发布 Claim",
    detail: "该 Property 不允许多值，需要保留一个事实并取代其余 Claim。",
  },
  multiple_preferred_values: {
    title: "多值属性存在多个首选值",
    detail: "多值可以并存，但同一属性通常只应有一个 preferred Claim。",
  },
  verified_without_support: {
    title: "人工验证的 Claim 缺少支持证据",
    detail: "补充 supports Citation，或重新评估验证状态。",
  },
  merged_entity_target: {
    title: "Claim 指向已合并的 Entity",
    detail: "目标应更新为合并后的稳定 Entity，避免继续扩散旧身份。",
  },
  evidence_disagreement: {
    title: "支持与反驳证据同时存在",
    detail: "核对证据并把 Claim 标为 disputed，或修正错误的来源关系。",
  },
};

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function IssueCard({ issue }: { issue: FactConsistencyIssue }) {
  const meta = ISSUE_META[issue.issueType];
  const isError = issue.severity === "error";
  const Icon = isError ? TriangleAlert : CircleAlert;

  return (
    <li className="rounded-2xl border bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)]">
      <div className="flex items-start gap-4">
        <span
          className={cn(
            "flex size-10 shrink-0 items-center justify-center rounded-xl",
            isError
              ? "bg-red-100 text-red-700"
              : "bg-amber-100 text-amber-800",
          )}
        >
          <Icon className="size-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <p className="font-semibold">{meta.title}</p>
                {issue.status === "resolved" ? (
                  <span className="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium text-emerald-800">
                    <CheckCircle2 className="size-2.5" aria-hidden />
                    已恢复一致
                  </span>
                ) : null}
              </div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {meta.detail}
              </p>
            </div>
            <span
              className={cn(
                "rounded-full px-2.5 py-1 text-[10px] font-semibold uppercase",
                isError
                  ? "bg-red-50 text-red-700"
                  : "bg-amber-50 text-amber-800",
              )}
            >
              {isError ? "Error" : "Warning"}
            </span>
          </div>

          <div className="mt-4 grid gap-3 rounded-xl bg-muted/35 p-3 sm:grid-cols-2">
            <div>
              <p className="text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">
                Subject Entity
              </p>
              <Link
                href={`/entities/${issue.subjectEntityId}`}
                className="mt-1 inline-flex max-w-full items-center gap-1.5 text-sm font-medium text-primary hover:underline"
              >
                <Waypoints className="size-3.5 shrink-0" aria-hidden />
                <span className="truncate">{issue.subjectLabel}</span>
              </Link>
            </div>
            <div>
              <p className="text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">
                Property
              </p>
              <p className="mt-1 truncate text-sm font-medium">
                {issue.propertyName || issue.propertyKey || "跨属性检查"}
              </p>
              {issue.propertyKey ? (
                <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
                  {issue.propertyKey}
                </p>
              ) : null}
            </div>
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-2">
            {issue.claimIds.map((claimId) => (
              <Button
                key={claimId}
                asChild
                size="xs"
                variant="outline"
              >
                <Link href={`/claims/${claimId}`}>
                  Claim {claimId.slice(0, 8)}
                  <ArrowRight aria-hidden />
                </Link>
              </Button>
            ))}
          </div>

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
            <p className="text-[10px] text-muted-foreground">
              最近检查：{DATE_FORMATTER.format(issue.lastCheckedAt)}
            </p>
            <details className="text-right">
              <summary className="cursor-pointer text-[10px] font-medium text-muted-foreground hover:text-foreground">
                检查详情
              </summary>
              <pre className="mt-2 max-w-xl overflow-auto rounded-lg bg-muted/45 p-2 text-left text-[10px] leading-4">
                {JSON.stringify(issue.details, null, 2)}
              </pre>
            </details>
          </div>
        </div>
      </div>
    </li>
  );
}

export function FactConsistencyWorkspace() {
  const session = useSession();
  const [status, setStatus] = useState<StatusFilter>("open");
  const [issueType, setIssueType] = useState<IssueTypeFilter>("");
  const [scanning, setScanning] = useState(false);

  const issues = useSWRInfinite<FactConsistencyIssueListPage>(
    (pageIndex, previousPage) => {
      if (!session.isAuthenticated) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "knowledge:fact-consistency",
        status,
        issueType,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, selectedStatus, selectedType, cursor] =
        cacheKey as readonly [
          string,
          StatusFilter,
          IssueTypeFilter,
          string,
        ];
      return knowledgeApi().listFactConsistencyIssues({
        status: selectedStatus,
        issueType: selectedType || undefined,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
    {
      revalidateFirstPage: true,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );
  const items = useMemo(
    () => issues.data?.flatMap((page) => page.items) ?? [],
    [issues.data],
  );
  const lastPage = issues.data?.[issues.data.length - 1];
  const errorCount = items.filter(
    (item) => item.severity === "error",
  ).length;
  const entityCount = new Set(
    items.map((item) => item.subjectEntityId),
  ).size;

  const scan = async () => {
    setScanning(true);
    try {
      const result = await knowledgeApi().scanFactConsistency();
      await issues.mutate();
      toast.success("事实一致性扫描完成", {
        description: `扫描 ${result.scannedSubjects} 个 Entity，发现 ${result.openIssues} 个当前问题，关闭 ${result.resolvedIssues} 个旧问题。`,
      });
    } catch (error) {
      if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("只有站点管理员可以发起全量扫描", {
          description: "审核者仍可查看由 Worker 持续更新的问题队列。",
        });
      } else {
        toast.error("事实一致性扫描失败");
      }
    } finally {
      setScanning(false);
    }
  };

  if (session.isLoading) {
    return (
      <div className="flex min-h-72 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在确认审核权限…
      </div>
    );
  }
  if (!session.isAuthenticated) {
    return (
      <div className="rounded-3xl border border-dashed bg-muted/25 p-10 text-center">
        <LockKeyhole className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后查看一致性问题</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          问题可能包含尚在治理流程中的 Claim，只向审核者与管理员开放。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }
  if (
    issues.error instanceof ResponseError &&
    issues.error.response.status === 403
  ) {
    return (
      <div className="rounded-3xl border border-amber-200 bg-amber-50/65 p-8">
        <ShieldAlert className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">
          需要审核者权限
        </h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-amber-900/70">
          普通读者仍可查看已发布事实；一致性问题队列仅向 reviewer 与 admin
          角色开放。
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <section className="grid gap-3 sm:grid-cols-3">
        {[
          {
            label: "已加载问题",
            value: items.length,
            icon: FileWarning,
          },
          { label: "错误级问题", value: errorCount, icon: TriangleAlert },
          { label: "涉及 Entity", value: entityCount, icon: Waypoints },
        ].map(({ label, value, icon: Icon }) => (
          <div key={label} className="rounded-2xl border bg-card p-5">
            <Icon className="size-4 text-amber-700" aria-hidden />
            <p className="mt-4 text-3xl font-semibold tracking-tight">
              {issues.isLoading ? "—" : value}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">{label}</p>
          </div>
        ))}
      </section>

      <section>
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h2 className="text-xl font-semibold tracking-tight">
              一致性问题队列
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Worker 随 Claim 变化增量重算；管理员也可随时执行全量扫描。
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={issues.isValidating}
              onClick={() => void issues.mutate()}
            >
              <RefreshCw
                className={cn(
                  issues.isValidating && "animate-spin",
                )}
                aria-hidden
              />
              刷新
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={scanning}
              onClick={() => void scan()}
            >
              {scanning ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <ScanSearch aria-hidden />
              )}
              {scanning ? "扫描中…" : "全量扫描"}
            </Button>
          </div>
        </div>

        <div className="mt-5 grid gap-2 sm:grid-cols-2">
          <select
            value={status}
            onChange={(event) => {
              setStatus(event.target.value as StatusFilter);
              void issues.setSize(1);
            }}
            aria-label="问题状态"
            className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
          >
            <option value="open">待处理</option>
            <option value="resolved">已恢复一致</option>
            <option value="all">全部状态</option>
          </select>
          <select
            value={issueType}
            onChange={(event) => {
              setIssueType(event.target.value as IssueTypeFilter);
              void issues.setSize(1);
            }}
            aria-label="问题类型"
            className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
          >
            {ISSUE_TYPES.map((item) => (
              <option key={item.value} value={item.value}>
                {item.label}
              </option>
            ))}
          </select>
        </div>

        {issues.isLoading && !issues.data ? (
          <div className="mt-5 space-y-3">
            {[0, 1, 2].map((item) => (
              <div
                key={item}
                className="h-64 animate-pulse rounded-2xl border bg-muted/30"
              />
            ))}
          </div>
        ) : null}
        {issues.error &&
        !(
          issues.error instanceof ResponseError &&
          issues.error.response.status === 403
        ) ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm">
            <p className="font-medium text-destructive">问题队列暂时无法读取</p>
            <p className="mt-1 text-muted-foreground">
              请确认 API 与数据库可用后重试。
            </p>
          </div>
        ) : null}
        {!issues.isLoading && !issues.error && items.length === 0 ? (
          <div className="mt-5 rounded-3xl border border-dashed px-6 py-14 text-center">
            <Inbox className="mx-auto size-8 text-emerald-700" aria-hidden />
            <h3 className="mt-4 font-semibold">当前筛选下没有问题</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              这表示已扫描的权威事实满足当前一致性规则；仍可执行全量扫描复核。
            </p>
          </div>
        ) : null}
        {items.length > 0 ? (
          <ol className="mt-5 space-y-3">
            {items.map((issue) => (
              <IssueCard key={issue.id} issue={issue} />
            ))}
          </ol>
        ) : null}
        {lastPage?.nextCursor ? (
          <Button
            type="button"
            variant="outline"
            className="mt-4 w-full"
            disabled={issues.isValidating}
            onClick={() => void issues.setSize(issues.size + 1)}
          >
            {issues.isValidating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : null}
            加载更多问题
          </Button>
        ) : null}
      </section>
    </div>
  );
}
