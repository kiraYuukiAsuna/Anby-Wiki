"use client";

import {
  BadgeCheck,
  CircleAlert,
  CircleDashed,
  LoaderCircle,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";

import {
  ResponseError,
  type ClaimDetail,
  type UpdateClaimVerificationRequestVerificationStatusEnum,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { knowledgeApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

const OPTIONS: Array<{
  value: UpdateClaimVerificationRequestVerificationStatusEnum;
  label: string;
  detail: string;
  icon: typeof CircleDashed;
  tone: string;
}> = [
  {
    value: "unverified",
    label: "未核验",
    detail: "尚未给出独立核验结论",
    icon: CircleDashed,
    tone: "border-slate-200 bg-slate-50 text-slate-700",
  },
  {
    value: "ai_checked",
    label: "AI 已核对",
    detail: "自动检查完成，仍可由人工复核",
    icon: Sparkles,
    tone: "border-violet-200 bg-violet-50 text-violet-800",
  },
  {
    value: "human_verified",
    label: "人工已核验",
    detail: "Reviewer 确认事实与证据链",
    icon: BadgeCheck,
    tone: "border-emerald-200 bg-emerald-50 text-emerald-800",
  },
  {
    value: "disputed",
    label: "存在争议",
    detail: "值、时效或来源需要进一步处理",
    icon: CircleAlert,
    tone: "border-rose-200 bg-rose-50 text-rose-800",
  },
];

export function ClaimVerificationControl({
  initialDetail,
}: {
  initialDetail: ClaimDetail;
}) {
  const router = useRouter();
  const session = useSession();
  const state = useSWR(
    ["claim:detail", initialDetail.id],
    () => knowledgeApi().getClaim({ id: initialDetail.id }),
    { fallbackData: initialDetail },
  );
  const [saving, setSaving] =
    useState<UpdateClaimVerificationRequestVerificationStatusEnum>();
  const detail = state.data ?? initialDetail;

  const update = async (
    verificationStatus: UpdateClaimVerificationRequestVerificationStatusEnum,
  ) => {
    if (verificationStatus === detail.verificationStatus) return;
    setSaving(verificationStatus);
    try {
      await knowledgeApi().updateClaimVerification({
        id: detail.id,
        updateClaimVerificationRequest: { verificationStatus },
      });
      await state.mutate();
      router.refresh();
      toast.success(
        OPTIONS.find((item) => item.value === verificationStatus)?.label ??
          "核验状态已更新",
        {
          description: "引用此 Claim 的动态阅读内容会由 Worker 精准重渲染。",
        },
      );
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 403
      ) {
        toast.error("需要 Reviewer 或管理员权限才能更新核验状态");
      } else {
        toast.error("核验状态更新失败，请稍后重试");
      }
    } finally {
      setSaving(undefined);
    }
  };

  return (
    <div className="rounded-xl border bg-muted/15 p-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="flex items-center gap-2 text-sm font-semibold">
            <ShieldCheck className="size-4 text-emerald-700" aria-hidden />
            独立核验状态
          </p>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
            核验与 Claim 的业务生命周期正交；结论会进入审计，但不会改写不可变事实版本。
          </p>
        </div>
        {!session.isAuthenticated ? (
          <Button asChild type="button" size="sm">
            <Link href={LOGIN_PATH}>登录后核验</Link>
          </Button>
        ) : null}
      </div>

      <div className="mt-4 grid gap-2 sm:grid-cols-2">
        {OPTIONS.map((option) => {
          const Icon = option.icon;
          const active = detail.verificationStatus === option.value;
          const pending = saving === option.value;
          return (
            <button
              key={option.value}
              type="button"
              disabled={!session.isAuthenticated || Boolean(saving)}
              aria-pressed={active}
              onClick={() => void update(option.value)}
              className={cn(
                "flex items-start gap-3 rounded-xl border p-3 text-left transition",
                option.tone,
                active
                  ? "ring-2 ring-primary/20 ring-offset-2"
                  : "opacity-75 hover:opacity-100",
                (!session.isAuthenticated || saving) && "cursor-not-allowed",
              )}
            >
              <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-white/70">
                {pending ? (
                  <LoaderCircle className="size-4 animate-spin" aria-hidden />
                ) : (
                  <Icon className="size-4" aria-hidden />
                )}
              </span>
              <span>
                <span className="block text-xs font-semibold">{option.label}</span>
                <span className="mt-0.5 block text-[10px] leading-4 opacity-75">
                  {option.detail}
                </span>
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
