import { SystemConsole } from "@/components/admin/system-console";

export default function SystemAdminPage() {
  return <div className="mx-auto w-full min-w-0 max-w-6xl px-5 py-10 lg:px-8">
    <h1 className="text-3xl font-semibold">系统管理</h1>
    <p className="mt-3 text-sm text-muted-foreground">设置邮件服务，管理系统管理员，查看账号安全记录。</p>
    <SystemConsole />
  </div>;
}
