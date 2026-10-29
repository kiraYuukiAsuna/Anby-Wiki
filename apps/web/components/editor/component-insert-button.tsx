"use client";

import Link from "next/link";
import { Boxes, LoaderCircle, RefreshCw } from "lucide-react";
import { useState } from "react";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";

import {
  ComponentConfigure,
  type ComponentSelection,
} from "@/components/components/component-configure";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { componentsApi, knowledgeApi } from "@/lib/api";
import { useSession } from "@/lib/auth";

function ComponentPicker({
  pageId,
  onClose,
  onInsert,
}: {
  pageId?: string;
  onClose: () => void;
  onInsert: (selection: ComponentSelection) => void;
}) {
  const { session } = useSession();
  const [selectedId, setSelectedId] = useState("");
  const [selectedVersion, setSelectedVersion] = useState(0);
  const bindings = useSWR(
    session && pageId
      ? ["editor:page-bindings", session.actorId, session.wikiId, pageId]
      : null,
    () => knowledgeApi().listPageEntityBindings({ id: pageId! }),
  );
  const catalog = useSWRInfinite(
    (index, previous) => {
      if (!session || (index > 0 && !previous?.nextCursor)) return null;
      return [
        "editor:components",
        session.actorId,
        session.wikiId,
        index === 0 ? "" : previous.nextCursor,
      ] as const;
    },
    ([, , , cursor]) =>
      componentsApi().listWikiComponents({
        cursor: cursor || undefined,
        pageSize: 100,
      }),
  );
  const items = catalog.data?.flatMap((page) => page.items) ?? [];
  const component =
    items.find((item) => item.id === selectedId) ??
    items.find((item) => item.componentKey === "article-infobox") ??
    items[0];
  const versions = useSWR(
    session && component
      ? [
          "editor:component-versions",
          session.actorId,
          session.wikiId,
          component.id,
        ]
      : null,
    () => componentsApi().listWikiComponentVersions({ id: component!.id }),
  );
  const published =
    versions.data?.items
      .filter((version) => version.status === "published")
      .sort((left, right) => right.version - left.version) ?? [];
  const version =
    published.find((item) => item.version === selectedVersion) ?? published[0];
  const primary = bindings.data?.items.find(
    (binding) => binding.role === "primary",
  );
  const unavailable = catalog.error || versions.error;

  return (
    <>
      <div className="grid gap-3 border-b pb-4 pr-8 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="insert-component">组件</Label>
          <select
            id="insert-component"
            value={component?.id ?? ""}
            className="h-9 w-full min-w-0 rounded-lg border bg-background px-2 text-sm"
            onChange={(event) => {
              setSelectedId(event.target.value);
              setSelectedVersion(0);
            }}
          >
            {!items.length ? <option value="">暂无组件</option> : null}
            {items.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="insert-component-version">已发布版本</Label>
          <select
            id="insert-component-version"
            value={version?.version ?? 0}
            className="h-9 w-full rounded-lg border bg-background px-2 text-sm"
            onChange={(event) => setSelectedVersion(Number(event.target.value))}
          >
            {!published.length ? (
              <option value={0}>暂无已发布版本</option>
            ) : null}
            {published.map((item) => (
              <option key={item.version} value={item.version}>
                v{item.version}
                {item === published[0] ? " · 最新" : ""}
              </option>
            ))}
          </select>
        </div>
        {catalog.data?.at(-1)?.nextCursor ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={catalog.isValidating}
            onClick={() => void catalog.setSize(catalog.size + 1)}
          >
            加载更多组件
          </Button>
        ) : null}
      </div>
      {bindings.error ? (
        <p className="text-xs text-muted-foreground">
          页面主实体读取失败，可在下方手动选择实体。
        </p>
      ) : null}
      {unavailable ? (
        <>
          <DialogHeader>
            <DialogTitle>组件暂时无法读取</DialogTitle>
            <DialogDescription>请重试加载组件目录和版本。</DialogDescription>
          </DialogHeader>
          <Button
            variant="outline"
            onClick={() => {
              void catalog.mutate();
              void versions.mutate();
            }}
          >
            <RefreshCw aria-hidden />
            重试
          </Button>
        </>
      ) : catalog.isLoading || versions.isLoading || bindings.isLoading ? (
        <>
          <DialogHeader>
            <DialogTitle>插入组件</DialogTitle>
            <DialogDescription>正在加载组件和页面实体…</DialogDescription>
          </DialogHeader>
          <LoaderCircle className="mx-auto my-8 animate-spin" aria-hidden />
        </>
      ) : component && version ? (
        <ComponentConfigure
          key={`${session?.actorId}:${session?.wikiId}:${component.id}:${version.version}`}
          component={component}
          version={version}
          initialEntityId={primary?.entityId}
          initialLanguage={primary?.language}
          onClose={onClose}
          onInsert={onInsert}
        />
      ) : (
        <DialogHeader>
          <DialogTitle>暂无可插入的已发布版本</DialogTitle>
          <DialogDescription>
            选择其他组件，或
            <Link
              href={component ? `/components/${component.id}` : "/components"}
              className="underline"
            >
              前往组件中心
            </Link>
            创建、预览并发布一个版本。
          </DialogDescription>
        </DialogHeader>
      )}
    </>
  );
}

export function ComponentInsertButton({
  pageId,
  onInsert,
}: {
  pageId?: string;
  onInsert: (selection: ComponentSelection) => void;
}) {
  const [open, setOpen] = useState(false);
  const { session } = useSession();
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
      >
        <Boxes className="size-4" aria-hidden />
        组件
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-4xl">
          {open ? (
            <ComponentPicker
              key={`${session?.actorId}:${session?.wikiId}`}
              pageId={pageId}
              onClose={() => setOpen(false)}
              onInsert={onInsert}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}
