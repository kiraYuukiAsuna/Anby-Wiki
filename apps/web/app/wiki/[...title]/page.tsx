// /wiki/[...title]：按标题（或旧标题别名）阅读页面。catch-all 同时
// 接受编码斜杠与多段 URL，因此页面标题可以包含「/」。
import { notFound } from "next/navigation";

import { PageView } from "@/components/page-view";
import { fetchPageByTitle } from "@/lib/reading";

// 数据直连 Go API（渲染直连 ContentSnapshot，M3 才引入缓存投影），禁用静态化。
export const dynamic = "force-dynamic";

export default async function WikiTitlePage({
  params,
}: {
  params: Promise<{ title: string[] }>;
}) {
  const { title: rawSegments } = await params;

  // Next 16 在本项目运行模式下保留 params 的百分号编码；逐段解码后再用
  // 「/」还原领域标题。直接访问 /wiki/a/b 与 /wiki/a%2Fb 等价。
  let title: string;
  try {
    title = rawSegments.map((segment) => decodeURIComponent(segment)).join("/");
  } catch {
    notFound();
  }
  if (!title) {
    notFound();
  }

  const result = await fetchPageByTitle(title);

  if (result.kind === "not_found") {
    notFound();
  }
  if (result.kind === "gone") {
    return (
      <div className="mx-auto w-full max-w-3xl px-4 py-16 text-center">
        <h1 className="text-2xl font-semibold">页面已删除</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          「{title}」曾存在，但现已被删除。
        </p>
      </div>
    );
  }

  return <PageView data={result.data} />;
}
