// 回滚入口 + 确认对话框（M2-T05）：历史列表行与单版详情页共用。
//
// 流程：确认对话框（目标版本短 id + 时间 + 可编辑 summary，默认「回滚到 {短id}」）
// → historyApi.rollbackPage → 成功 toast + 跳转 /pages/[id]（阅读页展示新当前版本）。
// 错误：409 stale_revision（回滚与并发发布交错）提示刷新重试；401 引导登录。
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import {
  ResponseError,
  type Revision,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { historyApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";

import { formatDateTime, shortId } from "./utils";

/** 从 ResponseError 中尽力读取契约 Error.code（与 editor-session-view 同款兜底）。 */
async function readErrorCode(error: ResponseError): Promise<string | null> {
  try {
    const body = (await error.response.json()) as { code?: string };
    return body.code ?? null;
  } catch {
    return null;
  }
}

export function RollbackButton({
  pageId,
  revision,
}: {
  pageId: string;
  /** 回滚目标 Revision。 */
  revision: Revision;
}) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [summary, setSummary] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const openDialog = () => {
    setSummary(`回滚到 ${shortId(revision.id)}`);
    setOpen(true);
  };

  const confirm = async () => {
    setSubmitting(true);
    try {
      await historyApi().rollbackPage({
        id: pageId,
        rollbackRequest: {
          targetRevisionId: revision.id,
          summary: summary.trim() || undefined,
        },
      });
      setOpen(false);
      toast.success(`已回滚到版本 ${shortId(revision.id)}`);
      // 回滚以旧快照追加新 Revision，阅读页展示的即是回滚后的新当前版本。
      router.push(`/pages/${pageId}`);
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("请先登录后再回滚");
        router.push(LOGIN_PATH);
      } else if (error instanceof ResponseError && error.response.status === 409) {
        const code = await readErrorCode(error);
        if (code === "stale_revision") {
          // 回滚与并发发布交错（行锁串行化后基线过期）：提示刷新重试。
          toast.warning("页面在你操作期间已有新版本", {
            description: "请刷新历史列表确认最新版本后重试回滚。",
          });
          setOpen(false);
          return;
        }
      }
      if (error instanceof ResponseError) {
        toast.error(`回滚失败（HTTP ${error.response.status}）`, {
          description: "页面内容未受影响，可重试。",
        });
      } else {
        toast.error("网络错误，回滚未完成", {
          description: "页面内容未受影响，请检查网络后重试。",
        });
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <Button variant="outline" size="sm" onClick={openDialog}>
        回滚
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>回滚到此版本</DialogTitle>
            <DialogDescription>
              将以版本 {shortId(revision.id)}（
              {formatDateTime(revision.createdAt)}
              ）的内容追加一个新 Revision 并设为当前版本；历史版本不会被修改。
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor={`rollback-summary-${revision.id}`}>修改摘要</Label>
            <Textarea
              id={`rollback-summary-${revision.id}`}
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              rows={2}
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
            <Button onClick={() => void confirm()} disabled={submitting}>
              {submitting ? "回滚中…" : "确认回滚"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
