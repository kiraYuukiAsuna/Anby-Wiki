import Link from "next/link";
import { WorkspaceLink } from "@/components/workspace-link";

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-border/70 bg-background/55">
      <div className="mx-auto flex w-full max-w-7xl flex-wrap items-center justify-between gap-3 px-5 py-5 text-xs text-muted-foreground lg:px-8">
        <span>© Anby Wiki</span>
        <nav aria-label="页脚导航" className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <WorkspaceLink href="/collections" className="transition-colors hover:text-foreground">
            专题合集
          </WorkspaceLink>
          <WorkspaceLink href="/imports" className="transition-colors hover:text-foreground">
            导入中心
          </WorkspaceLink>
          <Link href="/assets" className="transition-colors hover:text-foreground">
            媒体库
          </Link>
          <Link href="/sources" className="transition-colors hover:text-foreground">
            来源
          </Link>
          <Link href="/entities" className="transition-colors hover:text-foreground">
            实体
          </Link>
          <Link href="/components" className="transition-colors hover:text-foreground">
            组件
          </Link>
          <WorkspaceLink
            href="/governance/review"
            className="transition-colors hover:text-foreground"
          >
            审核队列
          </WorkspaceLink>
        </nav>
      </div>
    </footer>
  );
}
