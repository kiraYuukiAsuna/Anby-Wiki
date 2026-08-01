import { Waypoints } from "lucide-react";

import { EntityLibrary } from "@/components/knowledge/entity-library";

export default function EntitiesPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-indigo-100 text-indigo-700">
          <Waypoints className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-indigo-700 uppercase">
          Knowledge graph
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          实体与知识
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          Entity 是跨页面复用的稳定身份。按标签、别名或 canonical key
          查找人物、组织、作品与概念，查看它们承载的事实和页面，并在重复身份出现时进入受审计的合并流程。
        </p>
      </header>
      <EntityLibrary />
    </div>
  );
}
