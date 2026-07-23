// 阅读页内容视图：/wiki/[title] 与 /pages/[id] 共用。
// 渲染重定向/别名提示条、正文 AST、目录与 Revision 信息条；
// 标题行右侧是「编辑」入口（M2-T03，P0 无认证，不做登录态区分）。
import Link from "next/link";
import { ArrowRight, MoveRight, Pencil } from "lucide-react";

import { AstDocument } from "@/components/ast";
import { RenamePageButton } from "@/components/rename-page-button";
import { RevisionInfo } from "@/components/revision-info";
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
  const document = content ? parseDocument(content.astJson) : null;
  const toc = document ? extractToc(document) : [];

  return (
    <div className="mx-auto flex w-full max-w-5xl gap-8 px-4 py-8">
      <StableAnchorResolver pageId={page.id} />
      <article className="min-w-0 max-w-3xl flex-1">
        {data.redirect ? (
          <p
            role="status"
            className="mb-4 flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground"
          >
            <ArrowRight className="size-4 shrink-0" aria-hidden />
            重定向自 <span className="font-medium">{data.redirect.fromTitle}</span>
          </p>
        ) : null}
        {data.viaAlias && !data.redirect ? (
          <p
            role="status"
            className="mb-4 flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground"
          >
            <MoveRight className="size-4 shrink-0" aria-hidden />
            已移动至 <span className="font-medium">{page.displayTitle}</span>
          </p>
        ) : null}

        <div className="mb-6 flex items-start justify-between gap-4">
          <h1 className="text-3xl font-bold tracking-tight">
            {page.displayTitle}
          </h1>
          <div className="flex shrink-0 items-center gap-2">
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

        <TableOfContents entries={toc} />

        {document ? (
          <AstDocument document={document} />
        ) : (
          <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
            本页面已创建，但尚未发布任何内容。
          </p>
        )}

        {content ? (
          <RevisionInfo revision={content.revision} pageId={page.id} />
        ) : null}
      </article>
      <TocSidebar entries={toc} />
    </div>
  );
}
