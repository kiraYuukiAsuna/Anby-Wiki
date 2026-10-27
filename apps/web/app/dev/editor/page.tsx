// M2-T02 手工验证页（仅 development 可见，生产构建直接 404）。
// 左侧为 BlockEditor（受控：初始 AST + onChange(AST)），右侧实时显示序列化 AST。
"use client";

import { useState } from "react";

import dynamic from "next/dynamic";
import { notFound } from "next/navigation";

import { AstDocument } from "@/components/ast";
import { PageButtonRuntime } from "@/components/page-button-runtime";
import { parseDocument, type Document } from "@/lib/ast/schema";

const BlockEditor = dynamic(
  () =>
    import("@/components/editor/block-editor").then(
      (module) => module.BlockEditor,
    ),
  {
    ssr: false,
    loading: () => (
      <div className="min-h-80 animate-pulse bg-muted" aria-label="编辑器加载中" />
    ),
  },
);

const SAMPLE_AST: Document = parseDocument({
  type: "document",
  schema_version: 1,
  children: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      type: "heading",
      level: 1,
      is_toggleable: true,
      content: [{ type: "text", text: "编辑器手工验证", marks: ["bold"] }],
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a10",
          type: "paragraph",
          content: [{ type: "text", text: "可折叠标题正文" }],
        },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02",
      type: "paragraph",
      content: [
        { type: "text", text: "含 " },
        { type: "inline_code", text: "inline_code" },
        { type: "text", text: "、" },
        {
          type: "page_reference",
          target_page_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
          display_text: "已解析页面",
        },
        { type: "text", text: "、" },
        {
          type: "page_reference",
          resolution_status: "unresolved",
          target_namespace: "Main",
          normalized_title: "wei-lai-de-ye-mian",
        },
        { type: "text", text: " 和 " },
        {
          type: "external_link",
          url: "https://example.com/spec",
          display_text: "外部链接",
        },
        { type: "text", text: "；" },
        {
          type: "text",
          text: "带下划线的彩色文本",
          marks: ["bold", "underline"],
          text_color: "red",
          background_color: "yellow",
        },
        { type: "text", text: "；" },
        {
          type: "mention",
          actor_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b02",
          display_text: "Anby 编辑者",
        },
        { type: "text", text: "；" },
        {
          type: "date_mention",
          date: "2026-10-20",
          end_date: "2026-10-22",
          display_text: "2026-10-20 至 2026-10-22",
        },
        { type: "text", text: "。" },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a03",
      type: "bullet_list",
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a04",
          type: "list_item",
          children: [
            {
              id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a05",
              type: "paragraph",
              content: [{ type: "text", text: "列表项一" }],
            },
          ],
        },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a06",
      type: "callout",
      kind: "warning",
      icon: "!",
      color: "orange",
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a07",
          type: "paragraph",
          content: [{ type: "text", text: "注意！" }],
        },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a08",
      type: "code",
      language: "go",
      caption: "Go 示例",
      content: "package main\n",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a11",
      type: "todo",
      checked: true,
      content: [{ type: "text", text: "已完成事项" }],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a12",
      type: "toggle",
      content: [{ type: "text", text: "展开详情" }],
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a13",
          type: "paragraph",
          content: [{ type: "text", text: "Toggle 正文" }],
        },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a14",
      type: "table",
      caption: "语义表格",
      has_column_header: true,
      has_row_header: true,
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a15",
          type: "table_row",
          children: [
            {
              id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a16",
              type: "table_cell",
              children: [
                {
                  id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a17",
                  type: "paragraph",
                  content: [{ type: "text", text: "名称" }],
                },
              ],
            },
            {
              id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a18",
              type: "table_cell",
              children: [
                {
                  id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a19",
                  type: "paragraph",
                  content: [{ type: "text", text: "值" }],
                },
              ],
            },
          ],
        },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a20",
      type: "preformatted",
      kind: "ascii_diagram",
      caption: "ASCII 图",
      content: "+---+\n| A |\n+---+",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a21",
      type: "equation",
      expression: "E = mc^2",
      caption: "质能方程",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a22",
      type: "breadcrumb",
      items: [
        {
          target_page_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01",
          display_text: "父页面",
        },
        { display_text: "当前页面" },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a23",
      type: "column_list",
      children: [
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a24",
          type: "column",
          width_ratio: 1,
          children: [
            {
              id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a25",
              type: "paragraph",
              content: [{ type: "text", text: "窄列" }],
            },
          ],
        },
        {
          id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a26",
          type: "column",
          width_ratio: 2,
          children: [
            {
              id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a27",
              type: "paragraph",
              content: [{ type: "text", text: "宽列" }],
            },
          ],
        },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a28",
      type: "button",
      label: "运行操作",
      confirmation: "确定执行示例动作吗？",
      actions: [
        {
          type: "notification",
          message: "示例动作完成",
          tone: "success",
        },
      ],
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a29",
      type: "html",
      html: "<h2>隔离 HTML</h2><script>alert('blocked')</script>",
      height: 240,
      caption: "无脚本、无网络的 HTML 预览",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a30",
      type: "audio",
      asset_revision_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b30",
      title: "示例音频",
      artist: "Anby Wiki",
      album: "媒体块验证",
      duration_seconds: 95,
      caption: "固定到不可变 AssetRevision",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a31",
      type: "file",
      asset_revision_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b31",
      name: "protocol-fixture.zip",
      caption: "正文文件下载卡片",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a32",
      type: "pdf",
      asset_revision_id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b32",
      title: "示例 PDF",
      start_page: 2,
      height: 480,
      caption: "正文 PDF 阅读块",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a33",
      type: "bookmark",
      url: "https://example.com/reference",
      title: "示例书签",
      description: "冻结的标题、描述与站点元数据。",
      site_name: "Example",
      caption: "独立 Bookmark caption",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a34",
      type: "link_preview",
      url: "https://example.com/news",
      title: "第三方链接预览",
      description: "阅读页不再实时回源。",
      site_name: "Example",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a35",
      type: "embed",
      url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
      title: "YouTube 白名单嵌入",
      provider: "youtube",
      caption: "不匹配供应商时会降级为普通链接",
    },
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a09",
      type: "divider",
    },
  ],
});

export default function DevEditorPage() {
  if (process.env.NODE_ENV === "production") {
    notFound();
  }
  const [ast, setAst] = useState<Document>(SAMPLE_AST);
  return (
    <div className="mx-auto grid w-full max-w-6xl gap-6 p-8 lg:grid-cols-2">
      <PageButtonRuntime pageId="0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01" />
      <section className="min-w-0">
        <h1 className="mb-4 text-lg font-semibold">BlockEditor（M2-T02 验证页）</h1>
        <div className="rounded-lg border border-border p-4">
          <BlockEditor initialAst={SAMPLE_AST} onChange={setAst} />
        </div>
      </section>
      <section className="min-w-0">
        <h2 className="mb-4 text-lg font-semibold">onChange 输出的 AST</h2>
        <pre className="max-h-[80vh] overflow-auto rounded-lg border border-border bg-muted p-4 text-xs leading-5">
          {JSON.stringify(ast, null, 2)}
        </pre>
      </section>
      <section className="min-w-0 lg:col-span-2">
        <h2 className="mb-4 text-lg font-semibold">阅读端 AST 预览</h2>
        <div className="rounded-lg border border-border p-4">
          <AstDocument document={ast} />
        </div>
      </section>
    </div>
  );
}
