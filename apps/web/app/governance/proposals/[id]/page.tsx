import Link from "next/link";
import { notFound } from "next/navigation";

import { ProposalPreview } from "@/components/governance/proposal-preview";
import { KnowledgeProposalDetail } from "@/components/governance/knowledge-proposal-detail";
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

  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-8">
      <div className="mb-4 flex justify-end">
        <Button variant="outline" size="sm" asChild>
          <Link href="/governance/review">返回审核队列</Link>
        </Button>
      </div>
      {result.preview ? (
        <ProposalPreview proposal={result.proposal} preview={result.preview} />
      ) : (
        <KnowledgeProposalDetail proposal={result.proposal} />
      )}
    </div>
  );
}
