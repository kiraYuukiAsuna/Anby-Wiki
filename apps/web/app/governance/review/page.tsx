import Link from "next/link";

import { ReviewQueue } from "@/components/governance/review-queue";
import { Button } from "@/components/ui/button";

export default function ReviewQueuePage() {
  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-5 py-10 lg:px-8 lg:py-12">
      <header className="flex flex-wrap items-end justify-between gap-4 border-b border-border/75 pb-7">
        <div>
          <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">
            Governance
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-0.035em]">
            人工审核队列
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            高风险或不满足自动批准策略的提案必须由 human Actor
            留下明确审核证据。
          </p>
        </div>
        <Button variant="outline" size="sm" asChild>
          <Link href="/governance">返回治理中心</Link>
        </Button>
      </header>
      <ReviewQueue />
    </div>
  );
}
