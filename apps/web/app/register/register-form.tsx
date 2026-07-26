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

const registerSchema = z
  .object({
    username: z
      .string()
      .trim()
      .min(3, "用户名至少 3 个字符")
      .max(32, "用户名最多 32 个字符")
      .regex(
        /^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/,
        "用户名只能包含字母、数字、点、下划线和连字符",
      ),
    email: z.string().trim().email("请输入有效邮箱").max(254),
    displayName: z.string().trim().max(128, "显示名最多 128 个字符"),
    password: z.string().min(12, "密码至少 12 个字符").max(128, "密码最多 128 个字符"),
    confirmPassword: z.string(),
  })
  .refine((value) => value.password === value.confirmPassword, {
    message: "两次输入的密码不一致",
    path: ["confirmPassword"],
  });

export function RegisterForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { mutate } = useSWRConfig();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const rawNext = searchParams.get("next") ?? "/";
  const next = rawNext.startsWith("/") && !rawNext.startsWith("//") ? rawNext : "/";
  const loginHref = `/login?next=${encodeURIComponent(next)}`;

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (submitting) return;
    const parsed = registerSchema.safeParse({
      username,
      email,
      displayName,
      password,
      confirmPassword,
    });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查注册信息");
      return;
    }
    setSubmitting(true);
    try {
      const result = await authApi().register({
        registerRequest: {
          username: parsed.data.username,
          email: parsed.data.email,
          password: parsed.data.password,
          displayName: parsed.data.displayName || undefined,
        },
      });
      await mutate(AUTH_SESSION_KEY);
      toast.success(`账号创建成功，已登录为 ${result.displayName}`);
      router.replace(next);
    } catch (error: unknown) {
      if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("用户名或邮箱已被使用");
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("当前站点已关闭账号注册");
      } else if (error instanceof ResponseError && error.response.status === 429) {
        toast.error("尝试过于频繁", { description: "请稍后再试。" });
      } else {
        toast.error("注册失败，请稍后重试");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="mx-auto flex w-full max-w-sm flex-col gap-6 px-4 py-12">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">创建账号</h1>
        <p className="text-sm text-muted-foreground">
          首个注册账号会成为站点管理员，后续账号默认为编辑者。
        </p>
      </div>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <div className="flex flex-col gap-2">
          <Label htmlFor="username">用户名</Label>
          <Input
            id="username"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            required
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="email">邮箱</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="display-name">显示名（可选）</Label>
          <Input
            id="display-name"
            autoComplete="name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="password">密码</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            minLength={12}
            required
          />
          <p className="text-xs text-muted-foreground">至少 12 个字符。</p>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="confirm-password">确认密码</Label>
          <Input
            id="confirm-password"
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            required
          />
        </div>
        <Button type="submit" disabled={submitting}>
          {submitting ? "创建中…" : "创建账号"}
        </Button>
      </form>
      <p className="text-center text-sm text-muted-foreground">
        已有账号？{" "}
        <Link className="font-medium text-foreground underline-offset-4 hover:underline" href={loginHref}>
          登录
        </Link>
      </p>
    </main>
  );
}
