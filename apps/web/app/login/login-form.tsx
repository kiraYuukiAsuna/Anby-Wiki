// 引导令牌登录表单。
//
// 早期阶段占位登录：以共享引导令牌换取服务端 session，没有身份提供方。
// 该表单不校验调用者真实身份，公网暴露前必须替换为真实登录流程。
"use client";

import { useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { useSWRConfig } from "swr";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authApi } from "@/lib/api";
import { AUTH_SESSION_KEY } from "@/lib/auth";

export function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { mutate } = useSWRConfig();
  const [token, setToken] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // 只接受站内相对路径，避免开放重定向。
  const rawNext = searchParams.get("next") ?? "/";
  const next = rawNext.startsWith("/") && !rawNext.startsWith("//") ? rawNext : "/";

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!token.trim() || submitting) return;
    setSubmitting(true);
    try {
      const result = await authApi().devLogin({
        devLoginRequest: {
          token: token.trim(),
          displayName: displayName.trim() || undefined,
        },
      });
      await mutate(AUTH_SESSION_KEY);
      toast.success(`已登录为 ${result.displayName}`);
      router.replace(next);
    } catch (error: unknown) {
      if (error instanceof ResponseError && error.response.status === 401) {
        toast.error("引导令牌无效");
      } else if (error instanceof ResponseError && error.response.status === 404) {
        toast.error("引导登录未启用", {
          description: "服务端未配置 AUTH_DEV_LOGIN_ENABLED / AUTH_DEV_LOGIN_TOKEN。",
        });
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
          早期阶段使用共享引导令牌登录，尚未接入身份提供方。
        </p>
      </div>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <div className="flex flex-col gap-2">
          <Label htmlFor="token">引导令牌</Label>
          <Input
            id="token"
            type="password"
            autoComplete="current-password"
            value={token}
            onChange={(event) => setToken(event.target.value)}
            required
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="display-name">显示名（可选）</Label>
          <Input
            id="display-name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            placeholder="Bootstrap user"
          />
        </div>
        <Button type="submit" disabled={submitting || !token.trim()}>
          {submitting ? "登录中…" : "登录"}
        </Button>
      </form>
    </main>
  );
}
