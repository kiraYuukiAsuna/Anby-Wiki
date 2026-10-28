import { BookOpen } from "lucide-react";
import Link from "next/link";
import { notFound, redirect } from "next/navigation";
import { z } from "zod";

import { ResponseError } from "../../../../../contracts/generated/typescript";
import { knowledgeApi } from "@/lib/api";
import { serverApiOptions } from "@/lib/server-api";

export const dynamic = "force-dynamic";

// The ordinary anchor destination also resolves when copied, opened in a new
// tab, or followed before hydration. Normal body clicks use the inline popover.
export default async function ConceptArticlesPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ language?: string }>;
}) {
  const { id } = await params;
  if (!z.uuid().safeParse(id).success) notFound();
  const { language } = await searchParams;
  const client = knowledgeApi(await serverApiOptions());
  const result = await client.resolveEntityArticles({
    id,
    language: typeof language === "string" ? language : undefined,
  }).catch((error: unknown) => {
    if (error instanceof ResponseError && error.response.status === 404) notFound();
    throw error;
  });
  if (result.items.length === 1) redirect(`/pages/${result.items[0].pageId}`);

  return (
    <main className="mx-auto w-full max-w-2xl px-4 py-10">
      <h1 className="text-2xl font-semibold tracking-tight">
        {result.items.length ? "选择要阅读的条目" : "暂无可阅读条目"}
      </h1>
      {result.items.length ? (
        <>
          <p className="mt-3 text-sm text-muted-foreground">这个概念对应多篇条目，请选择阅读内容。</p>
          <nav aria-label="概念条目" className="mt-6 space-y-2">
            {result.items.map((item) => (
              <Link
                key={item.pageId}
                href={`/pages/${item.pageId}`}
                className="flex gap-3 rounded-xl border p-4 transition-colors hover:bg-accent"
              >
                <BookOpen aria-hidden className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
                <span className="min-w-0">
                  <span className="block break-words font-medium">{item.displayTitle}</span>
                  <span className="mt-1 block text-xs text-muted-foreground">{item.namespace} · {item.language}</span>
                </span>
              </Link>
            ))}
          </nav>
        </>
      ) : (
        <p className="mt-3 text-sm text-muted-foreground">这个概念目前没有你可以阅读的已发布条目。</p>
      )}
      <Link
        href={`/entities/${result.entityId}`}
        className="mt-6 inline-block text-sm text-primary hover:underline"
      >
        查看概念信息
      </Link>
    </main>
  );
}
