// /pages/[id]/diff?from=&to=：两版结构 Diff（M2-T05）。
// from=base、to=current；from==to 时直接显示「两版相同」（不请求后端）。
import Link from "next/link";
import { notFound } from "next/navigation";

import { DiffView } from "@/components/history/diff-view";
import { shortId } from "@/components/history/utils";
import { Button } from "@/components/ui/button";
import { fetchRevisionDiff } from "@/lib/history";
import { fetchPageById } from "@/lib/reading";

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
          缺少 from/to 参数，请从
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

  // from == to 契约返回空 changes；前端直接短路，不发起请求。
  const diff =
    from === to
      ? { changes: [] }
      : await (async () => {
          const result = await fetchRevisionDiff(id, from, to);
          if (result.kind === "not_found") notFound();
          return result.data;
        })();

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">
            版本对比：{page.displayTitle}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            <span title={from}>{shortId(from)}</span>
            <span className="mx-1">→</span>
            <span title={to}>{shortId(to)}</span>
          </p>
        </div>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/pages/${page.id}/history`}>返回历史</Link>
        </Button>
      </header>
      <DiffView diff={diff} />
    </div>
  );
}
