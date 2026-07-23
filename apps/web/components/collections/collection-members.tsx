"use client";

import { useState } from "react";
import Link from "next/link";
import { toast } from "sonner";
import useSWRMutation from "swr/mutation";
import type {
  CollectionMembership,
  CollectionMembershipListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { collectionsApi } from "@/lib/api";

const PAGE_SIZE = 20;

function memberHref(member: CollectionMembership): string {
  return member.memberType === "page"
    ? `/pages/${member.pageId}`
    : `/entities/${member.entityId}`;
}

export function CollectionMembers({
  collectionId,
  initialPage,
}: {
  collectionId: string;
  initialPage: CollectionMembershipListPage;
}) {
  const [items, setItems] = useState<CollectionMembership[]>(
    initialPage.items,
  );
  const [nextCursor, setNextCursor] = useState<string | null>(
    initialPage.nextCursor ?? null,
  );
  const { trigger, isMutating } = useSWRMutation(
    ["collections:members", collectionId],
    (_key, { arg: cursor }: { arg: string }) =>
      collectionsApi().listCollectionMembers({
        id: collectionId,
        cursor,
        pageSize: PAGE_SIZE,
      }),
  );

  const loadMore = async () => {
    if (!nextCursor) return;
    try {
      const page = await trigger(nextCursor);
      setItems((current) => [...current, ...page.items]);
      setNextCursor(page.nextCursor ?? null);
    } catch {
      toast.error("加载成员失败", { description: "请稍后重试。" });
    }
  };

  if (items.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
        当前没有物化成员。
      </p>
    );
  }

  return (
    <div className="space-y-4">
      <ol className="divide-y divide-border rounded-lg border border-border">
        {items.map((member) => {
          const targetId =
            member.memberType === "page" ? member.pageId : member.entityId;
          return (
            <li
              key={`${member.memberType}:${targetId}`}
              className="flex flex-wrap items-center gap-x-4 gap-y-2 p-4"
            >
              <div className="min-w-0 flex-1">
                <Link
                  href={memberHref(member)}
                  className="font-medium text-blue-600 hover:underline"
                >
                  {member.displayTitle}
                </Link>
                <p className="mt-1 text-xs text-muted-foreground">
                  {member.memberType === "page" ? "Page" : "Entity"} · 排序键{" "}
                  {member.sortKey}
                </p>
              </div>
              <div className="text-right text-xs text-muted-foreground">
                <p>{member.sourceType === "manual" ? "人工来源" : "规则来源"}</p>
                <p className="font-mono" title={member.sourceRevisionId}>
                  Revision {member.sourceRevisionId.slice(0, 8)}
                </p>
              </div>
            </li>
          );
        })}
      </ol>
      {nextCursor ? (
        <Button
          className="w-full"
          variant="outline"
          disabled={isMutating}
          onClick={() => void loadMore()}
        >
          {isMutating ? "加载中…" : "加载更多成员"}
        </Button>
      ) : null}
    </div>
  );
}
