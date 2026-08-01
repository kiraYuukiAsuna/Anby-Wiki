import { Archive } from "lucide-react";

import { RevisionStorageWorkspace } from "@/components/governance/revision-storage-workspace";

export default function RevisionStoragePage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-sky-100 text-sky-800">
          <Archive className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-sky-800 uppercase">
          Revision storage
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          历史版本存储
        </h1>
        <p className="mt-4 max-w-3xl text-sm leading-7 text-muted-foreground">
          管理不可变 Revision 快照的冷热分层。近期与当前内容留在数据库，
          到期历史迁移到内容寻址对象存储；阅读、版本对比和回滚保持同一体验。
        </p>
      </header>
      <RevisionStorageWorkspace />
    </div>
  );
}
