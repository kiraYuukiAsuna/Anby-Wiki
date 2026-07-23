// 外部链接编辑器（M2-T04）：为选中文本插入/编辑外链。
// url 经 Zod 校验仅允许 http/https；display_text 与 url 分离
// （display_text 留空时回退为 url 本身）。
"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
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
import { httpUrlSchema } from "@/lib/http-url";

/** 外链 URL 校验：仅 http/https（AST external_link.url 本身只要求合法 URL）。 */
export const externalLinkUrlSchema = httpUrlSchema;

export interface ExternalLinkEditorProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 打开时的默认显示文本（通常是编辑器选中文本）。 */
  defaultDisplayText?: string;
  /** 校验通过后回调（url 与 displayText 分离）。 */
  onSubmit: (url: string, displayText: string) => void;
}

export function ExternalLinkEditor({
  open,
  onOpenChange,
  defaultDisplayText = "",
  onSubmit,
}: ExternalLinkEditorProps) {
  const [url, setUrl] = useState("");
  // 初次以 open=true 挂载时即注入默认显示文本；后续开关经下方渲染期调整重置。
  const [displayText, setDisplayText] = useState(defaultDisplayText);
  const [urlError, setUrlError] = useState<string | null>(null);

  // 打开时重置并注入默认显示文本（渲染期调整，避免 effect 级联渲染）。
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) {
      setUrl("");
      setDisplayText(defaultDisplayText);
      setUrlError(null);
    }
  }

  const submit = () => {
    const parsed = externalLinkUrlSchema.safeParse(url.trim());
    if (!parsed.success) {
      setUrlError(parsed.error.issues[0]?.message ?? "URL 不合法");
      return;
    }
    onSubmit(parsed.data, displayText.trim() || parsed.data);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>插入外部链接</DialogTitle>
          <DialogDescription>
            为选中文本添加 http/https 外链；显示文本与链接地址分离。
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="external-link-url">链接地址</Label>
            <Input
              id="external-link-url"
              value={url}
              onChange={(event) => {
                setUrl(event.target.value);
                setUrlError(null);
              }}
              placeholder="https://example.com"
              autoFocus
              aria-invalid={urlError ? true : undefined}
            />
            {urlError ? (
              <p className="text-xs text-destructive" role="alert">
                {urlError}
              </p>
            ) : null}
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="external-link-display-text">显示文本</Label>
            <Input
              id="external-link-display-text"
              value={displayText}
              onChange={(event) => setDisplayText(event.target.value)}
              placeholder="留空则显示链接地址"
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            取消
          </Button>
          <Button onClick={submit}>插入链接</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
