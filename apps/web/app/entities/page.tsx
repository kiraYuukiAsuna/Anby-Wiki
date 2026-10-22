import { Waypoints } from "lucide-react";

import { EntityLibrary } from "@/components/knowledge/entity-library";

export default function EntitiesPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-indigo-100 text-indigo-700">
          <Waypoints className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">实体与知识</h1>
      </header>
      <EntityLibrary />
    </div>
  );
}
