"use client";

import { useEffect, useRef, useState } from "react";
import { Command } from "cmdk";
import { CheckCircle2, FilePenLine, FilePlus2, FileText, Network, Search, Upload, X } from "lucide-react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import type { PageSearchHit } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { authApi, importsApi, searchApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { clientUUID } from "@/lib/client-uuid";
import { httpUrlSchema, safeHttpUrl } from "@/lib/http-url";
import { importRequestError } from "@/lib/import-error";
import { cn } from "@/lib/utils";

export type ImportPageTarget = Pick<PageSearchHit, "id" | "displayTitle">;

const planningFields = {
  title: z.string().trim().max(255).optional(),
  instructions: z.string().trim().max(4000).optional(),
  routeMode: z.enum(["auto", "force_create", "force_update"]),
  pageId: z.string().uuid().optional(),
  authorActorId: z.string().uuid().optional(),
};

function validatePlanning(
  value: {
    title?: string;
    routeMode: "auto" | "force_create" | "force_update";
    pageId?: string;
  },
  context: z.RefinementCtx,
) {
  if (value.routeMode === "force_create" && !value.title) {
    context.addIssue({ code: "custom", path: ["title"], message: "强制创建单页时必须填写页面标题" });
  }
  if (value.routeMode === "force_update" && !value.pageId) {
    context.addIssue({ code: "custom", path: ["pageId"], message: "更新指定页面时必须选择目标页面" });
  }
}

const sourceSchema = z.object({
  url: httpUrlSchema,
  ...planningFields,
}).superRefine(validatePlanning);

const UPLOAD_MIME_TYPES = [
  "text/markdown",
  "text/x-markdown",
  "text/html",
  "text/plain",
  "text/csv",
  "application/json",
  "application/pdf",
  "image/png",
  "image/jpeg",
];
const UPLOAD_ACCEPT = [".md", ".markdown", ".html", ".htm", ".txt", ".csv", ".json", ".pdf", ".png", ".jpg", ".jpeg", ...UPLOAD_MIME_TYPES].join(",");

const uploadFileSchema = z
  .custom<File>((value) => {
    if (typeof value !== "object" || value === null) return false;
    const candidate = value as { name?: unknown; size?: unknown; type?: unknown };
    return typeof candidate.name === "string" && typeof candidate.size === "number" && typeof candidate.type === "string";
  }, "请选择来源文件")
  .refine((file) => file.size > 0, "文件不能为空")
  .refine((file) => file.size <= 300 * 1024 * 1024, "文件不能超过 300 MiB")
  .refine(
    (file) => UPLOAD_MIME_TYPES.includes(file.type) || /\.(md|markdown|html?|txt|csv|json|pdf|png|jpe?g)$/i.test(file.name),
    "仅支持 Markdown、HTML、文本、JSON、CSV、PDF、PNG 或 JPEG",
  );

const uploadSchema = z.object({
  file: uploadFileSchema,
  ...planningFields,
}).superRefine(validatePlanning);

const SEARCH_DEBOUNCE_MS = 200;

const ROUTE_MODE_OPTIONS = [
  {
    value: "auto",
    label: "智能多页面",
    description: "理解来源主题，检索已有页面，并分别决定创建、更新、仅关联或忽略。",
    icon: Network,
  },
  {
    value: "force_create",
    label: "强制创建单页",
    description: "不拆分到已有页面，以指定标题生成一个新页面；标题冲突时停止并交由人工处理。",
    icon: FilePlus2,
  },
  {
    value: "force_update",
    label: "更新指定页面",
    description: "仅为选定页面规划新增或替换内容，不创建其他页面。",
    icon: FilePenLine,
  },
] as const;

function useDebouncedValue(value: string) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [value]);
  return debounced;
}

export function ImportPageTargetPicker({
  selected,
  onSelect,
}: {
  selected: ImportPageTarget | null;
  onSelect: (page: ImportPageTarget | null) => void;
}) {
  const { session } = useSession();
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query);
  const { data, isLoading } = useSWR(
    session && debouncedQuery.trim() ? ["import-page-target", session.actorId, session.wikiId, debouncedQuery] : null,
    ([, , , q]) => searchApi().searchPages({
      q,
      namespace: "main",
      fields: ["title", "alias"],
      limit: 8,
    }),
    { keepPreviousData: true },
  );

  if (selected) {
    return (
      <div className="flex items-center justify-between gap-3 rounded-lg border border-border p-3">
        <span className="min-w-0">
          <span className="block truncate text-sm font-medium">{selected.displayTitle}</span>
          <span className="block text-xs text-muted-foreground">将只为这个页面生成更新计划</span>
        </span>
        <Button type="button" size="icon" variant="ghost" title="清除目标页面" onClick={() => onSelect(null)}>
          <X aria-hidden />
        </Button>
      </div>
    );
  }

  return (
    <Command shouldFilter={false} label="搜索目标页面" className="rounded-lg border border-border p-2">
      <div className="flex items-center gap-2 px-2">
        <Search className="size-4 text-muted-foreground" aria-hidden />
        <Command.Input
          value={query}
          onValueChange={setQuery}
          placeholder="搜索要更新的页面"
          className="h-9 min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
      </div>
      {query.trim() ? (
        <Command.List className="mt-2 max-h-52 overflow-y-auto border-t border-border pt-2">
          {isLoading && !data ? <Command.Loading className="px-2 py-2 text-sm text-muted-foreground">搜索中…</Command.Loading> : null}
          {!isLoading && data?.items.length === 0 ? (
            <Command.Empty className="px-2 py-2 text-sm text-muted-foreground">没有匹配页面</Command.Empty>
          ) : null}
          {data?.items.map((hit) => (
            <Command.Item
              key={hit.id}
              value={hit.id}
              onSelect={() => {
                onSelect(hit);
                setQuery("");
              }}
              className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-2 text-sm data-[selected=true]:bg-accent"
            >
              <FileText className="size-4 shrink-0 text-muted-foreground" aria-hidden />
              <span className="truncate">{hit.displayTitle}</span>
            </Command.Item>
          ))}
        </Command.List>
      ) : null}
    </Command>
  );
}

export function ImportJobForm() {
  const router = useRouter();
  const { session } = useSession();
  const { data: actorChoices, error: actorError, mutate: reloadActors } = useSWR(
    session ? ["import-actors", session.actorId, session.wikiId] : null,
    () => authApi().listCLIMachineActors(),
  );
  const actors = actorChoices?.items.filter((actor) => actor.wikiId === session?.wikiId) ?? [];
  const [authorSelection, setAuthorSelection] = useState<{ account: string; wiki: string; id: string } | null>(null);
  const authorActorId = authorSelection?.account === session?.actorId && authorSelection?.wiki === session?.wikiId
    ? authorSelection?.id || undefined : undefined;
  const [sourceKind, setSourceKind] = useState<"url" | "upload">("url");
  const [url, setURL] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const [fileError, setFileError] = useState("");
  const [dragging, setDragging] = useState(false);
  const [title, setTitle] = useState("");
  const [instructions, setInstructions] = useState("");
  const [routeMode, setRouteMode] = useState<"auto" | "force_create" | "force_update">("auto");
  const [targetPage, setTargetPage] = useState<ImportPageTarget | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const selectFile = (selected: File) => {
    const parsed = uploadFileSchema.safeParse(selected);
    setFile(parsed.success ? parsed.data : null);
    setFileError(parsed.success ? "" : parsed.error.issues[0]?.message ?? "文件不合法");
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (authorActorId && !actors.some((actor) => actor.actorId === authorActorId && actor.status === "active")) {
      toast.error("选定的机器作者不可用，请刷新身份列表或选择其他作者");
      return;
    }
    const pageId = routeMode === "force_update" ? targetPage?.id : undefined;
    const parsed = sourceKind === "url"
      ? sourceSchema.safeParse({
        url, title: title || undefined, instructions: instructions || undefined,
        routeMode, pageId, authorActorId,
      })
      : uploadSchema.safeParse({
        file, title: title || undefined, instructions: instructions || undefined,
        routeMode, pageId, authorActorId,
      });
    if (!parsed.success) {
      if (sourceKind === "upload") {
        setFileError(parsed.error.issues.find((issue) => issue.path[0] === "file")?.message ?? "");
      }
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
                pageId,
                authorActorId,
              },
            },
          })
        : await importsApi().createImportUploadJob({
            ...common,
            file: file!,
            title: title || undefined,
            instructions: instructions || undefined,
            routeMode,
            pageId,
            authorActorId,
          });
      toast.success("导入任务已排队");
      router.push(`/imports/${job.id}`);
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("请先登录后再创建导入任务");
        router.push(LOGIN_PATH);
      } else {
        toast.error("创建导入任务失败", { description: await importRequestError(error, "请检查来源或稍后重试。") });
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} className="space-y-5 rounded-xl border border-border p-5">
      <div className="flex gap-2" aria-label="来源类型">
        <Button type="button" variant={sourceKind === "url" ? "default" : "outline"}
          aria-pressed={sourceKind === "url"} disabled={submitting} onClick={() => setSourceKind("url")}>公网 URL</Button>
        <Button type="button" variant={sourceKind === "upload" ? "default" : "outline"}
          aria-pressed={sourceKind === "upload"} disabled={submitting} onClick={() => setSourceKind("upload")}>上传文件</Button>
      </div>
      {sourceKind === "url" ? (
        <div className="space-y-2">
          <Label htmlFor="source-url">来源 URL</Label>
          <Input id="source-url" type="url" value={url} onChange={(event) => setURL(event.target.value)}
            placeholder="https://example.com/article" required />
          <p className="text-xs text-muted-foreground">仅允许 http/https 公网地址；重定向和目标 IP 会经过 SSRF 校验。</p>
        </div>
      ) : (
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="source-file" id="source-file-label">来源文件</Label>
            <span className="text-xs text-muted-foreground">最大 300 MiB</span>
          </div>
          <input
            ref={fileInput}
            id="source-file"
            type="file"
            accept={UPLOAD_ACCEPT}
            className="sr-only"
            tabIndex={-1}
            aria-invalid={Boolean(fileError)}
            aria-describedby={fileError ? "source-file-error source-file-help" : "source-file-help"}
            disabled={submitting}
            onChange={(event) => {
              const selected = event.target.files?.[0];
              if (selected) selectFile(selected);
              event.target.value = "";
            }}
          />
          <div
            aria-labelledby="source-file-label"
            aria-describedby={fileError ? "source-file-error source-file-help" : "source-file-help"}
            className={cn(
              "rounded-xl border border-dashed transition-colors",
              dragging ? "border-primary bg-primary/5" : "border-border bg-muted/20",
              fileError && "border-destructive/50",
            )}
            onDragOver={(event) => {
              event.preventDefault();
              if (!submitting && event.dataTransfer.types.includes("Files")) setDragging(true);
            }}
            onDragLeave={(event) => {
              if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false);
            }}
            onDrop={(event) => {
              event.preventDefault();
              setDragging(false);
              if (submitting) return;
              const dropped = event.dataTransfer.files;
              if (dropped.length !== 1) {
                setFile(null);
                setFileError("每次请选择一个来源文件");
                return;
              }
              selectFile(dropped[0]);
            }}
          >
            {file ? (
              <>
                <div className="flex items-center gap-3 p-4">
                  <span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                    <FileText className="size-5" aria-hidden />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium" title={file.name}>{file.name}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {/\.(md|markdown)$/i.test(file.name) ? "Markdown" : file.name.split(".").pop()?.toUpperCase() || "文件"}
                      {" · "}
                      {file.size < 1024
                        ? `${file.size} B`
                        : file.size < 1024 * 1024
                          ? `${(file.size / 1024).toFixed(1)} KiB`
                          : `${(file.size / (1024 * 1024)).toFixed(1)} MiB`}
                    </p>
                  </div>
                </div>
                <div className="flex items-center justify-between gap-2 border-t border-border/60 px-4 py-2">
                  <span className="flex items-center gap-1.5 text-xs text-muted-foreground" role="status">
                    <CheckCircle2 className="size-3.5 text-primary" aria-hidden />
                    {dragging ? "松开以更换文件" : "已选择"}
                  </span>
                  <div className="flex items-center gap-1">
                    <Button type="button" variant="ghost" size="sm" disabled={submitting}
                      onClick={() => fileInput.current?.click()}>更换文件</Button>
                    <Button type="button" variant="ghost" size="icon-sm" aria-label="移除来源文件" disabled={submitting}
                      onClick={() => {
                        setFile(null);
                        setFileError("");
                      }}>
                      <X aria-hidden />
                    </Button>
                  </div>
                </div>
              </>
            ) : (
              <div className="flex flex-col items-center gap-3 px-4 py-6 text-center">
                <span className="flex size-11 items-center justify-center rounded-xl border border-border bg-background text-muted-foreground">
                  <Upload className="size-5" aria-hidden />
                </span>
                <div className="space-y-1">
                  <p className="text-sm font-medium">{dragging ? "松开即可选择文件" : "拖放文件到这里"}</p>
                  <p className="text-xs text-muted-foreground">将文档或数据导入 Wiki，生成可审核的页面计划</p>
                </div>
                <Button type="button" variant="outline" disabled={submitting}
                  aria-describedby={fileError ? "source-file-error" : "source-file-help"}
                  onClick={() => fileInput.current?.click()}>选择文件</Button>
              </div>
            )}
          </div>
          {fileError ? <p id="source-file-error" role="alert" className="text-xs text-destructive">{fileError}</p> : null}
          <div id="source-file-help" className="flex flex-wrap gap-1.5 text-xs text-muted-foreground">
            {["Markdown / 文本", "HTML", "JSON / CSV", "PDF / 图片"].map((format) => (
              <span key={format} className="rounded-md bg-muted px-2 py-1">{format}</span>
            ))}
          </div>
          <p className="text-xs leading-5 text-muted-foreground">
            PDF 保留原生文本，并逐页识别表格、图表与图片；图片支持 PNG、JPEG。
          </p>
        </div>
      )}
      <div className="space-y-2">
        <Label htmlFor="import-author">提案作者</Label>
        <select id="import-author" value={authorActorId ?? ""} disabled={submitting}
          onChange={(event) => setAuthorSelection(session ? {
            account: session.actorId, wiki: session.wikiId, id: event.target.value,
          } : null)}
          className="h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm">
          <option value="">当前账号（人工作者）</option>
          {actors.map((actor) => (
            <option key={actor.actorId} value={actor.actorId} disabled={actor.status !== "active"}>
              {actor.displayName} · {actor.actorType === "import" ? "导入 Agent" : "AI Agent"}{actor.status !== "active" ? "（已停用）" : ""}
            </option>
          ))}
        </select>
        <p className="text-xs leading-5 text-muted-foreground">
          机器作者使用其信任与抽样策略，最终仍需真人审核。任务和来源由你管理，重试保留作者。
        </p>
        {actorError ? (
          <p role="alert" className="text-xs text-destructive">
            机器身份读取失败。<button type="button" className="ml-1 underline" onClick={() => void reloadActors()}>重试</button>
          </p>
        ) : actors.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            可在<Link href="/governance/ai-trust" className="mx-1 underline">AI 信任</Link>创建属于当前账户的机器 Actor。
          </p>
        ) : null}
      </div>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">页面路由方式</legend>
        <div className="grid gap-2">
          {ROUTE_MODE_OPTIONS.map((option) => {
            const selected = routeMode === option.value;
            const Icon = option.icon;
            return (
              <button
                key={option.value}
                type="button"
                aria-pressed={selected}
                onClick={() => setRouteMode(option.value)}
                className={cn(
                  "grid min-h-20 w-full grid-cols-[1.25rem_minmax(0,1fr)_1rem] items-start gap-3 rounded-lg border px-3 py-3 text-left transition-colors",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
                  selected
                    ? "border-primary bg-primary/5"
                    : "border-border hover:border-foreground/20 hover:bg-muted/50",
                )}
              >
                <Icon
                  className={cn(
                    "mt-0.5 size-5",
                    selected ? "text-primary" : "text-muted-foreground",
                  )}
                  aria-hidden
                />
                <span className="min-w-0">
                  <span className="block text-sm font-semibold">{option.label}</span>
                  <span className="mt-0.5 block text-xs leading-5 text-muted-foreground">
                    {option.description}
                  </span>
                </span>
                <span
                  className={cn(
                    "mt-0.5 flex size-4 items-center justify-center rounded-full border",
                    selected ? "border-primary" : "border-muted-foreground/40",
                  )}
                  aria-hidden
                >
                  {selected ? <span className="size-2 rounded-full bg-primary" /> : null}
                </span>
              </button>
            );
          })}
        </div>
      </fieldset>
      {routeMode === "force_update" ? (
        <div className="space-y-2">
          <Label>目标页面</Label>
          <ImportPageTargetPicker selected={targetPage} onSelect={setTargetPage} />
        </div>
      ) : null}
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
