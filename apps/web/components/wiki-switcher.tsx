"use client";

import { Globe2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { useSession } from "@/lib/auth";
import { persistSelectedWikiID } from "@/lib/wiki-context";

export function WikiSwitcher() {
  const { session } = useSession();
  const router = useRouter();

  if (!session || session.wikis.length <= 1) {
    return null;
  }

  const switchWiki = (wikiID: string) => {
    if (wikiID === session.wikiId) return;
    const target = session.wikis.find((wiki) => wiki.wikiId === wikiID);
    persistSelectedWikiID(wikiID);
    toast.success("已切换 Wiki", {
      description: target?.wikiName ?? "正在重新载入当前上下文。",
    });
    router.push("/");
    router.refresh();
  };

  return (
    <label
      className="hidden min-w-0 max-w-52 items-center gap-1.5 rounded-lg border bg-background px-2 py-1 text-xs text-muted-foreground md:flex"
      title="切换当前 Wiki"
    >
      <Globe2 className="size-3.5 shrink-0" aria-hidden />
      <select
        className="min-w-0 flex-1 bg-transparent text-foreground outline-none"
        value={session.wikiId}
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
