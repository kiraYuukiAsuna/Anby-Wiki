"use client";

import { useState } from "react";
import Link from "next/link";
import { ArrowUpRight, LoaderCircle, Network } from "lucide-react";
import useSWR from "swr";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { knowledgeApi } from "@/lib/api";
import { useSession } from "@/lib/auth";

function GraphEntityLink({ entityId, actorId, wikiId, onChoose }: { entityId: string; actorId: string; wikiId: string; onChoose: () => void }) {
  const entity = useSWR(["page:graph-entity", actorId, wikiId, entityId], () => knowledgeApi().getEntity({ id: entityId }));
  if (entity.error) return <div className="flex items-center justify-between gap-2 rounded-lg border p-3 text-sm text-muted-foreground">实体暂时无法读取<Button variant="ghost" size="sm" onClick={() => void entity.mutate()}>重试</Button></div>;
  if (!entity.data) return <p role="status" className="p-3 text-sm text-muted-foreground">正在加载关联实体…</p>;
  const name = entity.data.labels.find(label => label.isPrimary)?.label || entity.data.canonicalKey;
  return <Link href={`/explore/graph?entity_id=${encodeURIComponent(entityId)}`} onClick={onChoose} className="flex items-center justify-between gap-3 rounded-lg border p-3 hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
    <span className="min-w-0 break-words"><span className="block text-sm font-medium">{name}</span><span className="text-xs text-muted-foreground">{entity.data.entityType.name}</span></span>
    <ArrowUpRight className="size-4 shrink-0" aria-hidden />
  </Link>;
}

export function PageGraphButton({ pageId, pageTitle, canEdit }: { pageId: string; pageTitle: string; canEdit: boolean }) {
  const { session, isLoading } = useSession();
  const [open, setOpen] = useState(false);
  const bindings = useSWR(session ? ["page:graph-bindings", session.actorId, session.wikiId, pageId] : null,
    () => knowledgeApi().listPageEntityBindings({ id: pageId }));
  const items = bindings.data?.items ?? [];
  const entityIds = [...new Set(items.map(item => item.entityId))];
  const primary = items.find(item => item.role === "primary")?.entityId;
  const root = primary ?? (entityIds.length === 1 ? entityIds[0] : undefined);
  const loading = isLoading || bindings.isLoading;

  if (root && !bindings.error) return <Button variant="ghost" size="sm" asChild className="gap-1">
    <Link href={`/explore/graph?entity_id=${encodeURIComponent(root)}`}><Network className="size-3.5" aria-hidden />知识图谱</Link>
  </Button>;

  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger asChild><Button variant="ghost" size="sm" className="gap-1" disabled={loading || !session}>
      {loading ? <LoaderCircle className="size-3.5 animate-spin" aria-hidden /> : <Network className="size-3.5" aria-hidden />}知识图谱
    </Button></DialogTrigger>
    <DialogContent className="max-w-md">
      <DialogHeader><DialogTitle>知识图谱</DialogTitle><DialogDescription>查看“{pageTitle}”关联实体的关系。</DialogDescription></DialogHeader>
      {bindings.error ? <div className="flex items-center justify-between gap-2 text-sm text-muted-foreground">页面关联实体加载失败<Button variant="outline" size="sm" onClick={() => void bindings.mutate()}>重试</Button></div>
        : entityIds.length && session ? <div className="max-h-72 space-y-2 overflow-y-auto">{entityIds.map(entityId => <GraphEntityLink key={entityId} entityId={entityId} actorId={session.actorId} wikiId={session.wikiId} onChoose={() => setOpen(false)} />)}</div>
        : <div className="space-y-3"><p className="text-sm text-muted-foreground">这个页面还没有关联实体。</p>{canEdit && <Button variant="outline" asChild><Link href={`/pages/${pageId}/tools#page-entities`}>关联实体</Link></Button>}</div>}
      <Link href="/explore/graph" className="text-sm text-muted-foreground underline-offset-4 hover:underline">浏览知识图谱</Link>
    </DialogContent>
  </Dialog>;
}
