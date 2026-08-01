"use client";

import Link from "next/link";
import {
  ArrowLeft,
  BarChart3,
  Braces,
  Database,
  LayoutGrid,
  LoaderCircle,
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
  CreateDatasetViewRequest,
  Dataset,
  DatasetRecord,
  DatasetRecordPage,
  DatasetView,
  DatasetViewFilter,
  DatasetViewList,
} from "../../../../contracts/generated/typescript";

import { DatasetRecordTable } from "@/components/datasets/dataset-records";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
} as const;

type ViewType = CreateDatasetViewRequest["viewType"];
type FilterOperator = DatasetViewFilter["operator"];

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

function parseFilterValue(field: DatasetField | undefined, value: string): unknown {
  if (field?.type === "number" || field?.type === "integer") {
    return Number(value);
  }
  if (field?.type === "boolean") {
    return value === "true";
  }
  return value;
}

function ViewCreator({
  dataset,
  onClose,
  onSaved,
}: {
  dataset: Dataset;
  onClose: () => void;
  onSaved: (view: DatasetView) => Promise<void>;
}) {
  const fields = datasetFields(dataset);
  const [name, setName] = useState("");
  const [viewType, setViewType] = useState<ViewType>("table");
  const [columns, setColumns] = useState(() => new Set(fields.map((field) => field.key)));
  const [filterField, setFilterField] = useState("");
  const [filterOperator, setFilterOperator] =
    useState<FilterOperator>("contains");
  const [filterValue, setFilterValue] = useState("");
  const [sortField, setSortField] = useState("");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("asc");
  const [groupBy, setGroupBy] = useState("");
  const [saving, setSaving] = useState(false);

  const save = async () => {
    if (!name.trim() || columns.size === 0) {
      toast.error("请填写视图名称并至少选择一列");
      return;
    }
    if (viewType === "chart" && !groupBy) {
      toast.error("聚合图需要选择分组字段");
      return;
    }
    if (filterField && filterOperator !== "exists" && !filterValue.trim()) {
      toast.error("请填写筛选值");
      return;
    }
    const filterSource = fields.find((field) => field.key === filterField);
    const config: CreateDatasetViewRequest["config"] = {
      columns,
      filter: filterField
        ? {
            field: filterField,
            operator: filterOperator,
            value:
              filterOperator === "exists"
                ? undefined
                : parseFilterValue(filterSource, filterValue),
          }
        : undefined,
      sort: sortField
        ? { field: sortField, direction: sortDirection }
        : undefined,
      groupBy: groupBy || undefined,
    };
    setSaving(true);
    try {
      const view = await datasetsApi().createDatasetView({
        id: dataset.id,
        createDatasetViewRequest: {
          name: name.trim(),
          viewType,
          config,
        },
      });
      await onSaved(view);
      toast.success("保存视图已创建");
      onClose();
    } catch {
      toast.error("创建视图失败", {
        description: "请检查筛选、排序与分组字段。",
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>创建保存视图</DialogTitle>
        <DialogDescription>
          保存后的视图拥有稳定 ID，可从数据中心重新进入，也可嵌入页面。
        </DialogDescription>
      </DialogHeader>
      <div className="max-h-[65vh] space-y-5 overflow-y-auto pr-1">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="view-name">视图名称</Label>
            <Input
              id="view-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="例如：按年份查看"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="view-type">呈现方式</Label>
            <select
              id="view-type"
              value={viewType}
              onChange={(event) => setViewType(event.target.value as ViewType)}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
            >
              <option value="table">表格</option>
              <option value="cards">卡片</option>
              <option value="chart">聚合图</option>
            </select>
          </div>
        </div>

        <fieldset>
          <legend className="text-sm font-medium">显示列</legend>
          <div className="mt-2 grid gap-2 rounded-xl border bg-muted/20 p-3 sm:grid-cols-2">
            {fields.map((field) => (
              <label key={field.key} className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={columns.has(field.key)}
                  onCheckedChange={(checked) =>
                    setColumns((current) => {
                      const next = new Set(current);
                      if (checked === true) next.add(field.key);
                      else next.delete(field.key);
                      return next;
                    })
                  }
                />
                {field.title}
              </label>
            ))}
          </div>
        </fieldset>

        <div>
          <div className="flex items-center gap-2 text-sm font-medium">
            <SlidersHorizontal className="size-4 text-muted-foreground" aria-hidden />
            筛选
          </div>
          <div className="mt-2 grid gap-2 sm:grid-cols-3">
            <select
              value={filterField}
              onChange={(event) => setFilterField(event.target.value)}
              className="h-8 rounded-lg border border-input bg-background px-2.5 text-sm"
              aria-label="筛选字段"
            >
              <option value="">不筛选</option>
              {fields.map((field) => (
                <option key={field.key} value={field.key}>
                  {field.title}
                </option>
              ))}
            </select>
            <select
              value={filterOperator}
              onChange={(event) =>
                setFilterOperator(event.target.value as FilterOperator)
              }
              disabled={!filterField}
              className="h-8 rounded-lg border border-input bg-background px-2.5 text-sm disabled:opacity-50"
              aria-label="筛选操作"
            >
              <option value="contains">包含</option>
              <option value="eq">等于</option>
              <option value="gt">大于</option>
              <option value="gte">大于等于</option>
              <option value="lt">小于</option>
              <option value="lte">小于等于</option>
              <option value="exists">已填写</option>
            </select>
            <Input
              value={filterValue}
              onChange={(event) => setFilterValue(event.target.value)}
              disabled={!filterField || filterOperator === "exists"}
              placeholder="筛选值"
              aria-label="筛选值"
            />
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="sort-field">排序</Label>
            <div className="grid grid-cols-[1fr_5.5rem] gap-2">
              <select
                id="sort-field"
                value={sortField}
                onChange={(event) => setSortField(event.target.value)}
                className="h-8 rounded-lg border border-input bg-background px-2.5 text-sm"
              >
                <option value="">默认顺序</option>
                {fields.map((field) => (
                  <option key={field.key} value={field.key}>
                    {field.title}
                  </option>
                ))}
              </select>
              <select
                value={sortDirection}
                onChange={(event) =>
                  setSortDirection(event.target.value as "asc" | "desc")
                }
                disabled={!sortField}
                className="h-8 rounded-lg border border-input bg-background px-2 text-sm disabled:opacity-50"
                aria-label="排序方向"
              >
                <option value="asc">升序</option>
                <option value="desc">降序</option>
              </select>
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="group-field">分组</Label>
            <select
              id="group-field"
              value={groupBy}
              onChange={(event) => setGroupBy(event.target.value)}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
            >
              <option value="">不分组</option>
              {fields.map((field) => (
                <option key={field.key} value={field.key}>
                  {field.title}
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button type="button" disabled={saving} onClick={() => void save()}>
          {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          保存视图
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
                  {view.config.groupBy ? ` · 按 ${view.config.groupBy} 分组` : ""}
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
            <ViewCreator
              dataset={dataset}
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
