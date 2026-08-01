import Link from "next/link";
import { ArrowLeft, Layers3 } from "lucide-react";

import { BulkReviewHub } from "@/components/governance/bulk-review-hub";
import { Button } from "@/components/ui/button";

export default function BulkReviewPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="grid gap-6 border-b border-border/75 pb-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
        <div className="max-w-3xl">
          <p className="flex items-center gap-2 text-xs font-semibold tracking-[0.18em] text-primary uppercase">
            <Layers3 className="size-3.5" aria-hidden />
            Bulk governance
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-[-0.045em]">
            批量评审工作台
          </h1>
          <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground">
            把待审提案冻结成可追踪批次，按风险抽样、逐项决策，再用固定
            wave 渐进应用。暂停、失败和每一次人工判断都会留下审计记录。
          </p>
        </div>
        <Button variant="outline" asChild>
          <Link href="/governance">
            <ArrowLeft aria-hidden />
            返回治理中心
          </Link>
        </Button>
      </header>
      <BulkReviewHub />
    </div>
  );
}
