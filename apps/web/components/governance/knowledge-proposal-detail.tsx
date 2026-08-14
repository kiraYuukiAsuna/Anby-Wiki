import Link from "next/link";
import type { Proposal } from "../../../../contracts/generated/typescript";

import { compactId } from "@/lib/display-id";

const OPERATION_LABEL: Record<string, string> = {
  create_page: "创建页面", insert_block: "追加内容块", replace_block: "更新内容块",
  create_entity: "创建实体", merge_entity: "合并实体", create_claim: "创建事实",
  supersede_claim: "更新事实", add_claim_source: "补充事实来源",
};

function pageTarget(operation: Proposal["operations"][number]["operation"]) {
  return operation.target.pageId;
}

function operationTitle(operation: Proposal["operations"][number]["operation"]) {
  if (operation.operationType === "create_page") return operation.payload.title;
  return OPERATION_LABEL[operation.operationType] ?? operation.operationType;
}

function operationSummary(operation: Proposal["operations"][number]["operation"]) {
  if (operation.operationType === "create_page") return `生成初始页面内容 · ${operation.payload.summary ?? "无编辑摘要"}`;
  if (operation.operationType === "insert_block") return "在现有页面末尾追加有来源依据的内容块";
  if (operation.operationType === "replace_block") return `替换既有内容块 ${operation.target.blockId ?? ""}`;
  if (operation.operationType === "create_entity") return `创建 ${operation.payload.typeKey} 实体`;
  if (operation.operationType === "create_claim" || operation.operationType === "supersede_claim") return `属性 ${operation.payload.propertyKey}`;
  return null;
}

const RISK_STYLES: Record<string, string> = {
  low: "bg-green-500/10 text-green-700", medium: "bg-amber-500/10 text-amber-700",
  high: "bg-orange-500/10 text-orange-700", critical: "bg-red-500/10 text-red-700",
};

export function KnowledgeProposalDetail({ proposal }: { proposal: Proposal }) {
  const isWikiImport = proposal.targetType === "wiki" && proposal.importJobId;
  const pageOperations = proposal.operations.filter((record) => pageTarget(record.operation));
  const knowledgeOperations = proposal.operations.length - pageOperations.length;
  const affectedPages = new Set(pageOperations.map((record) => pageTarget(record.operation))).size;
  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-medium tracking-widest text-muted-foreground uppercase">{isWikiImport ? "Atomic Wiki Import" : "Knowledge Proposal"}</p>
          <h1 className="mt-1 text-2xl font-bold tracking-tight">{isWikiImport ? "百科导入变更批次" : "知识变更提案"} {compactId(proposal.id)}</h1>
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
      {isWikiImport ? (
        <section className="grid gap-3 sm:grid-cols-3">
          <div className="rounded-lg border border-border p-4"><p className="text-2xl font-semibold">{affectedPages}</p><p className="mt-1 text-xs text-muted-foreground">涉及页面</p></div>
          <div className="rounded-lg border border-border p-4"><p className="text-2xl font-semibold">{pageOperations.length}</p><p className="mt-1 text-xs text-muted-foreground">页面操作</p></div>
          <div className="rounded-lg border border-border p-4"><p className="text-2xl font-semibold">{knowledgeOperations}</p><p className="mt-1 text-xs text-muted-foreground">实体与事实操作</p></div>
          <p className="sm:col-span-3 rounded-lg bg-muted/60 p-3 text-sm text-muted-foreground">批准后，这些页面与知识操作会在同一个数据库事务内提交；任一操作冲突或失败，整批都不会部分生效。</p>
        </section>
      ) : null}
      <section>
        <h2 className="mb-3 text-lg font-semibold">
          批量 Operation 与证据链 · {proposal.operations.length} 项
        </h2>
        <ol className="space-y-3">
          {proposal.operations.map((record) => (
            <li key={record.id} className="rounded-lg border border-border p-4">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">#{record.sequence} {operationTitle(record.operation)}</span>
                <span className="rounded-full bg-muted px-2 py-0.5 text-xs">{record.operation.risk.level}</span>
              </div>
              {pageTarget(record.operation) ? (
                <p className="mt-2 text-xs text-muted-foreground">
                  {record.operation.operationType === "create_page" ? "计划新页面：" : "目标页面："}
                  {record.operation.operationType === "create_page" ? (
                    <span className="font-mono">{pageTarget(record.operation)}</span>
                  ) : (
                    <Link className="font-mono underline" href={`/pages/${pageTarget(record.operation)}`}>{pageTarget(record.operation)}</Link>
                  )}
                </p>
              ) : null}
              {operationSummary(record.operation) ? <p className="mt-2 text-sm text-muted-foreground">{operationSummary(record.operation)}</p> : null}
              <details className="mt-3">
                <summary className="cursor-pointer text-xs text-muted-foreground">查看 Operation JSON</summary>
                <pre className="mt-2 overflow-auto rounded-lg bg-muted p-3 text-xs">{JSON.stringify(record.operation.payload, null, 2)}</pre>
              </details>
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
