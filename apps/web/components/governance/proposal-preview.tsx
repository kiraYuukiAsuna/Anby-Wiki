import type {
  Proposal,
  ProposalPreview as ProposalPreviewModel,
  PreviewDocument,
} from "../../../../contracts/generated/typescript";

import { DiffView } from "@/components/history/diff-view";
import { MergeToWorkingDocument } from "@/components/governance/merge-to-working-document";
import { MergeConflictPanel } from "@/components/governance/merge-conflict-panel";

const RISK_STYLES: Record<string, string> = {
  low: "bg-green-500/10 text-green-700",
  medium: "bg-amber-500/10 text-amber-700",
  high: "bg-orange-500/10 text-orange-700",
  critical: "bg-red-500/10 text-red-700",
};

function Snapshot({
  label,
  document,
}: {
  label: string;
  document: PreviewDocument;
}) {
  return (
    <section className="min-w-0 rounded-lg border border-border">
      <header className="border-b border-border px-3 py-2">
        <h3 className="text-sm font-semibold">{label}</h3>
        <p className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
          {document.revisionId || "尚未写入 Revision"} · {document.contentHash}
        </p>
      </header>
      <pre className="max-h-80 overflow-auto p-3 font-mono text-xs leading-relaxed">
        {JSON.stringify(document.ast, null, 2)}
      </pre>
    </section>
  );
}

export function ProposalPreview({
  proposal,
  preview,
}: {
  proposal: Proposal;
  preview: ProposalPreviewModel;
}) {
  const impact = preview.impact;
  const metrics = [
    ["Operation", impact.operationCount],
    ["新增块", impact.addedBlocks],
    ["删除块", impact.removedBlocks],
    ["修改块", impact.changedBlocks],
    ["移动块", impact.movedBlocks],
  ] as const;

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">
            Proposal Preview
          </p>
          <h1 className="mt-1 text-2xl font-bold tracking-tight">
            变更提案 {proposal.id.slice(0, 8)}
          </h1>
          <p className="mt-1 font-mono text-xs text-muted-foreground">
            {proposal.id}
          </p>
        </div>
        <div className="flex flex-wrap gap-2 text-xs">
          <span className="rounded-full bg-muted px-2.5 py-1">
            {proposal.status}
          </span>
          <span
            className={`rounded-full px-2.5 py-1 ${RISK_STYLES[preview.riskLevel] ?? "bg-muted"}`}
          >
            风险 {preview.riskLevel}
          </span>
          {preview.stale ? (
            <span className="rounded-full bg-red-500/10 px-2.5 py-1 text-red-700">
              基线已过期
            </span>
          ) : (
            <span className="rounded-full bg-green-500/10 px-2.5 py-1 text-green-700">
              基线有效
            </span>
          )}
        </div>
      </header>

      <MergeConflictPanel proposal={proposal} />
      <MergeToWorkingDocument proposal={proposal} preview={preview} />

      <section
        aria-label="影响摘要"
        className="grid grid-cols-2 gap-2 sm:grid-cols-5"
      >
        {metrics.map(([label, value]) => (
          <div key={label} className="rounded-lg border border-border p-3">
            <p className="text-xs text-muted-foreground">{label}</p>
            <p className="mt-1 text-xl font-semibold tabular-nums">{value}</p>
          </div>
        ))}
      </section>

      <section>
        <h2 className="mb-3 text-lg font-semibold">Base / Current / Proposed</h2>
        <div className="grid gap-3 lg:grid-cols-3">
          <Snapshot label="Base（提案基线）" document={preview.base} />
          <Snapshot label="Current（当前线上）" document={preview.current} />
          <Snapshot label="Proposed（应用后）" document={preview.proposed} />
        </div>
      </section>

      <div className="grid gap-6 lg:grid-cols-2">
        <section>
          <h2 className="mb-3 text-lg font-semibold">Base → Current</h2>
          <DiffView diff={preview.baseToCurrent} />
        </section>
        <section>
          <h2 className="mb-3 text-lg font-semibold">Base → Proposed</h2>
          <DiffView diff={preview.baseToProposed} />
        </section>
      </div>

      <section>
        <h2 className="mb-3 text-lg font-semibold">
          证据链（{preview.evidence.length}）
        </h2>
        {preview.evidence.length === 0 ? (
          <p className="rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">
            本提案没有附加证据。
          </p>
        ) : (
          <ol className="space-y-2">
            {preview.evidence.map((item, index) => (
              <li
                key={`${item.citationId ?? item.sourceChunkId ?? "note"}-${index}`}
                className="rounded-lg border border-border p-3 text-sm"
              >
                <span className="font-medium">证据 {index + 1}</span>
                <dl className="mt-1 text-xs text-muted-foreground">
                  {item.citationId ? <div>citation: {item.citationId}</div> : null}
                  {item.sourceChunkId ? (
                    <div>source chunk: {item.sourceChunkId}</div>
                  ) : null}
                  {item.note ? <div>备注: {item.note}</div> : null}
                </dl>
              </li>
            ))}
          </ol>
        )}
      </section>
    </div>
  );
}
