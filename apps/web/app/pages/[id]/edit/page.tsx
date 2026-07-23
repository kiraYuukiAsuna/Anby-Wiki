// /pages/[id]/edit：编辑既有页面（M2-T03）。
//
// 数据边界：服务端组件经 readingApi 注入页面当前内容作为编辑会话初始值；
// 会话状态（Zustand）与草稿恢复都在客户端（SWR 不介入编辑路径——阅读
// 路径才有「服务端数据客户端缓存」需求，编辑会话是一次性初始注入 + 本地状态）。
import { notFound } from "next/navigation";

import { EditorSessionView } from "@/components/editor/editor-session-view";
import { parseDocument, type Document } from "@/lib/ast/schema";
import { fetchPageById } from "@/lib/reading";

export const dynamic = "force-dynamic";

/** 未发布页面（content=null）的空文档（M2-T02：BlockEditor 对空 children 降级为单空段落）。 */
const EMPTY_DOCUMENT: Document = {
  type: "document",
  schema_version: 1,
  children: [],
};

export default async function PageEditPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const result = await fetchPageById(id);

  if (result.kind === "not_found") {
    notFound();
  }
  if (result.kind === "gone") {
    return (
      <div className="mx-auto w-full max-w-3xl px-4 py-16 text-center">
        <h1 className="text-2xl font-semibold">页面已删除</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          该页面已被删除，无法编辑。
        </p>
      </div>
    );
  }

  const { page, content } = result.data;
  const ast = content ? parseDocument(content.astJson) : EMPTY_DOCUMENT;

  return (
    <EditorSessionView
      pageId={page.id}
      displayTitle={page.displayTitle}
      baseRevisionId={page.currentRevisionId ?? null}
      initialAst={ast}
    />
  );
}
