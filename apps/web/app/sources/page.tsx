import { LibraryBig } from "lucide-react";

import { SourceLibrary } from "@/components/sources/source-library";

export default function SourcesPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-emerald-100 text-emerald-700">
          <LibraryBig className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-emerald-700 uppercase">
          Evidence library
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          来源与证据
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          来源是长期身份，版本记录每次抓取，分片提供可复核定位，Citation
          把正文与结构化事实精确连回证据。
        </p>
      </header>
      <SourceLibrary />
    </div>
  );
}
