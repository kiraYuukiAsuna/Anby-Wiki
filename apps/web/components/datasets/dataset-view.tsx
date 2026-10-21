"use client";

import Link from "next/link";
import {
  ArrowLeft,
  BarChart3,
  CalendarDays,
  Check,
  ClipboardList,
  Clock3,
  Copy,
  Images,
  LayoutGrid,
  LayoutDashboard,
  List,
  LoaderCircle,
  PanelsTopLeft,
  SlidersHorizontal,
  Table2,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";

import type {
  Dataset,
  DatasetRecordPage,
  DatasetView,
} from "../../../../contracts/generated/typescript";

import {
  DatasetDashboard,
} from "@/components/datasets/dataset-dashboard";
import {
  DatasetViewRenderer,
  effectiveGroups,
} from "@/components/datasets/dataset-view-renderer";
import { Button } from "@/components/ui/button";
import { datasetsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { datasetFields } from "@/lib/datasets";

const PAGE_SIZE = 50;

const VIEW_ICON = {
  table: Table2,
  cards: LayoutGrid,
  chart: BarChart3,
  board: PanelsTopLeft,
  timeline: Clock3,
  calendar: CalendarDays,
  list: List,
  gallery: Images,
  form: ClipboardList,
  dashboard: LayoutDashboard,
} as const;

const OPERATOR_LABEL: Record<string, string> = {
  eq: "等于",
  contains: "包含",
  gt: "大于",
  gte: "大于等于",
  lt: "小于",
  lte: "小于等于",
  exists: "已填写",
};

export function DatasetViewPage({ id }: { id: string }) {
  const [copied, setCopied] = useState(false);
  const { isAuthenticated } = useSession();
  const viewState = useSWR<DatasetView>(["dataset-view", id], () =>
    datasetsApi().getDatasetView({ id }),
  );
  const datasetState = useSWR<Dataset>(
    viewState.data ? ["dataset", viewState.data.datasetId] : null,
    () => datasetsApi().getDataset({ id: viewState.data!.datasetId }),
  );
  const recordState = useSWRInfinite<DatasetRecordPage>(
    (pageIndex, previousPage) => {
      if (!viewState.data) return null;
      if (
        viewState.data.viewType === "dashboard" ||
        viewState.data.viewType === "form"
      ) {
        return null;
      }
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "dataset-view-records",
        id,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (key) => {
      const [, viewID, cursor] = key as readonly [string, string, string];
      return datasetsApi().queryDatasetView({
        id: viewID,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );

  if (viewState.isLoading || datasetState.isLoading) {
    return <div className="h-72 animate-pulse rounded-3xl border bg-muted/35" />;
  }
  if (
    viewState.error ||
    datasetState.error ||
    !viewState.data ||
    !datasetState.data
  ) {
    return (
      <div className="rounded-3xl border border-destructive/20 bg-destructive/5 p-8">
        <h1 className="text-xl font-semibold">保存视图无法打开</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          它可能不存在，或当前 API 暂时不可用。
        </p>
        <Button asChild variant="outline" className="mt-5">
          <Link href="/datasets">返回数据目录</Link>
        </Button>
      </div>
    );
  }

  const view = viewState.data;
  const dataset = datasetState.data;
  const allFields = datasetFields(dataset);
  const records = recordState.data?.flatMap((page) => page.items) ?? [];
  const groups = recordState.data?.[0]?.groups ?? [];
  const lastPage = recordState.data?.[recordState.data.length - 1];
  const canLoadMore = Boolean(recordState.data && lastPage?.nextCursor);
  const Icon = VIEW_ICON[view.viewType];
  const viewGroups = effectiveGroups(view);

  const copyID = async () => {
    await navigator.clipboard.writeText(view.id);
    setCopied(true);
    toast.success("DatasetView ID 已复制");
    window.setTimeout(() => setCopied(false), 1600);
  };

  const moveBoardRecord = async (
    record: (typeof records)[number],
    field: string,
    value: unknown,
  ) => {
    try {
      const values = { ...record.values };
      if (value == null || value === "") delete values[field];
      else values[field] = value;
      await datasetsApi().updateDatasetRecord({
        id: record.id,
        writeDatasetRecordRequest: {
          entityId: record.entityId ?? undefined,
          values,
        },
      });
      await recordState.mutate();
      toast.success("记录已移动");
    } catch {
      toast.error("移动记录失败", {
        description: "目标组值必须通过 Dataset Schema 校验。",
      });
    }
  };

  return (
    <>
      <header className="border-b border-border/75 pb-7">
        <Button variant="ghost" size="sm" asChild className="-ml-2 mb-5">
          <Link href={`/datasets/${dataset.id}`}>
            <ArrowLeft aria-hidden />
            {dataset.name}
          </Link>
        </Button>
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div>
            <span className="flex size-11 items-center justify-center rounded-2xl bg-cyan-100 text-cyan-700">
              <Icon className="size-5" aria-hidden />
            </span>
            <p className="mt-4 text-xs font-semibold tracking-[0.16em] text-cyan-700 uppercase">
              Saved dataset view
            </p>
            <h1 className="mt-1 text-4xl font-semibold tracking-[-0.045em]">
              {view.name}
            </h1>
          </div>
          <Button type="button" variant="outline" onClick={() => void copyID()}>
            {copied ? <Check aria-hidden /> : <Copy aria-hidden />}
            复制嵌入 ID
          </Button>
        </div>
        <div className="mt-5 flex flex-wrap gap-2 text-xs text-muted-foreground">
          {view.config.filter ? (
            <span className="rounded-full border bg-card px-3 py-1.5">
              {view.config.filter.conditions
                ? `高级筛选：${view.config.filter.conditions.length} 组`
                : `筛选：${view.config.filter.field ?? ""} ${
                    view.config.filter.operator
                      ? (OPERATOR_LABEL[view.config.filter.operator] ??
                        view.config.filter.operator)
                      : ""
                  }`}
            </span>
          ) : null}
          {(view.config.sorts?.length ?? 0) > 0 || view.config.sort ? (
            <span className="rounded-full border bg-card px-3 py-1.5">
              排序链：
              {view.config.sorts?.length ?? (view.config.sort ? 1 : 0)} 项
            </span>
          ) : null}
          {viewGroups.length > 0 ? (
            <span className="rounded-full border bg-card px-3 py-1.5">
              分组：{viewGroups.map((group) => group.field).join(" / ")}
            </span>
          ) : null}
          {!view.config.filter &&
          !view.config.sort &&
          !view.config.sorts?.length &&
          viewGroups.length === 0 ? (
            <span className="flex items-center gap-1.5 rounded-full border bg-card px-3 py-1.5">
              <SlidersHorizontal className="size-3.5" aria-hidden />
              无附加查询条件
            </span>
          ) : null}
        </div>
      </header>

      <section className="mt-7" aria-label="保存视图结果">
        {view.viewType === "dashboard" ? (
          <DatasetDashboard view={view} fields={allFields} />
        ) : recordState.error ? (
          <div className="rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            视图查询暂时失败。
          </div>
        ) : recordState.isLoading &&
          !recordState.data &&
          view.viewType !== "form" ? (
          <div className="h-56 animate-pulse rounded-2xl border bg-muted/35" />
        ) : (
          <DatasetViewRenderer
            view={view}
            fields={allFields}
            records={records}
            groupCounts={groups}
            onBoardMove={
              view.viewType === "board" && isAuthenticated
                ? moveBoardRecord
                : undefined
            }
          />
        )}
        {canLoadMore &&
        view.viewType !== "chart" &&
        view.viewType !== "dashboard" &&
        view.viewType !== "form" ? (
          <Button
            type="button"
            variant="outline"
            className="mt-4 w-full"
            disabled={recordState.isValidating}
            onClick={() => void recordState.setSize(recordState.size + 1)}
          >
            {recordState.isValidating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : null}
            加载更多结果
          </Button>
        ) : null}
      </section>

      <aside className="mt-8 rounded-2xl border bg-muted/25 p-5">
        <p className="text-xs font-semibold">嵌入百科页面</p>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          在 Typed Block AST 的 dataset_view Block 中使用上方复制的稳定 ID；
          页面 Revision 会保留对这个视图的显式引用。
        </p>
        <code className="mt-3 block overflow-x-auto rounded-xl bg-background p-3 text-[11px]">
          {`{"type":"dataset_view","view_id":"${view.id}"}`}
        </code>
      </aside>
    </>
  );
}
