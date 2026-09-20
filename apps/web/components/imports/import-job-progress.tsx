"use client";

import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  CircleDashed,
  CircleX,
  FilePlus2,
  FilePenLine,
  Gauge,
  Link2,
  LoaderCircle,
  MinusCircle,
  RotateCcw,
  ShieldCheck,
  SkipForward,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRMutation from "swr/mutation";

import {
  ResponseError,
  type ImportPlanningInput,
} from "../../../../contracts/generated/typescript";

import {
  ImportPageTargetPicker,
  type ImportPageTarget,
} from "@/components/imports/import-job-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { importsApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";
import { clientUUID } from "@/lib/client-uuid";
import { cn } from "@/lib/utils";

const STAGES = ["fetch", "parse", "extract", "plan", "match", "compose", "review"] as const;
const LABELS: Record<string, string> = {
  fetch: "获取与安全扫描", parse: "解析与分块", extract: "事实候选抽取",
  plan: "来源理解与多页面路由", match: "实体与 Claim 匹配", compose: "页面与知识 Proposal 合成", review: "进入审核",
};

const ROUTE_META = {
  create: { label: "创建页面", icon: FilePlus2, className: "text-emerald-700 bg-emerald-50 dark:bg-emerald-950/40 dark:text-emerald-300" },
  update: { label: "更新页面", icon: FilePenLine, className: "text-sky-700 bg-sky-50 dark:bg-sky-950/40 dark:text-sky-300" },
  link: { label: "建立页面关联", icon: Link2, className: "text-violet-700 bg-violet-50 dark:bg-violet-950/40 dark:text-violet-300" },
  ignore: { label: "忽略", icon: MinusCircle, className: "text-muted-foreground bg-muted" },
} as const;

const STAGE_STATUS_META = {
  pending: {
    label: "等待处理",
    icon: CircleDashed,
    className: "bg-muted text-muted-foreground ring-border",
  },
  running: {
    label: "处理中",
    icon: LoaderCircle,
    className: "bg-sky-50 text-sky-700 ring-sky-200 dark:bg-sky-950/50 dark:text-sky-300 dark:ring-sky-800",
  },
  succeeded: {
    label: "已完成",
    icon: CheckCircle2,
    className: "bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950/50 dark:text-emerald-300 dark:ring-emerald-800",
  },
  failed: {
    label: "失败",
    icon: CircleX,
    className: "bg-rose-50 text-rose-700 ring-rose-200 dark:bg-rose-950/50 dark:text-rose-300 dark:ring-rose-800",
  },
  skipped: {
    label: "已跳过",
    icon: SkipForward,
    className: "bg-slate-100 text-slate-600 ring-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:ring-slate-700",
  },
  cancelled: {
    label: "已取消",
    icon: Ban,
    className: "bg-amber-50 text-amber-700 ring-amber-200 dark:bg-amber-950/50 dark:text-amber-300 dark:ring-amber-800",
  },
} as const;

function StageStatusIcon({ status }: { status: keyof typeof STAGE_STATUS_META }) {
  const meta = STAGE_STATUS_META[status];
  const Icon = meta.icon;

  return (
    <span
      className={cn(
        "inline-flex size-7 shrink-0 items-center justify-center rounded-full ring-1 ring-inset",
        meta.className,
      )}
      title={meta.label}
    >
      <Icon
        className={cn(
          "size-3.5",
          status === "running" && "motion-safe:animate-spin",
        )}
        aria-hidden
      />
      <span className="sr-only">{meta.label}</span>
    </span>
  );
}
const ERROR_MESSAGES: Record<string, string> = {
  parse_failed: "来源内容无法解析。",
  pdf_extractor_unavailable: "服务器缺少 PDF 文本提取组件，请联系管理员。",
  pdf_text_too_large: "PDF 解压后的文本超过处理上限。",
  pdf_rasterizer_unavailable: "服务器缺少 PDF OCR 栅格化组件，请联系管理员。",
  pdf_ocr_page_limit_exceeded: "扫描 PDF 超过单次 OCR 的 20 页安全上限。",
  ocr_unavailable: "服务器缺少 OCR 组件或语言数据，请联系管理员。",
  ocr_image_too_large: "图片像素或 PDF 栅格页超过 OCR 安全上限。",
  ocr_text_too_large: "OCR 输出超过处理上限。",
  ocr_no_text: "图片中没有识别到可抽取文本。",
  ocr_failed: "OCR 执行失败，请确认图片清晰度或联系管理员。",
  extraction_failed: "结构化抽取失败，请稍后重试。",
  extraction_invalid_output: "模型返回的结构不符合抽取 Schema，请重试或更换模型。",
  extraction_output_truncated: "模型输出达到长度上限，请减少单次来源内容或使用支持更长输出的模型。",
  extraction_provider_failed: "模型供应商调用失败，请检查模型、额度和 API Key。",
  extraction_timeout: "模型调用超时，请稍后重试。",
  extraction_evidence_invalid: "模型返回的引用无法与来源文本核对。",
  page_plan_failed: "页面规划失败，请稍后重试。",
  page_plan_invalid_output: "模型返回的页面规划结构不合法，已完成自适应拆分仍无法修复。",
  page_plan_output_truncated: "页面规划输出达到模型长度上限，请提高输出上限或缩小来源。",
  page_plan_provider_failed: "页面规划模型调用失败，请检查模型、额度和 API Key。",
  page_plan_timeout: "页面规划模型调用超时，请稍后重试。",
  page_plan_evidence_invalid: "页面规划引用的原文无法通过逐字核验。",
  page_plan_target_conflict: "规划的新页面标题已存在，或目标页面在规划期间发生冲突。",
  page_plan_quality_gate: "来源理解质量未达到当前 AI 配置阈值。",
  no_page_plan: "来源有价值，但没有形成可审核的页面写入计划。",
  no_reviewable_proposal: "候选已完成匹配，但没有形成可写入的审核操作。",
};

const JOB_STATUS_LABEL = {
  queued: "排队中",
  running: "处理中",
  action_required: "等待你的确认",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
} as const;

const QUALITY_DIMENSIONS = [
  ["fidelity", "原文保真"],
  ["grounding", "证据支撑"],
  ["structure", "文章结构"],
  ["concision", "去重精炼"],
  ["routing", "路由置信"],
] as const;

type RouteMode = "auto" | "force_create" | "force_update";

function ReplanPanel({
  id,
  planningInput,
  onReplanned,
}: {
  id: string;
  planningInput: ImportPlanningInput;
  onReplanned: () => Promise<void>;
}) {
  const router = useRouter();
  const initialRouteMode = planningInput.routeMode;
  const [title, setTitle] = useState(planningInput.title ?? "");
  const [instructions, setInstructions] = useState(planningInput.instructions ?? "");
  const [routeMode, setRouteMode] = useState<RouteMode>(
    initialRouteMode === "force_create" || initialRouteMode === "force_update"
      ? initialRouteMode
      : "auto",
  );
  const [targetPage, setTargetPage] = useState<ImportPageTarget | null>(
    planningInput.pageId
      ? {
        id: planningInput.pageId,
        displayTitle: planningInput.title?.trim() || planningInput.pageId,
      }
      : null,
  );
  const [submitting, setSubmitting] = useState(false);

  const submit = async () => {
    if (routeMode === "force_create" && !title.trim()) {
      toast.error("强制创建单页时必须填写页面标题");
      return;
    }
    if (routeMode === "force_update" && !targetPage) {
      toast.error("更新指定页面时必须选择目标页面");
      return;
    }
    const pageId = routeMode === "force_update" ? targetPage?.id : undefined;
    setSubmitting(true);
    try {
      await importsApi().replanImportJob({
        id,
        idempotencyKey: clientUUID(),
        replanImportJobRequest: {
          title: title.trim(),
          instructions: instructions.trim(),
          routeMode,
          pageId,
        },
      });
      await onReplanned();
      toast.success("当前任务已重新排队");
    } catch (replanError) {
      if (isUnauthorized(replanError)) {
        router.push(LOGIN_PATH);
      } else {
        toast.error("重新规划失败");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section className="space-y-4 border-t border-border pt-5">
      <div>
        <h3 className="text-sm font-semibold">调整导入意图</h3>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          保留已校验的来源和抽取结果，在当前任务内追加新的计划版本。
        </p>
      </div>
      <div className="grid gap-2 sm:grid-cols-3">
        {([
          ["auto", "智能多页面"],
          ["force_create", "创建单页"],
          ["force_update", "更新指定页"],
        ] as const).map(([value, label]) => (
          <button
            key={value}
            type="button"
            aria-pressed={routeMode === value}
            onClick={() => setRouteMode(value)}
            className={cn(
              "rounded-lg border px-3 py-2 text-sm",
              routeMode === value ? "border-primary bg-primary/5 font-medium" : "border-border",
            )}
          >
            {label}
          </button>
        ))}
      </div>
      {routeMode === "force_update" ? (
        <div className="space-y-2">
          <Label>目标页面</Label>
          <ImportPageTargetPicker selected={targetPage} onSelect={setTargetPage} />
        </div>
      ) : null}
      <div className="space-y-2">
        <Label htmlFor="replan-title">{routeMode === "force_create" ? "新页面标题" : "建议标题"}</Label>
        <Input id="replan-title" value={title} maxLength={255} onChange={(event) => setTitle(event.target.value)} />
      </div>
      <div className="space-y-2">
        <Label htmlFor="replan-instructions">导入要求</Label>
        <Textarea id="replan-instructions" value={instructions} maxLength={4000} rows={4}
          onChange={(event) => setInstructions(event.target.value)} />
      </div>
      <Button type="button" variant="outline" disabled={submitting} onClick={() => void submit()}>
        <RotateCcw aria-hidden />
        {submitting ? "正在创建…" : "重新规划"}
      </Button>
    </section>
  );
}

function importErrorMessage(error: unknown) {
  if (!error || typeof error !== "object" || !("code" in error)) return null;
  const code = (error as { code?: unknown }).code;
  return typeof code === "string" ? ERROR_MESSAGES[code] ?? null : null;
}

export function ImportJobProgress({ id }: { id: string }) {
  const router = useRouter();
  const { data, error, mutate } = useSWR(
    ["import-job", id],
    () => importsApi().getImportJob({ id }),
    { refreshInterval: (latest) => latest?.job.status === "queued" || latest?.job.status === "running" ? 1500 : 0 },
  );
  const { trigger, isMutating } = useSWRMutation(
    ["import-job-action", id],
    (_key, { arg }: { arg: "cancel" | "retry" | "confirm" }) => {
      if (arg === "cancel") return importsApi().cancelImportJob({ id });
      if (arg === "confirm") {
        if (!data?.job.currentPlanId) {
          throw new Error("missing current plan");
        }
        return importsApi().confirmImportPlan({
          id,
          confirmImportPlanRequest: { planId: data.job.currentPlanId },
        });
      }
      return importsApi().retryImportJob({ id });
    },
  );
  const act = async (action: "cancel" | "retry" | "confirm") => {
    try {
      await trigger(action);
      await mutate();
      toast.success(
        action === "cancel"
          ? "任务已取消"
          : action === "confirm"
            ? "计划已确认，正在生成审核提案"
            : "任务已重新排队",
      );
    } catch (actionError) {
      if (isUnauthorized(actionError)) {
        toast.error("请先登录后再操作导入任务");
        router.push(LOGIN_PATH);
      } else {
        toast.error("任务操作失败");
      }
    }
  };

  if (isUnauthorized(error)) {
    return (
      <p className="rounded-lg border border-dashed p-5 text-sm text-muted-foreground">
        请先<Link className="mx-1 underline" href={LOGIN_PATH}>登录</Link>后查看导入任务。
      </p>
    );
  }
  if (error instanceof ResponseError && error.response.status === 403) {
    return <p className="rounded-lg border border-destructive/30 p-5 text-sm text-destructive">当前账号无权读取这个导入任务。</p>;
  }
  if (error) return <p className="rounded-lg border border-destructive/30 p-5 text-sm text-destructive">导入任务响应解析失败，请刷新页面；如果问题持续出现，请联系管理员。</p>;
  if (!data) return <p className="text-sm text-muted-foreground">正在加载导入进度…</p>;
  const latestByStage = new Map(data.stages.map((stage) => [stage.stage, stage]));
  const errorMessage = importErrorMessage(data.job.error);
  const planningInput = data.job.planningInput;
  const canReplan = Boolean(data.job.sourceVersionId) &&
    !data.job.proposalId && !["queued", "running"].includes(data.job.status);
  const currentPlanVersion = data.plans.find((item) => item.id === data.job.currentPlanId)
    ?? data.plans.at(-1);

  return (
    <div className="space-y-6">
      <section className="rounded-xl border border-border p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="text-sm font-semibold">{JOB_STATUS_LABEL[data.job.status]} · {data.job.progress}%</p>
            <p className="mt-1 font-mono text-xs text-muted-foreground">{data.job.id}</p>
          </div>
          <div className="flex gap-2">
            {(data.job.status === "queued" || data.job.status === "running" || data.job.status === "action_required") &&
              <Button variant="destructive" disabled={isMutating} onClick={() => void act("cancel")}>取消</Button>}
            {(data.job.status === "failed" || data.job.status === "cancelled") &&
              <Button disabled={isMutating} onClick={() => void act("retry")}>重试</Button>}
          </div>
        </div>
        <div className="mt-4 h-2 overflow-hidden rounded-full bg-muted" aria-label={`进度 ${data.job.progress}%`}>
          <div className="h-full bg-primary transition-[width]" style={{ width: `${data.job.progress}%` }} />
        </div>
        {errorMessage ? <p className="mt-4 rounded-lg bg-destructive/10 p-3 text-sm text-destructive">{errorMessage}</p> : null}
        {data.job.error ? <pre className="mt-2 rounded-lg bg-muted p-3 text-xs text-destructive">{JSON.stringify(data.job.error, null, 2)}</pre> : null}
      </section>

      {data.job.status === "action_required" ? (
        <section className="rounded-lg border border-primary/40 p-5">
          <div className="flex items-start gap-3">
            {data.job.actionRequired === "quality_gate"
              ? <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600" aria-hidden />
              : <ShieldCheck className="mt-0.5 size-5 shrink-0 text-primary" aria-hidden />}
            <div className="min-w-0 flex-1">
              <h2 className="font-semibold">
                {data.job.actionRequired === "quality_gate" ? "计划未通过质量门禁" : "计划等待确认"}
              </h2>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                {data.job.actionRequired === "quality_gate"
                  ? "查看下方质量分项并调整标题、目标页面或导入要求；低质量计划不能直接进入治理。"
                  : "确认后将复用当前不可变计划执行实体匹配、Proposal 合成并提交审核。"}
              </p>
              {data.job.actionRequired === "confirm_plan" ? (
                <Button className="mt-4" disabled={isMutating} onClick={() => void act("confirm")}>
                  <CheckCircle2 aria-hidden />
                  确认并生成 Proposal
                </Button>
              ) : null}
            </div>
          </div>
        </section>
      ) : null}

      <ol className="grid gap-3 sm:grid-cols-2">
        {STAGES.map((name) => {
          const stage = latestByStage.get(name);
          const status = stage?.status ?? "pending";
          return <li key={name} className="rounded-lg border border-border p-4">
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm font-medium">{LABELS[name]}</span>
              <StageStatusIcon status={status} />
            </div>
            {stage?.finishedAt ? <p className="mt-2 text-xs text-muted-foreground">完成于 {stage.finishedAt.toLocaleString()}</p> : null}
          </li>;
        })}
      </ol>

      {data.runs.length > 1 ? (
        <details className="rounded-lg border border-border p-4">
          <summary className="cursor-pointer text-sm font-medium">
            运行记录（{data.runs.length} 次）
          </summary>
          <ol className="mt-3 divide-y divide-border">
            {data.runs.map((run) => (
              <li key={run.id} className="flex items-center justify-between gap-3 py-2 text-xs">
                <span>第 {run.attempt} 次 · {JOB_STATUS_LABEL[run.status]}</span>
                <time className="text-muted-foreground" dateTime={run.startedAt.toISOString()}>
                  {run.startedAt.toLocaleString()}
                </time>
              </li>
            ))}
          </ol>
        </details>
      ) : null}

      {data.plan ? (
        <section className="space-y-4 rounded-xl border border-border p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="font-semibold">页面导入计划</h2>
              <p className="mt-1 text-sm text-muted-foreground">{data.plan.profile.summary || data.plan.profile.title}</p>
            </div>
            <span className="rounded-full bg-muted px-2.5 py-1 text-xs text-muted-foreground">
              {currentPlanVersion ? `v${currentPlanVersion.revision} · ` : ""}
              质量 {Math.round(data.plan.qualityScore * 100)}% · {data.plan.routes.length} 条路由
            </span>
          </div>
          {data.plan.quality ? (
            <div className="space-y-3 border-y border-border py-4">
              <div className="flex items-center gap-2 text-sm font-medium">
                <Gauge className="size-4 text-muted-foreground" aria-hidden />
                五维质量
              </div>
              <dl className="grid gap-3 sm:grid-cols-2">
                {QUALITY_DIMENSIONS.map(([key, label]) => {
                  const score = data.plan!.quality![key];
                  return (
                    <div key={key} className="grid grid-cols-[5rem_1fr_3rem] items-center gap-2">
                      <dt className="text-xs text-muted-foreground">{label}</dt>
                      <dd className="h-1.5 overflow-hidden rounded-full bg-muted">
                        <span className="block h-full bg-primary" style={{ width: `${Math.round(score * 100)}%` }} />
                      </dd>
                      <dd className="text-right font-mono text-xs">{Math.round(score * 100)}%</dd>
                    </div>
                  );
                })}
              </dl>
              <p className="text-xs text-muted-foreground">
                综合 {Math.round(data.plan.quality.overall * 100)}%，门槛 {Math.round(data.plan.quality.threshold * 100)}%。
              </p>
            </div>
          ) : null}
          <ol className="grid gap-3 lg:grid-cols-2">
            {data.plan.routes.map((route, index) => {
              const meta = ROUTE_META[route.action];
              const RouteIcon = meta.icon;
              return (
                <li key={`${route.action}:${route.pageId ?? route.title}:${index}`} className="rounded-lg border border-border p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{route.title}</p>
                      <p className="mt-1 text-xs leading-5 text-muted-foreground">{route.reason}</p>
                    </div>
                    <span className={`inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-1 text-[11px] ${meta.className}`}>
                      <RouteIcon className="size-3" aria-hidden />{meta.label}
                    </span>
                  </div>
                  <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <span>置信度 {Math.round(route.confidence * 100)}%</span>
                    <span>{route.blocks.length} 个内容块</span>
                    {route.relatedTo.length > 0 ? <span>关联到 {route.relatedTo.join("、")}</span> : null}
                    {route.pageId ? <Link className="underline" href={`/pages/${route.pageId}`}>查看目标页</Link> : null}
                  </div>
                  {route.blocks.length > 0 ? (
                    <div className="mt-3 space-y-2 border-t border-border pt-3">
                      {route.blocks.slice(0, 3).map((block, blockIndex) => (
                        <div key={`${block.type}:${block.targetBlockId ?? blockIndex}`} className="text-xs leading-5">
                          <p className="line-clamp-3 text-foreground">
                            {block.text || block.items?.join("；")}
                          </p>
                          {block.evidence[0] ? (
                            <blockquote className="mt-1 border-l-2 border-border pl-2 text-muted-foreground">
                              {block.evidence[0].quotation}
                            </blockquote>
                          ) : null}
                        </div>
                      ))}
                    </div>
                  ) : null}
                </li>
              );
            })}
          </ol>
        </section>
      ) : null}

      {data.plans.length > 1 ? (
        <details className="rounded-lg border border-border p-4">
          <summary className="cursor-pointer text-sm font-medium">
            计划版本（{data.plans.length} 个）
          </summary>
          <ol className="mt-3 divide-y divide-border">
            {[...data.plans].reverse().map((version) => (
              <li key={version.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-xs">
                <span className="font-medium">v{version.revision}</span>
                <span>质量 {Math.round(version.qualityScore * 100)}%</span>
                {version.id === data.job.currentPlanId ? <span className="text-primary">当前</span> : null}
                {version.id === data.job.confirmedPlanId ? <span className="text-primary">已确认</span> : null}
                <time className="ml-auto text-muted-foreground" dateTime={version.createdAt.toISOString()}>
                  {version.createdAt.toLocaleString()}
                </time>
              </li>
            ))}
          </ol>
        </details>
      ) : null}

      {canReplan ? (
        <ReplanPanel
          id={id}
          planningInput={planningInput}
          onReplanned={async () => {
            await mutate();
          }}
        />
      ) : null}

      {data.job.proposalId ? <Button asChild><Link href={`/governance/proposals/${data.job.proposalId}`}>查看待审核 Proposal</Link></Button> : null}
    </div>
  );
}
