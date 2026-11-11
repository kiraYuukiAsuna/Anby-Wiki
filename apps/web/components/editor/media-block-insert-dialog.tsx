"use client";

import { useEffect, useRef, useState } from "react";

import { toast } from "sonner";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  audioBlockSchema,
  fileBlockSchema,
  imageBlockSchema,
  pdfBlockSchema,
  videoBlockSchema,
  type Block,
} from "@/lib/ast/schema";
import { assetsApi } from "@/lib/api";
import { ResourcePicker } from "@/components/editor/resource-picker";
import { newBlockId } from "@/lib/editor/ids";

type MediaType = "image" | "video" | "audio" | "file" | "pdf";
export type MediaBlock = Extract<Block, { type: MediaType }>;

export function MediaBlockInsertDialog({
  open,
  onOpenChange,
  onInsert,
  initialBlock,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialBlock?: MediaBlock;
  onInsert: (block: MediaBlock) => boolean | void;
}) {
  const [type, setType] = useState<MediaType>(initialBlock?.type ?? "image");
  const [revisionId, setRevisionId] = useState(initialBlock?.asset_revision_id ?? "");
  const [posterRevisionId, setPosterRevisionId] = useState(initialBlock?.type === "video" ? initialBlock.poster_asset_revision_id ?? "" : "");
  const [altText, setAltText] = useState(initialBlock?.type === "image" ? initialBlock.alt_text : "");
  const [title, setTitle] = useState(initialBlock?.type === "file" ? initialBlock.name : initialBlock && "title" in initialBlock ? initialBlock.title ?? "" : "");
  const [artist, setArtist] = useState(initialBlock?.type === "audio" ? initialBlock.artist ?? "" : "");
  const [album, setAlbum] = useState(initialBlock?.type === "audio" ? initialBlock.album ?? "" : "");
  const [duration, setDuration] = useState(initialBlock?.type === "audio" ? String(initialBlock.duration_seconds ?? "") : "");
  const [caption, setCaption] = useState(initialBlock?.caption ?? "");
  const [startPage, setStartPage] = useState(String(initialBlock?.type === "pdf" ? initialBlock.start_page ?? 1 : 1));
  const [height, setHeight] = useState(String(initialBlock?.type === "pdf" ? initialBlock.height : 640));
  const [submitting, setSubmitting] = useState(false);

  const request = useRef(0);
  useEffect(() => () => { request.current++; }, []);
  const changeOpen = (next: boolean) => { if (!next) request.current++; onOpenChange(next); };

  const submit = async () => {
    const pending = ++request.current;
    setSubmitting(true);
    try {
      const revision = await assetsApi().getAssetRevision({
        revisionId: revisionId.trim(),
      });
      if (pending !== request.current) return;
      if (type === "image" && !revision.mimeType.startsWith("image/")) {
        toast.error("所选文件版本不是图片");
        return;
      }
      if (type === "video" && !revision.mimeType.startsWith("video/")) {
        toast.error("所选文件版本不是视频");
        return;
      }
      if (type === "audio" && !revision.mimeType.startsWith("audio/")) {
        toast.error("所选文件版本不是音频");
        return;
      }
      if (type === "pdf" && revision.mimeType !== "application/pdf") {
        toast.error("所选文件版本不是 PDF");
        return;
      }
      if (type === "video" && posterRevisionId.trim()) {
        const poster = await assetsApi().getAssetRevision({
          revisionId: posterRevisionId.trim(),
        });
        if (pending !== request.current) return;
        if (!poster.mimeType.startsWith("image/")) {
          toast.error("视频封面文件版本不是图片");
          return;
        }
      }
      const id = initialBlock?.id ?? newBlockId();
      const common = {
        id,
        asset_revision_id: revisionId.trim(),
        ...(caption.trim() ? { caption: caption.trim() } : {}),
      };
      const parsed =
        type === "image"
          ? imageBlockSchema.safeParse({
              ...common,
              type,
              alt_text: altText.trim(),
            })
          : type === "video"
            ? videoBlockSchema.safeParse({
                ...common,
                type,
                ...(posterRevisionId.trim()
                  ? { poster_asset_revision_id: posterRevisionId.trim() }
                  : {}),
                ...(title.trim() ? { title: title.trim() } : {}),
              })
            : type === "audio"
          ? audioBlockSchema.safeParse({
              ...common,
              type,
              title: title.trim(),
              ...(artist.trim() ? { artist: artist.trim() } : {}),
              ...(album.trim() ? { album: album.trim() } : {}),
              ...(duration ? { duration_seconds: Number(duration) } : {}),
            })
          : type === "file"
            ? fileBlockSchema.safeParse({
                ...common,
                type,
                name: title.trim(),
              })
            : pdfBlockSchema.safeParse({
                ...common,
                type,
                ...(title.trim() ? { title: title.trim() } : {}),
                start_page: Number(startPage),
                height: Number(height),
              });
      if (!parsed.success) {
        toast.error("媒体块配置不完整", {
          description: parsed.error.issues[0]?.message,
        });
        return;
      }
      if (pending === request.current && onInsert(parsed.data as MediaBlock) !== false) changeOpen(false);
    } catch {
      if (pending !== request.current) return;
      toast.error("无法读取文件版本", {
        description: "请重试选择文件，或先到媒体库上传。",
      });
    } finally {
      if (pending === request.current) setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg" onKeyDown={event => event.stopPropagation()}>
        <DialogHeader>
          <DialogTitle>{initialBlock ? "编辑媒体与文件" : "插入媒体与文件"}</DialogTitle>
          <DialogDescription>
            页面会使用你选定的文件版本，上传新版本不会替换这里的内容。
          </DialogDescription>
        </DialogHeader>
        <fieldset disabled={submitting} className="grid min-w-0 gap-4">
          {!initialBlock && <Label className="grid gap-2">
            类型
            <select
              value={type}
              onChange={(event) => { setType(event.target.value as MediaType); setRevisionId(""); }}
              className="mt-1 h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
            >
              <option value="image">图片</option>
              <option value="video">视频</option>
              <option value="audio">音频</option>
              <option value="file">文件</option>
              <option value="pdf">PDF</option>
            </select>
          </Label>}
          <div className="space-y-2"><Label className="grid gap-2">文件</Label><ResourcePicker kind="asset" mediaKind={type} value={revisionId} onChange={item => { setRevisionId(item.id); if (!initialBlock) { setTitle(item.label); setAltText(item.label); } }} /></div>
          {type === "image" ? (
            <Label className="grid gap-2">
              替代文字
              <Input
                value={altText}
                onChange={(event) => setAltText(event.target.value)}
              />
            </Label>
          ) : (
            <Label className="grid gap-2">
              {type === "file" ? "文件名" : "标题"}
              <Input
                value={title}
                onChange={(event) => setTitle(event.target.value)}
              />
            </Label>
          )}
          {type === "video" ? (
            <div className="space-y-2"><Label className="grid gap-2">封面图片（可选）</Label><ResourcePicker kind="asset" mediaKind="image" label="封面图片" value={posterRevisionId} clearable onChange={item => setPosterRevisionId(item.id)} /></div>
          ) : null}
          {type === "audio" ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <Label className="grid gap-2">
                艺术家（可选）
                <Input
                  value={artist}
                  onChange={(event) => setArtist(event.target.value)}
                />
              </Label>
              <Label className="grid gap-2">
                专辑（可选）
                <Input
                  value={album}
                  onChange={(event) => setAlbum(event.target.value)}
                />
              </Label>
              <Label className="grid gap-2">
                时长秒数（可选）
                <Input
                  type="number"
                  min={0}
                  step="0.1"
                  value={duration}
                  onChange={(event) => setDuration(event.target.value)}
                />
              </Label>
            </div>
          ) : null}
          {type === "pdf" ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <Label className="grid gap-2">
                起始页
                <Input
                  type="number"
                  min={1}
                  value={startPage}
                  onChange={(event) => setStartPage(event.target.value)}
                />
              </Label>
              <Label className="grid gap-2">
                阅读器高度
                <Input
                  type="number"
                  min={320}
                  max={1200}
                  value={height}
                  onChange={(event) => setHeight(event.target.value)}
                />
              </Label>
            </div>
          ) : null}
          <Label className="grid gap-2">
            说明（可选）
            <Input
              value={caption}
              onChange={(event) => setCaption(event.target.value)}
            />
          </Label>
        </fieldset>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => changeOpen(false)}>
            取消
          </Button>
          <Button type="button" disabled={submitting || !revisionId} onClick={() => void submit()}>
            {initialBlock ? "保存媒体" : "插入"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
