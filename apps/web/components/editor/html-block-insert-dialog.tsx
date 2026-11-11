"use client";

import { useState } from "react";

import { toast } from "sonner";
import { BlockView } from "@/components/ast";
import { htmlBlockSchema } from "@/lib/ast/schema";
import { newBlockId } from "@/lib/editor/ids";

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

export function HTMLBlockInsertDialog({
  open,
  onOpenChange,
  onInsert,
  initialValue,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialValue?: { html: string; height: number; caption?: string };
  onInsert: (value: { html: string; height: number; caption?: string }) => boolean | void;
}) {
  const [html, setHTML] = useState(initialValue?.html ?? "<p>HTML content</p>");
  const [height, setHeight] = useState(String(initialValue?.height ?? 320));
  const [caption, setCaption] = useState(initialValue?.caption ?? "");

  const readFile = async (file: File | undefined) => {
    if (!file) return;
    if (file.size > 200000) {
      toast.error("HTML 文件不能超过 200 KB");
      return;
    }
    try {
      setHTML(await file.text());
      if (!caption) setCaption(file.name);
    } catch { toast.error("无法读取 HTML 文件，请重试"); }
  };

  const submit = () => {
    const parsed = htmlBlockSchema.safeParse({ id: newBlockId(), type: "html", html, height: Number(height), ...(caption.trim() ? { caption: caption.trim() } : {}) });
    if (!parsed.success) {
      toast.error("请检查 HTML、标题和高度", { description: "HTML 不超过 200 KB，标题不超过 1000 字，高度为 120–1200 px。" });
      return;
    }
    if (onInsert({ html: parsed.data.html, height: parsed.data.height, caption: parsed.data.caption }) !== false) onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-5xl" onKeyDown={event => event.stopPropagation()}>
        <DialogHeader>
          <DialogTitle>{initialValue ? "编辑 HTML" : "插入 HTML"}</DialogTitle>
          <DialogDescription>
            HTML 仅用于展示，不能运行脚本、访问网络或提交表单。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <Label className="grid gap-2">
            HTML 文件
            <Input
              type="file"
              accept=".html,.htm,text/html"
              onChange={(event) => void readFile(event.target.files?.[0])}
            />
          </Label>
          <div className="grid min-w-0 gap-4 lg:grid-cols-2"><Label className="block min-w-0 space-y-2">
            HTML
            <Textarea
              className="min-h-72 font-mono" spellCheck={false} aria-label="HTML 代码"
              value={html}
              onChange={(event) => setHTML(event.target.value)}
            />
          </Label>
          <div className="min-w-0 space-y-2"><Label className="grid gap-2">预览</Label><div className="overflow-hidden rounded-lg border bg-background px-3"><BlockView block={{ id: "html-preview", type: "html", html, height: Math.min(1200, Math.max(120, Number(height) || 320)), caption: caption || "HTML 预览" }} /></div></div></div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Label className="grid gap-2">
              高度（px）
              <Input
                type="number"
                min={120}
                max={1200}
                value={height}
                onChange={(event) => setHeight(event.target.value)}
              />
            </Label>
            <Label className="grid gap-2">
              标题（可选）
              <Input
                value={caption}
                onChange={(event) => setCaption(event.target.value)}
              />
            </Label>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button type="button" onClick={submit}>
            {initialValue ? "保存 HTML" : "插入 HTML"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
