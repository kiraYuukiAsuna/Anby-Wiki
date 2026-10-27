"use client";

import { CheckCircle2, Globe2, LoaderCircle } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";
import { persistSelectedWikiID } from "@/lib/wiki-context";

const switchNoticeKey = "anby:wiki-switch-notice";
const switchToastID = "wiki-switch";
const switchHighlightDuration = 6000;

export function WikiSwitcher() {
  const { session, mutate } = useSession();
  const router = useRouter();
  const [pendingWikiID, setPendingWikiID] = useState<string | undefined>();
  const [highlightedWikiID, setHighlightedWikiID] = useState<string | undefined>();

  useEffect(() => {
    if (!session || typeof window === "undefined") return;
    const raw = window.sessionStorage.getItem(switchNoticeKey);
    if (!raw) return;
    try {
      const notice = JSON.parse(raw) as {
        wikiID?: string;
        wikiName?: string;
        createdAt?: number;
      };
      if (!notice.createdAt || Date.now() - notice.createdAt > 30_000) {
        window.sessionStorage.removeItem(switchNoticeKey);
        return;
      }
      if (notice.wikiID !== session.wikiId) {
        return;
      }
      window.sessionStorage.removeItem(switchNoticeKey);
      toast.success(`已切换到「${notice.wikiName ?? session.wikiName}」`, {
        id: switchToastID,
        description: "当前页面、目录和搜索都已使用新的 Wiki。",
        duration: switchHighlightDuration,
      });
      const startHighlight = window.setTimeout(
        () => setHighlightedWikiID(session.wikiId),
        0,
      );
      const endHighlight = window.setTimeout(
        () => setHighlightedWikiID(undefined),
        switchHighlightDuration,
      );
      return () => {
        window.clearTimeout(startHighlight);
        window.clearTimeout(endHighlight);
      };
    } catch {
      window.sessionStorage.removeItem(switchNoticeKey);
    }
  }, [session]);

  if (!session || session.wikis.length <= 1) {
    return null;
  }

  const switchWiki = (wikiID: string) => {
    if (wikiID === session.wikiId) return;
    const target = session.wikis.find((wiki) => wiki.wikiId === wikiID);
    setPendingWikiID(wikiID);
    persistSelectedWikiID(wikiID);
    if (typeof window !== "undefined") {
      window.sessionStorage.setItem(
        switchNoticeKey,
        JSON.stringify({
          wikiID,
          wikiName: target?.wikiName,
          createdAt: Date.now(),
        }),
      );
    }
    toast.loading(`正在切换到「${target?.wikiName ?? "目标 Wiki"}」`, {
      id: switchToastID,
      description: "正在更新当前 Wiki 上下文…",
    });
    void (async () => {
      try {
        await mutate();
        setPendingWikiID(undefined);
        router.replace("/");
        router.refresh();
      } catch {
        if (typeof window !== "undefined") {
          window.sessionStorage.removeItem(switchNoticeKey);
        }
        setPendingWikiID(undefined);
        toast.error("Wiki 切换失败", {
          id: switchToastID,
          description: "请稍后重试。",
        });
      }
    })();
  };
  const switching = Boolean(pendingWikiID);
  const highlighted = highlightedWikiID === session.wikiId;
  const Icon = switching ? LoaderCircle : highlighted ? CheckCircle2 : Globe2;

  return (
    <label
      className={cn(
        "hidden min-w-0 max-w-72 items-center gap-1.5 rounded-lg border bg-background px-2 py-1 text-xs text-muted-foreground transition-colors md:flex",
        switching && "border-primary/60 bg-primary/6 text-primary",
        highlighted && "border-emerald-300 bg-emerald-50 text-emerald-800",
      )}
      title="切换当前 Wiki"
    >
      <Icon
        className={cn("size-3.5 shrink-0", switching && "animate-spin")}
        aria-hidden
      />
      <span className="hidden shrink-0 font-medium lg:inline">
        {switching ? "切换中" : highlighted ? "已切换" : "当前 Wiki"}
      </span>
      <select
        className="min-w-0 flex-1 bg-transparent text-foreground outline-none disabled:cursor-wait disabled:opacity-75"
        value={pendingWikiID ?? session.wikiId}
        disabled={switching}
        onChange={(event) => switchWiki(event.target.value)}
        aria-label="切换当前 Wiki"
      >
        {session.wikis.map((wiki) => (
          <option key={wiki.wikiId} value={wiki.wikiId}>
            {wiki.wikiName}
          </option>
        ))}
      </select>
    </label>
  );
}
