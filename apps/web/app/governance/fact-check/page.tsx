import { ShieldAlert } from "lucide-react";

import { FactConsistencyWorkspace } from "@/components/governance/fact-consistency-workspace";

export default function FactConsistencyPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-amber-100 text-amber-800">
          <ShieldAlert className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-amber-800 uppercase">
          Fact consistency
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          事实一致性检查
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          自动识别单值冲突、多个首选值、缺少支持证据、指向已合并 Entity
          以及证据分歧。问题队列由当前权威事实派生，可重建且不会自动改写 Claim。
        </p>
      </header>
      <FactConsistencyWorkspace />
    </div>
  );
}
