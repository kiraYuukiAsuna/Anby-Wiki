import { Globe2, Link2, ShieldCheck } from "lucide-react";

import { FederationWorkspace } from "@/components/knowledge/federation-workspace";

export default function FederationPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="relative mb-9 overflow-hidden rounded-[2rem] border bg-[radial-gradient(circle_at_88%_14%,color-mix(in_oklch,var(--primary),transparent_79%),transparent_34%),linear-gradient(145deg,var(--card),color-mix(in_oklch,var(--muted),transparent_35%))] px-6 py-8 shadow-[0_20px_70px_rgb(15_23_42/0.06)] sm:px-9 lg:py-10">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-indigo-600 text-white shadow-lg shadow-indigo-600/20">
          <Globe2 className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-indigo-700 uppercase">
          Entity federation
        </p>
        <h1 className="mt-2 max-w-4xl text-4xl font-semibold tracking-[-0.045em] sm:text-5xl">
          连接不同 Wiki 的身份，不混淆彼此的事实
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground sm:text-base">
          为本地稳定 Entity 建立可核验的远端映射。每个身份源都有明确的信任等级，
          每条映射都有关系类型、核验状态与不可变审计。
        </p>
        <div className="mt-6 flex flex-wrap gap-2 text-[11px] font-medium text-muted-foreground">
          <span className="inline-flex items-center gap-1.5 rounded-full border bg-background/75 px-3 py-1.5">
            <Link2 className="size-3.5 text-indigo-700" aria-hidden />
            稳定 ID 映射
          </span>
          <span className="inline-flex items-center gap-1.5 rounded-full border bg-background/75 px-3 py-1.5">
            <ShieldCheck className="size-3.5 text-emerald-700" aria-hidden />
            人工核验与争议状态
          </span>
        </div>
      </header>

      <FederationWorkspace />
    </div>
  );
}
