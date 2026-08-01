"use client";

import { ImageIcon, LoaderCircle, VideoIcon } from "lucide-react";
import { useEffect, useMemo } from "react";
import useSWR from "swr";

import { assetsApi } from "@/lib/api";

export function useAssetObjectUrl(revisionId?: string) {
  const { data, error, isLoading } = useSWR(
    revisionId ? ["asset-content", revisionId] : null,
    ([, id]) => assetsApi().getAssetRevisionContent({ revisionId: id }),
    { revalidateOnFocus: false },
  );
  const url = useMemo(
    () => (data ? URL.createObjectURL(data) : undefined),
    [data],
  );

  useEffect(() => {
    return () => {
      if (url) URL.revokeObjectURL(url);
    };
  }, [url]);

  return { url, error, isLoading };
}

function MediaFallback({
  kind,
  error,
}: {
  kind: "image" | "video";
  error: unknown;
}) {
  const Icon = kind === "image" ? ImageIcon : VideoIcon;
  return (
    <div className="flex min-h-44 items-center justify-center rounded-2xl border border-dashed bg-muted/35 text-center">
      <div>
        <Icon className="mx-auto size-7 text-muted-foreground" aria-hidden />
        <p className="mt-3 text-sm font-medium">
          {error ? "媒体暂时无法读取" : "正在读取媒体"}
        </p>
        {!error ? (
          <LoaderCircle
            className="mx-auto mt-2 size-4 animate-spin text-muted-foreground"
            aria-hidden
          />
        ) : null}
      </div>
    </div>
  );
}

export function AssetImage({
  revisionId,
  alt,
}: {
  revisionId: string;
  alt: string;
}) {
  const { url, error } = useAssetObjectUrl(revisionId);
  if (!url) return <MediaFallback kind="image" error={error} />;
  // Blob URL 来自生成客户端读取的同源、不可变 AssetRevision。
  // eslint-disable-next-line @next/next/no-img-element
  return <img src={url} alt={alt} className="h-auto max-h-[70vh] w-full object-contain" />;
}

export function AssetVideo({
  revisionId,
  posterRevisionId,
  title,
}: {
  revisionId: string;
  posterRevisionId?: string;
  title?: string;
}) {
  const media = useAssetObjectUrl(revisionId);
  const poster = useAssetObjectUrl(posterRevisionId);
  if (!media.url) return <MediaFallback kind="video" error={media.error} />;
  return (
    <video
      controls
      preload="metadata"
      src={media.url}
      poster={poster.url}
      title={title}
      className="max-h-[70vh] w-full bg-black"
    />
  );
}
