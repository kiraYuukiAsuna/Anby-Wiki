"use client";

import { useState } from "react";

import { LoaderCircle, RefreshCw } from "lucide-react";
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
import { Textarea } from "@/components/ui/textarea";
import {
  bookmarkBlockSchema,
  embedBlockSchema,
  linkPreviewBlockSchema,
  type Block,
} from "@/lib/ast/schema";
import { sourcesApi } from "@/lib/api";
import { detectEmbedProvider } from "@/lib/embed";
import { newBlockId } from "@/lib/editor/ids";
import type { LinkPreviewMetadata } from "../../../../contracts/generated/typescript";

type ExternalType = "bookmark" | "link_preview" | "embed";
type ExternalBlock = Extract<Block, { type: ExternalType }>;

export function ExternalContentInsertDialog({
  open,
  onOpenChange,
  resolvePreview,
  onInsert,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  resolvePreview?: (url: string) => Promise<LinkPreviewMetadata>;
  onInsert: (block: ExternalBlock) => void;
}) {
  const [type, setType] = useState<ExternalType>("bookmark");
  const [url, setURL] = useState("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [siteName, setSiteName] = useState("");
  const [caption, setCaption] = useState("");
  const [resolving, setResolving] = useState(false);

  const resolve = async () => {
    setResolving(true);
    try {
      const preview = resolvePreview
        ? await resolvePreview(url.trim())
        : await sourcesApi().resolveLinkPreview({
            resolveLinkPreviewRequest: { url: url.trim() },
          });
      setURL(preview.url);
      setTitle(preview.title);
      setDescription(preview.description ?? "");
      setSiteName(preview.siteName ?? "");
      return preview;
    } catch {
      toast.error("无法解析第三方预览", {
        description: "地址必须是可公开访问的 http/https 页面。",
      });
      return null;
    } finally {
      setResolving(false);
    }
  };

  const submit = async () => {
    if (type === "embed") {
      const provider = detectEmbedProvider(url);
      if (!provider) {
        toast.error("交互式 Embed 只支持 YouTube、Vimeo 与 Figma");
        return;
      }
      const parsed = embedBlockSchema.safeParse({
        id: newBlockId(),
        type,
        url: url.trim(),
        ...(title.trim() ? { title: title.trim() } : {}),
        provider,
        ...(caption.trim() ? { caption: caption.trim() } : {}),
      });
      if (!parsed.success) {
        toast.error("嵌入内容配置无效", {
          description: parsed.error.issues[0]?.message,
        });
        return;
      }
      onInsert(parsed.data);
      onOpenChange(false);
      return;
    }

    let metadata = {
      url: url.trim(),
      title: title.trim(),
      description: description.trim(),
      siteName: siteName.trim(),
    };
    if (!metadata.title) {
      const preview = await resolve();
      if (!preview) return;
      metadata = {
        url: preview.url,
        title: preview.title,
        description: preview.description ?? "",
        siteName: preview.siteName ?? "",
      };
    }
    const common = {
      id: newBlockId(),
      type,
      url: metadata.url,
      title: metadata.title,
      ...(metadata.description ? { description: metadata.description } : {}),
      ...(metadata.siteName ? { site_name: metadata.siteName } : {}),
    };
    const parsed =
      type === "bookmark"
        ? bookmarkBlockSchema.safeParse({
            ...common,
            ...(caption.trim() ? { caption: caption.trim() } : {}),
          })
        : linkPreviewBlockSchema.safeParse(common);
    if (!parsed.success) {
      toast.error("链接预览配置无效", {
        description: parsed.error.issues[0]?.message,
      });
      return;
    }
    onInsert(parsed.data as ExternalBlock);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>插入外部内容</DialogTitle>
          <DialogDescription>
            标题、封面和摘要会在插入时保存，之后不会随原网站更新。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <Label>
            类型
            <select
              value={type}
              onChange={(event) => setType(event.target.value as ExternalType)}
              className="mt-1 h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
            >
              <option value="bookmark">书签</option>
              <option value="link_preview">链接预览</option>
              <option value="embed">交互内容</option>
            </select>
          </Label>
          <Label>
            链接地址
            <div className="mt-1 flex gap-2">
              <Input
                type="url"
                value={url}
                onChange={(event) => setURL(event.target.value)}
              />
              {type !== "embed" ? (
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  title="解析预览"
                  disabled={resolving}
                  onClick={() => void resolve()}
                >
                  {resolving ? (
                    <LoaderCircle className="animate-spin" aria-hidden />
                  ) : (
                    <RefreshCw aria-hidden />
                  )}
                </Button>
              ) : null}
            </div>
          </Label>
          <Label>
            标题
            <Input value={title} onChange={(event) => setTitle(event.target.value)} />
          </Label>
          {type !== "embed" ? (
            <>
              <Label>
                站点名
                <Input
                  value={siteName}
                  onChange={(event) => setSiteName(event.target.value)}
                />
              </Label>
              <Label>
                描述
                <Textarea
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                />
              </Label>
            </>
          ) : null}
          {type !== "link_preview" ? (
            <Label>
              说明（可选）
              <Input
                value={caption}
                onChange={(event) => setCaption(event.target.value)}
              />
            </Label>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button type="button" disabled={resolving} onClick={() => void submit()}>
            插入
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
