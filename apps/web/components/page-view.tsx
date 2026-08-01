// 阅读页内容视图：/wiki/[...title] 与 /pages/[id] 共用。
// 渲染重定向/别名提示条、正文 AST、目录与 Revision 信息条；
// 标题行右侧是「编辑」入口（M2-T03，P0 无认证，不做登录态区分）。
import Link from "next/link";
import {
  ArrowRight,
  ArrowUpRight,
  CircleHelp,
  ExternalLink,
  Hash,
  MoveRight,
  Pencil,
  Wrench,
} from "lucide-react";

import { AstDocument } from "@/components/ast";
import { LazySectionDocument } from "@/components/lazy-section-document";
import { RenamePageButton } from "@/components/rename-page-button";
import { RedirectPageButton } from "@/components/redirect-page-button";
import { RedirectSectionFocus } from "@/components/redirect-section-focus";
import { RevisionInfo } from "@/components/revision-info";
import { ServerRenderedDocument } from "@/components/server-rendered-document";
import { StableAnchorResolver } from "@/components/stable-anchor-resolver";
import { TableOfContents, TocSidebar } from "@/components/toc";
import { Button } from "@/components/ui/button";
import { parseDocument } from "@/lib/ast/schema";
import { extractToc } from "@/lib/ast/toc";
import type { PageWithContent } from "../../../contracts/generated/typescript";

export function PageView({ data }: { data: PageWithContent }) {
  const { page } = data;
  // 生成客户端把 nullable content 标为非空类型，运行时仍可能为 null（未发布）。
  const content = data.content ?? null;
  const sectioned = content?.deliveryMode === "sections";
  const document =
    content && !sectioned && content.astJson
      ? parseDocument(content.astJson)
      : null;
  const toc = document ? extractToc(document) : [];
  const redirect = data.redirect ?? null;
  const redirectSourceId = redirect?.fromPageId ?? page.id;
  const redirectSourceTitle = redirect?.fromTitle ?? page.displayTitle;
  const terminalRedirect = redirect?.resolved === false;
  const sectionTarget =
    redirect?.target.kind === "page_section"
      ? redirect.target.anchorBlockId
      : undefined;

  return (
    <div className="mx-auto flex w-full max-w-5xl gap-8 px-4 py-8">
      <StableAnchorResolver pageId={page.id} />
      <RedirectSectionFocus blockId={sectionTarget} />
      <article className="min-w-0 max-w-3xl flex-1">
        {redirect?.resolved ? (
          <p
            role="status"
            className="mb-4 flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground"
          >
            {sectionTarget ? (
              <Hash className="size-4 shrink-0" aria-hidden />
            ) : (
              <ArrowRight className="size-4 shrink-0" aria-hidden />
            )}
            重定向自 <span className="font-medium">{redirect.fromTitle}</span>
            {sectionTarget ? "，已定位到稳定章节" : null}
            {redirect.hops > 1 ? `（${redirect.hops} 跳）` : null}
          </p>
        ) : null}
        {data.viaAlias && !redirect ? (
          <p
            role="status"
            className="mb-4 flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground"
          >
            <MoveRight className="size-4 shrink-0" aria-hidden />
            已移动至 <span className="font-medium">{page.displayTitle}</span>
          </p>
        ) : null}

        <div className="mb-6 flex flex-col items-stretch gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
          <h1 className="min-w-0 break-words text-3xl font-bold tracking-tight">
            {page.displayTitle}
          </h1>
          <div className="-mx-1 flex w-[calc(100%+0.5rem)] items-center gap-2 overflow-x-auto px-1 pb-1 sm:mx-0 sm:w-auto sm:shrink-0 sm:overflow-visible sm:p-0">
            <Button variant="ghost" size="sm" asChild className="gap-1">
              <Link href={`/pages/${redirectSourceId}/tools`}>
                <Wrench aria-hidden className="size-3.5" />
                工具
              </Link>
            </Button>
            <RedirectPageButton
              sourcePageId={redirectSourceId}
              sourceTitle={redirectSourceTitle}
            />
            <RenamePageButton
              pageId={page.id}
              currentTitle={page.displayTitle}
            />
            <Button variant="outline" size="sm" asChild className="gap-1">
              <Link href={`/pages/${page.id}/edit`}>
                <Pencil aria-hidden className="size-3.5" />
                编辑
              </Link>
            </Button>
          </div>
        </div>

        {terminalRedirect && redirect ? (
          <div className="mb-6 rounded-2xl border border-violet-200 bg-gradient-to-br from-violet-50 to-background p-5">
            <div className="flex items-start gap-3">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-violet-100 text-violet-700">
                {redirect.target.kind === "interwiki" ? (
                  <ExternalLink className="size-4" aria-hidden />
                ) : (
                  <CircleHelp className="size-4" aria-hidden />
                )}
              </span>
              <div className="min-w-0">
                <p className="text-xs font-semibold tracking-[0.15em] text-violet-700 uppercase">
                  {redirect.target.kind === "interwiki"
                    ? "External wiki redirect"
                    : "Unresolved redirect"}
                </p>
                <h2 className="mt-1 text-lg font-semibold">
                  {redirect.target.kind === "interwiki"
                    ? redirect.target.targetTitle
                    : `${redirect.target.namespace ?? "?"}:${
                        redirect.target.targetTitle ?? "?"
                      }`}
                </h2>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {redirect.target.kind === "interwiki"
                    ? "该目标位于站外，外部内容不属于本站权威数据。请核对地址后继续。"
                    : "目标标题目前还没有对应的 Page。创建后，阅读路径会自动解析并继续跟随安全链路。"}
                </p>
                {redirect.target.externalUrl ? (
                  <a
                    href={redirect.target.externalUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="mt-4 inline-flex items-center gap-1.5 text-sm font-semibold text-violet-800 hover:underline"
                  >
                    前往外部 Wiki
                    <ArrowUpRight className="size-4" aria-hidden />
                  </a>
                ) : (
                  <Button asChild variant="outline" size="sm" className="mt-4">
                    <Link
                      href={`/explore?q=${encodeURIComponent(
                        redirect.target.targetTitle ?? "",
                      )}`}
                    >
                      搜索相近页面
                      <ArrowUpRight aria-hidden />
                    </Link>
                  </Button>
                )}
              </div>
            </div>
          </div>
        ) : null}

        {!terminalRedirect ? <TableOfContents entries={toc} /> : null}

        {sectioned && content && !terminalRedirect ? (
          <LazySectionDocument
            pageId={page.id}
            revisionId={content.revision.id}
            expectedSectionCount={content.sectionCount}
            initialBlockId={sectionTarget}
          />
        ) : document && content?.html && !terminalRedirect ? (
          <ServerRenderedDocument
            html={content.html}
            rendererVersion={content.rendererVersion}
          />
        ) : document && !terminalRedirect ? (
          <AstDocument document={document} />
        ) : !terminalRedirect ? (
          <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
            本页面已创建，但尚未发布任何内容。
          </p>
        ) : null}

        {content && !terminalRedirect ? (
          <RevisionInfo revision={content.revision} pageId={page.id} />
        ) : null}
      </article>
      <TocSidebar entries={toc} />
    </div>
  );
}
