"use client";

import {
  Bot,
  Check,
  CircleGauge,
  LoaderCircle,
  LockKeyhole,
  Plus,
  Power,
  ShieldAlert,
  ShieldCheck,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type AITrustProfile,
  type MachineActor,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authApi, governanceApi, makeRequestId } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { workspaceAccessNotice } from "@/lib/workspace-access";

const profileSchema = z.object({
  trustLevel: z.enum(["untrusted", "assisted", "trusted", "full_access"]),
  requiredSamplePercent: z.coerce.number().int().min(0).max(100),
});
const actorSchema = z.object({
  displayName: z.string().trim().min(1, "请输入机器人名称").max(120),
  actorType: z.enum(["ai", "import"]),
});
const NIL_UUID = "00000000-0000-0000-0000-000000000000";

const LEVELS = [
  { value: "untrusted", label: "未受信任" },
  { value: "assisted", label: "辅助" },
  { value: "trusted", label: "受信任" },
  { value: "full_access", label: "导入自动生效" },
] as const;

function TrustProfileCard({
  profile,
  actor,
  onSaved,
}: {
  profile: AITrustProfile;
  actor: MachineActor | undefined;
  onSaved: () => Promise<unknown>;
}) {
  const [trustLevel, setTrustLevel] = useState(profile.trustLevel);
  const [samplePercent, setSamplePercent] = useState(
    profile.requiredSamplePercent,
  );
  const [saving, setSaving] = useState(false);
  const [changingStatus, setChangingStatus] = useState(false);
  const disabled = profile.actorStatus !== "active";

  const changeStatus = async () => {
    if (!actor) return;
    setChangingStatus(true);
    try {
      await authApi().setMachineActorStatus({
        id: actor.actorId,
        setMachineActorStatusRequest: { status: disabled ? "active" : "disabled" },
      });
      await onSaved();
      toast.success(disabled ? "机器人已启用，请重新生成 CLI 授权码。" : "机器人已停用，所有 CLI 授权已撤销。");
    } catch {
      toast.error("机器人状态更新失败");
    } finally {
      setChangingStatus(false);
    }
  };

  const effectiveSample = trustLevel === "untrusted"
    ? 100
    : trustLevel === "full_access"
      ? 0
      : samplePercent;
  const dirty =
    trustLevel !== profile.trustLevel ||
    effectiveSample !== profile.requiredSamplePercent;

  const save = async () => {
    const parsed = profileSchema.safeParse({
      trustLevel,
      requiredSamplePercent: effectiveSample,
    });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查审核设置");
      return;
    }
    setSaving(true);
    try {
      await governanceApi().updateAITrustProfile({
        actorId: profile.actorId,
        updateAITrustProfileRequest: parsed.data,
      });
      await onSaved();
      toast.success("审核设置已保存");
    } catch (error) {
      if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("只有站点管理员可以修改 AI 审核设置");
      } else {
        toast.error("AI 审核设置保存失败");
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <article className="min-w-0 rounded-3xl border bg-card p-5 shadow-[0_16px_42px_-36px_rgb(15_23_42/0.5)]">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 max-w-full items-start gap-3">
          <span className="flex size-11 shrink-0 items-center justify-center rounded-2xl bg-fuchsia-100 text-fuchsia-800">
            <Bot className="size-5" aria-hidden />
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="max-w-full truncate font-semibold">
                {profile.actorDisplayName}
              </h2>
              {!profile.configured ? (
                <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium text-amber-800">
                  默认设置
                </span>
              ) : null}
              {disabled ? (
                <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">已停用</span>
              ) : null}
            </div>
          </div>
        </div>
      </div>

      <div className="mt-5 space-y-2">
        <Label htmlFor={`trust-${profile.actorId}`}>信任等级</Label>
        <select
          id={`trust-${profile.actorId}`}
          value={trustLevel}
          disabled={disabled}
          onChange={(event) => {
            const value = event.target.value as AITrustProfile["trustLevel"];
            setTrustLevel(value);
            if (value === "untrusted") setSamplePercent(100);
            if (value === "full_access") setSamplePercent(0);
          }}
          className="h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm disabled:opacity-50"
        >
          {LEVELS.map((level) => (
            <option key={level.value} value={level.value}>{level.label}</option>
          ))}
        </select>
      </div>

      {trustLevel === "assisted" || trustLevel === "trusted" ? (
        <div className="mt-4 rounded-2xl border bg-muted/25 p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <Label htmlFor={`sample-${profile.actorId}`}>
                人工审核 · 重点复核比例
              </Label>
            </div>
            <div className="relative w-24">
              <Input
                id={`sample-${profile.actorId}`}
                type="number"
                min={0}
                max={100}
                value={effectiveSample}
                disabled={disabled}
                onChange={(event) =>
                  setSamplePercent(Number(event.target.value))
                }
                className="pr-7 text-right font-mono"
                aria-label="低风险人工抽样百分比"
              />
              <span className="pointer-events-none absolute top-1.5 right-2 text-xs text-muted-foreground">
                %
              </span>
            </div>
          </div>
          <input
            type="range"
            min={0}
            max={100}
            step={1}
            value={effectiveSample}
            disabled={disabled}
            onChange={(event) => setSamplePercent(Number(event.target.value))}
            className="mt-4 h-1.5 w-full accent-fuchsia-700 disabled:opacity-45"
            aria-label="调整低风险人工抽样比例"
          />
        </div>
      ) : null}

      {trustLevel === "full_access" && profile.trustLevel !== "full_access" ? (
        <p className="mt-3 text-xs text-rose-800 dark:text-rose-300">
          保存后，导入修改将跳过人工审核并自动生效。
        </p>
      ) : null}

      <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
        {actor ? (
          <Button type="button" size="sm" variant="outline" disabled={changingStatus || actor.ownerActorId === NIL_UUID} onClick={() => void changeStatus()}>
            {changingStatus ? <LoaderCircle className="animate-spin" aria-hidden /> : <Power aria-hidden />}
            {actor.ownerActorId === NIL_UUID ? "已永久停用" : disabled ? "启用机器人" : "停用并撤销授权"}
          </Button>
        ) : null}
        <Button
          type="button"
          className="ml-auto"
          disabled={disabled || !dirty || saving}
          onClick={() => void save()}
        >
          {saving ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <Check aria-hidden />
          )}
          {saving ? "保存中…" : "保存设置"}
        </Button>
      </div>
    </article>
  );
}

export function AITrustWorkspace() {
  const session = useSession();
  const [actorName, setActorName] = useState("");
  const [actorType, setActorType] = useState<"ai" | "import">("ai");
  const [creationKey, setCreationKey] = useState(makeRequestId);
  const [creatingActor, setCreatingActor] = useState(false);
  const actors = useSWR(
    session.isAuthenticated ? ["governance:machine-actors", session.session?.actorId, session.session?.wikiId] : null,
    () => authApi().listMachineActors(),
    { revalidateOnFocus: true, shouldRetryOnError: (error) => !isUnauthorized(error) },
  );
  const profiles = useSWR(
    session.isAuthenticated ? ["governance:ai-trust-profiles", session.session?.actorId, session.session?.wikiId] : null,
    () => governanceApi().listAITrustProfiles(),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: (error) => !isUnauthorized(error),
    },
  );
  const items = useMemo(
    () => profiles.data?.items ?? [],
    [profiles.data?.items],
  );
  const configured = items.filter((item) => item.configured).length;
  const trusted = items.filter(
    (item) => item.trustLevel === "trusted" || item.trustLevel === "full_access",
  ).length;
  const cliVisible = !workspaceAccessNotice("/settings/cli", session.session);
  const refresh = async () => {
    await Promise.all([profiles.mutate(), actors.mutate()]);
  };
  const createActor = async () => {
    const parsed = actorSchema.safeParse({ displayName: actorName, actorType });
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查机器人设置");
      return;
    }
    setCreatingActor(true);
    try {
      await authApi().createMachineActor({ idempotencyKey: creationKey, createMachineActorRequest: parsed.data });
      setActorName("");
      setCreationKey(makeRequestId());
      await refresh();
      toast.success("机器人已创建");
    } catch (error) {
      toast.error(error instanceof ResponseError && error.response.status === 409
        ? "这个名称已被其他配置使用，请换一个名称。"
        : "创建失败，请确认你有管理员权限后重试。");
    } finally {
      setCreatingActor(false);
    }
  };

  if (session.isLoading) {
    return (
      <div className="flex min-h-72 items-center justify-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
        正在检查权限…
      </div>
    );
  }
  if (!session.isAuthenticated) {
    return (
      <div className="rounded-3xl border border-dashed bg-muted/25 p-10 text-center">
        <LockKeyhole className="mx-auto size-8 text-muted-foreground" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold">登录后管理 AI 审核设置</h2>
        <Button asChild className="mt-5">
          <Link href={LOGIN_PATH}>前往登录</Link>
        </Button>
      </div>
    );
  }
  if (
    profiles.error instanceof ResponseError &&
    profiles.error.response.status === 403
  ) {
    return (
      <div className="rounded-3xl border border-amber-200 bg-amber-50/65 p-8">
        <ShieldAlert className="size-7 text-amber-700" aria-hidden />
        <h2 className="mt-4 text-lg font-semibold text-amber-950">
          需要站点管理员权限
        </h2>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <section className="rounded-3xl border bg-card p-5">
        <h2 className="text-lg font-semibold">创建机器人账号</h2>
        <div className="mt-5 grid gap-4 sm:grid-cols-[minmax(0,1fr)_8rem_auto] sm:items-end">
          <div className="min-w-0">
            <Label htmlFor="machine-actor-name">机器人名称</Label>
            <Input id="machine-actor-name" className="mt-2" value={actorName} disabled={creatingActor} maxLength={120} placeholder="例如 research-agent"
              onChange={(event) => { setActorName(event.target.value); setCreationKey(makeRequestId()); }} />
          </div>
          <div>
            <Label htmlFor="machine-actor-type">类型</Label>
            <select id="machine-actor-type" disabled={creatingActor} className="mt-2 h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm" value={actorType}
              onChange={(event) => { setActorType(event.target.value as "ai" | "import"); setCreationKey(makeRequestId()); }}>
              <option value="ai">AI 助手</option>
              <option value="import">导入助手</option>
            </select>
          </div>
          <Button type="button" disabled={creatingActor} onClick={() => void createActor()}>
            {creatingActor ? <LoaderCircle className="animate-spin" aria-hidden /> : <Plus aria-hidden />}
            创建机器人
          </Button>
        </div>
        <p className="mt-4 text-xs leading-5 text-muted-foreground">
          {cliVisible ? (
            <Link href="/settings/cli" className="underline underline-offset-4">前往 Agent CLI 授权</Link>
          ) : (
            "切换到自己的 Wiki 后，可为机器人授权。"
          )}
        </p>
        {actors.error ? <p className="mt-3 text-xs text-destructive">机器人账号暂时无法读取，请刷新重试。</p> : null}
      </section>
      <section className="grid gap-3 sm:grid-cols-3">
        {[
          { label: "机器人账号", value: items.length, icon: Bot },
          { label: "已设置", value: configured, icon: ShieldCheck },
          { label: "受信任的机器人", value: trusted, icon: CircleGauge },
        ].map(({ label, value, icon: Icon }) => (
          <div key={label} className="rounded-2xl border bg-card p-5">
            <Icon className="size-4 text-fuchsia-700" aria-hidden />
            <p className="mt-4 text-3xl font-semibold tracking-tight">
              {profiles.isLoading ? "—" : value}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">{label}</p>
          </div>
        ))}
      </section>

      <section>
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h2 className="text-xl font-semibold tracking-tight">
              机器人审核设置
            </h2>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={profiles.isValidating}
            onClick={() => void refresh()}
          >
            {profiles.isValidating ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : null}
            刷新
          </Button>
        </div>

        {profiles.isLoading ? (
          <div className="mt-5 grid gap-4 xl:grid-cols-2">
            {[0, 1].map((item) => (
              <div
                key={item}
                className="h-96 animate-pulse rounded-3xl border bg-muted/30"
              />
            ))}
          </div>
        ) : null}
        {profiles.error &&
        !(
          profiles.error instanceof ResponseError &&
          profiles.error.response.status === 403
        ) ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm">
            <p className="font-medium text-destructive">审核设置暂时无法读取</p>
          </div>
        ) : null}
        {!profiles.isLoading && !profiles.error && items.length === 0 ? (
          <div className="mt-5 rounded-3xl border border-dashed px-6 py-14 text-center">
            <Bot className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">还没有机器人账号</h3>
            {cliVisible ? (
              <Button asChild variant="outline" className="mt-5">
                <Link href="/settings/cli">打开 Agent CLI</Link>
              </Button>
            ) : null}
          </div>
        ) : null}
        {items.length > 0 ? (
          <div className="mt-5 grid gap-4 xl:grid-cols-2">
            {items.map((profile) => (
              <TrustProfileCard
                key={`${profile.actorId}:${profile.actorStatus}:${profile.trustLevel}:${profile.requiredSamplePercent}`}
                profile={profile}
                actor={actors.data?.items.find((actor) => actor.actorId === profile.actorId)}
                onSaved={refresh}
              />
            ))}
          </div>
        ) : null}
      </section>
    </div>
  );
}
