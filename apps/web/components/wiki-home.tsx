"use client";

import Link from "next/link";
import {
  ArrowRight,
  BookOpenText,
  Clock3,
  Compass,
  FileText,
  Layers3,
  Search,
  SquarePen,
  Waypoints,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { collectionsApi, knowledgeApi, readingApi } from "@/lib/api";

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  month: "short",
  day: "numeric",
});
const homepageSearchSchema = z.string().trim().min(1).max(255);

export function WikiHome() {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const pages = useSWR("wiki-home:recent-pages", () =>
    readingApi().listPublishedPages({ sort: "recent", pageSize: 12 }),
  );
  const collections = useSWR("wiki-home:collections", () =>
    collectionsApi().listCollections({ pageSize: 6 }),
  );
  const entities = useSWR("wiki-home:entities", () =>
    knowledgeApi().listEntities({ status: "active", pageSize: 8 }),
  );

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = homepageSearchSchema.safeParse(query);
    if (!parsed.success) return;
    router.push(`/explore?q=${encodeURIComponent(parsed.data)}`);
  };

  return (
    <div className="w-full">
      <section className="relative isolate border-b border-border/70">
        <div className="wiki-grid pointer-events-none absolute inset-0 -z-10 opacity-40" aria-hidden />
        <div className="mx-auto grid w-full max-w-7xl gap-8 px-5 py-10 lg:grid-cols-[minmax(0,1fr)_20rem] lg:px-8 lg:py-12">
          <div className="max-w-3xl">
            <h1 className="text-3xl font-semibold">Anby Wiki</h1>
            <form
              onSubmit={submitSearch}
              className="mt-6 flex max-w-2xl gap-2 rounded-[8px] border bg-background p-2 shadow-sm"
              role="search"
            >
              <label htmlFor="wiki-home-search" className="sr-only">搜索百科</label>
              <Search className="ml-2 size-5 shrink-0 self-center text-muted-foreground" aria-hidden />
              <input
                id="wiki-home-search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                maxLength={255}
                placeholder="搜索人物、作品、概念或事实…"
                className="min-w-0 flex-1 bg-transparent px-1 text-sm outline-none placeholder:text-muted-foreground"
              />
              <Button type="submit" disabled={!homepageSearchSchema.safeParse(query).success}>
                搜索
                <ArrowRight aria-hidden />
              </Button>
            </form>
            <div className="mt-4 flex flex-wrap gap-2">
              <Button variant="ghost" size="sm" asChild>
                <Link href="/pages"><BookOpenText aria-hidden />全部页面</Link>
              </Button>
              <Button variant="ghost" size="sm" asChild>
                <Link href="/collections"><Layers3 aria-hidden />专题合集</Link>
              </Button>
              <Button variant="ghost" size="sm" asChild>
                <Link href="/explore"><Compass aria-hidden />高级探索</Link>
              </Button>
            </div>
          </div>

          <aside className="self-end rounded-[1.75rem] border bg-card/80 p-5 shadow-[0_24px_70px_rgb(15_23_42/0.08)] backdrop-blur">
            <div className="flex items-center gap-3">
              <span className="flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <BookOpenText className="size-5" aria-hidden />
              </span>
              <div>
                <p className="text-2xl font-semibold tabular-nums">{pages.data?.total ?? "—"}</p>
                <p className="text-xs text-muted-foreground">已发布页面</p>
              </div>
            </div>
            <div className="mt-5 grid grid-cols-2 gap-2 border-t pt-4 text-center">
              <div className="rounded-xl bg-muted/55 p-3">
                <p className="font-semibold tabular-nums">{collections.data?.items.length ?? "—"}</p>
                <p className="mt-1 text-[10px] text-muted-foreground">专题</p>
              </div>
              <div className="rounded-xl bg-muted/55 p-3">
                <p className="font-semibold tabular-nums">{entities.data?.items.length ?? "—"}</p>
                <p className="mt-1 text-[10px] text-muted-foreground">实体</p>
              </div>
            </div>
            <Button asChild variant="outline" className="mt-4 w-full">
              <Link href="/new"><SquarePen aria-hidden />创建页面</Link>
            </Button>
          </aside>
        </div>
      </section>

      <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
        <section aria-labelledby="recent-pages-title">
          <div className="flex items-end justify-between gap-4">
            <h2 id="recent-pages-title" className="text-2xl font-semibold">最近更新</h2>
            <Button variant="outline" size="sm" asChild>
              <Link href="/pages">浏览全部<ArrowRight aria-hidden /></Link>
            </Button>
          </div>

          {pages.isLoading ? (
            <div className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {[0, 1, 2, 3, 4, 5].map((item) => <div key={item} className="h-32 animate-pulse rounded-2xl border bg-muted/35" />)}
            </div>
          ) : pages.error ? (
            <p className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">页面目录暂时无法读取。</p>
          ) : pages.data?.items.length ? (
            <ol className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {pages.data.items.map((page) => (
                <li key={page.id}>
                  <Link
                    href={`/pages/${page.id}`}
                    className="group flex h-full min-w-0 items-start gap-3 rounded-2xl border bg-card p-4 transition hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-[0_12px_32px_rgb(15_23_42/0.07)]"
                  >
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary/8 text-primary"><FileText className="size-4" aria-hidden /></span>
                    <span className="min-w-0 flex-1">
                      <span className="line-clamp-2 font-semibold leading-6 [overflow-wrap:anywhere]">{page.displayTitle}</span>
                      <span className="mt-2 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                        <Clock3 className="size-3" aria-hidden />
                        {DATE_FORMATTER.format(page.updatedAt)} 更新
                        <span aria-hidden>·</span>{page.language}
                      </span>
                    </span>
                    <ArrowRight className="mt-1 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-primary" aria-hidden />
                  </Link>
                </li>
              ))}
            </ol>
          ) : (
            <div className="mt-5 rounded-2xl border border-dashed px-6 py-12 text-center">
              <BookOpenText className="mx-auto size-8 text-muted-foreground" aria-hidden />
              <h3 className="mt-4 font-semibold">还没有已发布页面</h3>
              <Button asChild className="mt-5"><Link href="/new">创建第一页</Link></Button>
            </div>
          )}
        </section>

        <div className="mt-12 grid gap-8 border-t pt-10 lg:grid-cols-[minmax(0,1fr)_22rem]">
          <section aria-labelledby="collections-title">
            <div className="flex items-end justify-between gap-4">
              <h2 id="collections-title" className="text-xl font-semibold">专题合集</h2>
              <Link href="/collections" className="text-sm font-medium text-primary hover:underline">全部专题</Link>
            </div>
            {collections.data?.items.length ? (
              <ul className="mt-4 grid gap-3 sm:grid-cols-2">
                {collections.data.items.map((collection) => (
                  <li key={collection.id}>
                    <Link href={`/collections/${collection.id}`} className="group flex min-w-0 items-center gap-3 rounded-2xl border bg-card p-4 hover:border-primary/25">
                      <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-violet-100 text-violet-700"><Layers3 className="size-4" aria-hidden /></span>
                      <span className="min-w-0 flex-1 truncate font-medium">{collection.title}</span>
                      <ArrowRight className="size-4 shrink-0 text-muted-foreground group-hover:text-primary" aria-hidden />
                    </Link>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="mt-4 rounded-2xl border border-dashed p-6 text-sm text-muted-foreground">暂无专题</p>
            )}
          </section>

          <section aria-labelledby="entities-title" className="rounded-2xl border bg-card p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h2 id="entities-title" className="font-semibold">实体</h2>
              </div>
              <Waypoints className="size-5 text-primary" aria-hidden />
            </div>
            <ul className="mt-4 divide-y">
              {(entities.data?.items ?? []).slice(0, 6).map((entity) => (
                <li key={entity.id}>
                  <Link href={`/entities/${entity.id}`} className="flex min-w-0 items-center gap-3 py-3 text-sm hover:text-primary">
                    <span className="min-w-0 flex-1 truncate font-medium">{entity.displayLabel}</span>
                    <span className="shrink-0 text-[10px] text-muted-foreground">{entity.entityType.name}</span>
                  </Link>
                </li>
              ))}
            </ul>
            <Button asChild variant="outline" className="mt-3 w-full"><Link href="/entities">全部实体</Link></Button>
          </section>
        </div>
      </div>
    </div>
  );
}
