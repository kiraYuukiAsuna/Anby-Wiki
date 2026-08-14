"use client";

import { Command } from "cmdk";
import {
  ArrowRight,
  FileText,
  GitMerge,
  LoaderCircle,
  RotateCcw,
  Search,
  TriangleAlert,
  Waypoints,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type EntityCatalogItem,
  type EntityMergeResult,
  type RollbackEntityMergeResult,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { knowledgeApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { compactId } from "@/lib/display-id";

const mergeSchema = z.object({
  targetEntityId: z.string().uuid("请选择合法目标 Entity"),
  reason: z
    .string()
    .trim()
    .min(8, "请用至少 8 个字符说明为何两者是同一身份")
    .max(1000, "说明不能超过 1000 个字符"),
  confirmed: z.literal(true, "请确认你理解合并影响"),
});

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}

function MergedEntityPanel({
  sourceEntityId,
  sourceTitle,
  mergedIntoEntityId,
}: {
  sourceEntityId: string;
  sourceTitle: string;
  mergedIntoEntityId?: string;
}) {
  const router = useRouter();
  const session = useSession();
  const [rollbackOpen, setRollbackOpen] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [rollbackResult, setRollbackResult] =
    useState<RollbackEntityMergeResult>();
  const mergeState = useSWR(
    ["entity-merge-record", sourceEntityId],
    () => knowledgeApi().getEntityMerge({ id: sourceEntityId }),
    { shouldRetryOnError: false },
  );

  const startRollback = () => {
    if (!session.isAuthenticated) {
      toast.error("请先登录后回滚 Entity 合并");
      router.push(LOGIN_PATH);
      return;
    }
    setConfirmed(false);
    setRollbackOpen(true);
  };

  const rollback = async () => {
    const merge = mergeState.data;
    if (!merge || !confirmed) {
      toast.error("请确认回滚影响");
      return;
    }
    setSubmitting(true);
    try {
      const result = await knowledgeApi().rollbackEntityMerge({ id: merge.id });
      setRollbackResult(result);
      setRollbackOpen(false);
      toast.success(result.idempotent ? "该合并此前已回滚" : "Entity 身份已恢复", {
        description: `补偿 ${result.compensatedClaimIds.length} 条 Claim，移除 ${result.removedTargetLabels} 个目标标签映射。`,
      });
      await mergeState.mutate();
      router.refresh();
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
        router.push(LOGIN_PATH);
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("需要站点管理员权限才能回滚 Entity 合并");
      } else if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("无法安全回滚这次合并", {
          description:
            "合并后的标签、Claim 或身份状态已经发生变化。为避免覆盖后续编辑，服务端拒绝了补偿。",
        });
      } else {
        toast.error("Entity 合并回滚失败，请稍后重试");
      }
    } finally {
      setSubmitting(false);
    }
  };

  const merge = mergeState.data;

  return (
    <>
      <div className="rounded-xl border border-amber-200 bg-amber-50/75 p-4 text-sm">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            <GitMerge
              className="mt-0.5 size-4 shrink-0 text-amber-700"
              aria-hidden
            />
            <div className="min-w-0">
              <p className="font-semibold text-amber-950">
                该 Entity 已成为永久合并映射
              </p>
              {mergedIntoEntityId ? (
                <Link
                  href={`/entities/${mergedIntoEntityId}`}
                  className="mt-2 inline-flex items-center gap-1.5 font-medium text-amber-800 hover:underline"
                >
                  打开当前有效身份
                  <ArrowRight className="size-3.5" aria-hidden />
                </Link>
              ) : null}
            </div>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={
              session.isLoading ||
              mergeState.isLoading ||
              !merge ||
              merge.status !== "applied"
            }
            onClick={startRollback}
          >
            <RotateCcw aria-hidden />
            补偿回滚
          </Button>
        </div>

        {mergeState.isLoading ? (
          <p className="mt-4 flex items-center gap-2 text-xs text-amber-800">
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
            正在读取合并账本…
          </p>
        ) : mergeState.error ? (
          <p className="mt-4 text-xs text-destructive">
            合并映射仍然有效，但暂时无法读取补偿账本。
          </p>
        ) : merge ? (
          <dl className="mt-4 grid gap-2 border-t border-amber-200 pt-4 text-xs sm:grid-cols-4">
            <div>
              <dt className="text-amber-800/75">合并批次</dt>
              <dd className="mt-1 font-mono font-semibold text-amber-950">
                {compactId(merge.id)}
              </dd>
            </div>
            <div>
              <dt className="text-amber-800/75">标签映射</dt>
              <dd className="mt-1 font-semibold text-amber-950">
                {merge.labelMappings.length}
              </dd>
            </div>
            <div>
              <dt className="text-amber-800/75">Claim 映射</dt>
              <dd className="mt-1 font-semibold text-amber-950">
                {merge.claimMappings.length}
              </dd>
            </div>
            <div>
              <dt className="text-amber-800/75">发生时间</dt>
              <dd className="mt-1 font-semibold text-amber-950">
                {new Intl.DateTimeFormat("zh-CN", {
                  dateStyle: "medium",
                  timeStyle: "short",
                }).format(merge.createdAt)}
              </dd>
            </div>
            <div className="sm:col-span-4">
              <dt className="text-amber-800/75">合并依据</dt>
              <dd className="mt-1 leading-5 text-amber-950">{merge.reason}</dd>
            </div>
          </dl>
        ) : null}
      </div>

      {rollbackResult ? (
        <div className="mt-3 rounded-xl border border-emerald-200 bg-emerald-50/70 p-4 text-sm text-emerald-900">
          「{sourceTitle}」已恢复为可写身份；页面刷新后可继续治理。
        </div>
      ) : null}

      <Dialog open={rollbackOpen} onOpenChange={setRollbackOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>补偿回滚 Entity 合并</DialogTitle>
            <DialogDescription>
              恢复「{sourceTitle}」并按不可变合并账本迁回标签与 Claim。服务端会逐项核对，
              任何后续变更都会令回滚安全失败。
            </DialogDescription>
          </DialogHeader>
          {merge ? (
            <div className="rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-xs leading-5">
              <p className="font-semibold text-destructive">
                将补偿批次 {compactId(merge.id)}
              </p>
              <p className="mt-1 text-muted-foreground">
                最多恢复 {merge.claimMappings.length} 条 Claim，并撤销{" "}
                {merge.labelMappings.length} 个标签映射。该动作本身也会写入审计日志。
              </p>
            </div>
          ) : null}
          <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-border p-3 text-xs leading-5">
            <Checkbox
              checked={confirmed}
              onCheckedChange={(checked) => setConfirmed(checked === true)}
              className="mt-0.5"
            />
            <span>
              我确认需要恢复源 Entity，并理解若合并后已有新编辑，系统会拒绝回滚而不覆盖数据。
            </span>
          </label>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={submitting}
              onClick={() => setRollbackOpen(false)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={!confirmed || submitting || !merge}
              onClick={() => void rollback()}
            >
              {submitting ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <RotateCcw aria-hidden />
              )}
              {submitting ? "核对并回滚中…" : "确认补偿回滚"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export function EntityMergePanel({
  sourceEntityId,
  sourceTitle,
  sourceCanonicalKey,
  entityTypeKey,
  entityTypeName,
  status,
  mergedIntoEntityId,
  sourceLabelCount,
  sourceAliasCount,
  sourcePageCount,
}: {
  sourceEntityId: string;
  sourceTitle: string;
  sourceCanonicalKey: string;
  entityTypeKey: string;
  entityTypeName: string;
  status: "active" | "merged" | "deleted";
  mergedIntoEntityId?: string;
  sourceLabelCount: number;
  sourceAliasCount: number;
  sourcePageCount: number;
}) {
  const router = useRouter();
  const session = useSession();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [target, setTarget] = useState<EntityCatalogItem>();
  const [reason, setReason] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<EntityMergeResult>();
  const debouncedQuery = useDebouncedValue(query.trim(), 220);

  const targetState = useSWR(
    open
      ? ["entity-merge-targets", sourceEntityId, entityTypeKey, debouncedQuery]
      : null,
    () =>
      knowledgeApi().listEntities({
        q: debouncedQuery || undefined,
        typeKey: entityTypeKey,
        status: "active",
        pageSize: 20,
      }),
    { keepPreviousData: true },
  );
  const candidates = (targetState.data?.items ?? []).filter(
    (candidate) => candidate.id !== sourceEntityId,
  );

  if (status === "merged") {
    return (
      <MergedEntityPanel
        sourceEntityId={sourceEntityId}
        sourceTitle={sourceTitle}
        mergedIntoEntityId={mergedIntoEntityId}
      />
    );
  }

  if (status !== "active") {
    return (
      <div className="rounded-xl border border-amber-200 bg-amber-50/75 p-4 text-sm">
        <div className="flex items-start gap-3">
          <GitMerge className="mt-0.5 size-4 shrink-0 text-amber-700" aria-hidden />
          <div>
            <p className="font-semibold text-amber-950">
              该 Entity 不可再治理
            </p>
            {mergedIntoEntityId ? (
              <Link
                href={`/entities/${mergedIntoEntityId}`}
                className="mt-2 inline-flex items-center gap-1.5 font-medium text-amber-800 hover:underline"
              >
                打开当前有效身份
                <ArrowRight className="size-3.5" aria-hidden />
              </Link>
            ) : (
              <p className="mt-1 text-muted-foreground">当前状态：{status}</p>
            )}
          </div>
        </div>
      </div>
    );
  }

  const start = () => {
    if (!session.isAuthenticated) {
      toast.error("请先登录后发起 Entity 合并");
      router.push(LOGIN_PATH);
      return;
    }
    setQuery("");
    setTarget(undefined);
    setReason("");
    setConfirmed(false);
    setOpen(true);
  };

  const submit = async () => {
    const parsed = mergeSchema.safeParse({
      targetEntityId: target?.id,
      reason,
      confirmed,
    });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查合并信息");
      return;
    }
    setSubmitting(true);
    try {
      const merge = await knowledgeApi().mergeEntity({
        id: sourceEntityId,
        mergeEntityRequest: {
          targetEntityId: parsed.data.targetEntityId,
          reason: parsed.data.reason,
        },
      });
      setResult(merge);
      setOpen(false);
      toast.success(merge.idempotent ? "该合并此前已经完成" : "Entity 合并已完成", {
        description: `${merge.labelMappings.length} 个标签映射，${merge.claimMappings.length} 条 Claim 映射。`,
      });
      router.refresh();
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
        router.push(LOGIN_PATH);
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("需要站点管理员权限才能合并 Entity");
      } else if (error instanceof ResponseError && error.response.status === 409) {
        toast.error("合并条件已变化", {
          description: "源或目标可能已被合并，或两者并非同站点、同类型实体。",
        });
      } else {
        toast.error("Entity 合并失败，请稍后重试");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <div className="rounded-xl border border-border bg-muted/25 p-4">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <p className="flex items-center gap-2 text-sm font-semibold">
              <GitMerge className="size-4 text-indigo-700" aria-hidden />
              重复身份治理
            </p>
            <p className="mt-1 max-w-xl text-xs leading-5 text-muted-foreground">
              仅当两个 {entityTypeName} Entity
              明确代表同一现实身份时使用。操作会保留审计与旧 ID 映射。
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            disabled={session.isLoading}
            onClick={start}
          >
            <GitMerge aria-hidden />
            合并到其他 Entity
          </Button>
        </div>
      </div>

      {result ? (
        <div className="mt-3 rounded-xl border border-emerald-200 bg-emerald-50/70 p-4 text-sm">
          <p className="font-semibold text-emerald-900">
            合并批次 {compactId(result.id)} 已记录
          </p>
          <p className="mt-1 text-xs text-emerald-800">
            映射了 {result.labelMappings.length} 个标签与{" "}
            {result.claimMappings.length} 条 Claim。
          </p>
          <Link
            href={`/entities/${result.targetEntityId}`}
            className="mt-3 inline-flex items-center gap-1.5 text-xs font-semibold text-emerald-800 hover:underline"
          >
            查看有效目标
            <ArrowRight className="size-3.5" aria-hidden />
          </Link>
        </div>
      ) : null}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>合并重复 Entity</DialogTitle>
            <DialogDescription>
              将「{sourceTitle}」并入同类型的有效 Entity。源 ID
              会保留为永久映射，标签与 Claim 由服务端在一个事务中迁移。
            </DialogDescription>
          </DialogHeader>

          <div className="rounded-xl border border-destructive/20 bg-destructive/5 p-4">
            <div className="flex items-start gap-3">
              <TriangleAlert
                className="mt-0.5 size-4 shrink-0 text-destructive"
                aria-hidden
              />
              <div className="text-xs leading-5">
                <p className="font-semibold text-destructive">高风险身份变更</p>
                <p className="mt-1 text-muted-foreground">
                  源 Entity 当前有 {sourceLabelCount} 个标签、{sourceAliasCount}{" "}
                  个别名与 {sourcePageCount} 个页面提及。合并后不再接受新写入。
                </p>
                <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                  {sourceCanonicalKey} · {sourceEntityId}
                </p>
              </div>
            </div>
          </div>

          {target ? (
            <div className="rounded-xl border border-indigo-200 bg-indigo-50/65 p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-[10px] font-semibold tracking-wide text-indigo-700 uppercase">
                    合并目标
                  </p>
                  <p className="mt-1 truncate font-semibold">{target.displayLabel}</p>
                  <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                    {target.canonicalKey} · {target.id}
                  </p>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setTarget(undefined)}
                >
                  重新选择
                </Button>
              </div>
              <dl className="mt-4 grid grid-cols-3 gap-2 text-center">
                <div className="rounded-lg bg-background/80 px-2 py-2">
                  <dd className="font-semibold">{target.claimCount}</dd>
                  <dt className="text-[10px] text-muted-foreground">已有事实</dt>
                </div>
                <div className="rounded-lg bg-background/80 px-2 py-2">
                  <dd className="font-semibold">{target.pageCount}</dd>
                  <dt className="text-[10px] text-muted-foreground">绑定页面</dt>
                </div>
                <div className="rounded-lg bg-background/80 px-2 py-2">
                  <dd className="font-semibold">
                    {target.labelCount + target.aliasCount}
                  </dd>
                  <dt className="text-[10px] text-muted-foreground">已有名称</dt>
                </div>
              </dl>
            </div>
          ) : (
            <Command shouldFilter={false} label="选择 Entity 合并目标">
              <div className="relative">
                <Search
                  className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
                  aria-hidden
                />
                <Command.Input
                  value={query}
                  onValueChange={setQuery}
                  placeholder={`搜索其他${entityTypeName} Entity…`}
                  autoFocus
                  className="flex h-9 w-full rounded-lg border border-input bg-transparent pr-3 pl-9 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                />
              </div>
              <Command.List className="mt-2 max-h-56 overflow-y-auto">
                {targetState.isLoading && !candidates.length ? (
                  <Command.Loading className="flex items-center justify-center gap-2 px-3 py-6 text-sm text-muted-foreground">
                    <LoaderCircle className="size-4 animate-spin" aria-hidden />
                    搜索中…
                  </Command.Loading>
                ) : null}
                {!targetState.isLoading && !candidates.length ? (
                  <Command.Empty className="px-3 py-6 text-center text-sm text-muted-foreground">
                    没有可用的同类型目标
                  </Command.Empty>
                ) : null}
                {candidates.map((candidate) => (
                  <Command.Item
                    key={candidate.id}
                    value={candidate.id}
                    onSelect={() => setTarget(candidate)}
                    className="flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-sm data-[selected=true]:bg-accent"
                  >
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-indigo-100 text-indigo-700">
                      <Waypoints className="size-4" aria-hidden />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">
                        {candidate.displayLabel}
                      </span>
                      <span className="block truncate font-mono text-[10px] text-muted-foreground">
                        {candidate.canonicalKey}
                      </span>
                    </span>
                    <span className="flex shrink-0 items-center gap-1 text-[10px] text-muted-foreground">
                      <FileText className="size-3" aria-hidden />
                      {candidate.claimCount}
                    </span>
                  </Command.Item>
                ))}
              </Command.List>
            </Command>
          )}

          <div className="space-y-2">
            <Label htmlFor="entity-merge-reason">合并依据</Label>
            <Textarea
              id="entity-merge-reason"
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              placeholder="说明二者为何是同一身份，以及你核对了哪些标签、事实或来源…"
              maxLength={1000}
            />
            <p className="text-right text-[10px] text-muted-foreground">
              {reason.length}/1000
            </p>
          </div>

          <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-border p-3 text-xs leading-5">
            <Checkbox
              checked={confirmed}
              onCheckedChange={(checked) => setConfirmed(checked === true)}
              className="mt-0.5"
            />
            <span>
              我确认两者代表同一身份，并理解源 Entity
              将变为只读映射；该操作会进入不可变审计日志。
            </span>
          </label>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={submitting}
              onClick={() => setOpen(false)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={!target || !confirmed || submitting}
              onClick={() => void submit()}
            >
              {submitting ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <GitMerge aria-hidden />
              )}
              {submitting ? "合并中…" : "确认合并身份"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
