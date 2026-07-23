// /wiki/[title]：按标题（或旧标题别名）阅读页面。
// title 为 URL 编码的页面标题（App Router 动态段，Next 自动解码）；
// 含斜杠的标题 v1 不支持（无 catch-all），进入带说明的 404 页。
import { notFound } from "next/navigation";

import { PageView } from "@/components/page-view";
import { fetchPageByTitle } from "@/lib/reading";

// 数据直连 Go API（渲染直连 ContentSnapshot，M3 才引入缓存投影），禁用静态化。
export const dynamic = "force-dynamic";

export default async function WikiTitlePage({
  params,
}: {
  params: Promise<{ title: string }>;
}) {
  const { title: rawTitle } = await params;

  // Next 16 的动态段 params 不做 URL 解码（与旧版行为不同），需自行解码；
  // 非法百分号编码按不存在处理。
  let title: string;
  try {
    title = decodeURIComponent(rawTitle);
  } catch {
    notFound();
  }

  // 含斜杠的标题暂不支持（编码斜杠 %2F 解码后在此拦截）。
  if (title.includes("/")) {
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
