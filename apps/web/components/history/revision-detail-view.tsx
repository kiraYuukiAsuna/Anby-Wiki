// 单版详情视图（M2-T05）：/pages/[id]/history/[rid] 的展示主体。
// Revision 元信息 + 阅读渲染器预览，失败时保留 AST 降级与回滚入口。
import type { RevisionDetail } from "../../../../contracts/generated/typescript";

import { ServerRenderedDocument } from "@/components/server-rendered-document";
import { AstDocument } from "@/components/ast";
import { parseDocument } from "@/lib/ast/schema";

import { RollbackButton } from "./rollback-button";
import {
  formatDateTime,
  revisionAuthorLabel,
  revisionAuthorMeta,
  revisionLabel,
} from "./utils";

export function RevisionDetailView({
  pageId,
  detail,
  currentRevisionId,
}: {
  pageId: string;
  detail: RevisionDetail;
  currentRevisionId: string | null;
}) {
  const { revision } = detail;
  // 服务端返回的 canonical AST 视为可信（与阅读页同一约定）；结构非法时 parseDocument 抛 ZodError。
  const document = parseDocument(detail.astJson);

  return (
    <article>
      <header className="mb-6 flex flex-wrap items-start justify-between gap-3 border-b border-border pb-4">
        <div className="min-w-0">
          <h2 className="text-xl font-semibold" title={revision.id}>
            {revisionLabel(revision)}
            {revision.id === currentRevisionId ? (
              <span className="ml-2 rounded-full bg-blue-500/10 px-2 py-0.5 align-middle text-xs font-normal text-blue-600">
                当前版本
              </span>
            ) : null}
          </h2>
          <dl className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
            <div className="flex gap-1">
              <dt>创建时间</dt>
              <dd>
                <time dateTime={revision.createdAt.toISOString()}>
                  {formatDateTime(revision.createdAt)}
                </time>
              </dd>
            </div>
            <div className="flex gap-1">
              <dt>编辑者</dt>
              <dd title={`身份 ${revision.actorId}`}>
                {revisionAuthorLabel(revision)}
                {revisionAuthorMeta(revision)
                  ? ` · ${revisionAuthorMeta(revision)}`
                  : ""}
              </dd>
            </div>
            <div className="flex gap-1">
              <dt>摘要</dt>
              <dd>{revision.summary || "（无摘要）"}</dd>
            </div>
            {revision.isMinor ? (
              <div className="flex gap-1">
                <dt>标记</dt>
                <dd>小修改</dd>
              </div>
            ) : null}
            {revision.parentRevisionId ? (
              <div className="flex gap-1">
                <dt>父版本</dt>
                <dd title={revision.parentRevisionId}>
                  已关联
                </dd>
              </div>
            ) : null}
          </dl>
        </div>
        <RollbackButton pageId={pageId} revision={revision} />
      </header>

      {detail.html !== undefined ? <>
        <p className="mb-4 text-xs text-muted-foreground">引用数据按当前状态展示。</p>
        <ServerRenderedDocument html={detail.html} rendererVersion={detail.rendererVersion ?? ""} />
      </> : <AstDocument document={document} />}
    </article>
  );
}
