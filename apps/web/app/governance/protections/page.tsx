import { LockKeyhole } from "lucide-react";

import { ProtectionWorkspace } from "@/components/governance/protection-workspace";

export default async function GovernanceProtectionsPage({
  searchParams,
}: {
  searchParams: Promise<{ page_id?: string }>;
}) {
  const { page_id: pageId } = await searchParams;

  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-cyan-100 text-cyan-800">
          <LockKeyhole className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-cyan-800 uppercase">
          Page protection
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          页面保护策略
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          为页面操作或尚未创建的标题设置最低角色门槛。规则可到期、可撤销、会叠加，并保留完整审计历史。
        </p>
      </header>
      <ProtectionWorkspace initialPageId={pageId} />
    </div>
  );
}
