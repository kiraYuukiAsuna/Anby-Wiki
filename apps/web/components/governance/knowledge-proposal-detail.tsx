import Link from "next/link";
import type { Proposal } from "../../../../contracts/generated/typescript";

const RISK_STYLES: Record<string, string> = {
  low: "bg-green-500/10 text-green-700", medium: "bg-amber-500/10 text-amber-700",
  high: "bg-orange-500/10 text-orange-700", critical: "bg-red-500/10 text-red-700",
};

export function KnowledgeProposalDetail({ proposal }: { proposal: Proposal }) {
  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">Knowledge Proposal</p>
          <h1 className="mt-1 text-2xl font-bold tracking-tight">知识变更提案 {proposal.id.slice(0, 8)}</h1>
          <p className="mt-1 font-mono text-xs text-muted-foreground">{proposal.id}</p>
        </div>
        <div className="flex gap-2 text-xs">
          <span className="rounded-full bg-muted px-2.5 py-1">{proposal.status}</span>
          <span className={`rounded-full px-2.5 py-1 ${RISK_STYLES[proposal.riskLevel] ?? "bg-muted"}`}>风险 {proposal.riskLevel}</span>
        </div>
      </header>
      <dl className="grid gap-3 rounded-lg border border-border p-4 text-sm sm:grid-cols-3">
        <div><dt className="text-xs text-muted-foreground">来源 ImportJob</dt><dd className="mt-1 font-mono text-xs">{proposal.importJobId ?? "—"}</dd></div>
        <div><dt className="text-xs text-muted-foreground">目标</dt><dd className="mt-1">{proposal.targetType} · {proposal.targetId ?? "新对象"}</dd></div>
        <div><dt className="text-xs text-muted-foreground">风险依据</dt><dd className="mt-1">{proposal.riskReasons.join("；") || "—"}</dd></div>
      </dl>
      <section>
        <h2 className="mb-3 text-lg font-semibold">Operation 与证据链</h2>
        <ol className="space-y-3">
          {proposal.operations.map((record) => (
            <li key={record.id} className="rounded-lg border border-border p-4">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">#{record.sequence} {record.operation.operationType}</span>
                <span className="rounded-full bg-muted px-2 py-0.5 text-xs">{record.operation.risk.level}</span>
              </div>
              <pre className="mt-3 overflow-auto rounded-lg bg-muted p-3 text-xs">{JSON.stringify(record.operation.payload, null, 2)}</pre>
              <ul className="mt-3 space-y-1 text-xs text-muted-foreground">
                {record.operation.evidence.map((item, index) => (
                  <li key={`${item.citationId ?? item.sourceChunkId ?? index}`}>
                    {item.citationId ? <Link className="underline" href={`/citations/${item.citationId}`}>Citation {item.citationId}</Link> : null}
                    {item.sourceChunkId ? <span> · Chunk {item.sourceChunkId}</span> : null}
                    {item.note ? <span> · {item.note}</span> : null}
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}
