"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";
import { useSWRConfig } from "swr";
import { z } from "zod";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authApi } from "@/lib/api";
import { AUTH_SESSION_KEY } from "@/lib/auth";

const loginSchema = z.object({
  identifier: z.string().trim().min(1, "请输入用户名或邮箱").max(254),
  password: z.string().min(1, "请输入密码").max(128),
});

export function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { mutate } = useSWRConfig();
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // 只接受站内相对路径，避免开放重定向。
  const rawNext = searchParams.get("next") ?? "/";
  const next = rawNext.startsWith("/") && !rawNext.startsWith("//") ? rawNext : "/";
  const registerHref = `/register?next=${encodeURIComponent(next)}`;

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
      await mutate(AUTH_SESSION_KEY);
      toast.success(`已登录为 ${result.displayName}`);
      router.replace(next);
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
    <main className="mx-auto flex w-full max-w-sm flex-col gap-6 px-4 py-16">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">登录</h1>
        <p className="text-sm text-muted-foreground">
          使用你的用户名或邮箱登录 Anby Wiki。
        </p>
      </div>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <div className="flex flex-col gap-2">
          <Label htmlFor="identifier">用户名或邮箱</Label>
          <Input
            id="identifier"
            autoComplete="username"
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
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
          />
        </div>
        <Button type="submit" disabled={submitting}>
          {submitting ? "登录中…" : "登录"}
        </Button>
      </form>
      <p className="text-center text-sm text-muted-foreground">
        还没有账号？{" "}
        <Link className="font-medium text-foreground underline-offset-4 hover:underline" href={registerHref}>
          注册
        </Link>
      </p>
    </main>
  );
}
