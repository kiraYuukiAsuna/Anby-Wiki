"use client";

import { LoaderCircle, Save, ShieldAlert } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWR, { useSWRConfig } from "swr";
import { z } from "zod";

import {
  ResponseError,
  type AccountProfile,
} from "../../../../contracts/generated/typescript";

import { AIConfigWorkspace } from "@/components/admin/ai-config-workspace";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { accountApi } from "@/lib/api";
import { AUTH_SESSION_KEY, isUnauthorized, useSession } from "@/lib/auth";

const ACCOUNT_PROFILE_KEY = "account:profile";

const profileSchema = z.object({
  displayName: z.string().trim().min(1, "请输入显示名").max(128, "显示名最多 128 个字符"),
  email: z.string().trim().email("请输入有效邮箱").max(254, "邮箱最多 254 个字符"),
  wikiName: z.string().trim().min(1, "请输入知识空间名称").max(128, "知识空间名称最多 128 个字符"),
});

function ProfileForm({
  profile,
  onSaved,
}: {
  profile: AccountProfile;
  onSaved: (profile: AccountProfile) => Promise<void>;
}) {
  const [displayName, setDisplayName] = useState(profile.displayName);
  const [email, setEmail] = useState(profile.email);
  const [wikiName, setWikiName] = useState(profile.wikiName);
  const [saving, setSaving] = useState(false);

  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (saving) return;
    const parsed = profileSchema.safeParse({ displayName, email, wikiName });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查个人资料");
      return;
    }
    setSaving(true);
    try {
      const result = await accountApi().updateAccountProfile({
        updateAccountProfileRequest: parsed.data,
      });
      await onSaved(result);
      toast.success("个人资料已保存");
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("邮箱已被使用");
      } else {
        toast.error("个人资料保存失败");
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="rounded-3xl border bg-card p-6 shadow-[0_18px_50px_-42px_rgb(15_23_42/0.55)] sm:p-7">
      <div className="flex flex-wrap items-start justify-between gap-4 border-b pb-5">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">个人资料</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            用户名创建后不可修改。
          </p>
        </div>
        <span className="rounded-full border bg-muted/30 px-3 py-1.5 text-xs font-medium text-muted-foreground">
          @{profile.username}
        </span>
      </div>

      <form className="mt-6 grid gap-5 sm:grid-cols-2" onSubmit={save}>
        <div>
          <Label htmlFor="account-username">用户名</Label>
          <Input
            id="account-username"
            className="mt-2 h-9"
            value={profile.username}
            disabled
          />
        </div>
        <div>
          <Label htmlFor="account-email">邮箱</Label>
          <Input
            id="account-email"
            className="mt-2 h-9"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="account-display-name">显示名</Label>
          <Input
            id="account-display-name"
            className="mt-2 h-9"
            autoComplete="name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="account-wiki-name">知识空间名称</Label>
          <Input
            id="account-wiki-name"
            className="mt-2 h-9"
            value={wikiName}
            onChange={(event) => setWikiName(event.target.value)}
          />
        </div>
        <div className="flex justify-end border-t pt-5 sm:col-span-2">
          <Button type="submit" disabled={saving}>
            {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : <Save aria-hidden />}
            {saving ? "保存中…" : "保存资料"}
          </Button>
        </div>
      </form>
    </section>
  );
}

export function AccountCenter() {
  const session = useSession();
  const { mutate } = useSWRConfig();
  const profile = useSWR(
    session.isAuthenticated ? ACCOUNT_PROFILE_KEY : null,
    () => accountApi().getAccountProfile(),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );

  if (session.isLoading || profile.isLoading) {
    return (
      <div className="flex min-h-72 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在读取个人中心…
      </div>
    );
  }

  if (profile.error || !profile.data) {
    return (
      <div className="rounded-3xl border border-destructive/25 bg-destructive/5 p-8">
        <ShieldAlert className="size-7 text-destructive" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">个人中心不可用</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          请稍后重试，或重新登录后再打开此页。
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <ProfileForm
        key={profile.data.updatedAt.toISOString()}
        profile={profile.data}
        onSaved={async (value) => {
          await profile.mutate(value, { revalidate: false });
          await mutate(AUTH_SESSION_KEY);
        }}
      />
      <AIConfigWorkspace mode="account" />
    </div>
  );
}
