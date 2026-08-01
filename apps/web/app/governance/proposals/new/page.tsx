import { FileJson2 } from "lucide-react";

import { ProposalOperationStudio } from "@/components/governance/proposal-operation-studio";

export default function NewProposalPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/10 text-primary">
          <FileJson2 className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-primary uppercase">
          Operation studio
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          新建人工提案
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          为高级编辑与运维任务编排版本化 Operation。草稿可恢复、可预览、可审核，并在应用时形成一个可补偿的 ChangeBatch。
        </p>
      </header>
      <ProposalOperationStudio />
    </div>
  );
}
