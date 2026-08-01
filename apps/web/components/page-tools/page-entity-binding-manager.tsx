"use client";

import {
  AtSign,
  Crown,
  LoaderCircle,
  Plus,
  Search,
  Trash2,
  TriangleAlert,
  Waypoints,
} from "lucide-react";
import Link from "next/link";
import { useDeferredValue, useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type PageEntityBinding,
} from "../../../../contracts/generated/typescript";

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
import { knowledgeApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const bindingSchema = z.object({
  entityId: z.string().uuid("请选择一个 Entity"),
  role: z.enum(["primary", "mentioned"]),
  language: z.string().trim().min(1, "请填写语言标记").max(64),
});

type BindingDraft = z.input<typeof bindingSchema>;

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  dateStyle: "medium",
  timeStyle: "short",
});

export function PageEntityBindingManager({
  pageId,
  pageTitle,
  pageLanguage,
}: {
  pageId: string;
  pageTitle: string;
  pageLanguage: string;
}) {
  const session = useSession();
  const bindings = useSWR(["page:entity-bindings", pageId], () =>
    knowledgeApi().listPageEntityBindings({ id: pageId }),
  );
  const items = useMemo(() => bindings.data?.items ?? [], [bindings.data?.items]);
  const bindingIDs = useMemo(
    () => [...new Set(items.map((item) => item.entityId))].sort(),
    [items],
  );
  const entityDetails = useSWR(
    bindingIDs.length
      ? (["page:entity-binding-details", ...bindingIDs] as const)
      : null,
    () =>
      Promise.all(
        bindingIDs.map((id) => knowledgeApi().getEntity({ id })),
      ),
  );
  const detailMap = new Map(
    (entityDetails.data ?? []).map((item) => [item.id, item]),
  );

  const [draft, setDraft] = useState<BindingDraft>();
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query.trim());
  const candidates = useSWR(
    draft
      ? (["page:entity-binding-candidates", deferredQuery] as const)
      : null,
    () =>
      knowledgeApi().listEntities({
        q: deferredQuery || undefined,
        status: "active",
        pageSize: 12,
      }),
  );
  const [removal, setRemoval] = useState<PageEntityBinding>();
  const [saving, setSaving] = useState(false);

  const startCreate = () => {
    setQuery("");
    setDraft({
      entityId: "",
      role: items.some((item) => item.role === "primary")
        ? "mentioned"
        : "primary",
      language: pageLanguage || "zh-Hans",
    });
  };

  const save = async () => {
    if (!draft) return;
    const parsed = bindingSchema.safeParse(draft);
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查 Entity 绑定");
      return;
    }
    setSaving(true);
    try {
      await knowledgeApi().setPageEntityBinding({
        id: pageId,
        setPageEntityBindingRequest: parsed.data,
      });
      await bindings.mutate();
      setDraft(undefined);
      toast.success(
        parsed.data.role === "primary"
          ? "页面主实体已更新"
          : "显式 Entity 绑定已添加",
        {
          description:
            parsed.data.role === "primary"
              ? "搜索元数据会由 Worker 刷新；旧稳定 Entity ID 仍可解析。"
              : "正文中的实际提及仍由 Current Revision 投影自动维护。",
        },
      );
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("当前账号没有编辑这个页面的权限");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 409
      ) {
        toast.error("该绑定已存在，或 Entity 已经合并");
      } else {
        toast.error("页面 Entity 绑定保存失败");
      }
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!removal) return;
    setSaving(true);
    try {
      await knowledgeApi().removePageEntityBinding({
        id: pageId,
        entityId: removal.entityId,
        role: removal.role,
      });
      await bindings.mutate();
      setRemoval(undefined);
      toast.success(
        removal.role === "primary" ? "页面主实体已清空" : "显式绑定已移除",
      );
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("当前账号没有编辑这个页面的权限");
      } else {
        toast.error("Entity 绑定移除失败");
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <section className="overflow-hidden rounded-2xl border bg-card shadow-sm">
        <header className="flex flex-wrap items-start justify-between gap-4 border-b px-5 py-5">
          <div className="flex items-start gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-fuchsia-100 text-fuchsia-700">
              <Waypoints className="size-4" aria-hidden />
            </span>
            <div>
              <h2 className="font-semibold">页面与 Entity</h2>
              <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
                主实体为搜索和结构化入口提供语义身份；显式 mentioned 绑定仅用于上下文，
                正文提及仍从 Current Revision 自动投影。
              </p>
            </div>
          </div>
          {session.isAuthenticated ? (
            <Button type="button" size="sm" onClick={startCreate}>
              <Plus aria-hidden />
              设置绑定
            </Button>
          ) : (
            <Button asChild type="button" size="sm">
              <Link href={LOGIN_PATH}>登录后管理</Link>
            </Button>
          )}
        </header>

        <div className="p-5">
          {bindings.isLoading ? (
            <div className="flex min-h-32 items-center justify-center gap-2 text-sm text-muted-foreground">
              <LoaderCircle className="size-4 animate-spin" aria-hidden />
              读取权威绑定…
            </div>
          ) : bindings.error ? (
            <div className="flex items-start gap-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4">
              <TriangleAlert className="mt-0.5 size-4 text-destructive" aria-hidden />
              <p className="text-sm text-destructive">页面 Entity 绑定暂时无法读取。</p>
            </div>
          ) : items.length ? (
            <ul className="grid gap-3 md:grid-cols-2">
              {items.map((item) => {
                const entity = detailMap.get(item.entityId);
                const title =
                  entity?.labels.find((label) => label.isPrimary)?.label ??
                  entity?.canonicalKey ??
                  item.entityId;
                const Icon = item.role === "primary" ? Crown : AtSign;
                return (
                  <li
                    key={`${item.entityId}:${item.role}`}
                    className={cn(
                      "rounded-xl border p-4",
                      item.role === "primary"
                        ? "border-fuchsia-200 bg-fuchsia-50/45"
                        : "bg-muted/15",
                    )}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <Link
                        href={`/entities/${item.entityId}`}
                        className="flex min-w-0 items-start gap-3 hover:text-primary"
                      >
                        <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-background">
                          <Icon className="size-4" aria-hidden />
                        </span>
                        <span className="min-w-0">
                          <span className="block truncate text-sm font-semibold">
                            {title}
                          </span>
                          <span className="mt-1 block text-[10px] text-muted-foreground">
                            {item.role === "primary" ? "页面主实体" : "显式提及"} ·{" "}
                            {item.language}
                          </span>
                        </span>
                      </Link>
                      {session.isAuthenticated ? (
                        <Button
                          type="button"
                          size="icon-sm"
                          variant="ghost"
                          className="shrink-0 text-destructive hover:text-destructive"
                          aria-label={`移除 ${title} 绑定`}
                          onClick={() => setRemoval(item)}
                        >
                          <Trash2 aria-hidden />
                        </Button>
                      ) : null}
                    </div>
                    <p className="mt-3 border-t pt-3 font-mono text-[9px] text-muted-foreground">
                      {item.entityId}
                    </p>
                    <p className="mt-1 text-[9px] text-muted-foreground">
                      {DATE_FORMATTER.format(item.createdAt)}
                    </p>
                  </li>
                );
              })}
            </ul>
          ) : (
            <div className="rounded-xl border border-dashed bg-muted/20 px-6 py-9 text-center">
              <Waypoints className="mx-auto size-6 text-muted-foreground" aria-hidden />
              <p className="mt-3 text-sm font-semibold">尚未绑定稳定 Entity</p>
              <p className="mt-1 text-xs text-muted-foreground">
                为「{pageTitle}」设置主实体后，搜索结果可携带类型、标签与别名。
              </p>
            </div>
          )}
        </div>
      </section>

      <Dialog
        open={Boolean(draft)}
        onOpenChange={(open) => {
          if (!open && !saving) setDraft(undefined);
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>设置页面 Entity 绑定</DialogTitle>
            <DialogDescription>
              选择已有稳定身份。主实体在单一事务中替换，Page 指针与 binding
              由数据库延迟约束保证一致。
            </DialogDescription>
          </DialogHeader>
          {draft ? (
            <div className="grid gap-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="page-binding-role">角色</Label>
                  <select
                    id="page-binding-role"
                    value={draft.role}
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        role: event.target.value as BindingDraft["role"],
                      })
                    }
                    className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                  >
                    <option value="primary">主实体 · 唯一，可替换</option>
                    <option value="mentioned">显式提及 · 可多个</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="page-binding-language">语言</Label>
                  <Input
                    id="page-binding-language"
                    value={draft.language}
                    onChange={(event) =>
                      setDraft({ ...draft, language: event.target.value })
                    }
                  />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="page-binding-search">查找 Entity</Label>
                <div className="relative">
                  <Search
                    className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground"
                    aria-hidden
                  />
                  <Input
                    id="page-binding-search"
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    className="pl-8"
                    placeholder="名称、别名或 canonical key"
                  />
                </div>
              </div>
              <div className="max-h-64 overflow-y-auto rounded-xl border">
                {candidates.isLoading ? (
                  <div className="flex items-center justify-center gap-2 py-10 text-xs text-muted-foreground">
                    <LoaderCircle className="size-4 animate-spin" aria-hidden />
                    搜索稳定身份…
                  </div>
                ) : candidates.error ? (
                  <p className="p-4 text-xs text-destructive">Entity 目录暂不可用。</p>
                ) : candidates.data?.items.length ? (
                  <ul className="divide-y">
                    {candidates.data.items.map((item) => (
                      <li key={item.id}>
                        <button
                          type="button"
                          onClick={() =>
                            setDraft({ ...draft, entityId: item.id })
                          }
                          className={cn(
                            "flex w-full items-start justify-between gap-3 px-4 py-3 text-left transition hover:bg-muted/50",
                            draft.entityId === item.id && "bg-primary/5",
                          )}
                        >
                          <span className="min-w-0">
                            <span className="block truncate text-sm font-semibold">
                              {item.displayLabel}
                            </span>
                            <span className="mt-1 block truncate font-mono text-[9px] text-muted-foreground">
                              {item.entityType.name} · {item.canonicalKey}
                            </span>
                          </span>
                          {draft.entityId === item.id ? (
                            <span className="rounded-full bg-primary px-2 py-1 text-[9px] font-semibold text-primary-foreground">
                              已选择
                            </span>
                          ) : null}
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="px-4 py-9 text-center text-xs text-muted-foreground">
                    没有匹配的活动 Entity；请先通过 Proposal 创建稳定身份。
                  </p>
                )}
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => setDraft(undefined)}
            >
              取消
            </Button>
            <Button type="button" disabled={saving} onClick={() => void save()}>
              {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : <Waypoints aria-hidden />}
              {saving ? "保存中…" : "保存绑定"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(removal)}
        onOpenChange={(open) => {
          if (!open && !saving) setRemoval(undefined);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {removal?.role === "primary" ? "清空页面主实体？" : "移除显式绑定？"}
            </DialogTitle>
            <DialogDescription>
              {removal?.role === "primary"
                ? "搜索结果将不再携带该 Entity 元数据；正文与历史 Revision 不受影响。"
                : "这不会删除正文中由 AST 自动投影的实际提及。"}
              审计记录会永久保留。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => setRemoval(undefined)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={saving}
              onClick={() => void remove()}
            >
              {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : <Trash2 aria-hidden />}
              {saving ? "移除中…" : "确认移除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
