"use client";

import Link from "next/link";
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  CircleDot,
  Code2,
  Component,
  Copy,
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
import { z } from "zod";

import type {
  WikiComponent,
  WikiComponentVersion,
  WikiComponentVersionList,
  WriteWikiComponentVersionRequest,
} from "../../../../contracts/generated/typescript";

import { Button } from "@/components/ui/button";
import { ComponentUsagePanel } from "@/components/projection/usage-panels";
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
import { componentsApi } from "@/lib/api";
import { useSession } from "@/lib/auth";
import { cn } from "@/lib/utils";

type RendererRef = WriteWikiComponentVersionRequest["rendererRef"];

const jsonObjectSchema = z.record(z.string(), z.unknown());

const DEFAULT_SCHEMA: Record<RendererRef, Record<string, unknown>> = {
  "builtin.key_value": {
    type: "object",
    additionalProperties: true,
  },
  "builtin.entity_claim_infobox": {
    type: "object",
    properties: {
      title: { type: "string" },
      language: { type: "string" },
      property_keys: {
        type: "array",
        items: { type: "string" },
        uniqueItems: true,
      },
    },
    additionalProperties: false,
  },
};

const DEFAULT_PROPS: Record<RendererRef, Record<string, unknown>> = {
  "builtin.key_value": {
    名称: "示例条目",
    类型: "可信组件",
  },
  "builtin.entity_claim_infobox": {
    title: "",
    language: "zh",
    property_keys: [],
  },
};

function parseJSONObject(source: string): Record<string, unknown> | null {
  try {
    const value: unknown = JSON.parse(source);
    const result = jsonObjectSchema.safeParse(value);
    return result.success ? result.data : null;
  } catch {
    return null;
  }
}

function VersionEditor({
  component,
  version,
  onClose,
  onSaved,
}: {
  component: WikiComponent;
  version: WikiComponentVersion | null;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [renderer, setRenderer] = useState<RendererRef>(
    version?.rendererRef ?? "builtin.entity_claim_infobox",
  );
  const [schema, setSchema] = useState(() =>
    JSON.stringify(
      version?.propsSchema ?? DEFAULT_SCHEMA["builtin.entity_claim_infobox"],
      null,
      2,
    ),
  );
  const [saving, setSaving] = useState(false);

  const switchRenderer = (next: RendererRef) => {
    setRenderer(next);
    if (!version) {
      setSchema(JSON.stringify(DEFAULT_SCHEMA[next], null, 2));
    }
  };

  const save = async () => {
    const propsSchema = parseJSONObject(schema);
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
          {version ? `编辑 v${version.version} 草稿` : "创建组件版本"}
        </DialogTitle>
        <DialogDescription>
          只有 draft 可修改；发布后 Schema 与渲染器永久冻结。
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="renderer-ref">可信渲染器</Label>
          <select
            id="renderer-ref"
            value={renderer}
            onChange={(event) => switchRenderer(event.target.value as RendererRef)}
            className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
          >
            <option value="builtin.entity_claim_infobox">
              Entity / Claim 信息框
            </option>
            <option value="builtin.key_value">通用键值卡片</option>
          </select>
        </div>
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <Label htmlFor="props-schema">Props JSON Schema</Label>
            <span className="font-mono text-[10px] text-muted-foreground">
              draft-2020-12
            </span>
          </div>
          <Textarea
            id="props-schema"
            value={schema}
            onChange={(event) => setSchema(event.target.value)}
            className="min-h-72 resize-y font-mono text-xs leading-5"
            spellCheck={false}
          />
        </div>
      </div>
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

function ComponentPreview({
  component,
  version,
  onClose,
}: {
  component: WikiComponent;
  version: WikiComponentVersion;
  onClose: () => void;
}) {
  const [props, setProps] = useState(() =>
    JSON.stringify(DEFAULT_PROPS[version.rendererRef], null, 2),
  );
  const [entityID, setEntityID] = useState("");
  const [html, setHTML] = useState("");
  const [loading, setLoading] = useState(false);

  const preview = async () => {
    const parsed = parseJSONObject(props);
    if (!parsed) {
      toast.error("预览 Props 必须是合法 JSON object");
      return;
    }
    if (
      version.rendererRef === "builtin.entity_claim_infobox" &&
      !entityID.trim()
    ) {
      toast.error("Entity 信息框预览需要 Entity ID");
      return;
    }
    setLoading(true);
    try {
      const result = await componentsApi().previewWikiComponentVersion({
        id: component.id,
        version: version.version,
        previewWikiComponentRequest: {
          props: parsed,
          entityId: entityID.trim() || undefined,
        },
      });
      setHTML(result.html);
    } catch {
      toast.error("组件预览失败", {
        description: "请检查 Props 是否符合 Schema，以及 Entity 是否存在。",
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>
          预览 {component.name} · v{version.version}
        </DialogTitle>
        <DialogDescription>
          请求由服务端 Schema 校验，并只执行白名单内置渲染器。
        </DialogDescription>
      </DialogHeader>
      <div className="grid max-h-[68vh] gap-4 overflow-y-auto lg:grid-cols-2">
        <div className="space-y-4">
          {version.rendererRef === "builtin.entity_claim_infobox" ? (
            <div className="space-y-2">
              <Label htmlFor="preview-entity">Entity ID</Label>
              <Input
                id="preview-entity"
                value={entityID}
                onChange={(event) => setEntityID(event.target.value)}
                placeholder="UUID"
                className="font-mono text-xs"
              />
            </div>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="preview-props">display_config / Props</Label>
            <Textarea
              id="preview-props"
              value={props}
              onChange={(event) => setProps(event.target.value)}
              className="min-h-60 resize-y font-mono text-xs leading-5"
              spellCheck={false}
            />
          </div>
        </div>
        <div className="min-h-64 rounded-2xl border bg-muted/20 p-4">
          <p className="mb-3 text-[11px] font-semibold tracking-[0.12em] text-muted-foreground uppercase">
            Server preview
          </p>
          {html ? (
            <div
              className="wiki-component-preview rounded-xl bg-background p-4"
              // HTML originates from a fixed server-side renderer registry; user values are escaped there.
              dangerouslySetInnerHTML={{ __html: html }}
            />
          ) : (
            <div className="flex min-h-48 items-center justify-center text-center text-xs leading-5 text-muted-foreground">
              填写示例 Props 后运行预览。
            </div>
          )}
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          关闭
        </Button>
        <Button type="button" disabled={loading} onClick={() => void preview()}>
          {loading ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <Eye aria-hidden />
          )}
          运行预览
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
  const [previewVersion, setPreviewVersion] =
    useState<WikiComponentVersion | null>(null);
  const [transitioning, setTransitioning] = useState("");
  const [copied, setCopied] = useState("");
  const { isAuthenticated, isLoading: sessionLoading } = useSession();
  const componentState = useSWR<WikiComponent>(["component", id], () =>
    componentsApi().getWikiComponent({ id }),
  );
  const versionState = useSWR<WikiComponentVersionList>(
    componentState.data ? ["component-versions", id] : null,
    () => componentsApi().listWikiComponentVersions({ id }),
  );

  if (componentState.isLoading || versionState.isLoading) {
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

  const copyBlock = async (version: WikiComponentVersion) => {
    const value = JSON.stringify(
      {
        type: "component",
        component_id: component.id,
        component_version: version.version,
        entity_id: "ENTITY_UUID",
        display_config: DEFAULT_PROPS[version.rendererRef],
      },
      null,
      2,
    );
    await navigator.clipboard.writeText(value);
    setCopied(String(version.version));
    toast.success("ComponentBlock 模板已复制");
    window.setTimeout(() => setCopied(""), 1600);
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
          <div>
            <span className="flex size-11 items-center justify-center rounded-2xl bg-violet-100 text-violet-700">
              <Component className="size-5" aria-hidden />
            </span>
            <h1 className="mt-4 text-4xl font-semibold tracking-[-0.045em]">
              {component.name}
            </h1>
            <p className="mt-2 font-mono text-xs text-muted-foreground">
              {component.componentKey}
            </p>
          </div>
          <Button
            type="button"
            disabled={!isAuthenticated || sessionLoading}
            onClick={() => {
              setEditingVersion(null);
              setEditorOpen(true);
            }}
          >
            <Plus aria-hidden />
            创建新版本
          </Button>
        </div>
        <p className="mt-4 max-w-3xl text-sm leading-6 text-muted-foreground">
          Component ID{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-[11px]">
            {component.id}
          </code>
          。页面只保存 ID、版本、Entity 与显示配置；信息框事实始终从 Claim
          权威模型读取。
        </p>
      </header>

      <section className="mt-8" aria-labelledby="component-version-title">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div>
            <h2 id="component-version-title" className="text-lg font-semibold">
              版本时间线
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              新版本不会改写旧页面；每个已发布版本永久可重现。
            </p>
          </div>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <LockKeyhole className="size-3.5" aria-hidden />
            发布即冻结
          </span>
        </div>

        {versions.length === 0 ? (
          <div className="rounded-2xl border border-dashed px-6 py-14 text-center">
            <FileJson2 className="mx-auto size-8 text-muted-foreground" aria-hidden />
            <h3 className="mt-4 font-semibold">还没有组件版本</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              创建 Props Schema，预览后再发布第一个版本。
            </p>
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
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => void copyBlock(version)}
                      >
                        {copied === String(version.version) ? (
                          <Check aria-hidden />
                        ) : (
                          <Copy aria-hidden />
                        )}
                        Block
                      </Button>
                      {version.status === "draft" ? (
                        <>
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            disabled={!isAuthenticated}
                            onClick={() => {
                              setEditingVersion(version);
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
                              !isAuthenticated ||
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
                      {version.status === "published" ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="destructive"
                          disabled={
                            !isAuthenticated ||
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

      <aside className="mt-8 grid gap-4 md:grid-cols-2">
        <div className="rounded-2xl border bg-muted/25 p-5">
          <Code2 className="size-4 text-violet-700" aria-hidden />
          <h2 className="mt-3 text-sm font-semibold">可信执行边界</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            数据库只保存 renderer_ref，绝不加载用户脚本或模板；实际代码来自进程内白名单。
          </p>
        </div>
        <div className="rounded-2xl border bg-muted/25 p-5">
          <LockKeyhole className="size-4 text-violet-700" aria-hidden />
          <h2 className="mt-3 text-sm font-semibold">可重现渲染</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            ComponentBlock 锁定组件版本，ClaimUsage 与 ComponentDependency
            负责精确失效和重建。
          </p>
        </div>
      </aside>

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
          {editorOpen ? (
            <VersionEditor
              key={editingVersion?.version ?? "new"}
              component={component}
              version={editingVersion}
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
            <ComponentPreview
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
