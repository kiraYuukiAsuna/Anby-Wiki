"use client";

import Link from "next/link";
import {
  ArrowRight,
  BookOpenText,
  Clock3,
  FileText,
  Globe2,
  Layers3,
  Search,
  SquarePen,
  Waypoints,
} from "lucide-react";
import useSWR from "swr";

import { Button } from "@/components/ui/button";
import { WikiSearchForm } from "@/components/wiki-search-form";
import { WorkspaceLink } from "@/components/workspace-link";
import { collectionsApi, knowledgeApi, readingApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { workspaceAccessNotice } from "@/lib/workspace-access";

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  month: "short",
  day: "numeric",
});

export function WikiHome() {
  const { session } = useSession();
  const wikiID = session?.wikiId;
  const wikiName = session?.wikiName ?? "Anby Wiki";
  const canReadCollections = !workspaceAccessNotice("/collections", session);
  const pages = useSWR(wikiID ? ["wiki-home:recent-pages", session?.actorId, wikiID] : null, () =>
    readingApi().listPublishedPages({ sort: "recent", pageSize: 12 }),
  );
  const collections = useSWR(wikiID && canReadCollections ? ["wiki-home:collections", session?.actorId, wikiID] : null, () =>
    collectionsApi().listCollections({ pageSize: 6 }),
  );
  const entities = useSWR(wikiID ? ["wiki-home:entities", session?.actorId, wikiID] : null, () =>
    knowledgeApi().listEntities({ status: "active", pageSize: 8 }),
  );

  return (
    <div className="w-full">
      <section className="relative isolate border-b border-border/70">
        <div className="wiki-grid pointer-events-none absolute inset-0 -z-10 opacity-40" aria-hidden />
        <div className="mx-auto grid w-full max-w-7xl grid-cols-1 gap-8 px-5 py-10 lg:grid-cols-[minmax(0,1fr)_20rem] lg:px-8 lg:py-12">
          <div className="max-w-3xl">
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex h-7 items-center gap-1.5 rounded-lg border bg-background/80 px-2.5 text-xs font-medium text-muted-foreground">
                <Globe2 className="size-3.5" aria-hidden />
                当前 Wiki
              </span>
              <h1 className="min-w-0 max-w-full truncate text-3xl font-semibold" title={wikiName}>
                {wikiName}
              </h1>
            </div>
            <WikiSearchForm id="wiki-home-search" className="mt-6" />
            <div className="mt-4 flex flex-wrap gap-2">
              <Button variant="ghost" size="sm" asChild>
                <Link href="/pages"><BookOpenText aria-hidden />页面目录</Link>
              </Button>
              <Button variant="ghost" size="sm" asChild>
                <WorkspaceLink href="/collections"><Layers3 aria-hidden />专题合集</WorkspaceLink>
              </Button>
              <Button variant="ghost" size="sm" asChild>
                <Link href="/explore"><Search aria-hidden />搜索</Link>
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
              {canReadCollections ? <div className="rounded-xl bg-muted/55 p-3">
                <p className="font-semibold tabular-nums">{collections.data?.items.length ?? "—"}</p>
                <p className="mt-1 text-[10px] text-muted-foreground">专题</p>
              </div> : null}
              <div className="rounded-xl bg-muted/55 p-3">
                <p className="font-semibold tabular-nums">{entities.data?.items.length ?? "—"}</p>
                <p className="mt-1 text-[10px] text-muted-foreground">实体</p>
              </div>
            </div>
            <Button asChild variant="outline" className="mt-4 w-full">
              <WorkspaceLink href="/new"><SquarePen aria-hidden />创建页面</WorkspaceLink>
            </Button>
          </aside>
        </div>
      </section>

      <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
        <section aria-labelledby="recent-pages-title">
          <div className="flex items-end justify-between gap-4">
            <h2 id="recent-pages-title" className="text-2xl font-semibold">最近更新</h2>
            <Button variant="outline" size="sm" asChild>
              <Link href="/pages">页面目录<ArrowRight aria-hidden /></Link>
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
              <Button asChild className="mt-5"><WorkspaceLink href="/new">创建第一页</WorkspaceLink></Button>
            </div>
          )}
        </section>

        <div className="mt-12 grid gap-8 border-t pt-10 lg:grid-cols-[minmax(0,1fr)_22rem]">
          {canReadCollections ? <section aria-labelledby="collections-title">
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
          </section> : null}

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
