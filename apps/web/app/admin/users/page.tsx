import { UsersRound } from "lucide-react";

import { UserManagementWorkspace } from "@/components/admin/user-management-workspace";

export default function AdminUsersPage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-10 items-center justify-center rounded-lg bg-sky-100 text-sky-800">
          <UsersRound className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">用户与角色</h1>
      </header>
      <UserManagementWorkspace />
    </div>
  );
}
