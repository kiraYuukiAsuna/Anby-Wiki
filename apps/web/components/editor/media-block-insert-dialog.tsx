"use client";

import { useState } from "react";

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
import { newBlockId } from "@/lib/editor/ids";

type MediaType = "image" | "video" | "audio" | "file" | "pdf";
type MediaBlock = Extract<Block, { type: MediaType }>;

export function MediaBlockInsertDialog({
  open,
  onOpenChange,
  onInsert,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onInsert: (block: MediaBlock) => void;
}) {
  const [type, setType] = useState<MediaType>("image");
  const [revisionId, setRevisionId] = useState("");
  const [posterRevisionId, setPosterRevisionId] = useState("");
  const [altText, setAltText] = useState("");
  const [title, setTitle] = useState("");
  const [artist, setArtist] = useState("");
  const [album, setAlbum] = useState("");
  const [duration, setDuration] = useState("");
  const [caption, setCaption] = useState("");
  const [startPage, setStartPage] = useState("1");
  const [height, setHeight] = useState("640");
  const [submitting, setSubmitting] = useState(false);

  const submit = async () => {
    setSubmitting(true);
    try {
      const revision = await assetsApi().getAssetRevision({
        revisionId: revisionId.trim(),
      });
      if (type === "image" && !revision.mimeType.startsWith("image/")) {
        toast.error("所选 AssetRevision 不是图片");
        return;
      }
      if (type === "video" && !revision.mimeType.startsWith("video/")) {
        toast.error("所选 AssetRevision 不是视频");
        return;
      }
      if (type === "audio" && !revision.mimeType.startsWith("audio/")) {
        toast.error("所选 AssetRevision 不是音频");
        return;
      }
      if (type === "pdf" && revision.mimeType !== "application/pdf") {
        toast.error("所选 AssetRevision 不是 PDF");
        return;
      }
      if (type === "video" && posterRevisionId.trim()) {
        const poster = await assetsApi().getAssetRevision({
          revisionId: posterRevisionId.trim(),
        });
        if (!poster.mimeType.startsWith("image/")) {
          toast.error("视频封面 AssetRevision 不是图片");
          return;
        }
      }
      const id = newBlockId();
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
      onInsert(parsed.data as MediaBlock);
      onOpenChange(false);
    } catch {
      toast.error("无法读取 AssetRevision", {
        description: "请确认 ID 存在，并先在资产库上传文件。",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>插入媒体与文件</DialogTitle>
          <DialogDescription>
            页面固定引用不可变 AssetRevision，并按媒体类型校验 MIME。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <Label>
            类型
            <select
              value={type}
              onChange={(event) => setType(event.target.value as MediaType)}
              className="mt-1 h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
            >
              <option value="image">Image</option>
              <option value="video">Video</option>
              <option value="audio">Audio</option>
              <option value="file">File</option>
              <option value="pdf">PDF</option>
            </select>
          </Label>
          <Label>
            AssetRevision ID
            <Input
              value={revisionId}
              onChange={(event) => setRevisionId(event.target.value)}
            />
          </Label>
          {type === "image" ? (
            <Label>
              Alt text
              <Input
                value={altText}
                onChange={(event) => setAltText(event.target.value)}
              />
            </Label>
          ) : (
            <Label>
              {type === "file" ? "文件名" : "标题"}
              <Input
                value={title}
                onChange={(event) => setTitle(event.target.value)}
              />
            </Label>
          )}
          {type === "video" ? (
            <Label>
              Poster AssetRevision ID（可选）
              <Input
                value={posterRevisionId}
                onChange={(event) => setPosterRevisionId(event.target.value)}
              />
            </Label>
          ) : null}
          {type === "audio" ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <Label>
                艺术家（可选）
                <Input
                  value={artist}
                  onChange={(event) => setArtist(event.target.value)}
                />
              </Label>
              <Label>
                专辑（可选）
                <Input
                  value={album}
                  onChange={(event) => setAlbum(event.target.value)}
                />
              </Label>
              <Label>
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
              <Label>
                起始页
                <Input
                  type="number"
                  min={1}
                  value={startPage}
                  onChange={(event) => setStartPage(event.target.value)}
                />
              </Label>
              <Label>
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
          <Label>
            Caption（可选）
            <Input
              value={caption}
              onChange={(event) => setCaption(event.target.value)}
            />
          </Label>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button type="button" disabled={submitting} onClick={() => void submit()}>
            插入
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
