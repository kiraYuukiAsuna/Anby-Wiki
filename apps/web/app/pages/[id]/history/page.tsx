// /pages/[id]/history：Revision 历史列表（M2-T05）。
// 服务端组件取第一页（page_size=20）注入客户端列表；翻页/对比跳转/回滚都在客户端完成。
import Link from "next/link";
import { notFound } from "next/navigation";

import { RevisionList } from "@/components/history/revision-list";
import { Button } from "@/components/ui/button";
import { fetchRevisionList } from "@/lib/history";
import { fetchPageById } from "@/lib/reading";

export const dynamic = "force-dynamic";

export default async function PageHistoryPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const [pageResult, listResult] = await Promise.all([
    fetchPageById(id),
    fetchRevisionList(id),
  ]);

  if (pageResult.kind !== "ok" || listResult.kind === "not_found") {
    notFound();
  }

  const { page } = pageResult.data;

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold tracking-tight">
          历史版本：{page.displayTitle}
        </h1>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/pages/${page.id}`}>返回阅读</Link>
        </Button>
      </header>
      <RevisionList
        pageId={page.id}
        currentRevisionId={page.currentRevisionId ?? null}
        initialPage={listResult.data}
      />
    </div>
  );
}
