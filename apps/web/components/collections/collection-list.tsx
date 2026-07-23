"use client";

import { useState } from "react";
import Link from "next/link";
import { toast } from "sonner";
import useSWRMutation from "swr/mutation";
import type {
  Collection,
  CollectionListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { collectionsApi } from "@/lib/api";

const PAGE_SIZE = 20;

export function ruleSummary(collection: Collection): string {
  if (collection.collectionType === "manual" || !collection.query) {
    return "人工维护";
  }
  if (collection.query.kind === "entity_type") {
    return `实体类型：${collection.query.entityType ?? "未知"}`;
  }
  return `存在属性：${collection.query.property ?? "未知"}`;
}

export function CollectionList({
  initialPage,
}: {
  initialPage: CollectionListPage;
}) {
  const [items, setItems] = useState<Collection[]>(initialPage.items);
  const [nextCursor, setNextCursor] = useState<string | null>(
    initialPage.nextCursor ?? null,
  );
  const { trigger, isMutating } = useSWRMutation(
    "collections:list",
    (_key, { arg: cursor }: { arg: string }) =>
      collectionsApi().listCollections({ cursor, pageSize: PAGE_SIZE }),
  );

  const loadMore = async () => {
    if (!nextCursor) return;
    try {
      const page = await trigger(nextCursor);
      setItems((current) => [...current, ...page.items]);
      setNextCursor(page.nextCursor ?? null);
    } catch {
      toast.error("加载 Collection 失败", { description: "请稍后重试。" });
    }
  };

  if (items.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
        当前还没有 Collection。
      </p>
    );
  }

  return (
    <div className="space-y-4">
      <ol className="grid gap-3 sm:grid-cols-2">
        {items.map((collection) => (
          <li key={collection.id}>
            <Link
              href={`/collections/${collection.id}`}
              className="block h-full rounded-lg border border-border p-4 transition-colors hover:border-foreground/30 hover:bg-muted/40"
            >
              <div className="flex items-start justify-between gap-3">
                <h2 className="font-semibold">{collection.title}</h2>
                <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                  {collection.collectionType === "manual" ? "Manual" : "Rule"}
                </span>
              </div>
              <p className="mt-2 text-sm text-muted-foreground">
                {ruleSummary(collection)}
              </p>
            </Link>
          </li>
        ))}
      </ol>
      {nextCursor ? (
        <Button
          className="w-full"
          variant="outline"
          disabled={isMutating}
          onClick={() => void loadMore()}
        >
          {isMutating ? "加载中…" : "加载更多"}
        </Button>
      ) : null}
    </div>
  );
}
