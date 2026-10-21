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
import { Textarea } from "@/components/ui/textarea";

export function HTMLBlockInsertDialog({
  open,
  onOpenChange,
  onInsert,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onInsert: (value: { html: string; height: number; caption?: string }) => void;
}) {
  const [html, setHTML] = useState("<p>HTML content</p>");
  const [height, setHeight] = useState("320");
  const [caption, setCaption] = useState("");

  const readFile = async (file: File | undefined) => {
    if (!file) return;
    if (file.size > 200000) {
      toast.error("HTML 文件不能超过 200 KB");
      return;
    }
    setHTML(await file.text());
    if (!caption) setCaption(file.name);
  };

  const submit = () => {
    const parsedHeight = Number(height);
    if (
      html.length > 200000 ||
      !Number.isInteger(parsedHeight) ||
      parsedHeight < 120 ||
      parsedHeight > 1200
    ) {
      toast.error("请将 HTML 控制在 200 KB 内，高度设为 120-1200 px");
      return;
    }
    onInsert({
      html,
      height: parsedHeight,
      ...(caption.trim() ? { caption: caption.trim() } : {}),
    });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>插入 HTML</DialogTitle>
          <DialogDescription>
            内容在无脚本、无网络、无表单权限的 sandbox iframe 中显示。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <Label>
            HTML 文件
            <Input
              type="file"
              accept=".html,.htm,text/html"
              onChange={(event) => void readFile(event.target.files?.[0])}
            />
          </Label>
          <Label>
            HTML
            <Textarea
              className="min-h-56 font-mono"
              value={html}
              onChange={(event) => setHTML(event.target.value)}
            />
          </Label>
          <div className="grid gap-4 sm:grid-cols-2">
            <Label>
              高度（px）
              <Input
                type="number"
                min={120}
                max={1200}
                value={height}
                onChange={(event) => setHeight(event.target.value)}
              />
            </Label>
            <Label>
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
            插入 HTML
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
