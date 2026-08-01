"use client";

import { BarChart3, Pencil, Rows3 } from "lucide-react";

import type {
  DatasetGroupCount,
  DatasetRecord,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  displayDatasetValue,
  type DatasetField,
} from "@/lib/datasets";

export function DatasetRecordTable({
  fields,
  records,
  onEdit,
}: {
  fields: DatasetField[];
  records: DatasetRecord[];
  onEdit?: (record: DatasetRecord) => void;
}) {
  if (records.length === 0) {
    return <DatasetEmptyState />;
  }

  return (
    <div className="overflow-hidden rounded-2xl border bg-card">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[42rem] border-collapse text-left text-sm">
          <thead className="border-b bg-muted/45 text-xs text-muted-foreground">
            <tr>
              {fields.map((field) => (
                <th key={field.key} scope="col" className="px-4 py-3 font-medium">
                  {field.title}
                </th>
              ))}
              {onEdit ? (
                <th scope="col" className="w-20 px-4 py-3 text-right font-medium">
                  操作
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody className="divide-y divide-border/65">
            {records.map((record) => (
              <tr key={record.id} className="transition-colors hover:bg-muted/25">
                {fields.map((field) => (
                  <td key={field.key} className="max-w-72 px-4 py-3">
                    <span className="line-clamp-2">
                      {displayDatasetValue(record.values[field.key])}
                    </span>
                  </td>
                ))}
                {onEdit ? (
                  <td className="px-4 py-3 text-right">
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      onClick={() => onEdit(record)}
                      aria-label="编辑记录"
                    >
                      <Pencil aria-hidden />
                    </Button>
                  </td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function DatasetRecordCards({
  fields,
  records,
}: {
  fields: DatasetField[];
  records: DatasetRecord[];
}) {
  if (records.length === 0) {
    return <DatasetEmptyState />;
  }

  return (
    <ul className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {records.map((record) => (
        <li
          key={record.id}
          className="rounded-2xl border bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)]"
        >
          <dl className="space-y-3">
            {fields.map((field) => (
              <div key={field.key}>
                <dt className="text-[11px] font-semibold tracking-[0.08em] text-muted-foreground uppercase">
                  {field.title}
                </dt>
                <dd className="mt-1 break-words text-sm">
                  {displayDatasetValue(record.values[field.key])}
                </dd>
              </div>
            ))}
          </dl>
        </li>
      ))}
    </ul>
  );
}

export function DatasetGroupChart({
  groups,
  label,
}: {
  groups: DatasetGroupCount[];
  label: string;
}) {
  if (groups.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed px-6 py-12 text-center">
        <BarChart3 className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <p className="mt-3 text-sm font-medium">没有可聚合的数据</p>
      </div>
    );
  }
  const maximum = Math.max(...groups.map((group) => group.count), 1);

  return (
    <div className="rounded-2xl border bg-card p-5">
      <p className="mb-5 text-xs font-semibold tracking-[0.12em] text-muted-foreground uppercase">
        按 {label} 聚合
      </p>
      <ol className="space-y-4">
        {groups.map((group) => (
          <li key={group.value} className="grid grid-cols-[8rem_1fr_3rem] items-center gap-3">
            <span className="truncate text-sm" title={group.value}>
              {group.value}
            </span>
            <span className="h-2 overflow-hidden rounded-full bg-muted">
              <span
                className="block h-full rounded-full bg-gradient-to-r from-cyan-500 to-primary"
                style={{ width: `${Math.max((group.count / maximum) * 100, 2)}%` }}
              />
            </span>
            <span className="text-right font-mono text-xs text-muted-foreground">
              {group.count}
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}

function DatasetEmptyState() {
  return (
    <div className="rounded-2xl border border-dashed px-6 py-14 text-center">
      <Rows3 className="mx-auto size-8 text-muted-foreground" aria-hidden />
      <p className="mt-3 text-sm font-medium">当前结果没有记录</p>
      <p className="mt-1 text-xs text-muted-foreground">
        新增记录或调整保存视图的筛选条件。
      </p>
    </div>
  );
}
