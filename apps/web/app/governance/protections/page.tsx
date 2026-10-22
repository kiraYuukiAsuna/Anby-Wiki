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
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-cyan-100 text-cyan-800">
          <LockKeyhole className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">页面保护策略</h1>
      </header>
      <ProtectionWorkspace initialPageId={pageId} />
    </div>
  );
}
