// 编辑会话视图（M2-T03）：/pages/[id]/edit 的客户端主体。
//
// 数据流：服务端组件注入页面当前内容（初始 props）→ startSession 建立
// Zustand 会话（含 localStorage 草稿恢复）→ BlockEditor onChange 回写 AST →
// 发布对话框（Zod 校验 → pagesApi.publishRevision）。
// 错误处理：409 stale_revision 拉取 Current 后进入常驻冲突条，本地内容不丢失；
// 网络/5xx 可重试；草稿只是恢复辅助，绝不自动发布。
"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { BlockEditor } from "@/components/editor/block-editor";
import { ConflictBanner } from "@/components/editor/conflict-banner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { pagesApi, readingApi } from "@/lib/api";
import { parseDocument, type Document } from "@/lib/ast/schema";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";
import {
  CollaborationClient,
  type CollaborationStatus,
} from "@/lib/collaboration/client";
import { useEditorSession } from "@/lib/editor-session";

export interface EditorSessionViewProps {
  pageId: string;
  /** 页面展示标题（顶栏显示与发布后跳转用）。 */
  displayTitle: string;
  /** 服务端当前 Revision；未发布过为 null（首发布）。 */
  baseRevisionId: string | null;
  /** 服务端当前内容；未发布过为空文档。 */
  initialAst: Document;
}

/** 从 ResponseError 中尽力读取契约 Error.code。 */
async function readErrorCode(error: ResponseError): Promise<string | null> {
  try {
    const body = (await error.response.json()) as { code?: string };
    return body.code ?? null;
  } catch {
    return null;
  }
}

export function EditorSessionView({
  pageId,
  displayTitle,
  baseRevisionId,
  initialAst,
}: EditorSessionViewProps) {
  const router = useRouter();
  const session = useEditorSession();
  const [publishOpen, setPublishOpen] = useState(false);
  const [collaborationStatus, setCollaborationStatus] =
    useState<CollaborationStatus>("connecting");
  const collaborationRef = useRef<CollaborationClient | null>(null);
  // 丢弃草稿后需要重挂载 BlockEditor（其 initialAst 只在挂载时读取一次）。
  const [editorEpoch, setEditorEpoch] = useState(0);

  // 进入编辑页：建立会话并恢复本地草稿。草稿 base 落后时提示，
  // 默认保留内容（base 已重置到服务端 current，可继续编辑或手动丢弃）。
  useEffect(() => {
    const restore = session.startSession({
      pageId,
      baseRevisionId,
      ast: initialAst,
    });
    if (restore === "stale") {
      toast.warning("本地草稿基于旧版本，已保留草稿内容", {
        id: "stale-draft",
        description: "页面在你上次编辑后已有新版本；请通过冲突提示条选择处理方式。",
        duration: Infinity,
      });
    }
    const currentAst = useEditorSession.getState().ast ?? initialAst;
    const collaboration = new CollaborationClient({
      pageId,
      initialAst: currentAst,
      onAst: (ast) => {
        useEditorSession.getState().setAst(ast);
        setEditorEpoch((epoch) => epoch + 1);
      },
      onStatus: setCollaborationStatus,
    });
    // Only an explicitly restored local draft should override recovered
    // WorkingDocument state. The published initial AST may be stale.
    if (restore !== "none") collaboration.syncAst(currentAst);
    collaborationRef.current = collaboration;
    collaboration.connect();
    return () => {
      collaborationRef.current = null;
      collaboration.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId, baseRevisionId]);

  const openPublishDialog = () => {
    if (session.conflict) {
      toast.warning("请先处理版本冲突", {
        description: "可查看差异、切换到最新版为基，或放弃本地修改。",
      });
      return;
    }
    if (!session.dirty) {
      toast.info("内容没有改动，无需发布");
      return;
    }
    setPublishOpen(true);
  };

  // Ctrl/Cmd+S 打开发布对话框（与保存按钮一致）。
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === "s") {
        event.preventDefault();
        openPublishDialog();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session.dirty]);

  const publish = async () => {
    session.setPublishing(true);
    try {
      const workingDocumentId =
        collaborationRef.current?.getDocumentId() ?? null;
      await pagesApi().publishRevision({
        id: pageId,
        publishRevisionRequest: {
          ...(session.baseRevisionId
            ? { expectedRevisionId: session.baseRevisionId }
            : {}),
          ...(workingDocumentId ? { workingDocumentId } : {}),
          ast: session.ast as { [key: string]: unknown },
          summary: session.summary || undefined,
          isMinor: session.isMinor,
        },
      });
      const rebased = session.rebasedFromConflict;
      session.clearDraft();
      session.clearRebasedFlag();
      setPublishOpen(false);
      toast.success(rebased ? "已基于最新版本发布" : "发布成功");
      router.push(`/pages/${pageId}`);
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("请先登录后再发布", {
          description: "编辑内容已保留，登录后可继续发布。",
        });
        router.push(LOGIN_PATH);
        return;
      }
      if (error instanceof ResponseError && error.response.status === 409) {
        const code = await readErrorCode(error);
        if (code === "stale_revision") {
          // 首次 409 拉取服务端 Current，供冲突条展示与「放弃修改」回退。
          // 已有冲突时幂等保留首次 Base/Current，不重复请求。
          if (!useEditorSession.getState().conflict) {
            try {
              const latest = await readingApi().getPageByID({ id: pageId });
              const serverAst = latest.content
                ? parseDocument(latest.content.astJson)
                : parseDocument({
                    type: "document",
                    schema_version: 1,
                    children: [],
                  });
              useEditorSession.getState().recordConflict({
                currentRevisionId: latest.page.currentRevisionId ?? null,
                serverAst,
              });
            } catch {
              toast.error("无法获取服务端最新版本", {
                description: "你的编辑内容已保留，请稍后重试发布以重新获取。",
              });
              setPublishOpen(false);
              return;
            }
          }
          toast.warning("页面已被他人更新", {
            description: "你的编辑内容已保留；请在冲突提示条中查看差异并选择处理方式。",
          });
          setPublishOpen(false);
          return;
        }
      }
      if (error instanceof ResponseError) {
        toast.error(`发布失败（HTTP ${error.response.status}）`, {
          description: "编辑内容已保留，可重试。",
        });
      } else {
        toast.error("网络错误，发布未完成", {
          description: "编辑内容已保留，请检查网络后重试。",
        });
      }
    } finally {
      session.setPublishing(false);
    }
  };

  const onConfirmPublish = () => {
    // Zod 校验失败：阻断发布并提示（parseDocument 抛 ZodError）。
    try {
      parseDocument(session.ast);
    } catch {
      toast.error("内容校验失败，无法发布", {
        description: "文档结构不符合 AST v1 Schema，请检查编辑内容。",
      });
      return;
    }
    void publish();
  };

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-4 px-4 py-6">
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-xl font-semibold">
            编辑：{displayTitle}
          </h1>
          <p className="text-xs text-muted-foreground">
            {baseRevisionId ? "基于当前发布版本编辑" : "首次发布（页面尚无内容）"}
            {session.dirty ? " · 有未发布改动" : " · 无改动"}
            {" · "}
            <span data-collaboration-status={collaborationStatus}>
              {collaborationStatusLabel(collaborationStatus)}
            </span>
          </p>
        </div>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/pages/${pageId}`}>返回阅读</Link>
        </Button>
        <Button
          size="sm"
          onClick={openPublishDialog}
          disabled={session.publishing}
        >
          发布…
        </Button>
      </div>

      <ConflictBanner
        pageId={pageId}
        onReset={() => setEditorEpoch((epoch) => epoch + 1)}
      />

      <div className="rounded-lg border border-border p-4">
        {session.ast ? (
          <BlockEditor
            key={editorEpoch}
            initialAst={session.ast}
            onChange={(ast) => {
              session.setAst(ast);
              collaborationRef.current?.syncAst(ast);
            }}
          />
        ) : null}
      </div>

      <Dialog open={publishOpen} onOpenChange={setPublishOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>发布修改</DialogTitle>
            <DialogDescription>
              为本次修改填写摘要后发布；发布即成为页面当前版本。
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="publish-summary">修改摘要</Label>
              <Textarea
                id="publish-summary"
                value={session.summary}
                onChange={(event) => session.setSummary(event.target.value)}
                placeholder="本次修改了什么？"
                rows={3}
              />
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="publish-is-minor"
                checked={session.isMinor}
                onCheckedChange={(checked) =>
                  session.setIsMinor(checked === true)
                }
              />
              <Label htmlFor="publish-is-minor">小修改（错别字、格式等）</Label>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setPublishOpen(false)}
              disabled={session.publishing}
            >
              取消
            </Button>
            <Button onClick={onConfirmPublish} disabled={session.publishing}>
              {session.publishing ? "发布中…" : "确认发布"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function collaborationStatusLabel(status: CollaborationStatus): string {
  switch (status) {
    case "connecting":
      return "协作连接中";
    case "syncing":
      return "协作同步中";
    case "online":
      return "协作在线";
    case "offline":
      return "协作离线，修改将在重连后同步";
    case "closed":
      return "协作已停止";
  }
}
