"use client";

import { Command } from "cmdk";
import { Plus, RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { knowledgeApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import {
  COMPONENT_PRESETS,
  COMPONENT_RENDERERS,
  componentFields,
  parseComponentObject,
  type ComponentField,
  type ComponentRenderer,
  type ComponentSchema,
} from "@/lib/components";

const INFOBOX_FIELD_LABELS: Record<string, string> = {
  title: "标题",
  language: "标签语言",
  property_keys: "展示属性",
};

function FieldName({
  value,
  onRename,
}: {
  value: string;
  onRename: (key: string) => boolean;
}) {
  const [name, setName] = useState(value);
  return (
    <Input
      aria-label="字段名称"
      value={name}
      onChange={(event) => setName(event.target.value)}
      onBlur={() => {
        if (!onRename(name.trim())) setName(value);
      }}
      onKeyDown={(event) => {
        if (event.key === "Enter") event.currentTarget.blur();
      }}
    />
  );
}

function FieldValue({
  field,
  value,
  onChange,
  id,
}: {
  field: ComponentField;
  value: unknown;
  onChange: (value: unknown) => void;
  id: string;
}) {
  const serialized = JSON.stringify(value);
  const [previous, setPrevious] = useState(serialized);
  const [listSource, setListSource] = useState(
    Array.isArray(value) ? value.join("\n") : "",
  );
  if (previous !== serialized) {
    setPrevious(serialized);
    setListSource(Array.isArray(value) ? value.join("\n") : "");
  }
  if (field.schema.type === "boolean") {
    return (
      <Checkbox
        id={id}
        checked={value === true}
        onCheckedChange={(next) => onChange(next === true)}
      />
    );
  }
  if (field.schema.type === "array") {
    return (
      <Textarea
        id={id}
        value={listSource}
        placeholder="每行一项"
        className="min-h-20"
        onChange={(event) => {
          const text = event.target.value;
          const items = text
            .split("\n")
            .map((item) => item.trim())
            .filter(Boolean);
          const next = field.schema.uniqueItems ? [...new Set(items)] : items;
          setListSource(text);
          setPrevious(JSON.stringify(next));
          onChange(next);
        }}
      />
    );
  }
  const numeric =
    field.schema.type === "number" || field.schema.type === "integer";
  return (
    <Input
      id={id}
      type={numeric ? "number" : "text"}
      step={field.schema.type === "integer" ? 1 : "any"}
      value={
        typeof value === "string" || typeof value === "number" ? value : ""
      }
      onChange={(event) =>
        onChange(
          numeric
            ? Number.isFinite(event.target.valueAsNumber)
              ? event.target.valueAsNumber
              : undefined
            : event.target.value,
        )
      }
    />
  );
}

export function ComponentDefinitionForm({
  renderer,
  source,
  onChange,
}: {
  renderer: ComponentRenderer;
  source: string;
  onChange: (renderer: ComponentRenderer, source: string) => void;
}) {
  const id = useId();
  const schema = parseComponentObject(source);
  const fields = schema ? componentFields(schema) : null;
  const [advanced, setAdvanced] = useState(fields === null);
  const update = (nextFields: ComponentField[]) => {
    if (!schema) return;
    onChange(
      renderer,
      JSON.stringify(
        {
          ...schema,
          properties: Object.fromEntries(
            nextFields.map((field) => [field.key, field.schema]),
          ),
          required: nextFields
            .filter((field) => field.required)
            .map((field) => field.key),
        },
        null,
        2,
      ),
    );
  };

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor={`${id}-renderer`}>组件类型</Label>
        <select
          id={`${id}-renderer`}
          value={renderer}
          className="h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
          onChange={(event) => {
            const next = event.target.value as ComponentRenderer;
            const preset = COMPONENT_PRESETS.find(
              (item) => item.renderer === next,
            )!;
            onChange(next, JSON.stringify(preset.schema, null, 2));
            setAdvanced(false);
          }}
        >
          <option value={COMPONENT_RENDERERS.BuiltinKeyValue}>
            资料卡 · 手动填写字段
          </option>
          <option value={COMPONENT_RENDERERS.BuiltinEntityClaimInfobox}>
            动态信息框 · 读取实体事实
          </option>
        </select>
      </div>
      {advanced || !fields ? (
        <div className="space-y-2">
          <Label htmlFor={`${id}-schema`}>高级 JSON Schema</Label>
          {!fields ? (
            <p className="text-xs text-muted-foreground">
              此配置含有复杂约束，请使用 JSON 编辑以保留完整定义。
            </p>
          ) : null}
          <Textarea
            id={`${id}-schema`}
            className="min-h-64 font-mono text-xs"
            value={source}
            onChange={(event) => onChange(renderer, event.target.value)}
            spellCheck={false}
          />
        </div>
      ) : renderer === COMPONENT_RENDERERS.BuiltinEntityClaimInfobox ? (
        <>
          <p className="text-xs text-muted-foreground">
            这些设置作为插入时的默认值；标题留空使用实体名称，属性留空展示全部已发布事实。
          </p>
          {fields.map((field) => (
            <div key={field.key} className="space-y-2">
              <Label htmlFor={`${id}-${field.key}`}>
                {String(
                  field.schema.title ??
                    INFOBOX_FIELD_LABELS[field.key] ??
                    field.key,
                )}
              </Label>
              <FieldValue
                field={field}
                value={field.schema.default}
                id={`${id}-${field.key}`}
                onChange={(value) =>
                  update(
                    fields.map((item) =>
                      item.key === field.key
                        ? {
                            ...item,
                            schema: { ...item.schema, default: value },
                          }
                        : item,
                    ),
                  )
                }
              />
            </div>
          ))}
        </>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            添加字段并填写默认内容。插入页面时可以再次修改；资料卡按字段名称排序。
          </p>
          {fields.map((field, index) => (
            <div key={field.key} className="space-y-3 rounded-xl border p-3">
              <div className="flex items-center gap-2">
                <div className="min-w-0 flex-1">
                  <FieldName
                    value={field.key}
                    onRename={(key) => {
                      if (
                        !key ||
                        fields.some(
                          (item) => item !== field && item.key === key,
                        )
                      ) {
                        toast.error("字段名称不能为空或重复");
                        return false;
                      }
                      update(
                        fields.map((item) =>
                          item === field ? { ...item, key } : item,
                        ),
                      );
                      return true;
                    }}
                  />
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`删除字段 ${field.key}`}
                  onClick={() =>
                    update(fields.filter((item) => item !== field))
                  }
                >
                  <Trash2 aria-hidden />
                </Button>
              </div>
              <div className="grid gap-3 sm:grid-cols-[8rem_minmax(0,1fr)]">
                <select
                  aria-label={`${field.key}的类型`}
                  value={String(field.schema.type)}
                  className="h-9 w-full rounded-lg border bg-background px-2 text-sm"
                  onChange={(event) => {
                    const type = event.target.value;
                    update(
                      fields.map((item) =>
                        item === field
                          ? {
                              ...item,
                              schema: {
                                type,
                                default:
                                  type === "boolean"
                                    ? false
                                    : type === "string"
                                      ? ""
                                      : 0,
                              },
                            }
                          : item,
                      ),
                    );
                  }}
                >
                  <option value="string">文本</option>
                  <option value="number">数字</option>
                  <option value="integer">整数</option>
                  <option value="boolean">是 / 否</option>
                  {field.schema.type === "array" ? (
                    <option value="array">文本列表</option>
                  ) : null}
                </select>
                <div className="space-y-2">
                  <Label htmlFor={`${id}-default-${index}`}>默认内容</Label>
                  <FieldValue
                    field={field}
                    value={field.schema.default}
                    id={`${id}-default-${index}`}
                    onChange={(value) =>
                      update(
                        fields.map((item) =>
                          item === field
                            ? {
                                ...item,
                                schema: { ...item.schema, default: value },
                              }
                            : item,
                        ),
                      )
                    }
                  />
                </div>
              </div>
              <label className="flex items-center gap-2 text-xs">
                <Checkbox
                  checked={field.required}
                  onCheckedChange={(checked) =>
                    update(
                      fields.map((item) =>
                        item === field
                          ? { ...item, required: checked === true }
                          : item,
                      ),
                    )
                  }
                />
                必填字段
              </label>
            </div>
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              let key = "新字段";
              for (
                let index = 2;
                fields.some((field) => field.key === key);
                index++
              )
                key = `新字段 ${index}`;
              update([
                ...fields,
                {
                  key,
                  schema: { type: "string", default: "" },
                  required: false,
                },
              ]);
            }}
          >
            <Plus aria-hidden />
            添加字段
          </Button>
        </>
      )}
      {fields ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setAdvanced(!advanced)}
        >
          {advanced ? "返回表单" : "高级：编辑 JSON Schema"}
        </Button>
      ) : null}
    </div>
  );
}

export function ComponentPropsForm({
  schema,
  renderer,
  source,
  onChange,
}: {
  schema: ComponentSchema;
  renderer: ComponentRenderer;
  source: string;
  onChange: (source: string) => void;
}) {
  const id = useId();
  const fields = componentFields(schema);
  const values = parseComponentObject(source);
  const [advanced, setAdvanced] = useState(
    fields === null || fields.length === 0,
  );
  const extraValues =
    values &&
    fields &&
    Object.keys(values).some(
      (key) => !fields.some((field) => field.key === key),
    );
  const useJSON = advanced || !fields?.length || !values || extraValues;
  return (
    <div className="space-y-4">
      {useJSON ? (
        <div className="space-y-2">
          <Label htmlFor={`${id}-props`}>显示配置 JSON</Label>
          <Textarea
            id={`${id}-props`}
            className="min-h-48 font-mono text-xs"
            value={source}
            onChange={(event) => onChange(event.target.value)}
            spellCheck={false}
          />
        </div>
      ) : (
        fields.map((field) => (
          <div key={field.key} className="space-y-2">
            <Label htmlFor={`${id}-${field.key}`}>
              {String(
                field.schema.title ??
                  (renderer === COMPONENT_RENDERERS.BuiltinEntityClaimInfobox
                    ? INFOBOX_FIELD_LABELS[field.key]
                    : undefined) ??
                  field.key,
              )}
              {field.required ? " *" : ""}
            </Label>
            <FieldValue
              field={field}
              id={`${id}-${field.key}`}
              value={values[field.key]}
              onChange={(value) => {
                const next = { ...values };
                if (
                  value === undefined ||
                  (value === "" &&
                    !field.required &&
                    field.schema.default === undefined)
                )
                  delete next[field.key];
                else next[field.key] = value;
                onChange(JSON.stringify(next, null, 2));
              }}
            />
            {field.schema.description ? (
              <p className="text-xs text-muted-foreground">
                {String(field.schema.description)}
              </p>
            ) : null}
            {renderer === COMPONENT_RENDERERS.BuiltinEntityClaimInfobox &&
            field.key === "property_keys" ? (
              <p className="text-xs text-muted-foreground">
                每行填写一个属性键（如
                document_identifier）；留空展示全部已发布事实。
              </p>
            ) : null}
          </div>
        ))
      )}
      {fields?.length && values && !extraValues ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setAdvanced(!advanced)}
        >
          {advanced ? "返回表单" : "高级：编辑显示配置 JSON"}
        </Button>
      ) : null}
    </div>
  );
}

export function ComponentEntityPicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (id: string) => void;
}) {
  const { session } = useSession();
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 200);
    return () => clearTimeout(timer);
  }, [query]);
  const state = useSWR(
    session
      ? ["component:entities", session.actorId, session.wikiId, debounced]
      : null,
    () =>
      knowledgeApi().listEntities({
        q: debounced || undefined,
        status: "active",
        pageSize: 20,
      }),
  );
  const selected = useSWR(
    session && value
      ? ["component:entity", session.actorId, session.wikiId, value]
      : null,
    () => knowledgeApi().getEntity({ id: value }),
  );
  const selectedName =
    state.data?.items.find((item) => item.id === value)?.displayLabel ??
    selected.data?.labels.find((item) => item.isPrimary)?.label ??
    selected.data?.canonicalKey;
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">选择实体</p>
      <Command shouldFilter={false} label="搜索实体">
        <Command.Input
          aria-label="搜索实体"
          value={query}
          onValueChange={setQuery}
          placeholder="按名称搜索，无需填写 ID…"
          className="h-9 w-full rounded-lg border px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
        <Command.List className="mt-2 max-h-40 overflow-y-auto rounded-lg border p-1">
          {state.isLoading ? (
            <p className="p-2 text-xs text-muted-foreground">正在读取实体…</p>
          ) : null}
          {state.error ? (
            <div className="p-2 text-xs text-destructive">
              实体无法读取。
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => void state.mutate()}
              >
                <RefreshCw aria-hidden />
                重试
              </Button>
            </div>
          ) : null}
          {!state.isLoading && !state.error && !state.data?.items.length ? (
            <p className="p-2 text-xs text-muted-foreground">
              没有可选实体。可先在实体中心创建实体，再返回选择。
            </p>
          ) : null}
          {state.data?.items.map((item) => (
            <Command.Item
              key={item.id}
              value={item.id}
              onSelect={() => onChange(item.id)}
              className={`cursor-pointer rounded-md p-2 text-sm data-[selected=true]:bg-accent ${value === item.id ? "bg-primary/10" : ""}`}
            >
              <span className="break-words font-medium">
                {value === item.id ? "✓ " : ""}
                {item.displayLabel}
              </span>
              <span className="ml-2 text-xs text-muted-foreground">
                {item.entityType.name} · {item.claimCount} 条事实
              </span>
            </Command.Item>
          ))}
        </Command.List>
      </Command>
      {value ? (
        <p className="text-xs text-muted-foreground">
          {selected.error
            ? "选中的实体无法读取，请重新选择。"
            : `已选中实体${selectedName ? `：${selectedName}` : "，正在读取名称…"}`}
        </p>
      ) : null}
    </div>
  );
}
