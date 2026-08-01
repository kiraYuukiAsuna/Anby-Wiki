"use client";

import Link from "next/link";
import {
  ArrowLeft,
  ArrowRight,
  BadgeCheck,
  ChevronDown,
  ClipboardCheck,
  ExternalLink,
  FileClock,
  FileSearch,
  LoaderCircle,
  Quote,
  RefreshCw,
  ShieldAlert,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";

import type {
  EvidenceSourceChunk,
  EvidenceSourceChunkListPage,
  EvidenceSourceDetail,
  EvidenceSourceVersion,
  EvidenceSourceVersionListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { SourceUsagePanel } from "@/components/projection/usage-panels";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { sourcesApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { safeHttpUrl } from "@/lib/http-url";
import { cn } from "@/lib/utils";

const VERSION_PAGE_SIZE = 20;
const CHUNK_PAGE_SIZE = 20;
const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

function CitationComposer({
  version,
  chunk,
  onClose,
}: {
  version: EvidenceSourceVersion;
  chunk?: EvidenceSourceChunk;
  onClose: () => void;
}) {
  const [quotation, setQuotation] = useState("");
  const [page, setPage] = useState("");
  const [section, setSection] = useState("");
  const [charStart, setCharStart] = useState("");
  const [charEnd, setCharEnd] = useState("");
  const [saving, setSaving] = useState(false);
  const [createdID, setCreatedID] = useState<string>();

  const submit = async () => {
    const parsedPage = page ? Number(page) : undefined;
    const parsedStart = charStart ? Number(charStart) : undefined;
    const parsedEnd = charEnd ? Number(charEnd) : undefined;
    if (
      (parsedPage !== undefined &&
        (!Number.isInteger(parsedPage) || parsedPage < 1)) ||
      (parsedStart !== undefined &&
        (!Number.isInteger(parsedStart) || parsedStart < 0)) ||
      (parsedEnd !== undefined &&
        (!Number.isInteger(parsedEnd) || parsedEnd < 0)) ||
      (parsedStart !== undefined &&
        parsedEnd !== undefined &&
        parsedEnd < parsedStart)
    ) {
      toast.error("请检查页码和字符范围");
      return;
    }
    setSaving(true);
    try {
      const citation = await sourcesApi().createCitation({
        createCitationRequest: {
          sourceVersionId: version.id,
          sourceChunkId: chunk?.id,
          quotation: quotation || undefined,
          locator:
            parsedPage !== undefined ||
            section.trim() ||
            parsedStart !== undefined ||
            parsedEnd !== undefined
              ? {
                  page: parsedPage,
                  section: section.trim() || undefined,
                  charStart: parsedStart,
                  charEnd: parsedEnd,
                }
              : undefined,
        },
      });
      setCreatedID(citation.id);
      toast.success("Citation 已创建", {
        description: "不可变证据定位已保存。",
      });
    } catch {
      toast.error("Citation 创建失败", {
        description: chunk
          ? "引文必须是所选 SourceChunk 文本的真实子串。"
          : "请检查来源版本与定位信息。",
      });
    } finally {
      setSaving(false);
    }
  };

  if (createdID) {
    return (
      <>
        <DialogHeader>
          <DialogTitle>证据定位已创建</DialogTitle>
          <DialogDescription>
            Citation 是不可变稳定身份，可绑定 Claim 或插入页面正文。
          </DialogDescription>
        </DialogHeader>
        <div className="rounded-2xl border border-emerald-200 bg-emerald-50 p-5">
          <BadgeCheck className="size-6 text-emerald-700" aria-hidden />
          <p className="mt-3 font-mono text-xs text-emerald-950">{createdID}</p>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            完成
          </Button>
          <Button asChild>
            <Link href={`/citations/${createdID}`}>
              打开 Citation
              <ArrowRight aria-hidden />
            </Link>
          </Button>
        </DialogFooter>
      </>
    );
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>建立 Citation</DialogTitle>
        <DialogDescription>
          {chunk
            ? `定位到 SourceChunk #${chunk.ordinal}；引文会由服务端核对。`
            : "定位到整个 SourceVersion，可选补充页码或章节。"}
        </DialogDescription>
      </DialogHeader>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2 sm:col-span-2">
          <Label htmlFor="citation-quotation">精确引文（可选）</Label>
          <Textarea
            id="citation-quotation"
            value={quotation}
            onChange={(event) => setQuotation(event.target.value)}
            className="min-h-24"
            placeholder={
              chunk ? "从上方分片复制一段真实文本" : "未绑定分片时可留空"
            }
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="citation-page">页码</Label>
          <Input
            id="citation-page"
            type="number"
            min={1}
            value={page}
            onChange={(event) => setPage(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="citation-section">章节</Label>
          <Input
            id="citation-section"
            value={section}
            onChange={(event) => setSection(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="citation-start">字符起点</Label>
          <Input
            id="citation-start"
            type="number"
            min={0}
            value={charStart}
            onChange={(event) => setCharStart(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="citation-end">字符终点</Label>
          <Input
            id="citation-end"
            type="number"
            min={0}
            value={charEnd}
            onChange={(event) => setCharEnd(event.target.value)}
          />
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button type="button" disabled={saving} onClick={() => void submit()}>
          {saving ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <Quote aria-hidden />
          )}
          创建 Citation
        </Button>
      </DialogFooter>
    </>
  );
}

function VersionPanel({ version }: { version: EvidenceSourceVersion }) {
  const [expanded, setExpanded] = useState(false);
  const [citationChunk, setCitationChunk] = useState<
    EvidenceSourceChunk | "version"
  >();
  const { isAuthenticated } = useSession();
  const chunks = useSWRInfinite<EvidenceSourceChunkListPage>(
    (pageIndex, previousPage) => {
      if (!expanded) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "source-chunks",
        version.id,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, versionID, cursor] = cacheKey as readonly [
        string,
        string,
        string,
      ];
      return sourcesApi().listSourceChunks({
        id: versionID,
        cursor: cursor || undefined,
        pageSize: CHUNK_PAGE_SIZE,
      });
    },
  );
  const items = chunks.data?.flatMap((page) => page.items) ?? [];
  const lastPage = chunks.data?.[chunks.data.length - 1];

  return (
    <li className="overflow-hidden rounded-2xl border bg-card">
      <div className="flex flex-wrap items-center gap-4 p-4">
        <span className="flex size-10 items-center justify-center rounded-xl bg-sky-100 text-sky-700">
          <FileClock className="size-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <p className="font-medium">
            获取于 {DATE_FORMATTER.format(version.fetchedAt)}
          </p>
          <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
            {version.versionHash}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!isAuthenticated}
          onClick={() => setCitationChunk("version")}
        >
          <Quote aria-hidden />
          引用版本
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setExpanded((value) => !value)}
          aria-expanded={expanded}
        >
          分片
          <ChevronDown
            className={cn(
              "transition-transform",
              expanded && "rotate-180",
            )}
            aria-hidden
          />
        </Button>
      </div>
      {expanded ? (
        <div className="border-t bg-muted/15 p-4">
          {chunks.isLoading && !chunks.data ? (
            <div className="h-28 animate-pulse rounded-xl bg-muted/55" />
          ) : null}
          {chunks.error ? (
            <p className="rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive">
              来源分片载入失败。
            </p>
          ) : null}
          {!chunks.isLoading && !chunks.error && items.length === 0 ? (
            <p className="rounded-xl border border-dashed p-5 text-center text-sm text-muted-foreground">
              此版本没有可定位分片。
            </p>
          ) : null}
          {items.length > 0 ? (
            <ol className="space-y-3">
              {items.map((chunk) => (
                <li key={chunk.id} className="rounded-xl border bg-background p-4">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <p className="text-xs font-semibold">
                        SourceChunk #{chunk.ordinal}
                      </p>
                      <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                        {chunk.id} · {chunk.textHash.slice(0, 12)}
                      </p>
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={!isAuthenticated}
                      onClick={() => setCitationChunk(chunk)}
                    >
                      <Quote aria-hidden />
                      建立引用
                    </Button>
                  </div>
                  <details className="mt-3">
                    <summary className="cursor-pointer text-xs font-medium text-primary">
                      查看来源文本与定位
                    </summary>
                    <pre className="mt-3 max-h-72 overflow-auto whitespace-pre-wrap rounded-lg bg-muted/45 p-3 text-xs leading-6">
                      {chunk.textContent}
                    </pre>
                    <p className="mt-2 font-mono text-[10px] text-muted-foreground">
                      {JSON.stringify(chunk.locator)}
                    </p>
                  </details>
                </li>
              ))}
            </ol>
          ) : null}
          {lastPage?.nextCursor ? (
            <Button
              type="button"
              variant="outline"
              className="mt-3 w-full"
              disabled={chunks.isValidating}
              onClick={() => void chunks.setSize(chunks.size + 1)}
            >
              {chunks.isValidating ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : null}
              加载更多分片
            </Button>
          ) : null}
        </div>
      ) : null}
      <Dialog
        open={Boolean(citationChunk)}
        onOpenChange={(open) => {
          if (!open) setCitationChunk(undefined);
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          {citationChunk ? (
            <CitationComposer
              version={version}
              chunk={
                citationChunk === "version" ? undefined : citationChunk
              }
              onClose={() => setCitationChunk(undefined)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </li>
  );
}

export function SourceWorkspace({ id }: { id: string }) {
  const detail = useSWR<EvidenceSourceDetail>(["source", id], () =>
    sourcesApi().getSource({ id }),
  );
  const versions = useSWRInfinite<EvidenceSourceVersionListPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "source-versions",
        id,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, sourceID, cursor] = cacheKey as readonly [
        string,
        string,
        string,
      ];
      return sourcesApi().listSourceVersions({
        id: sourceID,
        cursor: cursor || undefined,
        pageSize: VERSION_PAGE_SIZE,
      });
    },
  );

  if (detail.isLoading) {
    return <div className="h-72 animate-pulse rounded-3xl border bg-muted/35" />;
  }
  if (detail.error || !detail.data) {
    return (
      <div className="rounded-3xl border border-destructive/20 bg-destructive/5 p-8">
        <h1 className="text-xl font-semibold">来源无法打开</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          它可能不存在，或 API 暂时不可用。
        </p>
        <Button asChild variant="outline" className="mt-5">
          <Link href="/sources">返回来源目录</Link>
        </Button>
      </div>
    );
  }

  const source = detail.data;
  const resource = source.externalResource;
  const resourceURL = safeHttpUrl(
    resource?.canonicalUrl ?? resource?.normalizedUrl,
  );
  const versionItems = versions.data?.flatMap((page) => page.items) ?? [];
  const lastVersionPage = versions.data?.[versions.data.length - 1];
  const healthy = resource?.status === "ok" || resource?.status === "redirect";

  return (
    <>
      <header className="border-b border-border/75 pb-7">
        <Button variant="ghost" size="sm" asChild className="-ml-2 mb-5">
          <Link href="/sources">
            <ArrowLeft aria-hidden />
            来源目录
          </Link>
        </Button>
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div className="max-w-3xl">
            <p className="text-xs font-semibold tracking-[0.16em] text-emerald-700 uppercase">
              {source.sourceType} source
            </p>
            <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
              {source.title}
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">
              {[source.author, source.publisher].filter(Boolean).join(" · ") ||
                "未登记作者或发布者"}
              {source.publishedAt
                ? ` · ${DATE_FORMATTER.format(source.publishedAt)}`
                : ""}
            </p>
          </div>
          {resourceURL ? (
            <Button asChild variant="outline">
              <a href={resourceURL} target="_blank" rel="noopener noreferrer">
                打开原始资源
                <ExternalLink aria-hidden />
              </a>
            </Button>
          ) : null}
        </div>
      </header>

      <section className="mt-7 grid gap-4 md:grid-cols-3" aria-label="来源状态">
        <div className="rounded-2xl border bg-card p-5">
          {healthy ? (
            <ClipboardCheck className="size-5 text-emerald-600" aria-hidden />
          ) : (
            <ShieldAlert className="size-5 text-amber-600" aria-hidden />
          )}
          <p className="mt-4 text-sm font-semibold">
            {resource ? resource.status : "无外部资源"}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            {resource?.httpStatus
              ? `HTTP ${resource.httpStatus}`
              : "外链健康状态"}
          </p>
        </div>
        <div className="rounded-2xl border bg-card p-5">
          <FileClock className="size-5 text-sky-600" aria-hidden />
          <p className="mt-4 text-2xl font-semibold">{versionItems.length}</p>
          <p className="mt-1 text-xs text-muted-foreground">当前已加载版本</p>
        </div>
        <div className="rounded-2xl border bg-card p-5">
          <FileSearch className="size-5 text-violet-600" aria-hidden />
          <p className="mt-4 truncate font-mono text-xs">{source.id}</p>
          <p className="mt-2 text-xs text-muted-foreground">稳定 Source ID</p>
        </div>
      </section>

      {resource ? (
        <section className="mt-5 rounded-2xl border bg-muted/20 p-5">
          <h2 className="text-sm font-semibold">规范化外部资源</h2>
          <dl className="mt-4 grid gap-3 text-xs sm:grid-cols-[9rem_1fr]">
            <dt className="text-muted-foreground">Domain</dt>
            <dd className="font-mono">{resource.domain}</dd>
            <dt className="text-muted-foreground">Canonical URL</dt>
            <dd className="break-all font-mono">
              {resource.canonicalUrl ?? resource.normalizedUrl}
            </dd>
            <dt className="text-muted-foreground">Last checked</dt>
            <dd>
              {resource.lastCheckedAt
                ? DATE_FORMATTER.format(resource.lastCheckedAt)
                : "尚未检查"}
            </dd>
            <dt className="text-muted-foreground">连续失败</dt>
            <dd>{resource.consecutiveFailures}</dd>
          </dl>
        </section>
      ) : null}

      {Object.keys(source.metadata).length > 0 ? (
        <details className="mt-5 rounded-2xl border bg-muted/20">
          <summary className="cursor-pointer px-5 py-4 text-sm font-medium">
            查看来源 Metadata
          </summary>
          <pre className="overflow-x-auto border-t p-5 text-xs">
            {JSON.stringify(source.metadata, null, 2)}
          </pre>
        </details>
      ) : null}

      <section className="mt-9" aria-labelledby="source-versions-title">
        <div className="mb-4 flex items-end justify-between gap-3">
          <div>
            <h2 id="source-versions-title" className="text-xl font-semibold">
              不可变来源版本
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              每个版本按内容哈希去重；展开后可浏览定位分片并建立 Citation。
            </p>
          </div>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={versions.isValidating}
            onClick={() => void versions.mutate()}
          >
            <RefreshCw
              className={cn(
                "size-3.5",
                versions.isValidating && "animate-spin",
              )}
              aria-hidden
            />
            刷新
          </Button>
        </div>
        {versions.isLoading && !versions.data ? (
          <div className="h-40 animate-pulse rounded-2xl border bg-muted/35" />
        ) : null}
        {versions.error ? (
          <p className="rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            来源版本暂时无法读取。
          </p>
        ) : null}
        {!versions.isLoading && !versions.error && versionItems.length === 0 ? (
          <div className="rounded-2xl border border-dashed px-6 py-12 text-center">
            <FileClock className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">尚无来源版本</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              通过导入管道获取内容后，这里会出现不可变版本与可定位分片。
            </p>
            <Button asChild variant="outline" className="mt-5">
              <Link href="/imports">
                前往导入中心
                <ArrowRight aria-hidden />
              </Link>
            </Button>
          </div>
        ) : null}
        {versionItems.length > 0 ? (
          <ol className="space-y-3">
            {versionItems.map((version) => (
              <VersionPanel key={version.id} version={version} />
            ))}
          </ol>
        ) : null}
        {lastVersionPage?.nextCursor ? (
          <Button
            type="button"
            variant="outline"
            className="mt-4 w-full"
            disabled={versions.isValidating}
            onClick={() => void versions.setSize(versions.size + 1)}
          >
            {versions.isValidating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : null}
            加载更多版本
          </Button>
        ) : null}
      </section>
      <SourceUsagePanel sourceId={source.id} />
    </>
  );
}
