"use client";

import { LoaderCircle, PanelTop } from "lucide-react";
import useSWR from "swr";

import { Button } from "@/components/ui/button";
import { componentsApi, knowledgeApi } from "@/lib/api";

export interface InfoboxSelection {
  componentId: string;
  componentVersion: number;
  entityId: string;
  language: string;
}

export function InfoboxInsertButton({
  pageId,
  onInsert,
}: {
  pageId: string;
  onInsert: (selection: InfoboxSelection) => void;
}) {
  const bindings = useSWR(["editor:page-bindings", pageId], () =>
    knowledgeApi().listPageEntityBindings({ id: pageId }),
  );
  const components = useSWR(["editor:components", "article-infobox"], () =>
    componentsApi().listWikiComponents({ pageSize: 100 }),
  );
  const primary = bindings.data?.items.find((binding) => binding.role === "primary");
  const infobox = components.data?.items.find(
    (component) => component.componentKey === "article-infobox",
  );
  const versions = useSWR(
    infobox ? ["editor:component-versions", infobox.id] : null,
    () => componentsApi().listWikiComponentVersions({ id: infobox!.id }),
  );
  const publishedVersion = versions.data?.items
    .filter((version) => version.status === "published")
    .sort((left, right) => right.version - left.version)[0];
  const loading = bindings.isLoading || components.isLoading || versions.isLoading;
  const unavailable = bindings.error || components.error || versions.error;
  const title = unavailable
    ? "无法读取页面实体或信息框组件"
    : !primary
      ? "请先在实体中心为页面绑定主实体"
      : !infobox || !publishedVersion
        ? "没有可用的已发布文章信息框组件"
        : "在文首插入由主实体 Claim 驱动的信息框";

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={loading || Boolean(unavailable) || !primary || !publishedVersion}
      title={title}
      onClick={() => {
        if (!primary || !publishedVersion) return;
        onInsert({
          componentId: publishedVersion.componentId,
          componentVersion: publishedVersion.version,
          entityId: primary.entityId,
          language: primary.language,
        });
      }}
    >
      {loading ? (
        <LoaderCircle className="size-4 animate-spin" aria-hidden />
      ) : (
        <PanelTop className="size-4" aria-hidden />
      )}
      信息框
    </Button>
  );
}
