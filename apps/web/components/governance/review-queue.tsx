"use client";

import { useState } from "react";
import Link from "next/link";
import { WorkspaceLink } from "@/components/workspace-link";
import { useRouter } from "next/navigation";
import {
  ArrowRight,
  BadgeCheck,
  Layers3,
  LoaderCircle,
  LockKeyhole,
  ShieldAlert,
  ShieldCheck,
} from "lucide-react";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRMutation from "swr/mutation";
import { z } from "zod";
import { ResponseError } from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { governanceApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { compactId } from "@/lib/display-id";

const createBatchSchema = z.object({
  proposalIds: z.array(z.string().uuid()).min(1, "至少选择一个待审提案"),
  samplePercent: z.coerce.number().int().min(1).max(100),
  waveSize: z.coerce.number().int().min(1).max(1000),
  forceFull: z.boolean(),
});

function ProposalMeta({ id }: { id: string }) {
  const { data } = useSWR(["governance:proposal-meta", id], () => governanceApi().getProposal({ id }));
  if (!data) return <p className="mt-2 text-xs text-muted-foreground">正在读取提案分组信息…</p>;
  return (
    <div className="mt-2 flex flex-wrap gap-2 text-xs">
      {data.importJobId ? <span className="rounded-full bg-blue-500/10 px-2 py-1 text-blue-700">来源 {compactId(data.importJobId)}</span> : null}
      <span className="rounded-full bg-muted px-2 py-1">{data.targetType} {data.targetId ? compactId(data.targetId) : "new"}</span>
      <span className="rounded-full bg-amber-500/10 px-2 py-1 text-amber-700">风险 {data.riskLevel}</span>
    </div>
  );
}

export function ReviewQueue() {
  const router = useRouter();
  const session = useSession();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [samplePercent, setSamplePercent] = useState("20");
  const [waveSize, setWaveSize] = useState("25");
  const [forceFull, setForceFull] = useState(false);
  const [creating, setCreating] = useState(false);
  const { data, error, mutate, isLoading } = useSWR(
    session.isAuthenticated ? ["governance:pending-reviews", session.session?.actorId, session.session?.wikiId] : null,
    () => governanceApi().listReviewTasks({ pageSize: 100 }),
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
      toast.success(
        approve ? "提案已批准，可前往待应用队列" : "提案已拒绝",
        approve
          ? {
              action: {
                label: "去应用",
                onClick: () => router.push("/governance/apply"),
              },
            }
          : undefined,
      );
    } catch (decisionError) {
      if (isUnauthorized(decisionError)) {
        toast.error("请先登录后再提交审核");
        router.push(LOGIN_PATH);
      } else {
        toast.error("审核提交失败", { description: "请检查权限或稍后重试。" });
      }
    }
  };
  const tasks = data?.items ?? [];
  const allSelected = Boolean(tasks.length) && tasks.every((task) => selected.has(task.proposalId));
  const selectedProposalIds = tasks
    .filter((task) => selected.has(task.proposalId))
    .map((task) => task.proposalId);

  const toggleAll = (checked: boolean) => {
    setSelected(checked ? new Set(tasks.map((task) => task.proposalId)) : new Set());
  };

  const toggleProposal = (proposalId: string, checked: boolean) => {
    setSelected((current) => {
      const next = new Set(current);
      if (checked) next.add(proposalId);
      else next.delete(proposalId);
      return next;
    });
  };

  const createBatch = async () => {
    const parsed = createBatchSchema.safeParse({
      proposalIds: selectedProposalIds,
      samplePercent,
      waveSize,
      forceFull,
    });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查批次参数");
      return;
    }
    setCreating(true);
    try {
      const created = await governanceApi().createBulkReviewBatch({
        createBulkReviewBatchRequest: {
          proposalIds: new Set(parsed.data.proposalIds),
          samplePercent: parsed.data.samplePercent,
          waveSize: parsed.data.waveSize,
          forceFull: parsed.data.forceFull,
        },
      });
      await mutate();
      toast.success("批量审核批次已创建");
      router.push(`/governance/bulk/${created.id}`);
    } catch (createError) {
      if (isUnauthorized(createError)) {
        toast.error("请先登录后创建批次");
        router.push(LOGIN_PATH);
      } else {
        toast.error("批次创建失败", {
          description: "请确认所选提案仍在待审状态且拥有审核权限。",
        });
      }
    } finally {
      setCreating(false);
    }
  };

  if (session.isLoading) {
    return (
      <div className="flex min-h-56 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在确认审核权限…
      </div>
    );
  }
  if (!session.isAuthenticated) {
    return (
      <div className="rounded-2xl border border-dashed bg-muted/25 px-6 py-12 text-center">
        <LockKeyhole className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后查看审核队列</h2>
        <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">
          审核任务只向具备治理权限的共建者开放。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }
  if (error instanceof ResponseError && error.response.status === 403) {
    return (
      <div className="rounded-2xl border border-amber-200 bg-amber-50/65 p-8">
        <ShieldAlert className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">
          需要审核权限
        </h2>
        <p className="mt-2 text-sm leading-6 text-amber-900/70">
          当前账号可以继续使用站点，但不能查看或处理人工审核任务。
        </p>
      </div>
    );
  }
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
      <div className="rounded-2xl border border-dashed border-border p-8 text-center">
        <BadgeCheck
          className="mx-auto size-7 text-muted-foreground"
          aria-hidden
        />
        <p className="mt-3 text-sm text-muted-foreground">当前没有待审核提案。</p>
        <Button asChild variant="outline" size="sm" className="mt-4">
          <WorkspaceLink href="/governance/apply">
            查看已批准待应用
            <ArrowRight aria-hidden />
          </WorkspaceLink>
        </Button>
      </div>
    );
  }

  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_22rem]">
      <section className="min-w-0">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border/75 bg-muted/20 px-4 py-3">
          <label className="flex items-center gap-2 text-sm font-medium">
            <Checkbox
              checked={allSelected}
              onCheckedChange={(value) => toggleAll(value === true)}
              aria-label="选择全部待审提案"
            />
            待审提案
          </label>
          <span className="text-xs text-muted-foreground">
            已选 {selectedProposalIds.length}/{tasks.length}
          </span>
        </div>

        <ol className="space-y-3">
          {tasks.map((task) => (
            <li key={task.id} className="rounded-2xl border border-border p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex min-w-0 gap-3">
                  <Checkbox
                    checked={selected.has(task.proposalId)}
                    onCheckedChange={(value) => toggleProposal(task.proposalId, value === true)}
                    aria-label={`选择提案 ${task.proposalId}`}
                    className="mt-1"
                  />
                  <div className="min-w-0">
                    <p className="text-sm font-semibold">
                      审核任务 {compactId(task.id)}
                    </p>
                    <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                      Proposal {task.proposalId}
                    </p>
                    <ProposalMeta id={task.proposalId} />
                  </div>
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
      </section>

      <aside className="xl:sticky xl:top-24 xl:self-start">
        <div className="rounded-2xl border border-primary/15 bg-card shadow-[0_20px_55px_-38px_color-mix(in_oklch,var(--primary),transparent_45%)]">
          <div className="border-b border-border/75 p-5">
            <span className="flex size-9 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <Layers3 className="size-4" aria-hidden />
            </span>
            <h2 className="mt-4 font-semibold">批量审核</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              已选 {selectedProposalIds.length} 个待审提案
            </p>
          </div>

          <div className="p-5">
            <div className="grid grid-cols-2 gap-3">
              <label className="text-xs font-medium">
                抽样比例
                <span className="mt-1 flex items-center">
                  <Input
                    type="number"
                    min={1}
                    max={100}
                    value={samplePercent}
                    onChange={(event) => setSamplePercent(event.target.value)}
                    className="rounded-r-none"
                  />
                  <span className="flex h-8 items-center rounded-r-lg border border-l-0 px-2 text-muted-foreground">
                    %
                  </span>
                </span>
              </label>
              <label className="text-xs font-medium">
                每个 wave
                <Input
                  type="number"
                  min={1}
                  max={1000}
                  value={waveSize}
                  onChange={(event) => setWaveSize(event.target.value)}
                  className="mt-1"
                />
              </label>
            </div>
            <label className="mt-4 flex cursor-pointer gap-3 rounded-xl border border-border/75 p-3">
              <Checkbox
                checked={forceFull}
                onCheckedChange={(value) => setForceFull(value === true)}
                aria-label="强制全量审核"
              />
              <span>
                <span className="block text-xs font-medium">强制全量审核</span>
                <span className="mt-1 block text-[11px] leading-4 text-muted-foreground">
                  高危提案会自动切换为全量；这里可主动提高审查强度。
                </span>
              </span>
            </label>
            <Button
              className="mt-5 w-full"
              disabled={creating || !selectedProposalIds.length}
              onClick={() => void createBatch()}
            >
              {creating ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <ShieldCheck aria-hidden />
              )}
              冻结并开始批量审核
            </Button>
          </div>
        </div>
      </aside>
    </div>
  );
}
