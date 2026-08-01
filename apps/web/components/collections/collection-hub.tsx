"use client";

import Link from "next/link";
import {
  ArrowRight,
  Layers3,
  ListChecks,
  LoaderCircle,
  Plus,
  RefreshCw,
  Sparkles,
  Workflow,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";

import type {
  Collection,
  CollectionListPage,
  CreateCollectionRequest,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { collectionsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { collectionSummary } from "@/lib/collections";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 24;
type CollectionType = CreateCollectionRequest["collectionType"];

const TYPE_META = {
  manual: {
    label: "Manual",
    icon: ListChecks,
    color: "bg-sky-100 text-sky-700",
  },
  rule: {
    label: "Rule",
    icon: Workflow,
    color: "bg-amber-100 text-amber-700",
  },
  dynamic: {
    label: "Dynamic",
    icon: Sparkles,
    color: "bg-violet-100 text-violet-700",
  },
} as const;

function CollectionCard({ collection }: { collection: Collection }) {
  const meta = TYPE_META[collection.collectionType];
  const Icon = meta.icon;
  return (
    <li>
      <Link
        href={`/collections/${collection.id}`}
        className="group block rounded-2xl border bg-card p-5 transition hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-[0_14px_35px_rgb(15_23_42/0.08)]"
      >
        <div className="flex items-start justify-between gap-3">
          <span
            className={cn(
              "flex size-10 items-center justify-center rounded-xl",
              meta.color,
            )}
          >
            <Icon className="size-4.5" aria-hidden />
          </span>
          <span className="rounded-full border bg-background px-2 py-0.5 text-[10px] font-semibold tracking-wide">
            {meta.label}
          </span>
        </div>
        <div className="mt-5 flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="truncate font-semibold">{collection.title}</h3>
            <p className="mt-2 line-clamp-2 text-xs leading-5 text-muted-foreground">
              {collectionSummary(collection)}
            </p>
          </div>
          <ArrowRight
            className="mt-1 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-primary"
            aria-hidden
          />
        </div>
      </Link>
    </li>
  );
}

export function CollectionHub() {
  const [title, setTitle] = useState("");
  const [type, setType] = useState<CollectionType>("dynamic");
  const [descriptionPageID, setDescriptionPageID] = useState("");
  const [ruleKind, setRuleKind] = useState<"entity_type" | "claim_exists">(
    "entity_type",
  );
  const [ruleValue, setRuleValue] = useState("");
  const [memberType, setMemberType] = useState<"page" | "entity">("page");
  const [text, setText] = useState("");
  const [scope, setScope] = useState("");
  const [property, setProperty] = useState("");
  const [creating, setCreating] = useState(false);
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const state = useSWRInfinite<CollectionListPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "collections",
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, cursor] = cacheKey as readonly [string, string];
      return collectionsApi().listCollections({
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );
  const items = state.data?.flatMap((page) => page.items) ?? [];
  const lastPage = state.data?.[state.data.length - 1];
  const canLoadMore = Boolean(state.data && lastPage?.nextCursor);

  const create = async () => {
    if (!title.trim()) {
      toast.error("请填写合集标题");
      return;
    }
    let query: CreateCollectionRequest["query"];
    if (type === "rule") {
      if (!ruleValue.trim()) {
        toast.error("请填写规则引用键");
        return;
      }
      query =
        ruleKind === "entity_type"
          ? { version: 1, kind: "entity_type", entityType: ruleValue.trim() }
          : { version: 1, kind: "claim_exists", property: ruleValue.trim() };
    } else if (type === "dynamic") {
      query = {
        version: 1,
        memberType,
        text: text.trim() || undefined,
        namespace:
          memberType === "page" ? scope.trim() || undefined : undefined,
        entityType:
          memberType === "entity" ? scope.trim() || undefined : undefined,
        property:
          memberType === "entity" ? property.trim() || undefined : undefined,
      };
    }
    setCreating(true);
    try {
      await collectionsApi().createCollection({
        createCollectionRequest: {
          collectionType: type,
          title: title.trim(),
          descriptionPageId: descriptionPageID.trim() || undefined,
          query,
        },
      });
      await state.mutate();
      setTitle("");
      toast.success("专题合集已创建");
    } catch {
      toast.error("创建合集失败", {
        description: "请检查 Page、EntityType、Property 或 Namespace 引用。",
      });
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <section aria-labelledby="collection-catalog-title">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="collection-catalog-title" className="text-xl font-semibold">
              专题目录
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              人工策展、规则物化与实时查询使用同一个稳定入口。
            </p>
          </div>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={state.isValidating}
            onClick={() => void state.mutate()}
          >
            <RefreshCw
              className={cn("size-3.5", state.isValidating && "animate-spin")}
              aria-hidden
            />
            刷新
          </Button>
        </div>
        {state.error ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            专题目录暂时无法读取。
          </div>
        ) : null}
        {state.isLoading && !state.data ? (
          <div className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {[0, 1, 2].map((item) => (
              <div key={item} className="h-48 animate-pulse rounded-2xl border bg-muted/35" />
            ))}
          </div>
        ) : null}
        {!state.isLoading && !state.error && items.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
            <Layers3 className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">还没有专题合集</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              用右侧设计器创建第一个长期可发现的知识入口。
            </p>
          </div>
        ) : null}
        {items.length > 0 ? (
          <ul className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {items.map((item) => (
              <CollectionCard key={item.id} collection={item} />
            ))}
          </ul>
        ) : null}
        {canLoadMore ? (
          <Button
            type="button"
            variant="outline"
            className="mt-4 w-full"
            onClick={() => void state.setSize(state.size + 1)}
          >
            加载更多
          </Button>
        ) : null}
      </section>

      <aside className="xl:sticky xl:top-24 xl:self-start">
        <div className="rounded-2xl border bg-card p-5">
          <span className="flex size-10 items-center justify-center rounded-xl bg-primary/9 text-primary">
            <Plus className="size-4.5" aria-hidden />
          </span>
          <h2 className="mt-4 font-semibold">创建专题合集</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            Dynamic 查询只接受版本化字段，不允许任意 SQL。
          </p>
          <div className="mt-5 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="collection-title">标题</Label>
              <Input
                id="collection-title"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder="例如：值得关注的角色"
                disabled={!isAuthenticated || creating || sessionLoading}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="collection-type">维护方式</Label>
              <select
                id="collection-type"
                value={type}
                onChange={(event) => setType(event.target.value as CollectionType)}
                className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
                disabled={!isAuthenticated || creating || sessionLoading}
              >
                <option value="dynamic">Dynamic · 实时查询</option>
                <option value="manual">Manual · 人工策展</option>
                <option value="rule">Rule · 规则物化</option>
              </select>
            </div>

            {type === "rule" ? (
              <div className="grid grid-cols-[8rem_1fr] gap-2">
                <select
                  value={ruleKind}
                  onChange={(event) =>
                    setRuleKind(
                      event.target.value as "entity_type" | "claim_exists",
                    )
                  }
                  className="h-8 rounded-lg border border-input bg-background px-2 text-xs"
                  aria-label="规则类型"
                >
                  <option value="entity_type">实体类型</option>
                  <option value="claim_exists">存在属性</option>
                </select>
                <Input
                  value={ruleValue}
                  onChange={(event) => setRuleValue(event.target.value)}
                  placeholder={ruleKind === "entity_type" ? "character" : "author"}
                  aria-label="规则引用键"
                  className="font-mono text-xs"
                />
              </div>
            ) : null}

            {type === "dynamic" ? (
              <div className="space-y-3 rounded-xl border bg-muted/20 p-3">
                <div className="grid grid-cols-[7rem_1fr] gap-2">
                  <select
                    value={memberType}
                    onChange={(event) =>
                      setMemberType(event.target.value as "page" | "entity")
                    }
                    className="h-8 rounded-lg border border-input bg-background px-2 text-xs"
                    aria-label="动态成员类型"
                  >
                    <option value="page">页面</option>
                    <option value="entity">实体</option>
                  </select>
                  <Input
                    value={text}
                    onChange={(event) => setText(event.target.value)}
                    placeholder="标题或别名关键词（可选）"
                    aria-label="动态查询关键词"
                  />
                </div>
                <Input
                  value={scope}
                  onChange={(event) => setScope(event.target.value)}
                  placeholder={
                    memberType === "page"
                      ? "Namespace key（可选）"
                      : "EntityType key（可选）"
                  }
                  className="font-mono text-xs"
                />
                {memberType === "entity" ? (
                  <Input
                    value={property}
                    onChange={(event) => setProperty(event.target.value)}
                    placeholder="要求存在的 Property key（可选）"
                    className="font-mono text-xs"
                  />
                ) : null}
              </div>
            ) : null}

            <div className="space-y-2">
              <Label htmlFor="collection-description-page">
                描述 Page ID（可选）
              </Label>
              <Input
                id="collection-description-page"
                value={descriptionPageID}
                onChange={(event) => setDescriptionPageID(event.target.value)}
                placeholder="UUID"
                className="font-mono text-xs"
              />
            </div>
            <Button
              type="button"
              className="w-full"
              disabled={!isAuthenticated || creating || sessionLoading}
              onClick={() => void create()}
            >
              {creating ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Plus aria-hidden />
              )}
              {isAuthenticated ? "创建合集" : "登录后创建"}
            </Button>
          </div>
        </div>
      </aside>
    </div>
  );
}
