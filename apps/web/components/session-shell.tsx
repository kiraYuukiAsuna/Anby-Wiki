"use client";

import Link from "next/link";
import { LockKeyhole } from "lucide-react";
import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";

import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { SiteNavigation } from "@/components/site-navigation";
import { Button } from "@/components/ui/button";
import { useSession } from "@/lib/auth";

const AUTH_PATHS = new Set(["/login", "/register"]);

export function SessionShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  if (AUTH_PATHS.has(pathname)) {
    return children;
  }

  return (
    <PrivateWikiShell pathname={pathname}>{children}</PrivateWikiShell>
  );
}

function PrivateWikiShell({
  children,
  pathname,
}: {
  children: React.ReactNode;
  pathname: string;
}) {
  const { session, isLoading, error, mutate } = useSession();
  const router = useRouter();
  const loginHref = `/login?next=${encodeURIComponent(pathname)}`;

  useEffect(() => {
    if (!isLoading && !session && !error) {
      router.replace(loginHref);
    }
  }, [error, isLoading, loginHref, router, session]);

  if (isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center px-5 text-sm text-muted-foreground">
        正在验证登录状态…
      </main>
    );
  }

  if (!session) {
    if (!error) {
      return (
        <main className="flex min-h-screen items-center justify-center px-5 text-sm text-muted-foreground">
          正在前往登录…
        </main>
      );
    }
    return (
      <main className="flex min-h-screen items-center justify-center px-5 py-12">
        <div className="flex w-full max-w-sm flex-col items-center text-center">
          <span className="flex size-11 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <LockKeyhole className="size-5" aria-hidden />
          </span>
          <h1 className="mt-5 text-xl font-semibold">需要登录</h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            登录后才能查看和管理你的私人 Wiki。
          </p>
          <div className="mt-6 flex flex-wrap items-center justify-center gap-2">
            <Button asChild>
              <Link href={loginHref}>登录</Link>
            </Button>
            <Button variant="outline" asChild>
              <Link href={`/register?next=${encodeURIComponent(pathname)}`}>
                创建账号
              </Link>
            </Button>
            {error ? (
              <Button variant="ghost" onClick={() => void mutate()}>
                重试
              </Button>
            ) : null}
          </div>
        </div>
      </main>
    );
  }

  return (
    <>
      <a
        href="#main-content"
        className="fixed top-2 left-2 z-50 -translate-y-16 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-transform focus:translate-y-0"
      >
        跳到正文
      </a>
      <SiteHeader />
      <div className="flex min-h-0 w-full min-w-0 flex-1">
        <SiteNavigation />
        <div className="flex min-w-0 flex-1 flex-col">
          <main id="main-content" className="flex min-w-0 flex-1 flex-col">
            {children}
          </main>
          <SiteFooter />
        </div>
      </div>
    </>
  );
}
