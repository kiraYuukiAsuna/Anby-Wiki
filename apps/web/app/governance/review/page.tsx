import Link from "next/link";
import { BadgeCheck, Plus } from "lucide-react";

import { BulkReviewHub } from "@/components/governance/bulk-review-hub";
import { ReviewQueue } from "@/components/governance/review-queue";
import { Button } from "@/components/ui/button";

export default function ReviewQueuePage() {
  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-5 py-10 lg:px-8 lg:py-12">
      <header className="flex flex-wrap items-center justify-between gap-4 border-b border-border/75 pb-6">
        <h1 className="text-3xl font-semibold">人工审核队列</h1>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" asChild>
            <Link href="/governance/apply">
              <BadgeCheck aria-hidden />
              待应用
            </Link>
          </Button>
          <Button size="sm" asChild>
            <Link href="/governance/proposals/new">
              <Plus aria-hidden />
              新建提案
            </Link>
          </Button>
        </div>
      </header>
      <ReviewQueue />
      <BulkReviewHub />
    </div>
  );
}
