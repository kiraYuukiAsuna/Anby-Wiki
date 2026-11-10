"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Copy, KeyRound, LoaderCircle, MailCheck, Monitor, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWR, { useSWRConfig } from "swr";
import { z } from "zod";
import {
  AccountSecurityRequestActionEnum as Action,
  type AccountSecurityRequest,
  type AccountSecurityResult,
} from "../../../../contracts/generated/typescript";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { accountApi } from "@/lib/api";
import { AUTH_SESSION_KEY, isUnauthorized, useSession } from "@/lib/auth";
import { newPasswordSchema, SECURITY_ACTION_LABELS, securityErrorMessage } from "@/lib/account-security";

export function AccountSecurityPanel() {
  const { session } = useSession();
  const { mutate } = useSWRConfig();
  const router = useRouter();
  const security = useSWR(
    session ? ["account:security", session.actorId] : null,
    () => accountApi().getAccountSecurity(),
    { revalidateOnFocus: false, shouldRetryOnError: (error) => !isUnauthorized(error) },
  );
  const [request, setRequest] = useState<AccountSecurityRequest | null>(null);
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [email, setEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<AccountSecurityResult | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);

  const execute = async (input: AccountSecurityRequest) => {
    if (busy) return;
    setBusy(true);
    try {
      const value = await accountApi().mutateAccountSecurity({ accountSecurityRequest: input });
      setRequest(null);
      setPassword("");
      setCode("");
      setResult(value);
      if (value.recoveryCodes) setRecoveryCodes(value.recoveryCodes);
      if (value.signedOut) {
        toast.success("安全设置已更新，请重新登录");
        await mutate(AUTH_SESSION_KEY, undefined, { revalidate: false });
        router.replace("/login");
        router.refresh();
        return;
      }
      await security.mutate();
      await mutate(AUTH_SESSION_KEY);
      toast.success(input.action === Action.VerifyEmail || input.action === Action.ChangeEmail
        ? "验证邮件已发送，请检查收件箱" : "安全设置已更新");
    } catch (error) {
      toast.error(await securityErrorMessage(error));
    } finally {
      setBusy(false);
    }
  };

  const begin = (action: AccountSecurityRequest) => {
    setPassword("");
    setCode("");
    setEmail("");
    setNewPassword("");
    setRequest(action);
  };

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (!request) return;
    const base = z.object({ password: z.string().min(1, "请输入当前密码").max(128), code: z.string().trim().max(32) })
      .safeParse({ password, code });
    if (!base.success) { toast.error(base.error.issues[0]?.message); return; }
    if ((security.data?.mfaEnabled || request.action === Action.EnableMfa) && !base.data.code) {
      toast.error("请输入验证器验证码或恢复码"); return;
    }
    const input = { ...request, ...base.data };
    if (request.action === Action.ChangeEmail) {
      const parsed = z.string().trim().email("请输入有效邮箱").max(254).safeParse(email);
      if (!parsed.success) { toast.error(parsed.error.issues[0]?.message); return; }
      input.email = parsed.data;
    }
    if (request.action === Action.ChangePassword) {
      const parsed = newPasswordSchema.safeParse(newPassword);
      if (!parsed.success) { toast.error(parsed.error.issues[0]?.message); return; }
      input.newPassword = parsed.data;
    }
    void execute(input);
  };

  if (security.isLoading) return <p className="flex items-center gap-2 p-6 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />正在读取安全设置…</p>;
  if (security.error || !security.data) return <div className="rounded-3xl border p-6"><p>无法读取账号安全设置。</p><Button className="mt-3" variant="outline" onClick={() => void security.mutate()}>重试</Button></div>;
  const state = security.data;

  return (
    <section id="account-security" className="min-w-0 rounded-3xl border bg-card p-6 sm:p-7">
      <h2 className="flex items-center gap-2 text-xl font-semibold"><ShieldCheck className="size-5" />账号安全</h2>
      <div className="mt-6 grid min-w-0 gap-6 lg:grid-cols-2">
        <div className="min-w-0 rounded-xl border p-4">
          <h3 className="flex items-center gap-2 font-medium"><MailCheck className="size-4" />邮箱验证</h3>
          <p className="mt-3 break-all text-sm">{state.email}</p>
          <p className="mt-1 text-sm text-muted-foreground">{state.emailVerified ? "已验证，可用于密码恢复" : "尚未验证，暂不能用于密码恢复"}</p>
          {!state.mailEnabled && <p className="mt-2 text-sm text-muted-foreground">系统管理员尚未启用邮件发送。</p>}
          <div className="mt-4 flex flex-wrap gap-2">
            {!state.emailVerified && <Button size="sm" disabled={busy || !state.mailEnabled} onClick={() => void execute({ action: Action.VerifyEmail })}>发送验证邮件</Button>}
            <Button size="sm" variant="outline" disabled={busy || !state.mailEnabled} onClick={() => begin({ action: Action.ChangeEmail })}>变更邮箱</Button>
          </div>
        </div>
        <div className="min-w-0 rounded-xl border p-4">
          <h3 className="flex items-center gap-2 font-medium"><KeyRound className="size-4" />二次验证</h3>
          <p className="mt-3 text-sm text-muted-foreground">{state.mfaEnabled ? `已绑定验证器 · 剩余 ${state.recoveryCodesRemaining} 个恢复码` : "使用手机验证器生成动态验证码，提高账号安全性。"}</p>
          <div className="mt-4 flex flex-wrap gap-2">
            {state.mfaEnabled ? <>
              <Button size="sm" variant="outline" disabled={busy} onClick={() => begin({ action: Action.RegenerateCodes })}>重新生成恢复码</Button>
              <Button size="sm" variant="outline" disabled={busy} onClick={() => begin({ action: Action.DisableMfa })}>停用二次验证</Button>
            </> : <Button size="sm" disabled={busy} onClick={() => begin({ action: Action.SetupMfa })}>绑定验证器</Button>}
          </div>
        </div>
      </div>
      {result?.setupSecret && !state.mfaEnabled && <div className="mt-5 min-w-0 rounded-xl border bg-muted/30 p-4">
        <h3 className="font-medium">将密钥添加到验证器</h3>
        <p className="mt-2 text-sm text-muted-foreground">在 Google Authenticator、Microsoft Authenticator 或其他验证器中手动添加密钥，选择基于时间的 6 位验证码。10 分钟内完成绑定。</p>
        <code className="mt-3 block break-all rounded-lg border bg-background p-3">{result.setupSecret}</code>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={() => void navigator.clipboard.writeText(result.setupSecret!).then(() => toast.success("密钥已复制")).catch(() => toast.error("复制失败，请手动复制"))}><Copy className="size-4" />复制密钥</Button>
          <Button size="sm" variant="outline" asChild><a href={result.setupUri}>在验证器中打开</a></Button>
          <Button size="sm" onClick={() => begin({ action: Action.EnableMfa })}>输入验证码并启用</Button>
        </div>
      </div>}
      <div className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t pt-5">
        <div><h3 className="font-medium">登录密码</h3><p className="mt-1 text-sm text-muted-foreground">修改后，所有设备和 CLI 都需要重新登录或授权。</p></div>
        <Button variant="outline" disabled={busy} onClick={() => begin({ action: Action.ChangePassword })}>修改密码</Button>
      </div>
      <div className="mt-6 border-t pt-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="flex items-center gap-2 font-medium"><Monitor className="size-4" />登录设备</h3>
          <Button size="sm" variant="outline" disabled={busy} onClick={() => begin({ action: Action.RevokeSessions })}>退出所有设备并撤销 CLI 授权</Button>
        </div>
        <ul className="mt-3 divide-y">
          {state.sessions.map((device) => <li key={device.id} className="flex min-w-0 flex-wrap items-center justify-between gap-3 py-3">
            <div className="min-w-0 flex-1"><p className="break-all text-sm">{device.userAgent || "浏览器会话"}{device.current && <span className="ml-2 text-primary">当前设备</span>}</p>
              <p className="mt-1 text-xs text-muted-foreground">最近活动 {device.lastSeenAt.toLocaleString()} · 到期 {device.expiresAt.toLocaleString()}</p></div>
            <Button size="sm" variant="outline" disabled={busy} onClick={() => begin({ action: Action.RevokeSession, sessionId: device.id })}>撤销</Button>
          </li>)}
        </ul>
      </div>
      {state.events.length > 0 && <div className="mt-5 border-t pt-5"><h3 className="font-medium">近期安全活动</h3><ul className="mt-3 space-y-2 text-sm">{state.events.slice(0, 10).map((event) => <li key={event.id} className="flex flex-wrap justify-between gap-2"><span>{SECURITY_ACTION_LABELS[event.action] ?? event.action}</span><time className="text-muted-foreground">{event.createdAt.toLocaleString()}</time></li>)}</ul></div>}
      {state.systemAdmin && <p className="mt-5 text-sm"><Link className="font-medium text-primary underline underline-offset-4" href="/admin">打开系统管理</Link></p>}

      <Dialog open={!!request} onOpenChange={(open) => { if (!open && !busy) setRequest(null); }}>
        <DialogContent showCloseButton={!busy}>
          <DialogHeader><DialogTitle>{request && SECURITY_ACTION_LABELS[request.action]}</DialogTitle><DialogDescription>请重新验证身份。验证码每 30 秒更新，同一个验证码不能重复使用。修改邮箱须确认新邮箱；停用二次验证、修改密码或撤销全部会话后需要重新登录。</DialogDescription></DialogHeader>
          <form onSubmit={submit} className="space-y-4">
            <div><Label htmlFor="security-password">当前密码</Label><Input id="security-password" className="mt-2" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required /></div>
            {(state.mfaEnabled || request?.action === Action.EnableMfa) && <div><Label htmlFor="security-code">验证码{state.mfaEnabled ? "或恢复码" : ""}</Label><Input id="security-code" className="mt-2" autoComplete="one-time-code" value={code} onChange={(event) => setCode(event.target.value)} required /></div>}
            {request?.action === Action.ChangeEmail && <div><Label htmlFor="security-email">新邮箱</Label><Input id="security-email" className="mt-2" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></div>}
            {request?.action === Action.ChangePassword && <div><Label htmlFor="security-new-password">新密码（12–128 个字符）</Label><Input id="security-new-password" className="mt-2" type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} required /></div>}
            <Button className="w-full" disabled={busy} type="submit">{busy ? "处理中…" : "确认"}</Button>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={recoveryCodes.length > 0} onOpenChange={(open) => { if (!open) setRecoveryCodes([]); }}>
        <DialogContent><DialogHeader><DialogTitle>保存备用恢复码</DialogTitle><DialogDescription>这些恢复码只显示这一次，每个只能使用一次。请保存在安全的地方；重新生成后旧恢复码全部失效。</DialogDescription></DialogHeader>
          <pre className="overflow-x-auto rounded-lg bg-muted p-4 text-sm">{recoveryCodes.join("\n")}</pre>
          <Button variant="outline" onClick={() => void navigator.clipboard.writeText(recoveryCodes.join("\n")).then(() => toast.success("恢复码已复制")).catch(() => toast.error("复制失败，请手动复制"))}><Copy className="size-4" />复制全部恢复码</Button>
          <Button onClick={() => setRecoveryCodes([])}>我已保存</Button>
        </DialogContent>
      </Dialog>
    </section>
  );
}
