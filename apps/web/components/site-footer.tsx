import { WorkspaceLink } from "@/components/workspace-link";

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-border/70 bg-background/55">
      <div className="mx-auto flex w-full max-w-7xl flex-wrap items-center justify-between gap-3 px-5 py-5 text-xs text-muted-foreground lg:px-8">
        <span>© Anby Wiki</span>
        <nav aria-label="页脚导航" className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <WorkspaceLink href="/" className="transition-colors hover:text-foreground">
            首页
          </WorkspaceLink>
          <WorkspaceLink href="/pages" className="transition-colors hover:text-foreground">
            页面目录
          </WorkspaceLink>
          <WorkspaceLink href="/collections" className="transition-colors hover:text-foreground">
            专题集合
          </WorkspaceLink>
          <WorkspaceLink href="/explore" className="transition-colors hover:text-foreground">
            搜索
          </WorkspaceLink>
          <WorkspaceLink href="/imports" className="transition-colors hover:text-foreground">
            导入资料
          </WorkspaceLink>
          <WorkspaceLink
            href="/governance/review"
            className="transition-colors hover:text-foreground"
          >
            待审核
          </WorkspaceLink>
          <WorkspaceLink
            href="/governance/apply"
            className="transition-colors hover:text-foreground"
          >
            待生效
          </WorkspaceLink>
          <WorkspaceLink href="/account" className="transition-colors hover:text-foreground">
            个人中心
          </WorkspaceLink>
        </nav>
      </div>
    </footer>
  );
}
