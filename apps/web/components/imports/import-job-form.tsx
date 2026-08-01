"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { importsApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";
import { clientUUID } from "@/lib/client-uuid";
import { httpUrlSchema, safeHttpUrl } from "@/lib/http-url";

const sourceSchema = z.object({
  url: httpUrlSchema,
  title: z.string().trim().max(255).optional(),
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
  title: z.string().trim().max(255).optional(),
});

export function ImportJobForm() {
  const router = useRouter();
  const [sourceKind, setSourceKind] = useState<"url" | "upload">("url");
  const [url, setURL] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const parsed = sourceKind === "url"
      ? sourceSchema.safeParse({ url, title: title || undefined })
      : uploadSchema.safeParse({ file, title: title || undefined });
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
              config: { source: { kind: "url", url: safeHttpUrl(url)! }, title: title || undefined },
            },
          })
        : await importsApi().createImportUploadJob({ ...common, file: file!, title: title || undefined });
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
      <div className="space-y-2">
        <Label htmlFor="source-title">来源标题（可选）</Label>
        <Input id="source-title" value={title} onChange={(event) => setTitle(event.target.value)} maxLength={255} />
      </div>
      <Button type="submit" disabled={submitting}>{submitting ? "正在创建…" : "创建导入任务"}</Button>
    </form>
  );
}
