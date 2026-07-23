// 知识引用编辑命令（refs.go）单测：构造器校验、Patch 构建、插入应用的正反样例。
package ast

import (
	"encoding/json"
	"testing"
)

const (
	refEntityID   = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d01"
	refClaimID    = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d02"
	refCitationID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d03"
)

// newRefTestDoc 构造含一个 paragraph（两个 text 节点）与一个 code Block 的文档。
func newRefTestDoc(t *testing.T) *Document {
	t.Helper()
	doc, err := Parse([]byte(`{
		"type": "document",
		"schema_version": 1,
		"children": [
			{"id": "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01", "type": "paragraph",
				"content": [{"type": "text", "text": "前"}, {"type": "text", "text": "后"}]},
			{"id": "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c02", "type": "code", "content": "x"}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	return doc
}

func paragraphContent(t *testing.T, doc *Document, blockID string) []*InlineNode {
	t.Helper()
	loc, ok := FindBlock(doc, blockID)
	if !ok {
		t.Fatalf("Block %s 不存在", blockID)
	}
	nodes, err := loc.Block.InlineContent()
	if err != nil {
		t.Fatalf("解码行内 content 失败: %v", err)
	}
	return nodes
}

func TestNewRefNodes(t *testing.T) {
	n, err := NewEntityRefNode(refEntityID, "安比")
	if err != nil {
		t.Fatalf("NewEntityRefNode 失败: %v", err)
	}
	if n.Type != InlineEntityReference || n.EntityID != refEntityID || n.DisplayText != "安比" {
		t.Fatalf("节点字段不符: %+v", n)
	}

	n, err = NewClaimRefNode(refClaimID, "12 月 20 日")
	if err != nil {
		t.Fatalf("NewClaimRefNode 失败: %v", err)
	}
	if n.Type != InlineClaimReference || n.ClaimID != refClaimID {
		t.Fatalf("节点字段不符: %+v", n)
	}

	// citation 的 display_text 可省略，省略时不序列化该字段。
	n, err = NewCitationRefNode(refCitationID, "")
	if err != nil {
		t.Fatalf("NewCitationRefNode 失败: %v", err)
	}
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	want := `{"type":"citation_reference","citation_id":"` + refCitationID + `"}`
	if string(raw) != want {
		t.Fatalf("got %s, want %s", raw, want)
	}
}

func TestNewRefNodesRejectsInvalid(t *testing.T) {
	if _, err := NewEntityRefNode("not-a-uuid", "安比"); err == nil {
		t.Fatal("非法 entity_id 应被拒绝")
	}
	if _, err := NewEntityRefNode(refEntityID, "  "); err == nil {
		t.Fatal("空 display_text 应被拒绝")
	}
	if _, err := NewClaimRefNode(refClaimID, ""); err == nil {
		t.Fatal("claim 空 display_text 应被拒绝")
	}
	if _, err := NewCitationRefNode("", "x"); err == nil {
		t.Fatal("空 citation_id 应被拒绝")
	}
}

func TestInsertEntityRef(t *testing.T) {
	doc := newRefTestDoc(t)
	out, err := InsertEntityRef(doc, "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01", 1, refEntityID, "安比")
	if err != nil {
		t.Fatalf("InsertEntityRef 失败: %v", err)
	}
	nodes := paragraphContent(t, out, "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01")
	if len(nodes) != 3 {
		t.Fatalf("content 长度 got %d, want 3", len(nodes))
	}
	if nodes[1].Type != InlineEntityReference || nodes[1].EntityID != refEntityID || nodes[1].DisplayText != "安比" {
		t.Fatalf("插入位置/字段不符: %+v", nodes[1])
	}
	// 输入文档不被修改。
	if got := paragraphContent(t, doc, "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01"); len(got) != 2 {
		t.Fatalf("输入文档被修改: content 长度 %d", len(got))
	}
}

func TestInsertClaimRefAtEnds(t *testing.T) {
	doc := newRefTestDoc(t)
	const blockID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01"

	head, err := InsertClaimRef(doc, blockID, 0, refClaimID, "生日")
	if err != nil {
		t.Fatalf("index=0 插入失败: %v", err)
	}
	if got := paragraphContent(t, head, blockID)[0].Type; got != InlineClaimReference {
		t.Fatalf("首节点 got %q, want claim_reference", got)
	}

	tail, err := InsertClaimRef(doc, blockID, 2, refClaimID, "生日")
	if err != nil {
		t.Fatalf("index=len 追加失败: %v", err)
	}
	nodes := paragraphContent(t, tail, blockID)
	if got := nodes[len(nodes)-1].Type; got != InlineClaimReference {
		t.Fatalf("末节点 got %q, want claim_reference", got)
	}
}

func TestInsertCitationRefIntoHeading(t *testing.T) {
	doc, err := Parse([]byte(`{
		"type": "document", "schema_version": 1,
		"children": [{"id": "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c09", "type": "heading",
			"level": 2, "content": [{"type": "text", "text": "标题"}]}]
	}`))
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	out, err := InsertCitationRef(doc, "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c09", 1, refCitationID, "")
	if err != nil {
		t.Fatalf("InsertCitationRef 失败: %v", err)
	}
	nodes, err := out.Children[0].InlineContent()
	if err != nil {
		t.Fatalf("解码 content 失败: %v", err)
	}
	if nodes[1].Type != InlineCitationReference || nodes[1].CitationID != refCitationID {
		t.Fatalf("插入结果不符: %+v", nodes[1])
	}
}

func TestInsertRefRejectsInvalidTargets(t *testing.T) {
	doc := newRefTestDoc(t)
	const paraID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01"
	const codeID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c02"

	if _, err := InsertEntityRef(doc, "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4fff", 0, refEntityID, "x"); err == nil {
		t.Fatal("目标 Block 不存在应报错")
	}
	if _, err := InsertEntityRef(doc, codeID, 0, refEntityID, "x"); err == nil {
		t.Fatal("code Block 应被拒绝（非 paragraph/heading）")
	}
	if _, err := InsertEntityRef(doc, paraID, -1, refEntityID, "x"); err == nil {
		t.Fatal("负索引应被拒绝")
	}
	if _, err := InsertEntityRef(doc, paraID, 3, refEntityID, "x"); err == nil {
		t.Fatal("越界索引（> len）应被拒绝")
	}
	if _, err := InsertEntityRef(doc, paraID, 0, "bad-uuid", "x"); err == nil {
		t.Fatal("非法 entity_id 应被拒绝")
	}
	if _, err := InsertClaimRef(doc, paraID, 0, refClaimID, ""); err == nil {
		t.Fatal("空 display_text 应被拒绝")
	}
	if _, err := InsertEntityRef(nil, paraID, 0, refEntityID, "x"); err == nil {
		t.Fatal("nil 文档应被拒绝")
	}
}

func TestInsertRefPatchComposable(t *testing.T) {
	doc := newRefTestDoc(t)
	const blockID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4c01"

	// Patch 构建器不修改文档，可交由调用方决定是否应用（M5 Patch Engine 组合）。
	p, err := InsertEntityRefPatch(doc, blockID, 1, refEntityID, "安比")
	if err != nil {
		t.Fatalf("InsertEntityRefPatch 失败: %v", err)
	}
	if p.Op != OpReplaceBlock || p.ID != blockID || p.Block == nil {
		t.Fatalf("Patch 形态不符: %+v", p)
	}
	if got := paragraphContent(t, doc, blockID); len(got) != 2 {
		t.Fatalf("构建 Patch 不应修改输入文档")
	}
	out, err := ApplyPatch(doc, p)
	if err != nil {
		t.Fatalf("ApplyPatch 失败: %v", err)
	}
	if got := paragraphContent(t, out, blockID); len(got) != 3 {
		t.Fatalf("应用后 content 长度 got %d, want 3", len(got))
	}
}
