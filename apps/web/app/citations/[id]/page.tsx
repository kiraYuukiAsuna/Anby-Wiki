import { notFound } from "next/navigation";

import {
  DetailRows,
  DetailSection,
  DetailShell,
} from "@/components/knowledge/detail-shell";
import { ReferenceUsagePanel } from "@/components/knowledge/reference-usage-panel";
import { safeHttpUrl } from "@/lib/http-url";
import { fetchCitationDetail } from "@/lib/knowledge";
import type {
  ExternalResourceSummary,
  SourceChunk,
} from "../../../../../contracts/generated/typescript";

export const dynamic = "force-dynamic";

export default async function CitationDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const result = await fetchCitationDetail(id);
  if (result.kind === "not_found") notFound();

  const { detail, usages } = result;
  // OpenAPI Generator 目前未把 3.1 nullable 反映到 TS 类型；运行时 API 会返回 null。
  const chunk = detail.sourceChunk as SourceChunk | null;
  const resource = detail.externalResource as ExternalResourceSummary | null;
  const resourceUrl = resource?.canonicalUrl ?? resource?.normalizedUrl;
  const safeResourceUrl = safeHttpUrl(resourceUrl);

  return (
    <DetailShell eyebrow="Citation" title={detail.source.title} status="不可变证据">
      <DetailSection title="引用定位">
        <DetailRows
          rows={[
            { label: "Citation ID", value: detail.id },
            { label: "Source Version", value: detail.sourceVersionId },
            { label: "Chunk", value: detail.sourceChunkId ?? "—" },
            { label: "引文", value: detail.quotation ?? "—" },
            {
              label: "细粒度定位",
              value: detail.locator ? JSON.stringify(detail.locator) : "—",
            },
          ]}
        />
      </DetailSection>

      <DetailSection title="来源">
        <DetailRows
          rows={[
            { label: "标题", value: detail.source.title },
            { label: "类型", value: detail.source.sourceType },
            { label: "作者", value: detail.source.author ?? "—" },
            { label: "发布者", value: detail.source.publisher ?? "—" },
            { label: "版本哈希", value: detail.sourceVersion.versionHash },
            {
              label: "外部资源",
              value: safeResourceUrl ? (
                <a
                  href={safeResourceUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-600 hover:underline"
                >
                  {safeResourceUrl}
                </a>
              ) : resourceUrl ? (
                <span data-unsafe-external-link="">{resourceUrl}</span>
              ) : (
                "—"
              ),
            },
          ]}
        />
      </DetailSection>

      {chunk ? (
        <DetailSection title={`来源片段 #${chunk.ordinal}`}>
          <p className="mb-3 text-xs text-muted-foreground">
            定位：{JSON.stringify(chunk.locator)}
          </p>
          <blockquote className="border-l-4 border-border pl-4 text-sm leading-7">
            {chunk.textContent}
          </blockquote>
        </DetailSection>
      ) : null}

      <DetailSection
        title={`页面引用 (${usages.totalPageCount} 个页面 · ${usages.totalUsageCount} 处)`}
      >
        <ReferenceUsagePanel
          kind="citation"
          targetId={detail.id}
          initialPage={usages}
        />
      </DetailSection>
    </DetailShell>
  );
}
