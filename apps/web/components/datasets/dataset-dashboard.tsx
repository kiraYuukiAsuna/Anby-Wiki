"use client";

import { useMemo, useState } from "react";

import { LayoutDashboard, LoaderCircle } from "lucide-react";
import useSWR from "swr";

import type {
  DatasetRecordPage,
  DatasetView,
  DatasetViewFilter,
} from "../../../../contracts/generated/typescript";

import { DatasetViewRenderer } from "@/components/datasets/dataset-view-renderer";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { datasetsApi } from "@/lib/api";
import type { DatasetField } from "@/lib/datasets";

function runtimeFilter(
  fields: DatasetField[],
  values: Record<string, string>,
): DatasetViewFilter | undefined {
  const conditions = fields.flatMap((field) => {
    const value = values[field.key]?.trim();
    if (!value) return [];
    return [
      {
        field: field.key,
        operator: field.type === "string" ? "contains" : "eq",
        value:
          field.type === "number" || field.type === "integer"
            ? Number(value)
            : field.type === "boolean"
              ? value === "true"
              : value,
      } satisfies DatasetViewFilter,
    ];
  });
  if (conditions.length === 0) return undefined;
  if (conditions.length === 1) return conditions[0];
  return { logic: "and", conditions };
}

function DashboardPanel({
  viewID,
  fields,
  filter,
}: {
  viewID: string;
  fields: DatasetField[];
  filter?: DatasetViewFilter;
}) {
  const viewState = useSWR<DatasetView>(["dashboard-view", viewID], () =>
    datasetsApi().getDatasetView({ id: viewID }),
  );
  const filterKey = JSON.stringify(filter ?? null);
  const recordState = useSWR<DatasetRecordPage>(
    viewState.data ? ["dashboard-view-records", viewID, filterKey] : null,
    () =>
      filter
        ? datasetsApi().queryDatasetViewWithFilter({
            id: viewID,
            datasetViewRuntimeQuery: { filter },
            pageSize: 100,
          })
        : datasetsApi().queryDatasetView({ id: viewID, pageSize: 100 }),
  );

  if (viewState.error || recordState.error) {
    return (
      <section className="rounded-md border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
        子视图暂时无法读取。
      </section>
    );
  }
  if (!viewState.data || !recordState.data) {
    return (
      <section className="flex min-h-40 items-center justify-center rounded-md border">
        <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
      </section>
    );
  }

  return (
    <section className="min-w-0 border-t pt-3">
      <header>
        <p className="truncate text-sm font-semibold">{viewState.data.name}</p>
      </header>
      <div className="mt-3 min-w-0 overflow-hidden">
        <DatasetViewRenderer
          view={viewState.data}
          fields={fields}
          records={recordState.data.items}
          groupCounts={recordState.data.groups}
        />
      </div>
    </section>
  );
}

export function DatasetDashboard({
  view,
  fields,
}: {
  view: DatasetView;
  fields: DatasetField[];
}) {
  const filterFields = useMemo(() => {
    const configured = view.config.globalFilterFields;
    return configured && configured.size > 0
      ? fields.filter((field) => configured.has(field.key))
      : [];
  }, [fields, view.config.globalFilterFields]);
  const [values, setValues] = useState<Record<string, string>>({});
  const filter = runtimeFilter(filterFields, values);
  const columns = view.config.layoutColumns ?? 2;
  const gridClass =
    columns === 1
      ? "grid-cols-1"
      : columns === 3
        ? "lg:grid-cols-3"
        : "lg:grid-cols-2";

  return (
    <div className="space-y-5">
      {filterFields.length > 0 ? (
        <section className="border-b pb-5" aria-label="仪表盘全局筛选">
          <div className="flex items-center gap-2 text-sm font-medium">
            <LayoutDashboard className="size-4 text-muted-foreground" />
            全局筛选
          </div>
          <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {filterFields.map((field) => (
              <div key={field.key} className="space-y-1.5">
                <Label htmlFor={`dashboard-filter-${field.key}`}>
                  {field.title}
                </Label>
                {field.type === "boolean" ? (
                  <select
                    id={`dashboard-filter-${field.key}`}
                    value={values[field.key] ?? ""}
                    onChange={(event) =>
                      setValues((current) => ({
                        ...current,
                        [field.key]: event.target.value,
                      }))
                    }
                    className="h-8 w-full rounded-lg border bg-background px-2 text-sm"
                  >
                    <option value="">全部</option>
                    <option value="true">是</option>
                    <option value="false">否</option>
                  </select>
                ) : (
                  <Input
                    id={`dashboard-filter-${field.key}`}
                    type={
                      field.type === "date"
                        ? "date"
                        : field.type === "number" || field.type === "integer"
                          ? "number"
                          : "text"
                    }
                    value={values[field.key] ?? ""}
                    onChange={(event) =>
                      setValues((current) => ({
                        ...current,
                        [field.key]: event.target.value,
                      }))
                    }
                  />
                )}
              </div>
            ))}
          </div>
        </section>
      ) : null}

      <div className={`grid min-w-0 gap-4 ${gridClass}`}>
        {Array.from(view.config.viewIds ?? []).map((viewID) => (
          <DashboardPanel
            key={viewID}
            viewID={viewID}
            fields={fields}
            filter={filter}
          />
        ))}
      </div>
    </div>
  );
}
