"use client";

import { Command } from "cmdk";
import {
  ArrowRight,
  CornerDownRight,
  FileText,
  Link2,
  LoaderCircle,
  LockKeyhole,
  Route,
  Search,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type BlockRedirect,
  type PageSearchHit,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { pagesApi, projectionApi, searchApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const redirectSchema = z.object({
  sourceBlockId: z.string().uuid("来源 Block ID 不是合法 UUID"),
  targetPageId: z.string().uuid("请选择目标页面"),
  targetBlockId: z.string().uuid("请选择目标章节"),
});

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}

type TargetPage = {
  id: string;
  title: string;
};

export function BlockRedirectManager({
  pageId,
  pageTitle,
}: {
  pageId: string;
  pageTitle: string;
}) {
  const router = useRouter();
  const session = useSession();
  const [sourceBlockId, setSourceBlockId] = useState("");
  const [query, setQuery] = useState("");
  const [targetPage, setTargetPage] = useState<TargetPage>();
  const [targetBlockId, setTargetBlockId] = useState("");
  const [saving, setSaving] = useState(false);
  const [deletingBlockId, setDeletingBlockId] = useState<string>();
  const debouncedQuery = useDebouncedValue(query.trim(), 220);

  const redirects = useSWR(
    ["page:block-redirects", pageId],
    () => pagesApi().listBlockRedirects({ id: pageId }),
    { revalidateOnFocus: false },
  );
  const sourceOutline = useSWR(
    ["page:outline", pageId],
    () => projectionApi().getPageOutline({ id: pageId }),
    { revalidateOnFocus: false },
  );
  const pageSearch = useSWR(
    debouncedQuery
      ? ["block-redirect:target-search", pageId, debouncedQuery]
      : null,
    () =>
      searchApi().searchPages({
        q: debouncedQuery,
        namespace: "main",
        fields: ["title", "alias"],
        limit: 12,
      }),
    { keepPreviousData: true },
  );
  const targetOutline = useSWR(
    targetPage ? ["page:outline", targetPage.id] : null,
    () => projectionApi().getPageOutline({ id: targetPage!.id }),
    { revalidateOnFocus: false },
  );

  const chooseTargetPage = (page: TargetPage) => {
    setTargetPage(page);
    setTargetBlockId("");
    setQuery("");
  };

  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!session.isAuthenticated) {
      toast.error("请先登录后管理章节迁移");
      router.push(LOGIN_PATH);
      return;
    }
    const parsed = redirectSchema.safeParse({
      sourceBlockId: sourceBlockId.trim(),
      targetPageId: targetPage?.id,
      targetBlockId,
    });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查章节映射");
      return;
    }

    setSaving(true);
    try {
      const result = await pagesApi().upsertBlockRedirect({
        id: pageId,
        blockId: parsed.data.sourceBlockId,
        upsertBlockRedirectRequest: {
          targetPageId: parsed.data.targetPageId,
          targetBlockId: parsed.data.targetBlockId,
        },
      });
      await redirects.mutate();
      setSourceBlockId("");
      setTargetPage(undefined);
      setTargetBlockId("");
      toast.success("稳定章节迁移已保存", {
        description: `旧地址会前往「${result.targetPageTitle}」#${result.targetCurrentSlug}`,
      });
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
        router.push(LOGIN_PATH);
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("当前账号没有页面编辑权限");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 404
      ) {
        toast.error("目标页面或目标章节已不存在");
      } else {
        toast.error("章节迁移未保存", {
          description: "服务端会拒绝迁移环、自指向与悬空目标。",
        });
      }
    } finally {
      setSaving(false);
    }
  };

  const remove = async (redirect: BlockRedirect) => {
    if (!session.isAuthenticated) {
      router.push(LOGIN_PATH);
      return;
    }
    setDeletingBlockId(redirect.sourceBlockId);
    try {
      await pagesApi().deleteBlockRedirect({
        id: pageId,
        blockId: redirect.sourceBlockId,
      });
      await redirects.mutate();
      toast.success("章节迁移已删除", {
        description: "历史审计仍然保留，旧章节地址将不再跟随该映射。",
      });
    } catch (error) {
      if (isUnauthorized(error)) {
        router.push(LOGIN_PATH);
      } else {
        toast.error("章节迁移删除失败");
      }
    } finally {
      setDeletingBlockId(undefined);
    }
  };

  return (
    <div className="space-y-8">
      <section className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_19rem]">
        <div className="rounded-3xl border bg-gradient-to-br from-card to-cyan-50/35 p-6">
          <div className="flex items-start gap-4">
            <span className="flex size-11 shrink-0 items-center justify-center rounded-2xl bg-cyan-100 text-cyan-800">
              <Route className="size-5" aria-hidden />
            </span>
            <div>
              <h2 className="text-xl font-semibold tracking-tight">
                稳定章节地址
              </h2>
              <p className="mt-2 max-w-2xl text-sm leading-7 text-muted-foreground">
                当标题章节被删除、拆分或移动后，把旧 Heading Block ID
                显式映射到当前有效章节。服务端会折叠迁移链并拒绝环。
              </p>
            </div>
          </div>
        </div>
        <Link
          href={`/governance/protections?page_id=${pageId}`}
          className="group rounded-3xl border bg-card p-5 transition hover:border-primary/25 hover:bg-primary/[0.035]"
        >
          <LockKeyhole className="size-5 text-primary" aria-hidden />
          <p className="mt-5 font-semibold">页面保护策略</p>
          <p className="mt-2 text-xs leading-5 text-muted-foreground">
            限制谁能编辑、改名、审核或应用这个页面的变更。
          </p>
          <span className="mt-5 inline-flex items-center gap-1 text-xs font-semibold text-primary">
            管理保护
            <ArrowRight
              className="size-3.5 transition-transform group-hover:translate-x-0.5"
              aria-hidden
            />
          </span>
        </Link>
      </section>

      <div className="grid gap-8 xl:grid-cols-[26rem_minmax(0,1fr)]">
        <form
          onSubmit={(event) => void save(event)}
          className="h-fit rounded-3xl border bg-card p-5 shadow-[0_14px_40px_-32px_rgb(15_23_42/0.45)]"
        >
          <div className="flex items-center gap-3">
            <span className="flex size-9 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <Link2 className="size-4" aria-hidden />
            </span>
            <div>
              <h2 className="font-semibold">添加或更新映射</h2>
              <p className="text-xs text-muted-foreground">
                相同来源 Block ID 会更新到新目标。
              </p>
            </div>
          </div>

          <div className="mt-6 space-y-2">
            <Label htmlFor="source-block-id">旧 Heading Block ID</Label>
            <Input
              id="source-block-id"
              value={sourceBlockId}
              onChange={(event) => setSourceBlockId(event.target.value)}
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              className="font-mono text-xs"
            />
            {sourceOutline.data?.items.length ? (
              <details>
                <summary className="cursor-pointer text-[11px] font-medium text-primary">
                  从当前页面目录选择
                </summary>
                <div className="mt-2 max-h-36 space-y-1 overflow-y-auto rounded-xl border bg-muted/25 p-1.5">
                  {sourceOutline.data.items.map((item) => (
                    <button
                      key={item.headingBlockId}
                      type="button"
                      onClick={() => setSourceBlockId(item.headingBlockId)}
                      className={cn(
                        "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs hover:bg-accent",
                        sourceBlockId === item.headingBlockId && "bg-accent",
                      )}
                    >
                      <span
                        aria-hidden
                        style={{ width: `${Math.max(0, item.level - 1) * 8}px` }}
                      />
                      <CornerDownRight className="size-3 shrink-0 text-muted-foreground" />
                      <span className="truncate">{item.title}</span>
                    </button>
                  ))}
                </div>
              </details>
            ) : null}
            <p className="text-[10px] leading-4 text-muted-foreground">
              已从当前文档消失的旧 ID 仍可直接粘贴，这是本工具的主要用途。
            </p>
          </div>

          <div className="mt-5 space-y-2">
            <Label>目标页面</Label>
            {targetPage ? (
              <div className="rounded-xl border border-primary/20 bg-primary/5 p-3">
                <p className="truncate text-sm font-semibold">
                  {targetPage.title}
                </p>
                <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
                  {targetPage.id}
                </p>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="mt-2"
                  onClick={() => {
                    setTargetPage(undefined);
                    setTargetBlockId("");
                  }}
                >
                  重新选择
                </Button>
              </div>
            ) : (
              <Command shouldFilter={false} label="选择迁移目标页面">
                <div className="relative">
                  <Search className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground" />
                  <Command.Input
                    value={query}
                    onValueChange={setQuery}
                    placeholder="搜索目标页面…"
                    className="h-8 w-full rounded-lg border border-input bg-transparent pr-2.5 pl-8 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                  />
                </div>
                <Command.List className="mt-2 max-h-44 overflow-y-auto">
                  <Command.Item
                    value={`current:${pageId}`}
                    onSelect={() =>
                      chooseTargetPage({ id: pageId, title: pageTitle })
                    }
                    className="flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-xs data-[selected=true]:bg-accent"
                  >
                    <FileText className="size-3.5 text-primary" aria-hidden />
                    <span className="min-w-0">
                      <span className="block truncate font-medium">
                        {pageTitle}
                      </span>
                      <span className="text-[10px] text-muted-foreground">
                        当前页面
                      </span>
                    </span>
                  </Command.Item>
                  {pageSearch.isLoading ? (
                    <Command.Loading className="px-3 py-5 text-center text-xs text-muted-foreground">
                      搜索中…
                    </Command.Loading>
                  ) : null}
                  {(pageSearch.data?.items ?? []).map(
                    (hit: PageSearchHit) => (
                      <Command.Item
                        key={hit.id}
                        value={hit.id}
                        onSelect={() =>
                          chooseTargetPage({
                            id: hit.id,
                            title: hit.displayTitle,
                          })
                        }
                        className="flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-xs data-[selected=true]:bg-accent"
                      >
                        <FileText
                          className="size-3.5 text-muted-foreground"
                          aria-hidden
                        />
                        <span className="truncate">{hit.displayTitle}</span>
                      </Command.Item>
                    ),
                  )}
                </Command.List>
              </Command>
            )}
          </div>

          {targetPage ? (
            <div className="mt-5 space-y-2">
              <Label>当前有效目标章节</Label>
              {targetOutline.isLoading ? (
                <div className="flex items-center gap-2 rounded-xl border border-dashed px-3 py-5 text-xs text-muted-foreground">
                  <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
                  正在读取目录…
                </div>
              ) : targetOutline.data?.items.length ? (
                <div className="max-h-52 space-y-1 overflow-y-auto rounded-xl border bg-muted/20 p-1.5">
                  {targetOutline.data.items.map((item) => (
                    <button
                      key={item.headingBlockId}
                      type="button"
                      onClick={() => setTargetBlockId(item.headingBlockId)}
                      className={cn(
                        "flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs transition hover:bg-accent",
                        targetBlockId === item.headingBlockId &&
                          "bg-primary/10 text-primary",
                      )}
                    >
                      <span
                        aria-hidden
                        style={{ width: `${Math.max(0, item.level - 1) * 8}px` }}
                      />
                      <CornerDownRight className="size-3 shrink-0" />
                      <span className="min-w-0 flex-1 truncate">
                        {item.title}
                      </span>
                      <span className="shrink-0 font-mono text-[9px] text-muted-foreground">
                        #{item.slug}
                      </span>
                    </button>
                  ))}
                </div>
              ) : (
                <div className="rounded-xl border border-amber-200 bg-amber-50/60 p-3 text-xs text-amber-900">
                  该页面当前没有可作为目标的标题章节。
                </div>
              )}
            </div>
          ) : null}

          <Button
            type="submit"
            className="mt-6 w-full"
            disabled={saving || session.isLoading}
          >
            {saving ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : (
              <Route aria-hidden />
            )}
            {saving ? "正在校验并保存…" : "保存稳定迁移"}
          </Button>
        </form>

        <section>
          <div>
            <h2 className="text-xl font-semibold tracking-tight">现有迁移</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              只展示该页面作为旧地址来源的显式映射。
            </p>
          </div>

          {redirects.isLoading ? (
            <div className="mt-5 flex min-h-48 items-center justify-center gap-2 rounded-2xl border border-dashed text-sm text-muted-foreground">
              <LoaderCircle className="size-4 animate-spin" aria-hidden />
              正在读取章节迁移…
            </div>
          ) : redirects.error ? (
            <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm">
              <TriangleAlert className="size-5 text-destructive" aria-hidden />
              <p className="mt-3 font-semibold">无法读取章节迁移</p>
              <p className="mt-1 text-xs text-muted-foreground">
                请确认页面仍然存在并稍后重试。
              </p>
            </div>
          ) : redirects.data?.items.length ? (
            <ul className="mt-5 space-y-3">
              {redirects.data.items.map((redirect) => (
                <li
                  key={redirect.sourceBlockId}
                  className="rounded-2xl border bg-card p-5"
                >
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div className="min-w-0 flex-1">
                      <p className="text-[10px] font-semibold tracking-[0.12em] text-muted-foreground uppercase">
                        Old stable anchor
                      </p>
                      <p className="mt-2 truncate font-mono text-xs">
                        {redirect.sourceBlockId}
                      </p>
                      <div className="mt-4 flex items-start gap-3 rounded-xl bg-muted/45 p-3">
                        <CornerDownRight
                          className="mt-0.5 size-4 shrink-0 text-primary"
                          aria-hidden
                        />
                        <div className="min-w-0">
                          <Link
                            href={`/pages/${redirect.targetPageId}#${encodeURIComponent(redirect.targetCurrentSlug)}`}
                            className="block truncate text-sm font-semibold hover:text-primary hover:underline"
                          >
                            {redirect.targetPageTitle} #
                            {redirect.targetCurrentSlug}
                          </Link>
                          <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
                            {redirect.targetBlockId}
                          </p>
                        </div>
                      </div>
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={deletingBlockId === redirect.sourceBlockId}
                      onClick={() => void remove(redirect)}
                    >
                      {deletingBlockId === redirect.sourceBlockId ? (
                        <LoaderCircle className="animate-spin" aria-hidden />
                      ) : (
                        <Trash2 aria-hidden />
                      )}
                      删除
                    </Button>
                  </div>
                  <p className="mt-4 border-t pt-4 text-[10px] text-muted-foreground">
                    保存于 {DATE_FORMATTER.format(redirect.createdAt)}
                  </p>
                </li>
              ))}
            </ul>
          ) : (
            <div className="mt-5 rounded-2xl border border-dashed bg-muted/20 px-6 py-12 text-center">
              <Route
                className="mx-auto size-7 text-muted-foreground"
                aria-hidden
              />
              <p className="mt-3 text-sm font-semibold">还没有章节迁移</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                页面重构后如有旧稳定锚点失效，可在左侧建立第一条显式映射。
              </p>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
