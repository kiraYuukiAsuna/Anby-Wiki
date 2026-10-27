"use client";

import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useSWRConfig } from "swr";

import { ResponseError } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { wikiApi } from "@/lib/api";
import { AUTH_SESSION_KEY, isUnauthorized } from "@/lib/auth";
import { persistSelectedWikiID } from "@/lib/wiki-context";

function tokenFromHash() {
  const hash = window.location.hash.startsWith("#")
    ? window.location.hash.slice(1)
    : window.location.hash;
  return new URLSearchParams(hash).get("token")?.trim() ?? "";
}

export function ShareLinkAcceptView() {
  const router = useRouter();
  const { mutate } = useSWRConfig();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const accept = async () => {
      const token = tokenFromHash();
      if (!token) {
        setError("这个分享链接缺少授权 token。");
        return;
      }
      try {
        const result = await wikiApi().acceptPageShareLink({
          acceptPageShareLinkRequest: { token },
        });
        if (cancelled) return;
        persistSelectedWikiID(result.wikiId);
        await mutate(AUTH_SESSION_KEY);
        toast.success(result.changed ? "已获得页面权限" : "你已拥有页面权限");
        router.replace(`/pages/${result.pageId}`);
        router.refresh();
      } catch (candidate) {
        if (cancelled) return;
        if (isUnauthorized(candidate)) {
          setError("登录状态已失效，请重新登录后再打开分享链接。");
        } else if (
          candidate instanceof ResponseError &&
          candidate.response.status === 404
        ) {
          setError("这个分享链接不存在、已被使用，或对应页面已不可用。");
        } else if (
          candidate instanceof ResponseError &&
          candidate.response.status === 422
        ) {
          setError("这个分享链接格式不正确。");
        } else {
          setError("领取分享链接失败，请稍后重试。");
        }
      }
    };
    void accept();
    return () => {
      cancelled = true;
    };
  }, [mutate, router]);

  if (error) {
    return (
      <main className="mx-auto flex min-h-screen w-full max-w-md flex-col items-center justify-center px-5 text-center">
        <span className="flex size-11 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
          <TriangleAlert className="size-5" aria-hidden />
        </span>
        <h1 className="mt-5 text-xl font-semibold">无法领取分享链接</h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">{error}</p>
        <Button className="mt-6" onClick={() => router.replace("/")}>
          返回首页
        </Button>
      </main>
    );
  }

  return (
    <main className="flex min-h-screen items-center justify-center px-5 text-sm text-muted-foreground">
      <LoaderCircle className="mr-2 size-4 animate-spin" aria-hidden />
      正在领取页面权限…
    </main>
  );
}
