import Link from "next/link";
import { Bot, BookOpenText, Plus } from "lucide-react";

import { AccountMenu } from "@/components/account-menu";
import { GlobalSearchCommand } from "@/components/global-search-command";
import { Button } from "@/components/ui/button";

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 min-w-0 border-b border-border/80 bg-background/88 backdrop-blur-xl">
      <div className="flex h-16 w-full min-w-0 items-center gap-3 px-4 sm:px-5 lg:px-6">
        <Link
          href="/"
          className="flex shrink-0 items-center gap-2.5 rounded-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label="Anby Wiki 首页"
        >
          <span className="relative flex size-9 items-center justify-center overflow-hidden rounded-xl bg-primary text-primary-foreground shadow-[0_8px_20px_color-mix(in_oklch,var(--primary),transparent_72%)]">
            <BookOpenText className="size-4.5" aria-hidden />
            <span className="absolute inset-x-1 bottom-0 h-px bg-white/35" />
          </span>
          <span className="hidden sm:block">
            <span className="block text-[15px] font-semibold tracking-[-0.02em]">
              Anby Wiki
            </span>
            <span className="block text-[9px] font-semibold tracking-[0.18em] text-muted-foreground uppercase">
              Living knowledge
            </span>
          </span>
        </Link>

        <div className="mx-auto flex min-w-0 max-w-xl flex-1">
          <GlobalSearchCommand />
        </div>

        <Button
          size="sm"
          variant="ghost"
          asChild
          className="hidden gap-1.5 xl:inline-flex"
        >
          <Link href="/imports">
            <Bot aria-hidden />
            AI 导入
          </Link>
        </Button>
        <Button size="sm" asChild className="hidden gap-1.5 sm:inline-flex">
          <Link href="/new">
            <Plus aria-hidden />
            创建
          </Link>
        </Button>
        <AccountMenu />
      </div>
    </header>
  );
}
