"use client";

import Link from "next/link";
import {
  ArrowRight,
  Braces,
  Database,
  LoaderCircle,
  Plus,
  RefreshCw,
  Rows3,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";

import type {
  Dataset,
  DatasetListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { datasetsApi } from "@/lib/api";
import type { DatasetFieldType } from "@/lib/datasets";
import { useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 24;

interface FieldDraft {
  id: number;
  key: string;
  title: string;
  type: DatasetFieldType;
  required: boolean;
}

function fieldCount(dataset: Dataset): number {
  const properties = dataset.schema.properties;
  return typeof properties === "object" &&
    properties !== null &&
    !Array.isArray(properties)
    ? Object.keys(properties).length
    : 0;
}

function DatasetCard({ dataset }: { dataset: Dataset }) {
  return (
    <li>
      <Link
        href={`/datasets/${dataset.id}`}
        className="group block rounded-2xl border border-border/80 bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)] transition hover:-translate-y-0.5 hover:border-primary/20 hover:shadow-[0_14px_35px_rgb(15_23_42/0.08)]"
      >
        <span className="flex size-10 items-center justify-center rounded-xl bg-cyan-100 text-cyan-700">
          <Database className="size-4.5" aria-hidden />
        </span>
        <div className="mt-5 flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="truncate font-semibold">{dataset.name}</h3>
            <p className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
              <Rows3 className="size-3.5" aria-hidden />
              {fieldCount(dataset)} 个结构字段
            </p>
          </div>
          <ArrowRight
            className="mt-1 size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-primary"
            aria-hidden
          />
        </div>
        <p className="mt-4 truncate font-mono text-[10px] text-muted-foreground">
          {dataset.id}
        </p>
      </Link>
    </li>
  );
}

export function DatasetHub() {
  const [name, setName] = useState("");
  const [nextFieldID, setNextFieldID] = useState(2);
  const [fields, setFields] = useState<FieldDraft[]>([
    { id: 1, key: "name", title: "名称", type: "string", required: true },
  ]);
  const [creating, setCreating] = useState(false);
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const { data, error, isLoading, isValidating, mutate, size, setSize } =
    useSWRInfinite<DatasetListPage>(
      (pageIndex, previousPage) => {
        if (pageIndex > 0 && !previousPage?.nextCursor) return null;
        return [
          "datasets",
          pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
        ] as const;
      },
      ([, cursor]) =>
        datasetsApi().listDatasets({
          cursor: typeof cursor === "string" && cursor ? cursor : undefined,
          pageSize: PAGE_SIZE,
        }),
      { revalidateFirstPage: true },
    );

  const datasets = data?.flatMap((page) => page.items) ?? [];
  const lastPage = data?.[data.length - 1];
  const reachedEnd = Boolean(data && !lastPage?.nextCursor);

  const updateField = (id: number, patch: Partial<FieldDraft>) => {
    setFields((current) =>
      current.map((field) => (field.id === id ? { ...field, ...patch } : field)),
    );
  };

  const addField = () => {
    setFields((current) => [
      ...current,
      {
        id: nextFieldID,
        key: "",
        title: "",
        type: "string",
        required: false,
      },
    ]);
    setNextFieldID((value) => value + 1);
  };

  const create = async () => {
    const cleanName = name.trim();
    const normalized = fields.map((field) => ({
      ...field,
      key: field.key.trim(),
      title: field.title.trim(),
    }));
    if (!cleanName || normalized.some((field) => !field.key)) {
      toast.error("请填写数据集名称和每个字段键");
      return;
    }
    if (new Set(normalized.map((field) => field.key)).size !== normalized.length) {
      toast.error("字段键不能重复");
      return;
    }
    setCreating(true);
    try {
      const properties = Object.fromEntries(
        normalized.map((field) => [
          field.key,
          { type: field.type, title: field.title || field.key },
        ]),
      );
      const created = await datasetsApi().createDataset({
        createDatasetRequest: {
          name: cleanName,
          schema: {
            type: "object",
            properties,
            required: normalized
              .filter((field) => field.required)
              .map((field) => field.key),
            additionalProperties: false,
          },
        },
      });
      await mutate();
      setName("");
      toast.success("Dataset 已创建", {
        description: `${created.name} · ${normalized.length} 个字段`,
      });
    } catch {
      toast.error("创建 Dataset 失败", {
        description: "请检查字段定义、登录状态或名称是否重复。",
      });
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <section aria-labelledby="dataset-list-title">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="dataset-list-title" className="text-xl font-semibold">
              数据目录
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              把需要筛选、排序和聚合的信息从排版表格中分离出来。
            </p>
          </div>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={isValidating}
            onClick={() => void mutate()}
          >
            <RefreshCw
              className={cn("size-3.5", isValidating && "animate-spin")}
              aria-hidden
            />
            刷新
          </Button>
        </div>

        {error ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            Dataset 目录暂时无法读取。
          </div>
        ) : null}

        {isLoading && !data ? (
          <div className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {[0, 1, 2].map((item) => (
              <div
                key={item}
                className="h-48 animate-pulse rounded-2xl border bg-muted/45"
              />
            ))}
          </div>
        ) : null}

        {!isLoading && !error && datasets.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
            <Database className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">还没有 Dataset</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              用右侧结构设计器创建第一个可查询数据表。
            </p>
          </div>
        ) : null}

        {datasets.length > 0 ? (
          <ul className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {datasets.map((dataset) => (
              <DatasetCard key={dataset.id} dataset={dataset} />
            ))}
          </ul>
        ) : null}

        {data && !reachedEnd ? (
          <Button
            type="button"
            variant="outline"
            className="mt-4 w-full"
            onClick={() => void setSize(size + 1)}
          >
            加载更多
          </Button>
        ) : null}
      </section>

      <aside className="xl:sticky xl:top-24 xl:self-start">
        <div className="rounded-2xl border bg-card p-5">
          <span className="flex size-10 items-center justify-center rounded-xl bg-primary/9 text-primary">
            <Braces className="size-4.5" aria-hidden />
          </span>
          <h2 className="mt-4 font-semibold">新建结构化数据</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            每条记录都会按这里生成的 JSON Schema 严格校验。
          </p>
          <div className="mt-5 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="dataset-name">Dataset 名称</Label>
              <Input
                id="dataset-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="例如：历届赛事"
                disabled={!isAuthenticated || creating || sessionLoading}
              />
            </div>
            <div>
              <div className="flex items-center justify-between">
                <Label>字段</Label>
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  onClick={addField}
                  disabled={!isAuthenticated || creating || sessionLoading}
                >
                  <Plus aria-hidden />
                  添加
                </Button>
              </div>
              <div className="mt-2 space-y-3">
                {fields.map((field) => (
                  <div
                    key={field.id}
                    className="rounded-xl border bg-muted/25 p-3"
                  >
                    <div className="grid grid-cols-[1fr_6.5rem] gap-2">
                      <Input
                        value={field.key}
                        onChange={(event) =>
                          updateField(field.id, { key: event.target.value })
                        }
                        placeholder="字段键"
                        aria-label="字段键"
                      />
                      <select
                        value={field.type}
                        onChange={(event) =>
                          updateField(field.id, {
                            type: event.target.value as DatasetFieldType,
                          })
                        }
                        className="h-8 rounded-lg border border-input bg-background px-2 text-xs"
                        aria-label="字段类型"
                      >
                        <option value="string">文本</option>
                        <option value="number">数字</option>
                        <option value="integer">整数</option>
                        <option value="boolean">是/否</option>
                      </select>
                    </div>
                    <div className="mt-2 flex items-center gap-2">
                      <Input
                        value={field.title}
                        onChange={(event) =>
                          updateField(field.id, { title: event.target.value })
                        }
                        placeholder="显示名称"
                        aria-label="字段显示名称"
                      />
                      <label className="flex shrink-0 items-center gap-1.5 text-[11px] text-muted-foreground">
                        <Checkbox
                          checked={field.required}
                          onCheckedChange={(checked) =>
                            updateField(field.id, {
                              required: checked === true,
                            })
                          }
                        />
                        必填
                      </label>
                      {fields.length > 1 ? (
                        <Button
                          type="button"
                          size="icon-sm"
                          variant="ghost"
                          onClick={() =>
                            setFields((current) =>
                              current.filter((item) => item.id !== field.id),
                            )
                          }
                          aria-label="删除字段"
                        >
                          <Trash2 aria-hidden />
                        </Button>
                      ) : null}
                    </div>
                  </div>
                ))}
              </div>
            </div>
            <Button
              type="button"
              className="w-full"
              disabled={!isAuthenticated || creating || sessionLoading}
              onClick={() => void create()}
            >
              {creating ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Plus aria-hidden />
              )}
              {isAuthenticated ? "创建 Dataset" : "登录后创建"}
            </Button>
          </div>
        </div>
      </aside>
    </div>
  );
}
