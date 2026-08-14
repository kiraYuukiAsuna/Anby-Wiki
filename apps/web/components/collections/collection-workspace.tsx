"use client";

import Link from "next/link";
import {
  ArrowLeft,
  ArrowRight,
  CircleGauge,
  DatabaseZap,
  Layers3,
  ListChecks,
  LoaderCircle,
  Plus,
  RefreshCw,
  Sparkles,
  Trash2,
  Workflow,
} from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";
import { z } from "zod";

import type {
  Collection,
  CollectionMembership,
  CollectionMembershipListPage,
  WriteCollectionMember,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { collectionsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { compactId } from "@/lib/display-id";
import {
  collectionSummary,
  isDynamicCollectionQuery,
  isRuleCollectionQuery,
} from "@/lib/collections";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 50;
const uuidSchema = z.string().uuid();

interface MemberDraft {
  key: number;
  memberType: "page" | "entity";
  targetID: string;
  sortKey: string;
  sourceRevisionID: string;
}

const TYPE_META = {
  manual: {
    label: "Manual Collection",
    icon: ListChecks,
    color: "bg-sky-100 text-sky-700",
  },
  rule: {
    label: "Rule Collection",
    icon: Workflow,
    color: "bg-amber-100 text-amber-700",
  },
  dynamic: {
    label: "Dynamic Collection",
    icon: Sparkles,
    color: "bg-violet-100 text-violet-700",
  },
} as const;

function memberHref(member: CollectionMembership): string {
  return member.memberType === "page"
    ? `/pages/${member.pageId}`
    : `/entities/${member.entityId}`;
}

function MemberList({
  items,
  loading,
}: {
  items: CollectionMembership[];
  loading: boolean;
}) {
  if (loading) {
    return <div className="h-52 animate-pulse rounded-2xl border bg-muted/35" />;
  }
  if (items.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed px-6 py-14 text-center">
        <Layers3 className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h3 className="mt-4 font-semibold">当前没有成员</h3>
        <p className="mt-2 text-sm text-muted-foreground">
          人工添加成员、重建规则，或调整 Dynamic 查询条件。
        </p>
      </div>
    );
  }
  return (
    <ol className="overflow-hidden rounded-2xl border bg-card">
      {items.map((member, index) => {
        const targetID =
          member.memberType === "page" ? member.pageId : member.entityId;
        return (
          <li
            key={`${member.memberType}:${targetID}:${index}`}
            className="flex flex-wrap items-center gap-4 border-b border-border/65 p-4 last:border-b-0 hover:bg-muted/20"
          >
            <span
              className={cn(
                "flex size-9 shrink-0 items-center justify-center rounded-xl",
                member.memberType === "page"
                  ? "bg-sky-100 text-sky-700"
                  : "bg-violet-100 text-violet-700",
              )}
            >
              {member.memberType === "page" ? (
                <Layers3 className="size-4" aria-hidden />
              ) : (
                <DatabaseZap className="size-4" aria-hidden />
              )}
            </span>
            <div className="min-w-0 flex-1">
              <Link
                href={memberHref(member)}
                className="group inline-flex max-w-full items-center gap-1.5 font-medium hover:text-primary"
              >
                <span className="truncate">{member.displayTitle}</span>
                <ArrowRight
                  className="size-3.5 shrink-0 transition-transform group-hover:translate-x-0.5"
                  aria-hidden
                />
              </Link>
              <p className="mt-1 text-[11px] text-muted-foreground">
                {member.memberType === "page" ? "Page" : "Entity"} · 排序键{" "}
                <span className="font-mono">{member.sortKey}</span>
              </p>
            </div>
            <div className="text-right text-[11px] text-muted-foreground">
              <p>
                {member.sourceType === "manual"
                  ? "人工策展"
                  : member.sourceType === "rule"
                    ? "规则物化"
                    : "实时查询"}
              </p>
              {member.sourceRevisionId ? (
                <p className="mt-1 font-mono">
                  rev {compactId(member.sourceRevisionId)}
                </p>
              ) : null}
            </div>
          </li>
        );
      })}
    </ol>
  );
}

function ManualMemberEditor({
  collection,
  onClose,
  onSaved,
}: {
  collection: Collection;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [rows, setRows] = useState<MemberDraft[]>([]);
  const [nextKey, setNextKey] = useState(1);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const found: CollectionMembership[] = [];
        let cursor: string | undefined;
        do {
          const page = await collectionsApi().listCollectionMembers({
            id: collection.id,
            cursor,
            pageSize: 100,
          });
          found.push(...page.items);
          cursor = page.nextCursor || undefined;
        } while (cursor);
        if (cancelled) return;
        setRows(
          found.map((member, index) => ({
            key: index + 1,
            memberType: member.memberType,
            targetID:
              member.memberType === "page"
                ? (member.pageId ?? "")
                : (member.entityId ?? ""),
            sortKey: member.sortKey,
            sourceRevisionID: member.sourceRevisionId ?? "",
          })),
        );
        setNextKey(found.length + 1);
      } catch {
        if (cancelled) return;
        toast.error("无法载入完整成员清单");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [collection.id]);

  const update = (key: number, patch: Partial<MemberDraft>) => {
    setRows((current) =>
      current.map((row) => (row.key === key ? { ...row, ...patch } : row)),
    );
  };

  const add = () => {
    setRows((current) => [
      ...current,
      {
        key: nextKey,
        memberType: "page",
        targetID: "",
        sortKey: String(current.length + 1).padStart(6, "0"),
        sourceRevisionID: "",
      },
    ]);
    setNextKey((value) => value + 1);
  };

  const save = async () => {
    const invalid = rows.some(
      (row) =>
        !uuidSchema.safeParse(row.targetID.trim()).success ||
        !uuidSchema.safeParse(row.sourceRevisionID.trim()).success ||
        !row.sortKey.trim(),
    );
    if (invalid) {
      toast.error("请检查每个目标 ID、来源 Revision ID 与排序键");
      return;
    }
    const seen = new Set<string>();
    const items: WriteCollectionMember[] = [];
    for (const row of rows) {
      const targetID = row.targetID.trim();
      const identity = `${row.memberType}:${targetID}`;
      if (seen.has(identity)) {
        toast.error("同一目标不能重复添加");
        return;
      }
      seen.add(identity);
      items.push({
        memberType: row.memberType,
        pageId: row.memberType === "page" ? targetID : undefined,
        entityId: row.memberType === "entity" ? targetID : undefined,
        sortKey: row.sortKey.trim(),
        sourceRevisionId: row.sourceRevisionID.trim(),
      });
    }
    setSaving(true);
    try {
      await collectionsApi().replaceManualCollectionMembers({
        id: collection.id,
        replaceCollectionMembersRequest: { items },
      });
      await onSaved();
      toast.success("人工成员已原子替换", {
        description: `${items.length} 个成员`,
      });
      onClose();
    } catch {
      toast.error("保存成员失败", {
        description: "目标或来源 Revision 可能不属于当前 Wiki。",
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>维护人工成员</DialogTitle>
        <DialogDescription>
          保存会在一个事务中替换完整成员集；每个成员需记录来源 Revision。
        </DialogDescription>
      </DialogHeader>
      {loading ? (
        <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
          <LoaderCircle className="mr-2 size-4 animate-spin" aria-hidden />
          载入全部成员…
        </div>
      ) : (
        <div className="max-h-[58vh] space-y-3 overflow-y-auto pr-1">
          {rows.map((row) => (
            <div key={row.key} className="rounded-xl border bg-muted/20 p-3">
              <div className="grid grid-cols-[7rem_1fr_auto] gap-2">
                <select
                  value={row.memberType}
                  onChange={(event) =>
                    update(row.key, {
                      memberType: event.target.value as "page" | "entity",
                      targetID: "",
                    })
                  }
                  className="h-8 rounded-lg border border-input bg-background px-2 text-xs"
                  aria-label="成员类型"
                >
                  <option value="page">Page</option>
                  <option value="entity">Entity</option>
                </select>
                <Input
                  value={row.targetID}
                  onChange={(event) =>
                    update(row.key, { targetID: event.target.value })
                  }
                  placeholder={`${row.memberType} UUID`}
                  className="font-mono text-xs"
                  aria-label="目标 ID"
                />
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  onClick={() =>
                    setRows((current) =>
                      current.filter((item) => item.key !== row.key),
                    )
                  }
                  aria-label="移除成员"
                >
                  <Trash2 aria-hidden />
                </Button>
              </div>
              <div className="mt-2 grid gap-2 sm:grid-cols-[9rem_1fr]">
                <Input
                  value={row.sortKey}
                  onChange={(event) =>
                    update(row.key, { sortKey: event.target.value })
                  }
                  placeholder="排序键"
                  aria-label="排序键"
                  className="font-mono text-xs"
                />
                <Input
                  value={row.sourceRevisionID}
                  onChange={(event) =>
                    update(row.key, { sourceRevisionID: event.target.value })
                  }
                  placeholder="来源 Revision UUID"
                  aria-label="来源 Revision ID"
                  className="font-mono text-xs"
                />
              </div>
            </div>
          ))}
          <Button type="button" variant="outline" className="w-full" onClick={add}>
            <Plus aria-hidden />
            添加成员
          </Button>
        </div>
      )}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button
          type="button"
          disabled={loading || saving}
          onClick={() => void save()}
        >
          {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          保存完整成员集
        </Button>
      </DialogFooter>
    </>
  );
}

function RuleRebuild({
  collection,
  onClose,
  onSaved,
}: {
  collection: Collection;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [revisionID, setRevisionID] = useState("");
  const [saving, setSaving] = useState(false);

  const rebuild = async () => {
    if (!uuidSchema.safeParse(revisionID.trim()).success) {
      toast.error("请输入合法来源 Revision ID");
      return;
    }
    setSaving(true);
    try {
      const result = await collectionsApi().rebuildRuleCollection({
        id: collection.id,
        rebuildCollectionRequest: { sourceRevisionId: revisionID.trim() },
      });
      await onSaved();
      toast.success("规则成员已重建", {
        description: `${result.memberCount} 个成员`,
      });
      onClose();
    } catch {
      toast.error("重建失败", {
        description: "来源 Revision 或规则引用可能无效。",
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>重建规则集合</DialogTitle>
        <DialogDescription>
          从当前 Entity / Claim 权威数据重新物化成员，并记录本次规则定义的来源 Revision。
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-2">
        <Label htmlFor="collection-source-revision">来源 Revision ID</Label>
        <Input
          id="collection-source-revision"
          value={revisionID}
          onChange={(event) => setRevisionID(event.target.value)}
          placeholder="UUID"
          className="font-mono text-xs"
        />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button type="button" disabled={saving} onClick={() => void rebuild()}>
          {saving ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <RefreshCw aria-hidden />
          )}
          执行重建
        </Button>
      </DialogFooter>
    </>
  );
}

export function CollectionWorkspace({ id }: { id: string }) {
  const [manageOpen, setManageOpen] = useState(false);
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const collectionState = useSWR<Collection>(["collection", id], () =>
    collectionsApi().getCollection({ id }),
  );
  const memberState = useSWRInfinite<CollectionMembershipListPage>(
    (pageIndex, previousPage) => {
      if (!collectionState.data) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "collection-members",
        id,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, collectionID, cursor] = cacheKey as readonly [
        string,
        string,
        string,
      ];
      return collectionsApi().listCollectionMembers({
        id: collectionID,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );

  if (collectionState.isLoading) {
    return <div className="h-72 animate-pulse rounded-3xl border bg-muted/35" />;
  }
  if (collectionState.error || !collectionState.data) {
    return (
      <div className="rounded-3xl border border-destructive/20 bg-destructive/5 p-8">
        <h1 className="text-xl font-semibold">专题合集无法打开</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          它可能不存在，或当前 API 暂时不可用。
        </p>
        <Button asChild variant="outline" className="mt-5">
          <Link href="/collections">返回专题目录</Link>
        </Button>
      </div>
    );
  }

  const collection = collectionState.data;
  const meta = TYPE_META[collection.collectionType];
  const Icon = meta.icon;
  const members = memberState.data?.flatMap((page) => page.items) ?? [];
  const lastPage = memberState.data?.[memberState.data.length - 1];
  const canLoadMore = Boolean(memberState.data && lastPage?.nextCursor);
  const query = collection.query;

  const refreshMembers = async () => {
    await memberState.setSize(1);
    await memberState.mutate();
  };

  return (
    <>
      <header className="border-b border-border/75 pb-7">
        <Button variant="ghost" size="sm" asChild className="-ml-2 mb-5">
          <Link href="/collections">
            <ArrowLeft aria-hidden />
            专题目录
          </Link>
        </Button>
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div>
            <span
              className={cn(
                "flex size-11 items-center justify-center rounded-2xl",
                meta.color,
              )}
            >
              <Icon className="size-5" aria-hidden />
            </span>
            <p className="mt-4 text-xs font-semibold tracking-[0.16em] text-muted-foreground uppercase">
              {meta.label}
            </p>
            <h1 className="mt-1 text-4xl font-semibold tracking-[-0.045em]">
              {collection.title}
            </h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">
              {collectionSummary(collection)}
            </p>
          </div>
          {collection.collectionType !== "dynamic" ? (
            <Button
              type="button"
              disabled={!isAuthenticated || sessionLoading}
              onClick={() => setManageOpen(true)}
            >
              {collection.collectionType === "manual" ? (
                <ListChecks aria-hidden />
              ) : (
                <RefreshCw aria-hidden />
              )}
              {collection.collectionType === "manual" ? "维护成员" : "重建规则"}
            </Button>
          ) : (
            <span className="inline-flex items-center gap-2 rounded-full border bg-violet-50 px-3 py-1.5 text-xs font-medium text-violet-700">
              <CircleGauge className="size-3.5" aria-hidden />
              实时计算
            </span>
          )}
        </div>
      </header>

      <section className="mt-7 grid gap-4 md:grid-cols-3" aria-label="合集定义">
        <div className="rounded-2xl border bg-card p-4">
          <Layers3 className="size-4 text-primary" aria-hidden />
          <p className="mt-3 text-2xl font-semibold">{members.length}</p>
          <p className="mt-1 text-xs text-muted-foreground">当前已加载成员</p>
        </div>
        <div className="rounded-2xl border bg-card p-4 md:col-span-2">
          <p className="text-xs font-semibold text-muted-foreground">稳定 ID</p>
          <p className="mt-3 truncate font-mono text-xs">{collection.id}</p>
          {collection.descriptionPageId ? (
            <Link
              href={`/pages/${collection.descriptionPageId}`}
              className="mt-2 inline-flex items-center gap-1 text-xs text-primary hover:underline"
            >
              打开描述页面
              <ArrowRight className="size-3" aria-hidden />
            </Link>
          ) : null}
        </div>
      </section>

      {query ? (
        <details className="mt-5 rounded-2xl border bg-muted/20">
          <summary className="cursor-pointer px-5 py-4 text-sm font-medium">
            查看版本化查询定义
          </summary>
          <pre className="overflow-x-auto border-t p-5 text-[11px] leading-5">
            {JSON.stringify(query, null, 2)}
          </pre>
        </details>
      ) : null}

      {collection.collectionType === "dynamic" &&
      isDynamicCollectionQuery(query) ? (
        <aside className="mt-5 rounded-2xl border border-violet-200 bg-violet-50/55 p-5">
          <h2 className="flex items-center gap-2 text-sm font-semibold text-violet-900">
            <Sparkles className="size-4" aria-hidden />
            安全实时查询
          </h2>
          <p className="mt-1 text-xs leading-5 text-violet-800/75">
            查询只走 Page/Entity/Claim 索引，不接受 SQL，不扫描 Revision AST。
            {query.memberType === "page"
              ? " 页面改名或别名更新会即时反映。"
              : " Entity 标签与 published Claim 更新会即时反映。"}
          </p>
        </aside>
      ) : null}

      {collection.collectionType === "rule" && isRuleCollectionQuery(query) ? (
        <aside className="mt-5 rounded-2xl border border-amber-200 bg-amber-50/55 p-5 text-xs leading-5 text-amber-900/75">
          Rule Collection 使用显式物化成员，读取稳定且可追溯；点击“重建规则”才会按最新
          Entity / Claim 状态替换成员集。
        </aside>
      ) : null}

      <section className="mt-8" aria-labelledby="collection-members-title">
        <div className="mb-4 flex items-end justify-between gap-3">
          <div>
            <h2 id="collection-members-title" className="text-lg font-semibold">
              合集成员
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              按稳定 sort_key 与目标 ID 分页。
            </p>
          </div>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={memberState.isValidating}
            onClick={() => void memberState.mutate()}
          >
            <RefreshCw
              className={cn(
                "size-3.5",
                memberState.isValidating && "animate-spin",
              )}
              aria-hidden
            />
            刷新
          </Button>
        </div>
        {memberState.error ? (
          <div className="rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            成员查询暂时失败。
          </div>
        ) : (
          <MemberList
            items={members}
            loading={memberState.isLoading && !memberState.data}
          />
        )}
        {canLoadMore ? (
          <Button
            type="button"
            variant="outline"
            className="mt-4 w-full"
            disabled={memberState.isValidating}
            onClick={() => void memberState.setSize(memberState.size + 1)}
          >
            {memberState.isValidating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : null}
            加载更多成员
          </Button>
        ) : null}
      </section>

      <Dialog open={manageOpen} onOpenChange={setManageOpen}>
        <DialogContent className="sm:max-w-3xl">
          {manageOpen && collection.collectionType === "manual" ? (
            <ManualMemberEditor
              collection={collection}
              onClose={() => setManageOpen(false)}
              onSaved={refreshMembers}
            />
          ) : null}
          {manageOpen && collection.collectionType === "rule" ? (
            <RuleRebuild
              collection={collection}
              onClose={() => setManageOpen(false)}
              onSaved={refreshMembers}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}
