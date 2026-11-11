"use client";

import useSWR from "swr";
import { Button } from "@/components/ui/button";
import { componentsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import type { Block } from "@/lib/ast/schema";

export function ComponentBlockPreview({ block }: { block: Extract<Block, { type: "component" }> }) {
  const { session } = useSession();
  const scope = [session?.actorId, session?.wikiId];
  const component = useSWR(session ? ["editor:component", ...scope, block.component_id] : null,
    () => componentsApi().getWikiComponent({ id: block.component_id }));
  const preview = useSWR(session ? ["editor:component-preview", ...scope, block.component_id, block.component_version, block.entity_id, block.display_config] : null,
    () => componentsApi().previewWikiComponentVersion({ id: block.component_id, version: block.component_version, previewWikiComponentRequest: { entityId: block.entity_id, props: block.display_config } }),
    { revalidateOnFocus: false, shouldRetryOnError: false });
  return <div className="my-3 min-w-0 rounded-xl border p-4">
    <p className="mb-3 text-sm font-medium">{component.data?.name ?? "知识组件"}<span className="ml-2 text-xs text-muted-foreground">v{block.component_version}</span></p>
    {preview.error ? <div className="text-sm text-muted-foreground">暂时无法预览，原配置已保留。<Button type="button" variant="ghost" size="sm" onClick={() => void preview.mutate()}>重试预览</Button></div>
      : preview.data ? <div className="wiki-rendered wiki-component-preview" onClickCapture={event => { if ((event.target as Element).closest("a,button")) { event.preventDefault(); event.stopPropagation(); } }}
          // Fixed server renderers escape user values, as in ComponentConfigure.
          dangerouslySetInnerHTML={{ __html: preview.data.html }} />
      : <p role="status" className="text-sm text-muted-foreground">正在加载组件预览…</p>}
  </div>;
}
