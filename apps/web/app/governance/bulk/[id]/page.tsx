import Link from "next/link";
import { ArrowLeft } from "lucide-react";

import { BulkReviewWorkspace } from "@/components/governance/bulk-review-workspace";
import { Button } from "@/components/ui/button";

export default async function BulkReviewDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <div className="mb-6">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/governance/bulk">
            <ArrowLeft aria-hidden />
            返回批量评审
          </Link>
        </Button>
      </div>
      <BulkReviewWorkspace id={id} />
    </div>
  );
}
