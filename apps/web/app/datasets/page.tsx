import { DatabaseZap } from "lucide-react";

import { DatasetHub } from "@/components/datasets/dataset-hub";

export default function DatasetsPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-cyan-100 text-cyan-700">
          <DatabaseZap className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-cyan-700 uppercase">
          Structured knowledge
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          可查询数据
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground">
          为结构化记录定义 Schema，保存筛选、排序和分组视图，再把 DatasetView
          作为稳定 Block 嵌入百科页面。
        </p>
      </header>
      <DatasetHub />
    </div>
  );
}
