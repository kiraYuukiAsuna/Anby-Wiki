"use client";

import { BookOpen, LoaderCircle, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import useSWR from "swr";

import type {
  PageSectionSummary,
  RenderedPageSection,
} from "../../../contracts/generated/typescript";

import { AstDocument } from "@/components/ast";
import { ServerRenderedDocument } from "@/components/server-rendered-document";
import { Button } from "@/components/ui/button";
import { projectionApi, readingApi } from "@/lib/api";
import { parseDocument } from "@/lib/ast/schema";

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function FullDocumentFallback({ pageId }: { pageId: string }) {
  const full = useSWR(["page:full-content-fallback", pageId], () =>
    readingApi().getPageByID({ id: pageId, contentMode: "full" }),
  );
  if (full.isLoading) {
    return (
      <div className="space-y-3 py-4" aria-label="正在加载全文">
        <div className="h-5 w-2/3 animate-pulse rounded bg-muted" />
        <div className="h-4 w-full animate-pulse rounded bg-muted/80" />
        <div className="h-4 w-5/6 animate-pulse rounded bg-muted/80" />
      </div>
    );
  }
  const content = full.data?.content;
  const astJSON = content?.astJson;
  if (full.error || !content || (!content.html && !astJSON)) {
    return (
      <div className="rounded-xl border border-destructive/20 bg-destructive/5 p-5 text-sm">
        <p className="font-medium text-destructive">章节投影暂不可用</p>
        <p className="mt-1 text-muted-foreground">全文回退也未能完成，请稍后刷新。</p>
      </div>
    );
  }
  if (content.html) {
    return (
      <ServerRenderedDocument
        html={content.html}
        rendererVersion={content.rendererVersion}
      />
    );
  }
  return <AstDocument document={parseDocument(astJSON!)} />;
}

function LazySection({
  pageId,
  revisionId,
  summary,
  citationOrder,
  force,
  focusBlockId,
}: {
  pageId: string;
  revisionId: string;
  summary: PageSectionSummary;
  citationOrder: readonly string[];
  force: boolean;
  focusBlockId?: string;
}) {
  const containerRef = useRef<HTMLElement>(null);
  const focusedRef = useRef(false);
  const [nearViewport, setNearViewport] = useState(summary.position < 2);
  const shouldLoad = nearViewport || force;
  const section = useSWR<RenderedPageSection>(
    shouldLoad ? ["page:section", pageId, revisionId, summary.key] : null,
    () =>
      projectionApi().getPageSection({
        id: pageId,
        sectionKey: summary.key,
      }),
    { revalidateOnFocus: false },
  );

  useEffect(() => {
    const element = containerRef.current;
    if (!element || shouldLoad) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          setNearViewport(true);
          observer.disconnect();
        }
      },
      { rootMargin: "900px 0px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [shouldLoad]);

  useEffect(() => {
    if (!focusBlockId || !section.data || focusedRef.current) return;
    const frame = window.requestAnimationFrame(() => {
      const target = document.getElementById(focusBlockId);
      if (!target) return;
      focusedRef.current = true;
      window.history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}#${focusBlockId}`,
      );
      target.scrollIntoView({ block: "start" });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [focusBlockId, section.data]);

  const estimatedHeight = Math.min(
    560,
    Math.max(150, Math.round(summary.sizeBytes / 150)),
  );

  return (
    <section
      ref={containerRef}
      data-section-key={summary.key}
      className="scroll-mt-20"
      style={!section.data ? { minHeight: `${estimatedHeight}px` } : undefined}
    >
      {section.error ? (
        <div className="my-4 flex items-center justify-between gap-4 rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm">
          <span>「{summary.title}」加载失败。</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void section.mutate()}
          >
            <RefreshCw aria-hidden />
            重试
          </Button>
        </div>
      ) : section.data ? (
        section.data.revisionId === revisionId ? (
          section.data.html ? (
            <ServerRenderedDocument
              html={section.data.html}
              rendererVersion={section.data.rendererVersion}
            />
          ) : (
            <AstDocument
              document={parseDocument(section.data.astJson)}
              citationOrder={citationOrder}
            />
          )
        ) : (
          <div className="my-4 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
            页面已发布更新版本，请刷新后继续阅读。
          </div>
        )
      ) : (
        <div className="flex h-full min-h-36 items-center justify-center rounded-xl border border-dashed border-border/70 bg-muted/15 text-xs text-muted-foreground">
          {shouldLoad ? (
            <span className="flex items-center gap-2">
              <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
              加载「{summary.title}」…
            </span>
          ) : (
            <span>继续滚动以加载「{summary.title}」</span>
          )}
        </div>
      )}
    </section>
  );
}

export function LazySectionDocument({
  pageId,
  revisionId,
  expectedSectionCount,
  initialBlockId,
}: {
  pageId: string;
  revisionId: string;
  expectedSectionCount: number;
  initialBlockId?: string;
}) {
  const router = useRouter();
  const manifest = useSWR(["page:section-manifest", pageId, revisionId], () =>
    projectionApi().getPageSections({ id: pageId }),
  );
  const [targetSectionKey, setTargetSectionKey] = useState<string>();
  const [focusBlockId, setFocusBlockId] = useState(initialBlockId);

  useEffect(() => {
    if (!manifest.data?.ready) return;
    let cancelled = false;
    const locate = async () => {
      let blockId = initialBlockId;
      if (!blockId) {
        const rawHash = window.location.hash.slice(1);
        if (!rawHash) return;
        let hash: string;
        try {
          hash = decodeURIComponent(rawHash);
        } catch {
          return;
        }
        if (UUID_PATTERN.test(hash)) {
          blockId = hash;
        } else {
          try {
            const anchor = await projectionApi().resolvePageAnchor({
              id: pageId,
              slug: hash,
            });
            if (anchor.pageId !== pageId) {
              window.location.assign(
                `/pages/${anchor.pageId}#${encodeURIComponent(anchor.blockId)}`,
              );
              return;
            }
            blockId = anchor.blockId;
          } catch {
            return;
          }
        }
      }
      if (!blockId) return;
      try {
        const located = await projectionApi().locatePageSection({
          id: pageId,
          blockId,
        });
        if (cancelled) return;
        setFocusBlockId(blockId);
        setTargetSectionKey(located.sectionKey);
      } catch {
        // A stale or unknown hash keeps normal browser no-op semantics.
      }
    };
    void locate();
    return () => {
      cancelled = true;
    };
  }, [initialBlockId, manifest.data?.ready, pageId]);

  useEffect(() => {
    if (
      manifest.data?.ready &&
      manifest.data.revisionId &&
      manifest.data.revisionId !== revisionId
    ) {
      router.refresh();
    }
  }, [manifest.data, revisionId, router]);

  if (manifest.isLoading) {
    return (
      <div className="flex items-center justify-center gap-2 rounded-xl border border-dashed border-border py-12 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        准备章节阅读…
      </div>
    );
  }
  if (
    manifest.error ||
    !manifest.data?.ready ||
    manifest.data.revisionId !== revisionId
  ) {
    return <FullDocumentFallback pageId={pageId} />;
  }

  const items = manifest.data.items;
  const citationOrder = manifest.data.citationOrder;
  return (
    <div>
      <div className="mb-6 rounded-xl border border-sky-200 bg-sky-50/60 p-4">
        <p className="flex items-center gap-2 text-xs font-semibold text-sky-900">
          <BookOpen className="size-4" aria-hidden />
          按章节渐进加载
        </p>
        <p className="mt-1 text-xs leading-5 text-sky-800/75">
          当前 Revision 共 {expectedSectionCount} 个分片；首屏已预取，后续内容会在接近视口时加载。
        </p>
        <nav className="mt-3 flex flex-wrap gap-1.5" aria-label="章节分片">
          {items
            .filter((item) => item.headingBlockId)
            .map((item) => (
              <button
                key={item.key}
                type="button"
                onClick={() => {
                  setTargetSectionKey(item.key);
                  setFocusBlockId(item.headingBlockId);
                }}
                className="rounded-full border border-sky-200 bg-background/80 px-2.5 py-1 text-[11px] text-sky-900 hover:bg-sky-100"
              >
                {item.title}
              </button>
            ))}
        </nav>
      </div>
      <div>
        {items.map((item) => (
          <LazySection
            key={item.key}
            pageId={pageId}
            revisionId={revisionId}
            summary={item}
            citationOrder={citationOrder}
            force={targetSectionKey === item.key}
            focusBlockId={
              targetSectionKey === item.key ? focusBlockId : undefined
            }
          />
        ))}
      </div>
    </div>
  );
}
