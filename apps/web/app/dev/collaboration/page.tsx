import { notFound } from "next/navigation";
import { EditorSessionView } from "@/components/editor/editor-session-view";

export default function CollaborationFixture() {
  if (process.env.NODE_ENV === "production") notFound();
  return <EditorSessionView
    pageId="0198a1b2-c3d4-7e5f-8a9b-000000000010"
    displayTitle="协作成员显示验证"
    baseRevisionId="0198a1b2-c3d4-7e5f-8a9b-000000000011"
    initialAst={{ type: "document", schema_version: 1, children: [{ id: "0198a1b2-c3d4-7e5f-8a9b-000000000012", type: "paragraph", content: [{ type: "text", text: "协作测试正文" }] }] }}
  />;
}
