import { Compass, Network, Search, Sparkles } from "lucide-react";
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
      <header className="relative mb-8 overflow-hidden rounded-[2rem] border bg-[radial-gradient(circle_at_85%_15%,color-mix(in_oklch,var(--primary),transparent_80%),transparent_34%),linear-gradient(145deg,var(--card),color-mix(in_oklch,var(--muted),transparent_35%))] px-6 py-8 shadow-[0_20px_70px_rgb(15_23_42/0.06)] sm:px-9 lg:py-10">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-lg shadow-primary/20">
          <Compass className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-primary uppercase">
          Knowledge discovery
        </p>
        <h1 className="mt-2 max-w-4xl text-4xl font-semibold tracking-[-0.045em] sm:text-5xl">
          不只找到字面相同的词，也找到同一个意思
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground sm:text-base">
          在页面正文、标题、别名和 Entity 文本间探索。精确检索适合名称与术语，
          混合检索兼顾字面相关性和概念相似度，纯语义检索适合用自然语言提问。
        </p>
        <Button
          variant="outline"
          asChild
          className="mt-6 bg-background/70 backdrop-blur"
        >
          <Link href="/explore/graph">
            <Network className="size-4" aria-hidden />
            打开知识关系图
          </Link>
        </Button>
        <div className="pointer-events-none absolute right-8 top-8 hidden gap-3 xl:flex">
          <span className="flex size-16 rotate-6 items-center justify-center rounded-3xl border bg-background/70 text-primary shadow-sm backdrop-blur">
            <Search className="size-7" aria-hidden />
          </span>
          <span className="mt-14 flex size-14 -rotate-6 items-center justify-center rounded-3xl border bg-background/70 text-violet-600 shadow-sm backdrop-blur">
            <Sparkles className="size-6" aria-hidden />
          </span>
        </div>
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
