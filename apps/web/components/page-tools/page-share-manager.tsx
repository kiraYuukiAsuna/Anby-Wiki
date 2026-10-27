"use client";

import {
  LoaderCircle,
  LockKeyhole,
  Trash2,
  TriangleAlert,
  UserPlus,
  UsersRound,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type PageShare,
  type UpsertPageShareRequestRoleKeyEnum,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { wikiApi } from "@/lib/api";
import { cn } from "@/lib/utils";

type ShareRole = "viewer" | "editor";

const roleLabels: Record<ShareRole, string> = {
  viewer: "可查看",
  editor: "可编辑",
};

const shareSchema = z.object({
  identifier: z.string().trim().min(1, "请输入用户名、邮箱或 Actor ID").max(254),
  roleKey: z.enum(["viewer", "editor"]),
});

type ShareDraft = z.input<typeof shareSchema>;

function roleTone(role: ShareRole) {
  return role === "editor"
    ? "border-emerald-200 bg-emerald-50 text-emerald-800"
    : "border-sky-200 bg-sky-50 text-sky-800";
}

function formatTime(value: Date) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(value);
}

export function PageShareManager({
  pageId,
  pageTitle,
}: {
  pageId: string;
  pageTitle: string;
}) {
  const session = useSession();
  const shares = useSWR(
    session.isAuthenticated ? ["page:shares", pageId] : null,
    () => wikiApi().listPageShares({ id: pageId }),
  );
  const items = useMemo(() => shares.data?.items ?? [], [shares.data?.items]);
  const [draft, setDraft] = useState<ShareDraft>({
    identifier: "",
    roleKey: "viewer",
  });
  const [saving, setSaving] = useState(false);
  const [removingActorID, setRemovingActorID] = useState<string | null>(null);

  const save = async () => {
    const parsed = shareSchema.safeParse(draft);
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查分享对象");
      return;
    }
    setSaving(true);
    try {
      const result = await wikiApi().upsertPageShare({
        id: pageId,
        upsertPageShareRequest: {
          identifier: parsed.data.identifier,
          roleKey: parsed.data.roleKey as UpsertPageShareRequestRoleKeyEnum,
        },
      });
      await shares.mutate((current) => {
        if (!current) return current;
        const rest = current.items.filter(
          (item) => item.actorId !== result.share.actorId,
        );
        return { items: [result.share, ...rest] };
      }, false);
      await shares.mutate();
      setDraft({ identifier: "", roleKey: parsed.data.roleKey });
      toast.success(result.changed ? "页面已分享" : "分享权限未变化", {
        description: `${result.share.displayName} · ${roleLabels[result.share.roleKey as ShareRole]}`,
      });
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("当前账号不能管理这个页面的分享");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 404
      ) {
        toast.error("未找到可分享的本地账号");
      } else {
        toast.error("页面分享失败");
      }
    } finally {
      setSaving(false);
    }
  };

  const remove = async (share: PageShare) => {
    setRemovingActorID(share.actorId);
    try {
      await wikiApi().removePageShare({
        id: pageId,
        actorId: share.actorId,
      });
      await shares.mutate((current) => {
        if (!current) return current;
        return {
          items: current.items.filter((item) => item.actorId !== share.actorId),
        };
      }, false);
      toast.success("页面分享已取消", {
        description: share.displayName,
      });
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("当前账号不能管理这个页面的分享");
      } else {
        toast.error("取消分享失败");
      }
    } finally {
      setRemovingActorID(null);
    }
  };

  return (
    <section className="overflow-hidden rounded-2xl border bg-card shadow-sm">
      <header className="flex flex-wrap items-start justify-between gap-4 border-b px-5 py-5">
        <div className="flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-sky-100 text-sky-700">
            <UsersRound className="size-4" aria-hidden />
          </span>
          <div>
            <h2 className="font-semibold">已授权账号</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {pageTitle} · 可在阅读页直接生成分享链接。
            </p>
          </div>
        </div>
      </header>

      {!session.isAuthenticated ? (
        <div className="p-5">
          <Button asChild size="sm">
            <Link href={LOGIN_PATH}>登录后管理</Link>
          </Button>
        </div>
      ) : shares.error instanceof ResponseError &&
        shares.error.response.status === 403 ? (
        <div className="flex items-start gap-3 p-5 text-sm text-muted-foreground">
          <LockKeyhole className="mt-0.5 size-4 shrink-0" aria-hidden />
          <p>只有当前 Wiki 管理员可以分享这个页面。</p>
        </div>
      ) : shares.error ? (
        <div className="flex items-start gap-3 p-5 text-sm text-destructive">
          <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          <p>页面分享列表暂时无法读取。</p>
        </div>
      ) : (
        <div className="space-y-5 p-5">
          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_9rem_auto]">
            <Input
              value={draft.identifier}
              onChange={(event) =>
                setDraft({ ...draft, identifier: event.target.value })
              }
              placeholder="直接授权用户名、邮箱或 Actor ID"
              disabled={saving}
            />
            <select
              value={draft.roleKey}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  roleKey: event.target.value as ShareRole,
                })
              }
              disabled={saving}
              className="h-10 rounded-md border bg-background px-3 text-sm"
              aria-label="分享权限"
            >
              <option value="viewer">{roleLabels.viewer}</option>
              <option value="editor">{roleLabels.editor}</option>
            </select>
            <Button
              type="button"
              onClick={() => void save()}
              disabled={saving}
              className="lg:w-28"
            >
              {saving ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <UserPlus aria-hidden />
              )}
              授权
            </Button>
          </div>

          {shares.isLoading ? (
            <div className="flex min-h-24 items-center justify-center gap-2 text-sm text-muted-foreground">
              <LoaderCircle className="size-4 animate-spin" aria-hidden />
              读取分享列表…
            </div>
          ) : items.length > 0 ? (
            <ul className="grid gap-3 md:grid-cols-2">
              {items.map((share) => (
                <li key={share.actorId} className="rounded-xl border p-4">
                  <div className="flex min-w-0 items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="truncate text-sm font-semibold">
                          {share.displayName}
                        </h3>
                        <span
                          className={cn(
                            "rounded-md border px-2 py-0.5 text-xs",
                            roleTone(share.roleKey as ShareRole),
                          )}
                        >
                          {roleLabels[share.roleKey as ShareRole]}
                        </span>
                      </div>
                      <p className="mt-1 truncate text-xs text-muted-foreground">
                        {share.username} · {share.email}
                      </p>
                      <p className="mt-2 text-xs text-muted-foreground">
                        更新于 {formatTime(share.updatedAt)}
                      </p>
                    </div>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      className="shrink-0 text-destructive hover:text-destructive"
                      disabled={removingActorID === share.actorId}
                      onClick={() => void remove(share)}
                      aria-label={`取消分享给 ${share.displayName}`}
                    >
                      {removingActorID === share.actorId ? (
                        <LoaderCircle className="animate-spin" aria-hidden />
                      ) : (
                        <Trash2 aria-hidden />
                      )}
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <p className="rounded-xl border border-dashed p-5 text-sm text-muted-foreground">
              还没有账号领取分享链接，也没有直接授权账号。
            </p>
          )}
        </div>
      )}
    </section>
  );
}
