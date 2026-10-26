import Link from "next/link";
import {
  Archive,
  ArrowRight,
  BadgeCheck,
  BrainCircuit,
  Gavel,
  LockKeyhole,
  Plus,
  ShieldAlert,
  ScrollText,
} from "lucide-react";

import { ProposalList } from "@/components/governance/proposal-list";
import { PlatformHealthCard } from "@/components/governance/platform-health-card";
import { Button } from "@/components/ui/button";

export default function GovernancePage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b border-border/75 pb-6">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            <Gavel className="size-5" aria-hidden />
          </span>
          <h1 className="text-3xl font-semibold">治理中心</h1>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="outline">
            <Link href="/governance/review">审核队列</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/governance/apply">
              <BadgeCheck aria-hidden />
              待应用
            </Link>
          </Button>
          <Button asChild>
            <Link href="/governance/proposals/new">
              <Plus aria-hidden />
              新建提案
            </Link>
          </Button>
        </div>
      </header>

      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-8 pt-8 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <ProposalList />
        <aside className="min-w-0 space-y-3 lg:sticky lg:top-24 lg:self-start">
          <PlatformHealthCard />
          <Link
            href="/governance/apply"
            className="group flex items-center justify-between rounded-2xl border border-emerald-500/20 bg-emerald-500/5 p-4 text-sm font-medium transition hover:border-emerald-500/35 hover:bg-emerald-500/10"
          >
            <span className="flex items-center gap-2">
              <BadgeCheck className="size-4 text-emerald-700" aria-hidden />
              待原子应用
            </span>
            <ArrowRight className="size-4 transition-transform group-hover:translate-x-0.5" aria-hidden />
          </Link>
          <Link
            href="/imports"
            className="group flex items-center justify-between rounded-2xl border border-border/75 bg-muted/35 p-4 text-sm font-medium transition hover:border-primary/20 hover:bg-primary/5"
          >
            查看 AI 导入队列
            <ArrowRight
              className="size-4 transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </Link>
          <Link
            href="/governance/activity"
            className="group flex items-center justify-between rounded-2xl border border-border/75 bg-muted/35 p-4 text-sm font-medium transition hover:border-primary/20 hover:bg-primary/5"
          >
            <span className="flex items-center gap-2">
              <ScrollText className="size-4 text-primary" aria-hidden />
              审计与变更标签
            </span>
            <ArrowRight
              className="size-4 transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </Link>
          <Link
            href="/governance/protections"
            className="group flex items-center justify-between rounded-2xl border border-border/75 bg-muted/35 p-4 text-sm font-medium transition hover:border-primary/20 hover:bg-primary/5"
          >
            <span className="flex items-center gap-2">
              <LockKeyhole className="size-4 text-primary" aria-hidden />
              页面保护策略
            </span>
            <ArrowRight
              className="size-4 transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </Link>
          <Link
            href="/governance/fact-check"
            className="group flex items-center justify-between rounded-2xl border border-border/75 bg-muted/35 p-4 text-sm font-medium transition hover:border-primary/20 hover:bg-primary/5"
          >
            <span className="flex items-center gap-2">
              <ShieldAlert className="size-4 text-primary" aria-hidden />
              事实一致性检查
            </span>
            <ArrowRight
              className="size-4 transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </Link>
          <Link
            href="/governance/ai-trust"
            className="group flex items-center justify-between rounded-2xl border border-border/75 bg-muted/35 p-4 text-sm font-medium transition hover:border-primary/20 hover:bg-primary/5"
          >
            <span className="flex items-center gap-2">
              <BrainCircuit className="size-4 text-primary" aria-hidden />
              AI 信任与抽样
            </span>
            <ArrowRight
              className="size-4 transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </Link>
          <Link
            href="/governance/revision-storage"
            className="group flex items-center justify-between rounded-2xl border border-border/75 bg-muted/35 p-4 text-sm font-medium transition hover:border-primary/20 hover:bg-primary/5"
          >
            <span className="flex items-center gap-2">
              <Archive className="size-4 text-primary" aria-hidden />
              历史版本存储
            </span>
            <ArrowRight
              className="size-4 transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </Link>
        </aside>
      </div>
    </div>
  );
}
