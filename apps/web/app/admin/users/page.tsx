import { UsersRound } from "lucide-react";

import { UserManagementWorkspace } from "@/components/admin/user-management-workspace";

export default function AdminUsersPage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 border-b border-border/75 pb-7">
        <span className="flex size-10 items-center justify-center rounded-lg bg-sky-100 text-sky-800">
          <UsersRound className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-sky-800 uppercase">
          Admin center · Users
        </p>
        <h1 className="mt-2 text-3xl font-semibold">用户与角色</h1>
        <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground">
          管理本地账号在当前 Wiki 内的 RBAC 角色。角色变更不会写入 session，
          下一次授权检查即时生效。
        </p>
      </header>
      <UserManagementWorkspace />
    </div>
  );
}
