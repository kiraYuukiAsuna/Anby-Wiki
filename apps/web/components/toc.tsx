// 目录（TOC）：移动端顶部折叠面板（TableOfContents），桌面端右侧 sticky 栏（TocSidebar）。
// 锚点跳转依赖 globals.css 的 scroll-behavior: smooth；当前章节高亮暂省略（M2 后续可加 IntersectionObserver）。
import { cn } from "@/lib/utils";
import type { TocEntry } from "@/lib/ast/toc";

function TocList({ entries }: { entries: TocEntry[] }) {
  return (
    <ul className="space-y-1 text-sm">
      {entries.map((entry) => (
        <li key={entry.id}>
          <a
            href={`#${entry.id}`}
            className={cn(
              "block truncate text-muted-foreground transition-colors hover:text-foreground",
              entry.level > 1 &&
                "pl-[calc((var(--toc-level)-1)*0.75rem)]",
            )}
            style={{ "--toc-level": entry.level } as React.CSSProperties}
          >
            {entry.text}
          </a>
        </li>
      ))}
    </ul>
  );
}

/** 移动端折叠面板，置于正文之前。 */
export function TableOfContents({ entries }: { entries: TocEntry[] }) {
  if (entries.length === 0) return null;
  return (
    <details className="mb-6 rounded-lg border border-border bg-muted/40 px-4 py-2 lg:hidden">
      <summary className="cursor-pointer py-1 text-sm font-medium">
        本页目录
      </summary>
      <nav aria-label="本页目录" className="pb-2">
        <TocList entries={entries} />
      </nav>
    </details>
  );
}

/** 桌面端右侧 sticky 目录栏，作为正文 article 的兄弟节点。 */
export function TocSidebar({ entries }: { entries: TocEntry[] }) {
  if (entries.length === 0) return null;
  return (
    <aside className="hidden w-56 shrink-0 lg:block">
      <nav
        aria-label="本页目录"
        className="sticky top-20 border-l border-border pl-4"
      >
        <p className="mb-2 text-sm font-medium">本页目录</p>
        <TocList entries={entries} />
      </nav>
    </aside>
  );
}
