"use client";

import { useState } from "react";

import { CheckCircle2, LoaderCircle, Send } from "lucide-react";
import { toast } from "sonner";
import useSWR from "swr";

import type { PublicDatasetForm } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { datasetsApi } from "@/lib/api";

function formValues(
  form: PublicDatasetForm,
  raw: Record<string, string>,
): Record<string, string | number | boolean> | null {
  const values: Record<string, string | number | boolean> = {};
  for (const field of form.fields) {
    const value = raw[field.key] ?? "";
    if (!value && field.required) return null;
    if (!value) continue;
    if (field.type === "number" || field.type === "integer") {
      const number = Number(value);
      if (
        !Number.isFinite(number) ||
        (field.type === "integer" && !Number.isInteger(number))
      ) {
        return null;
      }
      values[field.key] = number;
    } else if (field.type === "boolean") {
      values[field.key] = value === "true";
    } else {
      values[field.key] = value;
    }
  }
  return values;
}

export function PublicDatasetFormView({ id }: { id: string }) {
  const formState = useSWR<PublicDatasetForm>(["public-dataset-form", id], () =>
    datasetsApi().getPublicDatasetForm({ id }),
  );
  const [raw, setRaw] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  if (formState.isLoading) {
    return <div className="h-96 animate-pulse rounded-md border bg-muted/35" />;
  }
  if (formState.error || !formState.data) {
    return (
      <div className="rounded-md border border-destructive/20 bg-destructive/5 p-8">
        <h1 className="text-xl font-semibold">表单不可用</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          表单可能未启用或已被移除。
        </p>
      </div>
    );
  }
  const form = formState.data;

  const submit = async () => {
    const values = formValues(form, raw);
    if (!values) {
      toast.error("请补齐必填字段并检查数字格式");
      return;
    }
    setSubmitting(true);
    try {
      await datasetsApi().submitPublicDatasetForm({
        id,
        submitPublicDatasetFormRequest: { values },
      });
      setSubmitted(true);
      setRaw({});
    } catch {
      toast.error("提交失败", {
        description: "请检查字段内容后重试。",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section className="mx-auto max-w-2xl">
      <header className="border-b pb-6">
        <p className="text-xs font-semibold tracking-[0.14em] text-cyan-700 uppercase">
          Dataset form
        </p>
        <h1 className="mt-2 text-3xl font-semibold">{form.name}</h1>
      </header>

      {submitted ? (
        <div className="mt-6 flex items-start gap-3 rounded-md border border-emerald-200 bg-emerald-50 p-5 text-emerald-900">
          <CheckCircle2 className="mt-0.5 size-5 shrink-0" />
          <div>
            <p className="font-medium">{form.successMessage}</p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="mt-4"
              onClick={() => setSubmitted(false)}
            >
              再次填写
            </Button>
          </div>
        </div>
      ) : (
        <form
          className="mt-6 space-y-5"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          {form.fields.map((field) => (
            <div key={field.key} className="space-y-2">
              <Label htmlFor={`public-form-${field.key}`}>
                {field.title}
                {field.required ? (
                  <span className="ml-1 text-destructive">*</span>
                ) : null}
              </Label>
              {field.type === "boolean" ? (
                <select
                  id={`public-form-${field.key}`}
                  required={field.required}
                  value={raw[field.key] ?? ""}
                  onChange={(event) =>
                    setRaw((current) => ({
                      ...current,
                      [field.key]: event.target.value,
                    }))
                  }
                  className="h-9 w-full rounded-lg border bg-background px-3 text-sm"
                >
                  <option value="">请选择</option>
                  <option value="true">是</option>
                  <option value="false">否</option>
                </select>
              ) : (
                <Input
                  id={`public-form-${field.key}`}
                  required={field.required}
                  type={
                    field.format === "date"
                      ? "date"
                      : field.type === "number" || field.type === "integer"
                        ? "number"
                        : "text"
                  }
                  step={field.type === "integer" ? "1" : "any"}
                  value={raw[field.key] ?? ""}
                  onChange={(event) =>
                    setRaw((current) => ({
                      ...current,
                      [field.key]: event.target.value,
                    }))
                  }
                />
              )}
            </div>
          ))}
          <Button type="submit" disabled={submitting}>
            {submitting ? (
              <LoaderCircle className="animate-spin" />
            ) : (
              <Send />
            )}
            {form.submitLabel}
          </Button>
        </form>
      )}
    </section>
  );
}
