import { Network, Waypoints } from "lucide-react";

import { EntityGraphWorkspace } from "@/components/knowledge/entity-graph-workspace";

export default async function EntityGraphPage({
  searchParams,
}: {
  searchParams: Promise<{ entity_id?: string }>;
}) {
  const { entity_id: entityId } = await searchParams;

  return (
    <div className="mx-auto w-full max-w-[96rem] px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 border-b border-border/75 pb-8">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-indigo-100 text-indigo-700">
            <Network className="size-5" aria-hidden />
          </span>
          <span className="flex size-9 items-center justify-center rounded-xl bg-violet-100 text-violet-700">
            <Waypoints className="size-4" aria-hidden />
          </span>
        </div>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-indigo-700 uppercase">
          Entity graph explorer
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          知识关系图
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          沿 Entity-valued Claim 查看人物、作品、组织与概念之间的可验证关系。
          图查询只读取可重建边投影，并受深度、节点与边数硬上限保护。
        </p>
      </header>

      <EntityGraphWorkspace initialEntityId={entityId ?? ""} />
    </div>
  );
}
