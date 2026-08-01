import { BrainCircuit } from "lucide-react";

import { AITrustWorkspace } from "@/components/governance/ai-trust-workspace";

export default function AITrustPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-fuchsia-100 text-fuchsia-800">
          <BrainCircuit className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-fuchsia-800 uppercase">
          AI trust policy
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          AI 信任与人工抽样
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          为每个 AI 或导入 Actor
          明确设置信任等级和低风险变更的人工抽样比例。高风险变更始终进入审核，未配置的
          Actor 默认按 100% 人工审核处理。
        </p>
      </header>
      <AITrustWorkspace />
    </div>
  );
}
