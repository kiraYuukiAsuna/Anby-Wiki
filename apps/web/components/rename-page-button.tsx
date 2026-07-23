// 页面改名入口（M2-T07 补齐浏览器生命周期）：经生成客户端调用 Page 领域服务，
// 成功后旧标题由后端写入 alias，Page ID 与正文 Revision 均保持不变。
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { TextCursorInput } from "lucide-react";
import { toast } from "sonner";
import { ResponseError } from "../../../contracts/generated/typescript";

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
import { pagesApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";

export function RenamePageButton({
  pageId,
  currentTitle,
}: {
  pageId: string;
  currentTitle: string;
}) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState(currentTitle);
  const [submitting, setSubmitting] = useState(false);

  const rename = async () => {
    const nextTitle = title.trim();
    if (!nextTitle) {
      toast.error("页面标题不能为空");
      return;
    }
    if (nextTitle === currentTitle) {
      setOpen(false);
      return;
    }

    setSubmitting(true);
    try {
      const page = await pagesApi().renamePage({
        id: pageId,
        renamePageRequest: { title: nextTitle },
      });
      setOpen(false);
      toast.success(`页面已改名为「${page.displayTitle}」`);
      router.push(`/wiki/${encodeURIComponent(page.displayTitle)}`);
      router.refresh();
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("请先登录后再改名");
        router.push(LOGIN_PATH);
      } else if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("标题已被占用", {
          description: "另一个页面或页面别名正在使用该标题。",
        });
      } else if (error instanceof ResponseError) {
        toast.error(`改名失败（HTTP ${error.response.status}）`);
      } else {
        toast.error("网络错误，改名未完成");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="shrink-0 gap-1"
        onClick={() => {
          setTitle(currentTitle);
          setOpen(true);
        }}
      >
        <TextCursorInput aria-hidden className="size-3.5" />
        改名
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>页面改名</DialogTitle>
            <DialogDescription>
              旧标题会保留为别名，既有链接仍能解析到同一 Page ID。
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor="rename-page-title">新标题</Label>
            <Input
              id="rename-page-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setOpen(false)}
              disabled={submitting}
            >
              取消
            </Button>
            <Button onClick={() => void rename()} disabled={submitting}>
              {submitting ? "改名中…" : "确认改名"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
