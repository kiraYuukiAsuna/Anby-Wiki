// M2-T02 手工验证页（仅 development 可见，生产构建直接 404）。
// 左侧为 BlockEditor（受控：初始 AST + onChange(AST)），右侧实时显示序列化 AST。
"use client";

import { useState } from "react";

import { notFound } from "next/navigation";

import { BlockEditor } from "@/components/editor/block-editor";
import { parseDocument, type Document } from "@/lib/ast/schema";

const SAMPLE_AST: Document = parseDocument({
  type: "document",
  schema_version: 1,
  children: [
    {
      id: "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01",
      type: "heading",
      level: 1,
      content: [{ type: "text", text: "编辑器手工验证", marks: ["bold"] }],
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
      content: "package main\n",
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
      <section>
        <h1 className="mb-4 text-lg font-semibold">BlockEditor（M2-T02 验证页）</h1>
        <div className="rounded-lg border border-border p-4">
          <BlockEditor initialAst={SAMPLE_AST} onChange={setAst} />
        </div>
      </section>
      <section>
        <h2 className="mb-4 text-lg font-semibold">onChange 输出的 AST</h2>
        <pre className="max-h-[80vh] overflow-auto rounded-lg border border-border bg-muted p-4 text-xs leading-5">
          {JSON.stringify(ast, null, 2)}
        </pre>
      </section>
    </div>
  );
}
