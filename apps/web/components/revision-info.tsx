// Revision 信息条：页面底部展示当前版本的短 id、创建时间、摘要与历史入口。
import Link from "next/link";
import { History } from "lucide-react";

import type { Revision } from "../../../contracts/generated/typescript";

function formatDateTime(date: Date): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export function RevisionInfo({
  revision,
  pageId,
}: {
  revision: Revision;
  pageId: string;
}) {
  return (
    <footer className="mt-10 flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-border pt-4 text-xs text-muted-foreground">
      <span title={revision.id}>版本 {revision.id.slice(0, 8)}</span>
      <time dateTime={revision.createdAt.toISOString()}>
        {formatDateTime(revision.createdAt)}
      </time>
      {revision.summary ? <span>摘要：{revision.summary}</span> : null}
      <Link
        href={`/pages/${pageId}/history`}
        className="ml-auto inline-flex items-center gap-1 text-blue-600 hover:underline"
      >
        <History className="size-3" aria-hidden />
        查看历史
      </Link>
    </footer>
  );
}
