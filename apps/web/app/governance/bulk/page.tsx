import Link from "next/link";
import { ArrowLeft, Layers3 } from "lucide-react";

import { BulkReviewHub } from "@/components/governance/bulk-review-hub";
import { Button } from "@/components/ui/button";

export default function BulkReviewPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b border-border/75 pb-6">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            <Layers3 className="size-5" aria-hidden />
          </span>
          <h1 className="text-3xl font-semibold">批量评审</h1>
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
