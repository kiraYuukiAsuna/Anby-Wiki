import { SystemConsole } from "@/components/admin/system-console";

export default function SystemAdminPage() {
  return <div className="mx-auto w-full min-w-0 max-w-6xl px-5 py-10 lg:px-8">
    <h1 className="text-3xl font-semibold">系统管理台</h1>
    <p className="mt-3 text-sm text-muted-foreground">配置全站邮件发送，管理系统管理员权限和查看账号安全活动。</p>
    <SystemConsole />
  </div>;
}
