"use client";

import { useEffect, useState } from "react";
import { Command } from "cmdk";
import { Search } from "lucide-react";
import useSWR, { useSWRConfig } from "swr";
import { toast } from "sonner";
import useSWRInfinite from "swr/infinite";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { assetsApi, componentsApi, knowledgeApi, projectionApi, readingApi, searchApi, wikiApi } from "@/lib/api";
import { useSession } from "@/lib/auth";

export type ResourceKind = "page" | "heading" | "entity" | "claim" | "citation" | "member" | "component" | "asset";
export type MediaKind = "image" | "video" | "audio" | "file" | "pdf";
export type ResourceChoice = { id: string; label: string; description?: string; pageId?: string };
const names: Record<ResourceKind, string> = { page: "页面", heading: "章节", entity: "实体", claim: "事实", citation: "引文", member: "人员", component: "组件", asset: "文件" };

export async function loadResourceChoices(kind: ResourceKind, query: string, cursor: string, mediaKind?: MediaKind, pageId?: string): Promise<{ items: ResourceChoice[]; nextCursor?: string }> {
  const params = { q: query || undefined, cursor: cursor || undefined, pageSize: 20 };
  switch (kind) {
    case "claim": return knowledgeApi().listClaimOptions(params);
    case "citation": return knowledgeApi().listCitationOptions(params);
    case "member": return wikiApi().listMemberOptions(params);
    case "entity": {
      const result = await knowledgeApi().listEntities({ ...params, status: "active" });
      return { ...result, items: result.items.map(item => ({ id: item.id, label: item.displayLabel, description: item.entityType.name })) };
    }
    case "page": {
      if (!query) {
        const result = await readingApi().listPublishedPages({ cursor: cursor || undefined, pageSize: 20 });
        return { ...result, items: result.items.map(item => ({ id: item.id, label: item.displayTitle })) };
      }
      const offset = Number(cursor || 0);
      const result = await searchApi().searchPages({ q: query, fields: ["title", "alias"], limit: 20, offset });
      return { items: result.items.map(item => ({ id: item.id, label: item.displayTitle, description: item.namespace })), nextCursor: result.items.length === 20 ? String(offset + 20) : undefined };
    }
    case "heading": {
      if (!pageId) return { items: [] };
      const result = await projectionApi().getPageOutline({ id: pageId });
      return { items: result.items.map(item => ({ id: item.headingBlockId, label: item.title || "未命名章节", description: `${item.level} 级标题`, pageId })) };
    }
    // shortcut: These catalogs have no server search; move filtering server-side when catalogs grow.
    case "component": {
      const items: ResourceChoice[] = [];
      let next: string | undefined;
      do {
        const page = await componentsApi().listWikiComponents({ cursor: next, pageSize: 100 });
        items.push(...page.items.map(item => ({ id: item.id, label: item.name, description: item.componentKey })));
        next = page.nextCursor || undefined;
      } while (next);
      return { items };
    }
    case "asset": {
      const items: ResourceChoice[] = [];
      let next: string | undefined;
      do {
        const page = await assetsApi().listAssets({ cursor: next, pageSize: 100, kind: mediaKind === "file" ? undefined : mediaKind });
        items.push(...page.items.filter(item => item.status === "active").map(item => ({ id: item.currentRevision.id, label: item.name, description: `${item.currentRevision.mimeType} · ${Math.ceil(item.currentRevision.sizeBytes / 1024)} KB` })));
        next = page.nextCursor || undefined;
      } while (next);
      return { items };
    }
  }
}

async function selectedResourceName(kind: ResourceKind, id: string): Promise<string> {
  switch (kind) {
    case "component": return (await componentsApi().getWikiComponent({ id })).name;
    case "page": return (await readingApi().getPageByID({ id })).page.displayTitle;
    case "entity": {
      const entity = await knowledgeApi().getEntity({ id });
      return entity.labels.find(label => label.isPrimary)?.label || entity.canonicalKey;
    }
    case "claim": return (await knowledgeApi().getClaim({ id })).property.name;
    case "citation": return (await knowledgeApi().getCitation({ id })).source.title;
    default: return `已选${names[kind]}`;
  }
}

export function ResourcePicker({ kind, value, valueLabel, onChange, mediaKind, pageId, clearable = false, label = names[kind] }: {
  kind: ResourceKind; value: string; valueLabel?: string; onChange: (choice: ResourceChoice) => void | boolean | Promise<void | boolean>;
  mediaKind?: MediaKind; pageId?: string; clearable?: boolean; label?: string;
}) {
  const { session } = useSession();
  const { mutate } = useSWRConfig();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => { const timer = setTimeout(() => setDebounced(query.trim()), 200); return () => clearTimeout(timer); }, [query]);
  const localSearch = ["asset", "component", "heading"].includes(kind);
  const search = localSearch ? "" : debounced;
  const state = useSWRInfinite(
    (index, previous) => !session || !open || (index > 0 && !previous?.nextCursor) ? null :
      ["editor:choices", session.actorId, session.wikiId, kind, mediaKind, pageId, search, index === 0 ? "" : previous.nextCursor] as const,
    ([, , , resource, media, parent, q, cursor]) => loadResourceChoices(resource, q, cursor, media, parent),
  );
  const selected = useSWR(session && value && !valueLabel ? ["editor:choice-name", session.actorId, session.wikiId, kind, value] : null,
    () => selectedResourceName(kind, value), { revalidateIfStale: false });
  const all = state.data?.flatMap(page => page.items) ?? [];
  const items = localSearch ? all.filter(item => `${item.label} ${item.description ?? ""}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())) : all;
  const currentName = all.find(item => item.id === value)?.label || valueLabel || selected.data;
  const pick = async (item: ResourceChoice) => {
    setBusy(true);
    try {
      if (await onChange(item) !== false) {
        if (item.id && session) void mutate(["editor:choice-name", session.actorId, session.wikiId, kind, item.id], item.label, { revalidate: false });
        setOpen(false);
      }
    } catch { toast.error("选择失败，请重试。"); } finally { setBusy(false); }
  };
  return <span className="flex min-w-0 flex-wrap items-center gap-2" contentEditable={false} onKeyDown={event => event.stopPropagation()}>
    <Button type="button" variant="outline" className="max-w-full justify-start" aria-label={`选择${label}`} disabled={kind === "heading" && !pageId} onClick={() => { setQuery(""); setOpen(true); }}>
      <Search className="size-4 shrink-0" /><span className="truncate">{value ? currentName || (selected.error ? `当前${label}无法读取，重新选择` : `已选${label}`) : `搜索选择${label}`}</span>
    </Button>
    {clearable && value ? <Button type="button" variant="ghost" size="sm" aria-label={`清除${label}`} onClick={() => void pick({ id: "", label: "" })}>清除</Button> : null}
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="sm:max-w-xl" onKeyDown={event => event.stopPropagation()}>
        <DialogHeader><DialogTitle>选择{label}</DialogTitle><DialogDescription className="sr-only">按名称搜索并选择{label}。</DialogDescription></DialogHeader>
        <Command shouldFilter={false} label={`搜索${label}`}>
          <Command.Input aria-label={`搜索${label}`} placeholder="按名称搜索…" value={query} onValueChange={setQuery} maxLength={255} className="h-10 w-full rounded-lg border px-3 outline-none focus-visible:ring-2 focus-visible:ring-ring" />
          <Command.List className="mt-2 max-h-80 overflow-y-auto">
            {state.isLoading || (!localSearch && query.trim() !== debounced) ? <p className="p-3 text-sm text-muted-foreground">正在搜索…</p> : null}
            {state.error ? <div className="p-3 text-sm text-destructive">加载失败<Button type="button" variant="ghost" onClick={() => void state.mutate()}>重试</Button></div> : null}
            {!state.isLoading && !state.error && !items.length ? <p className="p-3 text-sm text-muted-foreground">没有找到{label}</p> : null}
            {items.map(item => <Command.Item key={item.id} value={item.id} disabled={busy || (!localSearch && query.trim() !== debounced)} onSelect={() => void pick(item)} className="cursor-pointer rounded-lg p-3 data-[selected=true]:bg-accent data-[disabled=true]:opacity-50">
              <span className="block break-words font-medium">{value === item.id ? "✓ " : ""}{item.label}</span>
              {item.description ? <span className="block break-words text-xs text-muted-foreground">{item.description}</span> : null}
            </Command.Item>)}
          </Command.List>
        </Command>
        {state.data?.at(-1)?.nextCursor ? <Button type="button" variant="outline" disabled={state.isValidating} onClick={() => void state.setSize(state.size + 1)}>加载更多</Button> : null}
      </DialogContent>
    </Dialog>
  </span>;
}
