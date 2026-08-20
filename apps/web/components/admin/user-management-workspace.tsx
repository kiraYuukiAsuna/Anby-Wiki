"use client";

import {
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  Search,
  ShieldAlert,
  ShieldCheck,
  Trash2,
  UserX,
  UserRound,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type AdminUser,
  type GrantAdminUserRoleRoleKeyEnum,
  type RevokeAdminUserRoleRoleKeyEnum,
  type RoleSummary,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { adminApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

type RoleKey = "editor" | "reviewer" | "applier" | "admin";

const roleOrder: RoleKey[] = ["editor", "reviewer", "applier", "admin"];
const roleLabels: Record<RoleKey, string> = {
  editor: "编辑者",
  reviewer: "审核者",
  applier: "应用者",
  admin: "管理员",
};

const roleDescriptions: Record<RoleKey, string> = {
  editor: "创建、编辑、改名",
  reviewer: "审核 Proposal",
  applier: "应用与回滚",
  admin: "全部治理权限",
};

const searchSchema = z.string().trim().max(128);

function hasRole(user: AdminUser, role: RoleKey) {
  return user.roles.some((item) => item.key === role);
}

function roleTone(role: RoleKey) {
  switch (role) {
    case "admin":
      return "border-rose-200 bg-rose-50 text-rose-800";
    case "applier":
      return "border-amber-200 bg-amber-50 text-amber-800";
    case "reviewer":
      return "border-sky-200 bg-sky-50 text-sky-800";
    default:
      return "border-emerald-200 bg-emerald-50 text-emerald-800";
  }
}

function formatTime(value: Date) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(value);
}

function userMatches(users: AdminUser[], actorID: string | undefined) {
  return users.find((user) => user.actorId === actorID);
}

function RoleToggle({
  user,
  role,
  busy,
  selfIsLastAdmin,
  onGrant,
  onRevoke,
}: {
  user: AdminUser;
  role: RoleKey;
  busy: boolean;
  selfIsLastAdmin: boolean;
  onGrant: (user: AdminUser, role: RoleKey) => Promise<void>;
  onRevoke: (user: AdminUser, role: RoleKey) => Promise<void>;
}) {
  const enabled = hasRole(user, role);
  const blocked = enabled && role === "admin" && selfIsLastAdmin;
  return (
    <Button
      type="button"
      size="sm"
      variant={enabled ? "default" : "outline"}
      disabled={busy || blocked}
      title={blocked ? "不能撤销最后一个管理员" : roleDescriptions[role]}
      onClick={() => void (enabled ? onRevoke(user, role) : onGrant(user, role))}
      className={cn(
        "min-w-20",
        enabled && role === "admin" && "bg-rose-700 hover:bg-rose-800",
        enabled && role === "applier" && "bg-amber-700 hover:bg-amber-800",
        enabled && role === "reviewer" && "bg-sky-700 hover:bg-sky-800",
        enabled && role === "editor" && "bg-emerald-700 hover:bg-emerald-800",
      )}
    >
      {busy ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
      {roleLabels[role]}
    </Button>
  );
}

function UserRow({
  user,
  roles,
  busy,
  deleting,
  currentActorID,
  activeAdminCount,
  onGrant,
  onRevoke,
  onDelete,
}: {
  user: AdminUser;
  roles: RoleSummary[];
  busy: string | null;
    deleting: boolean;
  currentActorID: string | undefined;
  activeAdminCount: number;
  onGrant: (user: AdminUser, role: RoleKey) => Promise<void>;
  onRevoke: (user: AdminUser, role: RoleKey) => Promise<void>;
    onDelete: (user: AdminUser) => void;
}) {
  const roleKeys = new Set(roles.map((role) => role.key));
  const selfIsLastAdmin =
    user.actorId === currentActorID &&
    user.status === "active" &&
    hasRole(user, "admin") &&
    activeAdminCount <= 1;
  const isCurrentUser = user.actorId === currentActorID;
  const deleteBlocked = isCurrentUser || selfIsLastAdmin;
  return (
    <tr className="border-b last:border-b-0">
      <td className="px-4 py-4 align-top">
        <div className="flex min-w-0 items-start gap-3">
          <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <UserRound className="size-4" aria-hidden />
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{user.displayName}</span>
              {isCurrentUser ? (
                <span className="rounded border bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                  当前账号
                </span>
              ) : null}
              {user.status !== "active" ? (
                <span className="rounded border border-dashed px-1.5 py-0.5 text-[10px] text-muted-foreground">
                  已停用
                </span>
              ) : null}
            </div>
            <p className="mt-1 truncate text-xs text-muted-foreground">
              {user.username} · {user.email}
            </p>
            <p className="mt-1 font-mono text-[10px] text-muted-foreground">
              {user.actorId}
            </p>
          </div>
        </div>
      </td>
      <td className="px-4 py-4 align-top">
        <div className="flex flex-wrap gap-1.5">
          {user.roles.length > 0 ? (
            user.roles.map((role) => {
              const key = role.key as RoleKey;
              return (
                <span
                  key={role.key}
                  className={cn(
                    "rounded border px-2 py-1 text-xs font-medium",
                    roleTone(roleOrder.includes(key) ? key : "editor"),
                  )}
                  title={role.description}
                >
                  {role.name}
                </span>
              );
            })
          ) : (
            <span className="text-xs text-muted-foreground">无角色</span>
          )}
        </div>
      </td>
      <td className="px-4 py-4 align-top">
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap gap-2">
            {roleOrder
              .filter((role) => roleKeys.has(role))
              .map((role) => (
                <RoleToggle
                  key={role}
                  user={user}
                  role={role}
                  busy={busy === `${user.actorId}:${role}`}
                  selfIsLastAdmin={selfIsLastAdmin}
                  onGrant={onGrant}
                  onRevoke={onRevoke}
                />
              ))}
          </div>
          <Button
            type="button"
            size="sm"
            variant="destructive"
            disabled={Boolean(busy) || deleting || deleteBlocked}
            title={
              isCurrentUser
                ? "不能删除当前账号"
                : selfIsLastAdmin
                  ? "不能删除最后一个管理员"
                  : "删除注册用户"
            }
            onClick={() => onDelete(user)}
            className="w-fit"
          >
            {deleting ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : (
              <Trash2 aria-hidden />
            )}
            删除用户
          </Button>
        </div>
      </td>
      <td className="whitespace-nowrap px-4 py-4 text-xs text-muted-foreground align-top">
        {formatTime(user.createdAt)}
      </td>
    </tr>
  );
}

export function UserManagementWorkspace() {
  const session = useSession();
  const [search, setSearch] = useState("");
  const [includeDisabled, setIncludeDisabled] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [deletingID, setDeletingID] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<AdminUser | null>(null);
  const parsedSearch = searchSchema.safeParse(search);
  const normalizedSearch = parsedSearch.success ? parsedSearch.data : "";

  const users = useSWR(
    session.isAuthenticated
      ? ["admin:users", normalizedSearch, includeDisabled]
      : null,
    () =>
      adminApi().listAdminUsers({
        search: normalizedSearch || undefined,
        includeDisabled,
      }),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );

  const roleCatalog = useMemo(() => {
    const seen = new Map<string, RoleSummary>();
    for (const user of users.data?.items ?? []) {
      for (const role of user.roles) seen.set(role.key, role);
    }
    for (const role of roleOrder) {
      if (!seen.has(role)) {
        seen.set(role, {
          id: role,
          key: role,
          name: roleLabels[role],
          description: roleDescriptions[role],
        });
      }
    }
    return roleOrder.map((role) => seen.get(role)).filter(Boolean) as RoleSummary[];
  }, [users.data]);

  const activeAdminCount = useMemo(
    () =>
      (users.data?.items ?? []).filter(
        (user) => user.status === "active" && hasRole(user, "admin"),
      ).length,
    [users.data],
  );

  const replaceUser = async (updated: AdminUser) => {
    await users.mutate((current) => {
      if (!current) return current;
      return {
        items: current.items.map((item) =>
          item.actorId === updated.actorId ? updated : item,
        ),
      };
    }, false);
  };

  const mutateRole = async (
    user: AdminUser,
    role: RoleKey,
    direction: "grant" | "revoke",
  ) => {
    setBusy(`${user.actorId}:${role}`);
    try {
      const result =
        direction === "grant"
          ? await adminApi().grantAdminUserRole({
              actorId: user.actorId,
              roleKey: role as GrantAdminUserRoleRoleKeyEnum,
            })
          : await adminApi().revokeAdminUserRole({
              actorId: user.actorId,
              roleKey: role as RevokeAdminUserRoleRoleKeyEnum,
            });
      await replaceUser(result.user);
      toast.success(result.changed ? "角色已更新" : "角色未变化", {
        description: `${result.user.displayName} · ${roleLabels[role]}`,
      });
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("不能撤销最后一个管理员");
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("需要管理员权限");
      } else {
        toast.error("角色更新失败");
      }
    } finally {
      setBusy(null);
    }
  };

  const deleteUser = async () => {
    if (!deleteTarget) return;
    setDeletingID(deleteTarget.actorId);
    try {
      const result = await adminApi().deleteAdminUser({
        actorId: deleteTarget.actorId,
      });
      await users.mutate((current) => {
        if (!current) return current;
        return {
          items: current.items.filter((item) => item.actorId !== result.actorId),
        };
      }, false);
      toast.success("用户已删除", {
        description: `${deleteTarget.username} 的登录会话和 CLI Token 已失效。`,
      });
      setDeleteTarget(null);
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("用户不能删除", {
          description: "不能删除当前账号或最后一个管理员。",
        });
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("需要管理员权限");
      } else if (error instanceof ResponseError && error.response.status === 404) {
        toast.error("用户不存在");
      } else {
        toast.error("用户删除失败");
      }
    } finally {
      setDeletingID(null);
    }
  };

  if (session.isLoading || users.isLoading) {
    return (
      <div className="flex min-h-72 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在读取用户列表…
      </div>
    );
  }
  if (!session.isAuthenticated) {
    return (
      <div className="rounded-lg border border-dashed bg-muted/25 p-10 text-center">
        <LockKeyhole className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后管理用户</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          用户角色仅向管理员开放。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }
  if (users.error instanceof ResponseError && users.error.response.status === 403) {
    return (
      <div className="rounded-lg border border-amber-200 bg-amber-50/65 p-8">
        <ShieldAlert className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">需要管理员权限</h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-amber-900/70">
          只有 admin 角色可以查看用户并授予或撤销角色。
        </p>
      </div>
    );
  }
  if (users.error || !users.data) {
    return (
      <div className="rounded-lg border border-destructive/25 bg-destructive/5 p-8">
        <ShieldAlert className="size-7 text-destructive" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">用户管理服务不可用</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          请检查 API 与认证服务状态。
        </p>
      </div>
    );
  }

  const currentUser = userMatches(users.data.items, session.session?.actorId);

  return (
    <div className="space-y-5">
      <div className="grid gap-3 rounded-lg border bg-card p-4 md:grid-cols-[minmax(0,1fr)_auto_auto]">
        <label className="relative block">
          <Search
            className="pointer-events-none absolute left-2.5 top-2.5 size-4 text-muted-foreground"
            aria-hidden
          />
          <Input
            className="h-9 pl-8"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="搜索用户名、邮箱、显示名或 Actor ID"
            aria-invalid={!parsedSearch.success}
          />
        </label>
        <label className="flex h-9 items-center gap-2 rounded-lg border px-3 text-sm">
          <Checkbox
            checked={includeDisabled}
            onCheckedChange={(value) => setIncludeDisabled(value === true)}
          />
          包含停用用户
        </label>
        <Button
          type="button"
          variant="outline"
          disabled={users.isValidating}
          onClick={() => void users.mutate()}
        >
          <RefreshCw
            className={cn(users.isValidating && "animate-spin")}
            aria-hidden
          />
          刷新
        </Button>
      </div>

      <div className="grid gap-3 md:grid-cols-4">
        {roleOrder.map((role) => (
          <div key={role} className={cn("rounded-lg border p-3", roleTone(role))}>
            <div className="flex items-center gap-2">
              <ShieldCheck className="size-4" aria-hidden />
              <span className="text-sm font-semibold">{roleLabels[role]}</span>
            </div>
            <p className="mt-1 text-xs opacity-80">{roleDescriptions[role]}</p>
          </div>
        ))}
      </div>

      <div className="overflow-hidden rounded-lg border bg-card">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
          <div>
            <h2 className="text-sm font-semibold">本地账号</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              {users.data.items.length} 个用户
              {currentUser ? ` · 当前账号 ${currentUser.username}` : ""}
            </p>
          </div>
          {activeAdminCount <= 1 ? (
            <span className="rounded border border-amber-200 bg-amber-50 px-2 py-1 text-xs text-amber-900">
              当前仅 1 个 active admin
            </span>
          ) : null}
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-[980px] text-left text-sm">
            <thead className="bg-muted/40 text-xs text-muted-foreground">
              <tr>
                <th className="px-4 py-3 font-medium">用户</th>
                <th className="px-4 py-3 font-medium">当前角色</th>
                <th className="px-4 py-3 font-medium">授予/撤销</th>
                <th className="px-4 py-3 font-medium">创建时间</th>
              </tr>
            </thead>
            <tbody>
              {users.data.items.length > 0 ? (
                users.data.items.map((user) => (
                  <UserRow
                    key={user.actorId}
                    user={user}
                    roles={roleCatalog}
                    busy={busy}
                    deleting={deletingID === user.actorId}
                    currentActorID={session.session?.actorId}
                    activeAdminCount={activeAdminCount}
                    onGrant={(target, role) => mutateRole(target, role, "grant")}
                    onRevoke={(target, role) => mutateRole(target, role, "revoke")}
                    onDelete={setDeleteTarget}
                  />
                ))
              ) : (
                <tr>
                  <td
                    colSpan={4}
                    className="px-4 py-12 text-center text-sm text-muted-foreground"
                  >
                    没有匹配的用户
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && deletingID === null) setDeleteTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除注册用户</DialogTitle>
            <DialogDescription>
              {deleteTarget
                ? `将删除 ${deleteTarget.username} 的本地登录账号，并使其现有 Session 与 CLI Token 失效。历史内容会保留原 Actor 归属。`
                : ""}
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border border-destructive/25 bg-destructive/5 p-4 text-sm text-destructive">
            <div className="flex items-start gap-3">
              <UserX className="mt-0.5 size-4 shrink-0" aria-hidden />
              <p>此操作完成后，该账号会从用户列表中移除。</p>
            </div>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={deletingID !== null}
              onClick={() => setDeleteTarget(null)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deletingID !== null}
              onClick={() => void deleteUser()}
            >
              {deletingID !== null ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Trash2 aria-hidden />
              )}
              删除用户
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
