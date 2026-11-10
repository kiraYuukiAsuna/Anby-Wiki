"use client";

import Link from "next/link";
import { LoaderCircle, Mail, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWR, { useSWRConfig } from "swr";
import { z } from "zod";
import { type SystemMailConfig, type SystemAccount, SystemMailConfigTlsModeEnum } from "../../../../contracts/generated/typescript";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { adminApi } from "@/lib/api";
import { AUTH_SESSION_KEY, useSession } from "@/lib/auth";
import { SECURITY_ACTION_LABELS, securityErrorMessage } from "@/lib/account-security";
import { workspaceAccessNotice } from "@/lib/workspace-access";

const mailSchema = z.object({
  enabled: z.boolean(),
  host: z.string().trim().min(1, "请输入 SMTP 主机").max(253),
  port: z.number().int().min(1).max(65535),
  tlsMode: z.enum(["tls", "starttls", "loopback"]),
  username: z.string().trim().max(254),
  fromEmail: z.string().trim().email("请输入有效发件邮箱"),
  fromName: z.string().trim().max(128),
  publicUrl: z.url("请输入站点完整地址"),
});

function MailSettings({ configuration, mfaEnabled, onSaved }: {
  configuration: SystemMailConfig;
  mfaEnabled: boolean;
  onSaved: (value: SystemMailConfig) => Promise<void>;
}) {
  const [form, setForm] = useState(configuration);
  const [smtpPassword, setSMTPPassword] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [testEmail, setTestEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const setField = <K extends keyof SystemMailConfig>(key: K, value: SystemMailConfig[K]) =>
    setForm((current) => ({ ...current, [key]: value }));

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    const parsed = mailSchema.safeParse(form);
    if (!parsed.success) { toast.error(parsed.error.issues[0]?.message ?? "请检查邮件配置"); return; }
    if (!password || (mfaEnabled && !code.trim())) { toast.error("请填写当前密码和需要的二次验证码"); return; }
    setBusy(true);
    try {
      const value = await adminApi().updateSystemMail({
        updateSystemMailRequest: {
          _configuration: { ...parsed.data, passwordConfigured: configuration.passwordConfigured },
          smtpPassword, password, code: code.trim(),
        },
      });
      setSMTPPassword("");
      setPassword("");
      setCode("");
      await onSaved(value);
      toast.success("邮件配置已保存");
    } catch (error) { toast.error(await securityErrorMessage(error)); }
    finally { setBusy(false); }
  };

  const test = async (event: React.FormEvent) => {
    event.preventDefault();
    if (busy) return;
    const parsed = z.string().trim().email("请输入有效收件邮箱").max(254).safeParse(testEmail);
    if (!parsed.success) { toast.error(parsed.error.issues[0]?.message); return; }
    setBusy(true);
    try {
      await adminApi().testSystemMail({ testSystemMailRequest: { email: parsed.data } });
      toast.success("测试邮件已发送，请检查收件箱");
    } catch (error) { toast.error(await securityErrorMessage(error)); }
    finally { setBusy(false); }
  };

  return <section className="mt-7 min-w-0 rounded-2xl border bg-card p-5 sm:p-7">
    <h2 className="flex items-center gap-2 text-xl font-semibold"><Mail className="size-5" />邮件发送</h2>
    <p className="mt-2 text-sm text-muted-foreground">邮箱验证和密码恢复使用此配置。SMTP 密码加密保存，留空可保留已有密码。</p>
    <form onSubmit={submit} className="mt-6 grid min-w-0 gap-5 sm:grid-cols-2">
      <label className="flex items-center gap-2 text-sm sm:col-span-2"><input type="checkbox" checked={form.enabled} onChange={(event) => setField("enabled", event.target.checked)} />启用邮件发送</label>
      <div><Label htmlFor="smtp-host">SMTP 主机</Label><Input id="smtp-host" className="mt-2" value={form.host} onChange={(event) => setField("host", event.target.value)} placeholder="smtp.example.com" required /></div>
      <div><Label htmlFor="smtp-port">端口</Label><Input id="smtp-port" className="mt-2" type="number" min={1} max={65535} value={form.port} onChange={(event) => setField("port", Number(event.target.value))} required /></div>
      <div><Label htmlFor="smtp-tls">传输加密</Label><select id="smtp-tls" className="mt-2 h-9 w-full rounded-md border bg-background px-3 text-sm" value={form.tlsMode} onChange={(event) => setField("tlsMode", event.target.value as SystemMailConfigTlsModeEnum)}>
        <option value="starttls">STARTTLS（通常使用 587）</option><option value="tls">TLS（通常使用 465）</option><option value="loopback">仅本机测试</option>
      </select></div>
      <div><Label htmlFor="smtp-user">SMTP 用户名</Label><Input id="smtp-user" className="mt-2" autoComplete="off" value={form.username} onChange={(event) => setField("username", event.target.value)} /></div>
      <div><Label htmlFor="smtp-password">SMTP 密码或授权码</Label><Input id="smtp-password" className="mt-2" type="password" autoComplete="new-password" value={smtpPassword} onChange={(event) => setSMTPPassword(event.target.value)} placeholder={configuration.passwordConfigured ? "已配置，留空保留" : "尚未配置"} /></div>
      <div><Label htmlFor="smtp-from">发件邮箱</Label><Input id="smtp-from" className="mt-2" type="email" value={form.fromEmail} onChange={(event) => setField("fromEmail", event.target.value)} required /></div>
      <div><Label htmlFor="smtp-name">发件人名称</Label><Input id="smtp-name" className="mt-2" value={form.fromName} onChange={(event) => setField("fromName", event.target.value)} /></div>
      <div><Label htmlFor="smtp-site">站点地址</Label><Input id="smtp-site" className="mt-2" type="url" value={form.publicUrl} onChange={(event) => setField("publicUrl", event.target.value)} placeholder="https://wiki.example.com" required /><p className="mt-2 text-xs text-muted-foreground">用于邮件中的恢复链接，填写本站正式 HTTPS 地址。</p></div>
      <div className="border-t pt-5 sm:col-span-2"><h3 className="font-medium">确认身份</h3><p className="mt-1 text-sm text-muted-foreground">保存配置需要重新验证。每个验证码只能使用一次。</p></div>
      <div><Label htmlFor="system-password">当前账号密码</Label><Input id="system-password" className="mt-2" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></div>
      {mfaEnabled && <div><Label htmlFor="system-code">验证码或恢复码</Label><Input id="system-code" className="mt-2" autoComplete="one-time-code" value={code} onChange={(event) => setCode(event.target.value)} /></div>}
      <div className="flex flex-wrap gap-3 sm:col-span-2">
        <Button type="submit" disabled={busy}>{busy ? "处理中…" : "保存邮件配置"}</Button>
      </div>
    </form>
    <form onSubmit={test} className="mt-6 space-y-4 border-t pt-5">
      <div><Label htmlFor="smtp-test-email">测试收件邮箱</Label><Input id="smtp-test-email" className="mt-2" type="email" autoComplete="email" value={testEmail} onChange={(event) => setTestEmail(event.target.value)} placeholder="填写接收测试邮件的邮箱" maxLength={254} required /></div>
      <p className="text-xs text-muted-foreground">测试使用已保存的配置，发送到填写的收件邮箱，无需再次输入密码。</p>
      <Button type="submit" variant="outline" disabled={busy || !configuration.updatedAt}>{busy ? "处理中…" : "发送测试邮件"}</Button>
    </form>
  </section>;
}

function SystemPagination({ label, page, pageSize, total, disabled, onChange }: {
  label: string;
  page: number;
  pageSize: number;
  total: number;
  disabled: boolean;
  onChange: (page: number) => void;
}) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  return <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
    <p role="status" className="text-xs text-muted-foreground">共 {total} 条 · 第 {page} / {pages} 页 · 每页 {pageSize} 条</p>
    <nav aria-label={`${label}分页`} className="flex gap-2">
      <Button type="button" size="sm" variant="outline" disabled={disabled || page <= 1} onClick={() => onChange(page - 1)}>上一页</Button>
      <Button type="button" size="sm" variant="outline" disabled={disabled || page >= pages} onClick={() => onChange(page + 1)}>下一页</Button>
    </nav>
  </div>;
}

export function SystemConsole() {
  const { session } = useSession();
  const { mutate } = useSWRConfig();
  const permitted = !!session?.systemAdmin && session.method === "session";
  const accountVisible = !workspaceAccessNotice("/account", session);
  const mail = useSWR(permitted ? ["system:mail", session.actorId] : null, () => adminApi().getSystemMail(), { revalidateOnFocus: false });
  const [target, setTarget] = useState<SystemAccount | null>(null);
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [search, setSearch] = useState("");
  const [accountsPage, setAccountsPage] = useState(1);
  const [eventsPage, setEventsPage] = useState(1);
  const accounts = useSWR(
    permitted ? ["system:accounts", session.actorId, search, accountsPage, eventsPage] : null,
    () => adminApi().getSystemAccounts({ search, accountsPage, eventsPage }),
    { revalidateOnFocus: false, keepPreviousData: true },
  );
  const mfaEnabled = accounts.data?.mfaEnabled ?? false;

  const changeAdmin = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!target || busy || (target.systemAdmin && target.systemAdminProtected)) return;
    if (!password || (mfaEnabled && !code.trim())) { toast.error("请填写身份验证信息"); return; }
    setBusy(true);
    try {
      const revokingSelf = target.systemAdmin && target.actorId === session?.actorId;
      await adminApi().setSystemAdministrator({ setSystemAdministratorRequest: { actorId: target.actorId, grant: !target.systemAdmin, password, code: code.trim() } });
      setTarget(null);
      setPassword("");
      setCode("");
      await mutate(AUTH_SESSION_KEY);
      if (!revokingSelf) await accounts.mutate();
      toast.success("系统管理员权限已更新");
    } catch (error) { toast.error(await securityErrorMessage(error)); }
    finally { setBusy(false); }
  };

  if (!permitted) return <div className="mt-7 rounded-2xl border p-6"><ShieldCheck className="size-6" /><h2 className="mt-3 font-semibold">需要系统管理员权限</h2><p className="mt-2 text-sm text-muted-foreground">Wiki 管理员负责本 Wiki 的成员和内容。管理全站设置还需要由运维人员授予系统管理员权限。</p>{accountVisible ? <Button className="mt-4" variant="outline" asChild><Link href="/account">返回个人中心</Link></Button> : null}</div>;
  if (mail.isLoading || (accounts.isLoading && !accounts.data)) return <p className="mt-7 flex items-center gap-2 text-sm"><LoaderCircle className="size-4 animate-spin" />正在读取系统配置…</p>;
  if (mail.error || accounts.error || !mail.data || !accounts.data) return <div className="mt-7 rounded-2xl border p-6"><p>系统配置暂不可用。</p><Button className="mt-3" onClick={() => { void mail.mutate(); void accounts.mutate(); }}>重试</Button></div>;
  const items = accounts.data.accounts;

  return <>
    <MailSettings key={mail.data.updatedAt?.toISOString() ?? "initial"} configuration={mail.data} mfaEnabled={mfaEnabled} onSaved={async (value) => { await mail.mutate(value, { revalidate: false }); await accounts.mutate(); }} />
    <section aria-label="账号与系统权限" aria-busy={accounts.isValidating} className="mt-6 min-w-0 rounded-2xl border bg-card p-5 sm:p-7">
      <h2 className="text-xl font-semibold">账号与系统权限</h2>
      <p className="mt-2 text-sm text-muted-foreground">系统管理员可以管理全站设置和查看安全记录，但访问私人 Wiki 的内容仍需相应的成员权限。</p>
      <Input className="mt-5 max-w-md" aria-label="搜索账号" placeholder="按用户名或邮箱搜索" maxLength={254} value={search} onChange={(event) => { setSearch(event.target.value); setAccountsPage(1); }} />
      {accounts.isValidating && <p className="mt-3 text-xs text-muted-foreground">正在更新列表…</p>}
      <ul className="mt-4 divide-y">{items.map((account) => <li key={account.actorId} className="flex min-w-0 flex-wrap items-center justify-between gap-3 py-4">
        <div className="min-w-0"><p className="break-all font-medium">@{account.username}{account.systemAdmin && <span className="ml-2 text-xs text-primary">{account.systemAdminProtected ? "初始管理员" : "系统管理员"}</span>}</p><p className="mt-1 break-all text-sm text-muted-foreground">{account.email} · {account.emailVerified ? "邮箱已验证" : "邮箱未验证"} · {account.mfaEnabled ? "已启用二次验证" : "未启用二次验证"}</p></div>
        <Button size="sm" variant="outline" disabled={busy || accounts.isValidating || account.status !== "active" || (account.systemAdmin && account.systemAdminProtected)} onClick={() => { setPassword(""); setCode(""); setTarget(account); }}>{account.systemAdmin ? account.systemAdminProtected ? "不可撤销" : "撤销系统权限" : "授予系统权限"}</Button>
      </li>)}</ul>
      {items.length === 0 && <p className="mt-4 text-sm text-muted-foreground">没有匹配的账号。</p>}
      <SystemPagination label="账号" page={accounts.data.accountsPage} pageSize={accounts.data.pageSize} total={accounts.data.accountsTotal} disabled={busy || accounts.isValidating} onChange={setAccountsPage} />
    </section>
    <section aria-label="安全审计" aria-busy={accounts.isValidating} className="mt-6 min-w-0 rounded-2xl border bg-card p-5 sm:p-7"><h2 className="text-xl font-semibold">安全审计</h2><ul className="mt-4 divide-y">{accounts.data.events.map((event) => <li key={event.id} className="min-w-0 py-3 text-sm"><div className="flex flex-wrap justify-between gap-2"><span>{SECURITY_ACTION_LABELS[event.action] ?? event.action}</span><time className="text-muted-foreground">{event.createdAt.toLocaleString()}</time></div><p className="mt-1 break-all text-xs text-muted-foreground">操作账号：{event.actorUsername ?? event.actorId ?? "已删除"}{event.targetActorId !== event.actorId && ` · 目标：${event.targetUsername ?? event.targetActorId ?? "已删除"}`}</p></li>)}</ul>
      {accounts.data.events.length === 0 && <p className="mt-4 text-sm text-muted-foreground">暂无安全审计记录。</p>}
      <SystemPagination label="安全审计" page={accounts.data.eventsPage} pageSize={accounts.data.pageSize} total={accounts.data.eventsTotal} disabled={busy || accounts.isValidating} onChange={setEventsPage} />
    </section>
    <Dialog open={!!target} onOpenChange={(open) => { if (!open && !busy) setTarget(null); }}><DialogContent showCloseButton={!busy}><DialogHeader><DialogTitle>{target?.systemAdmin ? "撤销" : "授予"}系统管理员权限</DialogTitle><DialogDescription>目标账号：@{target?.username}。初始 admin 权限不可撤销，系统必须保留至少一名有效管理员。</DialogDescription></DialogHeader><form className="space-y-4" onSubmit={changeAdmin}><div><Label htmlFor="grant-password">当前密码</Label><Input id="grant-password" className="mt-2" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required /></div>{mfaEnabled && <div><Label htmlFor="grant-code">验证码或恢复码</Label><Input id="grant-code" className="mt-2" autoComplete="one-time-code" value={code} onChange={(event) => setCode(event.target.value)} required /></div>}<Button className="w-full" disabled={busy} type="submit">{busy ? "处理中…" : "确认权限变更"}</Button></form></DialogContent></Dialog>
  </>;
}
