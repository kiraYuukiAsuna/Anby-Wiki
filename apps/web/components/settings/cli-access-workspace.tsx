"use client";

import {
  Check,
  Copy,
  KeyRound,
  LoaderCircle,
  ShieldCheck,
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

export function CLIAccessWorkspace() {
  const session = useSession();
  const [name, setName] = useState("");
  const [tokenTtlDays, setTokenTtlDays] = useState("90");
  const [creating, setCreating] = useState(false);
  const [authorizationCode, setAuthorizationCode] = useState<CLIAuthCode>();
  const [revokingID, setRevokingID] = useState("");
  const [deletingID, setDeletingID] = useState("");
  const [cleaningInactive, setCleaningInactive] = useState(false);
  const [showInactive, setShowInactive] = useState(false);
  const tokens = useSWR(
    session.isAuthenticated ? "settings:cli-tokens" : null,
    () => authApi().listCLITokens(),
    {
      revalidateOnFocus: true,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );

  const create = async () => {
    const parsed = formSchema.safeParse({ name, tokenTtlDays });
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
      toast.success("一次性授权码已生成");
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("需要浏览器登录会话");
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
  const inactiveCount = allTokens.filter((token) => token.status !== "active").length;
  const visibleTokens = showInactive
    ? allTokens
    : allTokens.filter((token) => token.status === "active");

  return (
    <div className="space-y-10">
      <section className="grid gap-8 border-y py-7 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div>
          <div className="flex items-center gap-2">
            <TerminalSquare className="size-4 text-emerald-700" aria-hidden />
            <h2 className="text-lg font-semibold">新 Agent 授权</h2>
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
        <div className="border-l-0 pt-0 lg:border-l lg:pl-7">
          <ShieldCheck className="size-4 text-emerald-700" aria-hidden />
          <p className="mt-3 text-sm font-semibold">权限随账号实时变化</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            Agent 使用当前账号的角色和页面保护权限。停用账号或撤销 Token 后立即失效。
          </p>
        </div>
      </section>

      <section aria-labelledby="cli-token-list-title">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 id="cli-token-list-title" className="text-lg font-semibold">
              已授权 Agent
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Token 明文不会返回；这里只保留前缀、生命周期和最近使用时间。
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
          {tokens.error ? (
            <p className="p-6 text-sm text-destructive">CLI Token 暂时不可用。</p>
          ) : null}
          {!tokens.isLoading && !tokens.error && visibleTokens.length === 0 ? (
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
