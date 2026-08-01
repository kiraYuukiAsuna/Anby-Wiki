"use client";

import { Command } from "cmdk";
import {
  CalendarClock,
  CheckCircle2,
  FileText,
  LoaderCircle,
  LockKeyhole,
  Search,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type CreatePageProtectionRequest,
  type PageProtection,
  type PageSearchHit,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { governanceApi, readingApi, searchApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const ACTIONS = [
  ["edit", "编辑", "发布新的 Revision"],
  ["rename", "改名", "改变页面标题与别名"],
  ["review", "审核", "作出 Proposal 决议"],
  ["apply", "应用", "应用获批 Proposal"],
  ["batch_rollback", "批次回滚", "补偿回滚 ChangeBatch"],
] as const;

const ACTION_LABEL: Record<PageProtection["actionType"], string> = {
  create: "创建",
  edit: "编辑",
  rename: "改名",
  review: "审核",
  apply: "应用",
  batch_rollback: "批次回滚",
};

const uuidSchema = z.string().uuid("请选择合法页面");
const titleScopeSchema = z.object({
  namespaceKey: z
    .string()
    .trim()
    .min(1, "请填写命名空间 key")
    .max(64, "命名空间 key 不能超过 64 个字符"),
  title: z.string().trim().min(1, "请填写待保护标题").max(255),
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

function protectionState(rule: PageProtection, referenceNow: number) {
  if (rule.revokedAt) {
    return {
      label: "已撤销",
      className: "border-slate-200 bg-slate-50 text-slate-600",
      icon: XCircle,
    };
  }
  if (rule.expiresAt && rule.expiresAt.getTime() <= referenceNow) {
    return {
      label: "已到期",
      className: "border-amber-200 bg-amber-50 text-amber-700",
      icon: CalendarClock,
    };
  }
  return {
    label: "生效中",
    className: "border-emerald-200 bg-emerald-50 text-emerald-700",
    icon: CheckCircle2,
  };
}

export function ProtectionWorkspace({
  initialPageId,
}: {
  initialPageId?: string;
}) {
  const router = useRouter();
  const session = useSession();
  const [scope, setScope] = useState<"page" | "title">("page");
  const [query, setQuery] = useState("");
  const [selectedPage, setSelectedPage] = useState<{
    id: string;
    title: string;
  } | undefined>(
    initialPageId
      ? { id: initialPageId, title: "正在读取页面…" }
      : undefined,
  );
  const [namespaceKey, setNamespaceKey] = useState("main");
  const [title, setTitle] = useState("");
  const [action, setAction] =
    useState<(typeof ACTIONS)[number][0]>("edit");
  const [roleKey, setRoleKey] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [showEnded, setShowEnded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [revokingId, setRevokingId] = useState<string>();
  const [referenceNow] = useState(() => Date.now());
  const debouncedQuery = useDebouncedValue(query.trim(), 220);

  const initialPage = useSWR(
    initialPageId && uuidSchema.safeParse(initialPageId).success
      ? ["protection:initial-page", initialPageId]
      : null,
    () => readingApi().getPageByID({ id: initialPageId! }),
    { revalidateOnFocus: false },
  );
  const catalog = useSWR(
    session.isAuthenticated
      ? ["governance:protections", initialPageId ?? ""]
      : null,
    async () => {
      const [rules, roles] = await Promise.all([
        governanceApi().listPageProtections({ includeExpired: true }),
        governanceApi().listRoles(),
      ]);
      return { rules: rules.items, roles: roles.items };
    },
    {
      revalidateOnFocus: false,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );

  const pageSearch = useSWR(
    scope === "page" && debouncedQuery
      ? ["protection:page-search", debouncedQuery]
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

  const roles = catalog.data?.roles ?? [];
  const effectiveRoleKey = roleKey || roles[0]?.key || "";
  const rules = useMemo(
    () => catalog.data?.rules ?? [],
    [catalog.data?.rules],
  );
  const isActive = (rule: PageProtection) =>
    !rule.revokedAt &&
    (!rule.expiresAt || rule.expiresAt.getTime() > referenceNow);
  const visibleRules = showEnded ? rules : rules.filter(isActive);
  const activeCount = rules.filter(isActive).length;
  const pageCount = rules.filter(
    (rule) => rule.pageId && isActive(rule),
  ).length;
  const titleCount = rules.filter(
    (rule) => rule.namespaceId && isActive(rule),
  ).length;
  const selectedPageTitle =
    selectedPage?.id === initialPageId && initialPage.data
      ? initialPage.data.page.displayTitle
      : selectedPage?.title;

  const resetTarget = (nextScope: "page" | "title") => {
    setScope(nextScope);
    setQuery("");
    setSelectedPage(undefined);
    if (nextScope === "title") setAction("edit");
  };

  const createRule = async (event: FormEvent) => {
    event.preventDefault();
    if (!effectiveRoleKey) {
      toast.error("请选择最低所需角色");
      return;
    }

    let request: CreatePageProtectionRequest;
    if (scope === "page") {
      const pageID = uuidSchema.safeParse(selectedPage?.id);
      if (!pageID.success) {
        toast.error(pageID.error.issues[0]?.message ?? "请选择页面");
        return;
      }
      request = {
        pageId: pageID.data,
        actionType: action,
        requiredRoleKey: effectiveRoleKey,
      };
    } else {
      const parsed = titleScopeSchema.safeParse({ namespaceKey, title });
      if (!parsed.success) {
        toast.error(parsed.error.issues[0]?.message ?? "请检查标题范围");
        return;
      }
      request = {
        namespaceKey: parsed.data.namespaceKey,
        normalizedTitle: parsed.data.title,
        actionType: "create",
        requiredRoleKey: effectiveRoleKey,
      };
    }
    if (expiresAt) {
      const expires = new Date(expiresAt);
      if (
        Number.isNaN(expires.getTime()) ||
        expires.getTime() <= Date.now()
      ) {
        toast.error("到期时间必须晚于当前时间");
        return;
      }
      request.expiresAt = expires;
    }

    setSaving(true);
    try {
      const created = await governanceApi().createPageProtection({
        createPageProtectionRequest: request,
      });
      await catalog.mutate();
      setExpiresAt("");
      if (scope === "title") setTitle("");
      toast.success("页面保护规则已生效", {
        description: `${ACTION_LABEL[created.actionType]}需要${created.requiredRoleName}角色。`,
      });
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
        router.push(LOGIN_PATH);
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("只有站点管理员可以管理保护规则");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 404
      ) {
        toast.error("目标页面或命名空间不存在");
      } else {
        toast.error("保护规则未创建", {
          description: "请检查范围、角色与到期时间是否有效。",
        });
      }
    } finally {
      setSaving(false);
    }
  };

  const revoke = async (rule: PageProtection) => {
    setRevokingId(rule.id);
    try {
      await governanceApi().deletePageProtection({ id: rule.id });
      await catalog.mutate();
      toast.success("保护规则已撤销", {
        description: "规则停止生效，原记录与审计历史仍会保留。",
      });
    } catch (error) {
      if (isUnauthorized(error)) {
        router.push(LOGIN_PATH);
      } else {
        toast.error("保护规则撤销失败");
      }
    } finally {
      setRevokingId(undefined);
    }
  };

  if (session.isLoading) {
    return (
      <div className="flex min-h-72 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在确认治理权限…
      </div>
    );
  }
  if (!session.isAuthenticated) {
    return (
      <div className="rounded-3xl border border-dashed bg-muted/25 p-10 text-center">
        <LockKeyhole className="mx-auto size-7 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后管理页面保护</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          该工作台只向当前站点的管理员开放。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }

  if (
    catalog.error instanceof ResponseError &&
    catalog.error.response.status === 403
  ) {
    return (
      <div className="rounded-3xl border border-amber-200 bg-amber-50/65 p-8">
        <ShieldAlert className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">
          需要站点管理员权限
        </h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-amber-900/70">
          页面保护会改变谁能够创建、编辑、审核、应用或回滚知识，因此普通共建者只能查看页面本身。
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <section className="grid gap-3 sm:grid-cols-3">
        {[
          { label: "正在生效", value: activeCount, icon: ShieldCheck },
          { label: "页面范围", value: pageCount, icon: FileText },
          { label: "标题预留", value: titleCount, icon: Sparkles },
        ].map(({ label, value, icon: Icon }) => (
          <div key={label} className="rounded-2xl border bg-card p-5">
            <Icon className="size-4 text-primary" aria-hidden />
            <p className="mt-4 text-3xl font-semibold tracking-tight">{value}</p>
            <p className="mt-1 text-xs text-muted-foreground">{label}</p>
          </div>
        ))}
      </section>

      <div className="grid gap-8 xl:grid-cols-[25rem_minmax(0,1fr)]">
        <form
          onSubmit={(event) => void createRule(event)}
          className="h-fit rounded-3xl border bg-card p-5 shadow-[0_14px_40px_-32px_rgb(15_23_42/0.45)] xl:sticky xl:top-24"
        >
          <div className="flex items-start gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <LockKeyhole className="size-5" aria-hidden />
            </span>
            <div>
              <h2 className="font-semibold">新建保护规则</h2>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                规则叠加生效；满足全部最低角色要求才会放行。
              </p>
            </div>
          </div>

          <div className="mt-6 grid grid-cols-2 gap-2 rounded-xl bg-muted/60 p-1">
            {[
              ["page", "已有页面"],
              ["title", "预留标题"],
            ].map(([value, label]) => (
              <button
                key={value}
                type="button"
                onClick={() => resetTarget(value as "page" | "title")}
                className={cn(
                  "h-9 rounded-lg text-xs font-semibold transition",
                  scope === value
                    ? "bg-background text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {label}
              </button>
            ))}
          </div>

          {scope === "page" ? (
            <div className="mt-5 space-y-2">
              <Label>目标页面</Label>
              {selectedPage ? (
                <div className="rounded-xl border border-primary/20 bg-primary/5 p-3">
                  <p className="truncate text-sm font-semibold">
                    {selectedPageTitle}
                  </p>
                  <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
                    {selectedPage.id}
                  </p>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="mt-2"
                    onClick={() => setSelectedPage(undefined)}
                  >
                    重新选择
                  </Button>
                </div>
              ) : (
                <Command shouldFilter={false} label="搜索保护目标页面">
                  <div className="relative">
                    <Search className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground" />
                    <Command.Input
                      value={query}
                      onValueChange={setQuery}
                      placeholder="按标题或别名搜索…"
                      className="h-8 w-full rounded-lg border border-input bg-transparent pr-2.5 pl-8 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                    />
                  </div>
                  <Command.List className="mt-2 max-h-44 overflow-y-auto">
                    {pageSearch.isLoading ? (
                      <Command.Loading className="px-3 py-5 text-center text-xs text-muted-foreground">
                        搜索中…
                      </Command.Loading>
                    ) : null}
                    {debouncedQuery &&
                    !pageSearch.isLoading &&
                    !pageSearch.data?.items.length ? (
                      <Command.Empty className="px-3 py-5 text-center text-xs text-muted-foreground">
                        没有匹配页面
                      </Command.Empty>
                    ) : null}
                    {(pageSearch.data?.items ?? []).map(
                      (hit: PageSearchHit) => (
                        <Command.Item
                          key={hit.id}
                          value={hit.id}
                          onSelect={() =>
                            setSelectedPage({
                              id: hit.id,
                              title: hit.displayTitle,
                            })
                          }
                          className="flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-xs data-[selected=true]:bg-accent"
                        >
                          <FileText className="size-3.5 text-muted-foreground" />
                          <span className="truncate">{hit.displayTitle}</span>
                        </Command.Item>
                      ),
                    )}
                  </Command.List>
                </Command>
              )}
            </div>
          ) : (
            <div className="mt-5 grid gap-4">
              <div className="space-y-2">
                <Label htmlFor="protection-namespace">命名空间 key</Label>
                <Input
                  id="protection-namespace"
                  value={namespaceKey}
                  onChange={(event) => setNamespaceKey(event.target.value)}
                  placeholder="main"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="protection-title">待保护标题</Label>
                <Input
                  id="protection-title"
                  value={title}
                  onChange={(event) => setTitle(event.target.value)}
                  placeholder="尚未创建的页面标题"
                />
              </div>
            </div>
          )}

          <div className="mt-5 space-y-2">
            <Label htmlFor="protection-action">受保护操作</Label>
            <select
              id="protection-action"
              value={scope === "title" ? "create" : action}
              disabled={scope === "title"}
              onChange={(event) =>
                setAction(
                  event.target.value as (typeof ACTIONS)[number][0],
                )
              }
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm disabled:bg-muted disabled:text-muted-foreground"
            >
              {scope === "title" ? (
                <option value="create">创建 · 防止该标题被低权限用户创建</option>
              ) : (
                ACTIONS.map(([value, label, detail]) => (
                  <option key={value} value={value}>
                    {label} · {detail}
                  </option>
                ))
              )}
            </select>
          </div>

          <div className="mt-5 space-y-2">
            <Label htmlFor="protection-role">最低角色</Label>
            <select
              id="protection-role"
              value={effectiveRoleKey}
              onChange={(event) => setRoleKey(event.target.value)}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
            >
              <option value="">选择角色…</option>
              {roles.map((role) => (
                <option key={role.id} value={role.key}>
                  {role.name} · {role.description}
                </option>
              ))}
            </select>
          </div>

          <div className="mt-5 space-y-2">
            <Label htmlFor="protection-expiry">到期时间（可选）</Label>
            <Input
              id="protection-expiry"
              type="datetime-local"
              value={expiresAt}
              onChange={(event) => setExpiresAt(event.target.value)}
            />
            <p className="text-[10px] leading-4 text-muted-foreground">
              留空表示持续生效；后续仍可随时撤销且不会抹除审计。
            </p>
          </div>

          <Button
            type="submit"
            className="mt-6 w-full"
            disabled={saving || catalog.isLoading}
          >
            {saving ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : (
              <ShieldCheck aria-hidden />
            )}
            {saving ? "正在创建…" : "启用保护规则"}
          </Button>
        </form>

        <section>
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div>
              <h2 className="text-xl font-semibold tracking-tight">规则目录</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                当前 Wiki 的页面与标题保护，按创建时间倒序排列。
              </p>
            </div>
            <Button
              type="button"
              size="sm"
              variant={showEnded ? "secondary" : "outline"}
              onClick={() => setShowEnded((current) => !current)}
            >
              {showEnded ? "隐藏历史规则" : "显示已到期与已撤销"}
            </Button>
          </div>

          {catalog.isLoading ? (
            <div className="mt-5 flex min-h-48 items-center justify-center gap-2 rounded-2xl border border-dashed text-sm text-muted-foreground">
              <LoaderCircle className="size-4 animate-spin" aria-hidden />
              正在读取规则…
            </div>
          ) : visibleRules.length ? (
            <ul className="mt-5 space-y-3">
              {visibleRules.map((rule) => {
                const state = protectionState(rule, referenceNow);
                const StateIcon = state.icon;
                const target = rule.pageId
                  ? rule.pageTitle || rule.pageId
                  : `${rule.namespaceKey ?? "?"}:${rule.normalizedTitle ?? "?"}`;
                return (
                  <li
                    key={rule.id}
                    className="rounded-2xl border bg-card p-5 transition hover:border-primary/20"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-4">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span
                            className={cn(
                              "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-semibold",
                              state.className,
                            )}
                          >
                            <StateIcon className="size-3" aria-hidden />
                            {state.label}
                          </span>
                          <span className="rounded-full bg-primary/9 px-2 py-0.5 text-[10px] font-semibold text-primary">
                            {ACTION_LABEL[rule.actionType]}
                          </span>
                          <span className="text-[10px] text-muted-foreground">
                            至少 {rule.requiredRoleName}
                          </span>
                        </div>
                        {rule.pageId ? (
                          <Link
                            href={`/pages/${rule.pageId}`}
                            className="mt-3 block truncate font-semibold hover:text-primary hover:underline"
                          >
                            {target}
                          </Link>
                        ) : (
                          <p className="mt-3 truncate font-semibold">{target}</p>
                        )}
                        <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                          {rule.id}
                        </p>
                      </div>
                      {!rule.revokedAt &&
                      (!rule.expiresAt ||
                        rule.expiresAt.getTime() > referenceNow) ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          disabled={revokingId === rule.id}
                          onClick={() => void revoke(rule)}
                        >
                          {revokingId === rule.id ? (
                            <LoaderCircle className="animate-spin" aria-hidden />
                          ) : (
                            <XCircle aria-hidden />
                          )}
                          撤销
                        </Button>
                      ) : null}
                    </div>
                    <div className="mt-4 grid gap-2 border-t pt-4 text-[11px] text-muted-foreground sm:grid-cols-2">
                      <span>创建于 {DATE_FORMATTER.format(rule.createdAt)}</span>
                      <span className="sm:text-right">
                        {rule.revokedAt
                          ? `撤销于 ${DATE_FORMATTER.format(rule.revokedAt)}`
                          : rule.expiresAt
                            ? `到期于 ${DATE_FORMATTER.format(rule.expiresAt)}`
                            : "无自动到期时间"}
                      </span>
                    </div>
                  </li>
                );
              })}
            </ul>
          ) : (
            <div className="mt-5 rounded-2xl border border-dashed bg-muted/20 px-6 py-12 text-center">
              <ShieldCheck
                className="mx-auto size-7 text-muted-foreground"
                aria-hidden
              />
              <p className="mt-3 text-sm font-semibold">暂无匹配保护规则</p>
              <p className="mt-1 text-xs text-muted-foreground">
                可在左侧为页面操作或尚未创建的标题添加第一条规则。
              </p>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
