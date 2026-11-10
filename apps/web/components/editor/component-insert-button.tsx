"use client";

import { Boxes, LoaderCircle } from "lucide-react";
import { useState } from "react";
import useSWR from "swr";

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
import { ResourcePicker } from "@/components/editor/resource-picker";

function ComponentPicker({ pageId, initialSelection, onClose, onInsert }: {
  pageId?: string;
  initialSelection?: ComponentSelection;
  onClose: () => void;
  onInsert: (selection: ComponentSelection) => void | boolean;
}) {
  const { session } = useSession();
  const [selectedId, setSelectedId] = useState(initialSelection?.componentId ?? "");
  const [selectedVersion, setSelectedVersion] = useState(initialSelection?.componentVersion ?? 0);
  const scope = [session?.actorId, session?.wikiId];
  const bindings = useSWR(session && pageId && !initialSelection ? ["editor:page-bindings", ...scope, pageId] : null,
    () => knowledgeApi().listPageEntityBindings({ id: pageId! }));
  const selected = useSWR(session && selectedId ? ["editor:component", ...scope, selectedId] : null,
    () => componentsApi().getWikiComponent({ id: selectedId }));
  const versions = useSWR(session && selectedId ? ["editor:component-versions", ...scope, selectedId] : null,
    () => componentsApi().listWikiComponentVersions({ id: selectedId }));
  const published = versions.data?.items.filter(item => item.status === "published").sort((a, b) => b.version - a.version) ?? [];
  const version = selectedVersion ? published.find(item => item.version === selectedVersion) : published[0];
  const primary = bindings.data?.items.find(binding => binding.role === "primary");
  const component = selected.data;
  const unavailable = selected.error || versions.error;
  const configured = component && version && !unavailable && !selected.isLoading && !versions.isLoading && !bindings.isLoading;
  return <>
    {!configured ? <DialogHeader><DialogTitle>{initialSelection ? "编辑知识组件" : "插入知识组件"}</DialogTitle><DialogDescription className="sr-only">选择组件、已发布版本和关联实体。</DialogDescription></DialogHeader> : null}
    <div className="grid gap-3 border-b pb-4 sm:grid-cols-2">
      <div className="space-y-2"><Label>组件</Label><ResourcePicker kind="component" value={selectedId} valueLabel={component?.name} onChange={item => { if (item.id !== selectedId) { setSelectedId(item.id); setSelectedVersion(0); } }} /></div>
      <div className="space-y-2"><Label htmlFor="insert-component-version">已发布版本</Label><select id="insert-component-version" value={version?.version ?? selectedVersion} className="h-9 w-full rounded-lg border bg-background px-2 text-sm" onChange={event => setSelectedVersion(Number(event.target.value))}>
        {!version ? <option value={selectedVersion}>{selectedVersion ? `v${selectedVersion} 暂不可用，请选择其他版本` : "请选择组件"}</option> : null}
        {published.map(item => <option key={item.version} value={item.version}>v{item.version}{item === published[0] ? " · 最新" : ""}</option>)}
      </select></div>
    </div>
    {bindings.error ? <p className="text-sm text-muted-foreground">页面关联实体读取失败，可直接选择实体。<Button type="button" variant="ghost" size="sm" onClick={() => void bindings.mutate()}>重试</Button></p> : null}
    {unavailable ? <div>组件读取失败<Button variant="outline" onClick={() => { void selected.mutate(); void versions.mutate(); }}>重试</Button></div> : selected.isLoading || versions.isLoading || bindings.isLoading ? <LoaderCircle className="mx-auto my-8 animate-spin" aria-label="正在加载组件" /> : component && version ?
      <ComponentConfigure key={`${scope.join(":")}:${component.id}:${version.version}`} component={component} version={version}
        initialEntityId={initialSelection?.entityId ?? primary?.entityId} initialLanguage={primary?.language}
        initialDisplayConfig={initialSelection?.componentId === component.id ? initialSelection.displayConfig : undefined}
        submitLabel={initialSelection ? "保存组件" : "插入组件"} onClose={onClose} onInsert={onInsert} /> :
      <p className="text-sm text-muted-foreground">{selectedId ? "该组件暂无可用的已发布版本。" : "按名称搜索并选择一个知识组件。"}</p>}
  </>;
}

export function ComponentInsertDialog({
  open,
  onOpenChange,
  pageId,
  initialSelection,
  onInsert,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pageId?: string;
  initialSelection?: ComponentSelection;
  onInsert: (selection: ComponentSelection) => void | boolean;
}) {
  const { session } = useSession();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-4xl">
        {open ? (
          <ComponentPicker
            key={`${session?.actorId}:${session?.wikiId}`}
            pageId={pageId}
            initialSelection={initialSelection}
            onClose={() => onOpenChange(false)}
            onInsert={onInsert}
          />
        ) : null}
      </DialogContent>
    </Dialog>
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
      <ComponentInsertDialog
        open={open}
        onOpenChange={setOpen}
        pageId={pageId}
        onInsert={onInsert}
      />
    </>
  );
}
