"use client";

import {
  ArrowUpRight,
  Check,
  CheckCircle2,
  Copy,
  FileArchive,
  FileText,
  ImageIcon,
  Inbox,
  LoaderCircle,
  Music2,
  RefreshCw,
  Upload,
  Video,
  X,
} from "lucide-react";
import Link from "next/link";
import { useRef, useState } from "react";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";

import type {
  Asset,
  AssetListPage,
  ListAssetsKindEnum,
} from "../../../../contracts/generated/typescript";

import { AssetImage } from "@/components/ast/asset-media";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { assetsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { compactId } from "@/lib/display-id";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 24;
type Filter = "all" | ListAssetsKindEnum;

const FILTERS: Array<{ value: Filter; label: string }> = [
  { value: "all", label: "全部" },
  { value: "image", label: "图片" },
  { value: "video", label: "视频" },
  { value: "audio", label: "音频" },
  { value: "pdf", label: "PDF" },
  { value: "other", label: "其他" },
];

const MAX_ASSET_UPLOAD_BYTES = 50 * 1024 * 1024;

function assetFileError(file: File): string {
  if (file.size <= 0) return "文件不能为空";
  if (file.size > MAX_ASSET_UPLOAD_BYTES) return "文件不能超过 50 MiB";
  return "";
}

function fileKindLabel(file: File): string {
  if (file.type.startsWith("image/")) return "图片";
  if (file.type.startsWith("video/")) return "视频";
  if (file.type.startsWith("audio/")) return "音频";
  if (file.type === "application/pdf" || /\.pdf$/i.test(file.name)) return "PDF";
  return file.name.split(".").pop()?.toUpperCase() || "文件";
}

function FileKindIcon({ file }: { file: File }) {
  const Icon = file.type.startsWith("image/")
    ? ImageIcon
    : file.type.startsWith("video/")
      ? Video
      : file.type.startsWith("audio/")
        ? Music2
        : file.type === "application/pdf" || /\.pdf$/i.test(file.name)
          ? FileText
          : FileArchive;
  return <Icon className="size-5" aria-hidden />;
}

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
});

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
  return `${(value / 1024 / 1024).toFixed(1)} MiB`;
}

function AssetCard({ asset }: { asset: Asset }) {
  const [copied, setCopied] = useState(false);
  const revision = asset.currentRevision;
  const image = revision.mimeType.startsWith("image/");
  const video = revision.mimeType.startsWith("video/");
  const audio = revision.mimeType.startsWith("audio/");
  const pdf = revision.mimeType === "application/pdf";
  const KindIcon = image
    ? ImageIcon
    : video
      ? Video
      : audio
        ? Music2
        : pdf
          ? FileText
          : FileArchive;

  const copyRevision = async () => {
    try {
      await navigator.clipboard.writeText(revision.id);
      setCopied(true);
      toast.success("已复制 AssetRevision ID");
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      toast.error("复制失败");
    }
  };

  return (
    <li className="overflow-hidden rounded-2xl border border-border/80 bg-card shadow-[0_1px_0_rgb(15_23_42/0.03)]">
      <div className="aspect-4/3 overflow-hidden bg-muted/45">
        {image ? (
          <AssetImage revisionId={revision.id} alt={asset.name} />
        ) : (
          <div className="flex h-full items-center justify-center">
            <span className="flex size-14 items-center justify-center rounded-2xl bg-background text-muted-foreground shadow-sm">
              <KindIcon className="size-6" aria-hidden />
            </span>
          </div>
        )}
      </div>
      <div className="p-4">
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold" title={asset.name}>
              {asset.name}
            </p>
            <p className="mt-1 truncate text-[11px] text-muted-foreground">
              {revision.mimeType} · {formatBytes(revision.sizeBytes)}
            </p>
          </div>
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            onClick={() => void copyRevision()}
            aria-label={`复制 ${asset.name} 的 AssetRevision ID`}
          >
            {copied ? (
              <Check className="text-emerald-600" aria-hidden />
            ) : (
              <Copy aria-hidden />
            )}
          </Button>
        </div>
        <div className="mt-3 flex items-center justify-between text-[11px] text-muted-foreground">
          <time dateTime={asset.updatedAt.toISOString()}>
            {DATE_FORMATTER.format(asset.updatedAt)}
          </time>
          <span className="font-mono" title={revision.id}>{compactId(revision.id)}</span>
        </div>
        <Button asChild type="button" size="sm" variant="outline" className="mt-4 w-full">
          <Link href={`/assets/revisions/${revision.id}`}>
            查看资产信息
            <ArrowUpRight aria-hidden />
          </Link>
        </Button>
      </div>
    </li>
  );
}

export function AssetLibrary() {
  const [filter, setFilter] = useState<Filter>("all");
  const [name, setName] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState("");
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const { data, error, isLoading, isValidating, mutate, size, setSize } =
    useSWRInfinite<AssetListPage>(
      (pageIndex, previousPage) => {
        if (pageIndex > 0 && !previousPage?.nextCursor) return null;
        return [
          "assets",
          filter,
          pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
        ] as const;
      },
      ([, selectedFilter, cursor]) =>
        assetsApi().listAssets({
          cursor: typeof cursor === "string" && cursor ? cursor : undefined,
          pageSize: PAGE_SIZE,
          kind:
            selectedFilter === "all"
              ? undefined
              : (selectedFilter as ListAssetsKindEnum),
        }),
      { revalidateFirstPage: true },
    );

  const assets = data?.flatMap((page) => page.items) ?? [];
  const pageCount = data?.length ?? 0;
  const lastPage = pageCount > 0 ? data?.[pageCount - 1] : undefined;
  const reachedEnd = Boolean(data && !lastPage?.nextCursor);
  const loadingMore = isValidating && pageCount > 0 && size > pageCount;
  const uploadDisabled = !isAuthenticated || uploading || sessionLoading;

  const selectFile = (selected: File) => {
    const error = assetFileError(selected);
    setFile(error ? null : selected);
    setFileError(error);
  };

  const upload = async () => {
    if (!file) {
      toast.error("请先选择文件");
      return;
    }
    if (!isAuthenticated) {
      toast.error("登录后才能上传资产");
      return;
    }
    setUploading(true);
    try {
      const asset = await assetsApi().uploadAsset({
        file,
        name: name.trim() || file.name,
      });
      await mutate();
      if (inputRef.current) inputRef.current.value = "";
      setFile(null);
      setFileError("");
      setName("");
      toast.success("资产已保存", {
        description: `不可变版本 ${compactId(asset.currentRevision.id)}`,
      });
    } catch {
      toast.error("资产上传失败", {
        description: "请确认对象存储已配置、文件不超过 50 MiB，并检查账户权限。",
      });
    } finally {
      setUploading(false);
    }
  };

  return (
    <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_20rem]">
      <section aria-labelledby="asset-list-title">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 id="asset-list-title" className="text-xl font-semibold">
            资产目录
          </h2>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={isValidating}
            onClick={() => void mutate()}
          >
            <RefreshCw
              className={cn("size-3.5", isValidating && "animate-spin")}
              aria-hidden
            />
            刷新
          </Button>
        </div>

        <div className="mt-5 flex gap-1 rounded-xl bg-muted/70 p-1">
          {FILTERS.map((item) => (
            <button
              key={item.value}
              type="button"
              aria-pressed={filter === item.value}
              onClick={() => setFilter(item.value)}
              className={cn(
                "rounded-lg px-3 py-1.5 text-xs font-medium text-muted-foreground transition",
                filter === item.value &&
                  "bg-background text-foreground shadow-sm ring-1 ring-border/70",
              )}
            >
              {item.label}
            </button>
          ))}
        </div>

        {error ? (
          <div className="mt-4 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm">
            <p className="font-medium text-destructive">资产目录暂时不可用</p>
            <p className="mt-1 text-muted-foreground">
              请确认 API 与对象存储已配置。
            </p>
          </div>
        ) : null}

        {isLoading && !data ? (
          <div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {[0, 1, 2, 3, 4, 5].map((item) => (
              <div
                key={item}
                className="aspect-4/3 animate-pulse rounded-2xl border bg-muted/45"
              />
            ))}
          </div>
        ) : null}

        {!isLoading && !error && assets.length === 0 ? (
          <div className="mt-4 rounded-2xl border border-dashed px-6 py-14 text-center">
            <Inbox className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">还没有符合条件的资产</h3>
          </div>
        ) : null}

        {assets.length > 0 ? (
          <ul className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {assets.map((asset) => (
              <AssetCard key={asset.id} asset={asset} />
            ))}
          </ul>
        ) : null}

        {data && !reachedEnd ? (
          <Button
            type="button"
            variant="outline"
            className="mt-4 w-full"
            disabled={loadingMore}
            onClick={() => void setSize(size + 1)}
          >
            {loadingMore ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : null}
            加载更多
          </Button>
        ) : null}
      </section>

      <aside className="xl:sticky xl:top-24 xl:self-start">
        <div className="rounded-2xl border bg-card p-5">
          <span className="flex size-10 items-center justify-center rounded-xl bg-primary/9 text-primary">
            <Upload className="size-4.5" aria-hidden />
          </span>
          <h2 className="mt-4 font-semibold">上传新资产</h2>
          <div className="mt-5 space-y-4">
            <div className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <Label htmlFor="asset-file" id="asset-file-label">
                  文件
                </Label>
                <span className="text-xs text-muted-foreground">最大 50 MiB</span>
              </div>
              <input
                ref={inputRef}
                id="asset-file"
                type="file"
                className="sr-only"
                tabIndex={-1}
                aria-invalid={Boolean(fileError)}
                aria-describedby={
                  fileError ? "asset-file-error asset-file-help" : "asset-file-help"
                }
                disabled={uploadDisabled}
                onChange={(event) => {
                  const selected = event.target.files?.[0];
                  if (selected) selectFile(selected);
                  event.target.value = "";
                }}
              />
              <div
                aria-labelledby="asset-file-label"
                aria-describedby={
                  fileError ? "asset-file-error asset-file-help" : "asset-file-help"
                }
                className={cn(
                  "rounded-xl border border-dashed transition-colors",
                  dragging ? "border-primary bg-primary/5" : "border-border bg-muted/20",
                  fileError && "border-destructive/50",
                )}
                onDragOver={(event) => {
                  event.preventDefault();
                  if (!uploadDisabled && event.dataTransfer.types.includes("Files")) {
                    setDragging(true);
                  }
                }}
                onDragLeave={(event) => {
                  if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
                    setDragging(false);
                  }
                }}
                onDrop={(event) => {
                  event.preventDefault();
                  setDragging(false);
                  if (uploadDisabled) return;
                  const dropped = event.dataTransfer.files;
                  if (dropped.length !== 1) {
                    setFile(null);
                    setFileError("每次请选择一个资产文件");
                    return;
                  }
                  selectFile(dropped[0]);
                }}
              >
                {file ? (
                  <>
                    <div className="flex items-center gap-3 p-4">
                      <span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                        <FileKindIcon file={file} />
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium" title={file.name}>
                          {file.name}
                        </p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {fileKindLabel(file)} · {formatBytes(file.size)}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center justify-between gap-2 border-t border-border/60 px-4 py-2">
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground" role="status">
                        <CheckCircle2 className="size-3.5 text-primary" aria-hidden />
                        {dragging ? "松开以更换文件" : "已选择"}
                      </span>
                      <div className="flex items-center gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          disabled={uploadDisabled}
                          onClick={() => inputRef.current?.click()}
                        >
                          更换文件
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          aria-label="移除资产文件"
                          disabled={uploadDisabled}
                          onClick={() => {
                            setFile(null);
                            setFileError("");
                          }}
                        >
                          <X aria-hidden />
                        </Button>
                      </div>
                    </div>
                  </>
                ) : (
                  <div className="flex flex-col items-center gap-3 px-4 py-6 text-center">
                    <span className="flex size-11 items-center justify-center rounded-xl border border-border bg-background text-muted-foreground">
                      <Upload className="size-5" aria-hidden />
                    </span>
                    <div className="space-y-1">
                      <p className="text-sm font-medium">
                        {dragging ? "松开即可选择文件" : "拖放文件到这里"}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        保存图片、PDF、音视频或其他附件到媒体库
                      </p>
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      disabled={uploadDisabled}
                      aria-describedby={fileError ? "asset-file-error" : "asset-file-help"}
                      onClick={() => inputRef.current?.click()}
                    >
                      选择文件
                    </Button>
                  </div>
                )}
              </div>
              {fileError ? (
                <p id="asset-file-error" role="alert" className="text-xs text-destructive">
                  {fileError}
                </p>
              ) : null}
              <div id="asset-file-help" className="flex flex-wrap gap-1.5 text-xs text-muted-foreground">
                {["图片", "PDF", "音视频", "其他附件"].map((format) => (
                  <span key={format} className="rounded-md bg-muted px-2 py-1">
                    {format}
                  </span>
                ))}
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="asset-name">站内名称（可选）</Label>
              <Input
                id="asset-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="默认使用文件名"
                disabled={uploadDisabled}
              />
            </div>
            <Button
              type="button"
              className="w-full"
              disabled={uploadDisabled || !file}
              onClick={() => void upload()}
            >
              {uploading ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Upload aria-hidden />
              )}
              {isAuthenticated ? "保存到媒体库" : "登录后上传"}
            </Button>
          </div>
        </div>
      </aside>
    </div>
  );
}
