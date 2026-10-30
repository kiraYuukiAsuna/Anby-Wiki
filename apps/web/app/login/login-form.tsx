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
    const parsed = loginSchema.safeParse({ identifier, password });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查登录信息");
      return;
    }
    setSubmitting(true);
    try {
      const result = await authApi().login({
        loginRequest: parsed.data,
      });
      persistSelectedWikiID(result.wikiId);
      await mutate(AUTH_SESSION_KEY);
      toast.success(`已登录为 ${result.displayName}`);
      redirectToNext();
    } catch (error: unknown) {
      if (error instanceof ResponseError && error.response.status === 401) {
        toast.error("用户名、邮箱或密码错误");
      } else if (error instanceof ResponseError && error.response.status === 429) {
        toast.error("尝试过于频繁", { description: "请稍后再试。" });
      } else {
        toast.error("登录失败，请稍后重试");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthShell
      title="欢迎回来"
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
        <Button
          type="submit"
          size="lg"
          className="mt-1 h-12 w-full rounded-[8px] text-sm font-semibold"
          disabled={submitting}
        >
          <LogIn data-icon="inline-start" aria-hidden />
          <span>{submitting ? "登录中…" : "登录"}</span>
        </Button>
      </form>
    </AuthShell>
  );
}
