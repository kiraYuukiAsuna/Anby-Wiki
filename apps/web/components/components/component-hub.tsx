"use client";

import Link from "next/link";
import {
  ArrowRight,
  Boxes,
  Component,
  LoaderCircle,
  Plus,
  RefreshCw,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";

import type {
  WikiComponent,
  WikiComponentListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { componentsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 24;

function ComponentCard({ item }: { item: WikiComponent }) {
  return (
    <li>
      <Link
        href={`/components/${item.id}`}
        className="group block rounded-2xl border bg-card p-5 transition hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-[0_14px_35px_rgb(15_23_42/0.08)]"
      >
        <span className="flex size-10 items-center justify-center rounded-xl bg-violet-100 text-violet-700">
          <Component className="size-4.5" aria-hidden />
        </span>
        <div className="mt-5 flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="truncate font-semibold">{item.name}</h3>
            <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
              {item.componentKey}
            </p>
          </div>
          <ArrowRight
            className="mt-1 size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-primary"
            aria-hidden
          />
        </div>
      </Link>
    </li>
  );
}

export function ComponentHub() {
  const [key, setKey] = useState("");
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const state = useSWRInfinite<WikiComponentListPage>(
    (pageIndex, previousPage) => {
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "components",
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, cursor] = cacheKey as readonly [string, string];
      return componentsApi().listWikiComponents({
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );
  const items = state.data?.flatMap((page) => page.items) ?? [];
  const lastPage = state.data?.[state.data.length - 1];
  const canLoadMore = Boolean(state.data && lastPage?.nextCursor);

  const create = async () => {
    if (!key.trim() || !name.trim()) {
      toast.error("请填写组件键与显示名称");
      return;
    }
    setCreating(true);
    try {
      const created = await componentsApi().createWikiComponent({
        createWikiComponentRequest: {
          componentKey: key.trim().toLowerCase(),
          name: name.trim(),
        },
      });
      await state.mutate();
      setKey("");
      setName("");
      toast.success("组件注册项已创建", {
        description: `${created.name} · 下一步创建第一个版本`,
      });
    } catch {
      toast.error("创建组件失败", {
        description: "组件键只能使用小写字母、数字、点、横线与下划线，且必须唯一。",
      });
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_22rem]">
      <section aria-labelledby="component-catalog-title">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="component-catalog-title" className="text-xl font-semibold">
              组件目录
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              稳定组件身份与不可变版本分离，旧 Revision 始终可重现。
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
            组件目录暂时无法读取。
          </div>
        ) : null}
        {state.isLoading && !state.data ? (
          <div className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {[0, 1, 2].map((item) => (
              <div key={item} className="h-44 animate-pulse rounded-2xl border bg-muted/35" />
            ))}
          </div>
        ) : null}
        {!state.isLoading && !state.error && items.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
            <Boxes className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">还没有组件</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              注册第一个信息框或可信展示组件。
            </p>
          </div>
        ) : null}
        {items.length > 0 ? (
          <ul className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {items.map((item) => (
              <ComponentCard key={item.id} item={item} />
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
          <h2 className="mt-4 font-semibold">注册新组件</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            先注册稳定身份，再为它创建带 JSON Schema 与可信渲染器的不可变版本。
          </p>
          <div className="mt-5 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="component-key">组件键</Label>
              <Input
                id="component-key"
                value={key}
                onChange={(event) => setKey(event.target.value)}
                placeholder="character.infobox"
                className="font-mono"
                disabled={!isAuthenticated || creating || sessionLoading}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="component-name">显示名称</Label>
              <Input
                id="component-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="角色信息框"
                disabled={!isAuthenticated || creating || sessionLoading}
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
              {isAuthenticated ? "创建注册项" : "登录后创建"}
            </Button>
          </div>
        </div>
      </aside>
    </div>
  );
}
