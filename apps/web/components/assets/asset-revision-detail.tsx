"use client";

import {
  ArrowLeft,
  Check,
  Copy,
  Download,
  FileArchive,
  Fingerprint,
  ImageIcon,
  Video,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import type { AssetRevision } from "../../../../contracts/generated/typescript";

import {
  AssetImage,
  AssetVideo,
  useAssetObjectUrl,
} from "@/components/ast/asset-media";
import { Button } from "@/components/ui/button";

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  dateStyle: "long",
  timeStyle: "medium",
});

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
  if (value < 1024 * 1024 * 1024) {
    return `${(value / 1024 / 1024).toFixed(1)} MiB`;
  }
  return `${(value / 1024 / 1024 / 1024).toFixed(2)} GiB`;
}

export function AssetRevisionDetail({
  revision,
}: {
  revision: AssetRevision;
}) {
  const content = useAssetObjectUrl(revision.id);
  const [copied, setCopied] = useState<"id" | "hash">();
  const image = revision.mimeType.startsWith("image/");
  const video = revision.mimeType.startsWith("video/");

  const copy = async (kind: "id" | "hash", value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(kind);
      window.setTimeout(() => setCopied(undefined), 1600);
      toast.success(kind === "id" ? "已复制 AssetRevision ID" : "已复制内容摘要");
    } catch {
      toast.error("复制失败");
    }
  };

  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="flex flex-wrap items-end justify-between gap-6 border-b pb-8">
        <div>
          <span className="flex size-11 items-center justify-center rounded-2xl bg-sky-100 text-sky-700">
            {image ? (
              <ImageIcon className="size-5" aria-hidden />
            ) : video ? (
              <Video className="size-5" aria-hidden />
            ) : (
              <FileArchive className="size-5" aria-hidden />
            )}
          </span>
          <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-sky-700 uppercase">
            Immutable asset revision
          </p>
          <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
            不可变媒体版本
          </h1>
          <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground">
            这个稳定 URL 始终指向同一内容摘要。页面 Revision
            引用它后，不会被未来的同名上传悄悄替换。
          </p>
        </div>
        <Button asChild variant="outline">
          <Link href="/assets">
            <ArrowLeft aria-hidden />
            返回媒体库
          </Link>
        </Button>
      </header>

      <div className="mt-8 grid gap-6 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <section className="overflow-hidden rounded-2xl border bg-card shadow-sm">
          <div className="flex min-h-80 items-center justify-center bg-[radial-gradient(circle_at_top,_var(--color-sky-100),_transparent_55%)] p-4 sm:p-8">
            {image ? (
              <AssetImage revisionId={revision.id} alt="不可变资产版本预览" />
            ) : video ? (
              <AssetVideo
                revisionId={revision.id}
                title="不可变资产版本预览"
              />
            ) : (
              <div className="py-20 text-center">
                <span className="mx-auto flex size-20 items-center justify-center rounded-3xl bg-background text-muted-foreground shadow-sm">
                  <FileArchive className="size-9" aria-hidden />
                </span>
                <p className="mt-5 font-semibold">此附件没有内嵌预览</p>
                <p className="mt-2 text-sm text-muted-foreground">
                  {revision.mimeType} · {formatBytes(revision.sizeBytes)}
                </p>
              </div>
            )}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t px-5 py-4">
            <p className="text-xs text-muted-foreground">
              服务端以 ETag 与一年 immutable cache 提供原始字节。
            </p>
            <Button asChild size="sm" disabled={!content.url}>
              <a
                href={content.url}
                download={`asset-revision-${revision.id}`}
              >
                <Download aria-hidden />
                下载原文件
              </a>
            </Button>
          </div>
        </section>

        <aside className="space-y-4 xl:sticky xl:top-24 xl:self-start">
          <section className="rounded-2xl border bg-card p-5 shadow-sm">
            <p className="flex items-center gap-2 text-sm font-semibold">
              <Fingerprint className="size-4 text-sky-700" aria-hidden />
              版本身份
            </p>
            <dl className="mt-4 space-y-4 text-xs">
              <div>
                <dt className="text-muted-foreground">AssetRevision ID</dt>
                <dd className="mt-1 flex items-start gap-2">
                  <code className="min-w-0 flex-1 break-all rounded-lg bg-muted px-2.5 py-2 text-[10px]">
                    {revision.id}
                  </code>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label="复制 AssetRevision ID"
                    onClick={() => void copy("id", revision.id)}
                  >
                    {copied === "id" ? (
                      <Check className="text-emerald-600" aria-hidden />
                    ) : (
                      <Copy aria-hidden />
                    )}
                  </Button>
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Asset ID</dt>
                <dd className="mt-1 break-all font-mono text-[10px]">
                  {revision.assetId}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Content hash</dt>
                <dd className="mt-1 flex items-start gap-2">
                  <code className="min-w-0 flex-1 break-all text-[10px]">
                    {revision.contentHash}
                  </code>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label="复制内容摘要"
                    onClick={() => void copy("hash", revision.contentHash)}
                  >
                    {copied === "hash" ? (
                      <Check className="text-emerald-600" aria-hidden />
                    ) : (
                      <Copy aria-hidden />
                    )}
                  </Button>
                </dd>
              </div>
            </dl>
          </section>

          <section className="rounded-2xl border bg-card p-5 shadow-sm">
            <p className="text-sm font-semibold">文件元数据</p>
            <dl className="mt-4 grid grid-cols-[6rem_1fr] gap-x-3 gap-y-3 text-xs">
              <dt className="text-muted-foreground">MIME</dt>
              <dd className="break-all font-medium">{revision.mimeType}</dd>
              <dt className="text-muted-foreground">大小</dt>
              <dd className="font-medium">{formatBytes(revision.sizeBytes)}</dd>
              <dt className="text-muted-foreground">尺寸</dt>
              <dd className="font-medium">
                {revision.width && revision.height
                  ? `${revision.width} × ${revision.height}`
                  : "不适用"}
              </dd>
              <dt className="text-muted-foreground">创建时间</dt>
              <dd className="font-medium">
                {DATE_FORMATTER.format(revision.createdAt)}
              </dd>
            </dl>
          </section>
        </aside>
      </div>
    </div>
  );
}
