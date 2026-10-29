"use client";

import { ArrowRight, Search } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const searchQuerySchema = z.string().trim().min(1).max(255);

export function WikiSearchForm({
  id,
  className,
}: {
  id: string;
  className?: string;
}) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const parsed = searchQuerySchema.safeParse(query);

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!parsed.success) return;
        router.push(`/explore?q=${encodeURIComponent(parsed.data)}`);
      }}
      className={cn(
        "flex w-full max-w-2xl gap-2 rounded-[8px] border bg-background p-2 shadow-sm",
        className,
      )}
      role="search"
    >
      <label htmlFor={id} className="sr-only">搜索百科</label>
      <Search className="ml-2 size-5 shrink-0 self-center text-muted-foreground" aria-hidden />
      <input
        id={id}
        type="search"
        enterKeyHint="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        maxLength={255}
        placeholder="搜索人物、作品、概念或事实…"
        className="min-w-0 flex-1 bg-transparent px-1 text-sm outline-none placeholder:text-muted-foreground"
      />
      <Button type="submit" disabled={!parsed.success}>
        搜索
        <ArrowRight aria-hidden />
      </Button>
    </form>
  );
}
