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
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-2xl bg-indigo-100 text-indigo-700">
            <Network className="size-5" aria-hidden />
          </span>
          <span className="flex size-9 items-center justify-center rounded-xl bg-violet-100 text-violet-700">
            <Waypoints className="size-4" aria-hidden />
          </span>
        </div>
        <h1 className="text-3xl font-semibold">知识关系图</h1>
      </header>

      <EntityGraphWorkspace initialEntityId={entityId ?? ""} />
    </div>
  );
}
