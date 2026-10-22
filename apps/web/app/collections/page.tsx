import { Layers3 } from "lucide-react";

import { CollectionHub } from "@/components/collections/collection-hub";

export default function CollectionsPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-sky-100 text-sky-700">
          <Layers3 className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">专题合集</h1>
      </header>
      <CollectionHub />
    </div>
  );
}
