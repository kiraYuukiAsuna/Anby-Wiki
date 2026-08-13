"use client";

import {
  ArrowRight,
  ChevronDown,
  ChevronRight,
  Component,
  FileText,
  Inbox,
  LoaderCircle,
  Network,
  Quote,
  Waypoints,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import useSWRInfinite from "swr/infinite";

import type {
  ComponentUsageListPage,
  SourceUsage,
  SourceUsageListPage,
  SourceUsageLocationListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { projectionApi } from "@/lib/api";

const PAGE_SIZE = 20;

export function SourceUsagePanel({ sourceId }: { sourceId: string }) {
  const usages = useSWRInfinite<SourceUsageListPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "projection:source-usages",
        sourceId,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, id, cursor] = cacheKey as readonly [string, string, string];
      return projectionApi().listSourceUsages({
        id,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );
  const items = useMemo(
    () => usages.data?.flatMap((page) => page.items) ?? [],
    [usages.data],
  );
  const lastPage = usages.data?.[usages.data.length - 1];
  const totals = usages.data?.[0];

  return (
    <section className="mt-9 border-t border-border/75 pt-8">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <span className="inline-flex items-center gap-1.5 text-xs font-semibold tracking-[0.12em] text-emerald-700 uppercase">
            <Network className="size-3.5" aria-hidden />
            Citation usage projection
          </span>
          <h2 className="mt-2 text-xl font-semibold">哪些页面使用此来源</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            按页面聚合 SourceVersion 与 Citation 身份链；展开后查看精确引用位置。
          </p>
        </div>
        {totals && totals.totalUsageCount > 0 ? (
          <span className="rounded-full border bg-muted/35 px-3 py-1 text-xs text-muted-foreground">
            {totals.totalUsageCount} 处引用 · {totals.totalPageCount} 个页面
          </span>
        ) : null}
      </div>

      {usages.isLoading && !usages.data ? (
        <div className="mt-5 h-32 animate-pulse rounded-2xl border bg-muted/30" />
      ) : null}
      {usages.error ? (
        <p className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
          来源使用关系暂时无法读取。
        </p>
      ) : null}
      {!usages.isLoading && !usages.error && items.length === 0 ? (
        <div className="mt-5 rounded-2xl border border-dashed px-6 py-10 text-center">
          <Inbox className="mx-auto size-7 text-muted-foreground" aria-hidden />
          <p className="mt-3 text-sm font-semibold">尚未被当前页面引用</p>
          <p className="mt-1 text-xs text-muted-foreground">
            创建 Citation 后，还需在 Claim 或页面正文中实际使用才会出现。
          </p>
        </div>
      ) : null}
      {items.length > 0 ? (
        <ol className="mt-5 divide-y overflow-hidden rounded-2xl border bg-card">
          {items.map((item) => (
            <SourceUsagePageRow key={item.pageId} sourceId={sourceId} item={item} />
          ))}
        </ol>
      ) : null}
      {lastPage?.nextCursor ? (
        <Button
          type="button"
          variant="outline"
          className="mt-3 w-full"
          disabled={usages.isValidating}
          onClick={() => void usages.setSize(usages.size + 1)}
        >
          {usages.isValidating ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : null}
          加载更多页面
        </Button>
      ) : null}
    </section>
  );
}

function SourceUsagePageRow({
  sourceId,
  item,
}: {
  sourceId: string;
  item: SourceUsage;
}) {
  const [expanded, setExpanded] = useState(false);
  const locations = useSWRInfinite<SourceUsageLocationListPage>(
    (pageIndex, previousPage) => {
      if (!expanded) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "projection:source-usage-locations",
        sourceId,
        item.pageId,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, id, pageId, cursor] = cacheKey as readonly [
        string,
        string,
        string,
        string,
      ];
      return projectionApi().listSourceUsageLocations({
        id,
        pageId,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );
  const locationItems = useMemo(
    () => locations.data?.flatMap((page) => page.items) ?? [],
    [locations.data],
  );
  const lastLocationPage = locations.data?.[locations.data.length - 1];

  return (
    <li>
      <div className="flex flex-wrap items-center gap-3 p-4">
        <button
          type="button"
          onClick={() => setExpanded((value) => !value)}
          aria-expanded={expanded}
          aria-label={`${expanded ? "收起" : "展开"}${item.pageTitle}的引用位置`}
          className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-emerald-100 text-emerald-700 transition-colors hover:bg-emerald-200"
        >
          {expanded ? (
            <ChevronDown className="size-4" aria-hidden />
          ) : (
            <ChevronRight className="size-4" aria-hidden />
          )}
        </button>
        <div className="min-w-0 flex-1">
          <Link
            href={`/pages/${item.pageId}`}
            className="font-medium hover:text-primary hover:underline"
          >
            {item.pageTitle}
          </Link>
          <p className="mt-1 text-xs text-muted-foreground">
            {item.usageCount} 处引用 · {item.blockCount} 个区块 ·{" "}
            {item.citationCount} 个 Citation
          </p>
        </div>
        <span className="max-w-full break-all font-mono text-[10px] text-muted-foreground">
          revision {item.revisionId}
        </span>
      </div>

      {expanded ? (
        <div className="border-t bg-muted/15 px-4 py-3 pl-16">
          {locations.isLoading && !locations.data ? (
            <p className="text-xs text-muted-foreground">正在加载精确位置…</p>
          ) : null}
          {locations.error ? (
            <p className="text-xs text-destructive">引用位置暂时无法读取。</p>
          ) : null}
          {locationItems.length > 0 ? (
            <ul className="space-y-2">
              {locationItems.map((location) => (
                <li
                  key={`${location.blockId}:${location.nodeId}:${location.citationId}`}
                  className="flex flex-wrap items-center gap-2 rounded-xl border bg-background px-3 py-2"
                >
                  <FileText className="size-3.5 text-muted-foreground" aria-hidden />
                  <Link
                    href={`/pages/${item.pageId}#${location.blockId}`}
                    className="min-w-0 flex-1 break-all font-mono text-[10px] text-muted-foreground hover:text-primary hover:underline"
                  >
                    Block {location.blockId} · Node {location.nodeId}
                  </Link>
                  {location.claimId ? (
                    <Button asChild size="xs" variant="ghost">
                      <Link href={`/claims/${location.claimId}`}>
                        Claim
                        <ArrowRight aria-hidden />
                      </Link>
                    </Button>
                  ) : null}
                  <Button asChild size="xs" variant="outline">
                    <Link href={`/citations/${location.citationId}`}>
                      <Quote aria-hidden />
                      Citation
                    </Link>
                  </Button>
                </li>
              ))}
            </ul>
          ) : null}
          {lastLocationPage?.nextCursor ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="mt-2 w-full"
              disabled={locations.isValidating}
              onClick={() => void locations.setSize(locations.size + 1)}
            >
              {locations.isValidating ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : null}
              加载更多引用位置
            </Button>
          ) : null}
        </div>
      ) : null}
    </li>
  );
}

export function ComponentUsagePanel({
  componentId,
  versions,
}: {
  componentId: string;
  versions: number[];
}) {
  const [version, setVersion] = useState<number | "all">("all");
  const usages = useSWRInfinite<ComponentUsageListPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "projection:component-usages",
        componentId,
        version,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, id, selectedVersion, cursor] = cacheKey as readonly [
        string,
        string,
        number | "all",
        string,
      ];
      return projectionApi().listComponentUsages({
        id,
        version:
          selectedVersion === "all" ? undefined : selectedVersion,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );
  const items = useMemo(
    () => usages.data?.flatMap((page) => page.items) ?? [],
    [usages.data],
  );
  const lastPage = usages.data?.[usages.data.length - 1];
  const pageCount = new Set(items.map((item) => item.pageId)).size;

  return (
    <section className="mt-9 border-t border-border/75 pt-8">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <span className="inline-flex items-center gap-1.5 text-xs font-semibold tracking-[0.12em] text-violet-700 uppercase">
            <Network className="size-3.5" aria-hidden />
            Component dependency
          </span>
          <h2 className="mt-2 text-xl font-semibold">页面依赖关系</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            展示当前 Revision 中锁定的组件版本与 Entity 上下文。
          </p>
        </div>
        <select
          value={version}
          onChange={(event) => {
            setVersion(
              event.target.value === "all"
                ? "all"
                : Number(event.target.value),
            );
            void usages.setSize(1);
          }}
          aria-label="按组件版本筛选使用关系"
          className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
        >
          <option value="all">全部版本</option>
          {[...new Set(versions)]
            .sort((left, right) => right - left)
            .map((item) => (
              <option key={item} value={item}>
                v{item}
              </option>
            ))}
        </select>
      </div>

      {items.length > 0 ? (
        <p className="mt-4 text-xs text-muted-foreground">
          已加载 {items.length} 处依赖，覆盖 {pageCount} 个页面。
        </p>
      ) : null}
      {usages.isLoading && !usages.data ? (
        <div className="mt-5 h-32 animate-pulse rounded-2xl border bg-muted/30" />
      ) : null}
      {usages.error ? (
        <p className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
          组件依赖关系暂时无法读取。
        </p>
      ) : null}
      {!usages.isLoading && !usages.error && items.length === 0 ? (
        <div className="mt-5 rounded-2xl border border-dashed px-6 py-10 text-center">
          <Component className="mx-auto size-7 text-muted-foreground" aria-hidden />
          <p className="mt-3 text-sm font-semibold">没有匹配的页面依赖</p>
          <p className="mt-1 text-xs text-muted-foreground">
            在页面中插入已发布 ComponentBlock 后会自动建立投影。
          </p>
        </div>
      ) : null}
      {items.length > 0 ? (
        <ol className="mt-5 divide-y overflow-hidden rounded-2xl border bg-card">
          {items.map((item) => (
            <li
              key={`${item.pageId}:${item.blockId}`}
              className="flex flex-wrap items-center gap-3 p-4"
            >
              <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-violet-100 text-violet-700">
                <Component className="size-4" aria-hidden />
              </span>
              <div className="min-w-0 flex-1">
                <Link
                  href={`/pages/${item.pageId}`}
                  className="font-medium hover:text-primary hover:underline"
                >
                  {item.pageTitle}
                </Link>
                <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
                  v{item.componentVersion} · block{" "}
                  {item.blockId.slice(0, 8)}
                </p>
              </div>
              <Button asChild size="xs" variant="outline">
                <Link href={`/entities/${item.entityId}`}>
                  <Waypoints aria-hidden />
                  Entity
                </Link>
              </Button>
              <Button asChild size="xs" variant="ghost">
                <Link href={`/pages/${item.pageId}`}>
                  打开页面
                  <ArrowRight aria-hidden />
                </Link>
              </Button>
            </li>
          ))}
        </ol>
      ) : null}
      {lastPage?.nextCursor ? (
        <Button
          type="button"
          variant="outline"
          className="mt-3 w-full"
          disabled={usages.isValidating}
          onClick={() => void usages.setSize(usages.size + 1)}
        >
          {usages.isValidating ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : null}
          加载更多依赖
        </Button>
      ) : null}
    </section>
  );
}
