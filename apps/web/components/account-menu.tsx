"use client";

import { LogIn, LogOut, UserRound } from "lucide-react";
import { toast } from "sonner";
import { useSWRConfig } from "swr";

import { Button } from "@/components/ui/button";
import { authApi } from "@/lib/api";
import { AUTH_SESSION_KEY, LOGIN_PATH, useSession } from "@/lib/auth";

export function AccountMenu() {
  const { session, isLoading, error } = useSession();
  const { mutate } = useSWRConfig();

  if (isLoading) {
    return (
      <Button size="sm" variant="ghost" disabled>
        <UserRound aria-hidden />
        账户
      </Button>
    );
  }

  if (!session) {
    return (
      <Button size="sm" variant="outline" asChild className="gap-1">
        <a href={LOGIN_PATH}>
          <LogIn aria-hidden />
          {error ? "重试登录" : "登录"}
        </a>
      </Button>
    );
  }

  const logout = async () => {
    try {
      await authApi().logout();
      await mutate(AUTH_SESSION_KEY, undefined, { revalidate: false });
      toast.success("已退出登录");
    } catch {
      toast.error("退出登录失败，请稍后重试");
    }
  };

  return (
    <div className="flex items-center gap-1">
      <span
        className="hidden max-w-32 truncate text-sm text-muted-foreground md:inline"
        title={`${session.displayName} (${session.actorType})`}
      >
        {session.displayName}
      </span>
      <Button
        size="sm"
        variant="ghost"
        className="gap-1"
        onClick={() => void logout()}
      >
        <LogOut aria-hidden />
        退出
      </Button>
    </div>
  );
}
