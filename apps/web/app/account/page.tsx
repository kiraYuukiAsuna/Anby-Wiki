import { UserRound } from "lucide-react";

import { AccountCenter } from "@/components/account/account-center";

export default function AccountPage() {
  return (
    <div className="mx-auto w-full max-w-6xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-xl bg-sky-100 text-sky-800">
          <UserRound className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">个人中心</h1>
      </header>
      <AccountCenter />
    </div>
  );
}
