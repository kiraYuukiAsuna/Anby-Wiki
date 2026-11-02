"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { LogIn } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { useSWRConfig } from "swr";
import { z } from "zod";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { AuthShell } from "@/components/auth-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authApi } from "@/lib/api";
import { AUTH_SESSION_KEY, useSession } from "@/lib/auth";
import { persistSelectedWikiID } from "@/lib/wiki-context";
import { securityErrorMessage } from "@/lib/account-security";

const loginSchema = z.object({
  identifier: z.string().trim().min(1, "请输入用户名或邮箱").max(254),
  password: z.string().min(1, "请输入密码").max(128),
});

export function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { mutate } = useSWRConfig();
  const { session } = useSession();
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [challenge, setChallenge] = useState("");
  const [code, setCode] = useState("");

  // 只接受站内相对路径，避免开放重定向。
  const rawNext = searchParams.get("next") ?? "/";
  const next = rawNext.startsWith("/") && !rawNext.startsWith("//") ? rawNext : "/";
  const registerHref = `/register?next=${encodeURIComponent(next)}`;

  const redirectToNext = useCallback(() => {
    router.replace(next);
    router.refresh();
  }, [next, router]);

  useEffect(() => {
    if (session && !submitting) {
      redirectToNext();
    }
  }, [redirectToNext, session, submitting]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (submitting) return;
    const parsed = loginSchema.safeParse({ identifier, password: challenge ? "mfa" : password });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查登录信息");
      return;
    }
    setSubmitting(true);
    try {
      if (challenge && !z.string().trim().min(1).max(32).safeParse(code).success) {
        toast.error("请输入验证器验证码或恢复码"); return;
      }
      const result = challenge
        ? await authApi().verifyMFA({ securityTokenRequest: { token: challenge, code: code.trim() } })
        : await authApi().login({ loginRequest: parsed.data });
      if (result.mfaRequired && result.challengeToken) {
        setChallenge(result.challengeToken);
        setPassword("");
        return;
      }
      persistSelectedWikiID(result.wikiId);
      await mutate(AUTH_SESSION_KEY);
      toast.success(`已登录为 ${result.displayName}`);
      redirectToNext();
    } catch (error: unknown) {
      // The shared client displays Retry-After for rate-limited requests.
      if (error instanceof ResponseError && error.response.status === 429) return;
      if (error instanceof ResponseError && error.response.status === 401) {
        toast.error("用户名、邮箱或密码错误");
      } else if (challenge) {
        toast.error(await securityErrorMessage(error));
      } else {
        toast.error("登录失败，请稍后重试");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthShell
      title={challenge ? "完成二次验证" : "欢迎回来"}
      footer={
        <>
          还没有账号？{" "}
          <Link
            className="font-semibold text-foreground underline-offset-4 hover:underline"
            href={registerHref}
          >
            创建私人 Wiki
          </Link>
        </>
      }
    >
      <form className="flex flex-col gap-5" onSubmit={submit}>
        {challenge ? <div className="flex flex-col gap-2"><Label htmlFor="login-code">验证码或备用恢复码</Label><Input id="login-code" autoComplete="one-time-code" autoFocus value={code} onChange={(event) => setCode(event.target.value)} required /><p className="text-sm text-muted-foreground">输入手机验证器的 6 位验证码，或一个未使用的恢复码。验证请求 5 分钟内有效。</p><Button type="button" variant="ghost" onClick={() => { setChallenge(""); setCode(""); }}>返回密码登录</Button></div> : <>
        <div className="flex flex-col gap-2">
          <Label htmlFor="identifier">用户名或邮箱</Label>
          <Input
            id="identifier"
            autoComplete="username"
            className="h-12 rounded-[8px] px-3.5"
            placeholder="name@example.com"
            value={identifier}
            onChange={(event) => setIdentifier(event.target.value)}
            required
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="password">密码</Label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            className="h-12 rounded-[8px] px-3.5"
            placeholder="输入密码"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
          />
        </div>
        <Link href="/forgot-password" className="text-sm font-medium text-primary underline-offset-4 hover:underline">忘记密码？</Link>
        </>}
        <Button
          type="submit"
          size="lg"
          className="mt-1 h-12 w-full rounded-[8px] text-sm font-semibold"
          disabled={submitting}
        >
          <LogIn data-icon="inline-start" aria-hidden />
          <span>{submitting ? "登录中…" : challenge ? "验证并登录" : "登录"}</span>
        </Button>
      </form>
    </AuthShell>
  );
}
