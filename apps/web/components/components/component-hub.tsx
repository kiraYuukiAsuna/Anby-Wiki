"use client";

import Link from "next/link";
import {
  ArrowRight,
  Boxes,
  Component,
  LoaderCircle,
  Plus,
  RefreshCw,
  Sparkles,
} from "lucide-react";
import { Fragment } from "react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";

import type {
  WikiComponent,
  WikiComponentListPage,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { ComponentDefinitionForm } from "@/components/components/component-forms";
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
import { componentsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import {
  COMPONENT_PRESETS,
  COMPONENT_RENDERERS,
  componentDefaultProps,
  parseComponentObject,
  type ComponentPreset,
} from "@/lib/components";
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

function CreateFromExample({
  preset,
  onClose,
}: {
  preset: ComponentPreset;
  onClose: () => void;
}) {
  const router = useRouter();
  const [key, setKey] = useState(preset.key);
  const [name, setName] = useState(preset.name);
  const [renderer, setRenderer] = useState(preset.renderer);
  const [source, setSource] = useState(JSON.stringify(preset.schema, null, 2));
  const [registered, setRegistered] = useState<WikiComponent | null>(null);
  const [creating, setCreating] = useState(false);
  const schema = parseComponentObject(source);
  const values = schema ? componentDefaultProps(schema, renderer) : {};

  const create = async () => {
    if (
      !/^[a-z0-9][a-z0-9._-]*$/.test(key.trim()) ||
      !name.trim() ||
      !schema ||
      schema.type !== "object"
    ) {
      toast.error("请填写名称、合法组件键与字段配置");
      return;
    }
    setCreating(true);
    try {
      const component =
        registered ??
        (await componentsApi().createWikiComponent({
          createWikiComponentRequest: {
            componentKey: key.trim(),
            name: name.trim(),
          },
        }));
      setRegistered(component);
      await componentsApi().createWikiComponentVersion({
        id: component.id,
        writeWikiComponentVersionRequest: {
          propsSchema: schema,
          rendererRef: renderer,
        },
      });
      toast.success("组件草稿已创建", {
        description: "预览确认后发布，即可在编辑器中选择。",
      });
      router.push(`/components/${component.id}`);
      onClose();
    } catch {
      toast.error("创建草稿失败", {
        description:
          "请检查组件键是否重复及配置是否有效。注册成功后重试会继续创建版本。",
      });
    } finally {
      setCreating(false);
    }
  };
  return (
    <>
      <DialogHeader>
        <DialogTitle>定制 {preset.name}</DialogTitle>
        <DialogDescription>
          修改字段和默认内容，创建后预览并发布。
        </DialogDescription>
      </DialogHeader>
      <div className="grid max-h-[65vh] min-w-0 gap-5 overflow-y-auto lg:grid-cols-2">
        <fieldset disabled={creating} className="min-w-0 space-y-4">
          <div className="space-y-2">
            <Label htmlFor="component-name">显示名称</Label>
            <Input
              id="component-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              disabled={Boolean(registered) || creating}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="component-key">组件键</Label>
            <Input
              id="component-key"
              value={key}
              onChange={(event) => setKey(event.target.value)}
              disabled={Boolean(registered) || creating}
            />
            <p className="text-xs text-muted-foreground">
              使用小写字母、数字、点、横线或下划线，用于唯一标识组件。
            </p>
          </div>
          <ComponentDefinitionForm
            renderer={renderer}
            source={source}
            onChange={(next, value) => {
              setRenderer(next);
              setSource(value);
            }}
          />
          {registered ? (
            <p className="text-xs text-muted-foreground">
              注册项已保留。可修改配置后重试，或
              <Link className="underline" href={`/components/${registered.id}`}>
                打开组件继续编辑
              </Link>
              。
            </p>
          ) : null}
        </fieldset>
        <div className="min-w-0 rounded-xl border bg-muted/20 p-4 lg:sticky lg:top-0 lg:self-start">
          <p className="mb-3 text-xs font-medium text-muted-foreground">
            即时示意 · 示例数据
          </p>
          <ExamplePreview
            renderer={renderer}
            name={name}
            values={
              renderer === COMPONENT_RENDERERS.BuiltinKeyValue
                ? values
                : preset.sample
            }
          />
          {renderer === COMPONENT_RENDERERS.BuiltinEntityClaimInfobox ? (
            <p className="mt-3 text-xs text-muted-foreground">
              发布前可选择真实实体，查看服务端生成的实际内容。
            </p>
          ) : null}
        </div>
      </div>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          disabled={creating}
          onClick={onClose}
        >
          取消
        </Button>
        <Button type="button" disabled={creating} onClick={() => void create()}>
          {creating ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <Plus aria-hidden />
          )}
          创建组件草稿
        </Button>
      </DialogFooter>
    </>
  );
}

function ExamplePreview({
  renderer,
  name,
  values,
}: {
  renderer: ComponentPreset["renderer"];
  name: string;
  values: Record<string, unknown>;
}) {
  const entries = Object.entries(values).sort(([left], [right]) =>
    left < right ? -1 : left > right ? 1 : 0,
  );
  return (
    <div className="wiki-rendered wiki-component-preview">
      {renderer === COMPONENT_RENDERERS.BuiltinKeyValue ? (
        <dl data-component-renderer="key-value">
          {entries.map(([key, value]) => (
            <Fragment key={key}>
              <dt>{key}</dt>
              <dd>
                {typeof value === "string" ? value : JSON.stringify(value)}
              </dd>
            </Fragment>
          ))}
        </dl>
      ) : (
        <aside className="entity-infobox">
          <header className="entity-infobox-header">
            <p className="entity-infobox-type">实体事实</p>
            <h2>{name}</h2>
          </header>
          <dl>
            {entries.map(([key, value]) => (
              <div key={key}>
                <dt>{key}</dt>
                <dd>{String(value)}</dd>
              </div>
            ))}
          </dl>
        </aside>
      )}
    </div>
  );
}

export function ComponentHub() {
  const [preset, setPreset] = useState<ComponentPreset | null>(null);
  const { session, isLoading: sessionLoading } = useSession();
  const canCreate = Boolean(
    session &&
    (session.actorType === "system" || session.homeWikiId === session.wikiId),
  );
  const state = useSWRInfinite<WikiComponentListPage>(
    (pageIndex, previousPage) => {
      if (sessionLoading) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "components",
        session?.actorId ?? "",
        session?.wikiId ?? "",
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (cacheKey) => {
      const [, , , cursor] = cacheKey as readonly [
        string,
        string,
        string,
        string,
      ];
      return componentsApi().listWikiComponents({
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
  );
  const items = state.data?.flatMap((page) => page.items) ?? [];
  const lastPage = state.data?.[state.data.length - 1];
  const canLoadMore = Boolean(state.data && lastPage?.nextCursor);

  return (
    <div className="min-w-0 space-y-10">
      <section aria-labelledby="component-examples-title">
        <div className="flex items-center gap-2">
          <Sparkles className="size-5 text-primary" aria-hidden />
          <h2 id="component-examples-title" className="text-xl font-semibold">
            从示例开始
          </h2>
        </div>
        <p className="mt-2 text-sm text-muted-foreground">
          先看效果，再套用并定制。资料卡填写固定内容；动态信息框随实体已发布事实更新。
        </p>
        <ul className="mt-5 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {COMPONENT_PRESETS.map((item) => (
            <li
              key={item.key}
              className="flex min-w-0 flex-col rounded-2xl border bg-card p-5"
            >
              <h3 className="font-semibold">{item.name}</h3>
              <p className="mt-2 min-h-10 text-xs leading-5 text-muted-foreground">
                {item.description}
              </p>
              <div className="my-4 min-w-0 flex-1">
                <ExamplePreview
                  renderer={item.renderer}
                  name={item.name}
                  values={item.sample}
                />
              </div>
              <p className="mb-3 text-[11px] text-muted-foreground">
                示例数据 ·{" "}
                {item.renderer === COMPONENT_RENDERERS.BuiltinKeyValue
                  ? "手动资料卡"
                  : "动态信息框示意"}
              </p>
              {canCreate ? (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setPreset(item)}
                >
                  <Plus aria-hidden />
                  使用并定制
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
        {!canCreate && !sessionLoading ? (
          <p className="mt-4 rounded-xl border bg-muted/30 p-4 text-sm text-muted-foreground">
            {session
              ? "当前为协作 Wiki。切换到自己的 Wiki 后可创建和定制组件。"
              : "登录后可套用示例创建组件。"}
          </p>
        ) : null}
      </section>
      <section aria-labelledby="component-catalog-title">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 id="component-catalog-title" className="text-xl font-semibold">
            组件目录
          </h2>
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
              <div
                key={item}
                className="h-44 animate-pulse rounded-2xl border bg-muted/35"
              />
            ))}
          </div>
        ) : null}
        {!state.isLoading && !state.error && items.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed px-6 py-14 text-center">
            <Boxes
              className="mx-auto size-8 text-muted-foreground"
              aria-hidden
            />
            <h3 className="mt-4 font-semibold">还没有组件</h3>
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

      <Dialog
        open={Boolean(preset)}
        onOpenChange={(open) => {
          if (!open) setPreset(null);
        }}
      >
        <DialogContent className="sm:max-w-4xl">
          {preset && canCreate ? (
            <CreateFromExample
              key={`${session?.actorId}:${session?.wikiId}:${preset.key}`}
              preset={preset}
              onClose={() => setPreset(null)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
