"use client";

import { Copy, Eye, LoaderCircle, Plus } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { z } from "zod";

import type {
  WikiComponent,
  WikiComponentVersion,
} from "../../../../contracts/generated/typescript";
import {
  ComponentEntityPicker,
  ComponentPropsForm,
} from "@/components/components/component-forms";
import { Button } from "@/components/ui/button";
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { componentsApi } from "@/lib/api";
import {
  COMPONENT_RENDERERS,
  componentDefaultProps,
  parseComponentObject,
} from "@/lib/components";

export interface ComponentSelection {
  componentId: string;
  componentVersion: number;
  entityId: string;
  displayConfig: Record<string, unknown>;
}

export function ComponentConfigure({
  component,
  version,
  initialEntityId = "",
  initialLanguage,
  onClose,
  onInsert,
}: {
  component: WikiComponent;
  version: WikiComponentVersion;
  initialEntityId?: string;
  initialLanguage?: string;
  onClose: () => void;
  onInsert?: (selection: ComponentSelection) => void;
}) {
  const [source, setSource] = useState(() => {
    const defaults = componentDefaultProps(
      version.propsSchema,
      version.rendererRef,
    );
    if (
      initialLanguage &&
      defaults.language === undefined &&
      version.rendererRef === COMPONENT_RENDERERS.BuiltinEntityClaimInfobox
    )
      defaults.language = initialLanguage;
    return JSON.stringify(defaults, null, 2);
  });
  const [entityId, setEntityId] = useState(initialEntityId);
  const [preview, setPreview] = useState<{
    html: string;
    source: string;
    entityId: string;
  } | null>(null);
  const [loading, setLoading] = useState(false);
  const dynamic =
    version.rendererRef === COMPONENT_RENDERERS.BuiltinEntityClaimInfobox;
  const selection = (): ComponentSelection | null => {
    const displayConfig = parseComponentObject(source);
    if (!displayConfig) {
      toast.error("显示配置必须是合法 JSON 对象");
      return null;
    }
    if (!z.uuid().safeParse(entityId).success) {
      toast.error("请选择关联实体");
      return null;
    }
    return {
      componentId: component.id,
      componentVersion: version.version,
      entityId,
      displayConfig,
    };
  };
  const runPreview = async () => {
    const props = parseComponentObject(source);
    if (!props || (dynamic && !z.uuid().safeParse(entityId).success)) {
      toast.error(
        dynamic ? "请选择实体并填写有效配置" : "显示配置必须是合法 JSON 对象",
      );
      return;
    }
    setLoading(true);
    try {
      const result = await componentsApi().previewWikiComponentVersion({
        id: component.id,
        version: version.version,
        previewWikiComponentRequest: { props, entityId: entityId || undefined },
      });
      setPreview({ html: result.html, source, entityId });
    } catch {
      setPreview(null);
      toast.error("组件预览失败", {
        description: "请检查字段配置及实体是否仍然可用。",
      });
    } finally {
      setLoading(false);
    }
  };
  const currentPreview =
    preview?.source === source && preview.entityId === entityId;
  return (
    <>
      <DialogHeader>
        <DialogTitle>
          {onInsert ? "插入" : "预览"} {component.name} · v{version.version}
        </DialogTitle>
        <DialogDescription>
          {dynamic
            ? "选择实体并调整显示范围，查看实际已发布事实。"
            : "填写资料卡内容，运行预览查看实际效果。插入时需关联一个实体。"}
          使用此版本的页面会保留版本号。
        </DialogDescription>
      </DialogHeader>
      <div className="grid max-h-[65vh] min-w-0 gap-5 overflow-y-auto lg:grid-cols-2">
        <div className="min-w-0 space-y-4">
          <ComponentPropsForm
            schema={version.propsSchema}
            renderer={version.rendererRef}
            source={source}
            onChange={setSource}
          />
          <ComponentEntityPicker value={entityId} onChange={setEntityId} />
        </div>
        <div className="min-w-0 rounded-xl border bg-muted/20 p-4 lg:self-start">
          <p className="mb-3 text-xs font-medium text-muted-foreground">
            实际渲染效果
            {preview && !currentPreview ? " · 配置已修改，请重新预览" : ""}
          </p>
          {preview ? (
            <div
              className="wiki-rendered wiki-component-preview"
              // Fixed server renderers escape all user values.
              dangerouslySetInnerHTML={{ __html: preview.html }}
            />
          ) : (
            <p className="py-16 text-center text-xs leading-5 text-muted-foreground">
              填写内容后点击「运行预览」。
            </p>
          )}
        </div>
      </div>
      <DialogFooter className="flex-wrap">
        <Button type="button" variant="outline" onClick={onClose}>
          关闭
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={loading}
          onClick={() => void runPreview()}
        >
          {loading ? (
            <LoaderCircle className="animate-spin" aria-hidden />
          ) : (
            <Eye aria-hidden />
          )}
          运行预览
        </Button>
        {version.status === "published" ? (
          <Button
            type="button"
            disabled={loading || !currentPreview || !entityId}
            onClick={async () => {
              const value = selection();
              if (!value) return;
              if (onInsert) {
                onInsert(value);
                onClose();
              } else {
                try {
                  await navigator.clipboard.writeText(
                    JSON.stringify(
                      {
                        type: "component",
                        component_id: value.componentId,
                        component_version: value.componentVersion,
                        entity_id: value.entityId,
                        display_config: value.displayConfig,
                      },
                      null,
                      2,
                    ),
                  );
                  toast.success("组件区块配置已复制");
                } catch {
                  toast.error("复制失败");
                }
              }
            }}
          >
            {onInsert ? <Plus aria-hidden /> : <Copy aria-hidden />}
            {onInsert ? "插入组件" : "复制区块配置"}
          </Button>
        ) : null}
      </DialogFooter>
    </>
  );
}
