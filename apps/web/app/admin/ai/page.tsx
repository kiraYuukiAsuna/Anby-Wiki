import { Settings2 } from "lucide-react";

import { AIConfigWorkspace } from "@/components/admin/ai-config-workspace";

export default function AIConfigPage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-violet-100 text-violet-800">
          <Settings2 className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-violet-800 uppercase">
          Admin center · AI runtime
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          AI 模型配置
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          管理来源抽取使用的模型、超时和重试策略。供应商密钥会在服务端加密保存，
          不写入环境变量，也不会从管理接口再次返回。
        </p>
      </header>
      <AIConfigWorkspace />
    </div>
  );
}
