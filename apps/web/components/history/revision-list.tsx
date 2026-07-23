// Revision 历史列表（M2-T05）：/pages/[id]/history 的客户端主体。
//
// 服务端组件注入第一页（page_size=20）；「加载更多」按 next_cursor 经 SWR
// 翻页追加。每行：短 id、本地化创建时间、actor 短 id、summary（无则占位）、
// is_minor 标记、当前版本徽章；操作：复选两版对比 / 与上一版对比 / 单版查看 / 回滚。
"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import useSWRMutation from "swr/mutation";
import type {
  Revision,
  RevisionListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { historyApi } from "@/lib/api";
import { HISTORY_PAGE_SIZE } from "@/lib/history";

import { RollbackButton } from "./rollback-button";
import { formatDateTime, shortId } from "./utils";

export interface RevisionListProps {
  pageId: string;
  /** 页面当前 Revision（徽章展示）；未发布过为 null。 */
  currentRevisionId: string | null;
  /** 服务端注入的第一页。 */
  initialPage: RevisionListPage;
}

export function RevisionList({
  pageId,
  currentRevisionId,
  initialPage,
}: RevisionListProps) {
  const router = useRouter();
  const [items, setItems] = useState<Revision[]>(initialPage.items);
  // 生成客户端把 nullable 标为非空类型，运行时 next_cursor 仍可能为 null。
  const [nextCursor, setNextCursor] = useState<string | null>(
    initialPage.nextCursor ?? null,
  );
  // 复选对比：最多两个，超出时淘汰最早选中的。
  const [selected, setSelected] = useState<string[]>([]);

  const { trigger, isMutating } = useSWRMutation(
    ["history", pageId],
    (_key, { arg: cursor }: { arg: string }) =>
      historyApi().listRevisions({
        id: pageId,
        cursor,
        pageSize: HISTORY_PAGE_SIZE,
      }),
  );

  const loadMore = async () => {
    if (!nextCursor) return;
    try {
      const page = await trigger(nextCursor);
      setItems((prev) => [...prev, ...page.items]);
      setNextCursor(page.nextCursor ?? null);
    } catch {
      toast.error("加载更多失败", { description: "请稍后重试。" });
    }
  };

  const toggleSelect = (revisionId: string) => {
    setSelected((prev) =>
      prev.includes(revisionId)
        ? prev.filter((x) => x !== revisionId)
        : [...prev.slice(-1), revisionId],
    );
  };

  /** 对比所选：列表按时间倒序，索引大者为旧版（from=base），小者为新版（to=current）。 */
  const compareSelected = () => {
    if (selected.length !== 2) return;
    const indexOf = (rid: string) =>
      items.findIndex((rev) => rev.id === rid);
    const [a, b] = selected.map(indexOf);
    if (a < 0 || b < 0) return;
    const from = items[Math.max(a, b)].id;
    const to = items[Math.min(a, b)].id;
    router.push(`/pages/${pageId}/diff?from=${from}&to=${to}`);
  };

  if (items.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
        本页面尚未发布任何版本。
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3">
        <p className="flex-1 text-xs text-muted-foreground">
          勾选两个版本后对比；或直接使用每行的「与上一版对比」。
        </p>
        <Button
          size="sm"
          variant="outline"
          onClick={compareSelected}
          disabled={selected.length !== 2}
        >
          对比所选（{selected.length}/2）
        </Button>
      </div>

      <ol className="flex flex-col gap-2">
        {items.map((revision, index) => {
          const older = items[index + 1];
          return (
            <li
              key={revision.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-border px-3 py-2"
            >
              <Checkbox
                aria-label={`选择版本 ${shortId(revision.id)}`}
                checked={selected.includes(revision.id)}
                onCheckedChange={() => toggleSelect(revision.id)}
              />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
                  <span className="font-medium" title={revision.id}>
                    版本 {shortId(revision.id)}
                  </span>
                  {revision.id === currentRevisionId ? (
                    <span className="rounded-full bg-blue-500/10 px-2 py-0.5 text-xs text-blue-600">
                      当前版本
                    </span>
                  ) : null}
                  {revision.isMinor ? (
                    <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                      小修改
                    </span>
                  ) : null}
                </div>
                <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
                  <time dateTime={revision.createdAt.toISOString()}>
                    {formatDateTime(revision.createdAt)}
                  </time>
                  <span title={revision.actorId}>
                    编辑者 {shortId(revision.actorId)}
                  </span>
                  <span className="min-w-0 truncate">
                    {revision.summary || "（无摘要）"}
                  </span>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Button variant="ghost" size="sm" asChild>
                  <Link href={`/pages/${pageId}/history/${revision.id}`}>
                    查看
                  </Link>
                </Button>
                {older ? (
                  <Button variant="ghost" size="sm" asChild>
                    <Link
                      href={`/pages/${pageId}/diff?from=${older.id}&to=${revision.id}`}
                    >
                      与上一版对比
                    </Link>
                  </Button>
                ) : null}
                <RollbackButton pageId={pageId} revision={revision} />
              </div>
            </li>
          );
        })}
      </ol>

      {nextCursor ? (
        <Button
          variant="outline"
          onClick={() => void loadMore()}
          disabled={isMutating}
        >
          {isMutating ? "加载中…" : "加载更多"}
        </Button>
      ) : null}
    </div>
  );
}
