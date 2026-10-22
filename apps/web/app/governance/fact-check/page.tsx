import { ShieldAlert } from "lucide-react";

import { FactConsistencyWorkspace } from "@/components/governance/fact-consistency-workspace";

export default function FactConsistencyPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-amber-100 text-amber-800">
          <ShieldAlert className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">事实一致性检查</h1>
      </header>
      <FactConsistencyWorkspace />
    </div>
  );
}
