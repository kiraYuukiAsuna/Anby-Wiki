"use client";

import Link from "next/link";

import { ExternalLink } from "lucide-react";

import type {
  DatasetGroupCount,
  DatasetRecord,
  DatasetView,
  DatasetViewGroup,
} from "../../../../contracts/generated/typescript";

import {
  DatasetBoard,
  DatasetCalendar,
  DatasetGroupChart,
  DatasetRecordCards,
  DatasetRecordGallery,
  DatasetRecordList,
  DatasetRecordTable,
  DatasetTimeline,
} from "@/components/datasets/dataset-records";
import { Button } from "@/components/ui/button";
import type { DatasetField } from "@/lib/datasets";

export function effectiveGroups(view: DatasetView): DatasetViewGroup[] {
  if (view.config.groups && view.config.groups.length > 0) {
    return view.config.groups;
  }
  return view.config.groupBy
    ? [
        {
          field: view.config.groupBy,
          display: "expanded",
          sortDirection: "asc",
        },
      ]
    : [];
}

export function visibleDatasetFields(
  view: DatasetView,
  fields: DatasetField[],
): DatasetField[] {
  const configured = view.config.columns;
  if (!configured || configured.size === 0) return fields;
  return fields.filter((field) => configured.has(field.key));
}

export function DatasetViewRenderer({
  view,
  fields,
  records,
  groupCounts,
  onBoardMove,
}: {
  view: DatasetView;
  fields: DatasetField[];
  records: DatasetRecord[];
  groupCounts: DatasetGroupCount[];
  onBoardMove?: (
    record: DatasetRecord,
    field: string,
    value: unknown,
  ) => Promise<void>;
}) {
  const visibleFields = visibleDatasetFields(view, fields);
  const groups = effectiveGroups(view);
  const titleField = view.config.titleField;

  switch (view.viewType) {
    case "cards":
      return <DatasetRecordCards fields={visibleFields} records={records} />;
    case "chart":
      return (
        <DatasetGroupChart
          groups={groupCounts}
          label={
            fields.find((field) => field.key === groups[0]?.field)?.title ??
            groups[0]?.field ??
            "字段"
          }
        />
      );
    case "board":
      return (
        <DatasetBoard
          fields={visibleFields}
          records={records}
          groups={groups}
          titleField={titleField}
          onMove={onBoardMove}
        />
      );
    case "timeline":
      return (
        <DatasetTimeline
          fields={visibleFields}
          records={records}
          dateField={view.config.dateField ?? ""}
          endDateField={view.config.endDateField}
          titleField={titleField}
        />
      );
    case "calendar":
      return (
        <DatasetCalendar
          fields={visibleFields}
          records={records}
          dateField={view.config.dateField ?? ""}
          titleField={titleField}
        />
      );
    case "list":
      return (
        <DatasetRecordList
          fields={visibleFields}
          records={records}
          titleField={titleField}
        />
      );
    case "gallery":
      return (
        <DatasetRecordGallery
          fields={visibleFields}
          records={records}
          titleField={titleField}
          descriptionField={view.config.descriptionField}
          coverField={view.config.coverField}
          cardSize={view.config.cardSize}
        />
      );
    case "form":
      return (
        <div className="rounded-md border border-dashed px-6 py-10 text-center">
          <p className="text-sm font-medium">填写入口已就绪</p>
          <Button asChild variant="outline" className="mt-4">
            <Link href={`/forms/${view.id}`} target="_blank">
              <ExternalLink />
              打开填写页
            </Link>
          </Button>
        </div>
      );
    case "table":
    default:
      return <DatasetRecordTable fields={visibleFields} records={records} />;
  }
}
