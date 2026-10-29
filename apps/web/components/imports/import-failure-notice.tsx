import { ImportFailureToJSON, type ImportFailure } from "../../../../contracts/generated/typescript";

import { cn } from "@/lib/utils";

const STAGE_LABELS: Record<string, string> = {
  queued: "任务准备", fetch: "获取来源", parse: "解析内容", extract: "事实抽取",
  plan: "页面规划", match: "实体匹配", compose: "提案合成", review: "提交审核",
};

export function importFailureMessage(error: ImportFailure | null | undefined): string | null {
  if (!error) return null;
  if (error.message) return error.message;
  if (error.code === "quality_gate" && error.stage === "extract") {
    return "事实抽取结果未通过安全门禁；需要查看提取诊断确认拦截原因。";
  }
  if (error.code === "cancelled") return "导入任务已取消。";
  if (error.code === "worker_interrupted") return "Worker 中断，任务已自动重新排队。";
  return `${STAGE_LABELS[error.stage] ?? "导入阶段"}失败，请查看技术详情中的错误码。`;
}

export function ImportFailureNotice({
  error,
  compact = false,
  className,
}: {
  error: ImportFailure | null | undefined;
  compact?: boolean;
  className?: string;
}) {
  if (!error) return null;
  if (error.code === "entity_resolution_required") {
    return (
      <details className={cn("min-w-0 text-xs", className)}>
        <summary className="cursor-pointer text-muted-foreground">待确认的实体匹配详情</summary>
        <pre className="mt-2 max-h-80 overflow-y-auto whitespace-pre-wrap rounded-lg bg-muted p-3 [overflow-wrap:anywhere]">
          {JSON.stringify(error.matchReview, null, 2)}
        </pre>
      </details>
    );
  }
  const details = error.details;
  const diagnostics: Array<[string, string | number | undefined]> = [
    ["模型", details?.model],
    ["疑似提示注入", details?.promptInjectionDetected === undefined ? undefined : details.promptInjectionDetected ? "是（需核查原文）" : "否"],
    ["抽取质量", details?.qualityScore === undefined ? undefined : `${Math.round(details.qualityScore * 100)}%`],
    ["质量门槛", details?.qualityThreshold === undefined ? undefined : `${Math.round(details.qualityThreshold * 100)}%`],
    ["实体候选", details?.entityCount],
    ["事实候选", details?.claimCount],
    ["抽取结果", details?.extractionReused === undefined ? undefined : details.extractionReused ? "复用已保存的结果" : "本次生成"],
    ["模型错误码", details?.providerCode],
    ["HTTP 状态", details?.httpStatus],
    ["最大输入 Token", details?.maxInputTokens],
    ["来源大小上限", details?.maxSourceBytes === undefined ? undefined : `${Math.round(details.maxSourceBytes / 1024 / 1024)} MiB`],
    ["PDF 页数上限", details?.maxPages],
    ["失败页码", details?.page],
  ];

  return (
    <div className={cn("min-w-0 space-y-3 break-words text-sm [overflow-wrap:anywhere]", className)}>
      <div>
        {!compact ? <p className="mb-1 text-xs font-medium text-muted-foreground">失败阶段 · {STAGE_LABELS[error.stage] ?? error.stage}</p> : null}
        <p className="font-medium text-destructive">{importFailureMessage(error)}</p>
        {compact && details?.extractionReused ? <p className="mt-1 text-xs text-muted-foreground">本次复用已保存的抽取结果。</p> : null}
      </div>
      {!compact && error.suggestion ? (
        <div className="rounded-lg bg-muted/60 p-3">
          <p className="text-xs font-medium">建议处理</p>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">{error.suggestion}</p>
        </div>
      ) : null}
      {!compact && details ? (
        <dl className="grid gap-x-5 gap-y-2 text-xs sm:grid-cols-2">
          {diagnostics.filter(([, value]) => value !== undefined).map(([label, value]) => (
            <div key={label} className="min-w-0">
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="mt-0.5">{value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      <details className="text-xs">
        <summary className="cursor-pointer text-muted-foreground">技术详情 · {error.code}</summary>
        <pre className="mt-2 max-h-80 overflow-y-auto whitespace-pre-wrap rounded-lg bg-muted p-3 font-mono [overflow-wrap:anywhere]">
          {JSON.stringify(ImportFailureToJSON(error), null, 2)}
        </pre>
      </details>
    </div>
  );
}
