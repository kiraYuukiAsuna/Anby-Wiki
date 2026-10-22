import Link from "next/link";
import { ArrowLeft, BadgeCheck } from "lucide-react";

import { ApplyQueue } from "@/components/governance/apply-queue";
import { Button } from "@/components/ui/button";

export default function ApplyQueuePage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex flex-wrap items-center justify-between gap-4 border-b pb-6">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-emerald-600 text-white shadow-lg shadow-emerald-600/20"><BadgeCheck className="size-5" aria-hidden /></span>
          <h1 className="text-3xl font-semibold">待应用</h1>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href="/governance/review"><ArrowLeft aria-hidden />审核队列</Link>
        </Button>
      </header>
      <ApplyQueue />
    </div>
  );
}
