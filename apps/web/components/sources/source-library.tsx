"use client";

import Link from "next/link";
import {
  ArrowRight,
  BookText,
  Database,
  FileText,
  Globe2,
  ImageIcon,
  Inbox,
  LoaderCircle,
  Plus,
  RefreshCw,
  Search,
  Video,
} from "lucide-react";
import { FormEvent, useState } from "react";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";
import { z } from "zod";

import type {
  EvidenceSource,
  EvidenceSourceListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { sourcesApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { compactId } from "@/lib/display-id";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 24;
type SourceType = EvidenceSource["sourceType"];
type Filter = "all" | SourceType;

const TYPES: Array<{ value: SourceType; label: string }> = [
  { value: "webpage", label: "网页" },
  { value: "pdf", label: "PDF" },
  { value: "book", label: "书籍" },
  { value: "image", label: "图片" },
  { value: "video", label: "视频" },
  { value: "api", label: "API" },
  { value: "database", label: "数据库" },
];

const TYPE_META: Record<
  SourceType,
  { label: string; icon: typeof Globe2; color: string }
> = {
  webpage: { label: "网页", icon: Globe2, color: "bg-sky-100 text-sky-700" },
  pdf: { label: "PDF", icon: FileText, color: "bg-rose-100 text-rose-700" },
  book: { label: "书籍", icon: BookText, color: "bg-amber-100 text-amber-700" },
  image: { label: "图片", icon: ImageIcon, color: "bg-violet-100 text-violet-700" },
  video: { label: "视频", icon: Video, color: "bg-fuchsia-100 text-fuchsia-700" },
  api: { label: "API", icon: Database, color: "bg-emerald-100 text-emerald-700" },
  database: {
    label: "数据库",
    icon: Database,
    color: "bg-teal-100 text-teal-700",
  },
};

const formSchema = z.object({
  title: z.string().trim().min(1, "请填写来源标题"),
  url: z.union([z.literal(""), z.string().url("URL 必须是完整的 http/https 地址")]),
});

function SourceCard({ source }: { source: EvidenceSource }) {
  const meta = TYPE_META[source.sourceType];
  const Icon = meta.icon;
  return (
    <li className="group rounded-2xl border bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)] transition hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-md">
      <div className="flex items-start gap-4">
        <span
          className={cn(
            "flex size-10 shrink-0 items-center justify-center rounded-xl",
            meta.color,
          )}
        >
          <Icon className="size-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-[11px] font-semibold tracking-[0.12em] text-muted-foreground uppercase">
            {meta.label}
          </p>
          <Link
            href={`/sources/${source.id}`}
            className="mt-1 flex items-start gap-2 font-semibold leading-6 group-hover:text-primary"
          >
            <span className="line-clamp-2">{source.title}</span>
            <ArrowRight
              className="mt-1 size-3.5 shrink-0 transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </Link>
          <p className="mt-3 line-clamp-1 text-xs text-muted-foreground">
            {[source.author, source.publisher].filter(Boolean).join(" · ") ||
              "尚未登记作者或发布者"}
          </p>
          <p className="mt-3 font-mono text-[10px] text-muted-foreground">
            {compactId(source.id)}
          </p>
        </div>
      </div>
    </li>
  );
}

export function SourceLibrary() {
  const [filter, setFilter] = useState<Filter>("all");
  const [searchInput, setSearchInput] = useState("");
  const [query, setQuery] = useState("");
  const [sourceType, setSourceType] = useState<SourceType>("webpage");
  const [title, setTitle] = useState("");
  const [url, setURL] = useState("");
  const [author, setAuthor] = useState("");
  const [publisher, setPublisher] = useState("");
  const [publishedAt, setPublishedAt] = useState("");
  const [metadata, setMetadata] = useState("{}");
  const [creating, setCreating] = useState(false);
  const { isAuthenticated, isLoading: sessionLoading } = useSession();

  const state = useSWRInfinite<EvidenceSourceListPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "sources",
        filter,
        query,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, selectedType, selectedQuery, cursor] = cacheKey as readonly [
        string,
        Filter,
        string,
        string,
      ];
      return sourcesApi().listSources({
        sourceType: selectedType === "all" ? undefined : selectedType,
        q: selectedQuery || undefined,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
    { revalidateFirstPage: true },
  );

  const items = state.data?.flatMap((page) => page.items) ?? [];
  const lastPage = state.data?.[state.data.length - 1];
  const canLoadMore = Boolean(state.data && lastPage?.nextCursor);

  const search = (event: FormEvent) => {
    event.preventDefault();
    setQuery(searchInput.trim());
    void state.setSize(1);
  };

  const create = async (event: FormEvent) => {
    event.preventDefault();
    const parsed = formSchema.safeParse({ title, url });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查来源信息");
      return;
    }
    let metadataObject: Record<string, unknown>;
    try {
      const value: unknown = JSON.parse(metadata);
      if (!value || Array.isArray(value) || typeof value !== "object") {
        throw new Error("metadata must be object");
      }
      metadataObject = value as Record<string, unknown>;
    } catch {
      toast.error("Metadata 必须是 JSON object");
      return;
    }
    setCreating(true);
    try {
      const created = await sourcesApi().createSource({
        createEvidenceSourceRequest: {
          sourceType,
          title: parsed.data.title,
          url: parsed.data.url || undefined,
          author: author.trim() || undefined,
          publisher: publisher.trim() || undefined,
          publishedAt: publishedAt
            ? new Date(`${publishedAt}T00:00:00.000Z`)
            : undefined,
          metadata: metadataObject,
        },
      });
      setTitle("");
      setURL("");
      setAuthor("");
      setPublisher("");
      setPublishedAt("");
      setMetadata("{}");
      await state.mutate();
      toast.success("来源已登记", {
        description: `稳定 ID ${compactId(created.id)}`,
      });
    } catch {
      toast.error("来源登记失败", {
        description: "请检查 URL、关联对象与账户权限。",
      });
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_21rem]">
      <section aria-labelledby="source-directory-title">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h2 id="source-directory-title" className="text-xl font-semibold">
              来源目录
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              搜索只读取元数据，不在请求时扫描来源全文。
            </p>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={state.isValidating}
            onClick={() => void state.mutate()}
          >
            <RefreshCw
              className={cn(
                "size-3.5",
                state.isValidating && "animate-spin",
              )}
              aria-hidden
            />
            刷新
          </Button>
        </div>

        <form onSubmit={search} className="mt-5 flex gap-2">
          <div className="relative flex-1">
            <Search
              className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
              aria-hidden
            />
            <Input
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              className="pl-9"
              placeholder="搜索标题、作者或发布者"
              maxLength={200}
            />
          </div>
          <Button type="submit" variant="outline">
            搜索
          </Button>
        </form>

        <div className="mt-4 flex flex-wrap gap-1 rounded-xl bg-muted/65 p-1">
          {[{ value: "all" as const, label: "全部" }, ...TYPES].map((item) => (
            <button
              key={item.value}
              type="button"
              aria-pressed={filter === item.value}
              onClick={() => {
                setFilter(item.value);
                void state.setSize(1);
              }}
              className={cn(
                "rounded-lg px-3 py-1.5 text-xs font-medium text-muted-foreground transition",
                filter === item.value &&
                  "bg-background text-foreground shadow-sm ring-1 ring-border/70",
              )}
            >
              {item.label}
            </button>
          ))}
        </div>

        {state.error ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            来源目录暂时不可用。
          </div>
        ) : null}
        {state.isLoading && !state.data ? (
          <div className="mt-5 grid gap-4 md:grid-cols-2">
            {[0, 1, 2, 3].map((item) => (
              <div
                key={item}
                className="h-36 animate-pulse rounded-2xl border bg-muted/35"
              />
            ))}
          </div>
        ) : null}
        {!state.isLoading && !state.error && items.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
            <Inbox className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">没有匹配来源</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              可从右侧登记来源，或前往 AI 导入中心抓取并生成版本。
            </p>
            <Button asChild variant="outline" className="mt-5">
              <Link href="/imports">打开导入中心</Link>
            </Button>
          </div>
        ) : null}
        {items.length > 0 ? (
          <ol className="mt-5 grid gap-4 md:grid-cols-2">
            {items.map((source) => (
              <SourceCard key={source.id} source={source} />
            ))}
          </ol>
        ) : null}
        {canLoadMore ? (
          <Button
            type="button"
            variant="outline"
            className="mt-5 w-full"
            disabled={state.isValidating}
            onClick={() => void state.setSize(state.size + 1)}
          >
            {state.isValidating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : null}
            加载更多来源
          </Button>
        ) : null}
      </section>

      <aside className="xl:sticky xl:top-24 xl:self-start">
        <form onSubmit={create} className="rounded-2xl border bg-card p-5">
          <span className="flex size-9 items-center justify-center rounded-xl bg-emerald-100 text-emerald-700">
            <Plus className="size-4" aria-hidden />
          </span>
          <h2 className="mt-4 font-semibold">登记逻辑来源</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            来源身份可长期复用；后续每次获取内容都会形成不可变 SourceVersion。
          </p>
          <div className="mt-5 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="source-type">类型</Label>
              <select
                id="source-type"
                value={sourceType}
                onChange={(event) =>
                  setSourceType(event.target.value as SourceType)
                }
                className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm"
              >
                {TYPES.map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="source-title">标题</Label>
              <Input
                id="source-title"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                maxLength={500}
                placeholder="来源标题"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="source-url">URL（可选）</Label>
              <Input
                id="source-url"
                value={url}
                onChange={(event) => setURL(event.target.value)}
                placeholder="https://…"
                type="url"
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="source-author">作者</Label>
                <Input
                  id="source-author"
                  value={author}
                  onChange={(event) => setAuthor(event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="source-publisher">发布者</Label>
                <Input
                  id="source-publisher"
                  value={publisher}
                  onChange={(event) => setPublisher(event.target.value)}
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="source-date">发布日期</Label>
              <Input
                id="source-date"
                value={publishedAt}
                onChange={(event) => setPublishedAt(event.target.value)}
                type="date"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="source-metadata">Metadata JSON</Label>
              <Textarea
                id="source-metadata"
                value={metadata}
                onChange={(event) => setMetadata(event.target.value)}
                className="min-h-20 font-mono text-xs"
                spellCheck={false}
              />
            </div>
          </div>
          <Button
            type="submit"
            className="mt-5 w-full"
            disabled={!isAuthenticated || sessionLoading || creating}
          >
            {creating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : (
              <Plus aria-hidden />
            )}
            登记来源
          </Button>
          {!sessionLoading && !isAuthenticated ? (
            <p className="mt-3 text-center text-[11px] text-muted-foreground">
              登录后可登记来源。
            </p>
          ) : null}
        </form>
      </aside>
    </div>
  );
}
