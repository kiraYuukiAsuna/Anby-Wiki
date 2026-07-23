import Link from "next/link";
import { notFound } from "next/navigation";

import {
  DetailRows,
  DetailSection,
  DetailShell,
} from "@/components/knowledge/detail-shell";
import { UsageList } from "@/components/knowledge/usage-list";
import { fetchEntityDetail } from "@/lib/knowledge";

export const dynamic = "force-dynamic";

export default async function EntityDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const result = await fetchEntityDetail(id);
  if (result.kind === "not_found") notFound();

  const { detail, usages } = result;
  const title =
    detail.labels.find((label) => label.isPrimary)?.label ?? detail.canonicalKey;

  return (
    <DetailShell eyebrow="Entity" title={title} status={detail.status}>
      <DetailSection title="稳定身份">
        <DetailRows
          rows={[
            { label: "Entity ID", value: detail.id },
            { label: "Canonical key", value: detail.canonicalKey },
            {
              label: "类型",
              value: `${detail.entityType.name} (${detail.entityType.typeKey})`,
            },
            {
              label: "合并目标",
              value: detail.mergedIntoEntityId ? (
                <Link
                  className="text-blue-600 hover:underline"
                  href={`/entities/${detail.mergedIntoEntityId}`}
                >
                  {detail.mergedIntoEntityId}
                </Link>
              ) : (
                "—"
              ),
            },
          ]}
        />
      </DetailSection>

      <DetailSection title="标签与别名">
        <ul className="space-y-2 text-sm">
          {detail.labels.map((label) => (
            <li key={`${label.language}:${label.label}`}>
              <span className="text-muted-foreground">{label.language}</span>{" "}
              <span className="font-medium">{label.label}</span>
              {label.isPrimary ? " · 主标签" : ""}
              {label.description ? ` — ${label.description}` : ""}
            </li>
          ))}
          {detail.aliases.map((alias) => (
            <li key={alias.id}>
              <span className="text-muted-foreground">{alias.language}</span>{" "}
              {alias.alias} · 别名 ({alias.aliasType})
            </li>
          ))}
        </ul>
      </DetailSection>

      <DetailSection title={`页面提及 (${usages.items.length})`}>
        <UsageList items={usages.items} />
      </DetailSection>
    </DetailShell>
  );
}
