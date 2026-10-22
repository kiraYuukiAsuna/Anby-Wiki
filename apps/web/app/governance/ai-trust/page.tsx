import { BrainCircuit } from "lucide-react";

import { AITrustWorkspace } from "@/components/governance/ai-trust-workspace";

export default function AITrustPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-fuchsia-100 text-fuchsia-800">
          <BrainCircuit className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">AI 信任与人工抽样</h1>
      </header>
      <AITrustWorkspace />
    </div>
  );
}
