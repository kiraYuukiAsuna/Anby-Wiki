"use client";

// 目录（TOC）：移动端顶部折叠面板（TableOfContents），桌面端右侧 sticky 栏（TocSidebar）。
// 锚点跳转依赖 globals.css 的 scroll-behavior: smooth；滚动状态兼容异步挂载的章节分片。
import { useEffect, useState } from "react";

import { cn } from "@/lib/utils";
import type { TocEntry } from "@/lib/ast/toc";

function TocList({ entries }: { entries: TocEntry[] }) {
  const activeId = useActiveHeading(entries);

  return (
    <ul className="space-y-1 text-sm">
      {entries.map((entry) => (
        <li key={entry.id}>
          <a
            href={`#${entry.id}`}
            className={cn(
              "relative block truncate rounded-r-md py-0.5 text-muted-foreground transition-colors hover:text-foreground",
              entry.level > 1 &&
                "pl-[calc((var(--toc-level)-1)*0.75rem)]",
              activeId === entry.id &&
                "font-medium text-foreground before:absolute before:inset-y-0 before:-left-[17px] before:w-0.5 before:rounded-full before:bg-primary",
            )}
            aria-current={activeId === entry.id ? "location" : undefined}
            style={{ "--toc-level": entry.level } as React.CSSProperties}
          >
            {entry.text}
          </a>
        </li>
      ))}
    </ul>
  );
}

function useActiveHeading(entries: TocEntry[]): string | undefined {
  const [activeId, setActiveId] = useState<string>();
  const key = entries.map((entry) => entry.id).join("\u0000");

  useEffect(() => {
    if (entries.length === 0) return;
    let frame = 0;
    const update = () => {
      frame = 0;
      const offset = 104;
      let active: string | undefined;
      let nearestBelow: { id: string; top: number } | undefined;
      for (const entry of entries) {
        const heading = document.getElementById(entry.id);
        if (!heading) continue;
        const top = heading.getBoundingClientRect().top;
        if (top <= offset) {
          active = entry.id;
        } else if (!nearestBelow || top < nearestBelow.top) {
          nearestBelow = { id: entry.id, top };
        }
      }
      const next = active ?? nearestBelow?.id;
      setActiveId((current) => (current === next ? current : next));
    };
    const schedule = () => {
      if (frame === 0) frame = window.requestAnimationFrame(update);
    };
    const observer = new MutationObserver(schedule);
    observer.observe(document.body, { childList: true, subtree: true });
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule);
    schedule();
    return () => {
      observer.disconnect();
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
      if (frame !== 0) window.cancelAnimationFrame(frame);
    };
    // key is a stable primitive snapshot of the ordered heading IDs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  return activeId;
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
