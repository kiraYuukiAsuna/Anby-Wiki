"use client";

import { useMemo, useRef, useState } from "react";

import {
  ArrowDownUp,
  Filter,
  Layers3,
  LoaderCircle,
  Plus,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import type {
  CreateDatasetViewRequest,
  Dataset,
  DatasetView,
  DatasetViewFilter,
  DatasetViewGroup,
  DatasetViewSort,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { datasetsApi } from "@/lib/api";
import { datasetFields, type DatasetField } from "@/lib/datasets";

type ViewType = CreateDatasetViewRequest["viewType"];
type FilterOperator = NonNullable<DatasetViewFilter["operator"]>;
type FilterLogic = NonNullable<DatasetViewFilter["logic"]>;

interface FilterDraft {
  id: number;
  field: string;
  operator: FilterOperator;
  value: string;
}

interface FilterGroupDraft {
  id: number;
  logic: FilterLogic;
  conditions: FilterDraft[];
}

interface SortDraft {
  id: number;
  field: string;
  direction: "asc" | "desc";
}

interface GroupDraft {
  id: number;
  field: string;
  display: "expanded" | "collapsed";
  hideEmpty: boolean;
  sortDirection: "asc" | "desc";
  options: string;
}

const VIEW_LABELS: Record<ViewType, string> = {
  table: "表格",
  cards: "卡片（兼容）",
  chart: "聚合图",
  board: "看板",
  timeline: "时间轴",
  calendar: "日历",
  list: "列表",
  gallery: "画廊",
  form: "表单",
  dashboard: "仪表盘",
};

const UNARY_OPERATORS = new Set<FilterOperator>([
  "exists",
  "not_exists",
  "is_empty",
  "not_empty",
]);

const OPERATOR_LABELS: Record<FilterOperator, string> = {
  eq: "等于",
  neq: "不等于",
  contains: "包含",
  not_contains: "不包含",
  gt: "大于",
  gte: "大于等于",
  lt: "小于",
  lte: "小于等于",
  before: "早于",
  after: "晚于",
  on_or_before: "不晚于",
  on_or_after: "不早于",
  exists: "字段存在",
  not_exists: "字段不存在",
  is_empty: "为空",
  not_empty: "不为空",
};

function operatorsFor(field: DatasetField | undefined): FilterOperator[] {
  if (field?.type === "number" || field?.type === "integer") {
    return [
      "eq",
      "neq",
      "gt",
      "gte",
      "lt",
      "lte",
      "exists",
      "not_exists",
      "is_empty",
      "not_empty",
    ];
  }
  if (field?.type === "date") {
    return [
      "eq",
      "neq",
      "before",
      "after",
      "on_or_before",
      "on_or_after",
      "exists",
      "not_exists",
      "is_empty",
      "not_empty",
    ];
  }
  if (field?.type === "boolean") {
    return ["eq", "neq", "exists", "not_exists", "is_empty", "not_empty"];
  }
  return [
    "contains",
    "not_contains",
    "eq",
    "neq",
    "exists",
    "not_exists",
    "is_empty",
    "not_empty",
  ];
}

function parseTypedValue(
  field: DatasetField | undefined,
  value: string,
): string | number | boolean {
  if (field?.type === "number" || field?.type === "integer") {
    return Number(value);
  }
  if (field?.type === "boolean") {
    return value === "true";
  }
  return value;
}

function filterFromDraft(
  groups: FilterGroupDraft[],
  rootLogic: FilterLogic,
  fields: DatasetField[],
): DatasetViewFilter | undefined {
  const expressions = groups.flatMap((group) => {
    const conditions = group.conditions.flatMap((condition) => {
      if (!condition.field) return [];
      const field = fields.find((item) => item.key === condition.field);
      return [
        {
          field: condition.field,
          operator: condition.operator,
          value: UNARY_OPERATORS.has(condition.operator)
            ? undefined
            : parseTypedValue(field, condition.value),
        } satisfies DatasetViewFilter,
      ];
    });
    if (conditions.length === 0) return [];
    return [
      conditions.length === 1
        ? conditions[0]
        : ({
            logic: group.logic,
            conditions,
          } satisfies DatasetViewFilter),
    ];
  });
  if (expressions.length === 0) return undefined;
  if (expressions.length === 1) return expressions[0];
  return { logic: rootLogic, conditions: expressions };
}

function fieldOption(field: DatasetField) {
  return (
    <option key={field.key} value={field.key}>
      {field.title}
    </option>
  );
}

export function DatasetViewCreator({
  dataset,
  views,
  onClose,
  onSaved,
}: {
  dataset: Dataset;
  views: DatasetView[];
  onClose: () => void;
  onSaved: (view: DatasetView) => Promise<void>;
}) {
  const fields = datasetFields(dataset);
  const dateFields = fields.filter((field) => field.type === "date");
  const nextID = useRef(10);
  const [name, setName] = useState("");
  const [viewType, setViewType] = useState<ViewType>("table");
  const [columns, setColumns] = useState(
    () => new Set(fields.map((field) => field.key)),
  );
  const [rootLogic, setRootLogic] = useState<FilterLogic>("and");
  const [filterGroups, setFilterGroups] = useState<FilterGroupDraft[]>([
    {
      id: 1,
      logic: "and",
      conditions: [{ id: 2, field: "", operator: "contains", value: "" }],
    },
  ]);
  const [sorts, setSorts] = useState<SortDraft[]>([]);
  const [groups, setGroups] = useState<GroupDraft[]>([]);
  const [titleField, setTitleField] = useState(fields[0]?.key ?? "");
  const [descriptionField, setDescriptionField] = useState("");
  const [coverField, setCoverField] = useState("");
  const [dateField, setDateField] = useState(dateFields[0]?.key ?? "");
  const [endDateField, setEndDateField] = useState("");
  const [cardSize, setCardSize] =
    useState<"small" | "medium" | "large">("medium");
  const [publicForm, setPublicForm] = useState(true);
  const [submitLabel, setSubmitLabel] = useState("提交");
  const [successMessage, setSuccessMessage] = useState("提交成功");
  const [selectedViews, setSelectedViews] = useState<Set<string>>(
    () => new Set(),
  );
  const [globalFilterFields, setGlobalFilterFields] = useState<Set<string>>(
    () => new Set(),
  );
  const [layoutColumns, setLayoutColumns] = useState<1 | 2 | 3>(2);
  const [saving, setSaving] = useState(false);

  const childViews = useMemo(
    () => views.filter((view) => view.viewType !== "dashboard"),
    [views],
  );

  const allocateID = () => {
    const id = nextID.current;
    nextID.current += 1;
    return id;
  };

  const addFilterCondition = (groupID: number) => {
    setFilterGroups((current) =>
      current.map((group) =>
        group.id === groupID
          ? {
              ...group,
              conditions: [
                ...group.conditions,
                {
                  id: allocateID(),
                  field: "",
                  operator: "contains",
                  value: "",
                },
              ],
            }
          : group,
      ),
    );
  };

  const addFilterGroup = () => {
    setFilterGroups((current) => [
      ...current,
      {
        id: allocateID(),
        logic: "and",
        conditions: [
          {
            id: allocateID(),
            field: "",
            operator: "contains",
            value: "",
          },
        ],
      },
    ]);
  };

  const save = async () => {
    if (!name.trim()) {
      toast.error("请填写视图名称");
      return;
    }
    if (viewType !== "dashboard" && columns.size === 0) {
      toast.error("请至少选择一个字段");
      return;
    }
    const filter = filterFromDraft(filterGroups, rootLogic, fields);
    const invalidFilter = filterGroups.some((group) =>
      group.conditions.some(
        (condition) =>
          condition.field &&
          !UNARY_OPERATORS.has(condition.operator) &&
          !condition.value.trim(),
      ),
    );
    if (invalidFilter) {
      toast.error("请补齐筛选值");
      return;
    }
    if ((viewType === "board" || viewType === "chart") && !groups[0]?.field) {
      toast.error("该视图需要至少一个分组字段");
      return;
    }
    if (
      (viewType === "timeline" || viewType === "calendar") &&
      !dateField
    ) {
      toast.error("该视图需要日期字段");
      return;
    }
    if (viewType === "dashboard" && selectedViews.size === 0) {
      toast.error("仪表盘需要至少一个子视图");
      return;
    }

    const viewGroups: DatasetViewGroup[] = groups
      .filter((group) => group.field)
      .map((group) => {
        const field = fields.find((item) => item.key === group.field);
        const options = group.options
          .split(",")
          .map((value) => value.trim())
          .filter(Boolean)
          .map((value) => parseTypedValue(field, value));
        return {
          field: group.field,
          display: group.display,
          hideEmpty: group.hideEmpty,
          sortDirection: group.sortDirection,
          options: options.length > 0 ? options : undefined,
        };
      });
    const viewSorts: DatasetViewSort[] = sorts
      .filter((sort) => sort.field)
      .map(({ field, direction }) => ({ field, direction }));
    const config: CreateDatasetViewRequest["config"] = {
      columns,
      filter,
      sorts: viewSorts.length > 0 ? viewSorts : undefined,
      groups: viewGroups.length > 0 ? viewGroups : undefined,
      titleField: titleField || undefined,
      descriptionField: descriptionField || undefined,
      coverField: coverField || undefined,
      dateField:
        viewType === "timeline" || viewType === "calendar"
          ? dateField || undefined
          : undefined,
      endDateField:
        viewType === "timeline" ? endDateField || undefined : undefined,
      cardSize: viewType === "gallery" ? cardSize : undefined,
      formFields: viewType === "form" ? columns : undefined,
      _public: viewType === "form" ? publicForm : undefined,
      submitLabel: viewType === "form" ? submitLabel.trim() : undefined,
      successMessage:
        viewType === "form" ? successMessage.trim() : undefined,
      viewIds: viewType === "dashboard" ? selectedViews : undefined,
      globalFilterFields:
        viewType === "dashboard" ? globalFilterFields : undefined,
      layoutColumns: viewType === "dashboard" ? layoutColumns : undefined,
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
        description: "请检查字段类型、条件树和视图专属配置。",
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
          配置可复用的查询、布局与交互行为。
        </DialogDescription>
      </DialogHeader>
      <div className="max-h-[72vh] space-y-6 overflow-y-auto pr-1">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="view-name">视图名称</Label>
            <Input
              id="view-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
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
              {Object.entries(VIEW_LABELS).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </div>
        </div>

        {viewType !== "dashboard" ? (
          <fieldset>
            <legend className="text-sm font-medium">显示字段</legend>
            <div className="mt-2 grid gap-2 sm:grid-cols-2">
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
                  {field.required ? (
                    <span className="text-destructive">*</span>
                  ) : null}
                </label>
              ))}
            </div>
          </fieldset>
        ) : null}

        {viewType !== "form" && viewType !== "dashboard" ? (
          <>
            <section aria-labelledby="filter-title">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 id="filter-title" className="flex items-center gap-2 text-sm font-medium">
                  <Filter className="size-4 text-muted-foreground" />
                  高级筛选
                </h3>
                <div className="flex items-center gap-2">
                  <select
                    value={rootLogic}
                    onChange={(event) =>
                      setRootLogic(event.target.value as FilterLogic)
                    }
                    className="h-7 rounded-md border bg-background px-2 text-xs"
                    aria-label="筛选组连接逻辑"
                  >
                    <option value="and">所有组 AND</option>
                    <option value="or">任一组 OR</option>
                  </select>
                  <Button type="button" size="xs" variant="outline" onClick={addFilterGroup}>
                    <Plus />
                    条件组
                  </Button>
                </div>
              </div>
              <div className="mt-3 space-y-3">
                {filterGroups.map((group) => (
                  <div key={group.id} className="rounded-md border p-3">
                    <div className="mb-2 flex items-center justify-between">
                      <select
                        value={group.logic}
                        onChange={(event) =>
                          setFilterGroups((current) =>
                            current.map((item) =>
                              item.id === group.id
                                ? {
                                    ...item,
                                    logic: event.target.value as FilterLogic,
                                  }
                                : item,
                            ),
                          )
                        }
                        className="h-7 rounded-md border bg-background px-2 text-xs"
                        aria-label="组内条件逻辑"
                      >
                        <option value="and">组内 AND</option>
                        <option value="or">组内 OR</option>
                      </select>
                      <div className="flex gap-1">
                        <Button
                          type="button"
                          size="icon-xs"
                          variant="ghost"
                          onClick={() => addFilterCondition(group.id)}
                          aria-label="添加筛选条件"
                        >
                          <Plus />
                        </Button>
                        {filterGroups.length > 1 ? (
                          <Button
                            type="button"
                            size="icon-xs"
                            variant="ghost"
                            onClick={() =>
                              setFilterGroups((current) =>
                                current.filter((item) => item.id !== group.id),
                              )
                            }
                            aria-label="删除筛选组"
                          >
                            <Trash2 />
                          </Button>
                        ) : null}
                      </div>
                    </div>
                    <div className="space-y-2">
                      {group.conditions.map((condition) => {
                        const field = fields.find(
                          (item) => item.key === condition.field,
                        );
                        const operators = operatorsFor(field);
                        return (
                          <div
                            key={condition.id}
                            className="grid gap-2 sm:grid-cols-[1fr_8rem_1fr_auto]"
                          >
                            <select
                              value={condition.field}
                              onChange={(event) => {
                                const nextField = fields.find(
                                  (item) => item.key === event.target.value,
                                );
                                setFilterGroups((current) =>
                                  current.map((item) =>
                                    item.id === group.id
                                      ? {
                                          ...item,
                                          conditions: item.conditions.map(
                                            (value) =>
                                              value.id === condition.id
                                                ? {
                                                    ...value,
                                                    field: event.target.value,
                                                    operator:
                                                      operatorsFor(nextField)[0],
                                                    value: "",
                                                  }
                                                : value,
                                          ),
                                        }
                                      : item,
                                  ),
                                );
                              }}
                              className="h-8 rounded-lg border bg-background px-2 text-sm"
                              aria-label="筛选字段"
                            >
                              <option value="">选择字段</option>
                              {fields.map(fieldOption)}
                            </select>
                            <select
                              value={condition.operator}
                              onChange={(event) =>
                                setFilterGroups((current) =>
                                  current.map((item) =>
                                    item.id === group.id
                                      ? {
                                          ...item,
                                          conditions: item.conditions.map(
                                            (value) =>
                                              value.id === condition.id
                                                ? {
                                                    ...value,
                                                    operator: event.target
                                                      .value as FilterOperator,
                                                    value: "",
                                                  }
                                                : value,
                                          ),
                                        }
                                      : item,
                                  ),
                                )
                              }
                              disabled={!condition.field}
                              className="h-8 rounded-lg border bg-background px-2 text-sm"
                              aria-label="筛选操作"
                            >
                              {operators.map((operator) => (
                                <option key={operator} value={operator}>
                                  {OPERATOR_LABELS[operator]}
                                </option>
                              ))}
                            </select>
                            {field?.type === "boolean" &&
                            !UNARY_OPERATORS.has(condition.operator) ? (
                              <select
                                value={condition.value}
                                onChange={(event) =>
                                  setFilterGroups((current) =>
                                    current.map((item) =>
                                      item.id === group.id
                                        ? {
                                            ...item,
                                            conditions: item.conditions.map(
                                              (value) =>
                                                value.id === condition.id
                                                  ? {
                                                      ...value,
                                                      value: event.target.value,
                                                    }
                                                  : value,
                                            ),
                                          }
                                        : item,
                                    ),
                                  )
                                }
                                className="h-8 rounded-lg border bg-background px-2 text-sm"
                                aria-label="筛选值"
                              >
                                <option value="">选择</option>
                                <option value="true">是</option>
                                <option value="false">否</option>
                              </select>
                            ) : (
                              <Input
                                type={
                                  field?.type === "date"
                                    ? "date"
                                    : field?.type === "number" ||
                                        field?.type === "integer"
                                      ? "number"
                                      : "text"
                                }
                                value={condition.value}
                                onChange={(event) =>
                                  setFilterGroups((current) =>
                                    current.map((item) =>
                                      item.id === group.id
                                        ? {
                                            ...item,
                                            conditions: item.conditions.map(
                                              (value) =>
                                                value.id === condition.id
                                                  ? {
                                                      ...value,
                                                      value: event.target.value,
                                                    }
                                                  : value,
                                            ),
                                          }
                                        : item,
                                    ),
                                  )
                                }
                                disabled={
                                  !condition.field ||
                                  UNARY_OPERATORS.has(condition.operator)
                                }
                                aria-label="筛选值"
                              />
                            )}
                            <Button
                              type="button"
                              size="icon-sm"
                              variant="ghost"
                              disabled={group.conditions.length === 1}
                              onClick={() =>
                                setFilterGroups((current) =>
                                  current.map((item) =>
                                    item.id === group.id
                                      ? {
                                          ...item,
                                          conditions: item.conditions.filter(
                                            (value) =>
                                              value.id !== condition.id,
                                          ),
                                        }
                                      : item,
                                  ),
                                )
                              }
                              aria-label="删除筛选条件"
                            >
                              <Trash2 />
                            </Button>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            </section>

            <section aria-labelledby="sort-title">
              <div className="flex items-center justify-between gap-2">
                <h3 id="sort-title" className="flex items-center gap-2 text-sm font-medium">
                  <ArrowDownUp className="size-4 text-muted-foreground" />
                  排序链
                </h3>
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  disabled={sorts.length >= 5}
                  onClick={() =>
                    setSorts((current) => [
                      ...current,
                      { id: allocateID(), field: "", direction: "asc" },
                    ])
                  }
                >
                  <Plus />
                  排序
                </Button>
              </div>
              <div className="mt-3 space-y-2">
                {sorts.map((sort, index) => (
                  <div key={sort.id} className="grid grid-cols-[1.5rem_1fr_6rem_auto] gap-2">
                    <span className="self-center text-center text-xs text-muted-foreground">
                      {index + 1}
                    </span>
                    <select
                      value={sort.field}
                      onChange={(event) =>
                        setSorts((current) =>
                          current.map((item) =>
                            item.id === sort.id
                              ? { ...item, field: event.target.value }
                              : item,
                          ),
                        )
                      }
                      className="h-8 rounded-lg border bg-background px-2 text-sm"
                      aria-label={`排序字段 ${index + 1}`}
                    >
                      <option value="">选择字段</option>
                      {fields.map(fieldOption)}
                    </select>
                    <select
                      value={sort.direction}
                      onChange={(event) =>
                        setSorts((current) =>
                          current.map((item) =>
                            item.id === sort.id
                              ? {
                                  ...item,
                                  direction: event.target.value as "asc" | "desc",
                                }
                              : item,
                          ),
                        )
                      }
                      className="h-8 rounded-lg border bg-background px-2 text-sm"
                      aria-label={`排序方向 ${index + 1}`}
                    >
                      <option value="asc">升序</option>
                      <option value="desc">降序</option>
                    </select>
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      onClick={() =>
                        setSorts((current) =>
                          current.filter((item) => item.id !== sort.id),
                        )
                      }
                      aria-label="删除排序"
                    >
                      <Trash2 />
                    </Button>
                  </div>
                ))}
              </div>
            </section>

            <section aria-labelledby="group-title">
              <div className="flex items-center justify-between gap-2">
                <h3 id="group-title" className="flex items-center gap-2 text-sm font-medium">
                  <Layers3 className="size-4 text-muted-foreground" />
                  分组与子分组
                </h3>
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  disabled={groups.length >= 2}
                  onClick={() =>
                    setGroups((current) => [
                      ...current,
                      {
                        id: allocateID(),
                        field: "",
                        display: "expanded",
                        hideEmpty: false,
                        sortDirection: "asc",
                        options: "",
                      },
                    ])
                  }
                >
                  <Plus />
                  {groups.length === 0 ? "分组" : "子分组"}
                </Button>
              </div>
              <div className="mt-3 space-y-3">
                {groups.map((group, index) => (
                  <div key={group.id} className="grid gap-2 rounded-md border p-3 sm:grid-cols-2">
                    <select
                      value={group.field}
                      onChange={(event) =>
                        setGroups((current) =>
                          current.map((item) =>
                            item.id === group.id
                              ? { ...item, field: event.target.value }
                              : item,
                          ),
                        )
                      }
                      className="h-8 rounded-lg border bg-background px-2 text-sm"
                      aria-label={index === 0 ? "分组字段" : "子分组字段"}
                    >
                      <option value="">选择字段</option>
                      {fields.map(fieldOption)}
                    </select>
                    <div className="grid grid-cols-2 gap-2">
                      <select
                        value={group.display}
                        onChange={(event) =>
                          setGroups((current) =>
                            current.map((item) =>
                              item.id === group.id
                                ? {
                                    ...item,
                                    display: event.target.value as
                                      | "expanded"
                                      | "collapsed",
                                  }
                                : item,
                            ),
                          )
                        }
                        className="h-8 rounded-lg border bg-background px-2 text-sm"
                        aria-label="分组默认显示"
                      >
                        <option value="expanded">默认展开</option>
                        <option value="collapsed">默认折叠</option>
                      </select>
                      <select
                        value={group.sortDirection}
                        onChange={(event) =>
                          setGroups((current) =>
                            current.map((item) =>
                              item.id === group.id
                                ? {
                                    ...item,
                                    sortDirection: event.target.value as
                                      | "asc"
                                      | "desc",
                                  }
                                : item,
                            ),
                          )
                        }
                        className="h-8 rounded-lg border bg-background px-2 text-sm"
                        aria-label="分组排序"
                      >
                        <option value="asc">组名升序</option>
                        <option value="desc">组名降序</option>
                      </select>
                    </div>
                    <Input
                      value={group.options}
                      onChange={(event) =>
                        setGroups((current) =>
                          current.map((item) =>
                            item.id === group.id
                              ? { ...item, options: event.target.value }
                              : item,
                          ),
                        )
                      }
                      placeholder="固定组选项，逗号分隔"
                      aria-label="固定组选项"
                    />
                    <div className="flex items-center justify-between">
                      <label className="flex items-center gap-2 text-xs">
                        <Checkbox
                          checked={group.hideEmpty}
                          onCheckedChange={(checked) =>
                            setGroups((current) =>
                              current.map((item) =>
                                item.id === group.id
                                  ? { ...item, hideEmpty: checked === true }
                                  : item,
                              ),
                            )
                          }
                        />
                        隐藏空值组
                      </label>
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        onClick={() =>
                          setGroups((current) =>
                            current.filter((item) => item.id !== group.id),
                          )
                        }
                        aria-label="删除分组"
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          </>
        ) : null}

        {viewType === "board" ||
        viewType === "timeline" ||
        viewType === "calendar" ||
        viewType === "list" ||
        viewType === "gallery" ? (
          <section className="grid gap-4 border-t pt-5 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="view-title-field">标题字段</Label>
              <select
                id="view-title-field"
                value={titleField}
                onChange={(event) => setTitleField(event.target.value)}
                className="h-8 w-full rounded-lg border bg-background px-2 text-sm"
              >
                {fields.map(fieldOption)}
              </select>
            </div>
            {viewType === "gallery" ? (
              <>
                <div className="space-y-2">
                  <Label htmlFor="view-cover-field">媒体封面字段</Label>
                  <select
                    id="view-cover-field"
                    value={coverField}
                    onChange={(event) => setCoverField(event.target.value)}
                    className="h-8 w-full rounded-lg border bg-background px-2 text-sm"
                  >
                    <option value="">无封面</option>
                    {fields
                      .filter((field) => field.type === "string")
                      .map(fieldOption)}
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="view-description-field">描述字段</Label>
                  <select
                    id="view-description-field"
                    value={descriptionField}
                    onChange={(event) => setDescriptionField(event.target.value)}
                    className="h-8 w-full rounded-lg border bg-background px-2 text-sm"
                  >
                    <option value="">无描述</option>
                    {fields.map(fieldOption)}
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="view-card-size">卡片尺寸</Label>
                  <select
                    id="view-card-size"
                    value={cardSize}
                    onChange={(event) =>
                      setCardSize(
                        event.target.value as "small" | "medium" | "large",
                      )
                    }
                    className="h-8 w-full rounded-lg border bg-background px-2 text-sm"
                  >
                    <option value="small">紧凑</option>
                    <option value="medium">标准</option>
                    <option value="large">宽幅</option>
                  </select>
                </div>
              </>
            ) : null}
            {viewType === "timeline" || viewType === "calendar" ? (
              <>
                <div className="space-y-2">
                  <Label htmlFor="view-date-field">日期字段</Label>
                  <select
                    id="view-date-field"
                    value={dateField}
                    onChange={(event) => setDateField(event.target.value)}
                    className="h-8 w-full rounded-lg border bg-background px-2 text-sm"
                  >
                    <option value="">选择日期字段</option>
                    {dateFields.map(fieldOption)}
                  </select>
                </div>
                {viewType === "timeline" ? (
                  <div className="space-y-2">
                    <Label htmlFor="view-end-date-field">结束日期字段</Label>
                    <select
                      id="view-end-date-field"
                      value={endDateField}
                      onChange={(event) => setEndDateField(event.target.value)}
                      className="h-8 w-full rounded-lg border bg-background px-2 text-sm"
                    >
                      <option value="">单点事件</option>
                      {dateFields.map(fieldOption)}
                    </select>
                  </div>
                ) : null}
              </>
            ) : null}
          </section>
        ) : null}

        {viewType === "form" ? (
          <section className="grid gap-4 border-t pt-5 sm:grid-cols-2">
            <label className="flex items-center gap-2 text-sm sm:col-span-2">
              <Checkbox
                checked={publicForm}
                onCheckedChange={(checked) => setPublicForm(checked === true)}
              />
              允许公开填写
            </label>
            <div className="space-y-2">
              <Label htmlFor="form-submit-label">提交按钮</Label>
              <Input
                id="form-submit-label"
                value={submitLabel}
                onChange={(event) => setSubmitLabel(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="form-success-message">提交成功提示</Label>
              <Input
                id="form-success-message"
                value={successMessage}
                onChange={(event) => setSuccessMessage(event.target.value)}
              />
            </div>
          </section>
        ) : null}

        {viewType === "dashboard" ? (
          <section className="space-y-5 border-t pt-5">
            <div>
              <p className="text-sm font-medium">子视图</p>
              <div className="mt-2 grid gap-2 sm:grid-cols-2">
                {childViews.map((view) => (
                  <label key={view.id} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={selectedViews.has(view.id)}
                      onCheckedChange={(checked) =>
                        setSelectedViews((current) => {
                          const next = new Set(current);
                          if (checked === true) next.add(view.id);
                          else next.delete(view.id);
                          return next;
                        })
                      }
                    />
                    {view.name}
                  </label>
                ))}
              </div>
            </div>
            <div>
              <p className="text-sm font-medium">全局筛选字段</p>
              <div className="mt-2 grid gap-2 sm:grid-cols-2">
                {fields.map((field) => (
                  <label key={field.key} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={globalFilterFields.has(field.key)}
                      onCheckedChange={(checked) =>
                        setGlobalFilterFields((current) => {
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
            </div>
            <div className="space-y-2">
              <Label htmlFor="dashboard-columns">布局列数</Label>
              <select
                id="dashboard-columns"
                value={layoutColumns}
                onChange={(event) =>
                  setLayoutColumns(Number(event.target.value) as 1 | 2 | 3)
                }
                className="h-8 w-32 rounded-lg border bg-background px-2 text-sm"
              >
                <option value="1">一列</option>
                <option value="2">两列</option>
                <option value="3">三列</option>
              </select>
            </div>
          </section>
        ) : null}
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button type="button" disabled={saving} onClick={() => void save()}>
          {saving ? <LoaderCircle className="animate-spin" /> : null}
          保存视图
        </Button>
      </DialogFooter>
    </>
  );
}
