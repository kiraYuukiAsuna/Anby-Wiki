// 站点顶部栏：站点名、全局搜索和主要写入入口。
import Link from "next/link";
import { Plus, Upload } from "lucide-react";

import { AccountMenu } from "@/components/account-menu";
import { GlobalSearchCommand } from "@/components/global-search-command";
import { Button } from "@/components/ui/button";

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-10 border-b border-border bg-background/95 backdrop-blur">
      <div className="mx-auto flex h-14 w-full max-w-5xl items-center gap-4 px-4">
        <Link href="/" className="text-base font-semibold tracking-tight">
          Anby Wiki
        </Link>
        <GlobalSearchCommand />
        <Button size="sm" variant="outline" asChild className="gap-1">
          <Link href="/imports">
            <Upload aria-hidden />
            导入
          </Link>
        </Button>
        <Button size="sm" asChild className="gap-1">
          <Link href="/new">
            <Plus aria-hidden />
            新建页面
          </Link>
        </Button>
        <AccountMenu />
      </div>
    </header>
  );
}
