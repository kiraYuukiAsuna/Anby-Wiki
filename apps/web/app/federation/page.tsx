import { Globe2 } from "lucide-react";

import { FederationWorkspace } from "@/components/knowledge/federation-workspace";

export default function FederationPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-indigo-600 text-white shadow-lg shadow-indigo-600/20">
          <Globe2 className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">跨 Wiki 身份</h1>
      </header>

      <FederationWorkspace />
    </div>
  );
}
