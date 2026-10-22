import { Compass, Network } from "lucide-react";
import Link from "next/link";

import { SearchExplorer } from "@/components/search/search-explorer";
import { Button } from "@/components/ui/button";

type ExploreSearchParams = {
  q?: string;
  mode?: string;
  namespace?: string;
  language?: string;
  entity_type?: string;
};

export default async function ExplorePage({
  searchParams,
}: {
  searchParams: Promise<ExploreSearchParams>;
}) {
  const params = await searchParams;

  return (
    <div className="mx-auto w-full max-w-[92rem] px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex flex-wrap items-center justify-between gap-4 border-b border-border/75 pb-6">
        <div className="flex items-center gap-3">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-lg shadow-primary/20">
          <Compass className="size-5" aria-hidden />
        </span>
          <h1 className="text-3xl font-semibold">搜索</h1>
        </div>
        <Button
          variant="outline"
          asChild
        >
          <Link href="/explore/graph">
            <Network className="size-4" aria-hidden />
            知识关系图
          </Link>
        </Button>
      </header>

      <SearchExplorer
        initialQuery={params.q ?? ""}
        initialMode={params.mode ?? "auto"}
        initialNamespace={params.namespace ?? ""}
        initialLanguage={params.language ?? ""}
        initialEntityType={params.entity_type ?? ""}
      />
    </div>
  );
}
