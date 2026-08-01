"use client";

import Link from "next/link";
import {
  ArrowRight,
  BookOpenText,
  Braces,
  Building2,
  CalendarDays,
  CircleUser,
  FileText,
  Inbox,
  Languages,
  Leaf,
  Lightbulb,
  LoaderCircle,
  MapPin,
  Package,
  RefreshCw,
  Search,
  Sparkles,
  Tags,
  Waypoints,
} from "lucide-react";
import { useEffect, useState } from "react";
import useSWRInfinite from "swr/infinite";
import { z } from "zod";

import type {
  EntityCatalogItem,
  EntityListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { knowledgeApi } from "@/lib/api";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 24;
const querySchema = z.string().trim().max(255, "搜索词不能超过 255 个字符");
type EntityStatus = "active" | "merged" | "all";

const ENTITY_TYPES = [
  { value: "", label: "全部类型" },
  { value: "person", label: "人物" },
  { value: "organization", label: "组织" },
  { value: "place", label: "地点" },
  { value: "work", label: "作品" },
  { value: "character", label: "角色" },
  { value: "event", label: "事件" },
  { value: "product", label: "产品" },
  { value: "concept", label: "概念" },
  { value: "species", label: "物种" },
  { value: "software", label: "软件" },
] as const;

const STATUS_OPTIONS: Array<{ value: EntityStatus; label: string }> = [
  { value: "active", label: "有效实体" },
  { value: "all", label: "全部可追溯" },
  { value: "merged", label: "已合并" },
];

const TYPE_META = {
  person: { icon: CircleUser, tone: "bg-sky-100 text-sky-700" },
  organization: { icon: Building2, tone: "bg-amber-100 text-amber-700" },
  place: { icon: MapPin, tone: "bg-emerald-100 text-emerald-700" },
  work: { icon: BookOpenText, tone: "bg-violet-100 text-violet-700" },
  character: { icon: Sparkles, tone: "bg-pink-100 text-pink-700" },
  event: { icon: CalendarDays, tone: "bg-orange-100 text-orange-700" },
  product: { icon: Package, tone: "bg-cyan-100 text-cyan-700" },
  concept: { icon: Lightbulb, tone: "bg-yellow-100 text-yellow-700" },
  species: { icon: Leaf, tone: "bg-lime-100 text-lime-700" },
  software: { icon: Braces, tone: "bg-indigo-100 text-indigo-700" },
} as const;

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}

function EntityCard({ entity }: { entity: EntityCatalogItem }) {
  const meta =
    TYPE_META[entity.entityType.typeKey as keyof typeof TYPE_META] ?? {
      icon: Waypoints,
      tone: "bg-slate-100 text-slate-700",
    };
  const Icon = meta.icon;

  return (
    <li>
      <article className="group flex h-full flex-col rounded-2xl border border-border/80 bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)] transition duration-300 hover:-translate-y-0.5 hover:border-indigo-300/70 hover:shadow-[0_18px_45px_rgb(15_23_42/0.08)]">
        <div className="flex items-start justify-between gap-3">
          <span
            className={cn(
              "flex size-10 items-center justify-center rounded-xl",
              meta.tone,
            )}
          >
            <Icon className="size-4.5" aria-hidden />
          </span>
          <span
            className={cn(
              "rounded-full px-2 py-1 text-[10px] font-semibold tracking-wide",
              entity.status === "active"
                ? "bg-emerald-100 text-emerald-700"
                : "bg-amber-100 text-amber-700",
            )}
          >
            {entity.status === "active" ? "有效" : "已合并"}
          </span>
        </div>

        <div className="mt-5 min-w-0">
          <p className="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">
            {entity.entityType.name} · {entity.displayLanguage || "未标语言"}
          </p>
          <Link
            href={`/entities/${entity.id}`}
            className="mt-1.5 block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <h3 className="truncate text-lg font-semibold tracking-[-0.02em] group-hover:text-indigo-700">
              {entity.displayLabel}
            </h3>
          </Link>
          <p className="mt-2 line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">
            {entity.description || `稳定键 · ${entity.canonicalKey}`}
          </p>
          <p className="mt-2 truncate font-mono text-[10px] text-muted-foreground/75">
            {entity.canonicalKey}
          </p>
        </div>

        <dl className="mt-5 grid grid-cols-3 gap-2 border-y border-border/70 py-3 text-center">
          <div>
            <dd className="text-sm font-semibold tabular-nums">{entity.claimCount}</dd>
            <dt className="mt-0.5 text-[10px] text-muted-foreground">事实</dt>
          </div>
          <div>
            <dd className="text-sm font-semibold tabular-nums">{entity.pageCount}</dd>
            <dt className="mt-0.5 text-[10px] text-muted-foreground">页面</dt>
          </div>
          <div>
            <dd className="text-sm font-semibold tabular-nums">
              {entity.labelCount + entity.aliasCount}
            </dd>
            <dt className="mt-0.5 text-[10px] text-muted-foreground">名称</dt>
          </div>
        </dl>

        <Link
          href={`/entities/${entity.id}`}
          className="mt-4 flex items-center justify-between text-xs font-semibold text-indigo-700"
        >
          查看稳定身份
          <ArrowRight
            className="size-3.5 transition-transform group-hover:translate-x-0.5"
            aria-hidden
          />
        </Link>
      </article>
    </li>
  );
}

export function EntityLibrary() {
  const [query, setQuery] = useState("");
  const [typeKey, setTypeKey] = useState("");
  const [status, setStatus] = useState<EntityStatus>("active");
  const debouncedQuery = useDebouncedValue(query, 240);
  const parsedQuery = querySchema.safeParse(debouncedQuery);
  const normalizedQuery = parsedQuery.success ? parsedQuery.data : "";

  const state = useSWRInfinite<EntityListPage>(
    (pageIndex, previousPage) => {
      if (!parsedQuery.success) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "entity-catalog",
        normalizedQuery,
        typeKey,
        status,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, q, selectedType, selectedStatus, cursor] = cacheKey as readonly [
        string,
        string,
        string,
        EntityStatus,
        string,
      ];
      return knowledgeApi().listEntities({
        q: q || undefined,
        typeKey: selectedType || undefined,
        status: selectedStatus,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
    { keepPreviousData: true, revalidateFirstPage: true },
  );

  const entities = state.data?.flatMap((page) => page.items) ?? [];
  const lastPage = state.data?.[state.data.length - 1];
  const canLoadMore = Boolean(state.data && lastPage?.nextCursor);
  const loadingMore =
    state.isValidating &&
    Boolean(state.data) &&
    state.size > (state.data?.length ?? 0);

  return (
    <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_18rem]">
      <section aria-labelledby="entity-catalog-title">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="entity-catalog-title" className="text-xl font-semibold">
              实体目录
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              名称相同不等于身份相同；合并必须由治理动作明确发生。
            </p>
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
        </div>

        <div className="mt-5 grid gap-3 rounded-2xl border border-border/80 bg-card/70 p-4 md:grid-cols-[minmax(0,1fr)_11rem_10rem]">
          <label className="relative block">
            <span className="sr-only">搜索实体</span>
            <Search
              className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground"
              aria-hidden
            />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              aria-invalid={!querySchema.safeParse(query).success}
              placeholder="搜索名称、别名或稳定键…"
              className="pl-8"
            />
          </label>
          <label>
            <span className="sr-only">实体类型</span>
            <select
              value={typeKey}
              onChange={(event) => setTypeKey(event.target.value)}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              {ENTITY_TYPES.map((type) => (
                <option key={type.value} value={type.value}>
                  {type.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="sr-only">实体状态</span>
            <select
              value={status}
              onChange={(event) => setStatus(event.target.value as EntityStatus)}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              {STATUS_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
        </div>
        {!parsedQuery.success ? (
          <p className="mt-2 text-xs text-destructive">
            {parsedQuery.error.issues[0]?.message}
          </p>
        ) : null}

        {state.error ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm">
            <p className="font-medium text-destructive">实体目录暂时不可用</p>
            <p className="mt-1 text-muted-foreground">
              请确认 API 与 PostgreSQL 已启动后重试。
            </p>
          </div>
        ) : null}

        {state.isLoading && !state.data ? (
          <div className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {[0, 1, 2, 3, 4, 5].map((item) => (
              <div
                key={item}
                className="h-72 animate-pulse rounded-2xl border bg-muted/35"
              />
            ))}
          </div>
        ) : null}

        {!state.isLoading && !state.error && entities.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
            <Inbox className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">没有符合条件的实体</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              试试清除搜索词或切换类型；新 Entity 也会由审核后的 Proposal
              与导入流程创建。
            </p>
          </div>
        ) : null}

        {entities.length > 0 ? (
          <ul className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {entities.map((entity) => (
              <EntityCard key={entity.id} entity={entity} />
            ))}
          </ul>
        ) : null}

        {canLoadMore ? (
          <div className="mt-6 flex justify-center">
            <Button
              type="button"
              variant="outline"
              disabled={loadingMore}
              onClick={() => void state.setSize(state.size + 1)}
            >
              {loadingMore ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : null}
              {loadingMore ? "载入中…" : "载入更多实体"}
            </Button>
          </div>
        ) : null}
      </section>

      <aside className="space-y-4">
        <div className="rounded-2xl border border-indigo-200/70 bg-indigo-50/70 p-5">
          <Waypoints className="size-5 text-indigo-700" aria-hidden />
          <h2 className="mt-4 font-semibold">稳定身份，不靠页面标题</h2>
          <p className="mt-2 text-xs leading-6 text-muted-foreground">
            Page 负责叙述，Entity 负责身份，Claim
            负责结构化事实。页面改名不会改变事实引用的目标。
          </p>
        </div>
        <div className="rounded-2xl border border-border/80 bg-card p-5">
          <h2 className="text-sm font-semibold">目录里的数字</h2>
          <dl className="mt-4 space-y-3 text-xs">
            <div className="flex gap-3">
              <FileText className="mt-0.5 size-3.5 text-muted-foreground" aria-hidden />
              <div>
                <dt className="font-medium">事实</dt>
                <dd className="mt-0.5 text-muted-foreground">
                  未被拒绝或取代的 Claim 数量
                </dd>
              </div>
            </div>
            <div className="flex gap-3">
              <Languages
                className="mt-0.5 size-3.5 text-muted-foreground"
                aria-hidden
              />
              <div>
                <dt className="font-medium">名称</dt>
                <dd className="mt-0.5 text-muted-foreground">
                  多语言标签与历史、缩写等别名
                </dd>
              </div>
            </div>
            <div className="flex gap-3">
              <Tags className="mt-0.5 size-3.5 text-muted-foreground" aria-hidden />
              <div>
                <dt className="font-medium">页面</dt>
                <dd className="mt-0.5 text-muted-foreground">
                  通过稳定绑定关联的不同 Page
                </dd>
              </div>
            </div>
          </dl>
        </div>
      </aside>
    </div>
  );
}
