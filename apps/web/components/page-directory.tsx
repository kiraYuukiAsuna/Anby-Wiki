"use client";

import Link from "next/link";
import { WorkspaceLink } from "@/components/workspace-link";
import {
  ArrowRight,
  BookOpenText,
  ChevronLeft,
  ChevronRight,
  Clock3,
  FilePenLine,
  FileText,
  RefreshCw,
  SortAsc,
} from "lucide-react";
import { Fragment, useState } from "react";
import useSWR from "swr";

import type { PageCatalogPage } from "../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { readingApi } from "@/lib/api";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 30;
type SortMode = "recent" | "title";
type PageStatus = "published" | "draft";

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
});

export function PageDirectory() {
  const [sort, setSort] = useState<SortMode>("recent");
  const [status, setStatus] = useState<PageStatus>("published");
  const [page, setPage] = useState(1);
  const state = useSWR<PageCatalogPage>(
    ["page-directory", status, sort, page] as const,
    (cacheKey) => {
      const [, selectedStatus, selectedSort, selectedPage] = cacheKey as readonly [
        string,
        PageStatus,
        SortMode,
        number,
      ];
      return readingApi().listPublishedPages({
        status: selectedStatus,
        sort: selectedSort,
        page: selectedPage,
        pageSize: PAGE_SIZE,
      });
    },
    {
      onSuccess(data) {
        const lastPage = Math.max(1, Math.ceil(data.total / PAGE_SIZE));
        if (page > lastPage) setPage(lastPage);
      },
    },
  );

  const items = state.data?.items ?? [];
  const total = state.data?.total;
  const pageCount = Math.max(1, Math.ceil((total ?? 0) / PAGE_SIZE));
  const pageNumbers = pageCount <= 7
    ? Array.from({ length: pageCount }, (_, index) => index + 1)
    : [...new Set([1, page - 1, page, page + 1, pageCount])]
      .filter((number) => number >= 1 && number <= pageCount)
      .sort((a, b) => a - b);
  const published = status === "published";

  return (
    <section aria-labelledby="page-directory-title" aria-busy={state.isValidating}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="page-directory-title" className="text-xl font-semibold">
          {typeof total === "number"
            ? `${total} 个${published ? "已发布条目" : "未发布草稿"}`
            : published
              ? "已发布条目"
              : "未发布草稿"}
        </h2>
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex rounded-xl bg-muted p-1" aria-label="页面状态">
            <button
              type="button"
              aria-pressed={published}
              onClick={() => { setStatus("published"); setPage(1); }}
              className={cn(
                "flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-muted-foreground",
                published && "bg-background text-foreground shadow-sm",
              )}
            >
              <FileText className="size-3.5" aria-hidden />已发布
            </button>
            <button
              type="button"
              aria-pressed={!published}
              onClick={() => { setStatus("draft"); setPage(1); }}
              className={cn(
                "flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium text-muted-foreground",
                !published && "bg-background text-foreground shadow-sm",
              )}
            >
              <FilePenLine className="size-3.5" aria-hidden />未发布
            </button>
          </div>
          <div className="flex rounded-xl bg-muted p-1" aria-label="页面排序">
            <button
              type="button"
              aria-pressed={sort === "recent"}
              onClick={() => { setSort("recent"); setPage(1); }}
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
              onClick={() => { setSort("title"); setPage(1); }}
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
          <p>页面目录暂时无法读取，请稍后重试。</p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-3"
            disabled={state.isValidating}
            onClick={() => void state.mutate()}
          >
            重试
          </Button>
        </div>
      ) : null}
      {state.isLoading && !state.data ? (
        <div className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2, 3, 4, 5].map((item) => <div key={item} className="h-28 animate-pulse rounded-2xl border bg-muted/35" />)}
        </div>
      ) : null}
      {!state.isLoading && !state.error && total === 0 ? (
        <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
          <BookOpenText className="mx-auto size-8 text-muted-foreground" aria-hidden />
          <h3 className="mt-4 font-semibold">
            {published ? "还没有已发布页面" : "没有未发布草稿"}
          </h3>
          <Button asChild className="mt-5"><WorkspaceLink href="/new">创建页面</WorkspaceLink></Button>
        </div>
      ) : null}
      {items.length ? (
        <ol className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((page) => (
            <li key={page.id}>
              <Link
                href={published ? `/pages/${page.id}` : `/pages/${page.id}/edit`}
                className="group flex h-full min-w-0 items-start gap-3 rounded-2xl border bg-card p-4 transition hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-[0_12px_32px_rgb(15_23_42/0.07)]"
              >
                <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary/8 text-primary">
                  {published ? (
                    <FileText className="size-4" aria-hidden />
                  ) : (
                    <FilePenLine className="size-4" aria-hidden />
                  )}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="line-clamp-2 font-semibold leading-6 [overflow-wrap:anywhere]">{page.displayTitle}</span>
                  <span className="mt-2 block text-[11px] text-muted-foreground">
                    {DATE_FORMATTER.format(page.updatedAt)} · {page.language}
                    {!published ? " · 继续编辑" : ""}
                  </span>
                </span>
                <ArrowRight className="mt-1 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-primary" aria-hidden />
              </Link>
            </li>
          ))}
        </ol>
      ) : null}
      {typeof total === "number" && total > 0 ? (
        <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground" role="status">
            第 {page} / {pageCount} 页 · 每页 {PAGE_SIZE} 条
          </p>
          <nav aria-label="页面目录分页" className="flex flex-wrap items-center gap-1">
            <Button
              type="button"
              variant="outline"
              size="sm"
              aria-label="上一页"
              disabled={page <= 1 || state.isLoading}
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft className="size-4" aria-hidden />
              <span className="hidden sm:inline">上一页</span>
            </Button>
            {pageNumbers.map((number, index) => (
              <Fragment key={number}>
                {index > 0 && number - pageNumbers[index - 1] > 1 ? (
                  <span className="px-1 text-muted-foreground" aria-hidden>…</span>
                ) : null}
                <Button
                  type="button"
                  variant={number === page ? "default" : "outline"}
                  size="sm"
                  className="min-w-9 px-2"
                  aria-label={`第 ${number} 页`}
                  aria-current={number === page ? "page" : undefined}
                  disabled={state.isLoading}
                  onClick={() => setPage(number)}
                >
                  {number}
                </Button>
              </Fragment>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              aria-label="下一页"
              disabled={page >= pageCount || state.isLoading}
              onClick={() => setPage(page + 1)}
            >
              <span className="hidden sm:inline">下一页</span>
              <ChevronRight className="size-4" aria-hidden />
            </Button>
          </nav>
        </div>
      ) : null}
    </section>
  );
}
