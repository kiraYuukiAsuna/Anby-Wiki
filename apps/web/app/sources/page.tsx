import { LibraryBig } from "lucide-react";

import { SourceLibrary } from "@/components/sources/source-library";

export default function SourcesPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-emerald-100 text-emerald-700">
          <LibraryBig className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">来源与证据</h1>
      </header>
      <SourceLibrary />
    </div>
  );
}
