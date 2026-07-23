"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRMutation from "swr/mutation";
import type { ReviewTaskList } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { governanceApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH } from "@/lib/auth";

function ProposalMeta({ id }: { id: string }) {
  const { data } = useSWR(["governance:proposal-meta", id], () => governanceApi().getProposal({ id }));
  if (!data) return <p className="mt-2 text-xs text-muted-foreground">正在读取提案分组信息…</p>;
  return (
    <div className="mt-2 flex flex-wrap gap-2 text-xs">
      {data.importJobId ? <span className="rounded-full bg-blue-500/10 px-2 py-1 text-blue-700">来源 {data.importJobId.slice(0, 8)}</span> : null}
      <span className="rounded-full bg-muted px-2 py-1">{data.targetType} {data.targetId?.slice(0, 8) ?? "new"}</span>
      <span className="rounded-full bg-amber-500/10 px-2 py-1 text-amber-700">风险 {data.riskLevel}</span>
    </div>
  );
}

export function ReviewQueue({ initialData }: { initialData: ReviewTaskList }) {
  const router = useRouter();
  const { data, error, mutate, isLoading } = useSWR(
    "governance:pending-reviews",
    () => governanceApi().listReviewTasks({ pageSize: 100 }),
    { fallbackData: initialData },
  );
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const { trigger, isMutating } = useSWRMutation(
    "governance:review-decision",
    (
      _key,
      {
        arg,
      }: {
        arg: { id: string; approve: boolean; reason: string };
      },
    ) =>
      governanceApi().decideReviewTask({
        id: arg.id,
        reviewDecisionRequest: { approve: arg.approve, reason: arg.reason },
      }),
  );

  const decide = async (id: string, approve: boolean) => {
    const reason = reasons[id]?.trim() ?? "";
    if (!reason) {
      toast.error("请填写审核理由");
      return;
    }
    try {
      await trigger({ id, approve, reason });
      await mutate();
      toast.success(approve ? "提案已批准" : "提案已拒绝");
    } catch (decisionError) {
      if (isUnauthorized(decisionError)) {
        toast.error("请先登录后再提交审核");
        router.push(LOGIN_PATH);
      } else {
        toast.error("审核提交失败", { description: "请检查权限或稍后重试。" });
      }
    }
  };

  if (error) {
    return (
      <p className="rounded-lg border border-destructive/30 p-4 text-sm text-destructive">
        审核队列加载失败。
      </p>
    );
  }
  if (isLoading && !data) {
    return <p className="text-sm text-muted-foreground">正在加载审核队列…</p>;
  }
  if (!data?.items.length) {
    return (
      <p className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
        当前没有待审核提案。
      </p>
    );
  }

  return (
    <ol className="space-y-3">
      {data.items.map((task) => (
        <li key={task.id} className="rounded-lg border border-border p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-sm font-semibold">
                审核任务 {task.id.slice(0, 8)}
              </p>
              <p className="mt-1 font-mono text-xs text-muted-foreground">
                Proposal {task.proposalId}
              </p>
              <ProposalMeta id={task.proposalId} />
            </div>
            <Button variant="outline" size="sm" asChild>
              <Link href={`/governance/proposals/${task.proposalId}`}>
                查看三态预览
              </Link>
            </Button>
          </div>
          <label
            className="mt-4 block text-xs font-medium"
            htmlFor={`reason-${task.id}`}
          >
            审核理由
          </label>
          <textarea
            id={`reason-${task.id}`}
            value={reasons[task.id] ?? ""}
            onChange={(event) =>
              setReasons((current) => ({
                ...current,
                [task.id]: event.target.value,
              }))
            }
            placeholder="记录批准或拒绝的依据"
            className="mt-1 min-h-20 w-full resize-y rounded-lg border border-input bg-background px-3 py-2 text-sm"
          />
          <div className="mt-3 flex justify-end gap-2">
            <Button
              variant="destructive"
              disabled={isMutating}
              onClick={() => void decide(task.id, false)}
            >
              拒绝
            </Button>
            <Button
              disabled={isMutating}
              onClick={() => void decide(task.id, true)}
            >
              批准
            </Button>
          </div>
        </li>
      ))}
    </ol>
  );
}
