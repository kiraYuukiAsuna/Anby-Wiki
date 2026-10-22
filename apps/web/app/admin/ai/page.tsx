import { Settings2 } from "lucide-react";

import { AIConfigWorkspace } from "@/components/admin/ai-config-workspace";

export default function AIConfigPage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-violet-100 text-violet-800">
          <Settings2 className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">管理员模型配置</h1>
      </header>
      <AIConfigWorkspace />
    </div>
  );
}
