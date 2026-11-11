import Link from "next/link";
import { notFound } from "next/navigation";

import { RevisionComparison } from "@/components/history/revision-comparison";
import { Button } from "@/components/ui/button";
import { fetchRevisionDetail, fetchRevisionDiff } from "@/lib/history";
import { fetchPageById } from "@/lib/reading";
import { serverApiOptions } from "@/lib/server-api";

export const dynamic = "force-dynamic";

export default async function PageDiffPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ from?: string; to?: string }>;
}) {
  const { id } = await params;
  const { from, to } = await searchParams;

  const pageResult = await fetchPageById(id);
  if (pageResult.kind !== "ok") {
    notFound();
  }
  const { page } = pageResult.data;

  if (!from || !to) {
    return (
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 px-4 py-8">
        <h1 className="text-2xl font-bold tracking-tight">
          版本对比：{page.displayTitle}
        </h1>
        <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
          请选择两个版本进行对比，请从
          <Link
            href={`/pages/${page.id}/history`}
            className="mx-1 text-blue-600 hover:underline"
          >
            历史版本
          </Link>
          选择两个版本进行对比。
        </p>
      </div>
    );
  }

  const options = await serverApiOptions();
  const [diffResult, beforeResult, afterResult] = await Promise.all([
    fetchRevisionDiff(id, from, to, options),
    fetchRevisionDetail(id, from, options, true),
    fetchRevisionDetail(id, to, options, true),
  ]);
  if (diffResult.kind === "not_found" || beforeResult.kind === "not_found" || afterResult.kind === "not_found") notFound();

  return (
    <div className="mx-auto flex w-full max-w-[1600px] flex-col gap-6 px-4 py-8">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">
            版本对比：{page.displayTitle}
          </h1>

        </div>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/pages/${page.id}/history`}>返回历史</Link>
        </Button>
      </header>
      <RevisionComparison key={`${from}:${to}`} pageId={id} diff={diffResult.data} before={beforeResult.data} after={afterResult.data} />
    </div>
  );
}
