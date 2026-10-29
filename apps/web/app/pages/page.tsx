import Link from "next/link";
import { BookOpenText, Search, SquarePen } from "lucide-react";

import { PageDirectory } from "@/components/page-directory";
import { Button } from "@/components/ui/button";
import { WikiSearchForm } from "@/components/wiki-search-form";

export default function PagesPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex flex-wrap items-center justify-between gap-4 border-b pb-6">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
            <BookOpenText className="size-5" aria-hidden />
          </span>
          <h1 className="text-3xl font-semibold">页面目录</h1>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" asChild><Link href="/explore"><Search aria-hidden />全文搜索</Link></Button>
          <Button asChild><Link href="/new"><SquarePen aria-hidden />创建页面</Link></Button>
        </div>
      </header>
      <WikiSearchForm id="page-directory-search" className="mb-8" />
      <PageDirectory />
    </div>
  );
}
