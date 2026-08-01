import { Layers3 } from "lucide-react";

import { CollectionHub } from "@/components/collections/collection-hub";

export default function CollectionsPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-sky-100 text-sky-700">
          <Layers3 className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-sky-700 uppercase">
          Curated knowledge
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          专题合集
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground">
          像百科专题页一样策展，也能按 Entity、Claim 与页面目录自动维护。
          每个合集都有稳定地址，Dynamic 查询在打开时反映最新权威知识。
        </p>
      </header>
      <CollectionHub />
    </div>
  );
}
