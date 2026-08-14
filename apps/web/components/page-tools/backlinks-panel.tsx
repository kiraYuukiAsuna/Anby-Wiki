"use client";

import {
  ArrowUpRight,
  CornerUpLeft,
  Inbox,
  Link2,
  LoaderCircle,
  RefreshCw,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";
import useSWRInfinite from "swr/infinite";

import type {
  Backlink,
  BacklinkListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { projectionApi } from "@/lib/api";
import { compactId } from "@/lib/display-id";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 24;

type BacklinkPageGroup = {
  pageId: string;
  pageTitle: string;
  items: Backlink[];
  blockCount: number;
  displayTexts: string[];
};

function groupBacklinksByPage(items: Backlink[]): BacklinkPageGroup[] {
  const groups = new Map<
    string,
    BacklinkPageGroup & { blockIds: Set<string>; displayTextSet: Set<string> }
  >();
  for (const item of items) {
    let group = groups.get(item.sourcePageId);
    if (!group) {
      group = {
        pageId: item.sourcePageId,
        pageTitle: item.sourceTitle,
        items: [],
        blockCount: 0,
        displayTexts: [],
        blockIds: new Set<string>(),
        displayTextSet: new Set<string>(),
      };
      groups.set(item.sourcePageId, group);
    }
    group.items.push(item);
    group.blockIds.add(item.sourceBlockId);
    if (item.displayText.trim()) group.displayTextSet.add(item.displayText.trim());
  }
  return Array.from(groups.values(), (group) => ({
    pageId: group.pageId,
    pageTitle: group.pageTitle,
    items: group.items,
    blockCount: group.blockIds.size,
    displayTexts: Array.from(group.displayTextSet),
  }));
}

export function BacklinksPanel({
  pageId,
  pageTitle,
}: {
  pageId: string;
  pageTitle: string;
}) {
  const state = useSWRInfinite<BacklinkListPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "page:backlinks",
        pageId,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, id, cursor] = cacheKey as readonly [string, string, string];
      return projectionApi().listBacklinks({
        id,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
    { revalidateFirstPage: true },
  );

  const items = useMemo(
    () => state.data?.flatMap((page) => page.items) ?? [],
    [state.data],
  );
  const groups = useMemo(() => groupBacklinksByPage(items), [items]);
  const lastPage = state.data?.at(-1);
  const loadingMore =
    state.isValidating &&
    Boolean(state.data) &&
    state.size > (state.data?.length ?? 0);

  return (
    <section className="overflow-hidden rounded-2xl border bg-card shadow-sm">
      <header className="flex flex-wrap items-start justify-between gap-4 border-b px-5 py-5">
        <div className="flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-blue-100 text-blue-700">
            <Link2 className="size-4" aria-hidden />
          </span>
          <div>
            <h2 className="font-semibold">链入本页</h2>
            <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
              查看哪些当前 Revision 引用了「{pageTitle}」。关系来自可重建投影，发布后可能有短暂延迟。
            </p>
          </div>
        </div>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={state.isValidating}
          onClick={() => void state.mutate()}
        >
          <RefreshCw
            className={cn("size-3.5", state.isValidating && "animate-spin")}
            aria-hidden
          />
          刷新
        </Button>
      </header>

      <div className="p-5">
        {state.isLoading && !state.data ? (
          <div className="flex min-h-36 items-center justify-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="size-4 animate-spin" aria-hidden />
            正在读取关系投影…
          </div>
        ) : state.error ? (
          <div className="rounded-xl border border-destructive/20 bg-destructive/5 p-4">
            <TriangleAlert className="size-5 text-destructive" aria-hidden />
            <p className="mt-3 text-sm font-semibold">反向链接暂不可用</p>
            <p className="mt-1 text-xs text-muted-foreground">
              页面或投影可能刚刚变化，请稍后刷新。
            </p>
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-xl border border-dashed bg-muted/20 px-6 py-10 text-center">
            <Inbox className="mx-auto size-6 text-muted-foreground" aria-hidden />
            <p className="mt-3 text-sm font-semibold">还没有页面链入这里</p>
            <p className="mt-1 text-xs text-muted-foreground">
              未解析标题引用不会出现在这里，解析完成后会由 Worker 自动补入。
            </p>
          </div>
        ) : (
          <>
            <p className="mb-3 text-xs text-muted-foreground">
              已加载 {groups.length} 个来源页面 · {items.length} 处链入
            </p>
            <ul className="grid gap-3 md:grid-cols-2">
              {groups.map((group) => (
                <li
                  key={group.pageId}
                  className="group rounded-xl border bg-background/55 p-4 transition hover:border-primary/25 hover:bg-primary/[0.025]"
                >
                  <Link
                    href={`/pages/${group.pageId}#${group.items[0].sourceBlockId}`}
                    className="flex items-start gap-3"
                  >
                    <CornerUpLeft
                      className="mt-0.5 size-4 shrink-0 text-primary"
                      aria-hidden
                    />
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-1 font-semibold">
                        <span className="truncate">{group.pageTitle}</span>
                        <ArrowUpRight
                          className="size-3.5 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
                          aria-hidden
                        />
                      </span>
                      <span className="mt-1 block text-xs leading-5 text-muted-foreground">
                        {group.items.length} 处链入 · {group.blockCount} 个区块
                      </span>
                    </span>
                  </Link>
                  {group.displayTexts.length > 0 ? (
                    <p className="mt-2 line-clamp-2 text-xs text-muted-foreground">
                      展示文本：
                      {group.displayTexts.map((text) => `“${text}”`).join("、")}
                    </p>
                  ) : null}
                  {group.items.length > 1 ? (
                    <details className="mt-2 text-xs text-muted-foreground">
                      <summary className="cursor-pointer select-none hover:text-foreground">
                        查看 {group.items.length} 个精确位置
                      </summary>
                      <ul className="mt-2 space-y-1.5 border-l pl-3">
                        {group.items.map((item) => (
                          <li
                            key={`${item.sourceBlockId}:${item.sourceNodeId}`}
                          >
                            <Link
                              href={`/pages/${item.sourcePageId}#${item.sourceBlockId}`}
                              className="hover:text-foreground hover:underline"
                            >
                              Block{" "}
                              <span title={item.sourceBlockId}>
                                {compactId(item.sourceBlockId)}
                              </span>{" "}
                              · Node {item.sourceNodeId} · “{item.displayText}”
                            </Link>
                          </li>
                        ))}
                      </ul>
                    </details>
                  ) : (
                    <p className="mt-2 font-mono text-[9px] text-muted-foreground/75">
                      Block{" "}
                      <span title={group.items[0].sourceBlockId}>
                        {compactId(group.items[0].sourceBlockId)}
                      </span>{" "}
                      · Node {group.items[0].sourceNodeId}
                    </p>
                  )}
                </li>
              ))}
            </ul>
            {lastPage?.nextCursor ? (
              <Button
                type="button"
                variant="outline"
                className="mt-4 w-full"
                disabled={loadingMore}
                onClick={() => void state.setSize(state.size + 1)}
              >
                {loadingMore ? (
                  <LoaderCircle className="animate-spin" aria-hidden />
                ) : (
                  <Link2 aria-hidden />
                )}
                加载更多链入
              </Button>
            ) : null}
          </>
        )}
      </div>
    </section>
  );
}
