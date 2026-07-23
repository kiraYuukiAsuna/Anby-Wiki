// /pages/[id]：按页面 ID 阅读（已解析 page_reference 的落地路由）。
import { notFound } from "next/navigation";

import { PageView } from "@/components/page-view";
import { fetchPageById } from "@/lib/reading";

export const dynamic = "force-dynamic";

export default async function PageByIdPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const result = await fetchPageById(id);

  if (result.kind === "not_found") {
    notFound();
  }
  if (result.kind === "gone") {
    return (
      <div className="mx-auto w-full max-w-3xl px-4 py-16 text-center">
        <h1 className="text-2xl font-semibold">页面已删除</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          该页面曾存在，但现已被删除。
        </p>
      </div>
    );
  }

  return <PageView data={result.data} />;
}
