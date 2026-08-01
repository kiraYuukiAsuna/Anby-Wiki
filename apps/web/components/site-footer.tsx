import Link from "next/link";

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-border/70 bg-background/55">
      <div className="mx-auto flex w-full max-w-7xl flex-wrap items-center justify-between gap-3 px-5 py-5 text-xs text-muted-foreground lg:px-8">
        <span>© Anby Wiki · 人工与 AI 共同维护</span>
        <nav aria-label="页脚导航" className="flex items-center gap-4">
          <Link href="/collections" className="transition-colors hover:text-foreground">
            专题合集
          </Link>
          <Link href="/imports" className="transition-colors hover:text-foreground">
            导入中心
          </Link>
          <Link href="/assets" className="transition-colors hover:text-foreground">
            媒体库
          </Link>
          <Link href="/sources" className="transition-colors hover:text-foreground">
            来源
          </Link>
          <Link href="/entities" className="transition-colors hover:text-foreground">
            实体
          </Link>
          <Link href="/datasets" className="transition-colors hover:text-foreground">
            数据集
          </Link>
          <Link href="/components" className="transition-colors hover:text-foreground">
            组件
          </Link>
          <Link
            href="/governance"
            className="transition-colors hover:text-foreground"
          >
            治理中心
          </Link>
        </nav>
      </div>
    </footer>
  );
}
