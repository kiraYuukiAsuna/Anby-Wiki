"use client";

import {
  Bot,
  Check,
  CircleGauge,
  Clock3,
  LoaderCircle,
  LockKeyhole,
  Plus,
  Power,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
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
import { cn } from "@/lib/utils";

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
  {
    value: "untrusted",
    label: "未受信任",
    detail: "所有变更必须人工审核",
    icon: ShieldAlert,
  },
  {
    value: "assisted",
    label: "辅助",
    detail: "辅助生成修改建议，抽样标记重点复核",
    icon: ShieldCheck,
  },
  {
    value: "trusted",
    label: "受信任",
    detail: "已受信任，仍需人工最终审核",
    icon: Sparkles,
  },
  {
    value: "full_access",
    label: "全自动处理",
    detail: "自动批准导入修改并使其生效",
    icon: CircleGauge,
  },
] as const;

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

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
      toast.error(parsed.error.issues[0]?.message ?? "请检查信任策略");
      return;
    }
    setSaving(true);
    try {
      await governanceApi().updateAITrustProfile({
        actorId: profile.actorId,
        updateAITrustProfileRequest: parsed.data,
      });
      await onSaved();
      toast.success("AI 审核设置已更新", {
        description: trustLevel === "full_access"
          ? `${profile.actorDisplayName} · 全自动导入`
          : `${profile.actorDisplayName} · ${effectiveSample}% 低风险人工抽样`,
      });
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
              <span className="rounded-full border bg-muted/45 px-2 py-0.5 text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">
                {profile.actorType}
              </span>
              {!profile.configured ? (
                <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium text-amber-800">
                  保守默认值
                </span>
              ) : null}
              {disabled ? (
                <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">已停用</span>
              ) : null}
            </div>
            <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
              {profile.actorId}
            </p>
          </div>
        </div>
        {profile.updatedAt ? (
          <span className="inline-flex items-center gap-1 text-[10px] text-muted-foreground">
            <Clock3 className="size-3" aria-hidden />
            {DATE_FORMATTER.format(profile.updatedAt)}
          </span>
        ) : null}
      </div>

      <div className="mt-6">
        <Label>信任等级</Label>
        <div className="mt-2 grid gap-2 md:grid-cols-4">
          {LEVELS.map((level) => {
            const Icon = level.icon;
            const selected = trustLevel === level.value;
            return (
              <button
                key={level.value}
                type="button"
                disabled={disabled}
                aria-pressed={selected}
                onClick={() => {
                  setTrustLevel(level.value);
                  if (level.value === "untrusted") setSamplePercent(100);
                  if (level.value === "full_access") setSamplePercent(0);
                }}
                className={cn(
                  "rounded-2xl border p-3 text-left transition",
                  selected
                    ? "border-fuchsia-300 bg-fuchsia-50 text-fuchsia-950 ring-2 ring-fuchsia-100"
                    : "border-border/75 bg-background hover:border-fuchsia-200 hover:bg-fuchsia-50/35",
                )}
              >
                <span className="flex items-center gap-2 text-xs font-semibold">
                  <Icon
                    className={cn(
                      "size-3.5",
                      selected
                        ? "text-fuchsia-700"
                        : "text-muted-foreground",
                    )}
                    aria-hidden
                  />
                  {level.label}
                </span>
                <span className="mt-1.5 block text-[10px] leading-4 text-muted-foreground">
                  {level.detail}
                </span>
              </button>
            );
          })}
        </div>
      </div>

      <div className="mt-6 rounded-2xl border bg-muted/25 p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <Label htmlFor={`sample-${profile.actorId}`}>
              低风险人工抽样
            </Label>
            <p className="mt-1 text-[10px] leading-4 text-muted-foreground">
              全自动处理会跳过抽样审核，自动批准导入修改并使其生效。
            </p>
          </div>
          <div className="relative w-24">
            <Input
              id={`sample-${profile.actorId}`}
              type="number"
              min={0}
              max={100}
              value={effectiveSample}
              disabled={disabled || trustLevel === "untrusted" || trustLevel === "full_access"}
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
          disabled={disabled || trustLevel === "untrusted" || trustLevel === "full_access"}
          onChange={(event) => setSamplePercent(Number(event.target.value))}
          className="mt-4 h-1.5 w-full accent-fuchsia-700 disabled:opacity-45"
          aria-label="调整低风险人工抽样比例"
        />
        {trustLevel === "untrusted" ? (
          <p className="mt-3 text-[10px] font-medium text-amber-800">
            未受信任的机器人提交的每条修改都需要人工审核。
          </p>
        ) : trustLevel === "full_access" ? (
          <p className="mt-3 text-[10px] font-medium text-rose-800">
            开启后，导入修改会自动批准并生效。请只为已确认可靠的专用机器人开启。
          </p>
        ) : null}
      </div>

      <div className="mt-5 flex flex-wrap items-center justify-between gap-4 border-t pt-4">
        <p className="text-[10px] leading-4 text-muted-foreground">
          只有导入修改支持全自动处理，其他修改仍需人工审核。抽样只用于标出重点复核项。
        </p>
        <Button
          type="button"
          disabled={disabled || !dirty || saving}
          onClick={() => void save()}
        >
          {saving ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <Check aria-hidden />
          )}
          {saving ? "保存中…" : "保存策略"}
        </Button>
      </div>
      {actor ? (
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
          <p className="text-[10px] leading-4 text-muted-foreground">
            {actor.ownerActorId === NIL_UUID
              ? "所属用户账号已删除，这个机器人已永久停用。"
              : "停用会撤销全部访问凭证；再次启用后需要重新授权。"}
          </p>
          <Button type="button" size="sm" variant="outline" disabled={changingStatus || actor.ownerActorId === NIL_UUID} onClick={() => void changeStatus()}>
            {changingStatus ? <LoaderCircle className="animate-spin" aria-hidden /> : <Power aria-hidden />}
            {disabled ? "启用机器人" : "停用机器人"}
          </Button>
        </div>
      ) : null}
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
      toast.success("机器人账号已创建，提交的修改默认需要人工审核。");
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
        <p className="mt-2 text-sm text-muted-foreground">
          管理员可以创建机器人账号，设置哪些修改需要重点复核。
        </p>
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
        <p className="mt-2 max-w-2xl text-sm leading-6 text-amber-900/70">
          审核者可以处理 AI 修改建议；机器人的信任等级、抽样比例和全自动处理设置仅管理员可修改。
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <section className="rounded-3xl border bg-card p-5">
        <h2 className="text-lg font-semibold">创建机器人账号</h2>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">
          用于 Agent CLI 的独立身份，归属于当前 Wiki，由创建者账号管理授权。
        </p>
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
            <>创建后到 <Link href="/settings/cli" className="underline underline-offset-4">Agent CLI</Link> 选择该身份并生成授权码。</>
          ) : (
            "切换到自己的 Wiki 后可在 Agent CLI 中选择该身份并生成授权码。"
          )}
          机器人可以读取所属 Wiki 并提交修改建议。是否需要人工审核，取决于下方的审核设置。
        </p>
        {actors.error ? <p className="mt-3 text-xs text-destructive">机器人账号暂时无法读取，请刷新重试。</p> : null}
      </section>
      <section className="grid gap-3 sm:grid-cols-3">
        {[
          { label: "机器人账号", value: items.length, icon: Bot },
          { label: "已显式配置", value: configured, icon: ShieldCheck },
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
            <p className="mt-1 text-sm text-muted-foreground">
              未单独设置的机器人也会列在这里，提交的修改默认全部人工审核。
            </p>
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
            刷新策略
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
            <p className="font-medium text-destructive">信任策略暂时无法读取</p>
            <p className="mt-1 text-muted-foreground">
              请稍后重试。如果问题持续，请联系管理员。
            </p>
          </div>
        ) : null}
        {!profiles.isLoading && !profiles.error && items.length === 0 ? (
          <div className="mt-5 rounded-3xl border border-dashed px-6 py-14 text-center">
            <Bot className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">还没有机器人账号</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              在上方创建机器人账号，再到 Agent CLI 绑定授权，策略会自动出现在这里。
            </p>
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
