"use client";

import Link from "next/link";
import {
  ArrowRight,
  BookOpenText,
  Clock3,
  FileText,
  LoaderCircle,
  RefreshCw,
  SortAsc,
} from "lucide-react";
import { useState } from "react";
import useSWRInfinite from "swr/infinite";

import type { PageCatalogPage } from "../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { readingApi } from "@/lib/api";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 30;
type SortMode = "recent" | "title";

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
});

export function PageDirectory() {
  const [sort, setSort] = useState<SortMode>("recent");
  const state = useSWRInfinite<PageCatalogPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "published-page-directory",
        sort,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, selectedSort, cursor] = cacheKey as readonly [
        string,
        SortMode,
        string,
      ];
      return readingApi().listPublishedPages({
        sort: selectedSort,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
    { revalidateFirstPage: true },
  );

  const items = state.data?.flatMap((page) => page.items) ?? [];
  const total = state.data?.[0]?.total;
  const lastPage = state.data?.[state.data.length - 1];
  const canLoadMore = Boolean(lastPage?.nextCursor);
  const loadingMore = state.isValidating && Boolean(state.data);

  return (
    <section aria-labelledby="page-directory-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 id="page-directory-title" className="text-xl font-semibold">
            {typeof total === "number" ? `${total} 个已发布条目` : "已发布条目"}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            目录只包含已正式发布的内容页面；草稿与软删除页面不会出现。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex rounded-xl bg-muted p-1" aria-label="页面排序">
            <button
              type="button"
              aria-pressed={sort === "recent"}
              onClick={() => setSort("recent")}
              className={cn(
                "flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-muted-foreground",
                sort === "recent" && "bg-background text-foreground shadow-sm",
              )}
            >
              <Clock3 className="size-3.5" aria-hidden />最近更新
            </button>
            <button
              type="button"
              aria-pressed={sort === "title"}
              onClick={() => setSort("title")}
              className={cn(
                "flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-muted-foreground",
                sort === "title" && "bg-background text-foreground shadow-sm",
              )}
            >
              <SortAsc className="size-3.5" aria-hidden />按标题
            </button>
          </div>
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label="刷新页面目录"
            disabled={state.isValidating}
            onClick={() => void state.mutate()}
          >
            <RefreshCw className={cn("size-4", state.isValidating && "animate-spin")} aria-hidden />
          </Button>
        </div>
      </div>

      {state.error ? (
        <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
          页面目录暂时无法读取，请稍后重试。
        </div>
      ) : null}
      {state.isLoading && !state.data ? (
        <div className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2, 3, 4, 5].map((item) => <div key={item} className="h-28 animate-pulse rounded-2xl border bg-muted/35" />)}
        </div>
      ) : null}
      {!state.isLoading && !state.error && items.length === 0 ? (
        <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
          <BookOpenText className="mx-auto size-8 text-muted-foreground" aria-hidden />
          <h3 className="mt-4 font-semibold">还没有已发布页面</h3>
          <p className="mt-2 text-sm text-muted-foreground">发布第一个百科页面后，它会进入此目录。</p>
          <Button asChild className="mt-5"><Link href="/new">创建页面</Link></Button>
        </div>
      ) : null}
      {items.length ? (
        <ol className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((page) => (
            <li key={page.id}>
              <Link
                href={`/pages/${page.id}`}
                className="group flex h-full min-w-0 items-start gap-3 rounded-2xl border bg-card p-4 transition hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-[0_12px_32px_rgb(15_23_42/0.07)]"
              >
                <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary/8 text-primary"><FileText className="size-4" aria-hidden /></span>
                <span className="min-w-0 flex-1">
                  <span className="line-clamp-2 font-semibold leading-6 [overflow-wrap:anywhere]">{page.displayTitle}</span>
                  <span className="mt-2 block text-[11px] text-muted-foreground">
                    {DATE_FORMATTER.format(page.updatedAt)} · {page.language}
                  </span>
                </span>
                <ArrowRight className="mt-1 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-primary" aria-hidden />
              </Link>
            </li>
          ))}
        </ol>
      ) : null}
      {canLoadMore ? (
        <Button
          type="button"
          variant="outline"
          className="mt-5 w-full"
          disabled={loadingMore}
          onClick={() => void state.setSize(state.size + 1)}
        >
          {loadingMore ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          加载更多页面
        </Button>
      ) : null}
    </section>
  );
}
