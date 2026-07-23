"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRMutation from "swr/mutation";

import { Button } from "@/components/ui/button";
import { importsApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";

const STAGES = ["fetch", "parse", "extract", "match", "compose", "review"] as const;
const LABELS: Record<string, string> = {
  fetch: "获取与安全扫描", parse: "解析与分块", extract: "结构化抽取",
  match: "实体与 Claim 匹配", compose: "Proposal 合成", review: "进入审核",
};

export function ImportJobProgress({ id }: { id: string }) {
  const router = useRouter();
  const { data, error, mutate } = useSWR(
    ["import-job", id],
    () => importsApi().getImportJob({ id }),
    { refreshInterval: (latest) => latest?.job.status === "queued" || latest?.job.status === "running" ? 1500 : 0 },
  );
  const { trigger, isMutating } = useSWRMutation(
    ["import-job-action", id],
    (_key, { arg }: { arg: "cancel" | "retry" }) =>
      arg === "cancel"
        ? importsApi().cancelImportJob({ id })
        : importsApi().retryImportJob({ id }),
  );
  const act = async (action: "cancel" | "retry") => {
    try {
      await trigger(action);
      await mutate();
      toast.success(action === "cancel" ? "任务已取消" : "任务已重新排队");
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
  if (error) return <p className="rounded-lg border border-destructive/30 p-5 text-sm text-destructive">导入任务加载失败，或当前 Actor 无权读取它。</p>;
  if (!data) return <p className="text-sm text-muted-foreground">正在加载导入进度…</p>;
  const latestByStage = new Map(data.stages.map((stage) => [stage.stage, stage]));

  return (
    <div className="space-y-6">
      <section className="rounded-xl border border-border p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="text-sm font-semibold">{data.job.status} · {data.job.progress}%</p>
            <p className="mt-1 font-mono text-xs text-muted-foreground">{data.job.id}</p>
          </div>
          <div className="flex gap-2">
            {(data.job.status === "queued" || data.job.status === "running") &&
              <Button variant="destructive" disabled={isMutating} onClick={() => void act("cancel")}>取消</Button>}
            {(data.job.status === "failed" || data.job.status === "cancelled") &&
              <Button disabled={isMutating} onClick={() => void act("retry")}>重试</Button>}
          </div>
        </div>
        <div className="mt-4 h-2 overflow-hidden rounded-full bg-muted" aria-label={`进度 ${data.job.progress}%`}>
          <div className="h-full bg-primary transition-[width]" style={{ width: `${data.job.progress}%` }} />
        </div>
        {data.job.error ? <pre className="mt-4 rounded-lg bg-muted p-3 text-xs text-destructive">{JSON.stringify(data.job.error, null, 2)}</pre> : null}
      </section>

      <ol className="grid gap-3 sm:grid-cols-2">
        {STAGES.map((name) => {
          const stage = latestByStage.get(name);
          return <li key={name} className="rounded-lg border border-border p-4">
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm font-medium">{LABELS[name]}</span>
              <span className="rounded-full bg-muted px-2 py-0.5 text-xs">{stage?.status ?? "pending"}</span>
            </div>
            {stage?.finishedAt ? <p className="mt-2 text-xs text-muted-foreground">完成于 {stage.finishedAt.toLocaleString()}</p> : null}
          </li>;
        })}
      </ol>

      {data.job.proposalId ? <Button asChild><Link href={`/governance/proposals/${data.job.proposalId}`}>查看待审核 Proposal</Link></Button> : null}
    </div>
  );
}
