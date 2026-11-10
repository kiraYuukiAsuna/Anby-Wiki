"use client";

import { Fragment, useEffect, useState } from "react";
import { Command } from "cmdk";
import {
  Archive,
  BadgeCheck,
  Bot,
  BookOpenText,
  BrainCircuit,
  Boxes,
  FileText,
  Globe2,
  Images,
  Layers3,
  LibraryBig,
  LockKeyhole,
  Network,
  Search,
  ShieldAlert,
  ShieldCheck,
  TerminalSquare,
  UserRound,
  UsersRound,
  Waypoints,
} from "lucide-react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import { z } from "zod";

import type {
  EntityCatalogItem,
  PageSearchHit,
} from "../../../contracts/generated/typescript";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { knowledgeApi, searchApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { workspaceAccessNotice } from "@/lib/workspace-access";

const SEARCH_DEBOUNCE_MS = 200;
const SEARCH_LIMIT = 20;
const searchQuerySchema = z.string().trim().max(255, "搜索词不能超过 255 个字符");

const QUICK_LINKS = [
  {
    href: "/pages",
    label: "页面目录",
    icon: BookOpenText,
  },
  { href: "/explore", label: "搜索", icon: Search },
  { href: "/explore/graph", label: "知识关系图", icon: Network },
  { href: "/entities", label: "实体与知识", icon: Waypoints },
  { href: "/federation", label: "跨 Wiki 关联", icon: Globe2 },
  { href: "/collections", label: "专题合集", icon: Layers3 },
  { href: "/sources", label: "来源与证据", icon: LibraryBig },
  { href: "/imports", label: "导入资料", icon: Bot },
  { href: "/governance/review", label: "待审核", icon: ShieldCheck },
  {
    href: "/governance/apply",
    label: "待生效",
    icon: BadgeCheck,
  },
  { href: "/governance/protections", label: "页面保护", icon: LockKeyhole },
  { href: "/governance/fact-check", label: "事实一致性", icon: ShieldAlert },
  { href: "/governance/ai-trust", label: "AI 审核设置", icon: BrainCircuit },
  { href: "/governance/revision-storage", label: "历史版本存储", icon: Archive },
  { href: "/account", label: "个人中心", icon: UserRound },
  { href: "/admin/members", label: "成员与角色", icon: UsersRound },
  { href: "/settings/cli", label: "Agent CLI 授权", icon: TerminalSquare },
  { href: "/assets", label: "媒体与附件", icon: Images },
  { href: "/components", label: "组件中心", icon: Boxes },
] as const;

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
  const { session } = useSession();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, SEARCH_DEBOUNCE_MS);
  const parsedQuery = searchQuerySchema.safeParse(debouncedQuery);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!event.isComposing && (event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const { data, isLoading } = useSWR(
    open && parsedQuery.success && parsedQuery.data
      ? ["global-search", session?.actorId, session?.wikiId, parsedQuery.data]
      : null,
    async (cacheKey) => {
      const [, , , q] = cacheKey as readonly [string, string, string, string];
      const [pages, entities] = await Promise.all([
        searchApi().searchPages({
          q,
          namespace: "main",
          limit: SEARCH_LIMIT,
        }),
        knowledgeApi().listEntities({
          q,
          status: "active",
          pageSize: 10,
        }),
      ]);
      return { pages, entities };
    },
    { keepPreviousData: true },
  );
  const hits = data?.pages.items ?? [];
  const entities = data?.entities.items ?? [];
  const trimmedQuery = query.trim();

  const selectHit = (hit: PageSearchHit) => {
    setOpen(false);
    setQuery("");
    router.push(`/pages/${hit.id}`);
  };

  const selectEntity = (entity: EntityCatalogItem) => {
    setOpen(false);
    setQuery("");
    router.push(`/entities/${entity.id}`);
  };

  const goTo = (href: string) => {
    setOpen(false);
    setQuery("");
    router.push(href);
  };

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="ml-auto flex h-8 w-full min-w-0 max-w-xs items-center gap-2 rounded-lg border border-input bg-muted px-2.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
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
        <DialogContent className="max-h-[calc(100dvh-2rem)] min-w-0 overflow-hidden sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>全局搜索</DialogTitle>
            <DialogDescription className="sr-only">
              搜索页面、实体，或快速打开常用功能。
            </DialogDescription>
          </DialogHeader>
          <Command
            shouldFilter={false}
            label="全局搜索"
            className="flex min-h-0 min-w-0 flex-col overflow-hidden"
          >
            <Command.Input
              value={query}
              onValueChange={setQuery}
              placeholder="输入关键词…"
              autoFocus
              className="border-input placeholder:text-muted-foreground flex h-10 w-full rounded-md border bg-transparent px-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]"
            />
            <Command.List className="mt-2 min-h-0 min-w-0 max-h-[min(32rem,calc(100dvh-13rem))] overflow-x-hidden overflow-y-auto overscroll-contain scroll-py-2 pr-1 [scrollbar-gutter:stable]">
              {!trimmedQuery ? (
                <Command.Group
                  heading="快速前往"
                  className="[&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-wider [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group-heading]]:uppercase"
                >
                  <div className="grid gap-1 sm:grid-cols-2">
                    {QUICK_LINKS.filter((link) => !workspaceAccessNotice(link.href, session)).map((link) => {
                      const Icon = link.icon;
                      return (
                        <Command.Item
                          key={link.href}
                          value={link.href}
                          onSelect={() => goTo(link.href)}
                          className="flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2.5 data-[selected=true]:bg-accent"
                        >
                          <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/9 text-primary">
                            <Icon className="size-4" aria-hidden />
                          </span>
                          <span className="min-w-0 flex-1 overflow-hidden">
                            <span className="block truncate text-sm font-medium">
                              {link.label}
                            </span>
                          </span>
                        </Command.Item>
                      );
                    })}
                  </div>
                </Command.Group>
              ) : null}
              {isLoading && hits.length === 0 && entities.length === 0 ? (
                <Command.Loading>
                  <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                    搜索中…
                  </div>
                </Command.Loading>
              ) : null}
              {!parsedQuery.success ? (
                <Command.Empty className="px-3 py-6 text-center text-sm text-destructive">
                  {parsedQuery.error.issues[0]?.message}
                </Command.Empty>
              ) : null}
              {!isLoading &&
              parsedQuery.success &&
              trimmedQuery &&
              hits.length === 0 &&
              entities.length === 0 ? (
                <Command.Empty className="px-3 py-6 text-center text-sm text-muted-foreground">
                  没有匹配结果
                </Command.Empty>
              ) : null}
              {hits.length > 0 ? (
                <Command.Group
                  heading="百科页面"
                  className="[&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-wider [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group-heading]]:uppercase"
                >
                  {hits.map((hit) => (
                    <Command.Item
                      key={hit.id}
                      value={`page:${hit.id}`}
                      onSelect={() => selectHit(hit)}
                      className="flex cursor-pointer gap-3 rounded-md px-3 py-2 data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground"
                    >
                      <FileText className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 overflow-hidden">
                        <span className="flex min-w-0 items-center gap-2">
                          <span className="min-w-0 flex-1 truncate font-medium">
                            {hit.displayTitle}
                          </span>
                          <span className="shrink-0 text-xs text-muted-foreground">
                            {MATCH_LABEL[hit.matchedOn]}命中
                          </span>
                        </span>
                        <span className="line-clamp-2 block break-words text-xs text-muted-foreground [overflow-wrap:anywhere]">
                          <SearchHighlight value={hit.highlight} />
                        </span>
                      </span>
                    </Command.Item>
                  ))}
                </Command.Group>
              ) : null}
              {entities.length > 0 ? (
                <Command.Group
                  heading="实体"
                  className="[&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-wider [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group-heading]]:uppercase"
                >
                  {entities.map((entity) => (
                    <Command.Item
                      key={entity.id}
                      value={`entity:${entity.id}`}
                      onSelect={() => selectEntity(entity)}
                      className="flex cursor-pointer gap-3 rounded-md px-3 py-2 data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground"
                    >
                      <Waypoints className="mt-0.5 size-4 shrink-0 text-indigo-600" />
                      <span className="min-w-0 flex-1">
                        <span className="flex min-w-0 items-center gap-2">
                          <span className="min-w-0 flex-1 truncate font-medium">
                            {entity.displayLabel}
                          </span>
                          <span className="shrink-0 text-xs text-muted-foreground">
                            {entity.entityType.name}
                          </span>
                        </span>
                        <span className="block max-w-full truncate font-mono text-[10px] text-muted-foreground">
                          {entity.canonicalKey} · {entity.claimCount} 条事实
                        </span>
                      </span>
                    </Command.Item>
                  ))}
                </Command.Group>
              ) : null}
            </Command.List>
            {data && data.pages.total > hits.length ? (
              <p className="mt-2 min-w-0 border-t px-3 pt-2 text-xs break-words text-muted-foreground">
                页面显示 {hits.length} 条，共 {data.pages.total} 条；另有{" "}
                {entities.length} 个相关实体
              </p>
            ) : null}
          </Command>
        </DialogContent>
      </Dialog>
    </>
  );
}
