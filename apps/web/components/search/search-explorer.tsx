"use client";

import {
  ArrowLeft,
  ArrowRight,
  BadgeCheck,
  BookOpenText,
  Braces,
  Check,
  CircleAlert,
  Cpu,
  FileText,
  Filter,
  Languages,
  LoaderCircle,
  RotateCcw,
  Search,
  Sparkles,
  Tags,
  Waypoints,
} from "lucide-react";
import Link from "next/link";
import { Fragment, type FormEvent, useMemo, useState } from "react";
import useSWR from "swr";
import { z } from "zod";

import type {
  PageSearchHit,
  SearchCapabilities,
  SearchFacetValue,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { searchApi } from "@/lib/api";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 20;
const querySchema = z.string().trim().min(1, "请输入搜索内容").max(255, "搜索词不能超过 255 个字符");
const modeSchema = z.enum(["auto", "keyword", "hybrid", "semantic"]);
type SearchMode = z.infer<typeof modeSchema>;
type SearchField = "title" | "alias" | "body" | "entity";

const MODE_META: Record<
  SearchMode,
  { label: string; detail: string; icon: typeof Search }
> = {
  auto: { label: "智能", detail: "使用当前环境的推荐模式", icon: Sparkles },
  keyword: { label: "精确", detail: "名称、术语与正文关键词", icon: Search },
  hybrid: { label: "混合", detail: "关键词与概念相关性融合", icon: BadgeCheck },
  semantic: { label: "语义", detail: "按自然语言含义寻找页面", icon: Cpu },
};

const FIELD_META: Array<{ value: SearchField; label: string }> = [
  { value: "title", label: "标题" },
  { value: "alias", label: "别名" },
  { value: "body", label: "正文" },
  { value: "entity", label: "Entity" },
];

const MATCH_LABEL: Record<PageSearchHit["matchedOn"], string> = {
  title: "标题",
  alias: "别名",
  body: "正文",
  entity: "Entity",
};

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
      <mark
        key={`${part}-${index}`}
        className="rounded-sm bg-primary/15 px-0.5 text-foreground"
      >
        {part}
      </mark>
    ) : (
      <Fragment key={`${part}-${index}`}>{part}</Fragment>
    );
  });
}

function FacetGroup({
  title,
  icon: Icon,
  values,
  selected,
  onSelect,
}: {
  title: string;
  icon: typeof Filter;
  values: Array<SearchFacetValue>;
  selected: string;
  onSelect: (value: string) => void;
}) {
  if (values.length === 0 && !selected) return null;
  return (
    <section>
      <h3 className="flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
        <Icon className="size-3.5" aria-hidden />
        {title}
      </h3>
      <div className="mt-2 space-y-1">
        {selected ? (
          <button
            type="button"
            onClick={() => onSelect("")}
            className="flex w-full items-center justify-between rounded-lg bg-primary/9 px-2.5 py-2 text-left text-xs font-medium text-primary"
          >
            <span className="truncate">{selected}</span>
            <span className="text-[10px]">清除</span>
          </button>
        ) : null}
        {values
          .filter((item) => item.value !== selected)
          .slice(0, 10)
          .map((item) => (
            <button
              key={item.value}
              type="button"
              onClick={() => onSelect(item.value)}
              className="flex w-full items-center justify-between rounded-lg px-2.5 py-2 text-left text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            >
              <span className="truncate">{item.value}</span>
              <span className="tabular-nums">{item.count}</span>
            </button>
          ))}
      </div>
    </section>
  );
}

function backendLabel(capabilities?: SearchCapabilities) {
  return capabilities?.backend === "meilisearch"
    ? "Meilisearch 独立索引"
    : "PostgreSQL 开发回退";
}

function normalizedMode(value: string): SearchMode {
  const parsed = modeSchema.safeParse(value);
  return parsed.success ? parsed.data : "auto";
}

export function SearchExplorer({
  initialQuery,
  initialMode,
  initialNamespace,
  initialLanguage,
  initialEntityType,
}: {
  initialQuery: string;
  initialMode: string;
  initialNamespace: string;
  initialLanguage: string;
  initialEntityType: string;
}) {
  const [input, setInput] = useState(initialQuery);
  const [query, setQuery] = useState(initialQuery.trim());
  const [mode, setMode] = useState<SearchMode>(normalizedMode(initialMode));
  const [semanticRatio, setSemanticRatio] = useState(0.5);
  const [namespace, setNamespace] = useState(initialNamespace);
  const [language, setLanguage] = useState(initialLanguage);
  const [entityType, setEntityType] = useState(initialEntityType);
  const [fields, setFields] = useState<Set<SearchField>>(
    () => new Set(["title", "alias", "body", "entity"]),
  );
  const [offset, setOffset] = useState(0);
  const [validationMessage, setValidationMessage] = useState("");

  const capabilities = useSWR(
    "search-capabilities",
    () => searchApi().getSearchCapabilities(),
    { revalidateOnFocus: false },
  );
  const availableModes = capabilities.data
    ? new Set(capabilities.data.modes)
    : new Set(["keyword"]);
  const effectiveMode =
    mode === "auto"
      ? (capabilities.data?.defaultMode ?? "keyword")
      : mode;
  const modeAvailable =
    effectiveMode === "keyword" || availableModes.has(effectiveMode);
  const selectedFields = useMemo(
    () => FIELD_META.map((field) => field.value).filter((field) => fields.has(field)),
    [fields],
  );

  const search = useSWR(
    capabilities.data && query && modeAvailable
      ? [
          "search-explorer",
          query,
          effectiveMode,
          semanticRatio,
          namespace,
          language,
          entityType,
          selectedFields.join(","),
          offset,
        ]
      : null,
    () =>
      searchApi().searchPages({
        q: query,
        mode: effectiveMode,
        semanticRatio:
          effectiveMode === "hybrid" ? semanticRatio : undefined,
        namespace: namespace || undefined,
        language: language || undefined,
        entityType: entityType || undefined,
        fields: selectedFields,
        limit: PAGE_SIZE,
        offset,
      }),
    { keepPreviousData: true },
  );

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const parsed = querySchema.safeParse(input);
    if (!parsed.success) {
      setValidationMessage(parsed.error.issues[0]?.message ?? "搜索词无效");
      return;
    }
    setValidationMessage("");
    setQuery(parsed.data);
    setOffset(0);
  };

  const chooseMode = (nextMode: SearchMode) => {
    setMode(nextMode);
    setOffset(0);
  };

  const updateFacet = (
    setter: (value: string) => void,
    value: string,
  ) => {
    setter(value);
    setOffset(0);
  };

  const toggleField = (field: SearchField, checked: boolean) => {
    setFields((current) => {
      const next = new Set(current);
      if (checked) next.add(field);
      else if (next.size > 1) next.delete(field);
      return next;
    });
    setOffset(0);
  };

  const clearFilters = () => {
    setNamespace("");
    setLanguage("");
    setEntityType("");
    setFields(new Set(["title", "alias", "body", "entity"]));
    setOffset(0);
  };

  const result = search.data;
  const hasNext = result ? offset + result.items.length < result.total : false;
  const hasFilters =
    Boolean(namespace || language || entityType) || fields.size !== FIELD_META.length;

  return (
    <div className="grid gap-6 xl:grid-cols-[17rem_minmax(0,1fr)]">
      <aside className="order-2 xl:order-1">
        <div className="sticky top-24 space-y-6 rounded-2xl border bg-card p-4 shadow-sm">
          <div>
            <div className="flex items-center justify-between gap-3">
              <h2 className="flex items-center gap-2 text-sm font-semibold">
                <Filter className="size-4 text-primary" aria-hidden />
                缩小结果
              </h2>
              {hasFilters ? (
                <button
                  type="button"
                  onClick={clearFilters}
                  className="text-[11px] font-medium text-primary hover:underline"
                >
                  全部重置
                </button>
              ) : null}
            </div>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              聚合数量来自当前查询，不扫描页面 AST。
            </p>
          </div>

          <FacetGroup
            title="命名空间"
            icon={BookOpenText}
            values={result?.facets.namespaces ?? []}
            selected={namespace}
            onSelect={(value) => updateFacet(setNamespace, value)}
          />
          <FacetGroup
            title="语言"
            icon={Languages}
            values={result?.facets.languages ?? []}
            selected={language}
            onSelect={(value) => updateFacet(setLanguage, value)}
          />
          <FacetGroup
            title="Entity 类型"
            icon={Waypoints}
            values={result?.facets.entityTypes ?? []}
            selected={entityType}
            onSelect={(value) => updateFacet(setEntityType, value)}
          />

          <section className="border-t pt-5">
            <h3 className="flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              <Tags className="size-3.5" aria-hidden />
              搜索字段
            </h3>
            <div className="mt-3 grid grid-cols-2 gap-2">
              {FIELD_META.map((field) => (
                <label
                  key={field.value}
                  className="flex cursor-pointer items-center gap-2 rounded-lg border px-2.5 py-2 text-xs"
                >
                  <Checkbox
                    checked={fields.has(field.value)}
                    onCheckedChange={(checked) =>
                      toggleField(field.value, checked === true)
                    }
                    aria-label={`搜索${field.label}`}
                  />
                  {field.label}
                </label>
              ))}
            </div>
          </section>
        </div>
      </aside>

      <main className="order-1 min-w-0 xl:order-2">
        <section className="rounded-2xl border bg-card p-4 shadow-sm sm:p-5">
          <form onSubmit={submit} className="flex gap-2">
            <div className="relative min-w-0 flex-1">
              <Search
                className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden
              />
              <Input
                value={input}
                onChange={(event) => setInput(event.target.value)}
                className="h-12 pl-10 text-base"
                placeholder="试试：擅长机械维修的角色、可验证知识、都市奇幻作品…"
                aria-label="探索 Wiki"
              />
            </div>
            <Button type="submit" size="lg" className="h-12 px-5">
              探索
            </Button>
          </form>
          {validationMessage ? (
            <p className="mt-2 text-xs text-destructive">{validationMessage}</p>
          ) : null}

          <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            {(Object.keys(MODE_META) as Array<SearchMode>).map((item) => {
              const meta = MODE_META[item];
              const Icon = meta.icon;
              const target =
                item === "auto"
                  ? capabilities.data?.defaultMode ?? "keyword"
                  : item;
              const enabled =
                item === "auto" ||
                target === "keyword" ||
                availableModes.has(target);
              const active = mode === item;
              return (
                <button
                  key={item}
                  type="button"
                  onClick={() => chooseMode(item)}
                  disabled={!enabled}
                  className={cn(
                    "flex min-h-16 items-center gap-3 rounded-xl border px-3 text-left transition-colors",
                    active
                      ? "border-primary/35 bg-primary/8 text-foreground"
                      : "text-muted-foreground hover:bg-accent",
                    !enabled && "cursor-not-allowed opacity-45 hover:bg-transparent",
                  )}
                >
                  <span
                    className={cn(
                      "flex size-8 shrink-0 items-center justify-center rounded-lg",
                      active ? "bg-primary text-primary-foreground" : "bg-muted",
                    )}
                  >
                    <Icon className="size-4" aria-hidden />
                  </span>
                  <span className="min-w-0">
                    <span className="flex items-center gap-1.5 text-xs font-semibold">
                      {meta.label}
                      {active ? <Check className="size-3" aria-hidden /> : null}
                    </span>
                    <span className="mt-0.5 block truncate text-[10px]">
                      {meta.detail}
                    </span>
                  </span>
                </button>
              );
            })}
          </div>

          {effectiveMode === "hybrid" && modeAvailable ? (
            <div className="mt-4 flex flex-wrap items-center gap-4 rounded-xl bg-muted/55 px-4 py-3">
              <label htmlFor="semantic-ratio" className="text-xs font-medium">
                语义权重
              </label>
              <input
                id="semantic-ratio"
                type="range"
                min="0.1"
                max="0.9"
                step="0.1"
                value={semanticRatio}
                onChange={(event) => {
                  setSemanticRatio(Number(event.target.value));
                  setOffset(0);
                }}
                className="min-w-40 flex-1 accent-primary"
              />
              <span className="w-10 text-right font-mono text-xs tabular-nums">
                {Math.round(semanticRatio * 100)}%
              </span>
              <span className="text-[11px] text-muted-foreground">
                左侧偏精确，右侧偏概念
              </span>
            </div>
          ) : null}
        </section>

        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 px-1">
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1.5 rounded-full border bg-background px-2.5 py-1">
              <Cpu className="size-3" aria-hidden />
              {capabilities.isLoading
                ? "读取搜索能力…"
                : backendLabel(capabilities.data)}
            </span>
            {result ? (
              <span>
                {result.total.toLocaleString("zh-CN")} 条结果 ·{" "}
                {MODE_META[result.mode].label}模式
              </span>
            ) : null}
          </div>
          {search.isLoading || search.isValidating ? (
            <span className="inline-flex items-center gap-2 text-xs text-muted-foreground">
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
              正在检索
            </span>
          ) : null}
        </div>

        {capabilities.error || search.error ? (
          <div className="mt-4 flex gap-3 rounded-2xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
            <div>
              <p className="font-medium">暂时无法完成搜索</p>
              <p className="mt-1 text-xs opacity-85">
                请稍后重试；若当前环境未启用独立索引，可切换到精确模式。
              </p>
            </div>
          </div>
        ) : null}

        {!modeAvailable ? (
          <div className="mt-4 flex gap-3 rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
            当前环境未配置此语义模式。请选择“智能”或“精确”后继续。
          </div>
        ) : null}

        {!query ? (
          <div className="mt-4 rounded-3xl border border-dashed bg-muted/25 px-6 py-16 text-center">
            <span className="mx-auto flex size-12 items-center justify-center rounded-2xl bg-primary/9 text-primary">
              <Search className="size-5" aria-hidden />
            </span>
            <h2 className="mt-4 text-lg font-semibold">从一个问题或主题开始</h2>
            <p className="mx-auto mt-2 max-w-lg text-sm leading-6 text-muted-foreground">
              结果会带来源字段、Entity 入口和可组合聚合；所有命中都来自当前已发布 Revision
              的可重建搜索投影。
            </p>
          </div>
        ) : null}

        {query && result && result.items.length === 0 && !search.isLoading ? (
          <div className="mt-4 rounded-3xl border border-dashed px-6 py-16 text-center">
            <h2 className="text-lg font-semibold">没有找到匹配页面</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              尝试清除筛选、降低语义权重，或换一种描述方式。
            </p>
            {hasFilters ? (
              <Button variant="outline" className="mt-5" onClick={clearFilters}>
                <RotateCcw className="size-4" aria-hidden />
                清除筛选
              </Button>
            ) : null}
          </div>
        ) : null}

        {result && result.items.length > 0 ? (
          <ol className="mt-4 space-y-3">
            {result.items.map((hit, index) => (
              <li
                key={hit.id}
                className="group rounded-2xl border bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)] transition-all hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-lg hover:shadow-slate-950/5"
              >
                <div className="flex gap-4">
                  <span className="mt-0.5 flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/8 text-primary">
                    <FileText className="size-4.5" aria-hidden />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                      <span className="text-[10px] font-medium tabular-nums text-muted-foreground">
                        #{offset + index + 1}
                      </span>
                      <Link
                        href={`/pages/${hit.id}`}
                        className="text-lg font-semibold tracking-[-0.02em] group-hover:text-primary"
                      >
                        {hit.displayTitle}
                      </Link>
                      <span className="rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                        {MATCH_LABEL[hit.matchedOn]}相关
                      </span>
                    </div>
                    <p className="mt-2 line-clamp-3 text-sm leading-6 text-muted-foreground">
                      <SearchHighlight value={hit.highlight} />
                    </p>
                    <div className="mt-3 flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                      <span>{hit.namespace}</span>
                      <span aria-hidden>·</span>
                      <span>{hit.language}</span>
                      {hit.entityType ? (
                        <>
                          <span aria-hidden>·</span>
                          <span className="inline-flex items-center gap-1">
                            <Braces className="size-3" aria-hidden />
                            {hit.entityType}
                          </span>
                        </>
                      ) : null}
                      {hit.entityId ? (
                        <>
                          <span aria-hidden>·</span>
                          <Link
                            href={`/entities/${hit.entityId}`}
                            className="inline-flex items-center gap-1 font-medium text-primary hover:underline"
                          >
                            <Waypoints className="size-3" aria-hidden />
                            查看 Entity
                          </Link>
                        </>
                      ) : null}
                    </div>
                  </div>
                  <ArrowRight
                    className="mt-2 hidden size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-1 group-hover:text-primary sm:block"
                    aria-hidden
                  />
                </div>
              </li>
            ))}
          </ol>
        ) : null}

        {result && result.total > PAGE_SIZE ? (
          <nav
            className="mt-5 flex items-center justify-between rounded-2xl border bg-card p-3"
            aria-label="搜索结果分页"
          >
            <Button
              variant="ghost"
              disabled={offset === 0}
              onClick={() => setOffset((current) => Math.max(0, current - PAGE_SIZE))}
            >
              <ArrowLeft className="size-4" aria-hidden />
              上一页
            </Button>
            <span className="text-xs text-muted-foreground">
              第 {Math.floor(offset / PAGE_SIZE) + 1} 页
            </span>
            <Button
              variant="ghost"
              disabled={!hasNext}
              onClick={() => setOffset((current) => current + PAGE_SIZE)}
            >
              下一页
              <ArrowRight className="size-4" aria-hidden />
            </Button>
          </nav>
        ) : null}
      </main>
    </div>
  );
}
