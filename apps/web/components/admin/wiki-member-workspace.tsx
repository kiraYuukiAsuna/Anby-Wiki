"use client";

import {
  LoaderCircle,
  LockKeyhole,
  RefreshCw,
  Search,
  ShieldAlert,
  ShieldCheck,
  Trash2,
  UserPlus,
  UserRound,
  UsersRound,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type AddWikiMemberRequestRoleKeysEnum,
  type GrantWikiMemberRoleRoleKeyEnum,
  type RevokeWikiMemberRoleRoleKeyEnum,
  type RoleSummary,
  type WikiMember,
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
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { wikiApi } from "@/lib/api";
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
  admin: "成员、保护与治理管理",
};

const searchSchema = z.string().trim().max(128);
const identifierSchema = z.string().trim().min(1).max(254);

function hasRole(member: WikiMember, role: RoleKey) {
  return member.roles.some((item) => item.key === role);
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

function roleCatalogFrom(members: WikiMember[] | undefined) {
  const seen = new Map<string, RoleSummary>();
  for (const member of members ?? []) {
    for (const role of member.roles) seen.set(role.key, role);
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
}

function RoleToggle({
  member,
  role,
  busy,
  activeAdminCount,
  onGrant,
  onRevoke,
}: {
  member: WikiMember;
  role: RoleKey;
  busy: boolean;
  activeAdminCount: number;
  onGrant: (member: WikiMember, role: RoleKey) => Promise<void>;
  onRevoke: (member: WikiMember, role: RoleKey) => Promise<void>;
}) {
  const enabled = hasRole(member, role);
  const removingLastRole = enabled && member.roles.length <= 1;
  const removingLastAdmin =
    enabled &&
    role === "admin" &&
    member.status === "active" &&
    activeAdminCount <= 1;
  const blocked = removingLastRole || removingLastAdmin;
  return (
    <Button
      type="button"
      size="sm"
      variant={enabled ? "default" : "outline"}
      disabled={busy || blocked}
      title={
        removingLastRole
          ? "成员至少需要保留一个角色"
          : removingLastAdmin
            ? "不能撤销最后一个管理员"
            : roleDescriptions[role]
      }
      onClick={() => void (enabled ? onRevoke(member, role) : onGrant(member, role))}
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

function MemberCard({
  member,
  roles,
  busy,
  removing,
  currentActorID,
  activeAdminCount,
  onGrant,
  onRevoke,
  onRemove,
}: {
  member: WikiMember;
  roles: RoleSummary[];
  busy: string | null;
  removing: boolean;
  currentActorID: string | undefined;
  activeAdminCount: number;
  onGrant: (member: WikiMember, role: RoleKey) => Promise<void>;
  onRevoke: (member: WikiMember, role: RoleKey) => Promise<void>;
  onRemove: (member: WikiMember) => void;
}) {
  const roleKeys = new Set(roles.map((role) => role.key));
  const isCurrentUser = member.actorId === currentActorID;
  const isLastAdmin =
    member.status === "active" && hasRole(member, "admin") && activeAdminCount <= 1;
  const removeBlocked = isCurrentUser || isLastAdmin;
  return (
    <article className="rounded-lg border bg-card p-4">
      <div className="flex min-w-0 items-start gap-3">
        <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <UserRound className="size-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-sm font-semibold">{member.displayName}</h2>
            {isCurrentUser ? (
              <span className="rounded border bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                当前账号
              </span>
            ) : null}
            {member.status !== "active" ? (
              <span className="rounded border border-dashed px-1.5 py-0.5 text-[10px] text-muted-foreground">
                已停用
              </span>
            ) : null}
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {member.username} · {member.email}
          </p>
          <p className="mt-1 break-all font-mono text-[10px] text-muted-foreground">
            {member.actorId}
          </p>
        </div>
      </div>

      <div className="mt-4 flex flex-wrap gap-1.5">
        {member.roles.map((role) => {
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
        })}
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        {roleOrder
          .filter((role) => roleKeys.has(role))
          .map((role) => (
            <RoleToggle
              key={role}
              member={member}
              role={role}
              busy={busy === `${member.actorId}:${role}`}
              activeAdminCount={activeAdminCount}
              onGrant={onGrant}
              onRevoke={onRevoke}
            />
          ))}
      </div>

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t pt-3">
        <span className="text-xs text-muted-foreground">
          加入于 {formatTime(member.createdAt)}
        </span>
        <Button
          type="button"
          size="sm"
          variant="destructive"
          disabled={Boolean(busy) || removing || removeBlocked}
          title={
            isCurrentUser
              ? "不能移除当前成员"
              : isLastAdmin
                ? "不能移除最后一个管理员"
                : "移除协作者"
          }
          onClick={() => onRemove(member)}
        >
          {removing ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <Trash2 aria-hidden />
          )}
          移除
        </Button>
      </div>
    </article>
  );
}

export function WikiMemberWorkspace() {
  const session = useSession();
  const [search, setSearch] = useState("");
  const [includeDisabled, setIncludeDisabled] = useState(false);
  const [identifier, setIdentifier] = useState("");
  const [selectedRoles, setSelectedRoles] = useState<Set<RoleKey>>(
    () => new Set(["editor"]),
  );
  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [removingID, setRemovingID] = useState<string | null>(null);
  const [removeTarget, setRemoveTarget] = useState<WikiMember | null>(null);
  const parsedSearch = searchSchema.safeParse(search);
  const parsedIdentifier = identifierSchema.safeParse(identifier);
  const normalizedSearch = parsedSearch.success ? parsedSearch.data : "";

  const members = useSWR(
    session.isAuthenticated
      ? ["wiki:members", session.session?.wikiId, normalizedSearch, includeDisabled]
      : null,
    () =>
      wikiApi().listWikiMembers({
        search: normalizedSearch || undefined,
        includeDisabled,
      }),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );

  const roleCatalog = useMemo(
    () => roleCatalogFrom(members.data?.items),
    [members.data],
  );

  const activeAdminCount = useMemo(
    () =>
      (members.data?.items ?? []).filter(
        (member) => member.status === "active" && hasRole(member, "admin"),
      ).length,
    [members.data],
  );

  const replaceMember = async (updated: WikiMember) => {
    await members.mutate((current) => {
      if (!current) return current;
      const exists = current.items.some((item) => item.actorId === updated.actorId);
      return {
        items: exists
          ? current.items.map((item) =>
              item.actorId === updated.actorId ? updated : item,
            )
          : [updated, ...current.items],
      };
    }, false);
  };

  const toggleSelectedRole = (role: RoleKey, checked: boolean) => {
    setSelectedRoles((current) => {
      const next = new Set(current);
      if (checked) {
        next.add(role);
      } else {
        next.delete(role);
      }
      return next;
    });
  };

  const addMember = async () => {
    if (!parsedIdentifier.success || selectedRoles.size === 0) return;
    setAdding(true);
    try {
      const result = await wikiApi().addWikiMember({
        addWikiMemberRequest: {
          identifier: parsedIdentifier.data,
          roleKeys: new Set(
            Array.from(selectedRoles),
          ) as Set<AddWikiMemberRequestRoleKeysEnum>,
        },
      });
      await replaceMember(result.member);
      setIdentifier("");
      setSelectedRoles(new Set(["editor"]));
      toast.success(result.changed ? "协作者已添加" : "协作者已在成员列表中", {
        description: result.member.displayName,
      });
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("需要管理员权限");
      } else if (error instanceof ResponseError && error.response.status === 404) {
        toast.error("没有找到可添加的本地账号");
      } else {
        toast.error("添加协作者失败");
      }
    } finally {
      setAdding(false);
    }
  };

  const mutateRole = async (
    member: WikiMember,
    role: RoleKey,
    direction: "grant" | "revoke",
  ) => {
    setBusy(`${member.actorId}:${role}`);
    try {
      const result =
        direction === "grant"
          ? await wikiApi().grantWikiMemberRole({
              actorId: member.actorId,
              roleKey: role as GrantWikiMemberRoleRoleKeyEnum,
            })
          : await wikiApi().revokeWikiMemberRole({
              actorId: member.actorId,
              roleKey: role as RevokeWikiMemberRoleRoleKeyEnum,
            });
      await replaceMember(result.member);
      toast.success(result.changed ? "角色已更新" : "角色未变化", {
        description: `${result.member.displayName} · ${roleLabels[role]}`,
      });
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("角色不能撤销", {
          description: "成员至少保留一个角色，且不能撤销最后一个管理员。",
        });
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("需要管理员权限");
      } else {
        toast.error("角色更新失败");
      }
    } finally {
      setBusy(null);
    }
  };

  const removeMember = async () => {
    if (!removeTarget) return;
    setRemovingID(removeTarget.actorId);
    try {
      const result = await wikiApi().removeWikiMember({
        actorId: removeTarget.actorId,
      });
      await members.mutate((current) => {
        if (!current) return current;
        return {
          items: current.items.filter((item) => item.actorId !== result.actorId),
        };
      }, false);
      toast.success("协作者已移除", {
        description: `${removeTarget.displayName} 的账号和个人 Wiki 不受影响。`,
      });
      setRemoveTarget(null);
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("协作者不能移除", {
          description: "不能移除当前成员或最后一个管理员。",
        });
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("需要管理员权限");
      } else if (error instanceof ResponseError && error.response.status === 404) {
        toast.error("成员不存在");
      } else {
        toast.error("移除协作者失败");
      }
    } finally {
      setRemovingID(null);
    }
  };

  if (session.isLoading || members.isLoading) {
    return (
      <div className="flex min-h-72 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在读取成员列表…
      </div>
    );
  }

  if (!session.isAuthenticated) {
    return (
      <div className="rounded-lg border border-dashed bg-muted/25 p-10 text-center">
        <LockKeyhole className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后管理成员</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          Wiki 成员和角色仅向管理员开放。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }

  if (members.error instanceof ResponseError && members.error.response.status === 403) {
    return (
      <div className="rounded-lg border border-amber-200 bg-amber-50/65 p-8">
        <ShieldAlert className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">需要管理员权限</h2>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-amber-900/70">
          只有 admin 角色可以查看成员、添加协作者并分配角色。
        </p>
      </div>
    );
  }

  if (members.error || !members.data) {
    return (
      <div className="rounded-lg border border-destructive/25 bg-destructive/5 p-8">
        <ShieldAlert className="size-7 text-destructive" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">成员服务不可用</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          请检查 API 与认证服务状态。
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="rounded-lg border bg-card p-4">
        <div className="flex items-center gap-2">
          <UserPlus className="size-4 text-primary" aria-hidden />
          <h2 className="text-sm font-semibold">添加协作者</h2>
        </div>
        <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto]">
          <Input
            value={identifier}
            onChange={(event) => setIdentifier(event.target.value)}
            placeholder="用户名、邮箱或 Actor ID"
            aria-invalid={identifier.length > 0 && !parsedIdentifier.success}
          />
          <Button
            type="button"
            disabled={adding || !parsedIdentifier.success || selectedRoles.size === 0}
            onClick={() => void addMember()}
          >
            {adding ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : (
              <UserPlus aria-hidden />
            )}
            添加
          </Button>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          {roleOrder.map((role) => (
            <label
              key={role}
              className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm"
              title={roleDescriptions[role]}
            >
              <Checkbox
                checked={selectedRoles.has(role)}
                onCheckedChange={(value) => toggleSelectedRole(role, value === true)}
              />
              {roleLabels[role]}
            </label>
          ))}
        </div>
      </div>

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
          包含停用成员
        </label>
        <Button
          type="button"
          variant="outline"
          disabled={members.isValidating}
          onClick={() => void members.mutate()}
        >
          <RefreshCw
            className={cn(members.isValidating && "animate-spin")}
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

      <section className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">成员列表</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              {members.data.items.length} 个成员 · 当前 Wiki {session.session?.wikiName}
            </p>
          </div>
          {activeAdminCount <= 1 ? (
            <span className="rounded border border-amber-200 bg-amber-50 px-2 py-1 text-xs text-amber-900">
              当前仅 1 个 active admin
            </span>
          ) : null}
        </div>
        {members.data.items.length > 0 ? (
          <div className="grid gap-3 xl:grid-cols-2">
            {members.data.items.map((member) => (
              <MemberCard
                key={member.actorId}
                member={member}
                roles={roleCatalog}
                busy={busy}
                removing={removingID === member.actorId}
                currentActorID={session.session?.actorId}
                activeAdminCount={activeAdminCount}
                onGrant={(target, role) => mutateRole(target, role, "grant")}
                onRevoke={(target, role) => mutateRole(target, role, "revoke")}
                onRemove={setRemoveTarget}
              />
            ))}
          </div>
        ) : (
          <div className="rounded-lg border border-dashed bg-muted/20 p-10 text-center">
            <UsersRound className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h2 className="mt-4 text-lg font-semibold">没有匹配的成员</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              调整搜索条件，或添加一个已有本地账号作为协作者。
            </p>
          </div>
        )}
      </section>

      <Dialog
        open={removeTarget !== null}
        onOpenChange={(open) => {
          if (!open && removingID === null) setRemoveTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>移除协作者</DialogTitle>
            <DialogDescription>
              {removeTarget
                ? `将撤销 ${removeTarget.displayName} 在当前 Wiki 的全部角色。`
                : ""}
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border border-destructive/25 bg-destructive/5 p-4 text-sm text-destructive">
            此操作不会删除对方账号，也不会影响其个人 Wiki。
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={removingID !== null}
              onClick={() => setRemoveTarget(null)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={removingID !== null}
              onClick={() => void removeMember()}
            >
              {removingID !== null ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Trash2 aria-hidden />
              )}
              移除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
