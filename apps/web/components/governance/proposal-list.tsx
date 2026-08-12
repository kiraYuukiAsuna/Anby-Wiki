"use client";

import Link from "next/link";
import {
  AlertTriangle,
  ArrowRight,
  Bot,
  CheckCircle2,
  CircleDashed,
  Clock3,
  FilePenLine,
  GitMerge,
  Inbox,
  LoaderCircle,
  Plus,
  RefreshCw,
  RotateCcw,
  ShieldAlert,
  XCircle,
} from "lucide-react";
import { useState } from "react";
import useSWRInfinite from "swr/infinite";

import type {
  Proposal,
  ProposalListPage,
  ProposalRiskLevelEnum,
  ProposalStatusEnum,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { governanceApi } from "@/lib/api";
import { LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 20;

type Filter = "all" | ProposalStatusEnum;

const FILTERS: Array<{ value: Filter; label: string }> = [
  { value: "all", label: "全部" },
  { value: "draft", label: "草稿" },
  { value: "in_review", label: "审核中" },
  { value: "approved", label: "已批准" },
  { value: "conflicted", label: "有冲突" },
  { value: "applied", label: "已生效" },
  { value: "failed", label: "失败" },
];

const STATUS_META: Record<
  ProposalStatusEnum,
  { label: string; className: string; icon: typeof Clock3 }
> = {
  draft: {
    label: "草稿",
    className: "bg-slate-100 text-slate-700 ring-slate-200",
    icon: FilePenLine,
  },
  submitted: {
    label: "已提交",
    className: "bg-blue-50 text-blue-700 ring-blue-200",
    icon: Clock3,
  },
  in_review: {
    label: "审核中",
    className: "bg-amber-50 text-amber-700 ring-amber-200",
    icon: ShieldAlert,
  },
  approved: {
    label: "已批准",
    className: "bg-emerald-50 text-emerald-700 ring-emerald-200",
    icon: CheckCircle2,
  },
  rejected: {
    label: "已拒绝",
    className: "bg-rose-50 text-rose-700 ring-rose-200",
    icon: XCircle,
  },
  conflicted: {
    label: "待解冲突",
    className: "bg-orange-50 text-orange-700 ring-orange-200",
    icon: GitMerge,
  },
  applying: {
    label: "应用中",
    className: "bg-violet-50 text-violet-700 ring-violet-200",
    icon: LoaderCircle,
  },
  applied: {
    label: "已生效",
    className: "bg-emerald-50 text-emerald-700 ring-emerald-200",
    icon: CheckCircle2,
  },
  failed: {
    label: "应用失败",
    className: "bg-rose-50 text-rose-700 ring-rose-200",
    icon: AlertTriangle,
  },
  rolled_back: {
    label: "已回滚",
    className: "bg-slate-100 text-slate-600 ring-slate-200",
    icon: RotateCcw,
  },
};

const TARGET_LABEL: Record<Proposal["targetType"], string> = {
  wiki: "百科导入批次",
  page: "百科页面",
  entity: "实体",
  claim: "事实声明",
  collection: "专题合集",
  external_resource: "外部资源",
};

const RISK_META: Record<
  ProposalRiskLevelEnum,
  { label: string; className: string }
> = {
  low: { label: "低风险", className: "text-emerald-700" },
  medium: { label: "中风险", className: "text-amber-700" },
  high: { label: "高风险", className: "text-orange-700" },
  critical: { label: "关键风险", className: "text-rose-700" },
};

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function ProposalRow({ proposal }: { proposal: Proposal }) {
  const status = STATUS_META[proposal.status];
  const risk = RISK_META[proposal.riskLevel];
  const StatusIcon = status.icon;
  const targetIdentity = proposal.targetId
    ? proposal.targetId.slice(0, 8)
    : "新建目标";

  return (
    <li>
      <Link
        href={`/governance/proposals/${proposal.id}`}
        className="group block rounded-2xl border border-border/80 bg-card p-4 shadow-[0_1px_0_rgb(15_23_42/0.03)] transition hover:-translate-y-0.5 hover:border-primary/20 hover:shadow-[0_14px_35px_rgb(15_23_42/0.08)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <div className="flex items-start gap-3">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/7 text-primary">
            {proposal.importJobId ? (
              <Bot className="size-4.5" aria-hidden />
            ) : (
              <CircleDashed className="size-4.5" aria-hidden />
            )}
          </span>
          <span className="min-w-0 flex-1">
            <span className="flex flex-wrap items-start justify-between gap-2">
              <span className="min-w-0">
                <span className="block text-sm font-semibold text-foreground">
                  {TARGET_LABEL[proposal.targetType]} · {targetIdentity}
                </span>
                <span className="mt-0.5 block truncate font-mono text-[11px] text-muted-foreground">
                  Proposal {proposal.id}
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
                    proposal.status === "applying" && "animate-spin",
                  )}
                  aria-hidden
                />
                {status.label}
              </span>
            </span>

            <span className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span className={risk.className}>{risk.label}</span>
              {proposal.importJobId ? (
                <>
                  <span aria-hidden>·</span>
                  <span>AI 导入提议</span>
                </>
              ) : null}
              <span aria-hidden>·</span>
              <time dateTime={proposal.updatedAt.toISOString()}>
                更新于 {DATE_FORMATTER.format(proposal.updatedAt)}
              </time>
              <ArrowRight
                className="ml-auto size-3.5 transition-transform group-hover:translate-x-0.5"
                aria-hidden
              />
            </span>
          </span>
        </div>
      </Link>
    </li>
  );
}

export function ProposalList() {
  const [filter, setFilter] = useState<Filter>("all");
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const { data, error, isLoading, isValidating, mutate, size, setSize } =
    useSWRInfinite<ProposalListPage>(
      (pageIndex, previousPage) => {
        if (!isAuthenticated) return null;
        if (pageIndex > 0 && !previousPage?.nextCursor) return null;
        return [
          "governance:owned-proposals",
          filter,
          pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
        ] as const;
      },
      ([, selectedFilter, cursor]) =>
        governanceApi().listProposals({
          cursor: typeof cursor === "string" && cursor ? cursor : undefined,
          pageSize: PAGE_SIZE,
          status:
            selectedFilter === "all"
              ? undefined
              : (selectedFilter as ProposalStatusEnum),
        }),
      {
        refreshInterval: (pages) =>
          pages?.some((page) =>
            page.items.some((proposal) =>
              ["submitted", "in_review", "applying"].includes(proposal.status),
            ),
          )
            ? 5000
            : 0,
        revalidateFirstPage: true,
      },
    );

  const proposals = data?.flatMap((page) => page.items) ?? [];
  const pageCount = data?.length ?? 0;
  const lastPage = pageCount > 0 ? data?.[pageCount - 1] : undefined;
  const reachedEnd = Boolean(data && !lastPage?.nextCursor);
  const loadingMore = isValidating && pageCount > 0 && size > pageCount;

  if (sessionLoading) {
    return (
      <div className="flex min-h-56 items-center justify-center rounded-2xl border border-dashed">
        <LoaderCircle
          className="size-5 animate-spin text-muted-foreground"
          aria-label="正在读取账户"
        />
      </div>
    );
  }

  if (!isAuthenticated) {
    return (
      <div className="rounded-2xl border border-dashed bg-muted/25 px-6 py-12 text-center">
        <Inbox className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后恢复你的全部提案</h2>
        <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">
          草稿、AI 导入结果、冲突和已生效记录都会长期保留在治理中心。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>登录并查看</Link>
        </Button>
      </div>
    );
  }

  return (
    <section className="min-w-0" aria-labelledby="my-proposals-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-xs font-semibold tracking-[0.16em] text-primary uppercase">
            Your proposals
          </p>
          <h2 id="my-proposals-title" className="mt-1 text-xl font-semibold">
            我的提案
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            从草稿到生效，每一步都可回来继续。
          </p>
        </div>
        <div className="flex gap-2">
          <Button asChild size="sm">
            <Link href="/governance/proposals/new">
              <Plus aria-hidden />
              新建提案
            </Link>
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={isValidating}
            onClick={() => void mutate()}
          >
            <RefreshCw
              className={cn("size-3.5", isValidating && "animate-spin")}
              aria-hidden
            />
            刷新
          </Button>
        </div>
      </div>

      <div
        className="mt-5 flex gap-1 overflow-x-auto rounded-xl bg-muted/70 p-1"
        aria-label="按状态筛选提案"
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
          <p className="font-medium text-destructive">提案暂时无法读取</p>
          <p className="mt-1 text-muted-foreground">
            请检查服务状态，或稍后重试。
          </p>
        </div>
      ) : null}

      {isLoading && !data ? (
        <div className="mt-4 space-y-3" aria-label="正在加载提案">
          {[0, 1, 2].map((item) => (
            <div
              key={item}
              className="h-28 animate-pulse rounded-2xl border bg-muted/45"
            />
          ))}
        </div>
      ) : null}

      {!isLoading && !error && proposals.length === 0 ? (
        <div className="mt-4 rounded-2xl border border-dashed bg-muted/20 px-6 py-12 text-center">
          <CircleDashed
            className="mx-auto size-8 text-muted-foreground"
            aria-hidden
          />
          <h3 className="mt-4 font-semibold">
            {filter === "all" ? "还没有提案" : "这个状态下没有提案"}
          </h3>
          <p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-muted-foreground">
            AI 导入形成的建议会自动出现在这里；页面编辑也可以创建可审核的变更。
          </p>
          <div className="mt-5 flex justify-center gap-2">
            <Button asChild>
              <Link href="/governance/proposals/new">创建人工提案</Link>
            </Button>
            <Button asChild variant="outline">
              <Link href="/imports">从资料导入</Link>
            </Button>
          </div>
        </div>
      ) : null}

      {proposals.length > 0 ? (
        <ol className="mt-4 space-y-3">
          {proposals.map((proposal) => (
            <ProposalRow key={proposal.id} proposal={proposal} />
          ))}
        </ol>
      ) : null}

      {data && !reachedEnd ? (
        <Button
          type="button"
          variant="outline"
          className="mt-4 w-full"
          disabled={loadingMore}
          onClick={() => void setSize(size + 1)}
        >
          {loadingMore ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : null}
          加载更多
        </Button>
      ) : null}
    </section>
  );
}
