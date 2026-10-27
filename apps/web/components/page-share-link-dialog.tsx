"use client";

import { Check, Copy, LoaderCircle, Share2 } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import {
  ResponseError,
  type CreatePageShareLinkRequestLinkTypeEnum,
  type CreatePageShareLinkRequestRoleKeyEnum,
} from "../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { wikiApi } from "@/lib/api";
import { isUnauthorized } from "@/lib/auth";

type ShareRole = "viewer" | "editor";
type LinkType = "once" | "permanent";

const roleLabels: Record<ShareRole, string> = {
  viewer: "可查看",
  editor: "可编辑",
};

const linkTypeLabels: Record<LinkType, string> = {
  once: "一次性",
  permanent: "永久有效",
};

export function PageShareLinkDialog({
  pageId,
  pageTitle,
}: {
  pageId: string;
  pageTitle: string;
}) {
  const [open, setOpen] = useState(false);
  const [roleKey, setRoleKey] = useState<ShareRole>("viewer");
  const [linkType, setLinkType] = useState<LinkType>("once");
  const [shareUrl, setShareUrl] = useState("");
  const [creating, setCreating] = useState(false);

  const canCopy = useMemo(() => shareUrl.length > 0, [shareUrl]);

  const createLink = async () => {
    setCreating(true);
    try {
      const result = await wikiApi().createPageShareLink({
        id: pageId,
        createPageShareLinkRequest: {
          roleKey: roleKey as CreatePageShareLinkRequestRoleKeyEnum,
          linkType: linkType as CreatePageShareLinkRequestLinkTypeEnum,
        },
      });
      const url = `${window.location.origin}/share#token=${encodeURIComponent(
        result.token,
      )}`;
      setShareUrl(url);
      await navigator.clipboard.writeText(url);
      toast.success("分享链接已生成并复制", {
        description: `${roleLabels[roleKey]} · ${linkTypeLabels[linkType]}`,
      });
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("当前账号不能分享这个页面");
      } else {
        toast.error("生成分享链接失败");
      }
    } finally {
      setCreating(false);
    }
  };

  const copyLink = async () => {
    if (!shareUrl) return;
    await navigator.clipboard.writeText(shareUrl);
    toast.success("分享链接已复制");
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1">
          <Share2 aria-hidden className="size-3.5" />
          分享
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>分享当前页面</DialogTitle>
          <DialogDescription>{pageTitle}</DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="grid gap-2 text-sm font-medium">
              权限
              <select
                value={roleKey}
                onChange={(event) => {
                  setRoleKey(event.target.value as ShareRole);
                  setShareUrl("");
                }}
                className="h-10 rounded-md border bg-background px-3 text-sm font-normal"
              >
                <option value="viewer">{roleLabels.viewer}</option>
                <option value="editor">{roleLabels.editor}</option>
              </select>
            </label>
            <label className="grid gap-2 text-sm font-medium">
              有效方式
              <select
                value={linkType}
                onChange={(event) => {
                  setLinkType(event.target.value as LinkType);
                  setShareUrl("");
                }}
                className="h-10 rounded-md border bg-background px-3 text-sm font-normal"
              >
                <option value="once">{linkTypeLabels.once}</option>
                <option value="permanent">{linkTypeLabels.permanent}</option>
              </select>
            </label>
          </div>

          {shareUrl ? (
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="share-url">
                分享链接
              </label>
              <div className="flex gap-2">
                <Input
                  id="share-url"
                  value={shareUrl}
                  readOnly
                  className="font-mono text-xs"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  onClick={() => void copyLink()}
                  aria-label="复制分享链接"
                >
                  <Copy aria-hidden />
                </Button>
              </div>
            </div>
          ) : null}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => setOpen(false)}
          >
            关闭
          </Button>
          <Button
            type="button"
            onClick={() => void (canCopy ? copyLink() : createLink())}
            disabled={creating}
          >
            {creating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : canCopy ? (
              <Check aria-hidden />
            ) : (
              <Share2 aria-hidden />
            )}
            {canCopy ? "复制链接" : "生成链接"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
