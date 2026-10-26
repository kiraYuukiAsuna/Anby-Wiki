"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";
import { useSWRConfig } from "swr";
import { z } from "zod";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { AuthShell } from "@/components/auth-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authApi } from "@/lib/api";
import { AUTH_SESSION_KEY } from "@/lib/auth";
import { persistSelectedWikiID } from "@/lib/wiki-context";

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
      persistSelectedWikiID(result.wikiId);
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
    <AuthShell
      title="创建私人 Wiki"
      wide
      footer={
        <>
          已有账号？{" "}
          <Link
            className="font-semibold text-foreground underline-offset-4 hover:underline"
            href={loginHref}
          >
            返回登录
          </Link>
        </>
      }
    >
      <form
        className="grid gap-5 sm:grid-cols-2"
        onSubmit={submit}
      >
        <div className="flex flex-col gap-2 sm:col-span-1">
          <Label htmlFor="username">用户名</Label>
          <Input
            id="username"
            autoComplete="username"
            className="h-12 rounded-[8px] px-3.5"
            placeholder="至少 3 个字符"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            required
          />
        </div>
        <div className="flex flex-col gap-2 sm:col-span-1">
          <Label htmlFor="email">邮箱</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            className="h-12 rounded-[8px] px-3.5"
            placeholder="name@example.com"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
        </div>
        <div className="flex flex-col gap-2 sm:col-span-2">
          <Label htmlFor="display-name">显示名（可选）</Label>
          <Input
            id="display-name"
            autoComplete="name"
            className="h-12 rounded-[8px] px-3.5"
            placeholder="你的显示名称"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
          />
        </div>
        <div className="flex flex-col gap-2 sm:col-span-1">
          <Label htmlFor="password">密码</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            className="h-12 rounded-[8px] px-3.5"
            placeholder="至少 12 个字符"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            minLength={12}
            required
          />
        </div>
        <div className="flex flex-col gap-2 sm:col-span-1">
          <Label htmlFor="confirm-password">确认密码</Label>
          <Input
            id="confirm-password"
            type="password"
            autoComplete="new-password"
            className="h-12 rounded-[8px] px-3.5"
            placeholder="再次输入密码"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            required
          />
        </div>
        <Button
          type="submit"
          size="lg"
          className="h-12 w-full rounded-[8px] text-sm font-semibold sm:col-span-2"
          disabled={submitting}
        >
          {submitting ? "创建中…" : "创建账号"}
        </Button>
      </form>
    </AuthShell>
  );
}
