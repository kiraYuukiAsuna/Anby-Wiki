import Link from "next/link";
import { notFound } from "next/navigation";

import {
  DetailRows,
  DetailSection,
  DetailShell,
} from "@/components/knowledge/detail-shell";
import { UsageList } from "@/components/knowledge/usage-list";
import { fetchClaimDetail } from "@/lib/knowledge";

export const dynamic = "force-dynamic";

function displayValue(value: unknown): string {
  if (typeof value === "string" || typeof value === "number") return String(value);
  return JSON.stringify(value, null, 2);
}

export default async function ClaimDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const result = await fetchClaimDetail(id);
  if (result.kind === "not_found") notFound();

  const { detail, usages } = result;

  return (
    <DetailShell
      eyebrow="Claim"
      title={detail.property.name}
      status={`${detail.status} · ${detail.verificationStatus}`}
    >
      <DetailSection title="事实">
        <DetailRows
          rows={[
            { label: "Claim ID", value: detail.id },
            {
              label: "主体 Entity",
              value: (
                <Link
                  href={`/entities/${detail.subjectEntityId}`}
                  className="text-blue-600 hover:underline"
                >
                  {detail.subjectEntityId}
                </Link>
              ),
            },
            {
              label: "谓词",
              value: `${detail.property.propertyKey} (${detail.valueType})`,
            },
            { label: "值", value: <span className="whitespace-pre-wrap">{displayValue(detail.value)}</span> },
            { label: "Rank", value: detail.rank },
            { label: "来源类型", value: detail.originType },
            {
              label: "取代为",
              value: detail.supersededBy ? (
                <Link
                  href={`/claims/${detail.supersededBy}`}
                  className="text-blue-600 hover:underline"
                >
                  {detail.supersededBy}
                </Link>
              ) : (
                "—"
              ),
            },
          ]}
        />
      </DetailSection>

      <DetailSection title={`证据 (${detail.sources.length})`}>
        {detail.sources.length ? (
          <ul className="divide-y divide-border">
            {detail.sources.map((source) => (
              <li key={source.citationId} className="py-3 first:pt-0 last:pb-0">
                <Link
                  href={`/citations/${source.citationId}`}
                  className="font-medium text-blue-600 hover:underline"
                >
                  Citation {source.citationId}
                </Link>
                <p className="mt-1 text-xs text-muted-foreground">
                  {source.supportType}
                </p>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">该 Claim 尚未绑定 Citation。</p>
        )}
      </DetailSection>

      <DetailSection title={`页面使用位置 (${usages.items.length})`}>
        <UsageList items={usages.items} />
      </DetailSection>
    </DetailShell>
  );
}
