"use client";

import {
  ArrowUpRight,
  CircleHelp,
  ExternalLink,
  FileText,
  Hash,
  LoaderCircle,
  Route,
  Trash2,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";

import {
  PageRedirectTargetKindEnum,
  ResponseError,
  type PageRedirect,
} from "../../../../contracts/generated/typescript";

import { RedirectPageButton } from "@/components/redirect-page-button";
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
import { pagesApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  dateStyle: "medium",
  timeStyle: "short",
});

async function fetchRedirect(pageId: string): Promise<PageRedirect | null> {
  try {
    return await pagesApi().getPageRedirect({ id: pageId });
  } catch (error) {
    if (error instanceof ResponseError && error.response.status === 404) {
      return null;
    }
    throw error;
  }
}

function targetPresentation(redirect: PageRedirect) {
  const { target } = redirect;
  switch (target.kind) {
    case PageRedirectTargetKindEnum.Page:
      return {
        label: "已有 Page",
        title: target.targetPageTitle ?? target.targetPageId ?? "未知 Page",
        detail: target.targetPageId ?? "",
        icon: FileText,
      };
    case PageRedirectTargetKindEnum.PageSection:
      return {
        label: "稳定章节",
        title: target.targetPageTitle ?? target.targetPageId ?? "未知 Page",
        detail: target.anchorBlockId ?? "",
        icon: Hash,
      };
    case PageRedirectTargetKindEnum.Unresolved:
      return {
        label: "未解析标题",
        title: `${target.namespace ?? "?"}:${target.targetTitle ?? "?"}`,
        detail: "目标创建后将动态解析",
        icon: CircleHelp,
      };
    case PageRedirectTargetKindEnum.Interwiki:
      return {
        label: "外部 Wiki",
        title: target.targetTitle ?? target.externalUrl ?? "外部目标",
        detail: target.externalUrl ?? "",
        icon: ExternalLink,
      };
  }
}

export function PageRedirectManager({
  pageId,
  pageTitle,
}: {
  pageId: string;
  pageTitle: string;
}) {
  const router = useRouter();
  const session = useSession();
  const redirectState = useSWR(["page:redirect", pageId], () =>
    fetchRedirect(pageId),
  );
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const startDelete = () => {
    if (!session.isAuthenticated) {
      toast.error("请先登录后删除重定向");
      router.push(LOGIN_PATH);
      return;
    }
    setConfirmed(false);
    setDeleteOpen(true);
  };

  const remove = async () => {
    if (!confirmed) return;
    setDeleting(true);
    try {
      await pagesApi().deletePageRedirect({ id: pageId });
      await redirectState.mutate(null, { revalidate: false });
      setDeleteOpen(false);
      toast.success("页面重定向已删除", {
        description: "页面自身 Revision 未受影响，现在会直接展示其当前内容。",
      });
      router.refresh();
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
        router.push(LOGIN_PATH);
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("你没有编辑这个页面的权限");
      } else {
        toast.error("无法删除重定向，请稍后重试");
      }
    } finally {
      setDeleting(false);
    }
  };

  const redirect = redirectState.data;
  const presentation = redirect ? targetPresentation(redirect) : undefined;
  const TargetIcon = presentation?.icon ?? Route;

  return (
    <>
      <section className="rounded-2xl border border-border bg-card shadow-sm">
        <header className="flex flex-wrap items-start justify-between gap-4 border-b border-border/75 px-5 py-5">
          <div>
            <p className="flex items-center gap-2 text-sm font-semibold">
              <Route className="size-4 text-violet-700" aria-hidden />
              页面重定向
            </p>
            <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
              一个稳定 Page ID 对应一个判别式目标。未解析标题会在未来动态落地，外部目标始终显示离站提示。
            </p>
          </div>
          <RedirectPageButton
            sourcePageId={pageId}
            sourceTitle={pageTitle}
            initialTarget={redirect?.target}
            label={redirect ? "修改目标" : "创建重定向"}
            onSaved={async () => {
              await redirectState.mutate();
            }}
          />
        </header>

        <div className="p-5">
          {redirectState.isLoading ? (
            <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
              <LoaderCircle className="size-4 animate-spin" aria-hidden />
              读取当前目标…
            </div>
          ) : redirectState.error ? (
            <div className="rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive">
              当前重定向状态暂时不可用。
            </div>
          ) : redirect && presentation ? (
            <div className="rounded-xl border border-violet-200 bg-violet-50/55 p-4">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="flex min-w-0 items-start gap-3">
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-violet-100 text-violet-700">
                    <TargetIcon className="size-4" aria-hidden />
                  </span>
                  <div className="min-w-0">
                    <p className="text-[10px] font-semibold tracking-[0.16em] text-violet-700 uppercase">
                      {presentation.label}
                    </p>
                    <p className="mt-1 truncate font-semibold text-violet-950">
                      {presentation.title}
                    </p>
                    <p className="mt-1 break-all font-mono text-[10px] text-violet-900/65">
                      {presentation.detail}
                    </p>
                    {redirect.target.targetPageId ? (
                      <Link
                        href={`/pages/${redirect.target.targetPageId}${
                          redirect.target.anchorBlockId
                            ? `#${redirect.target.anchorBlockId}`
                            : ""
                        }`}
                        className="mt-3 inline-flex items-center gap-1.5 text-xs font-semibold text-violet-800 hover:underline"
                      >
                        打开本地目标
                        <ArrowUpRight className="size-3.5" aria-hidden />
                      </Link>
                    ) : redirect.target.externalUrl ? (
                      <a
                        href={redirect.target.externalUrl}
                        target="_blank"
                        rel="noreferrer"
                        className="mt-3 inline-flex items-center gap-1.5 text-xs font-semibold text-violet-800 hover:underline"
                      >
                        检查外部目标
                        <ArrowUpRight className="size-3.5" aria-hidden />
                      </a>
                    ) : null}
                  </div>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="text-destructive hover:text-destructive"
                  onClick={startDelete}
                >
                  <Trash2 aria-hidden />
                  删除
                </Button>
              </div>
              <dl className="mt-4 grid gap-3 border-t border-violet-200 pt-4 text-xs sm:grid-cols-2">
                <div>
                  <dt className="text-violet-800/65">最近更新</dt>
                  <dd className="mt-1 font-medium text-violet-950">
                    {DATE_FORMATTER.format(redirect.updatedAt)}
                  </dd>
                </div>
                <div>
                  <dt className="text-violet-800/65">更新 Actor</dt>
                  <dd className="mt-1 font-mono text-[10px] text-violet-950">
                    {redirect.updatedBy}
                  </dd>
                </div>
              </dl>
            </div>
          ) : (
            <div className="rounded-xl border border-dashed border-border px-5 py-9 text-center">
              <Route className="mx-auto size-5 text-muted-foreground" aria-hidden />
              <p className="mt-3 text-sm font-medium">当前没有页面重定向</p>
              <p className="mt-1 text-xs text-muted-foreground">
                访问这个 Page ID 时会直接展示其当前 Revision。
              </p>
            </div>
          )}
        </div>
      </section>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>删除页面重定向</DialogTitle>
            <DialogDescription>
              「{pageTitle}」将恢复直接阅读自身内容。历史 Revision 与审计记录不会删除。
            </DialogDescription>
          </DialogHeader>
          <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-border p-3 text-xs leading-5">
            <Checkbox
              checked={confirmed}
              onCheckedChange={(value) => setConfirmed(value === true)}
              className="mt-0.5"
            />
            <span>我确认移除当前目标，并保留该动作的不可变审计记录。</span>
          </label>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={deleting}
              onClick={() => setDeleteOpen(false)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={!confirmed || deleting}
              onClick={() => void remove()}
            >
              {deleting ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Trash2 aria-hidden />
              )}
              {deleting ? "删除中…" : "确认删除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
