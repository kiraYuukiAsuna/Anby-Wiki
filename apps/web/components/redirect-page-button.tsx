"use client";

import { Command } from "cmdk";
import {
  CircleHelp,
  ExternalLink,
  FileText,
  Hash,
  Link2,
  LoaderCircle,
  Route,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  PageRedirectTargetKindEnum,
  ResponseError,
  type PageRedirectTarget,
} from "../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { pagesApi, projectionApi, searchApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";

type RedirectKind = PageRedirectTarget["kind"];
type SelectedPage = { id: string; displayTitle: string };

const pageTargetSchema = z.object({
  pageId: z.string().uuid("请选择有效目标页面"),
});
const sectionTargetSchema = pageTargetSchema.extend({
  anchorBlockId: z.string().uuid("请选择目标章节"),
});
const unresolvedTargetSchema = z.object({
  namespace: z
    .string()
    .trim()
    .min(1, "命名空间不能为空")
    .max(64, "命名空间标识过长"),
  title: z.string().trim().min(1, "目标标题不能为空").max(255, "目标标题过长"),
});
const interwikiTargetSchema = z.object({
  title: z.string().trim().min(1, "请填写外部页面标题").max(255),
  url: z
    .string()
    .trim()
    .url("请输入完整 URL")
    .max(2048)
    .refine(
      (value) => value.startsWith("https://") || value.startsWith("http://"),
      "外部目标必须使用 HTTP(S)",
    ),
});

const MODES: Array<{
  kind: RedirectKind;
  label: string;
  description: string;
  icon: typeof FileText;
}> = [
  {
    kind: PageRedirectTargetKindEnum.Page,
    label: "已有页面",
    description: "跟随页面跳转",
    icon: FileText,
  },
  {
    kind: PageRedirectTargetKindEnum.PageSection,
    label: "页面章节",
    description: "链接到指定章节",
    icon: Hash,
  },
  {
    kind: PageRedirectTargetKindEnum.Unresolved,
    label: "待创建页面标题",
    description: "页面创建后跳转",
    icon: CircleHelp,
  },
  {
    kind: PageRedirectTargetKindEnum.Interwiki,
    label: "外部 Wiki",
    description: "打开外部链接",
    icon: ExternalLink,
  },
];

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}

export function RedirectPageButton({
  sourcePageId,
  sourceTitle,
  initialTarget,
  onSaved,
  label = "重定向",
}: {
  sourcePageId: string;
  sourceTitle: string;
  initialTarget?: PageRedirectTarget;
  onSaved?: () => void | Promise<void>;
  label?: string;
}) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<RedirectKind>(
    initialTarget?.kind ?? PageRedirectTargetKindEnum.Page,
  );
  const [query, setQuery] = useState("");
  const [targetPage, setTargetPage] = useState<SelectedPage>();
  const [anchorBlockId, setAnchorBlockId] = useState("");
  const [namespace, setNamespace] = useState("main");
  const [targetTitle, setTargetTitle] = useState("");
  const [externalURL, setExternalURL] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const debounced = useDebouncedValue(query.trim(), 200);
  const pageMode =
    kind === PageRedirectTargetKindEnum.Page ||
    kind === PageRedirectTargetKindEnum.PageSection;

  const searchState = useSWR(
    open && pageMode && debounced
      ? ["redirect-target-search", sourcePageId, debounced]
      : null,
    () =>
      searchApi().searchPages({
        q: debounced,
        fields: ["title", "alias"],
        limit: 12,
      }),
    { keepPreviousData: true },
  );
  const hits = (searchState.data?.items ?? []).filter(
    (hit) => hit.id !== sourcePageId,
  );
  const outlineState = useSWR(
    open &&
      kind === PageRedirectTargetKindEnum.PageSection &&
      targetPage?.id
      ? ["redirect-target-outline", targetPage.id]
      : null,
    () => projectionApi().getPageOutline({ id: targetPage!.id }),
  );

  const reset = () => {
    const target = initialTarget;
    setKind(target?.kind ?? PageRedirectTargetKindEnum.Page);
    setQuery("");
    setTargetPage(
      target?.targetPageId
        ? {
            id: target.targetPageId,
            displayTitle: target.targetPageTitle ?? target.targetPageId,
          }
        : undefined,
    );
    setAnchorBlockId(target?.anchorBlockId ?? "");
    setNamespace(target?.namespace ?? "main");
    setTargetTitle(target?.targetTitle ?? "");
    setExternalURL(target?.externalUrl ?? "");
  };

  const selectKind = (next: RedirectKind) => {
    setKind(next);
    setQuery("");
    setTargetPage(undefined);
    setAnchorBlockId("");
  };

  const submit = async () => {
    let target: PageRedirectTarget;
    if (kind === PageRedirectTargetKindEnum.Page) {
      const parsed = pageTargetSchema.safeParse({ pageId: targetPage?.id });
      if (!parsed.success) {
        toast.error(parsed.error.issues[0]?.message ?? "请选择目标页面");
        return;
      }
      target = { kind, targetPageId: parsed.data.pageId };
    } else if (kind === PageRedirectTargetKindEnum.PageSection) {
      const parsed = sectionTargetSchema.safeParse({
        pageId: targetPage?.id,
        anchorBlockId,
      });
      if (!parsed.success) {
        toast.error(parsed.error.issues[0]?.message ?? "请选择目标章节");
        return;
      }
      target = {
        kind,
        targetPageId: parsed.data.pageId,
        anchorBlockId: parsed.data.anchorBlockId,
      };
    } else if (kind === PageRedirectTargetKindEnum.Unresolved) {
      const parsed = unresolvedTargetSchema.safeParse({
        namespace,
        title: targetTitle,
      });
      if (!parsed.success) {
        toast.error(parsed.error.issues[0]?.message ?? "请检查目标标题");
        return;
      }
      target = {
        kind,
        namespace: parsed.data.namespace,
        targetTitle: parsed.data.title,
      };
    } else {
      const parsed = interwikiTargetSchema.safeParse({
        title: targetTitle,
        url: externalURL,
      });
      if (!parsed.success) {
        toast.error(parsed.error.issues[0]?.message ?? "请检查外部目标");
        return;
      }
      target = {
        kind,
        targetTitle: parsed.data.title,
        externalUrl: parsed.data.url,
      };
    }

    setSubmitting(true);
    try {
      await pagesApi().createPageRedirect({
        id: sourcePageId,
        createPageRedirectRequest: { target },
      });
      setOpen(false);
      toast.success(`「${sourceTitle}」的重定向已保存`, {
        description:
          kind === PageRedirectTargetKindEnum.Unresolved
            ? "目标页面创建后，这个链接会自动指向它。"
            : "跳转目标已保存，修改前的设置已记录。",
      });
      await onSaved?.();
      router.refresh();
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("请先登录后管理重定向");
        router.push(LOGIN_PATH);
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("你没有编辑这个页面的权限");
      } else if (error instanceof ResponseError && error.response.status === 410) {
        toast.error("目标页面已被删除");
      } else if (
        error instanceof ResponseError &&
        (error.response.status === 400 || error.response.status === 422)
      ) {
        toast.error("重定向目标未通过校验", {
          description: "请核对章节是否仍存在，以及目标链是否形成循环。",
        });
      } else {
        toast.error("重定向未保存，请稍后重试");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => {
          reset();
          setOpen(true);
        }}
      >
        <Route aria-hidden />
        {label}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>管理页面重定向</DialogTitle>
            <DialogDescription>
              为「{sourceTitle}」设置跳转目标：已有页面、指定章节、待创建页面或外部网站。
            </DialogDescription>
          </DialogHeader>

          <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
            {MODES.map((mode) => {
              const Icon = mode.icon;
              const active = kind === mode.kind;
              return (
                <button
                  key={mode.kind}
                  type="button"
                  onClick={() => selectKind(mode.kind)}
                  className={`rounded-xl border p-3 text-left transition-colors ${
                    active
                      ? "border-primary bg-primary/5 text-foreground"
                      : "border-border hover:bg-muted/50"
                  }`}
                >
                  <Icon
                    className={`size-4 ${active ? "text-primary" : "text-muted-foreground"}`}
                    aria-hidden
                  />
                  <span className="mt-2 block text-xs font-semibold">{mode.label}</span>
                  <span className="mt-1 block text-[10px] leading-4 text-muted-foreground">
                    {mode.description}
                  </span>
                </button>
              );
            })}
          </div>

          {pageMode ? (
            targetPage ? (
              <div className="rounded-xl border border-primary/20 bg-primary/5 p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="text-xs font-medium text-muted-foreground">
                      目标页面
                    </p>
                    <p className="mt-1 truncate font-semibold">
                      {targetPage.displayTitle}
                    </p>
                    <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
                      {targetPage.id}
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setTargetPage(undefined);
                      setAnchorBlockId("");
                    }}
                  >
                    重新选择
                  </Button>
                </div>
                {kind === PageRedirectTargetKindEnum.PageSection ? (
                  <div className="mt-4 border-t border-primary/15 pt-4">
                    <Label>指定章节</Label>
                    {outlineState.isLoading ? (
                      <p className="mt-3 flex items-center gap-2 text-xs text-muted-foreground">
                        <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
                        读取章节 ID…
                      </p>
                    ) : outlineState.error ? (
                      <p className="mt-3 text-xs text-destructive">
                        暂时无法加载章节列表，请稍后重试。
                      </p>
                    ) : outlineState.data?.items.length ? (
                      <div className="mt-2 max-h-48 space-y-1 overflow-y-auto pr-1">
                        {outlineState.data.items.map((item) => (
                          <button
                            key={item.headingBlockId}
                            type="button"
                            onClick={() => setAnchorBlockId(item.headingBlockId)}
                            className={`flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs ${
                              anchorBlockId === item.headingBlockId
                                ? "bg-primary text-primary-foreground"
                                : "hover:bg-background"
                            }`}
                            style={{ paddingLeft: `${Math.max(8, item.level * 10)}px` }}
                          >
                            <Hash className="size-3 shrink-0" aria-hidden />
                            <span className="min-w-0 flex-1 truncate">{item.title}</span>
                            <span className="font-mono text-[9px] opacity-70">
                              {item.positionKey}
                            </span>
                          </button>
                        ))}
                      </div>
                    ) : (
                      <p className="mt-3 text-xs text-muted-foreground">
                        这个页面还没有可选章节，请改为链接到整个页面。
                      </p>
                    )}
                  </div>
                ) : null}
              </div>
            ) : (
              <Command shouldFilter={false} label="选择重定向目标页面">
                <Command.Input
                  value={query}
                  onValueChange={setQuery}
                  placeholder="搜索任何命名空间的页面标题或别名…"
                  autoFocus
                  className="flex h-9 w-full rounded-lg border border-input bg-transparent px-3 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                />
                <Command.List className="mt-2 max-h-64 overflow-y-auto">
                  {searchState.isLoading && !hits.length ? (
                    <Command.Loading className="px-3 py-6 text-center text-sm text-muted-foreground">
                      搜索中…
                    </Command.Loading>
                  ) : null}
                  {!searchState.isLoading && query.trim() && !hits.length ? (
                    <Command.Empty className="px-3 py-6 text-center text-sm text-muted-foreground">
                      没有可用的目标页面
                    </Command.Empty>
                  ) : null}
                  {hits.map((hit) => (
                    <Command.Item
                      key={hit.id}
                      value={hit.id}
                      onSelect={() =>
                        setTargetPage({
                          id: hit.id,
                          displayTitle: hit.displayTitle,
                        })
                      }
                      className="flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-sm data-[selected=true]:bg-accent"
                    >
                      <FileText className="size-4 text-muted-foreground" aria-hidden />
                      <span className="min-w-0">
                        <span className="block truncate font-medium">
                          {hit.displayTitle}
                        </span>
                        <span className="block truncate text-[11px] text-muted-foreground">
                          {hit.namespace} · {hit.id}
                        </span>
                      </span>
                    </Command.Item>
                  ))}
                </Command.List>
              </Command>
            )
          ) : kind === PageRedirectTargetKindEnum.Unresolved ? (
            <div className="grid gap-4 rounded-xl border border-border bg-muted/20 p-4 sm:grid-cols-[10rem_1fr]">
              <div className="space-y-2">
                <Label htmlFor="redirect-namespace">命名空间标识</Label>
                <Input
                  id="redirect-namespace"
                  value={namespace}
                  onChange={(event) => setNamespace(event.target.value)}
                  placeholder="main"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="redirect-title">尚未解析的标题</Label>
                <Input
                  id="redirect-title"
                  value={targetTitle}
                  onChange={(event) => setTargetTitle(event.target.value)}
                  placeholder="未来可能创建的页面标题"
                />
              </div>
              <p className="text-xs leading-5 text-muted-foreground sm:col-span-2">
                填写目标页面的标题。页面创建后，这个链接会自动指向它。
              </p>
            </div>
          ) : (
            <div className="space-y-4 rounded-xl border border-border bg-muted/20 p-4">
              <div className="space-y-2">
                <Label htmlFor="redirect-external-title">外部页面标题</Label>
                <Input
                  id="redirect-external-title"
                  value={targetTitle}
                  onChange={(event) => setTargetTitle(event.target.value)}
                  placeholder="例如：维基百科"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="redirect-external-url">完整外部 Wiki URL</Label>
                <Input
                  id="redirect-external-url"
                  type="url"
                  value={externalURL}
                  onChange={(event) => setExternalURL(event.target.value)}
                  placeholder="https://example.org/wiki/Target"
                />
              </div>
              <p className="flex items-start gap-2 text-xs leading-5 text-muted-foreground">
                <Link2 className="mt-0.5 size-3.5 shrink-0" aria-hidden />
                请输入不含账号或密码的 HTTP(S) 链接。读者打开时会看到离站提示。
              </p>
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={submitting}
              onClick={() => setOpen(false)}
            >
              取消
            </Button>
            <Button type="button" disabled={submitting} onClick={() => void submit()}>
              {submitting ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Route aria-hidden />
              )}
              {submitting ? "验证并保存中…" : "保存重定向"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
