"use client";

import Link from "next/link";
import {
  ArrowLeft,
  BarChart3,
  Braces,
  CalendarDays,
  ClipboardList,
  Clock3,
  Database,
  Images,
  LayoutGrid,
  LayoutDashboard,
  List,
  LoaderCircle,
  PanelsTopLeft,
  Plus,
  Rows3,
  SlidersHorizontal,
  Table2,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";

import type {
  Dataset,
  DatasetRecord,
  DatasetRecordPage,
  DatasetView,
  DatasetViewList,
} from "../../../../contracts/generated/typescript";

import { DatasetRecordTable } from "@/components/datasets/dataset-records";
import { DatasetViewCreator } from "@/components/datasets/dataset-view-creator";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { datasetsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { datasetFields, type DatasetField } from "@/lib/datasets";

const PAGE_SIZE = 50;

const VIEW_META = {
  table: { label: "表格", icon: Table2 },
  cards: { label: "卡片", icon: LayoutGrid },
  chart: { label: "聚合图", icon: BarChart3 },
  board: { label: "看板", icon: PanelsTopLeft },
  timeline: { label: "时间轴", icon: Clock3 },
  calendar: { label: "日历", icon: CalendarDays },
  list: { label: "列表", icon: List },
  gallery: { label: "画廊", icon: Images },
  form: { label: "表单", icon: ClipboardList },
  dashboard: { label: "仪表盘", icon: LayoutDashboard },
} as const;

function parseRecordValues(
  fields: DatasetField[],
  rawValues: Record<string, string>,
): Record<string, string | number | boolean> | null {
  const values: Record<string, string | number | boolean> = {};
  for (const field of fields) {
    const raw = rawValues[field.key] ?? "";
    if (field.type === "boolean") {
      if (raw === "") {
        if (field.required) return null;
        continue;
      }
      values[field.key] = raw === "true";
      continue;
    }
    const value = raw.trim();
    if (!value) {
      if (field.required) return null;
      continue;
    }
    if (field.type === "number" || field.type === "integer") {
      const numeric = Number(value);
      if (
        !Number.isFinite(numeric) ||
        (field.type === "integer" && !Number.isInteger(numeric))
      ) {
        return null;
      }
      values[field.key] = numeric;
    } else {
      values[field.key] = value;
    }
  }
  return values;
}

function RecordEditor({
  dataset,
  record,
  onClose,
  onSaved,
}: {
  dataset: Dataset;
  record: DatasetRecord | null;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const fields = datasetFields(dataset);
  const [rawValues, setRawValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      fields.map((field) => {
        const value = record?.values[field.key];
        return [field.key, value == null ? "" : String(value)];
      }),
    ),
  );
  const [entityID, setEntityID] = useState(record?.entityId ?? "");
  const [saving, setSaving] = useState(false);

  const save = async () => {
    const values = parseRecordValues(fields, rawValues);
    if (!values) {
      toast.error("请补齐必填字段，并检查数字格式");
      return;
    }
    setSaving(true);
    try {
      const request = {
        entityId: entityID.trim() || undefined,
        values,
      };
      if (record) {
        await datasetsApi().updateDatasetRecord({
          id: record.id,
          writeDatasetRecordRequest: request,
        });
      } else {
        await datasetsApi().createDatasetRecord({
          id: dataset.id,
          writeDatasetRecordRequest: request,
        });
      }
      await onSaved();
      toast.success(record ? "记录已更新" : "记录已添加");
      onClose();
    } catch {
      toast.error(record ? "更新记录失败" : "新增记录失败", {
        description: "请检查字段格式、Entity ID 与登录状态。",
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{record ? "编辑结构化记录" : "新增结构化记录"}</DialogTitle>
        <DialogDescription>
          保存前会按 {dataset.name} 的 JSON Schema 在服务端重新校验。
        </DialogDescription>
      </DialogHeader>
      <div className="grid max-h-[58vh] gap-4 overflow-y-auto pr-1 sm:grid-cols-2">
        {fields.map((field) => (
          <div key={field.key} className="space-y-2">
            <Label htmlFor={`record-${field.key}`}>
              {field.title}
              {field.required ? <span className="ml-1 text-destructive">*</span> : null}
            </Label>
            {field.type === "boolean" ? (
              <select
                id={`record-${field.key}`}
                value={rawValues[field.key] ?? ""}
                onChange={(event) =>
                  setRawValues((current) => ({
                    ...current,
                    [field.key]: event.target.value,
                  }))
                }
                className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
              >
                {!field.required ? <option value="">未设置</option> : null}
                <option value="true">是</option>
                <option value="false">否</option>
              </select>
            ) : (
              <Input
                id={`record-${field.key}`}
                type={
                  field.type === "number" || field.type === "integer"
                    ? "number"
                    : field.type === "date"
                      ? "date"
                      : "text"
                }
                step={field.type === "integer" ? "1" : "any"}
                value={rawValues[field.key] ?? ""}
                onChange={(event) =>
                  setRawValues((current) => ({
                    ...current,
                    [field.key]: event.target.value,
                  }))
                }
              />
            )}
          </div>
        ))}
        <div className="space-y-2 sm:col-span-2">
          <Label htmlFor="record-entity-id">关联 Entity ID（可选）</Label>
          <Input
            id="record-entity-id"
            value={entityID}
            onChange={(event) => setEntityID(event.target.value)}
            placeholder="UUID"
            className="font-mono text-xs"
          />
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button type="button" disabled={saving} onClick={() => void save()}>
          {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          {record ? "保存修改" : "添加记录"}
        </Button>
      </DialogFooter>
    </>
  );
}

function SavedViews({ views }: { views: DatasetView[] }) {
  if (views.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed p-5 text-sm text-muted-foreground">
        尚未创建保存视图。筛选、排序和分组可以成为可复用、可嵌入的知识界面。
      </div>
    );
  }
  return (
    <ul className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {views.map((view) => {
        const meta = VIEW_META[view.viewType];
        const Icon = meta.icon;
        const groupFields =
          view.config.groups?.map((group) => group.field) ??
          (view.config.groupBy ? [view.config.groupBy] : []);
        return (
          <li key={view.id}>
            <Link
              href={`/datasets/views/${view.id}`}
              className="group flex items-center gap-3 rounded-2xl border bg-card p-4 transition hover:border-primary/25 hover:shadow-[0_8px_24px_rgb(15_23_42/0.06)]"
            >
              <span className="flex size-9 items-center justify-center rounded-xl bg-cyan-100 text-cyan-700">
                <Icon className="size-4" aria-hidden />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold">{view.name}</span>
                <span className="mt-0.5 block text-[11px] text-muted-foreground">
                  {meta.label}
                  {groupFields.length > 0
                    ? ` · 按 ${groupFields.join(" / ")} 分组`
                    : ""}
                </span>
              </span>
            </Link>
          </li>
        );
      })}
    </ul>
  );
}

export function DatasetWorkspace({ id }: { id: string }) {
  const [recordOpen, setRecordOpen] = useState(false);
  const [editing, setEditing] = useState<DatasetRecord | null>(null);
  const [viewOpen, setViewOpen] = useState(false);
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const datasetState = useSWR<Dataset>(["dataset", id], () =>
    datasetsApi().getDataset({ id }),
  );
  const recordState = useSWRInfinite<DatasetRecordPage>(
    (pageIndex, previousPage) => {
      if (!datasetState.data) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "dataset-records",
        id,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (key) => {
      const [, datasetID, cursor] = key as readonly [string, string, string];
      return datasetsApi().listDatasetRecords({
        id: datasetID,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );
  const viewState = useSWR<DatasetViewList>(
    datasetState.data ? ["dataset-views", id] : null,
    () => datasetsApi().listDatasetViews({ id }),
  );

  if (datasetState.isLoading) {
    return <div className="h-72 animate-pulse rounded-3xl border bg-muted/35" />;
  }
  if (datasetState.error || !datasetState.data) {
    return (
      <div className="rounded-3xl border border-destructive/20 bg-destructive/5 p-8">
        <h1 className="text-xl font-semibold">Dataset 无法打开</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          它可能不存在，或当前 API 暂时不可用。
        </p>
        <Button asChild variant="outline" className="mt-5">
          <Link href="/datasets">返回数据目录</Link>
        </Button>
      </div>
    );
  }

  const dataset = datasetState.data;
  const fields = datasetFields(dataset);
  const records = recordState.data?.flatMap((page) => page.items) ?? [];
  const lastRecordPage = recordState.data?.[recordState.data.length - 1];
  const canLoadMore = Boolean(recordState.data && lastRecordPage?.nextCursor);

  return (
    <>
      <header className="border-b border-border/75 pb-7">
        <Button variant="ghost" size="sm" asChild className="-ml-2 mb-5">
          <Link href="/datasets">
            <ArrowLeft aria-hidden />
            数据目录
          </Link>
        </Button>
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div>
            <span className="flex size-11 items-center justify-center rounded-2xl bg-cyan-100 text-cyan-700">
              <Database className="size-5" aria-hidden />
            </span>
            <h1 className="mt-4 text-4xl font-semibold tracking-[-0.045em]">
              {dataset.name}
            </h1>
            <p className="mt-2 font-mono text-[11px] text-muted-foreground">
              {dataset.id}
            </p>
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={!isAuthenticated || sessionLoading}
              onClick={() => setViewOpen(true)}
            >
              <SlidersHorizontal aria-hidden />
              新建视图
            </Button>
            <Button
              type="button"
              disabled={!isAuthenticated || sessionLoading}
              onClick={() => {
                setEditing(null);
                setRecordOpen(true);
              }}
            >
              <Plus aria-hidden />
              添加记录
            </Button>
          </div>
        </div>
        {!isAuthenticated && !sessionLoading ? (
          <p className="mt-4 text-xs text-muted-foreground">
            当前为只读浏览；登录后可新增记录与创建保存视图。
          </p>
        ) : null}
      </header>

      <div className="mt-7 grid gap-4 md:grid-cols-3">
        <div className="rounded-2xl border bg-card p-4">
          <Rows3 className="size-4 text-cyan-700" aria-hidden />
          <p className="mt-3 text-2xl font-semibold">{records.length}</p>
          <p className="mt-1 text-xs text-muted-foreground">当前已加载记录</p>
        </div>
        <div className="rounded-2xl border bg-card p-4">
          <Braces className="size-4 text-cyan-700" aria-hidden />
          <p className="mt-3 text-2xl font-semibold">{fields.length}</p>
          <p className="mt-1 text-xs text-muted-foreground">Schema 字段</p>
        </div>
        <div className="rounded-2xl border bg-card p-4">
          <SlidersHorizontal className="size-4 text-cyan-700" aria-hidden />
          <p className="mt-3 text-2xl font-semibold">
            {viewState.data?.items.length ?? 0}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">保存视图</p>
        </div>
      </div>

      <section className="mt-9" aria-labelledby="saved-views-title">
        <div className="mb-4">
          <h2 id="saved-views-title" className="text-lg font-semibold">
            保存视图
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            每个视图都有固定页面，不会在刷新或重新登录后消失。
          </p>
        </div>
        {viewState.error ? (
          <div className="rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            保存视图暂时无法读取。
          </div>
        ) : (
          <SavedViews views={viewState.data?.items ?? []} />
        )}
      </section>

      <section className="mt-9" aria-labelledby="records-title">
        <div className="mb-4 flex items-end justify-between gap-3">
          <div>
            <h2 id="records-title" className="text-lg font-semibold">
              全部记录
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              默认按创建时间稳定排序；编辑时服务端会再次执行 Schema 校验。
            </p>
          </div>
        </div>
        {recordState.error ? (
          <div className="rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            记录暂时无法读取。
          </div>
        ) : recordState.isLoading && !recordState.data ? (
          <div className="h-56 animate-pulse rounded-2xl border bg-muted/35" />
        ) : (
          <DatasetRecordTable
            fields={fields}
            records={records}
            onEdit={
              isAuthenticated
                ? (record) => {
                    setEditing(record);
                    setRecordOpen(true);
                  }
                : undefined
            }
          />
        )}
        {canLoadMore ? (
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
            加载更多记录
          </Button>
        ) : null}
      </section>

      <Dialog
        open={recordOpen}
        onOpenChange={(open) => {
          setRecordOpen(open);
          if (!open) setEditing(null);
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          {recordOpen ? (
            <RecordEditor
              key={editing?.id ?? "new"}
              dataset={dataset}
              record={editing}
              onClose={() => setRecordOpen(false)}
              onSaved={async () => {
                await recordState.mutate();
              }}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog open={viewOpen} onOpenChange={setViewOpen}>
        <DialogContent className="sm:max-w-2xl">
          {viewOpen ? (
            <DatasetViewCreator
              dataset={dataset}
              views={viewState.data?.items ?? []}
              onClose={() => setViewOpen(false)}
              onSaved={async () => {
                await viewState.mutate();
              }}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}
