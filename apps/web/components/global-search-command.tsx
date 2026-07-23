"use client";

import { Fragment, useEffect, useState } from "react";
import { Command } from "cmdk";
import { FileText, Search } from "lucide-react";
import { useRouter } from "next/navigation";
import useSWR from "swr";

import type { PageSearchHit } from "../../../contracts/generated/typescript";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { searchApi } from "@/lib/api";

const SEARCH_DEBOUNCE_MS = 200;
const SEARCH_LIMIT = 20;

function useDebouncedValue(value: string, delayMs: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [delayMs, value]);
  return debounced;
}

function SearchHighlight({ value }: { value: string }) {
  const parts = value.split(/(\[\[|\]\])/);
  let marked = false;
  return parts.map((part, index) => {
    if (part === "[[") {
      marked = true;
      return null;
    }
    if (part === "]]") {
      marked = false;
      return null;
    }
    return marked ? (
      <mark key={index} className="rounded-sm bg-primary/15 text-foreground">
        {part}
      </mark>
    ) : (
      <Fragment key={index}>{part}</Fragment>
    );
  });
}

const MATCH_LABEL: Record<PageSearchHit["matchedOn"], string> = {
  title: "标题",
  alias: "别名",
  body: "正文",
  entity: "实体",
};

export function GlobalSearchCommand() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, SEARCH_DEBOUNCE_MS);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const { data, isLoading } = useSWR(
    open && debouncedQuery.trim()
      ? ["global-search", debouncedQuery.trim()]
      : null,
    ([, q]) =>
      searchApi().searchPages({
        q,
        namespace: "main",
        limit: SEARCH_LIMIT,
      }),
    { keepPreviousData: true },
  );
  const hits = data?.items ?? [];
  const trimmedQuery = query.trim();

  const selectHit = (hit: PageSearchHit) => {
    setOpen(false);
    setQuery("");
    router.push(`/pages/${hit.id}`);
  };

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="ml-auto flex h-8 w-full max-w-xs items-center gap-2 rounded-lg border border-input bg-muted px-2.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        aria-label="搜索站点"
      >
        <Search className="size-4" aria-hidden />
        <span className="truncate">搜索页面与知识</span>
        <kbd className="ml-auto hidden rounded border bg-background px-1.5 py-0.5 text-[10px] sm:inline">
          ⌘K
        </kbd>
      </button>
      <Dialog
        open={open}
        onOpenChange={(nextOpen) => {
          setOpen(nextOpen);
          if (!nextOpen) setQuery("");
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>全局搜索</DialogTitle>
            <DialogDescription>
              搜索页面标题、旧别名、正文和关联实体。
            </DialogDescription>
          </DialogHeader>
          <Command shouldFilter={false} label="全局搜索">
            <Command.Input
              value={query}
              onValueChange={setQuery}
              placeholder="输入关键词…"
              autoFocus
              className="border-input placeholder:text-muted-foreground flex h-10 w-full rounded-md border bg-transparent px-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]"
            />
            <Command.List className="mt-2 max-h-80 overflow-y-auto">
              {isLoading && hits.length === 0 ? (
                <Command.Loading>
                  <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                    搜索中…
                  </div>
                </Command.Loading>
              ) : null}
              {!isLoading && trimmedQuery && hits.length === 0 ? (
                <Command.Empty className="px-3 py-6 text-center text-sm text-muted-foreground">
                  没有匹配结果
                </Command.Empty>
              ) : null}
              {hits.map((hit) => (
                <Command.Item
                  key={hit.id}
                  value={hit.id}
                  onSelect={() => selectHit(hit)}
                  className="data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground flex cursor-pointer gap-3 rounded-md px-3 py-2"
                >
                  <FileText className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0">
                    <span className="flex items-center gap-2">
                      <span className="truncate font-medium">{hit.displayTitle}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {MATCH_LABEL[hit.matchedOn]}命中
                      </span>
                    </span>
                    <span className="line-clamp-2 block text-xs text-muted-foreground">
                      <SearchHighlight value={hit.highlight} />
                    </span>
                  </span>
                </Command.Item>
              ))}
            </Command.List>
            {data && data.total > hits.length ? (
              <p className="mt-2 border-t px-3 pt-2 text-xs text-muted-foreground">
                显示 {hits.length} 条，共 {data.total} 条结果
              </p>
            ) : null}
          </Command>
        </DialogContent>
      </Dialog>
    </>
  );
}
