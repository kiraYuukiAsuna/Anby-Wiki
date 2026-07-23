import { ReviewQueue } from "@/components/governance/review-queue";
import { governanceApi } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function ReviewQueuePage() {
  const initialData = await governanceApi().listReviewTasks({ pageSize: 100 });
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      <header>
        <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">
          Governance
        </p>
        <h1 className="mt-1 text-2xl font-bold tracking-tight">人工审核队列</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          高风险或不满足自动批准策略的提案必须由 human Actor 留下明确审核证据。
        </p>
      </header>
      <ReviewQueue initialData={initialData} />
    </div>
  );
}
