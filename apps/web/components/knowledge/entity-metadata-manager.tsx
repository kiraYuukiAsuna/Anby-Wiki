"use client";

import {
  Languages,
  LoaderCircle,
  Plus,
  Star,
  Tags,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";
import { z } from "zod";

import {
  ResponseError,
  type EntityAlias,
  type EntityDetail,
  type EntityLabel,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Textarea } from "@/components/ui/textarea";
import { knowledgeApi } from "@/lib/api";
import { isUnauthorized, LOGIN_PATH, useSession } from "@/lib/auth";

const labelSchema = z.object({
  language: z.string().trim().min(1, "请填写语言标记").max(64),
  label: z.string().trim().min(1, "请填写名称").max(255),
  description: z.string().trim().max(1000),
  isPrimary: z.boolean(),
});

const aliasSchema = z.object({
  language: z.string().trim().min(1, "请填写语言标记").max(64),
  alias: z.string().trim().min(1, "请填写别名").max(255),
  aliasType: z.enum(["common", "historical", "abbreviation", "import"]),
});

type LabelDraft = z.input<typeof labelSchema>;
type AliasDraft = z.input<typeof aliasSchema>;
type Removal =
  | { kind: "label"; value: EntityLabel }
  | { kind: "alias"; value: EntityAlias };

const ALIAS_TYPE_LABEL = {
  common: "通用别名",
  historical: "历史名称",
  abbreviation: "缩写",
  import: "导入名称",
} as const;

function mutationMessage(error: unknown, fallback: string) {
  if (isUnauthorized(error)) return "登录状态已失效";
  if (error instanceof ResponseError) {
    if (error.response.status === 403) return "当前账号没有实体编辑权限";
    if (error.response.status === 404) return "目标已不存在，请刷新后重试";
    if (error.response.status === 409) return "这个名称已存在，或与当前语言的主名称冲突。";
  }
  return fallback;
}

export function EntityMetadataManager({
  initialDetail,
}: {
  initialDetail: EntityDetail;
}) {
  const session = useSession();
  const entityState = useSWR(
    ["entity:detail", initialDetail.id],
    () => knowledgeApi().getEntity({ id: initialDetail.id }),
    { fallbackData: initialDetail },
  );
  const [labelDraft, setLabelDraft] = useState<LabelDraft>();
  const [aliasDraft, setAliasDraft] = useState<AliasDraft>();
  const [removal, setRemoval] = useState<Removal>();
  const [saving, setSaving] = useState(false);
  const detail = entityState.data ?? initialDetail;
  const canWrite = session.isAuthenticated && detail.status === "active";
  const primaryCount = detail.labels.filter((item) => item.isPrimary).length;

  const addLabel = async () => {
    if (!labelDraft) return;
    const parsed = labelSchema.safeParse(labelDraft);
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查名称");
      return;
    }
    setSaving(true);
    try {
      await knowledgeApi().addEntityLabel({
        id: detail.id,
        addEntityLabelRequest: {
          language: parsed.data.language,
          label: parsed.data.label,
          description: parsed.data.description,
          isPrimary: parsed.data.isPrimary,
        },
      });
      await entityState.mutate();
      setLabelDraft(undefined);
      toast.success("实体名称已添加", {
        description: "搜索结果和动态组件会自动更新。",
      });
    } catch (error) {
      toast.error(mutationMessage(error, "名称添加失败"));
    } finally {
      setSaving(false);
    }
  };

  const addAlias = async () => {
    if (!aliasDraft) return;
    const parsed = aliasSchema.safeParse(aliasDraft);
    if (!parsed.success) {
      toast.error(parsed.error.issues[0]?.message ?? "请检查别名");
      return;
    }
    setSaving(true);
    try {
      await knowledgeApi().addEntityAlias({
        id: detail.id,
        addEntityAliasRequest: {
          language: parsed.data.language,
          alias: parsed.data.alias,
          aliasType: parsed.data.aliasType,
        },
      });
      await entityState.mutate();
      setAliasDraft(undefined);
      toast.success("实体别名已添加");
    } catch (error) {
      toast.error(mutationMessage(error, "别名添加失败"));
    } finally {
      setSaving(false);
    }
  };

  const setPrimary = async (value: EntityLabel) => {
    setSaving(true);
    try {
      await knowledgeApi().setPrimaryEntityLabel({
        id: detail.id,
        setPrimaryEntityLabelRequest: {
          language: value.language,
          label: value.label,
        },
      });
      await entityState.mutate();
      toast.success(`“${value.label}”已设为 ${value.language} 主名称`);
    } catch (error) {
      toast.error(mutationMessage(error, "主名称更新失败"));
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!removal) return;
    setSaving(true);
    try {
      if (removal.kind === "label") {
        await knowledgeApi().removeEntityLabel({
          id: detail.id,
          language: removal.value.language,
          label: removal.value.label,
        });
      } else {
        await knowledgeApi().removeEntityAlias({
          id: detail.id,
          aliasId: removal.value.id,
        });
      }
      await entityState.mutate();
      toast.success(removal.kind === "label" ? "名称已移除" : "别名已移除");
      setRemoval(undefined);
    } catch (error) {
      toast.error(mutationMessage(error, "名称移除失败"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-4 rounded-xl border bg-muted/20 p-4">
        <p className="flex items-center gap-2 text-sm font-semibold">
          <Languages className="size-4 text-fuchsia-700" aria-hidden />
          多语言名称
        </p>
        {canWrite ? (
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() =>
                setAliasDraft({
                  language: "zh-Hans",
                  alias: "",
                  aliasType: "common",
                })
              }
            >
              <Tags aria-hidden />
              添加别名
            </Button>
            <Button
              type="button"
              size="sm"
              onClick={() =>
                setLabelDraft({
                  language: "zh-Hans",
                  label: "",
                  description: "",
                  isPrimary: false,
                })
              }
            >
              <Plus aria-hidden />
              添加名称
            </Button>
          </div>
        ) : !session.isAuthenticated ? (
          <Button asChild size="sm">
            <Link href={LOGIN_PATH}>登录后编辑</Link>
          </Button>
        ) : null}
      </div>

      {detail.status !== "active" ? (
        <div className="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50/60 p-4 text-xs leading-5 text-amber-950/75">
          <TriangleAlert className="mt-0.5 size-4 shrink-0 text-amber-700" aria-hidden />
          已合并或删除的实体无法编辑，请前往有效实体修改名称。
        </div>
      ) : null}

      <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
        <div className="overflow-hidden rounded-xl border">
          <div className="border-b bg-muted/25 px-4 py-3">
            <p className="text-xs font-semibold">名称 · {detail.labels.length}</p>
          </div>
          <ul className="divide-y">
            {detail.labels.map((item) => (
              <li
                key={`${item.language}:${item.label}`}
                className="flex items-start justify-between gap-3 px-4 py-3"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{item.label}</span>
                    <span className="rounded-full bg-muted px-2 py-0.5 font-mono text-[9px] text-muted-foreground">
                      {item.language}
                    </span>
                    {item.isPrimary ? (
                      <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 text-[9px] font-semibold text-amber-800">
                        <Star className="size-2.5 fill-current" aria-hidden />
                        主名称
                      </span>
                    ) : null}
                  </div>
                  {item.description ? (
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {item.description}
                    </p>
                  ) : null}
                </div>
                {canWrite ? (
                  <div className="flex shrink-0">
                    {!item.isPrimary ? (
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        disabled={saving}
                        aria-label={`设“${item.label}”为主名称`}
                        onClick={() => void setPrimary(item)}
                      >
                        <Star aria-hidden />
                      </Button>
                    ) : null}
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      disabled={saving || (item.isPrimary && primaryCount <= 1)}
                      className="text-destructive hover:text-destructive"
                      aria-label={`删除名称“${item.label}”`}
                      onClick={() => setRemoval({ kind: "label", value: item })}
                    >
                      <Trash2 aria-hidden />
                    </Button>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        </div>

        <div className="overflow-hidden rounded-xl border">
          <div className="border-b bg-muted/25 px-4 py-3">
            <p className="text-xs font-semibold">别名 · {detail.aliases.length}</p>
          </div>
          {detail.aliases.length ? (
            <ul className="divide-y">
              {detail.aliases.map((item) => (
                <li
                  key={item.id}
                  className="flex items-center justify-between gap-3 px-4 py-3"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{item.alias}</p>
                    <p className="mt-1 text-[10px] text-muted-foreground">
                      {item.language} · {ALIAS_TYPE_LABEL[item.aliasType]}
                    </p>
                  </div>
                  {canWrite ? (
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      disabled={saving}
                      className="shrink-0 text-destructive hover:text-destructive"
                      aria-label={`删除别名“${item.alias}”`}
                      onClick={() => setRemoval({ kind: "alias", value: item })}
                    >
                      <Trash2 aria-hidden />
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : (
            <div className="px-4 py-9 text-center text-xs text-muted-foreground">
              还没有别名，也可以通过实体标识搜索。
            </div>
          )}
        </div>
      </div>

      <Dialog
        open={Boolean(labelDraft)}
        onOpenChange={(open) => {
          if (!open && !saving) setLabelDraft(undefined);
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>添加多语言名称</DialogTitle>
            <DialogDescription>
              名称保留大小写，会自动整理空格和字符格式。每种语言只能有一个主名称。
            </DialogDescription>
          </DialogHeader>
          {labelDraft ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="entity-label-language">语言</Label>
                <Input
                  id="entity-label-language"
                  value={labelDraft.language}
                  onChange={(event) =>
                    setLabelDraft({ ...labelDraft, language: event.target.value })
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="entity-label-value">名称</Label>
                <Input
                  id="entity-label-value"
                  autoFocus
                  value={labelDraft.label}
                  onChange={(event) =>
                    setLabelDraft({ ...labelDraft, label: event.target.value })
                  }
                />
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="entity-label-description">简短描述</Label>
                <Textarea
                  id="entity-label-description"
                  value={labelDraft.description}
                  onChange={(event) =>
                    setLabelDraft({
                      ...labelDraft,
                      description: event.target.value,
                    })
                  }
                  placeholder="用于消歧与搜索摘要"
                />
              </div>
              <label className="flex cursor-pointer items-start gap-3 rounded-xl border p-3 text-xs leading-5 sm:col-span-2">
                <Checkbox
                  checked={labelDraft.isPrimary}
                  onCheckedChange={(value) =>
                    setLabelDraft({ ...labelDraft, isPrimary: value === true })
                  }
                  className="mt-0.5"
                />
                <span>
                  设为该语言的主名称。如果已有主名称，请先添加，再在名称列表中切换。
                </span>
              </label>
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => setLabelDraft(undefined)}
            >
              取消
            </Button>
            <Button
              type="button"
              disabled={saving}
              onClick={() => void addLabel()}
            >
              {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : <Plus aria-hidden />}
              {saving ? "保存中…" : "添加名称"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(aliasDraft)}
        onOpenChange={(open) => {
          if (!open && !saving) setAliasDraft(undefined);
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>添加实体别名</DialogTitle>
            <DialogDescription>
              添加别名后，可以用更多名称找到这个实体。不会因此新建或合并实体。
            </DialogDescription>
          </DialogHeader>
          {aliasDraft ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="entity-alias-language">语言</Label>
                <Input
                  id="entity-alias-language"
                  value={aliasDraft.language}
                  onChange={(event) =>
                    setAliasDraft({ ...aliasDraft, language: event.target.value })
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="entity-alias-type">类型</Label>
                <select
                  id="entity-alias-type"
                  value={aliasDraft.aliasType}
                  onChange={(event) =>
                    setAliasDraft({
                      ...aliasDraft,
                      aliasType: event.target.value as AliasDraft["aliasType"],
                    })
                  }
                  className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                >
                  {Object.entries(ALIAS_TYPE_LABEL).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="entity-alias-value">别名</Label>
                <Input
                  id="entity-alias-value"
                  autoFocus
                  value={aliasDraft.alias}
                  onChange={(event) =>
                    setAliasDraft({ ...aliasDraft, alias: event.target.value })
                  }
                />
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => setAliasDraft(undefined)}
            >
              取消
            </Button>
            <Button
              type="button"
              disabled={saving}
              onClick={() => void addAlias()}
            >
              {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : <Tags aria-hidden />}
              {saving ? "保存中…" : "添加别名"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(removal)}
        onOpenChange={(open) => {
          if (!open && !saving) setRemoval(undefined);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>移除这个名称？</DialogTitle>
            <DialogDescription>
              {removal?.kind === "label"
                ? `名称“${removal.value.label}”将不再用于展示与搜索。`
                : removal
                  ? `别名“${removal.value.alias}”将不再用于检索。`
                  : ""}
              审计记录会永久保留。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => setRemoval(undefined)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={saving}
              onClick={() => void remove()}
            >
              {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : <Trash2 aria-hidden />}
              {saving ? "移除中…" : "确认移除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
