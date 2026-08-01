"use client";

import Link from "next/link";
import {
  ArrowRight,
  BadgeCheck,
  Bot,
  FileClock,
  GitCommitHorizontal,
  Globe2,
  Inbox,
  LoaderCircle,
  Plus,
  RefreshCw,
  ScrollText,
  Tag,
  UserRound,
} from "lucide-react";
import { FormEvent, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";
import { z } from "zod";

import type {
  AuditEvent,
  AuditEventListPage,
  ChangeTag,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { governanceApi } from "@/lib/api";
import { LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 30;
const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
});

const AGGREGATE_TYPES = [
  ["", "全部聚合"],
  ["page", "Page"],
  ["working_document", "WorkingDocument"],
  ["entity", "Entity"],
  ["federated_wiki", "FederatedWiki"],
  ["entity_federation_link", "FederationLink"],
  ["claim", "Claim"],
  ["collection", "Collection"],
  ["page_protection", "PageProtection"],
  ["block_redirect", "BlockRedirect"],
  ["proposal", "Proposal"],
  ["entity_merge", "EntityMerge"],
  ["ai_trust", "AITrust"],
  ["change_tag", "ChangeTag"],
] as const;

const tagSchema = z.object({
  tagKey: z
    .string()
    .regex(/^[a-z][a-z0-9._-]{0,63}$/, "Key 需为小写机器标识"),
  name: z.string().trim().min(1, "请填写标签名称").max(120),
  description: z.string().trim().max(1000),
});

function eventTitle(eventType: string): string {
  const labels: Record<string, string> = {
    "page.created": "创建页面",
    "page.renamed": "页面改名",
    "page.redirected": "更新页面重定向",
    "page.deleted": "补偿删除新建页面",
    "revision.published": "发布 Revision",
    "revision.rolled_back": "发布回滚补偿版本",
    "working_document.created": "打开协作文档",
    "working_document.snapshotted": "保存 CRDT 快照",
    "working_document.rebased": "协作文档换基",
    "proposal.applied": "应用 Proposal",
    "proposal.rolled_back": "回滚 ChangeBatch",
    "proposal.conflict_resolved": "解决合并冲突",
    "claim.changed": "更新 Claim",
    "claim.rolled_back": "补偿恢复 Claim",
    "claim.source_removed": "补偿移除 Claim 来源",
    "entity.created": "创建 Entity",
    "entity.deleted": "补偿删除新建 Entity",
    "entity.merged": "合并 Entity",
    "entity.merge_rolled_back": "补偿恢复 Entity 合并",
    "federated_wiki.registered": "登记远端 Wiki",
    "federated_wiki.updated": "更新远端 Wiki",
    "entity_federation.linked": "创建跨 Wiki 身份映射",
    "entity_federation.updated": "更新跨 Wiki 身份映射",
    "collection.membership_added": "加入 Collection 成员",
    "collection.membership_removed": "移除 Collection 成员",
    "page_protection.created": "启用页面保护",
    "page_protection.deleted": "撤销页面保护",
    "block_redirect.updated": "更新稳定章节迁移",
    "block_redirect.deleted": "删除稳定章节迁移",
    "ai_trust.updated": "更新 AI 信任策略",
    "change_tag.created": "创建变更标签",
    "change_tag.assigned": "追加变更标签",
  };
  return labels[eventType] ?? eventType;
}

function targetHref(event: AuditEvent): string | undefined {
  switch (event.aggregateType) {
    case "page":
      return `/pages/${event.aggregateId}`;
    case "entity":
      return `/entities/${event.aggregateId}`;
    case "federated_wiki":
    case "entity_federation_link":
      return "/federation";
    case "claim":
      return `/claims/${event.aggregateId}`;
    case "collection":
      return `/collections/${event.aggregateId}`;
    case "proposal":
      return `/governance/proposals/${event.aggregateId}`;
    case "page_protection":
      return "/governance/protections";
    case "ai_trust":
      return "/governance/ai-trust";
    default:
      return undefined;
  }
}

function EventIcon({ event }: { event: AuditEvent }) {
  if (event.eventType.includes("proposal") || event.eventType.includes("claim")) {
    return <GitCommitHorizontal className="size-4" aria-hidden />;
  }
  if (event.eventType.includes("working_document")) {
    return <FileClock className="size-4" aria-hidden />;
  }
  if (event.eventType.includes("tag")) {
    return <Tag className="size-4" aria-hidden />;
  }
  if (event.eventType.includes("ai") || event.eventType.includes("import")) {
    return <Bot className="size-4" aria-hidden />;
  }
  if (event.eventType.includes("federat")) {
    return <Globe2 className="size-4" aria-hidden />;
  }
  return <ScrollText className="size-4" aria-hidden />;
}

function TagPill({ tag }: { tag: ChangeTag }) {
  return (
    <span
      title={tag.description}
      className="inline-flex items-center gap-1 rounded-full border border-violet-200 bg-violet-50 px-2 py-0.5 text-[10px] font-medium text-violet-700"
    >
      <Tag className="size-2.5" aria-hidden />
      {tag.name}
    </span>
  );
}

function AuditCard({
  event,
  tags,
  onTagged,
}: {
  event: AuditEvent;
  tags: ChangeTag[];
  onTagged: () => Promise<void>;
}) {
  const [selectedTag, setSelectedTag] = useState("");
  const [saving, setSaving] = useState(false);
  const href = targetHref(event);
  const availableTags = tags.filter(
    (tag) => !event.tags.some((current) => current.id === tag.id),
  );

  const assign = async () => {
    if (!selectedTag) return;
    setSaving(true);
    try {
      await governanceApi().assignChangeTag({
        id: selectedTag,
        assignChangeTagRequest: {
          targetType: "audit_event",
          targetId: event.id,
        },
      });
      setSelectedTag("");
      await onTagged();
      toast.success("标签已追加到审计事件");
    } catch {
      toast.error("标签追加失败", {
        description: "需要 reviewer 或 admin 权限。",
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <li className="rounded-2xl border bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)]">
      <div className="flex items-start gap-4">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-violet-100 text-violet-700">
          <EventIcon event={event} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="font-semibold">{eventTitle(event.eventType)}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                <span className="inline-flex items-center gap-1">
                  <UserRound className="size-3" aria-hidden />
                  {event.actorDisplayName}
                </span>
                {" · "}
                <time dateTime={event.createdAt.toISOString()}>
                  {DATE_FORMATTER.format(event.createdAt)}
                </time>
              </p>
            </div>
            <span className="rounded-full bg-muted px-2.5 py-1 font-mono text-[10px] text-muted-foreground">
              {event.aggregateType}
            </span>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            {event.tags.map((tag) => (
              <TagPill key={tag.id} tag={tag} />
            ))}
            {event.changeBatchId ? (
              <span className="rounded-full border px-2 py-0.5 font-mono text-[10px] text-muted-foreground">
                batch {event.changeBatchId.slice(0, 8)}
              </span>
            ) : null}
          </div>
          <div className="mt-4 flex flex-wrap items-center gap-3 border-t pt-4">
            <span className="min-w-0 truncate font-mono text-[10px] text-muted-foreground">
              {event.aggregateId}
            </span>
            {href ? (
              <Link
                href={href}
                className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
              >
                打开目标
                <ArrowRight className="size-3" aria-hidden />
              </Link>
            ) : null}
            <details className="basis-full">
              <summary className="cursor-pointer text-xs font-medium text-muted-foreground hover:text-foreground">
                查看结构化 Payload
              </summary>
              <pre className="mt-3 max-h-60 overflow-auto rounded-xl bg-muted/45 p-3 text-[10px] leading-5">
                {JSON.stringify(event.payload, null, 2)}
              </pre>
            </details>
          </div>
          {availableTags.length > 0 ? (
            <div className="mt-3 flex max-w-sm gap-2">
              <select
                value={selectedTag}
                onChange={(change) => setSelectedTag(change.target.value)}
                aria-label="选择审计标签"
                className="h-8 min-w-0 flex-1 rounded-lg border border-input bg-background px-2 text-xs"
              >
                <option value="">追加标签…</option>
                {availableTags.map((tag) => (
                  <option key={tag.id} value={tag.id}>
                    {tag.name} · {tag.tagKey}
                  </option>
                ))}
              </select>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={!selectedTag || saving}
                onClick={() => void assign()}
              >
                {saving ? (
                  <LoaderCircle className="animate-spin" aria-hidden />
                ) : (
                  <Tag aria-hidden />
                )}
                追加
              </Button>
            </div>
          ) : null}
        </div>
      </div>
    </li>
  );
}

export function AuditActivity() {
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const [aggregateType, setAggregateType] = useState("");
  const [eventType, setEventType] = useState("");
  const [tagKey, setTagKey] = useState("");
  const [newTagKey, setNewTagKey] = useState("");
  const [newTagName, setNewTagName] = useState("");
  const [newTagDescription, setNewTagDescription] = useState("");
  const [creating, setCreating] = useState(false);

  const tagState = useSWR(
    "governance:change-tags",
    () => governanceApi().listChangeTags(),
    { revalidateOnFocus: false },
  );
  const activity = useSWRInfinite<AuditEventListPage>(
    (pageIndex, previousPage) => {
      if (!isAuthenticated) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "governance:audit",
        aggregateType,
        eventType,
        tagKey,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, aggregate, event, tag, cursor] = cacheKey as readonly [
        string,
        string,
        string,
        string,
        string,
      ];
      return governanceApi().listAuditEvents({
        aggregateType: aggregate || undefined,
        eventType: event || undefined,
        tagKey: tag || undefined,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
    { revalidateFirstPage: true },
  );
  const items = activity.data?.flatMap((page) => page.items) ?? [];
  const lastPage = activity.data?.[activity.data.length - 1];

  const createTag = async (event: FormEvent) => {
    event.preventDefault();
    const parsed = tagSchema.safeParse({
      tagKey: newTagKey.trim(),
      name: newTagName,
      description: newTagDescription,
    });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查标签");
      return;
    }
    setCreating(true);
    try {
      await governanceApi().createChangeTag({
        createChangeTagRequest: parsed.data,
      });
      setNewTagKey("");
      setNewTagName("");
      setNewTagDescription("");
      await tagState.mutate();
      toast.success("不可变 ChangeTag 已创建");
    } catch {
      toast.error("创建标签失败", {
        description: "Tag key 不能重复，且创建全局词表需要 admin 权限。",
      });
    } finally {
      setCreating(false);
    }
  };

  if (!sessionLoading && !isAuthenticated) {
    return (
      <div className="rounded-3xl border border-dashed px-6 py-16 text-center">
        <ScrollText className="mx-auto size-9 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后查看 Wiki 审计流</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          审计 Payload 只向具有 reviewer 或 admin 权限的成员开放。
        </p>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>登录</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_21rem]">
      <section aria-labelledby="audit-stream-title">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h2 id="audit-stream-title" className="text-xl font-semibold">
              不可变活动流
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              按创建时间倒序，游标分页不丢失并发写入。
            </p>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={activity.isValidating}
            onClick={() => void activity.mutate()}
          >
            <RefreshCw
              className={cn(
                "size-3.5",
                activity.isValidating && "animate-spin",
              )}
              aria-hidden
            />
            刷新
          </Button>
        </div>
        <div className="mt-5 grid gap-2 sm:grid-cols-3">
          <select
            value={aggregateType}
            onChange={(event) => {
              setAggregateType(event.target.value);
              void activity.setSize(1);
            }}
            className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
            aria-label="聚合类型"
          >
            {AGGREGATE_TYPES.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
          <Input
            value={eventType}
            onChange={(event) => setEventType(event.target.value)}
            onBlur={() => void activity.setSize(1)}
            placeholder="event_type 精确过滤"
          />
          <select
            value={tagKey}
            onChange={(event) => {
              setTagKey(event.target.value);
              void activity.setSize(1);
            }}
            className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
            aria-label="变更标签"
          >
            <option value="">全部标签</option>
            {(tagState.data?.items ?? []).map((tag) => (
              <option key={tag.id} value={tag.tagKey}>
                {tag.name}
              </option>
            ))}
          </select>
        </div>

        {activity.isLoading && !activity.data ? (
          <div className="mt-5 space-y-3">
            {[0, 1, 2].map((item) => (
              <div
                key={item}
                className="h-44 animate-pulse rounded-2xl border bg-muted/35"
              />
            ))}
          </div>
        ) : null}
        {activity.error ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm">
            <p className="font-medium text-destructive">审计流无法读取</p>
            <p className="mt-1 text-muted-foreground">
              当前账号可能没有 reviewer 权限，或 API 暂时不可用。
            </p>
          </div>
        ) : null}
        {!activity.isLoading && !activity.error && items.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
            <Inbox className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">没有匹配的审计事件</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              调整过滤条件，或在完成第一笔权威变更后回来查看。
            </p>
          </div>
        ) : null}
        {items.length > 0 ? (
          <ol className="mt-5 space-y-3">
            {items.map((event) => (
              <AuditCard
                key={event.id}
                event={event}
                tags={tagState.data?.items ?? []}
                onTagged={async () => {
                  await activity.mutate();
                }}
              />
            ))}
          </ol>
        ) : null}
        {lastPage?.nextCursor ? (
          <Button
            type="button"
            variant="outline"
            className="mt-4 w-full"
            disabled={activity.isValidating}
            onClick={() => void activity.setSize(activity.size + 1)}
          >
            {activity.isValidating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : null}
            加载更早事件
          </Button>
        ) : null}
      </section>

      <aside className="space-y-4 xl:sticky xl:top-24 xl:self-start">
        <div className="rounded-2xl border bg-card p-5">
          <BadgeCheck className="size-5 text-violet-700" aria-hidden />
          <h2 className="mt-4 font-semibold">ChangeTag 词表</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            标签与关联都只增不改，便于统计、审计与长期策略判断。
          </p>
          <div className="mt-4 flex flex-wrap gap-2">
            {(tagState.data?.items ?? []).map((tag) => (
              <TagPill key={tag.id} tag={tag} />
            ))}
          </div>
        </div>
        <form onSubmit={createTag} className="rounded-2xl border bg-card p-5">
          <span className="flex size-9 items-center justify-center rounded-xl bg-violet-100 text-violet-700">
            <Plus className="size-4" aria-hidden />
          </span>
          <h2 className="mt-4 font-semibold">创建全局标签</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            仅 admin 可扩展词表；创建后不可重命名或删除。
          </p>
          <div className="mt-4 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="tag-key">机器 Key</Label>
              <Input
                id="tag-key"
                value={newTagKey}
                onChange={(event) => setNewTagKey(event.target.value)}
                placeholder="needs-review"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="tag-name">显示名称</Label>
              <Input
                id="tag-name"
                value={newTagName}
                onChange={(event) => setNewTagName(event.target.value)}
                placeholder="需要复核"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="tag-description">说明</Label>
              <Textarea
                id="tag-description"
                value={newTagDescription}
                onChange={(event) => setNewTagDescription(event.target.value)}
                className="min-h-20"
              />
            </div>
          </div>
          <Button type="submit" className="mt-5 w-full" disabled={creating}>
            {creating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : (
              <Plus aria-hidden />
            )}
            创建 ChangeTag
          </Button>
        </form>
      </aside>
    </div>
  );
}
