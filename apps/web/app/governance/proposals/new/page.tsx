import { FileJson2 } from "lucide-react";

import { ProposalOperationStudio } from "@/components/governance/proposal-operation-studio";

export default function NewProposalPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/10 text-primary">
          <FileJson2 className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">新建人工提案</h1>
      </header>
      <ProposalOperationStudio />
    </div>
  );
}
