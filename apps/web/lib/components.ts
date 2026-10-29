import { z } from "zod";

import {
  WikiComponentVersionRendererRefEnum,
  type WikiComponentVersion,
} from "../../../contracts/generated/typescript";

export const COMPONENT_RENDERERS = WikiComponentVersionRendererRefEnum;
export type ComponentRenderer = WikiComponentVersion["rendererRef"];
export type ComponentSchema = Record<string, unknown>;

const objectSchema = z.record(z.string(), z.unknown());

export function parseComponentObject(source: string): ComponentSchema | null {
  try {
    const result = objectSchema.safeParse(JSON.parse(source));
    return result.success ? result.data : null;
  } catch {
    return null;
  }
}

export interface ComponentField {
  key: string;
  schema: ComponentSchema;
  required: boolean;
}

// Only schemas fully represented by the form may be edited without JSON.
// In particular, unions, nested objects and validation constraints stay intact.
export function componentFields(
  schema: ComponentSchema,
): ComponentField[] | null {
  const rootKeys = [
    "type",
    "properties",
    "required",
    "additionalProperties",
    "title",
    "description",
    "$schema",
  ];
  if (
    schema.type !== "object" ||
    Object.keys(schema).some((key) => !rootKeys.includes(key)) ||
    (schema.additionalProperties !== undefined &&
      typeof schema.additionalProperties !== "boolean")
  )
    return null;
  const properties = objectSchema.safeParse(schema.properties ?? {});
  const required = z.array(z.string()).safeParse(schema.required ?? []);
  if (!properties.success || !required.success) return null;
  if (required.data.some((key) => !Object.hasOwn(properties.data, key)))
    return null;
  const fields: ComponentField[] = [];
  for (const [key, raw] of Object.entries(properties.data)) {
    const field = objectSchema.safeParse(raw);
    if (!field.success) return null;
    const definition = field.data;
    const allowed = ["type", "title", "description", "default"];
    if (definition.type === "array") {
      allowed.push("items", "uniqueItems");
      const items = objectSchema.safeParse(definition.items);
      if (
        !items.success ||
        items.data.type !== "string" ||
        Object.keys(items.data).length !== 1
      )
        return null;
      if (
        definition.uniqueItems !== undefined &&
        typeof definition.uniqueItems !== "boolean"
      )
        return null;
    } else if (
      !["string", "number", "integer", "boolean"].includes(
        String(definition.type),
      )
    ) {
      return null;
    }
    if (Object.keys(definition).some((name) => !allowed.includes(name)))
      return null;
    if (
      ["title", "description"].some(
        (name) =>
          definition[name] !== undefined &&
          typeof definition[name] !== "string",
      )
    )
      return null;
    if (definition.default !== undefined) {
      const defaults = {
        string: z.string(),
        number: z.number(),
        integer: z.number().int(),
        boolean: z.boolean(),
        array: z.array(z.string()),
      };
      if (
        !defaults[definition.type as keyof typeof defaults].safeParse(
          definition.default,
        ).success
      )
        return null;
    }
    fields.push({
      key,
      schema: definition,
      required: required.data.includes(key),
    });
  }
  return fields;
}

export function componentDefaultProps(
  schema: ComponentSchema,
  renderer: ComponentRenderer,
): ComponentSchema {
  const properties = objectSchema.safeParse(schema.properties);
  const required = z.array(z.string()).safeParse(schema.required);
  const result: ComponentSchema = Object.create(null);
  if (!properties.success) return result;
  for (const [key, raw] of Object.entries(properties.data)) {
    const field = objectSchema.safeParse(raw);
    if (!field.success) continue;
    if (field.data.default !== undefined) {
      result[key] = field.data.default;
    } else if (required.success && required.data.includes(key)) {
      switch (field.data.type) {
        case "string":
          result[key] = "";
          break;
        case "number":
        case "integer":
          result[key] = 0;
          break;
        case "boolean":
          result[key] = false;
          break;
        case "array":
          result[key] = [];
          break;
      }
    }
  }
  if (renderer === COMPONENT_RENDERERS.BuiltinEntityClaimInfobox) {
    const propertyKeys = objectSchema.safeParse(properties.data.property_keys);
    if (
      propertyKeys.success &&
      propertyKeys.data.type === "array" &&
      result.property_keys === undefined
    ) {
      result.property_keys = [];
    }
  }
  return result;
}

export interface ComponentPreset {
  key: string;
  name: string;
  description: string;
  renderer: ComponentRenderer;
  schema: ComponentSchema;
  sample: Record<string, string>;
}

function keyValuePreset(
  key: string,
  name: string,
  description: string,
  sample: Record<string, string>,
): ComponentPreset {
  return {
    key,
    name,
    description,
    sample,
    renderer: COMPONENT_RENDERERS.BuiltinKeyValue,
    schema: {
      type: "object",
      properties: Object.fromEntries(
        Object.entries(sample).map(([label, value]) => [
          label,
          { type: "string", default: value },
        ]),
      ),
      additionalProperties: false,
    },
  };
}

export const COMPONENT_PRESETS: ComponentPreset[] = [
  keyValuePreset(
    "character-card",
    "角色资料卡",
    "整理角色的所属阵营、身份与登场作品。",
    {
      名称: "安比",
      所属: "狡兔屋",
      身份: "委托执行者",
      登场作品: "绝区零",
    },
  ),
  keyValuePreset(
    "organization-card",
    "组织概况卡",
    "汇总组织的类型、成立时间与主要职责。",
    {
      名称: "星港研究院",
      类型: "研究机构",
      成立时间: "2042 年",
      主要职责: "深空通信研究",
    },
  ),
  keyValuePreset(
    "software-card",
    "软件资料卡",
    "记录版本、许可证与支持平台。",
    {
      名称: "Atlas Editor",
      版本: "2.4.0",
      许可证: "MIT",
      支持平台: "macOS / Linux / Windows",
    },
  ),
  keyValuePreset("protocol-card", "协议摘要卡", "展示文档编号、类别和状态。", {
    名称: "JSON",
    文档编号: "RFC 8259",
    类别: "数据交换格式",
    状态: "Internet Standard",
  }),
  {
    key: "entity-infobox",
    name: "动态实体信息框",
    description:
      "选择真实实体，自动展示已发布事实；标题、语言和属性范围可配置。",
    renderer: COMPONENT_RENDERERS.BuiltinEntityClaimInfobox,
    schema: {
      type: "object",
      properties: {
        title: { type: "string", title: "标题", default: "" },
        language: { type: "string", title: "标签语言", default: "zh" },
        property_keys: {
          type: "array",
          title: "展示属性",
          items: { type: "string" },
          uniqueItems: true,
          default: [],
        },
      },
      additionalProperties: false,
    },
    sample: {
      文档编号: "RFC 8259",
      类别: "数据交换格式",
      发布组织: "IETF",
      状态: "Internet Standard",
    },
  },
];
