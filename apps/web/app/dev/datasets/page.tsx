"use client";

import { notFound } from "next/navigation";

import type {
  DatasetRecord,
  DatasetView,
} from "../../../../../contracts/generated/typescript";

import { DatasetViewRenderer } from "@/components/datasets/dataset-view-renderer";
import type { DatasetField } from "@/lib/datasets";

const FIELDS: DatasetField[] = [
  { key: "title", title: "事项", type: "string", required: true },
  { key: "status", title: "状态", type: "string", required: true },
  { key: "category", title: "类别", type: "string", required: false },
  { key: "start", title: "开始日期", type: "date", required: true },
  { key: "end", title: "结束日期", type: "date", required: false },
  { key: "score", title: "优先级", type: "integer", required: false },
  { key: "cover", title: "封面", type: "string", required: false },
];

const RECORDS: DatasetRecord[] = [
  ["研究资料", "待处理", "内容", "2026-10-03", "2026-10-06", 3],
  ["验证导入", "进行中", "工程", "2026-10-08", "2026-10-14", 5],
  ["补充引用", "待处理", "内容", "2026-10-12", "2026-10-12", 2],
  ["移动端验收", "进行中", "工程", "2026-10-18", "2026-10-21", 4],
  ["发布说明", "已完成", "运营", "2026-10-23", "2026-10-24", 1],
  ["归档材料", "已完成", "内容", "2026-10-28", "2026-10-28", 2],
].map(([title, status, category, start, end, score], index) => ({
  id: `0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5${String(index).padStart(3, "0")}`,
  datasetId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5000",
  entityId: "",
  values: { title, status, category, start, end, score, cover: "" },
  createdAt: new Date("2026-10-01T00:00:00Z"),
  updatedAt: new Date("2026-10-01T00:00:00Z"),
}));

const BASE = {
  datasetId: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5000",
  createdBy: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5001",
  createdAt: new Date("2026-10-01T00:00:00Z"),
};

const VIEWS: DatasetView[] = [
  {
    ...BASE,
    id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5101",
    name: "Board",
    viewType: "board",
    config: {
      columns: new Set(["title", "status", "category", "score"]),
      titleField: "title",
      groups: [
        {
          field: "status",
          display: "expanded",
          sortDirection: "asc",
          options: ["待处理", "进行中", "已完成"],
        },
        {
          field: "category",
          display: "expanded",
          sortDirection: "asc",
        },
      ],
    },
  },
  {
    ...BASE,
    id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5102",
    name: "Timeline",
    viewType: "timeline",
    config: {
      columns: new Set(["title", "start", "end"]),
      titleField: "title",
      dateField: "start",
      endDateField: "end",
    },
  },
  {
    ...BASE,
    id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5103",
    name: "Calendar",
    viewType: "calendar",
    config: {
      columns: new Set(["title", "start"]),
      titleField: "title",
      dateField: "start",
    },
  },
  {
    ...BASE,
    id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5104",
    name: "List",
    viewType: "list",
    config: {
      columns: new Set(["title", "status", "category"]),
      titleField: "title",
    },
  },
  {
    ...BASE,
    id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f5105",
    name: "Gallery",
    viewType: "gallery",
    config: {
      columns: new Set(["title", "category", "cover"]),
      titleField: "title",
      descriptionField: "category",
      coverField: "cover",
      cardSize: "medium",
    },
  },
];

export default function DevDatasetViewsPage() {
  if (process.env.NODE_ENV === "production") {
    notFound();
  }
  return (
    <main className="mx-auto w-full max-w-7xl space-y-12 px-4 py-8 sm:px-6">
      <header className="border-b pb-6">
        <h1 className="text-2xl font-semibold">Dataset 多视图验证</h1>
      </header>
      {VIEWS.map((view) => (
        <section key={view.id} className="min-w-0">
          <h2 className="mb-4 text-lg font-semibold">{view.name}</h2>
          <DatasetViewRenderer
            view={view}
            fields={FIELDS}
            records={RECORDS}
            groupCounts={[]}
          />
        </section>
      ))}
      <section className="min-w-0">
        <h2 className="mb-4 text-lg font-semibold">Dashboard</h2>
        <div className="grid min-w-0 gap-6 lg:grid-cols-2">
          {VIEWS.slice(0, 2).map((view) => (
            <section key={view.id} className="min-w-0 border-t pt-3">
              <h3 className="mb-3 text-sm font-semibold">{view.name}</h3>
              <DatasetViewRenderer
                view={view}
                fields={FIELDS}
                records={RECORDS}
                groupCounts={[]}
              />
            </section>
          ))}
        </div>
      </section>
    </main>
  );
}
