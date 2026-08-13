import Link from "next/link";
import { BookOpenText, Search, SquarePen } from "lucide-react";

import { PageDirectory } from "@/components/page-directory";
import { Button } from "@/components/ui/button";

export default function PagesPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex flex-wrap items-end justify-between gap-6 border-b pb-8">
        <div className="max-w-3xl">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
            <BookOpenText className="size-5" aria-hidden />
          </span>
          <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-primary uppercase">All articles</p>
          <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em] sm:text-5xl">全部百科页面</h1>
          <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
            像浏览标准 Wiki 一样，从最近更新或标题目录发现条目；需要精确查找时再进入全文搜索。
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" asChild><Link href="/explore"><Search aria-hidden />全文搜索</Link></Button>
          <Button asChild><Link href="/new"><SquarePen aria-hidden />创建页面</Link></Button>
        </div>
      </header>
      <PageDirectory />
    </div>
  );
}
