import { Bot, KeyRound, TerminalSquare } from "lucide-react";

import { CLIAccessWorkspace } from "@/components/settings/cli-access-workspace";

export default function CLIAccessPage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-xl bg-emerald-100 text-emerald-800">
          <TerminalSquare className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-emerald-800 uppercase">
          Account · Agent access
        </p>
        <h1 className="mt-2 text-4xl font-semibold">Agent CLI 授权</h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          为自动化 Agent 签发可撤销的 CLI 凭据。一次性授权码与长期 Token 分离，
          服务端不保存任何明文凭据。
        </p>
        <div className="mt-6 flex flex-wrap gap-2 text-[11px] font-medium text-muted-foreground">
          <span className="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5">
            <KeyRound className="size-3.5 text-emerald-700" aria-hidden />
            一次性授权码
          </span>
          <span className="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5">
            <Bot className="size-3.5 text-sky-700" aria-hidden />
            实时继承账号权限
          </span>
        </div>
      </header>
      <CLIAccessWorkspace />
    </div>
  );
}
