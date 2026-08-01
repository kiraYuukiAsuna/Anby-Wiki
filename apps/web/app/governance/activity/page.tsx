import { ScrollText } from "lucide-react";

import { AuditActivity } from "@/components/governance/audit-activity";

export default function GovernanceActivityPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-violet-100 text-violet-700">
          <ScrollText className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-violet-700 uppercase">
          Immutable activity
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          审计与变更标签
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          按 Actor、聚合、ChangeBatch 与不可变标签追踪每次权威变更。这里展示的是领域服务写入的审计事实，不是可修改的操作日志。
        </p>
      </header>
      <AuditActivity />
    </div>
  );
}
