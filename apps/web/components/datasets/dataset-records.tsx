"use client";

import { useMemo, useState } from "react";

import {
  BarChart3,
  ChevronLeft,
  ChevronRight,
  Clock3,
  GripVertical,
  ImageIcon,
  List,
  Pencil,
} from "lucide-react";

import type {
  DatasetGroupCount,
  DatasetRecord,
  DatasetViewGroup,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  displayDatasetValue,
  type DatasetField,
} from "@/lib/datasets";
import { safeHttpUrl } from "@/lib/http-url";

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

function recordTitle(
  record: DatasetRecord,
  fields: DatasetField[],
  titleField?: string,
): string {
  const field = titleField || fields[0]?.key;
  return field
    ? displayDatasetValue(record.values[field])
    : record.id.slice(0, 8);
}

export function DatasetRecordList({
  fields,
  records,
  titleField,
}: {
  fields: DatasetField[];
  records: DatasetRecord[];
  titleField?: string;
}) {
  if (records.length === 0) return <DatasetEmptyState />;
  const detailFields = fields.filter((field) => field.key !== titleField);
  return (
    <ol className="divide-y rounded-md border bg-card">
      {records.map((record, index) => (
        <li
          key={record.id}
          className="grid min-w-0 grid-cols-[2rem_minmax(0,1fr)] gap-3 px-4 py-3"
        >
          <span className="pt-0.5 font-mono text-xs text-muted-foreground">
            {String(index + 1).padStart(2, "0")}
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold">
              {recordTitle(record, fields, titleField)}
            </p>
            <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
              {detailFields
                .map((field) => displayDatasetValue(record.values[field.key]))
                .filter((value) => value !== "—")
                .join(" · ")}
            </p>
          </div>
        </li>
      ))}
    </ol>
  );
}

export function DatasetRecordGallery({
  fields,
  records,
  titleField,
  descriptionField,
  coverField,
  cardSize = "medium",
}: {
  fields: DatasetField[];
  records: DatasetRecord[];
  titleField?: string;
  descriptionField?: string;
  coverField?: string;
  cardSize?: "small" | "medium" | "large";
}) {
  if (records.length === 0) return <DatasetEmptyState />;
  const gridClass =
    cardSize === "small"
      ? "sm:grid-cols-2 lg:grid-cols-4"
      : cardSize === "large"
        ? "lg:grid-cols-2"
        : "sm:grid-cols-2 xl:grid-cols-3";
  return (
    <ul className={`grid gap-4 ${gridClass}`}>
      {records.map((record) => {
        const cover =
          coverField && typeof record.values[coverField] === "string"
            ? safeHttpUrl(String(record.values[coverField]))
            : null;
        return (
          <li key={record.id} className="min-w-0 overflow-hidden rounded-md border bg-card">
            <div className="flex aspect-[16/9] items-center justify-center overflow-hidden bg-muted">
              {cover ? (
                // Runtime Dataset content is user-authored media, not a bundled site asset.
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={cover}
                  alt=""
                  className="size-full object-cover"
                  loading="lazy"
                  referrerPolicy="no-referrer"
                />
              ) : (
                <ImageIcon className="size-7 text-muted-foreground" aria-hidden />
              )}
            </div>
            <div className="p-4">
              <p className="truncate text-sm font-semibold">
                {recordTitle(record, fields, titleField)}
              </p>
              {descriptionField ? (
                <p className="mt-2 line-clamp-3 text-xs leading-5 text-muted-foreground">
                  {displayDatasetValue(record.values[descriptionField])}
                </p>
              ) : null}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function groupValues(
  records: DatasetRecord[],
  group: DatasetViewGroup,
): unknown[] {
  const values = [...(group.options ?? [])];
  for (const record of records) {
    const value = record.values[group.field];
    if (
      !values.some(
        (current) => JSON.stringify(current) === JSON.stringify(value),
      )
    ) {
      values.push(value);
    }
  }
  const filtered = values.filter(
    (value) => !group.hideEmpty || (value != null && String(value) !== ""),
  );
  const direction = group.sortDirection === "desc" ? -1 : 1;
  return filtered.sort(
    (left, right) =>
      displayDatasetValue(left).localeCompare(displayDatasetValue(right)) *
      direction,
  );
}

function sameGroup(left: unknown, right: unknown): boolean {
  return JSON.stringify(left ?? null) === JSON.stringify(right ?? null);
}

export function DatasetBoard({
  fields,
  records,
  groups,
  titleField,
  onMove,
}: {
  fields: DatasetField[];
  records: DatasetRecord[];
  groups: DatasetViewGroup[];
  titleField?: string;
  onMove?: (
    record: DatasetRecord,
    field: string,
    value: unknown,
  ) => Promise<void>;
}) {
  const [draggedID, setDraggedID] = useState<string | null>(null);
  const primary = groups[0];
  const secondary = groups[1];
  if (!primary) return <DatasetEmptyState />;
  const columns = groupValues(records, primary);
  if (columns.length === 0) return <DatasetEmptyState />;
  const moveDragged = async (value: unknown) => {
    const record = records.find((item) => item.id === draggedID);
    if (record && onMove) await onMove(record, primary.field, value);
    setDraggedID(null);
  };

  return (
    <div className="grid min-w-0 gap-3 lg:grid-cols-[repeat(auto-fit,minmax(15rem,1fr))]">
      {columns.map((value) => {
        const columnRecords = records.filter((record) =>
          sameGroup(record.values[primary.field], value),
        );
        const subgroups = secondary
          ? groupValues(columnRecords, secondary)
          : [undefined];
        return (
          <details
            key={JSON.stringify(value)}
            open={primary.display !== "collapsed"}
            className="min-w-0 rounded-md border bg-muted/25"
            onDragOver={(event) => event.preventDefault()}
            onDrop={() => void moveDragged(value)}
          >
            <summary className="flex cursor-pointer list-none items-center justify-between border-b px-3 py-2.5">
              <h3 className="truncate text-sm font-semibold">
                {displayDatasetValue(value)}
              </h3>
              <span className="font-mono text-xs text-muted-foreground">
                {columnRecords.length}
              </span>
            </summary>
            <div className="space-y-3 p-2.5">
              {subgroups.map((subgroup) => {
                const items = secondary
                  ? columnRecords.filter((record) =>
                      sameGroup(record.values[secondary.field], subgroup),
                    )
                  : columnRecords;
                return (
                  <details
                    key={JSON.stringify(subgroup)}
                    open={!secondary || secondary.display !== "collapsed"}
                  >
                    {secondary ? (
                      <summary className="mb-2 cursor-pointer truncate text-[11px] font-semibold text-muted-foreground">
                        {displayDatasetValue(subgroup)}
                      </summary>
                    ) : null}
                    <div className="space-y-2">
                      {items.map((record) => (
                        <article
                          key={record.id}
                          draggable={Boolean(onMove)}
                          onDragStart={(event) => {
                            setDraggedID(record.id);
                            event.dataTransfer.effectAllowed = "move";
                          }}
                          className="min-w-0 rounded-md border bg-card p-3 shadow-sm"
                        >
                          <div className="flex items-start gap-2">
                            {onMove ? (
                              <GripVertical
                                className="mt-0.5 size-4 shrink-0 cursor-grab text-muted-foreground"
                                aria-hidden
                              />
                            ) : null}
                            <p className="min-w-0 flex-1 break-words text-sm font-medium">
                              {recordTitle(record, fields, titleField)}
                            </p>
                          </div>
                          {onMove ? (
                            <select
                              value={String(record.values[primary.field] ?? "")}
                              onChange={(event) => {
                                const target = columns.find(
                                  (option) =>
                                    String(option ?? "") === event.target.value,
                                );
                                void onMove(record, primary.field, target);
                              }}
                              className="mt-3 h-7 w-full rounded border bg-background px-2 text-xs"
                              aria-label="移动到分组"
                            >
                              {columns.map((option) => (
                                <option
                                  key={JSON.stringify(option)}
                                  value={String(option ?? "")}
                                >
                                  {displayDatasetValue(option)}
                                </option>
                              ))}
                            </select>
                          ) : null}
                        </article>
                      ))}
                    </div>
                  </details>
                );
              })}
            </div>
          </details>
        );
      })}
    </div>
  );
}

export function DatasetTimeline({
  fields,
  records,
  dateField,
  endDateField,
  titleField,
}: {
  fields: DatasetField[];
  records: DatasetRecord[];
  dateField: string;
  endDateField?: string;
  titleField?: string;
}) {
  const ordered = useMemo(
    () =>
      [...records]
        .filter((record) => typeof record.values[dateField] === "string")
        .sort((left, right) =>
          String(left.values[dateField]).localeCompare(
            String(right.values[dateField]),
          ),
        ),
    [dateField, records],
  );
  if (ordered.length === 0) return <DatasetEmptyState />;
  return (
    <ol className="relative ml-3 border-l">
      {ordered.map((record) => (
        <li key={record.id} className="relative pb-7 pl-7">
          <span className="absolute -left-2 top-0 flex size-4 items-center justify-center rounded-full border bg-background">
            <span className="size-1.5 rounded-full bg-cyan-600" />
          </span>
          <time className="flex items-center gap-1.5 text-xs font-medium text-cyan-800">
            <Clock3 className="size-3.5" />
            {String(record.values[dateField])}
            {endDateField && record.values[endDateField]
              ? ` - ${String(record.values[endDateField])}`
              : ""}
          </time>
          <p className="mt-1 break-words text-sm font-semibold">
            {recordTitle(record, fields, titleField)}
          </p>
        </li>
      ))}
    </ol>
  );
}

function dateParts(value: Date): { year: number; month: number } {
  return { year: value.getFullYear(), month: value.getMonth() };
}

export function DatasetCalendar({
  fields,
  records,
  dateField,
  titleField,
}: {
  fields: DatasetField[];
  records: DatasetRecord[];
  dateField: string;
  titleField?: string;
}) {
  const firstDate = records.find(
    (record) => typeof record.values[dateField] === "string",
  )?.values[dateField];
  const [month, setMonth] = useState(() => {
    const parsed =
      typeof firstDate === "string" ? new Date(`${firstDate}T00:00:00`) : new Date();
    return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
  });
  const { year, month: monthIndex } = dateParts(month);
  const firstWeekday = new Date(year, monthIndex, 1).getDay();
  const days = new Date(year, monthIndex + 1, 0).getDate();
  const cells = Array.from(
    { length: Math.ceil((firstWeekday + days) / 7) * 7 },
    (_, index) => {
      const day = index - firstWeekday + 1;
      return day >= 1 && day <= days ? day : null;
    },
  );
  const recordsByDay = new Map<number, DatasetRecord[]>();
  for (const record of records) {
    const raw = record.values[dateField];
    if (typeof raw !== "string") continue;
    const [recordYear, recordMonth, recordDay] = raw
      .slice(0, 10)
      .split("-")
      .map(Number);
    if (recordYear !== year || recordMonth !== monthIndex + 1) continue;
    recordsByDay.set(recordDay, [
      ...(recordsByDay.get(recordDay) ?? []),
      record,
    ]);
  }
  return (
    <div className="min-w-0 rounded-md border bg-card">
      <header className="flex items-center justify-between border-b p-3">
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          onClick={() => setMonth(new Date(year, monthIndex - 1, 1))}
          aria-label="上个月"
        >
          <ChevronLeft />
        </Button>
        <h3 className="text-sm font-semibold">
          {year} 年 {monthIndex + 1} 月
        </h3>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          onClick={() => setMonth(new Date(year, monthIndex + 1, 1))}
          aria-label="下个月"
        >
          <ChevronRight />
        </Button>
      </header>
      <div className="grid grid-cols-7 border-b bg-muted/35 text-center text-[10px] text-muted-foreground">
        {"日一二三四五六".split("").map((day) => (
          <span key={day} className="py-2">
            {day}
          </span>
        ))}
      </div>
      <div className="grid grid-cols-7">
        {cells.map((day, index) => (
          <div
            key={index}
            className="min-h-16 min-w-0 border-b border-r p-1.5 sm:min-h-24"
          >
            {day ? (
              <>
                <span className="text-[11px] font-medium">{day}</span>
                <div className="mt-1 space-y-1">
                  {(recordsByDay.get(day) ?? []).slice(0, 3).map((record) => (
                    <p
                      key={record.id}
                      className="truncate rounded bg-cyan-100 px-1 py-0.5 text-[9px] text-cyan-900 sm:text-[10px]"
                      title={recordTitle(record, fields, titleField)}
                    >
                      {recordTitle(record, fields, titleField)}
                    </p>
                  ))}
                </div>
              </>
            ) : null}
          </div>
        ))}
      </div>
    </div>
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
        {groups.map((group, index) => {
          const groupLabel = group.path?.join(" / ") ?? group.value;
          return (
            <li
              key={`${groupLabel}-${index}`}
              className="grid grid-cols-[8rem_1fr_3rem] items-center gap-3"
            >
              <span className="truncate text-sm" title={groupLabel}>
                {groupLabel}
              </span>
              <span className="h-2 overflow-hidden rounded-full bg-muted">
                <span
                  className="block h-full rounded-full bg-gradient-to-r from-cyan-500 to-primary"
                  style={{
                    width: `${Math.max((group.count / maximum) * 100, 2)}%`,
                  }}
                />
              </span>
              <span className="text-right font-mono text-xs text-muted-foreground">
                {group.count}
              </span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}

function DatasetEmptyState() {
  return (
    <div className="rounded-2xl border border-dashed px-6 py-14 text-center">
      <List className="mx-auto size-8 text-muted-foreground" aria-hidden />
      <p className="mt-3 text-sm font-medium">当前结果没有记录</p>
      <p className="mt-1 text-xs text-muted-foreground">
        新增记录或调整保存视图的筛选条件。
      </p>
    </div>
  );
}
