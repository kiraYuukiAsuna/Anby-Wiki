"use client";

import {
  ArrowUpRight,
  BadgeCheck,
  CircleOff,
  Globe2,
  Inbox,
  Link2,
  LoaderCircle,
  LockKeyhole,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  ShieldQuestion,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";
import { z } from "zod";

import {
  ResponseError,
  type EntityFederationLink,
  type EntityFederationLinkListPage,
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
import { LOGIN_PATH, useSession } from "@/lib/auth";
import {
  FEDERATION_TRUST_LEVELS,
  RELATION_LABEL,
  TRUST_LABEL,
  VERIFICATION_LABEL,
  type FederatedWikiStatus,
  type FederationTrust,
} from "@/lib/federation";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 30;
const querySchema = z.string().trim().max(255, "搜索词不能超过 255 个字符");
const wikiSchema = z
  .object({
    wikiKey: z
      .string()
      .trim()
      .regex(
        /^[a-z][a-z0-9._-]{0,99}$/,
        "Key 需以小写字母开头，仅含小写字母、数字、点、下划线或连字符",
      ),
    displayName: z.string().trim().min(1, "请填写显示名称").max(255),
    baseUrl: z.url("请填写完整的 http(s) URL"),
    entityUrlTemplate: z
      .string()
      .trim()
      .min(1, "请填写 Entity URL 模板")
      .refine((value) => value.includes("{entity_id}"), {
        message: "模板必须包含 {entity_id}",
      }),
    trustLevel: z.enum(["untrusted", "reference", "trusted"]),
    status: z.enum(["active", "disabled"]),
  })
  .superRefine((value, context) => {
    try {
      const base = new URL(value.baseUrl);
      const template = new URL(
        value.entityUrlTemplate.replaceAll("{entity_id}", "probe"),
      );
      if (
        !["http:", "https:"].includes(base.protocol) ||
        base.username ||
        base.password ||
        base.search ||
        base.hash
      ) {
        context.addIssue({
          code: "custom",
          path: ["baseUrl"],
          message: "基础地址须为无凭据、查询或锚点的 http(s) URL",
        });
      }
      if (
        template.origin !== base.origin ||
        template.username ||
        template.password ||
        template.hash
      ) {
        context.addIssue({
          code: "custom",
          path: ["entityUrlTemplate"],
          message: "模板必须与基础地址同源，且不能包含凭据或锚点",
        });
      }
    } catch {
      context.addIssue({
        code: "custom",
        path: ["entityUrlTemplate"],
        message: "Entity URL 模板不是合法 URL",
      });
    }
  });

type LinkStatus = "active" | "deprecated" | "all";
type WikiDraft = z.input<typeof wikiSchema> & { id?: string };

const EMPTY_WIKI: WikiDraft = {
  wikiKey: "",
  displayName: "",
  baseUrl: "https://",
  entityUrlTemplate: "https://",
  trustLevel: "reference",
  status: "active",
};

const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "short",
  day: "numeric",
});

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}

function WikiEditor({
  draft,
  onChange,
  onClose,
  onSaved,
}: {
  draft: WikiDraft;
  onChange: (next: WikiDraft) => void;
  onClose: () => void;
  onSaved: () => Promise<unknown>;
}) {
  const [saving, setSaving] = useState(false);

  const save = async () => {
    const parsed = wikiSchema.safeParse(draft);
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查远端 Wiki 配置");
      return;
    }
    setSaving(true);
    try {
      if (draft.id) {
        await knowledgeApi().updateFederatedWiki({
          id: draft.id,
          updateFederatedWikiRequest: {
            displayName: parsed.data.displayName,
            baseUrl: parsed.data.baseUrl,
            entityUrlTemplate: parsed.data.entityUrlTemplate,
            trustLevel: parsed.data.trustLevel,
            status: parsed.data.status,
            metadata: {},
          },
        });
        toast.success("远端 Wiki 配置已更新");
      } else {
        await knowledgeApi().registerFederatedWiki({
          createFederatedWikiRequest: {
            ...parsed.data,
            metadata: {},
          },
        });
        toast.success("远端 Wiki 已登记");
      }
      await onSaved();
      onClose();
    } catch (error) {
      if (error instanceof ResponseError && error.response.status === 403) {
        toast.error("只有站点管理员可以管理 Federation");
      } else if (
        error instanceof ResponseError &&
        error.response.status === 409
      ) {
        toast.error("Wiki key 已存在或状态与现有映射冲突");
      } else {
        toast.error("远端 Wiki 配置保存失败");
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <div className="grid gap-4 py-1 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="federation-wiki-key">稳定 Key</Label>
          <Input
            id="federation-wiki-key"
            value={draft.wikiKey}
            disabled={Boolean(draft.id)}
            placeholder="wikidata"
            onChange={(event) =>
              onChange({ ...draft, wikiKey: event.target.value.toLowerCase() })
            }
          />
          <p className="text-[10px] leading-4 text-muted-foreground">
            创建后不可变，用于审计和持久引用。
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="federation-wiki-name">显示名称</Label>
          <Input
            id="federation-wiki-name"
            value={draft.displayName}
            placeholder="Wikidata"
            onChange={(event) =>
              onChange({ ...draft, displayName: event.target.value })
            }
          />
        </div>
        <div className="space-y-2 sm:col-span-2">
          <Label htmlFor="federation-base-url">基础地址</Label>
          <Input
            id="federation-base-url"
            type="url"
            value={draft.baseUrl}
            placeholder="https://www.wikidata.org"
            onChange={(event) =>
              onChange({ ...draft, baseUrl: event.target.value })
            }
          />
        </div>
        <div className="space-y-2 sm:col-span-2">
          <Label htmlFor="federation-template">Entity URL 模板</Label>
          <Input
            id="federation-template"
            value={draft.entityUrlTemplate}
            placeholder="https://www.wikidata.org/wiki/{entity_id}"
            onChange={(event) =>
              onChange({ ...draft, entityUrlTemplate: event.target.value })
            }
          />
          <p className="text-[10px] text-muted-foreground">
            必须与基础地址同源，并包含 {"{entity_id}"}。
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="federation-trust">信任等级</Label>
          <select
            id="federation-trust"
            value={draft.trustLevel}
            onChange={(event) =>
              onChange({
                ...draft,
                trustLevel: event.target.value as FederationTrust,
              })
            }
            className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            {FEDERATION_TRUST_LEVELS.map((level) => (
              <option key={level.value} value={level.value}>
                {level.label} · {level.detail}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="federation-status">目录状态</Label>
          <select
            id="federation-status"
            value={draft.status}
            onChange={(event) =>
              onChange({
                ...draft,
                status: event.target.value as FederatedWikiStatus,
              })
            }
            className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            <option value="active">启用</option>
            <option value="disabled">停用</option>
          </select>
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button type="button" disabled={saving} onClick={() => void save()}>
          {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          {saving ? "保存中…" : draft.id ? "保存配置" : "登记 Wiki"}
        </Button>
      </DialogFooter>
    </>
  );
}

function WikiCard({
  wiki,
  canManage,
  onEdit,
}: {
  wiki: FederatedWiki;
  canManage: boolean;
  onEdit: () => void;
}) {
  const host = new URL(wiki.baseUrl).host;
  return (
    <article
      className={cn(
        "rounded-2xl border bg-card p-5 shadow-[0_12px_36px_-32px_rgb(15_23_42/0.55)]",
        wiki.status === "disabled" && "border-dashed opacity-70",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <span
          className={cn(
            "flex size-10 items-center justify-center rounded-xl",
            wiki.trustLevel === "trusted"
              ? "bg-emerald-100 text-emerald-700"
              : wiki.trustLevel === "reference"
                ? "bg-indigo-100 text-indigo-700"
                : "bg-slate-100 text-slate-600",
          )}
        >
          {wiki.status === "disabled" ? (
            <CircleOff className="size-4.5" aria-hidden />
          ) : wiki.trustLevel === "trusted" ? (
            <BadgeCheck className="size-4.5" aria-hidden />
          ) : (
            <Globe2 className="size-4.5" aria-hidden />
          )}
        </span>
        {canManage ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={`编辑 ${wiki.displayName}`}
            onClick={onEdit}
          >
            <Pencil aria-hidden />
          </Button>
        ) : null}
      </div>
      <h3 className="mt-4 truncate font-semibold">{wiki.displayName}</h3>
      <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">
        {wiki.wikiKey} · {host}
      </p>
      <div className="mt-4 flex flex-wrap gap-1.5">
        <span className="rounded-full bg-muted px-2 py-1 text-[10px] font-medium">
          {TRUST_LABEL[wiki.trustLevel]}
        </span>
        <span
          className={cn(
            "rounded-full px-2 py-1 text-[10px] font-medium",
            wiki.status === "active"
              ? "bg-emerald-100 text-emerald-800"
              : "bg-slate-100 text-slate-600",
          )}
        >
          {wiki.status === "active" ? "正在使用" : "已停用"}
        </span>
      </div>
    </article>
  );
}

function LinkCard({ item }: { item: EntityFederationLink }) {
  return (
    <article className="rounded-2xl border bg-card p-5 transition-colors hover:border-indigo-200">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-[10px] font-semibold tracking-[0.12em] text-indigo-700 uppercase">
            {item.remoteWikiName} · {TRUST_LABEL[item.remoteTrustLevel]}
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <Link
              href={`/entities/${item.localEntityId}`}
              className="truncate font-semibold hover:text-indigo-700 hover:underline"
            >
              {item.localLabel}
            </Link>
            <span className="text-muted-foreground" aria-hidden>
              ↔
            </span>
            <a
              href={item.remoteEntityUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex min-w-0 items-center gap-1 truncate font-semibold text-indigo-700 hover:underline"
            >
              {item.remoteLabel || item.remoteCanonicalKey || item.remoteEntityId}
              <ArrowUpRight className="size-3.5 shrink-0" aria-hidden />
            </a>
          </div>
          <p className="mt-2 truncate font-mono text-[10px] text-muted-foreground">
            {item.localCanonicalKey} · {item.remoteWikiKey}:
            {item.remoteEntityId}
          </p>
        </div>
        <span
          className={cn(
            "rounded-full px-2 py-1 text-[10px] font-semibold",
            item.status === "deprecated"
              ? "bg-slate-100 text-slate-600"
              : item.verificationStatus === "human_verified"
                ? "bg-emerald-100 text-emerald-800"
                : item.verificationStatus === "disputed"
                  ? "bg-rose-100 text-rose-800"
                  : "bg-amber-100 text-amber-800",
          )}
        >
          {item.status === "deprecated"
            ? "已弃用"
            : VERIFICATION_LABEL[item.verificationStatus]}
        </span>
      </div>
      <div className="mt-4 flex items-center justify-between border-t pt-3 text-[10px] text-muted-foreground">
        <span>{RELATION_LABEL[item.relationType]}</span>
        <span>更新于 {DATE_FORMATTER.format(item.updatedAt)}</span>
      </div>
    </article>
  );
}

export function FederationWorkspace() {
  const session = useSession();
  const [query, setQuery] = useState("");
  const [remoteWikiID, setRemoteWikiID] = useState("");
  const [status, setStatus] = useState<LinkStatus>("active");
  const [wikiDraft, setWikiDraft] = useState<WikiDraft>();
  const debouncedQuery = useDebouncedValue(query, 220);
  const parsedQuery = querySchema.safeParse(debouncedQuery);
  const normalizedQuery = parsedQuery.success ? parsedQuery.data : "";

  const wikiState = useSWR("federation:wikis:all", () =>
    knowledgeApi().listFederatedWikis({ includeDisabled: true }),
  );
  const wikis = useMemo(() => wikiState.data?.items ?? [], [wikiState.data]);

  const linksState = useSWRInfinite<EntityFederationLinkListPage>(
    (pageIndex, previousPage) => {
      if (!parsedQuery.success) return null;
      if (pageIndex > 0 && !previousPage?.nextCursor) return null;
      return [
        "federation:links",
        normalizedQuery,
        remoteWikiID,
        status,
        pageIndex === 0 ? "" : (previousPage?.nextCursor ?? ""),
      ] as const;
    },
    (key) => {
      const [, q, selectedWiki, selectedStatus, cursor] = key as readonly [
        string,
        string,
        string,
        LinkStatus,
        string,
      ];
      return knowledgeApi().listFederationLinks({
        q: q || undefined,
        remoteWikiId: selectedWiki || undefined,
        status: selectedStatus,
        cursor: cursor || undefined,
        pageSize: PAGE_SIZE,
      });
    },
    { keepPreviousData: true, revalidateFirstPage: true },
  );
  const links = linksState.data?.flatMap((page) => page.items) ?? [];
  const nextCursor = linksState.data?.at(-1)?.nextCursor;
  const activeWikis = wikis.filter((wiki) => wiki.status === "active").length;
  const trustedWikis = wikis.filter(
    (wiki) => wiki.status === "active" && wiki.trustLevel === "trusted",
  ).length;

  const editWiki = (wiki: FederatedWiki) =>
    setWikiDraft({
      id: wiki.id,
      wikiKey: wiki.wikiKey,
      displayName: wiki.displayName,
      baseUrl: wiki.baseUrl,
      entityUrlTemplate: wiki.entityUrlTemplate,
      trustLevel: wiki.trustLevel,
      status: wiki.status,
    });

  const refresh = async () => {
    await Promise.all([wikiState.mutate(), linksState.mutate()]);
  };

  return (
    <div className="space-y-9">
      <section className="grid gap-3 sm:grid-cols-3">
        {[
          { label: "启用的远端 Wiki", value: activeWikis, icon: Globe2 },
          { label: "可信身份源", value: trustedWikis, icon: BadgeCheck },
          { label: "当前筛选映射", value: links.length, icon: Link2 },
        ].map(({ label, value, icon: Icon }) => (
          <div key={label} className="rounded-2xl border bg-card p-5">
            <Icon className="size-4 text-indigo-700" aria-hidden />
            <p className="mt-4 text-3xl font-semibold tracking-tight">
              {wikiState.isLoading || linksState.isLoading ? "—" : value}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">{label}</p>
          </div>
        ))}
      </section>

      <section aria-labelledby="remote-wiki-title">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h2 id="remote-wiki-title" className="text-xl font-semibold">
              远端 Wiki 目录
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              先登记身份源和信任边界，再把本地 Entity 显式映射过去。
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={wikiState.isValidating}
              onClick={() => void refresh()}
            >
              <RefreshCw
                className={cn(
                  "size-3.5",
                  wikiState.isValidating && "animate-spin",
                )}
                aria-hidden
              />
              刷新
            </Button>
            {session.isAuthenticated ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setWikiDraft({ ...EMPTY_WIKI })}
              >
                <Plus aria-hidden />
                登记远端 Wiki
              </Button>
            ) : (
              <Button asChild size="sm">
                <Link href={LOGIN_PATH}>
                  <LockKeyhole aria-hidden />
                  登录后管理
                </Link>
              </Button>
            )}
          </div>
        </div>

        {wikiState.error ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            远端 Wiki 目录暂时不可用。
          </div>
        ) : null}
        {wikiState.isLoading ? (
          <div className="mt-5 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            {[0, 1, 2, 3].map((item) => (
              <div
                key={item}
                className="h-48 animate-pulse rounded-2xl border bg-muted/30"
              />
            ))}
          </div>
        ) : null}
        {!wikiState.isLoading && !wikiState.error && wikis.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed p-8 text-center">
            <Globe2 className="mx-auto size-7 text-muted-foreground" aria-hidden />
            <h3 className="mt-3 font-semibold">还没有登记远端 Wiki</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              管理员登记第一个身份源后，Entity 详情页即可创建映射。
            </p>
          </div>
        ) : null}
        {wikis.length > 0 ? (
          <div className="mt-5 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            {wikis.map((wiki) => (
              <WikiCard
                key={wiki.id}
                wiki={wiki}
                canManage={session.isAuthenticated}
                onEdit={() => editWiki(wiki)}
              />
            ))}
          </div>
        ) : null}
      </section>

      <section aria-labelledby="federation-links-title">
        <div>
          <h2 id="federation-links-title" className="text-xl font-semibold">
            Entity 映射流
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            映射是独立、可审计的知识，不会把远端内容复制成本站事实。
          </p>
        </div>
        <div className="mt-5 grid gap-3 rounded-2xl border bg-card/70 p-4 md:grid-cols-[minmax(0,1fr)_13rem_10rem]">
          <label className="relative block">
            <span className="sr-only">搜索 Federation 映射</span>
            <Search
              className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground"
              aria-hidden
            />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="本地或远端名称、Key、ID…"
              className="pl-8"
              aria-invalid={!querySchema.safeParse(query).success}
            />
          </label>
          <label>
            <span className="sr-only">远端 Wiki</span>
            <select
              value={remoteWikiID}
              onChange={(event) => setRemoteWikiID(event.target.value)}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="">全部远端 Wiki</option>
              {wikis.map((wiki) => (
                <option key={wiki.id} value={wiki.id}>
                  {wiki.displayName}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="sr-only">映射状态</span>
            <select
              value={status}
              onChange={(event) => setStatus(event.target.value as LinkStatus)}
              className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <option value="active">有效映射</option>
              <option value="all">全部可追溯</option>
              <option value="deprecated">已弃用</option>
            </select>
          </label>
        </div>
        {!parsedQuery.success ? (
          <p className="mt-2 text-xs text-destructive">
            {parsedQuery.error.issues[0]?.message}
          </p>
        ) : null}

        {linksState.error ? (
          <div className="mt-5 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">
            Federation 映射暂时无法读取。
          </div>
        ) : null}
        {linksState.isLoading ? (
          <div className="mt-5 grid gap-4 lg:grid-cols-2">
            {[0, 1, 2, 3].map((item) => (
              <div
                key={item}
                className="h-40 animate-pulse rounded-2xl border bg-muted/30"
              />
            ))}
          </div>
        ) : null}
        {!linksState.isLoading && !linksState.error && links.length === 0 ? (
          <div className="mt-5 rounded-2xl border border-dashed px-6 py-12 text-center">
            <Inbox className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">没有符合条件的映射</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              从任意 Entity 详情页建立远端身份映射。
            </p>
            <Button asChild variant="outline" className="mt-5">
              <Link href="/entities">浏览 Entity</Link>
            </Button>
          </div>
        ) : null}
        {links.length > 0 ? (
          <div className="mt-5 grid gap-4 lg:grid-cols-2">
            {links.map((item) => (
              <LinkCard key={item.id} item={item} />
            ))}
          </div>
        ) : null}
        {nextCursor ? (
          <div className="mt-6 flex justify-center">
            <Button
              type="button"
              variant="outline"
              disabled={linksState.isValidating}
              onClick={() => void linksState.setSize(linksState.size + 1)}
            >
              {linksState.isValidating ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : null}
              载入更多映射
            </Button>
          </div>
        ) : null}
      </section>

      <aside className="grid gap-4 rounded-3xl border border-indigo-200/70 bg-indigo-50/55 p-6 md:grid-cols-[auto_1fr]">
        <ShieldQuestion className="size-5 text-indigo-700" aria-hidden />
        <div>
          <h2 className="font-semibold text-indigo-950">信任等级不是自动同步许可</h2>
          <p className="mt-1 max-w-4xl text-xs leading-6 text-indigo-950/65">
            Federation 只声明身份关系。远端事实仍需经过 Source、Citation、Claim
            与 Proposal 治理链路才能进入本站权威状态。
          </p>
        </div>
      </aside>

      <Dialog
        open={Boolean(wikiDraft)}
        onOpenChange={(open) => {
          if (!open) setWikiDraft(undefined);
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {wikiDraft?.id ? "编辑远端 Wiki" : "登记远端 Wiki"}
            </DialogTitle>
            <DialogDescription>
              配置身份链接模板与本地信任边界；不会抓取或镜像远端内容。
            </DialogDescription>
          </DialogHeader>
          {wikiDraft ? (
            <WikiEditor
              draft={wikiDraft}
              onChange={setWikiDraft}
              onClose={() => setWikiDraft(undefined)}
              onSaved={refresh}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}
