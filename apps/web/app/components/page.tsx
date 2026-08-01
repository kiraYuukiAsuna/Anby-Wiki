import { Boxes } from "lucide-react";

import { ComponentHub } from "@/components/components/component-hub";

export default function ComponentsPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-violet-100 text-violet-700">
          <Boxes className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-violet-700 uppercase">
          Component registry
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          组件中心
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground">
          用版本化 JSON Schema 与白名单渲染器构建信息框。发布后版本冻结，
          Claim 更新则通过依赖投影精准触发页面重渲染。
        </p>
      </header>
      <ComponentHub />
    </div>
  );
}
