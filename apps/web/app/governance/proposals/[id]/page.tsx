import { WorkspaceLink } from "@/components/workspace-link";
import { ShieldCheck } from "lucide-react";
import { notFound } from "next/navigation";

import { ProposalPreview } from "@/components/governance/proposal-preview";
import { KnowledgeProposalDetail } from "@/components/governance/knowledge-proposal-detail";
import { ProposalActions } from "@/components/governance/proposal-actions";
import { Button } from "@/components/ui/button";
import { fetchProposalWorkspace } from "@/lib/governance";

export const dynamic = "force-dynamic";

export default async function ProposalPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const result = await fetchProposalWorkspace(id);
  if (result.kind === "not_found") notFound();
  if (result.kind === "forbidden") {
    return (
      <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
        <header className="mb-8 border-b pb-6">
          <h1 className="text-3xl font-semibold">提案详情</h1>
        </header>
        <div className="rounded-3xl border border-amber-200 bg-amber-50/65 p-8">
          <ShieldCheck className="size-7 text-amber-700" aria-hidden />
          <h2 className="mt-4 text-lg font-semibold text-amber-950">没有查看此提案的权限</h2>
          <p className="mt-2 text-sm leading-6 text-amber-900/70">
            提案创建者、对应导入任务的发起者或当前 Wiki 的审核者、应用者及管理员可以查看此提案。
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-8">
      <div className="mb-4 flex justify-end">
        <Button variant="outline" size="sm" asChild>
          <WorkspaceLink href="/governance/review">返回审核队列</WorkspaceLink>
        </Button>
      </div>
      <ProposalActions proposal={result.proposal} />
      {result.preview ? (
        <ProposalPreview proposal={result.proposal} preview={result.preview} />
      ) : (
        <KnowledgeProposalDetail proposal={result.proposal} />
      )}
    </div>
  );
}
