import type { Dataset } from "../../../contracts/generated/typescript";

export type DatasetFieldType = "string" | "number" | "integer" | "boolean";

export interface DatasetField {
  key: string;
  title: string;
  type: DatasetFieldType;
  required: boolean;
}

function objectValue(value: unknown): Record<string, unknown> | null {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

export function datasetFields(dataset: Dataset): DatasetField[] {
  const properties = objectValue(dataset.schema.properties) ?? {};
  const required = new Set(
    Array.isArray(dataset.schema.required)
      ? dataset.schema.required.filter(
          (value): value is string => typeof value === "string",
        )
      : [],
  );
  return Object.entries(properties).flatMap(([key, raw]) => {
    const property = objectValue(raw);
    const type = property?.type;
    if (
      type !== "string" &&
      type !== "number" &&
      type !== "integer" &&
      type !== "boolean"
    ) {
      return [];
    }
    return [
      {
        key,
        type,
        required: required.has(key),
        title:
          typeof property?.title === "string" && property.title.trim()
            ? property.title
            : key,
      },
    ];
  });
}

export function displayDatasetValue(value: unknown): string {
  if (value == null) return "—";
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "string" || typeof value === "number") {
    return String(value);
  }
  return JSON.stringify(value);
}
