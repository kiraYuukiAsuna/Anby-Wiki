"use client";

import {
  ArrowUpRight,
  BadgeCheck,
  CircleOff,
  Globe2,
  Link2,
  LoaderCircle,
  Pencil,
  Plus,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type EntityFederationLink,
  type FederatedWiki,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { knowledgeApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";
import {
  FEDERATION_RELATIONS,
  FEDERATION_VERIFICATIONS,
  RELATION_LABEL,
  TRUST_LABEL,
  VERIFICATION_LABEL,
  type FederationLinkStatus,
  type FederationRelation,
  type FederationVerification,
} from "@/lib/federation";
import { cn } from "@/lib/utils";

const linkSchema = z.object({
  remoteWikiId: z.string().uuid("请选择远端 Wiki"),
  remoteEntityId: z
    .string()
    .trim()
    .min(1, "请填写远端 Entity ID")
    .max(512, "远端 ID 不能超过 512 个字符"),
  remoteCanonicalKey: z
    .string()
    .trim()
    .max(255, "远端稳定键不能超过 255 个字符"),
  remoteLabel: z
    .string()
    .trim()
    .max(255, "远端标签不能超过 255 个字符"),
  relationType: z.enum(["same_as", "broader", "narrower", "related"]),
  verificationStatus: z.enum([
    "unverified",
    "human_verified",
    "disputed",
  ]),
  status: z.enum(["active", "deprecated"]),
});

type LinkDraft = z.input<typeof linkSchema> & {
  id?: string;
  remoteWikiName?: string;
  metadata: Record<string, unknown>;
};

function emptyDraft(remoteWikiID = ""): LinkDraft {
  return {
    remoteWikiId: remoteWikiID,
    remoteEntityId: "",
    remoteCanonicalKey: "",
    remoteLabel: "",
    relationType: "same_as",
    verificationStatus: "unverified",
    status: "active",
    metadata: {},
  };
}

function draftFromLink(item: EntityFederationLink): LinkDraft {
  return {
    id: item.id,
    remoteWikiId: item.remoteWikiId,
    remoteWikiName: item.remoteWikiName,
    remoteEntityId: item.remoteEntityId,
    remoteCanonicalKey: item.remoteCanonicalKey,
    remoteLabel: item.remoteLabel,
    relationType: item.relationType,
    verificationStatus: item.verificationStatus,
    status: item.status,
    metadata: item.metadata,
  };
}

function FederationLinkEditor({
  entityID,
  draft,
  wikis,
  onChange,
  onClose,
  onSaved,
}: {
  entityID: string;
  draft: LinkDraft;
  wikis: FederatedWiki[];
  onChange: (next: LinkDraft) => void;
  onClose: () => void;
  onSaved: () => Promise<unknown>;
}) {
  const [saving, setSaving] = useState(false);
  const parsed = linkSchema.safeParse(draft);

  const save = async () => {
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查 Federation 映射");
      return;
    }
    setSaving(true);
    try {
      if (draft.id) {
        await knowledgeApi().updateFederationLink({
          id: draft.id,
          updateEntityFederationLinkRequest: {
            remoteCanonicalKey: parsed.data.remoteCanonicalKey,
            remoteLabel: parsed.data.remoteLabel,
            relationType: parsed.data.relationType,
            verificationStatus: parsed.data.verificationStatus,
            status: parsed.data.status,
            metadata: draft.metadata,
          },
        });
        toast.success(
          parsed.data.status === "deprecated"
            ? "Federation 映射已弃用"
            : "Federation 映射已更新",
        );
      } else {
        await knowledgeApi().createEntityFederationLink({
          id: entityID,
          createEntityFederationLinkRequest: {
            remoteWikiId: parsed.data.remoteWikiId,
            remoteEntityId: parsed.data.remoteEntityId,
            remoteCanonicalKey: parsed.data.remoteCanonicalKey,
            remoteLabel: parsed.data.remoteLabel,
            relationType: parsed.data.relationType,
            verificationStatus: parsed.data.verificationStatus,
            metadata: {},
          },
        });
        toast.success("远端身份映射已创建");
      }
      await onSaved();
      onClose();
    } catch (error) {
      if (isUnauthorized(error)) {
        toast.error("登录状态已失效");
      } else if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("只有站点管理员可以管理 Federation");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 409
      ) {
        toast.error("该映射已存在，或远端 Wiki 当前不可用");
      } else {
        toast.error("Federation 映射保存失败");
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <div className="grid gap-4 py-1 sm:grid-cols-2">
        <div className="space-y-2 sm:col-span-2">
          <Label htmlFor="entity-federation-wiki">远端 Wiki</Label>
          {draft.id ? (
            <div className="flex h-8 items-center rounded-lg border bg-muted/35 px-3 text-sm">
              {draft.remoteWikiName}
            </div>
          ) : (
            <select
              id="entity-federation-wiki"
              value={draft.remoteWikiId}
              onChange={(event) =>
                onChange({ ...draft, remoteWikiId: event.target.value })
              }
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="">选择身份源…</option>
              {wikis.map((wiki) => (
                <option key={wiki.id} value={wiki.id}>
                  {wiki.displayName} · {TRUST_LABEL[wiki.trustLevel]}
                </option>
              ))}
            </select>
          )}
        </div>
        <div className="space-y-2 sm:col-span-2">
          <Label htmlFor="entity-federation-remote-id">远端 Entity ID</Label>
          <Input
            id="entity-federation-remote-id"
            value={draft.remoteEntityId}
            disabled={Boolean(draft.id)}
            placeholder="Q42"
            onChange={(event) =>
              onChange({ ...draft, remoteEntityId: event.target.value })
            }
          />
          {draft.id ? (
            <p className="text-[10px] text-muted-foreground">
              远端目标是映射身份的一部分；如目标错误，请弃用此映射后新建。
            </p>
          ) : null}
        </div>
        <div className="space-y-2">
          <Label htmlFor="entity-federation-label">远端显示标签</Label>
          <Input
            id="entity-federation-label"
            value={draft.remoteLabel}
            placeholder="Douglas Adams"
            onChange={(event) =>
              onChange({ ...draft, remoteLabel: event.target.value })
            }
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="entity-federation-key">远端稳定键</Label>
          <Input
            id="entity-federation-key"
            value={draft.remoteCanonicalKey}
            placeholder="可选"
            onChange={(event) =>
              onChange({ ...draft, remoteCanonicalKey: event.target.value })
            }
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="entity-federation-relation">身份关系</Label>
          <select
            id="entity-federation-relation"
            value={draft.relationType}
            onChange={(event) =>
              onChange({
                ...draft,
                relationType: event.target.value as FederationRelation,
              })
            }
            className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            {FEDERATION_RELATIONS.map((relation) => (
              <option key={relation.value} value={relation.value}>
                {relation.label} · {relation.detail}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="entity-federation-verification">核验状态</Label>
          <select
            id="entity-federation-verification"
            value={draft.verificationStatus}
            onChange={(event) =>
              onChange({
                ...draft,
                verificationStatus: event.target
                  .value as FederationVerification,
              })
            }
            className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            {FEDERATION_VERIFICATIONS.map((verification) => (
              <option key={verification.value} value={verification.value}>
                {verification.label}
              </option>
            ))}
          </select>
        </div>
        {draft.id ? (
          <div className="space-y-2 sm:col-span-2">
            <Label htmlFor="entity-federation-status">映射状态</Label>
            <select
              id="entity-federation-status"
              value={draft.status}
              onChange={(event) =>
                onChange({
                  ...draft,
                  status: event.target.value as FederationLinkStatus,
                })
              }
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="active">有效</option>
              <option value="deprecated">弃用但保留审计</option>
            </select>
          </div>
        ) : null}
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button
          type="button"
          disabled={saving || !parsed.success}
          onClick={() => void save()}
        >
          {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          {saving ? "保存中…" : draft.id ? "保存映射" : "创建映射"}
        </Button>
      </DialogFooter>
    </>
  );
}

export function EntityFederationPanel({
  entityID,
  entityTitle,
  status,
}: {
  entityID: string;
  entityTitle: string;
  status: "active" | "merged" | "deleted";
}) {
  const session = useSession();
  const [draft, setDraft] = useState<LinkDraft>();
  const linksState = useSWR(["entity:federation-links", entityID], () =>
    knowledgeApi().listEntityFederationLinks({ id: entityID, status: "all" }),
  );
  const wikisState = useSWR("federation:wikis:active", () =>
    knowledgeApi().listFederatedWikis({ includeDisabled: false }),
  );
  const items = linksState.data?.items ?? [];
  const activeWikis = wikisState.data?.items ?? [];
  const canWrite = session.isAuthenticated && status === "active";

  const startCreate = () => {
    if (!session.isAuthenticated) {
      toast.error("请先登录后管理 Federation");
      return;
    }
    if (!activeWikis.length) {
      toast.error("请先在 Federation 中心登记一个启用的远端 Wiki");
      return;
    }
    setDraft(emptyDraft(activeWikis[0]?.id));
  };

  return (
    <div>
      <div className="flex flex-wrap items-start justify-between gap-4 rounded-xl border bg-muted/20 p-4">
        <div>
          <p className="flex items-center gap-2 text-sm font-semibold">
            <Globe2 className="size-4 text-indigo-700" aria-hidden />
            跨 Wiki 身份
          </p>
          <p className="mt-1 max-w-xl text-xs leading-5 text-muted-foreground">
            映射只连接稳定身份；远端事实不会绕过本站的来源、提案与审核链路。
          </p>
        </div>
        <div className="flex gap-2">
          <Button asChild type="button" variant="outline" size="sm">
            <Link href="/federation">Federation 中心</Link>
          </Button>
          {canWrite ? (
            <Button
              type="button"
              size="sm"
              disabled={wikisState.isLoading}
              onClick={startCreate}
            >
              <Plus aria-hidden />
              添加映射
            </Button>
          ) : !session.isAuthenticated ? (
            <Button asChild type="button" size="sm">
              <Link href={LOGIN_PATH}>登录后管理</Link>
            </Button>
          ) : null}
        </div>
      </div>

      {status !== "active" ? (
        <div className="mt-3 flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50/65 p-4 text-xs leading-5">
          <TriangleAlert
            className="mt-0.5 size-4 shrink-0 text-amber-700"
            aria-hidden
          />
          <p className="text-amber-950/75">
            当前 Entity 已{status === "merged" ? "合并" : "删除"}，映射只读保留；
            请在有效身份上继续治理。
          </p>
        </div>
      ) : null}

      {linksState.isLoading ? (
        <div className="mt-3 grid gap-3 md:grid-cols-2">
          {[0, 1].map((item) => (
            <div
              key={item}
              className="h-32 animate-pulse rounded-xl border bg-muted/30"
            />
          ))}
        </div>
      ) : null}
      {linksState.error ? (
        <p className="mt-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive">
          跨 Wiki 映射暂时无法读取。
        </p>
      ) : null}
      {!linksState.isLoading && !linksState.error && items.length === 0 ? (
        <div className="mt-3 rounded-xl border border-dashed p-6 text-center">
          <Link2 className="mx-auto size-6 text-muted-foreground" aria-hidden />
          <p className="mt-3 text-sm font-medium">尚无远端身份映射</p>
          <p className="mt-1 text-xs text-muted-foreground">
            为「{entityTitle}」连接可信百科或专业知识库中的稳定 ID。
          </p>
        </div>
      ) : null}
      {items.length > 0 ? (
        <ul className="mt-3 grid gap-3 md:grid-cols-2">
          {items.map((item) => (
            <li
              key={item.id}
              className={cn(
                "rounded-xl border bg-card p-4",
                item.status === "deprecated" && "border-dashed opacity-70",
              )}
            >
              <div className="flex items-start justify-between gap-3">
                <span
                  className={cn(
                    "flex size-8 shrink-0 items-center justify-center rounded-lg",
                    item.verificationStatus === "human_verified"
                      ? "bg-emerald-100 text-emerald-700"
                      : item.verificationStatus === "disputed"
                        ? "bg-rose-100 text-rose-700"
                        : "bg-indigo-100 text-indigo-700",
                  )}
                >
                  {item.status === "deprecated" ? (
                    <CircleOff className="size-4" aria-hidden />
                  ) : item.verificationStatus === "human_verified" ? (
                    <BadgeCheck className="size-4" aria-hidden />
                  ) : (
                    <Globe2 className="size-4" aria-hidden />
                  )}
                </span>
                {canWrite ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`编辑 ${item.remoteWikiName} 映射`}
                    onClick={() => setDraft(draftFromLink(item))}
                  >
                    <Pencil aria-hidden />
                  </Button>
                ) : null}
              </div>
              <p className="mt-3 text-[10px] font-semibold tracking-wide text-indigo-700 uppercase">
                {item.remoteWikiName} · {TRUST_LABEL[item.remoteTrustLevel]}
              </p>
              <a
                href={item.remoteEntityUrl}
                target="_blank"
                rel="noreferrer"
                className="mt-1 inline-flex max-w-full items-center gap-1 truncate text-sm font-semibold hover:text-indigo-700 hover:underline"
              >
                <span className="truncate">
                  {item.remoteLabel ||
                    item.remoteCanonicalKey ||
                    item.remoteEntityId}
                </span>
                <ArrowUpRight className="size-3.5 shrink-0" aria-hidden />
              </a>
              <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
                {item.remoteWikiKey}:{item.remoteEntityId}
              </p>
              <div className="mt-3 flex flex-wrap gap-1.5 text-[10px]">
                <span className="rounded-full bg-muted px-2 py-1">
                  {RELATION_LABEL[item.relationType]}
                </span>
                <span
                  className={cn(
                    "rounded-full px-2 py-1",
                    item.verificationStatus === "human_verified"
                      ? "bg-emerald-100 text-emerald-800"
                      : item.verificationStatus === "disputed"
                        ? "bg-rose-100 text-rose-800"
                        : "bg-amber-100 text-amber-800",
                  )}
                >
                  {VERIFICATION_LABEL[item.verificationStatus]}
                </span>
                {item.status === "deprecated" ? (
                  <span className="rounded-full bg-slate-100 px-2 py-1 text-slate-600">
                    已弃用
                  </span>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      ) : null}

      <Dialog
        open={Boolean(draft)}
        onOpenChange={(open) => {
          if (!open) setDraft(undefined);
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {draft?.id ? "编辑远端身份映射" : "添加远端身份映射"}
            </DialogTitle>
            <DialogDescription>
              为「{entityTitle}」声明一个明确、可核验的跨 Wiki 身份关系。
            </DialogDescription>
          </DialogHeader>
          {draft ? (
            <FederationLinkEditor
              entityID={entityID}
              draft={draft}
              wikis={activeWikis}
              onChange={setDraft}
              onClose={() => setDraft(undefined)}
              onSaved={() => linksState.mutate()}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
