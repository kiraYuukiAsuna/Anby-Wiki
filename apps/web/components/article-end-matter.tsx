import Link from "next/link";
import {
  ArrowUpRight,
  BookOpen,
  ChevronDown,
  FolderTree,
  Network,
  Quote,
  RotateCcw,
} from "lucide-react";

import { safeHttpUrl } from "@/lib/http-url";
import type {
  PageCollection,
  PageReference,
  PageReferenceList,
  RelatedPage,
  RelatedPageList,
} from "../../../contracts/generated/typescript";

const DEFAULT_VISIBLE_REFERENCES = 10;

const RELATED_REASON_LABELS: Record<string, string> = {
  page_link: "正文链接",
  backlink: "被本文提及",
  reciprocal_link: "双向关联",
  collection: "同一专题",
  same_entity: "同一实体",
  entity_relation: "知识图谱关联",
};

function validDate(value: Date | null | undefined): Date | null {
  if (!(value instanceof Date) || Number.isNaN(value.getTime())) return null;
  // OpenAPI Generator currently maps a nullable date-time null to Unix epoch.
  return value.getTime() <= 0 ? null : value;
}

function formatDate(value: Date | null | undefined): string | null {
  const date = validDate(value);
  return date
    ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium" }).format(date)
    : null;
}

function locatorText(locator: PageReference["locator"]): string | null {
  const record = locator as Record<string, unknown>;
  const labels: Array<[string, string]> = [
    ["page", "页"],
    ["section", "章节"],
    ["paragraph", "段落"],
    ["line", "行"],
    ["line_start", "起始行"],
    ["chunk_ordinal", "分块"],
  ];
  const parts = labels.flatMap(([key, label]) => {
    const value = record[key];
    return typeof value === "string" || typeof value === "number"
      ? [`${label} ${String(value)}`]
      : [];
  });
  return parts.length > 0 ? parts.join(" · ") : null;
}

function backlinkLabel(index: number): string {
  let value = index + 1;
  let label = "";
  while (value > 0) {
    value--;
    label = String.fromCharCode(97 + (value % 26)) + label;
    value = Math.floor(value / 26);
  }
  return label;
}

function ReferenceItem({ item }: { item: PageReference }) {
  const publishedAt = formatDate(item.publishedAt);
  const href = safeHttpUrl(item.externalUrl);
  const location = locatorText(item.locator);
  const details = [item.author, item.publisher, publishedAt, location].filter(
    (value): value is string => Boolean(value),
  );
  const occurrences = item.occurrences.length > 0
    ? item.occurrences
    : [{ blockId: item.firstBlockId, nodeId: item.firstNodeId }];

  return (
    <li
      id={`cite-note-${item.number}`}
      value={item.number}
      className="scroll-mt-20 pl-1 text-sm leading-6 text-foreground/90"
    >
      <span className="mr-1.5 inline-flex items-center gap-1 text-xs">
        <RotateCcw className="inline size-3.5 text-muted-foreground" aria-hidden />
        {occurrences.map((occurrence, index) => (
          <a
            key={`${occurrence.blockId}-${occurrence.nodeId}`}
            href={`#cite-ref-${occurrence.blockId}-${occurrence.nodeId}`}
            aria-label={`返回正文引用 ${item.number}${occurrences.length > 1 ? ` 第 ${index + 1} 处` : ""}`}
            className="rounded px-0.5 text-muted-foreground hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {occurrences.length > 1 ? backlinkLabel(index) : "↩"}
          </a>
        ))}
      </span>
      {href ? (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="font-medium text-blue-700 underline-offset-4 hover:underline"
        >
          {item.sourceTitle}
          <ArrowUpRight className="ml-1 inline size-3" aria-hidden />
        </a>
      ) : (
        <Link
          href={`/sources/${item.sourceId}`}
          className="font-medium underline-offset-4 hover:underline"
        >
          {item.sourceTitle}
        </Link>
      )}
      {details.length > 0 ? (
        <span className="text-muted-foreground"> · {details.join(" · ")}</span>
      ) : null}
      {item.occurrenceCount > 1 ? (
        <span className="text-muted-foreground">
          {` · 正文引用 ${item.occurrenceCount} 次`}
        </span>
      ) : null}
      {item.quotation ? (
        <details className="mt-2 rounded-lg border border-border/70 bg-muted/30 px-3 py-2">
          <summary className="flex cursor-pointer list-none items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <Quote className="size-3.5" aria-hidden />
            查看核对片段
          </summary>
          <blockquote className="mt-2 border-l-2 border-border pl-3 text-xs leading-5 text-muted-foreground">
            {item.quotation}
          </blockquote>
        </details>
      ) : null}
      <Link
        href={`/citations/${item.citationId}`}
        className="ml-2 whitespace-nowrap text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
      >
        证据详情
      </Link>
    </li>
  );
}

function reasonLabels(item: RelatedPage): string[] {
  return item.reasons.slice(0, 3).map((reason) =>
    reason.label
      ? `${RELATED_REASON_LABELS[reason.type] ?? reason.type}：${reason.label}`
      : (RELATED_REASON_LABELS[reason.type] ?? reason.type),
  );
}

export function ArticleEndMatter({
  references,
  related,
  collections,
}: {
  references?: PageReferenceList;
  related?: RelatedPageList;
  collections: PageCollection[];
}) {
  const showReferenceStatus = references?.ready === false;
  const referenceItems = references?.ready ? references.items : [];
  const visibleReferenceItems = referenceItems.slice(
    0,
    DEFAULT_VISIBLE_REFERENCES,
  );
  const collapsedReferenceItems = referenceItems.slice(
    DEFAULT_VISIBLE_REFERENCES,
  );
  const relatedItems = related?.ready ? related.items : [];
  if (
    !showReferenceStatus &&
    referenceItems.length === 0 &&
    relatedItems.length === 0 &&
    collections.length === 0
  ) {
    return null;
  }

  return (
    <div className="clear-both mt-10 space-y-9 border-t border-border pt-1">
      {relatedItems.length > 0 ? (
        <section aria-labelledby="related-topics">
          <h2
            id="related-topics"
            className="scroll-mt-20 border-b border-border pb-2 pt-7 text-2xl font-semibold tracking-tight"
          >
            Related topics
          </h2>
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            {relatedItems.map((item) => (
              <Link
                key={item.pageId}
                href={`/pages/${item.pageId}`}
                className="group rounded-xl border border-border bg-card p-4 transition hover:border-primary/25 hover:shadow-sm"
              >
                <span className="flex items-center gap-2 font-medium group-hover:text-primary">
                  <Network className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                  {item.displayTitle}
                </span>
                <span className="mt-2 block text-xs leading-5 text-muted-foreground">
                  {reasonLabels(item).join(" · ")}
                </span>
              </Link>
            ))}
          </div>
        </section>
      ) : null}

      {collections.length > 0 ? (
        <section aria-labelledby="related-outlines">
          <h2
            id="related-outlines"
            className="scroll-mt-20 border-b border-border pb-2 text-2xl font-semibold tracking-tight"
          >
            Related outlines
          </h2>
          <p className="mt-3 text-sm text-muted-foreground">
            收录本文的专题与结构化条目集合。
          </p>
          <div className="mt-4 flex flex-wrap gap-2">
            {collections.map((collection) => (
              <Link
                key={collection.id}
                href={`/collections/${collection.id}`}
                className="inline-flex items-center gap-1.5 rounded-full border border-border bg-muted/40 px-3 py-1.5 text-sm transition hover:border-primary/30 hover:bg-muted"
                title={`归属来源：${collection.membershipSource}`}
              >
                <FolderTree className="size-3.5 text-muted-foreground" aria-hidden />
                {collection.title}
              </Link>
            ))}
          </div>
        </section>
      ) : null}

      {showReferenceStatus || referenceItems.length > 0 ? (
        <section aria-labelledby="references">
          <h2
            id="references"
            className="scroll-mt-20 border-b border-border pb-2 text-2xl font-semibold tracking-tight"
          >
            References
          </h2>
          {showReferenceStatus ? (
            <p className="mt-3 flex items-center gap-2 text-sm text-muted-foreground">
              <BookOpen className="size-4" aria-hidden />
              当前版本的引用索引正在生成，完成后会自动显示可回溯的参考文献。
            </p>
          ) : (
            <>
              <ol className="mt-4 list-decimal space-y-4 pl-7 marker:font-medium marker:text-muted-foreground">
                {visibleReferenceItems.map((item) => (
                  <ReferenceItem key={item.citationId} item={item} />
                ))}
              </ol>
              {collapsedReferenceItems.length > 0 ? (
                <details className="group mt-5 rounded-xl border border-border bg-muted/20 px-4 py-3">
                  <summary className="flex cursor-pointer list-none items-center gap-2 text-sm font-medium text-muted-foreground transition hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                    <ChevronDown
                      className="size-4 transition-transform group-open:rotate-180"
                      aria-hidden
                    />
                    <span className="group-open:hidden">
                      展开后续 {collapsedReferenceItems.length} 条参考文献
                    </span>
                    <span className="hidden group-open:inline">
                      收起后续 {collapsedReferenceItems.length} 条参考文献
                    </span>
                  </summary>
                  <ol
                    start={DEFAULT_VISIBLE_REFERENCES + 1}
                    className="mt-4 list-decimal space-y-4 pl-7 marker:font-medium marker:text-muted-foreground"
                  >
                    {collapsedReferenceItems.map((item) => (
                      <ReferenceItem key={item.citationId} item={item} />
                    ))}
                  </ol>
                </details>
              ) : null}
            </>
          )}
        </section>
      ) : null}
    </div>
  );
}
