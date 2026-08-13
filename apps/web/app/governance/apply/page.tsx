import Link from "next/link";
import { ArrowRight, BadgeCheck, ShieldCheck } from "lucide-react";

import { ApplyQueue } from "@/components/governance/apply-queue";
import { Button } from "@/components/ui/button";

export default function ApplyQueuePage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 grid gap-6 border-b pb-8 lg:grid-cols-[minmax(0,1fr)_20rem] lg:items-end">
        <div className="max-w-3xl">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-emerald-600 text-white shadow-lg shadow-emerald-600/20"><BadgeCheck className="size-5" aria-hidden /></span>
          <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-emerald-700 uppercase">Atomic apply queue</p>
          <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em] sm:text-5xl">待原子应用</h1>
          <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">审核批准只表示证据与策略门槛已满足；在这里明确执行后，Proposal 才会以独立 ChangeBatch 正式写入。</p>
        </div>
        <div className="rounded-2xl border border-emerald-500/20 bg-emerald-500/5 p-4">
          <p className="flex items-center gap-2 text-sm font-semibold"><ShieldCheck className="size-4 text-emerald-700" aria-hidden />事务保证</p>
          <p className="mt-2 text-xs leading-5 text-muted-foreground">页面、实体、Claim、引用和审计要么全部成功，要么全部保持原状。</p>
          <Button asChild variant="outline" size="sm" className="mt-4 w-full bg-background"><Link href="/governance/review">返回审核队列<ArrowRight aria-hidden /></Link></Button>
        </div>
      </header>
      <ApplyQueue />
    </div>
  );
}
