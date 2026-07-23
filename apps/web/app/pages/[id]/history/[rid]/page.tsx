// /pages/[id]/history/[rid]：单版详情（M2-T05）。
// Revision 元信息 + 该版 canonical AST 只读渲染 + 基于此版本回滚入口。
import Link from "next/link";
import { notFound } from "next/navigation";

import { RevisionDetailView } from "@/components/history/revision-detail-view";
import { Button } from "@/components/ui/button";
import { fetchRevisionDetail } from "@/lib/history";
import { fetchPageById } from "@/lib/reading";

export const dynamic = "force-dynamic";

export default async function RevisionDetailPage({
  params,
}: {
  params: Promise<{ id: string; rid: string }>;
}) {
  const { id, rid } = await params;
  const [pageResult, detailResult] = await Promise.all([
    fetchPageById(id),
    fetchRevisionDetail(id, rid),
  ]);

  if (pageResult.kind !== "ok" || detailResult.kind === "not_found") {
    notFound();
  }

  const { page } = pageResult.data;

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold tracking-tight">
          历史版本：{page.displayTitle}
        </h1>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" asChild>
            <Link href={`/pages/${page.id}/history`}>返回历史</Link>
          </Button>
          <Button variant="outline" size="sm" asChild>
            <Link href={`/pages/${page.id}`}>返回阅读</Link>
          </Button>
        </div>
      </header>
      <RevisionDetailView
        pageId={page.id}
        detail={detailResult.data}
        currentRevisionId={page.currentRevisionId ?? null}
      />
    </div>
  );
}
