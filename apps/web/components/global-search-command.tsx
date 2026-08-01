"use client";

import { Fragment, useEffect, useState } from "react";
import { Command } from "cmdk";
import {
  Archive,
  Bot,
  BrainCircuit,
  Boxes,
  Compass,
  DatabaseZap,
  FileText,
  Gavel,
  Globe2,
  Images,
  Layers3,
  LibraryBig,
  LockKeyhole,
  Network,
  Search,
  ShieldAlert,
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

const SEARCH_DEBOUNCE_MS = 200;
const SEARCH_LIMIT = 20;
const searchQuerySchema = z.string().trim().max(255, "搜索词不能超过 255 个字符");

const QUICK_LINKS = [
  { href: "/explore", label: "探索与搜索", detail: "关键词、混合与语义检索", icon: Compass },
  { href: "/explore/graph", label: "知识关系图", detail: "沿 Claim 探索 Entity 网络", icon: Network },
  { href: "/entities", label: "实体与知识", detail: "稳定身份与结构化事实", icon: Waypoints },
  { href: "/federation", label: "跨 Wiki 联邦", detail: "远端身份源与可核验映射", icon: Globe2 },
  { href: "/collections", label: "专题合集", detail: "人工、规则与动态合集", icon: Layers3 },
  { href: "/sources", label: "来源与证据", detail: "来源版本、分片与 Citation", icon: LibraryBig },
  { href: "/imports", label: "AI 导入中心", detail: "队列、进度与导入治理", icon: Bot },
  { href: "/governance", label: "治理中心", detail: "提案、审核与批量评审", icon: Gavel },
  { href: "/governance/protections", label: "页面保护", detail: "角色门槛与标题预留", icon: LockKeyhole },
  { href: "/governance/fact-check", label: "事实一致性", detail: "冲突、证据与 Entity 引用检查", icon: ShieldAlert },
  { href: "/governance/ai-trust", label: "AI 信任策略", detail: "Actor 信任等级与人工抽样", icon: BrainCircuit },
  { href: "/governance/revision-storage", label: "历史版本存储", detail: "Revision 冷热分层与归档", icon: Archive },
  { href: "/assets", label: "媒体与附件", detail: "不可变资产版本", icon: Images },
  { href: "/datasets", label: "可查询数据", detail: "数据记录与保存视图", icon: DatabaseZap },
  { href: "/components", label: "组件中心", detail: "版本化信息框与渲染", icon: Boxes },
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
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, SEARCH_DEBOUNCE_MS);
  const parsedQuery = searchQuerySchema.safeParse(debouncedQuery);

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
    open && parsedQuery.success && parsedQuery.data
      ? ["global-search", parsedQuery.data]
      : null,
    async (cacheKey) => {
      const [, q] = cacheKey as readonly [string, string];
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
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>全局搜索</DialogTitle>
            <DialogDescription>
              搜索页面内容与稳定 Entity，或直接前往知识和治理工作区。
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
              {!trimmedQuery ? (
                <Command.Group
                  heading="快速前往"
                  className="[&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:py-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-wider [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group-heading]]:uppercase"
                >
                  <div className="grid gap-1 sm:grid-cols-2">
                    {QUICK_LINKS.map((link) => {
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
                          <span className="min-w-0">
                            <span className="block truncate text-sm font-medium">
                              {link.label}
                            </span>
                            <span className="block truncate text-[10px] text-muted-foreground">
                              {link.detail}
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
                      <span className="min-w-0">
                        <span className="flex items-center gap-2">
                          <span className="truncate font-medium">
                            {hit.displayTitle}
                          </span>
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
                </Command.Group>
              ) : null}
              {entities.length > 0 ? (
                <Command.Group
                  heading="稳定 Entity"
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
                        <span className="flex items-center gap-2">
                          <span className="truncate font-medium">
                            {entity.displayLabel}
                          </span>
                          <span className="shrink-0 text-xs text-muted-foreground">
                            {entity.entityType.name}
                          </span>
                        </span>
                        <span className="block truncate font-mono text-[10px] text-muted-foreground">
                          {entity.canonicalKey} · {entity.claimCount} 条事实
                        </span>
                      </span>
                    </Command.Item>
                  ))}
                </Command.Group>
              ) : null}
            </Command.List>
            {data && data.pages.total > hits.length ? (
              <p className="mt-2 border-t px-3 pt-2 text-xs text-muted-foreground">
                页面显示 {hits.length} 条，共 {data.pages.total} 条；另有{" "}
                {entities.length} 个 Entity 候选
              </p>
            ) : null}
          </Command>
        </DialogContent>
      </Dialog>
    </>
  );
}
