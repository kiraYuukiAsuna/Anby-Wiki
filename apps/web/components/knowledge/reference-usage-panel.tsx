"use client";

import { LoaderCircle } from "lucide-react";
import { useMemo } from "react";
import useSWRInfinite from "swr/infinite";

import type { ReferenceUsageListPage } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { projectionApi } from "@/lib/api";

import {
  groupReferenceUsagesByPage,
  PageGroupedUsageList,
} from "./usage-list";

const PAGE_SIZE = 50;

type ReferenceUsageKind = "entity" | "claim" | "citation";

function fetchUsagePage(
  kind: ReferenceUsageKind,
  id: string,
  cursor: string,
): Promise<ReferenceUsageListPage> {
  const request = {
    id,
    cursor: cursor || undefined,
    pageSize: PAGE_SIZE,
  };
  const api = projectionApi();
  switch (kind) {
    case "entity":
      return api.listEntityMentions(request);
    case "claim":
      return api.listClaimUsages(request);
    case "citation":
      return api.listCitationUsages(request);
  }
}

export function ReferenceUsagePanel({
  kind,
  targetId,
  initialPage,
}: {
  kind: ReferenceUsageKind;
  targetId: string;
  initialPage: ReferenceUsageListPage;
}) {
  const state = useSWRInfinite<ReferenceUsageListPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "projection:reference-usages",
        kind,
        targetId,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, usageKind, id, cursor] = cacheKey as readonly [
        string,
        ReferenceUsageKind,
        string,
        string,
      ];
      return fetchUsagePage(usageKind, id, cursor);
    },
    {
      fallbackData: [initialPage],
      revalidateFirstPage: false,
      revalidateOnFocus: false,
    },
  );

  const items = useMemo(
    () => state.data?.flatMap((page) => page.items) ?? initialPage.items,
    [initialPage.items, state.data],
  );
  const groups = useMemo(() => groupReferenceUsagesByPage(items), [items]);
  const lastPage = state.data?.at(-1) ?? initialPage;
  const occurrenceLabel = kind === "entity" ? "提及" : "引用";
  const loadingMore =
    state.isValidating && state.size > (state.data?.length ?? 0);

  return (
    <>
      <PageGroupedUsageList
        groups={groups}
        occurrenceLabel={occurrenceLabel}
        showClaimContexts={kind === "citation"}
      />
      {state.error ? (
        <p className="mt-3 text-xs text-destructive">
          更多{occurrenceLabel}位置加载失败，已保留当前结果，请稍后重试。
        </p>
      ) : null}
      {lastPage.nextCursor ? (
        <Button
          type="button"
          variant="outline"
          className="mt-4 w-full"
          disabled={loadingMore}
          onClick={() => void state.setSize(state.size + 1)}
        >
          {loadingMore ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          加载更多位置（已加载 {items.length} / {initialPage.totalUsageCount}）
        </Button>
      ) : null}
    </>
  );
}
