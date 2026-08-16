"use client";

import Link from "next/link";
import {
  ArrowRight,
  Bot,
  CheckCircle2,
  CircleDashed,
  FileCheck2,
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  ShieldAlert,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";

import {
  ResponseError,
  type Proposal,
  type ProposalListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { governanceApi } from "@/lib/api";
import { LOGIN_PATH, useSession } from "@/lib/auth";
import { compactId } from "@/lib/display-id";
import {
  isIdentityConflictMessage,
  readGovernanceError,
} from "@/lib/governance-error";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 30;
const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

const TARGET_LABEL: Record<Proposal["targetType"], string> = {
  wiki: "百科导入批次",
  page: "百科页面",
  entity: "实体",
  claim: "事实声明",
  collection: "专题合集",
  external_resource: "外部资源",
};

const RISK_META: Record<Proposal["riskLevel"], { label: string; className: string }> = {
  low: { label: "低风险", className: "bg-emerald-50 text-emerald-700" },
  medium: { label: "中风险", className: "bg-amber-50 text-amber-700" },
  high: { label: "高风险", className: "bg-orange-50 text-orange-700" },
  critical: { label: "关键风险", className: "bg-rose-50 text-rose-700" },
};

function ApplyQueueItem({
  proposal,
  busy,
  applying,
  onApply,
}: {
  proposal: Proposal;
  busy: boolean;
  applying: boolean;
  onApply: (proposal: Proposal) => Promise<void>;
}) {
  const risk = RISK_META[proposal.riskLevel];
  return (
    <li className="rounded-2xl border bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)]">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-700">
          {proposal.importJobId ? <Bot className="size-4.5" aria-hidden /> : <FileCheck2 className="size-4.5" aria-hidden />}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="font-semibold">
                {TARGET_LABEL[proposal.targetType]} · {proposal.targetId ? compactId(proposal.targetId) : "新建目标"}
              </p>
              <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground">Proposal {proposal.id}</p>
            </div>
            <span className={cn("rounded-full px-2 py-1 text-[11px] font-medium", risk.className)}>{risk.label}</span>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>已通过审核</span>
            {proposal.importJobId ? <><span aria-hidden>·</span><span>AI 导入提议</span></> : null}
            <span aria-hidden>·</span>
            <time dateTime={proposal.updatedAt.toISOString()}>批准队列更新于 {DATE_FORMATTER.format(proposal.updatedAt)}</time>
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button variant="outline" size="sm" asChild>
            <Link href={`/governance/proposals/${proposal.id}`}>检查详情<ArrowRight aria-hidden /></Link>
          </Button>
          <Button size="sm" disabled={busy} onClick={() => void onApply(proposal)}>
            {applying ? <LoaderCircle className="animate-spin" aria-hidden /> : <CheckCircle2 aria-hidden />}
            原子应用
          </Button>
        </div>
      </div>
    </li>
  );
}

export function ApplyQueue() {
  const session = useSession();
  const [applyingId, setApplyingId] = useState<string | null>(null);
  const state = useSWRInfinite<ProposalListPage>(
    (pageIndex, previousPage) => {
      if (!session.isAuthenticated) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return ["governance:apply-queue", pageIndex === 0 ? "" : (previousPage?.nextCursor ?? "")] as const;
    },
    (cacheKey) => {
      const [, cursor] = cacheKey as readonly [string, string];
      return governanceApi().listPendingApplyProposals({
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
    { refreshInterval: 10000, revalidateFirstPage: true },
  );
  const proposals = state.data?.flatMap((page) => page.items) ?? [];
  const lastPage = state.data?.[state.data.length - 1];

  const apply = async (proposal: Proposal) => {
    setApplyingId(proposal.id);
    try {
      const result = await governanceApi().applyProposal({ id: proposal.id });
      await state.mutate();
      toast.success(result.idempotent ? "该提案已经生效" : "提案已原子生效", {
        description: `ChangeBatch ${compactId(result.changeBatchId)} 已写入审计账本${result.revisionIds.length ? `，发布 ${result.revisionIds.length} 个页面 Revision` : ""}。`,
      });
    } catch (error) {
      const detail = await readGovernanceError(error);
      if (detail?.status === 409) {
        await state.mutate();
        const identityConflict = isIdentityConflictMessage(detail.message);
        toast.error(identityConflict ? "提案已过期，未执行任何写入" : "原子应用未执行", {
          description:
            detail.message ??
            "对象基线已经变化，请打开提案检查冲突后再决定如何处理。",
        });
      } else if (detail?.status === 403) {
        toast.error("当前账号没有应用权限");
      } else {
        toast.error("原子应用失败", {
          description: detail?.message ?? "服务暂时不可用，请记录时间并联系管理员。",
        });
      }
    } finally {
      setApplyingId(null);
    }
  };

  if (session.isLoading) {
    return <div className="flex min-h-56 items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" aria-hidden />正在确认应用权限…</div>;
  }
  if (!session.isAuthenticated) {
    return (
      <div className="rounded-2xl border border-dashed bg-muted/25 px-6 py-12 text-center">
        <LockKeyhole className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后查看待应用队列</h2>
        <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">这里包含审核已通过、但尚未写入权威知识的 Proposal。</p>
        <Button asChild className="mt-5"><Link href={LOGIN_PATH}>前往登录</Link></Button>
      </div>
    );
  }
  if (
    state.error instanceof ResponseError &&
    state.error.response.status === 403
  ) {
    return (
      <div className="rounded-2xl border border-amber-200 bg-amber-50/65 p-8">
        <ShieldAlert className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">需要应用者权限</h2>
        <p className="mt-2 text-sm leading-6 text-amber-900/70">只有 applier 或管理员能够查看并执行原子应用。</p>
      </div>
    );
  }
  if (state.error) {
    return <p className="rounded-2xl border border-destructive/30 p-5 text-sm text-destructive">待应用队列加载失败。</p>;
  }

  return (
    <section aria-labelledby="apply-queue-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-xs font-semibold tracking-[0.16em] text-primary uppercase">Approved proposals</p>
          <h2 id="apply-queue-title" className="mt-1 text-xl font-semibold">等待正式生效</h2>
          <p className="mt-1 text-sm text-muted-foreground">一次操作会在同一事务中执行全部有序 Operation，并记录为一个 ChangeBatch；任何一步失败，整批都不会提交。</p>
        </div>
        <Button variant="ghost" size="sm" disabled={state.isValidating} onClick={() => void state.mutate()}>
          <RefreshCw className={cn("size-3.5", state.isValidating && "animate-spin")} aria-hidden />刷新
        </Button>
      </div>
      {state.isLoading && !state.data ? (
        <div className="mt-5 space-y-3">{[0, 1, 2].map((item) => <div key={item} className="h-32 animate-pulse rounded-2xl border bg-muted/35" />)}</div>
      ) : null}
      {!state.isLoading && proposals.length === 0 ? (
        <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
          <CircleDashed className="mx-auto size-8 text-muted-foreground" aria-hidden />
          <h3 className="mt-4 font-semibold">当前没有待应用提案</h3>
          <p className="mt-2 text-sm text-muted-foreground">审核通过的 Proposal 会自动进入这里，不需要再回头寻找原导入任务。</p>
          <Button asChild variant="outline" className="mt-5"><Link href="/governance/review">打开审核队列</Link></Button>
        </div>
      ) : null}
      {proposals.length ? (
        <ol className="mt-5 space-y-3">
          {proposals.map((proposal) => (
            <ApplyQueueItem
              key={proposal.id}
              proposal={proposal}
              busy={applyingId !== null}
              applying={applyingId === proposal.id}
              onApply={apply}
            />
          ))}
        </ol>
      ) : null}
      {lastPage?.nextCursor ? (
        <Button
          variant="outline"
          className="mt-5 w-full"
          disabled={state.isValidating}
          onClick={() => void state.setSize(state.size + 1)}
        >
          {state.isValidating ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          加载更多
        </Button>
      ) : null}
    </section>
  );
}
