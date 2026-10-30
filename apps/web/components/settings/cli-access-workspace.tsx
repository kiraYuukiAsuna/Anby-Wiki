"use client";

import {
  Check,
  Copy,
  KeyRound,
  LoaderCircle,
  RefreshCw,
  TerminalSquare,
  Trash2,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type CLIAuthCode,
  type CLIPendingAuthorizationCode,
  type CLIToken,
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
import { Label } from "@/components/ui/label";
import { authApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const formSchema = z.object({
  actorId: z.string().uuid().optional(),
  name: z.string().trim().min(1, "请输入 Agent 名称").max(120),
  tokenTtlDays: z.coerce.number().int().min(1).max(365),
});

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  dateStyle: "medium",
  timeStyle: "short",
});

const STATUS: Record<
  CLIToken["status"],
  { label: string; className: string }
> = {
  active: {
    label: "有效",
    className: "bg-emerald-100 text-emerald-800",
  },
  expired: {
    label: "已过期",
    className: "bg-slate-100 text-slate-600",
  },
  revoked: {
    label: "已撤销",
    className: "bg-rose-100 text-rose-800",
  },
};

function AuthorizationCodeDialog({
  value,
  onClose,
}: {
  value: CLIAuthCode | undefined;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value.code);
      setCopied(true);
      toast.success("授权码已复制");
    } catch {
      toast.error("无法访问剪贴板");
    }
  };

  return (
    <Dialog open={Boolean(value)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>一次性 CLI 授权码</DialogTitle>
          <DialogDescription>
            {value
              ? `${DATE_FORMATTER.format(value.expiresAt)} 前可兑换一次。关闭后不会再次显示。`
              : ""}
          </DialogDescription>
        </DialogHeader>
        {value ? (
          <div className="space-y-3">
            <div className="flex items-center gap-2 rounded-lg border bg-muted/35 p-3">
              <code className="min-w-0 flex-1 break-all font-mono text-xs">
                {value.code}
              </code>
              <Button
                type="button"
                size="icon-sm"
                variant="outline"
                title="复制授权码"
                onClick={() => void copy()}
              >
                {copied ? <Check aria-hidden /> : <Copy aria-hidden />}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              绑定身份：{value.displayName} · {value.actorType}
            </p>
            <p className="text-xs text-muted-foreground">
              兑换后的 Token 有效至{" "}
              {DATE_FORMATTER.format(value.tokenExpiresAt)}。
            </p>
          </div>
        ) : null}
        <DialogFooter>
          <Button type="button" onClick={onClose}>
            已保存授权码
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function TokenRow({
  token,
  revoking,
  deleting,
  onRevoke,
  onDelete,
}: {
  token: CLIToken;
  revoking: boolean;
  deleting: boolean;
  onRevoke: () => void;
  onDelete: () => void;
}) {
  const status = STATUS[token.status];
  const inactive = token.status !== "active";
  return (
    <div className="grid gap-3 border-b px-4 py-4 last:border-b-0 md:grid-cols-[minmax(0,1fr)_10rem_10rem_auto] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate text-sm font-semibold">{token.name}</p>
          <span
            className={cn(
              "rounded-full px-2 py-0.5 text-[10px] font-semibold",
              status.className,
            )}
          >
            {status.label}
          </span>
        </div>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          {token.tokenPrefix}…
        </p>
        <p className="mt-1 break-all text-[11px] text-muted-foreground">
          {token.displayName} · {token.actorType} · {token.actorId}
        </p>
      </div>
      <div>
        <p className="text-[10px] font-semibold text-muted-foreground uppercase">
          最近使用
        </p>
        <p className="mt-1 text-xs">
          {token.lastUsedAt ? DATE_FORMATTER.format(token.lastUsedAt) : "尚未使用"}
        </p>
      </div>
      <div>
        <p className="text-[10px] font-semibold text-muted-foreground uppercase">
          到期
        </p>
        <p className="mt-1 text-xs">{DATE_FORMATTER.format(token.expiresAt)}</p>
      </div>
      <Button
        type="button"
        size="sm"
        variant={inactive ? "destructive" : "outline"}
        disabled={revoking || deleting}
        onClick={inactive ? onDelete : onRevoke}
      >
        {revoking || deleting ? (
          <LoaderCircle className="animate-spin" aria-hidden />
        ) : (
          <Trash2 aria-hidden />
        )}
        {inactive ? "删除记录" : "撤销"}
      </Button>
    </div>
  );
}

function PendingAuthorizationRow({
  authorization,
  deleting,
  onDelete,
}: {
  authorization: CLIPendingAuthorizationCode;
  deleting: boolean;
  onDelete: () => void;
}) {
  return (
    <div className="grid gap-3 border-b px-4 py-4 last:border-b-0 md:grid-cols-[minmax(0,1fr)_10rem_10rem_auto] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate text-sm font-semibold">{authorization.name}</p>
          <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-semibold text-amber-800">
            待兑换
          </span>
        </div>
        <p className="mt-1 text-[11px] text-muted-foreground">
          一次性授权码已生成，明文不会再次显示。
        </p>
        <p className="mt-1 break-all text-[11px] text-muted-foreground">
          {authorization.displayName} · {authorization.actorType}
        </p>
      </div>
      <div>
        <p className="text-[10px] font-semibold text-muted-foreground uppercase">
          授权码到期
        </p>
        <p className="mt-1 text-xs">
          {DATE_FORMATTER.format(authorization.expiresAt)}
        </p>
      </div>
      <div>
        <p className="text-[10px] font-semibold text-muted-foreground uppercase">
          Token 到期
        </p>
        <p className="mt-1 text-xs">
          {DATE_FORMATTER.format(authorization.tokenExpiresAt)}
        </p>
      </div>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={deleting}
        onClick={onDelete}
      >
        {deleting ? (
          <LoaderCircle className="animate-spin" aria-hidden />
        ) : (
          <Trash2 aria-hidden />
        )}
        删除
      </Button>
    </div>
  );
}

export function CLIAccessWorkspace() {
  const session = useSession();
  const [name, setName] = useState("");
  const [actorID, setActorID] = useState("");
  const [tokenTtlDays, setTokenTtlDays] = useState("90");
  const [creating, setCreating] = useState(false);
  const [authorizationCode, setAuthorizationCode] = useState<CLIAuthCode>();
  const [revokingID, setRevokingID] = useState("");
  const [deletingID, setDeletingID] = useState("");
  const [deletingAuthorizationID, setDeletingAuthorizationID] = useState("");
  const [cleaningInactive, setCleaningInactive] = useState(false);
  const [showInactive, setShowInactive] = useState(false);
  const tokens = useSWR(
    session.isAuthenticated ? ["settings:cli-tokens", session.session?.actorId] : null,
    () => authApi().listCLITokens(),
    {
      revalidateOnFocus: true,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );
  const actors = useSWR(
    session.isAuthenticated ? ["settings:cli-actors", session.session?.actorId] : null,
    () => authApi().listCLIMachineActors(),
    { revalidateOnFocus: true, shouldRetryOnError: (error) => !isUnauthorized(error) },
  );

  const create = async () => {
    const parsed = formSchema.safeParse({ name, tokenTtlDays, actorId: actorID || undefined });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查授权设置");
      return;
    }
    setCreating(true);
    try {
      const result = await authApi().createCLIAuthCode({
        createCLIAuthCodeRequest: parsed.data,
      });
      setAuthorizationCode(result);
      setName("");
      await tokens.mutate();
      toast.success("一次性授权码已生成");
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("Actor 已停用、归属不匹配或管理员权限已失效");
      } else {
        toast.error("CLI 授权码生成失败");
      }
    } finally {
      setCreating(false);
    }
  };

  const revoke = async (token: CLIToken) => {
    setRevokingID(token.id);
    try {
      await authApi().revokeCLIToken({ id: token.id });
      await tokens.mutate();
      toast.success("CLI Token 已撤销");
    } catch {
      toast.error("CLI Token 撤销失败");
    } finally {
      setRevokingID("");
    }
  };

  const deleteRecord = async (token: CLIToken) => {
    setDeletingID(token.id);
    try {
      await authApi().deleteCLITokenRecord({ id: token.id });
      await tokens.mutate();
      toast.success("CLI Token 记录已删除");
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("有效 Token 需要先撤销");
      } else {
        toast.error("CLI Token 记录删除失败");
      }
    } finally {
      setDeletingID("");
    }
  };

  const deletePendingAuthorization = async (
    authorization: CLIPendingAuthorizationCode,
  ) => {
    setDeletingAuthorizationID(authorization.id);
    try {
      await authApi().deleteCLIAuthCode({ id: authorization.id });
      await tokens.mutate();
      toast.success("CLI 授权码已删除");
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 404) {
        toast.error("授权码已失效或已被兑换");
      } else {
        toast.error("CLI 授权码删除失败");
      }
    } finally {
      setDeletingAuthorizationID("");
    }
  };

  const deleteInactiveRecords = async () => {
    setCleaningInactive(true);
    try {
      const result = await authApi().deleteInactiveCLITokenRecords();
      await tokens.mutate();
      toast.success("已清理失效 Token 记录", {
        description: `删除 ${result.deleted} 条记录。`,
      });
    } catch {
      toast.error("失效 Token 记录清理失败");
    } finally {
      setCleaningInactive(false);
    }
  };

  if (session.isLoading) {
    return (
      <div className="h-72 animate-pulse rounded-lg border bg-muted/25" />
    );
  }
  if (!session.isAuthenticated) {
    return (
      <div className="border-y py-12 text-center">
        <KeyRound className="mx-auto size-7 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后管理 Agent CLI</h2>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>登录</Link>
        </Button>
      </div>
    );
  }

  const allTokens = tokens.data?.items ?? [];
  const pendingAuthorizations = tokens.data?.pendingAuthorizationCodes ?? [];
  const inactiveCount = allTokens.filter((token) => token.status !== "active").length;
  const visibleTokens = showInactive
    ? allTokens
    : allTokens.filter((token) => token.status === "active");
  const tokenListUnavailable = Boolean(tokens.error && !tokens.data);

  return (
    <div className="space-y-10">
      <section className="border-y py-7">
        <div>
          <div className="flex items-center gap-2">
            <TerminalSquare className="size-4 text-emerald-700" aria-hidden />
            <h2 className="text-lg font-semibold">新 Agent 授权</h2>
          </div>
          <div className="mt-5 max-w-xl">
            <Label htmlFor="cli-actor">绑定身份</Label>
            <select
              id="cli-actor"
              className="mt-2 h-9 w-full min-w-0 rounded-lg border border-input bg-background px-2.5 text-sm"
              value={actorID}
              onChange={(event) => setActorID(event.target.value)}
            >
              <option value="">本人 · {session.session?.displayName}</option>
              {actors.data?.items.filter((actor) => actor.status === "active").map((actor) => (
                <option key={actor.actorId} value={actor.actorId}>
                  {actor.displayName} · {actor.actorType}
                </option>
              ))}
            </select>
            <p className="mt-2 text-xs leading-5 text-muted-foreground">
              机器 Actor 的提案按该身份署名，最终需要真人审核。
              <Link href="/governance/ai-trust" className="ml-1 underline underline-offset-4">
                创建或管理 Actor
              </Link>
            </p>
            {actors.error ? (
              <p className="mt-2 text-xs text-destructive">机器 Actor 暂时无法读取，请刷新重试。</p>
            ) : null}
          </div>
          <div className="mt-5 grid gap-4 sm:grid-cols-[minmax(0,1fr)_10rem_auto] sm:items-end">
            <div>
              <Label htmlFor="cli-token-name">名称</Label>
              <Input
                id="cli-token-name"
                className="mt-2"
                value={name}
                maxLength={120}
                placeholder="例如 research-agent"
                onChange={(event) => setName(event.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="cli-token-ttl">Token 有效期</Label>
              <select
                id="cli-token-ttl"
                className="mt-2 h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                value={tokenTtlDays}
                onChange={(event) => setTokenTtlDays(event.target.value)}
              >
                <option value="30">30 天</option>
                <option value="90">90 天</option>
                <option value="180">180 天</option>
                <option value="365">365 天</option>
              </select>
            </div>
            <Button type="button" disabled={creating} onClick={() => void create()}>
              {creating ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <KeyRound aria-hidden />
              )}
              生成授权码
            </Button>
          </div>
        </div>
      </section>

      <section aria-labelledby="cli-token-list-title">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 id="cli-token-list-title" className="text-lg font-semibold">
              已授权 Agent
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Token 明文不会再次显示。
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex h-8 items-center gap-2 rounded-lg border px-3 text-xs">
              <Checkbox
                checked={showInactive}
                onCheckedChange={(value) => setShowInactive(value === true)}
              />
              显示已失效
            </label>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={inactiveCount === 0 || cleaningInactive}
              onClick={() => void deleteInactiveRecords()}
            >
              {cleaningInactive ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Trash2 aria-hidden />
              )}
              清理已失效
            </Button>
          </div>
        </div>
        <div className="mt-5 overflow-hidden rounded-lg border">
          {tokens.isLoading ? (
            <div className="h-36 animate-pulse bg-muted/25" />
          ) : null}
          {tokenListUnavailable ? (
            <div className="flex flex-wrap items-center justify-between gap-3 p-6">
              <p className="text-sm text-destructive">CLI Token 暂时不可用。</p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void tokens.mutate()}
              >
                <RefreshCw aria-hidden />
                重新加载
              </Button>
            </div>
          ) : null}
          {!tokens.isLoading &&
          !tokenListUnavailable &&
          pendingAuthorizations.length === 0 &&
          visibleTokens.length === 0 ? (
            <div className="p-8 text-center">
              <TerminalSquare
                className="mx-auto size-6 text-muted-foreground"
                aria-hidden
              />
              <p className="mt-3 text-sm font-semibold">
                {allTokens.length === 0 ? "还没有已授权 Agent" : "没有需要显示的 Token"}
              </p>
            </div>
          ) : null}
          {pendingAuthorizations.map((authorization) => (
            <PendingAuthorizationRow
              key={authorization.id}
              authorization={authorization}
              deleting={deletingAuthorizationID === authorization.id}
              onDelete={() => void deletePendingAuthorization(authorization)}
            />
          ))}
          {visibleTokens.map((token) => (
            <TokenRow
              key={token.id}
              token={token}
              revoking={revokingID === token.id}
              deleting={deletingID === token.id}
              onRevoke={() => void revoke(token)}
              onDelete={() => void deleteRecord(token)}
            />
          ))}
        </div>
      </section>

      <AuthorizationCodeDialog
        value={authorizationCode}
        onClose={() => setAuthorizationCode(undefined)}
      />
    </div>
  );
}
