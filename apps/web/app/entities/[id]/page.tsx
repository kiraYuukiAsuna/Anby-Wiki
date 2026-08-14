import Link from "next/link";
import { Network } from "lucide-react";
import { notFound } from "next/navigation";

import {
  DetailRows,
  DetailSection,
  DetailShell,
} from "@/components/knowledge/detail-shell";
import { EntityFederationPanel } from "@/components/knowledge/entity-federation-panel";
import { EntityMergePanel } from "@/components/knowledge/entity-merge-panel";
import { EntityMetadataManager } from "@/components/knowledge/entity-metadata-manager";
import {
  groupReferenceUsagesByPage,
  PageGroupedUsageList,
} from "@/components/knowledge/usage-list";
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
  const mentionPages = groupReferenceUsagesByPage(usages.items);
  const title =
    detail.labels.find((label) => label.isPrimary)?.label ?? detail.canonicalKey;

  return (
    <DetailShell
      eyebrow="Entity"
      title={title}
      status={detail.status}
      modeLabel={detail.status === "active" ? "可治理" : "身份映射"}
      description="稳定身份匿名可读；合并是受权限控制、单事务提交并写入不可变审计的治理动作。"
    >
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

      <DetailSection title="实体治理">
        <EntityMergePanel
          sourceEntityId={detail.id}
          sourceTitle={title}
          sourceCanonicalKey={detail.canonicalKey}
          entityTypeKey={detail.entityType.typeKey}
          entityTypeName={detail.entityType.name}
          status={detail.status}
          mergedIntoEntityId={detail.mergedIntoEntityId}
          sourceLabelCount={detail.labels.length}
          sourceAliasCount={detail.aliases.length}
          sourcePageCount={mentionPages.length}
        />
      </DetailSection>

      <DetailSection title="关系探索">
        <Link
          href={`/explore/graph?entity_id=${detail.id}`}
          className="flex items-center justify-between rounded-xl border bg-card px-4 py-3 text-sm font-medium transition-colors hover:border-primary/30 hover:text-primary"
        >
          <span className="flex items-center gap-2">
            <Network className="size-4" aria-hidden />
            以此 Entity 为中心打开知识关系图
          </span>
          <span aria-hidden>→</span>
        </Link>
      </DetailSection>

      <DetailSection title="跨 Wiki 身份">
        <EntityFederationPanel
          entityID={detail.id}
          entityTitle={title}
          status={detail.status}
        />
      </DetailSection>

      <DetailSection title="标签与别名">
        <EntityMetadataManager initialDetail={detail} />
      </DetailSection>

      <DetailSection
        title={`页面提及 (${mentionPages.length} 个页面 · ${usages.items.length} 处)`}
      >
        <PageGroupedUsageList groups={mentionPages} />
      </DetailSection>
    </DetailShell>
  );
}
