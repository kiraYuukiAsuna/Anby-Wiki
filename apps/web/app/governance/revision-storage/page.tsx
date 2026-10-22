import { Archive } from "lucide-react";

import { RevisionStorageWorkspace } from "@/components/governance/revision-storage-workspace";

export default function RevisionStoragePage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-sky-100 text-sky-800">
          <Archive className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">历史版本存储</h1>
      </header>
      <RevisionStorageWorkspace />
    </div>
  );
}
