"use client";

import {
  ArrowUpRight,
  Check,
  Copy,
  FileArchive,
  ImageIcon,
  Inbox,
  LoaderCircle,
  RefreshCw,
  Upload,
  Video,
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
  { value: "other", label: "其他" },
];

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
  const KindIcon = image ? ImageIcon : video ? Video : FileArchive;

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
            查看不可变版本
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

  const upload = async () => {
    const file = inputRef.current?.files?.[0];
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
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="asset-list-title" className="text-xl font-semibold">
              资产目录
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              页面引用不可变版本，替换同名文件不会悄悄改变历史 Revision。
            </p>
          </div>
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
            <p className="mt-2 text-sm text-muted-foreground">
              登录后从右侧上传第一张图片、视频或附件。
            </p>
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
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            同名同内容自动去重；同名新内容会创建新版本。
          </p>
          <div className="mt-5 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="asset-file">文件</Label>
              <Input
                ref={inputRef}
                id="asset-file"
                type="file"
                disabled={!isAuthenticated || uploading || sessionLoading}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="asset-name">站内名称（可选）</Label>
              <Input
                id="asset-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="默认使用文件名"
                disabled={!isAuthenticated || uploading || sessionLoading}
              />
            </div>
            <Button
              type="button"
              className="w-full"
              disabled={!isAuthenticated || uploading || sessionLoading}
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
          <p className="mt-4 text-[11px] leading-5 text-muted-foreground">
            最大 50 MiB。资产内容按摘要寻址，公开页面可稳定读取历史版本。
          </p>
        </div>
      </aside>
    </div>
  );
}
