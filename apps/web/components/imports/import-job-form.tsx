"use client";

import { useState } from "react";
import { FilePlus2, Network } from "lucide-react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { importsApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";
import { clientUUID } from "@/lib/client-uuid";
import { httpUrlSchema, safeHttpUrl } from "@/lib/http-url";

const planningFields = {
  title: z.string().trim().max(255).optional(),
  instructions: z.string().trim().max(4000).optional(),
  routeMode: z.enum(["auto", "force_create"]),
};

const sourceSchema = z.object({
  url: httpUrlSchema,
  ...planningFields,
}).superRefine((value, context) => {
  if (value.routeMode === "force_create" && !value.title) {
    context.addIssue({ code: "custom", path: ["title"], message: "强制创建单页时必须填写页面标题" });
  }
});

const uploadSchema = z.object({
  file: z
    .custom<File>((value) => {
      if (typeof value !== "object" || value === null) return false;
      const candidate = value as { name?: unknown; size?: unknown; type?: unknown };
      return typeof candidate.name === "string" && typeof candidate.size === "number" && typeof candidate.type === "string";
    }, "请选择来源文件")
    .refine((file) => file.size <= 10 * 1024 * 1024, "文件不能超过 10 MiB")
    .refine(
      (file) =>
        [
          "text/html",
          "text/plain",
          "text/csv",
          "application/json",
          "application/pdf",
          "image/png",
          "image/jpeg",
        ].includes(file.type) ||
        /\.(html?|txt|csv|json|pdf|png|jpe?g)$/i.test(file.name),
      "仅支持 HTML、文本、JSON、CSV、PDF、PNG 或 JPEG",
    ),
  ...planningFields,
}).superRefine((value, context) => {
  if (value.routeMode === "force_create" && !value.title) {
    context.addIssue({ code: "custom", path: ["title"], message: "强制创建单页时必须填写页面标题" });
  }
});

export function ImportJobForm() {
  const router = useRouter();
  const [sourceKind, setSourceKind] = useState<"url" | "upload">("url");
  const [url, setURL] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [instructions, setInstructions] = useState("");
  const [routeMode, setRouteMode] = useState<"auto" | "force_create">("auto");
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const parsed = sourceKind === "url"
      ? sourceSchema.safeParse({ url, title: title || undefined, instructions: instructions || undefined, routeMode })
      : uploadSchema.safeParse({ file, title: title || undefined, instructions: instructions || undefined, routeMode });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "来源参数不合法");
      return;
    }
    setSubmitting(true);
    try {
      const common = { idempotencyKey: clientUUID() };
      const job = sourceKind === "url"
        ? await importsApi().createImportJob({
            ...common,
            createImportJobRequest: {
              jobType: "source_import",
              config: {
                source: { kind: "url", url: safeHttpUrl(url)! },
                title: title || undefined,
                instructions: instructions || undefined,
                routeMode,
              },
            },
          })
        : await importsApi().createImportUploadJob({
            ...common,
            file: file!,
            title: title || undefined,
            instructions: instructions || undefined,
            routeMode,
          });
      toast.success("导入任务已排队");
      router.push(`/imports/${job.id}`);
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("请先登录后再创建导入任务");
        router.push(LOGIN_PATH);
      } else {
        toast.error("创建导入任务失败", { description: "请检查来源地址或稍后重试。" });
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} className="space-y-5 rounded-xl border border-border p-5">
      <div className="flex gap-2" aria-label="来源类型">
        <Button type="button" variant={sourceKind === "url" ? "default" : "outline"}
          aria-pressed={sourceKind === "url"} onClick={() => setSourceKind("url")}>公网 URL</Button>
        <Button type="button" variant={sourceKind === "upload" ? "default" : "outline"}
          aria-pressed={sourceKind === "upload"} onClick={() => setSourceKind("upload")}>上传文件</Button>
      </div>
      {sourceKind === "url" ? (
        <div className="space-y-2">
          <Label htmlFor="source-url">来源 URL</Label>
          <Input id="source-url" type="url" value={url} onChange={(event) => setURL(event.target.value)}
            placeholder="https://example.com/article" required />
          <p className="text-xs text-muted-foreground">仅允许 http/https 公网地址；重定向和目标 IP 会经过 SSRF 校验。</p>
        </div>
      ) : (
        <div className="space-y-2">
          <Label htmlFor="source-file">来源文件</Label>
          <Input id="source-file" type="file" accept=".html,.htm,.txt,.csv,.json,.pdf,.png,.jpg,.jpeg,text/html,text/plain,text/csv,application/json,application/pdf,image/png,image/jpeg"
            onChange={(event) => setFile(event.target.files?.[0] ?? null)} required />
          <p className="text-xs text-muted-foreground">支持网页、文本、JSON/CSV 数据、PDF 与图片，最大 10 MiB；扫描件和图片会执行受限 OCR。</p>
        </div>
      )}
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">页面路由方式</legend>
        <div className="grid gap-3 sm:grid-cols-2">
          <button
            type="button"
            aria-pressed={routeMode === "auto"}
            onClick={() => setRouteMode("auto")}
            className={`rounded-xl border p-4 text-left transition ${routeMode === "auto" ? "border-primary bg-primary/5 ring-1 ring-primary" : "border-border hover:bg-muted/50"}`}
          >
            <span className="flex items-center gap-2 text-sm font-semibold"><Network className="size-4" aria-hidden />智能多页面</span>
            <span className="mt-1 block text-xs leading-5 text-muted-foreground">理解来源主题，检索已有页面，并分别决定创建、更新、仅关联或忽略。</span>
          </button>
          <button
            type="button"
            aria-pressed={routeMode === "force_create"}
            onClick={() => setRouteMode("force_create")}
            className={`rounded-xl border p-4 text-left transition ${routeMode === "force_create" ? "border-primary bg-primary/5 ring-1 ring-primary" : "border-border hover:bg-muted/50"}`}
          >
            <span className="flex items-center gap-2 text-sm font-semibold"><FilePlus2 className="size-4" aria-hidden />强制创建单页</span>
            <span className="mt-1 block text-xs leading-5 text-muted-foreground">不拆分到已有页面，以指定标题生成一个新页面；标题冲突时停止并交由人工处理。</span>
          </button>
        </div>
      </fieldset>
      <div className="space-y-2">
        <Label htmlFor="source-title">{routeMode === "force_create" ? "新页面标题" : "建议页面标题（可选）"}</Label>
        <Input id="source-title" value={title} onChange={(event) => setTitle(event.target.value)} maxLength={255}
          required={routeMode === "force_create"} placeholder={routeMode === "force_create" ? "必须填写且不能与现有标题冲突" : "作为页面检索与路由的优先线索"} />
      </div>
      <div className="space-y-2">
        <Label htmlFor="source-instructions">导入要求（可选）</Label>
        <Textarea id="source-instructions" value={instructions} onChange={(event) => setInstructions(event.target.value)}
          maxLength={4000} rows={4} placeholder="例如：重点补充角色经历；忽略营销段落；分别整理人物与组织页面。" />
        <p className="text-xs text-muted-foreground">要求只参与来源理解与页面路由，不能覆盖证据核验、安全扫描和治理审核。</p>
      </div>
      <Button type="submit" disabled={submitting}>{submitting ? "正在创建…" : "创建导入任务"}</Button>
    </form>
  );
}
