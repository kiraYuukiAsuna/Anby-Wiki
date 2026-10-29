"use client";

import Link from "next/link";
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  CircleDot,
  Component,
  Eye,
  FileJson2,
  LoaderCircle,
  LockKeyhole,
  Pencil,
  Plus,
  Rocket,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import useSWR from "swr";

import type {
  WikiComponent,
  WikiComponentVersion,
  WikiComponentVersionList,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { ComponentDefinitionForm } from "@/components/components/component-forms";
import { ComponentConfigure } from "@/components/components/component-configure";
import { ComponentUsagePanel } from "@/components/projection/usage-panels";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { componentsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { COMPONENT_PRESETS, parseComponentObject, type ComponentRenderer } from "@/lib/components";
import { cn } from "@/lib/utils";

function VersionEditor({
  component,
  version,
  copyFrom,
  onClose,
  onSaved,
}: {
  component: WikiComponent;
  version: WikiComponentVersion | null;
  copyFrom: WikiComponentVersion | null;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [renderer, setRenderer] = useState<ComponentRenderer>(
    version?.rendererRef ?? copyFrom?.rendererRef ?? COMPONENT_PRESETS[0].renderer,
  );
  const [schema, setSchema] = useState(() =>
    JSON.stringify(
      version?.propsSchema ?? copyFrom?.propsSchema ?? COMPONENT_PRESETS[0].schema,
      null,
      2,
    ),
  );
  const [saving, setSaving] = useState(false);

  const save = async () => {
    const propsSchema = parseComponentObject(schema);
    if (!propsSchema || propsSchema.type !== "object") {
      toast.error("Props Schema 必须是根 type=object 的合法 JSON");
      return;
    }
    setSaving(true);
    try {
      if (version) {
        await componentsApi().updateWikiComponentVersion({
          id: component.id,
          version: version.version,
          writeWikiComponentVersionRequest: {
            propsSchema,
            rendererRef: renderer,
          },
        });
      } else {
        await componentsApi().createWikiComponentVersion({
          id: component.id,
          writeWikiComponentVersionRequest: {
            propsSchema,
            rendererRef: renderer,
          },
        });
      }
      await onSaved();
      toast.success(version ? "草稿已更新" : "新版本草稿已创建");
      onClose();
    } catch {
      toast.error(version ? "更新草稿失败" : "创建版本失败", {
        description: "请检查 JSON Schema 与渲染器约束。",
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>
          {version ? `编辑 v${version.version} 草稿` : copyFrom ? `基于 v${copyFrom.version} 定制新版本` : "创建组件版本"}
        </DialogTitle>
        <DialogDescription>
          保存为草稿，预览确认后再发布。已有页面继续使用其原版本。
        </DialogDescription>
      </DialogHeader>
      <fieldset disabled={saving} className="max-h-[65vh] min-w-0 overflow-y-auto">
        <ComponentDefinitionForm renderer={renderer} source={schema} onChange={(next, value) => { setRenderer(next); setSchema(value); }} />
      </fieldset>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          取消
        </Button>
        <Button type="button" disabled={saving} onClick={() => void save()}>
          {saving ? <LoaderCircle className="animate-spin" aria-hidden /> : null}
          保存草稿
        </Button>
      </DialogFooter>
    </>
  );
}

const STATUS_META = {
  draft: {
    label: "草稿",
    className: "border-amber-300 bg-amber-50 text-amber-800",
    icon: Pencil,
  },
  published: {
    label: "已发布",
    className: "border-emerald-300 bg-emerald-50 text-emerald-800",
    icon: Check,
  },
  deprecated: {
    label: "已废弃",
    className: "border-border bg-muted text-muted-foreground",
    icon: AlertTriangle,
  },
} as const;

export function ComponentWorkspace({ id }: { id: string }) {
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingVersion, setEditingVersion] =
    useState<WikiComponentVersion | null>(null);
  const [copyFrom, setCopyFrom] = useState<WikiComponentVersion | null>(null);
  const [previewVersion, setPreviewVersion] =
    useState<WikiComponentVersion | null>(null);
  const [transitioning, setTransitioning] = useState("");
  const { session, isLoading: sessionLoading } = useSession();
  const canWrite = Boolean(session && (session.actorType === "system" || session.homeWikiId === session.wikiId));
  const componentState = useSWR<WikiComponent>(sessionLoading ? null : ["component", session?.actorId, session?.wikiId, id], () =>
    componentsApi().getWikiComponent({ id }),
  );
  const versionState = useSWR<WikiComponentVersionList>(
    componentState.data ? ["component-versions", session?.actorId, session?.wikiId, id] : null,
    () => componentsApi().listWikiComponentVersions({ id }),
  );

  if (sessionLoading || componentState.isLoading || versionState.isLoading) {
    return <div className="h-72 animate-pulse rounded-3xl border bg-muted/35" />;
  }
  if (
    componentState.error ||
    versionState.error ||
    !componentState.data ||
    !versionState.data
  ) {
    return (
      <div className="rounded-3xl border border-destructive/20 bg-destructive/5 p-8">
        <h1 className="text-xl font-semibold">组件无法打开</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          它可能不存在，或当前 API 暂时不可用。
        </p>
        <Button asChild variant="outline" className="mt-5">
          <Link href="/components">返回组件中心</Link>
        </Button>
      </div>
    );
  }

  const component = componentState.data;
  const versions = versionState.data.items;

  const transition = async (
    version: WikiComponentVersion,
    action: "publish" | "deprecate",
  ) => {
    const key = `${version.version}:${action}`;
    setTransitioning(key);
    try {
      if (action === "publish") {
        await componentsApi().publishWikiComponentVersion({
          id: component.id,
          version: version.version,
        });
      } else {
        await componentsApi().deprecateWikiComponentVersion({
          id: component.id,
          version: version.version,
        });
      }
      await versionState.mutate();
      toast.success(action === "publish" ? "版本已发布并冻结" : "版本已废弃");
    } catch {
      toast.error(action === "publish" ? "发布失败" : "废弃失败");
    } finally {
      setTransitioning("");
    }
  };

  return (
    <>
      <header className="border-b border-border/75 pb-7">
        <Button variant="ghost" size="sm" asChild className="-ml-2 mb-5">
          <Link href="/components">
            <ArrowLeft aria-hidden />
            组件中心
          </Link>
        </Button>
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div className="min-w-0">
            <span className="flex size-11 items-center justify-center rounded-2xl bg-violet-100 text-violet-700">
              <Component className="size-5" aria-hidden />
            </span>
            <h1 className="mt-4 break-words text-4xl font-semibold tracking-[-0.045em]">
              {component.name}
            </h1>
            <p className="mt-2 break-all font-mono text-xs text-muted-foreground">
              {component.componentKey}
            </p>
          </div>
          {canWrite ? <Button
            type="button"
            onClick={() => {
              setEditingVersion(null);
              setCopyFrom(versions.find((version) => version.status === "published") ?? versions[0] ?? null);
              setEditorOpen(true);
            }}
          >
            <Plus aria-hidden />
            创建新版本
          </Button> : null}
        </div>
      </header>

      <section className="mt-8" aria-labelledby="component-version-title">
        <div className="mb-4 flex items-center justify-between gap-3">
          <h2 id="component-version-title" className="text-lg font-semibold">
            版本
          </h2>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <LockKeyhole className="size-3.5" aria-hidden />
            发布即冻结
          </span>
        </div>

        {versions.length === 0 ? (
          <div className="rounded-2xl border border-dashed px-6 py-14 text-center">
            <FileJson2 className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">还没有组件版本</h3>
          </div>
        ) : (
          <ol className="space-y-4">
            {versions.map((version) => {
              const status = STATUS_META[version.status];
              const StatusIcon = status.icon;
              const busyPrefix = `${version.version}:`;
              return (
                <li
                  key={version.version}
                  className="rounded-2xl border bg-card p-5 shadow-[0_1px_0_rgb(15_23_42/0.03)]"
                >
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div className="flex items-start gap-3">
                      <span className="flex size-10 items-center justify-center rounded-xl bg-muted/60 font-mono text-sm font-semibold">
                        v{version.version}
                      </span>
                      <div>
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="font-semibold">
                            {version.rendererRef ===
                            "builtin.entity_claim_infobox"
                              ? "Entity / Claim 信息框"
                              : "通用键值卡片"}
                          </h3>
                          <span
                            className={cn(
                              "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-semibold",
                              status.className,
                            )}
                          >
                            <StatusIcon className="size-3" aria-hidden />
                            {status.label}
                          </span>
                        </div>
                        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
                          {version.rendererRef}
                        </p>
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => setPreviewVersion(version)}
                      >
                        <Eye aria-hidden />
                        预览
                      </Button>
                      {canWrite && version.status !== "draft" ? <Button type="button" size="sm" variant="outline" onClick={() => {
                        setEditingVersion(null);
                        setCopyFrom(version);
                        setEditorOpen(true);
                      }}><Plus aria-hidden />定制新版本</Button> : null}
                      {canWrite && version.status === "draft" ? (
                        <>
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              setEditingVersion(version);
                              setCopyFrom(null);
                              setEditorOpen(true);
                            }}
                          >
                            <Pencil aria-hidden />
                            编辑
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            disabled={
                              transitioning.startsWith(busyPrefix)
                            }
                            onClick={() => void transition(version, "publish")}
                          >
                            {transitioning === `${version.version}:publish` ? (
                              <LoaderCircle className="animate-spin" aria-hidden />
                            ) : (
                              <Rocket aria-hidden />
                            )}
                            发布
                          </Button>
                        </>
                      ) : null}
                      {canWrite && version.status === "published" ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="destructive"
                          disabled={
                            transitioning.startsWith(busyPrefix)
                          }
                          onClick={() => void transition(version, "deprecate")}
                        >
                          {transitioning === `${version.version}:deprecate` ? (
                            <LoaderCircle className="animate-spin" aria-hidden />
                          ) : (
                            <CircleDot aria-hidden />
                          )}
                          废弃
                        </Button>
                      ) : null}
                    </div>
                  </div>
                  <details className="mt-4 rounded-xl border bg-muted/20">
                    <summary className="cursor-pointer px-4 py-3 text-xs font-medium">
                      查看 Props Schema
                    </summary>
                    <pre className="overflow-x-auto border-t p-4 text-[11px] leading-5">
                      {JSON.stringify(version.propsSchema, null, 2)}
                    </pre>
                  </details>
                </li>
              );
            })}
          </ol>
        )}
      </section>

      <ComponentUsagePanel
        componentId={component.id}
        versions={versions.map((version) => version.version)}
      />

      <Dialog
        open={editorOpen}
        onOpenChange={(open) => {
          setEditorOpen(open);
          if (!open) setEditingVersion(null);
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          {editorOpen && canWrite ? (
            <VersionEditor
              key={editingVersion?.version ?? "new"}
              component={component}
              version={editingVersion}
              copyFrom={copyFrom}
              onClose={() => setEditorOpen(false)}
              onSaved={async () => {
                await versionState.mutate();
              }}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(previewVersion)}
        onOpenChange={(open) => {
          if (!open) setPreviewVersion(null);
        }}
      >
        <DialogContent className="sm:max-w-4xl">
          {previewVersion ? (
            <ComponentConfigure
              key={previewVersion.version}
              component={component}
              version={previewVersion}
              onClose={() => setPreviewVersion(null)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}
