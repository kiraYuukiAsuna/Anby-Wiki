import Link from "next/link";
import {
  Archive,
  ArrowRight,
  BadgeCheck,
  Bot,
  BrainCircuit,
  FileCheck2,
  GitCompareArrows,
  Layers3,
  LockKeyhole,
  Plus,
  ShieldCheck,
  ShieldAlert,
  ScrollText,
} from "lucide-react";

import { ProposalList } from "@/components/governance/proposal-list";
import { PlatformHealthCard } from "@/components/governance/platform-health-card";
import { Button } from "@/components/ui/button";

export default function GovernancePage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="grid gap-8 border-b border-border/75 pb-10 lg:grid-cols-[minmax(0,1fr)_23rem] lg:items-end">
        <div className="max-w-3xl">
          <p className="text-xs font-semibold tracking-[0.18em] text-primary uppercase">
            Governance center
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-[-0.045em] text-balance sm:text-5xl">
            让每次知识变更，
            <span className="text-primary">都清楚地走到结果。</span>
          </h1>
          <p className="mt-5 max-w-2xl text-base leading-8 text-muted-foreground">
            在一个长期工作台里恢复草稿、追踪 AI
            提议、处理冲突并查看最终状态。正式知识只会在规则与审核通过后更新。
          </p>
        </div>
        <div className="rounded-2xl border border-primary/15 bg-primary/[0.045] p-5">
          <div className="flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <ShieldCheck className="size-5" aria-hidden />
            </span>
            <div>
              <p className="font-semibold">等待人工判断</p>
              <p className="text-xs text-muted-foreground">
                面向有审核权限的共建者
              </p>
            </div>
          </div>
          <div className="mt-5 grid gap-2">
            <Button asChild>
              <Link href="/governance/review">
                打开审核队列
                <ArrowRight aria-hidden />
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link href="/governance/apply">
                <BadgeCheck aria-hidden />
                待原子应用
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link href="/governance/proposals/new">
                <Plus aria-hidden />
                新建人工提案
              </Link>
            </Button>
          </div>
        </div>
      </header>

      <section className="grid gap-3 py-8 sm:grid-cols-3">
        {[
          {
            label: "机器只提议",
            detail: "AI 与批量任务不能绕过 Proposal 直接发布。",
            icon: Bot,
          },
          {
            label: "差异可预览",
            detail: "同时比较基线、当前线上与拟应用结果。",
            icon: GitCompareArrows,
          },
          {
            label: "结果可追溯",
            detail: "审核证据、冲突选择与 ChangeBatch 全程留痕。",
            icon: FileCheck2,
          },
        ].map((item) => {
          const Icon = item.icon;
          return (
            <div
              key={item.label}
              className="rounded-2xl border border-border/75 bg-card/75 p-4"
            >
              <Icon className="size-4 text-primary" aria-hidden />
              <p className="mt-3 text-sm font-semibold">{item.label}</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {item.detail}
              </p>
            </div>
          );
        })}
      </section>

      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-8 border-t border-border/75 pt-8 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <ProposalList />
        <aside className="min-w-0 space-y-3 lg:sticky lg:top-24 lg:self-start">
          <PlatformHealthCard />
          <div className="rounded-2xl border bg-card p-4">
            <p className="text-sm font-semibold">下一步怎么走？</p>
            <ol className="mt-3 space-y-3 text-xs leading-5 text-muted-foreground">
              <li className="flex gap-2">
                <span className="font-semibold text-primary">01</span>
                打开提案，检查目标、风险与证据。
              </li>
              <li className="flex gap-2">
                <span className="font-semibold text-primary">02</span>
                在三态预览里确认修改，不覆盖他人新编辑。
              </li>
              <li className="flex gap-2">
                <span className="font-semibold text-primary">03</span>
                审核通过后应用；出现冲突则逐项解决。
              </li>
            </ol>
          </div>
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
            href="/governance/bulk"
            className="group flex items-center justify-between rounded-2xl border border-border/75 bg-muted/35 p-4 text-sm font-medium transition hover:border-primary/20 hover:bg-primary/5"
          >
            <span className="flex items-center gap-2">
              <Layers3 className="size-4 text-primary" aria-hidden />
              批量评审工作台
            </span>
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
