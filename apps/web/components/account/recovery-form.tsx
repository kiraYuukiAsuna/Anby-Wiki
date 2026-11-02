"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useSWRConfig } from "swr";
import { z } from "zod";
import { SecurityTokenRequestPurposeEnum } from "../../../../contracts/generated/typescript";
import { AuthShell } from "@/components/auth-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authApi } from "@/lib/api";
import { AUTH_SESSION_KEY } from "@/lib/auth";
import { newPasswordSchema, securityErrorMessage } from "@/lib/account-security";

export function RecoveryForm({ mode }: { mode: "forgot" | "reset" | "verify" }) {
  const { mutate } = useSWRConfig();
  const [token, setToken] = useState("");
  const [purpose, setPurpose] = useState<SecurityTokenRequestPurposeEnum>(SecurityTokenRequestPurposeEnum.VerifyEmail);
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [completed, setCompleted] = useState(false);
  const [message, setMessage] = useState("");
  useEffect(() => {
    // Mail tokens are client-only fragments. Remove them from browser history
    // after reading, without sending them to Next.js or analytics/access logs.
    const readFragment = () => {
      if (!window.location.hash) return;
      const fragment = new URLSearchParams(window.location.hash.slice(1));
      setToken(fragment.get("token") ?? "");
      if (fragment.get("purpose") === SecurityTokenRequestPurposeEnum.ChangeEmail) {
        setPurpose(SecurityTokenRequestPurposeEnum.ChangeEmail);
      }
      window.history.replaceState(null, "", window.location.pathname);
    };
    readFragment();
  }, []);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    if (mode !== "forgot" && !z.string().regex(/^[A-Za-z0-9_-]{43}$/).safeParse(token).success) {
      toast.error("链接无效，请重新申请邮件"); return;
    }
    if (mode === "forgot" && !z.string().trim().min(1).max(254).safeParse(identifier).success) {
      toast.error("请输入用户名或邮箱"); return;
    }
    if (mode === "reset") {
      const parsed = newPasswordSchema.safeParse(password);
      if (!parsed.success) { toast.error(parsed.error.issues[0]?.message); return; }
      if (password !== confirmation) { toast.error("两次密码输入不一致"); return; }
      if (!z.string().max(32).safeParse(code).success) { toast.error("验证码格式无效"); return; }
    }
    setBusy(true);
    try {
      if (mode === "forgot") {
        const value = await authApi().forgotPassword({ forgotPasswordRequest: { identifier: identifier.trim() } });
        setMessage(value.message);
      } else if (mode === "verify") {
        await authApi().verifyEmail({ securityTokenRequest: { token, purpose } });
        setMessage(purpose === SecurityTokenRequestPurposeEnum.ChangeEmail ? "新邮箱已验证，账号邮箱已更新，请重新登录。" : "邮箱已验证，可用于密码恢复。");
        await mutate(AUTH_SESSION_KEY).catch(() => undefined);
      } else {
        await authApi().resetPassword({ securityTokenRequest: { token, password, code: code.trim() } });
        setPassword("");
        setConfirmation("");
        setMessage("密码已重置，原有设备会话和 CLI 凭证已撤销，请重新登录。已启用的二次验证继续有效。");
        await mutate(AUTH_SESSION_KEY, undefined, { revalidate: false });
      }
      setCompleted(true);
      setToken("");
    } catch (error) { toast.error(await securityErrorMessage(error)); }
    finally { setBusy(false); }
  };

  return <AuthShell title={mode === "forgot" ? "找回账号" : mode === "reset" ? "重置密码" : "验证邮箱"} footer={<Link href="/login" className="font-medium text-primary hover:underline">返回登录</Link>}>
    {completed ? <div className="space-y-5"><p className="text-sm leading-6" role="status">{message}</p><Button className="w-full" asChild><Link href="/login">前往登录</Link></Button>{mode === "forgot" && <Button variant="outline" className="w-full" onClick={() => setCompleted(false)}>重新申请</Button>}</div> :
      <form className="space-y-5" onSubmit={submit}>
        {mode === "forgot" ? <><p className="text-sm text-muted-foreground">恢复邮件只发送到已验证的账号邮箱。若验证器不可用，可使用个人中心保存的恢复码。</p><div><Label htmlFor="recovery-identifier">用户名或邮箱</Label><Input id="recovery-identifier" className="mt-2" autoComplete="username" value={identifier} onChange={(event) => setIdentifier(event.target.value)} required /></div></> :
          mode === "verify" ? <p className="text-sm text-muted-foreground">确认此邮箱属于你。链接 30 分钟内有效，只能使用一次。</p> :
            <><div><Label htmlFor="reset-password">新密码（12–128 个字符）</Label><Input id="reset-password" className="mt-2" type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} required /></div><div><Label htmlFor="reset-confirmation">确认新密码</Label><Input id="reset-confirmation" className="mt-2" type="password" autoComplete="new-password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} required /></div><div><Label htmlFor="reset-code">验证码或恢复码</Label><Input id="reset-code" className="mt-2" autoComplete="one-time-code" value={code} onChange={(event) => setCode(event.target.value)} /><p className="mt-2 text-xs text-muted-foreground">账号启用二次验证时必须填写，未启用可留空。</p></div></>}
        {mode !== "forgot" && !token && <p className="text-sm text-destructive">缺少有效链接，请从最新邮件中重新打开此页面。</p>}
        <Button className="w-full" type="submit" disabled={busy || (mode !== "forgot" && !token)}>{busy ? "处理中…" : mode === "forgot" ? "发送恢复邮件" : mode === "verify" ? "确认邮箱" : "重置密码"}</Button>
      </form>}
  </AuthShell>;
}
