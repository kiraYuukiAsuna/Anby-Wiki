package ast

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/uuid"
)

const contractsSchemaPath = "../../../contracts/schemas/ast/v1/ast.schema.json"

func fixturesDir(t *testing.T, kind string) string {
	t.Helper()
	return filepath.Join("../../../contracts/schemas/ast/v1/fixtures", kind)
}

func listFixtures(t *testing.T, kind string) []string {
	t.Helper()
	entries, err := os.ReadDir(fixturesDir(t, kind))
	if err != nil {
		t.Fatalf("读取 fixtures 目录失败: %v", err)
	}
	var names []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatalf("fixtures/%s 目录为空", kind)
	}
	return names
}

func readFixture(t *testing.T, kind, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixturesDir(t, kind), name))
	if err != nil {
		t.Fatalf("读取 fixture 失败: %v", err)
	}
	return data
}

// TestValidFixtures 遍历共享 valid fixtures，全部必须通过 Schema 校验。
func TestValidFixtures(t *testing.T) {
	for _, name := range listFixtures(t, "valid") {
		t.Run(name, func(t *testing.T) {
			data := readFixture(t, "valid", name)
			if err := ValidateJSON(data); err != nil {
				t.Fatalf("valid fixture 被拒绝: %v", err)
			}
		})
	}
}

// TestInvalidFixtures 遍历共享 invalid fixtures，全部必须被拒绝。
func TestInvalidFixtures(t *testing.T) {
	for _, name := range listFixtures(t, "invalid") {
		t.Run(name, func(t *testing.T) {
			data := readFixture(t, "invalid", name)
			if err := ValidateJSON(data); err == nil {
				t.Fatalf("invalid fixture 被错误接受")
			}
		})
	}
}

// TestSchemaSyncWithContracts 防止内嵌副本与 contracts/ 权威 Schema 漂移。
func TestSchemaSyncWithContracts(t *testing.T) {
	original, err := os.ReadFile(contractsSchemaPath)
	if err != nil {
		t.Fatalf("读取 contracts Schema 失败: %v", err)
	}
	if !bytes.Equal(original, schemaJSON) {
		t.Fatalf("内嵌 schema/ast.schema.json 与 %s 不一致；请先修改 contracts 原文件再同步副本", contractsSchemaPath)
	}
}

// TestValidateConstructedDocument 用 Go 类型构造文档并通过校验。
func TestValidateConstructedDocument(t *testing.T) {
	blockID, err := NewID()
	if err != nil {
		t.Fatalf("NewID 失败: %v", err)
	}
	content, err := json.Marshal([]*InlineNode{
		{Type: InlineText, Text: "你好", Marks: []Mark{MarkBold}},
		{Type: InlineExternalLink, URL: "https://example.com", DisplayText: "链接"},
	})
	if err != nil {
		t.Fatalf("序列化 content 失败: %v", err)
	}
	doc := NewDocument()
	doc.Children = append(doc.Children, &Block{
		ID:      blockID,
		Type:    BlockParagraph,
		Content: content,
	})
	if err := Validate(doc); err != nil {
		t.Fatalf("构造的文档未通过校验: %v", err)
	}
}

// TestInvalidDocuments 覆盖任务要求的失败类别（细粒度，独立于 fixtures）。
func TestInvalidDocuments(t *testing.T) {
	cases := map[string]string{
		"缺 id":                     `{"type":"document","schema_version":1,"children":[{"type":"paragraph","content":[]}]}`,
		"非法 level":                 `{"type":"document","schema_version":1,"children":[{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e01","type":"heading","level":0,"content":[]}]}`,
		"list 直挂 paragraph":        `{"type":"document","schema_version":1,"children":[{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e02","type":"ordered_list","children":[{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e03","type":"paragraph","content":[]}]}]}`,
		"divider 带内容":              `{"type":"document","schema_version":1,"children":[{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e04","type":"divider","content":"x"}]}`,
		"未解析引用缺 resolution_status": `{"type":"document","schema_version":1,"children":[{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e05","type":"paragraph","content":[{"type":"page_reference","target_namespace":"Main","normalized_title":"x"}]}]}`,
		"非法 uuid":                  `{"type":"document","schema_version":1,"children":[{"id":"not-a-uuid","type":"divider"}]}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateJSON([]byte(doc)); err == nil {
				t.Fatalf("非法文档被错误接受: %s", doc)
			}
		})
	}
}

// TestParseRoundTrip 解析 valid fixture 为 Go 类型，再序列化并重新校验。
func TestParseRoundTrip(t *testing.T) {
	data := readFixture(t, "valid", "full_document.json")
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if doc.Type != "document" || doc.SchemaVersion != SchemaVersion {
		t.Fatalf("根节点字段不符: %+v", doc)
	}
	if len(doc.Children) == 0 {
		t.Fatalf("full_document.json 应有顶层 Block")
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("再序列化失败: %v", err)
	}
	if err := ValidateJSON(out); err != nil {
		t.Fatalf("往返后的文档未通过校验: %v", err)
	}
}

// TestNewID 校验生成的 ID 是合法 UUIDv7。
func TestNewID(t *testing.T) {
	s, err := NewID()
	if err != nil {
		t.Fatalf("NewID 失败: %v", err)
	}
	u, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("NewID 返回非法 UUID %q: %v", s, err)
	}
	if u.Version() != 7 {
		t.Fatalf("期望 UUIDv7，实际版本 %d（%s）", u.Version(), s)
	}
}
