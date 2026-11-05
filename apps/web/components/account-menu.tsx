"use client";

import { LogIn, LogOut, UserRound, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useSWRConfig } from "swr";

import { Button } from "@/components/ui/button";
import { authApi } from "@/lib/api";
import { AUTH_SESSION_KEY, LOGIN_PATH, useSession } from "@/lib/auth";
import { clearSelectedWikiID, persistSelectedWikiID } from "@/lib/wiki-context";

export function AccountMenu() {
  const { session, isLoading, error } = useSession();
  const { mutate } = useSWRConfig();
  const router = useRouter();

  if (isLoading) {
    return (
      <Button
        size="sm"
        variant="ghost"
        className="shrink-0 px-2 sm:px-3"
        disabled
        aria-label="账户加载中"
      >
        <UserRound aria-hidden />
        <span className="hidden sm:inline">账户</span>
      </Button>
    );
  }

  if (!session) {
    return (
      <Button
        size="sm"
        variant="outline"
        asChild
        className="shrink-0 gap-1 px-2 sm:px-3"
      >
        <a href={LOGIN_PATH}>
          <LogIn aria-hidden />
          <span className="hidden sm:inline">
            {error ? "重试登录" : "登录"}
          </span>
          <span className="sr-only sm:hidden">
            {error ? "重试登录" : "登录"}
          </span>
        </a>
      </Button>
    );
  }

  const logout = async () => {
    try {
      await authApi().logout();
      clearSelectedWikiID();
      await mutate(AUTH_SESSION_KEY, undefined, { revalidate: false });
      toast.success("已退出登录");
    } catch {
      toast.error("退出登录失败，请稍后重试");
    }
  };

  const openAccountCenter = async () => {
    try {
      if (session.homeWikiId !== session.wikiId) {
        persistSelectedWikiID(session.homeWikiId);
        await mutate(AUTH_SESSION_KEY);
      }
      router.push("/account");
      router.refresh();
    } catch {
      toast.error("切换到个人中心失败，请稍后重试");
    }
  };

  return (
    <div className="flex shrink-0 items-center gap-1">
      {session.systemAdmin && <Button size="sm" variant="ghost" asChild className="px-2" aria-label="系统管理台"><Link href="/admin"><ShieldCheck aria-hidden /></Link></Button>}
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="hidden max-w-36 gap-1 px-2 md:inline-flex"
        title={`${session.displayName} (${session.actorType})`}
        onClick={() => void openAccountCenter()}
      >
        <UserRound aria-hidden />
        <span className="truncate">{session.displayName}</span>
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="px-2 md:hidden"
        aria-label="个人中心"
        onClick={() => void openAccountCenter()}
      >
        <UserRound aria-hidden />
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className="gap-1 px-2 sm:px-3"
        onClick={() => void logout()}
        aria-label="退出登录"
      >
        <LogOut aria-hidden />
        <span className="hidden sm:inline">退出</span>
      </Button>
    </div>
  );
}
