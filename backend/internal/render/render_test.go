// render 单测：每种 Block/Inline 的输出快照、嵌套 marks、未解析引用、
// XSS 四类用例（<script>/事件属性/javascript:/data:）与非法 URL 降级。
package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/ast"
)

// parseDoc 解析并校验 AST JSON；非法输入直接失败测试。
func parseDoc(t *testing.T, raw string) *ast.Document {
	t.Helper()
	doc, err := ast.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("ast.Parse 失败: %v", err)
	}
	return doc
}

// renderDoc 渲染并断言无错误。
func renderDoc(t *testing.T, doc *ast.Document) string {
	t.Helper()
	html, err := RenderHTML(doc)
	if err != nil {
		t.Fatalf("RenderHTML 失败: %v", err)
	}
	return html
}

func TestRenderBlocksSnapshot(t *testing.T) {
	doc := parseDoc(t, `{
		"type": "document",
		"schema_version": 1,
		"children": [
			{"id": "00000000-0000-7000-8000-000000000001", "type": "heading", "level": 2,
				"content": [{"type": "text", "text": "标题"}]},
			{"id": "00000000-0000-7000-8000-000000000002", "type": "paragraph",
				"content": [{"type": "text", "text": "正文"}]},
			{"id": "00000000-0000-7000-8000-000000000003", "type": "bullet_list", "children": [
				{"id": "00000000-0000-7000-8000-000000000004", "type": "list_item", "children": [
					{"id": "00000000-0000-7000-8000-000000000005", "type": "paragraph",
						"content": [{"type": "text", "text": "甲"}]}
				]}
			]},
			{"id": "00000000-0000-7000-8000-000000000006", "type": "ordered_list", "children": [
				{"id": "00000000-0000-7000-8000-000000000007", "type": "list_item", "children": [
					{"id": "00000000-0000-7000-8000-000000000008", "type": "paragraph",
						"content": [{"type": "text", "text": "乙"}]}
				]}
			]},
			{"id": "00000000-0000-7000-8000-000000000009", "type": "table", "children": [
				{"id": "00000000-0000-7000-8000-00000000000a", "type": "table_row", "children": [
					{"id": "00000000-0000-7000-8000-00000000000b", "type": "table_cell", "children": [
						{"id": "00000000-0000-7000-8000-00000000000c", "type": "paragraph",
							"content": [{"type": "text", "text": "格"}]}
					]}
				]}
			]},
			{"id": "00000000-0000-7000-8000-00000000000d", "type": "code",
				"content": "fmt.Println()", "language": "go"},
			{"id": "00000000-0000-7000-8000-00000000000e", "type": "quote", "children": [
				{"id": "00000000-0000-7000-8000-00000000000f", "type": "paragraph",
					"content": [{"type": "text", "text": "引用"}]}
			]},
			{"id": "00000000-0000-7000-8000-000000000010", "type": "callout", "kind": "warning", "children": [
				{"id": "00000000-0000-7000-8000-000000000011", "type": "paragraph",
					"content": [{"type": "text", "text": "注意"}]}
			]},
			{"id": "00000000-0000-7000-8000-000000000012", "type": "divider"}
		]
	}`)

	want := `<h2 id="00000000-0000-7000-8000-000000000001">标题</h2>` +
		`<p>正文</p>` +
		`<ul><li><p>甲</p></li></ul>` +
		`<ol><li><p>乙</p></li></ol>` +
		`<table><tbody><tr><td><p>格</p></td></tr></tbody></table>` +
		`<pre><code class="language-go">fmt.Println()</code></pre>` +
		`<blockquote><p>引用</p></blockquote>` +
		`<div class="callout" data-callout="warning"><div class="callout-title">warning</div><p>注意</p></div>` +
		`<hr>`

	if got := renderDoc(t, doc); got != want {
		t.Fatalf("HTML 快照不匹配:\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderHeadingLevels(t *testing.T) {
	for level := 1; level <= 6; level++ {
		doc := parseDoc(t, fmt.Sprintf(`{"type":"document","schema_version":1,"children":[
			{"id":"00000000-0000-7000-8000-000000000001","type":"heading","level":%d,"content":[]}]}`, level))
		tag := fmt.Sprintf("h%d", level)
		want := `<` + tag + ` id="00000000-0000-7000-8000-000000000001"></` + tag + `>`
		if got := renderDoc(t, doc); got != want {
			t.Fatalf("level=%d: got %s, want %s", level, got, want)
		}
	}
}

func TestRenderNestedMarks(t *testing.T) {
	doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
		{"id":"00000000-0000-7000-8000-000000000001","type":"paragraph","content":[
			{"type":"text","text":"叠加","marks":["bold","italic","strikethrough","code"]},
			{"type":"text","text":"普通"},
			{"type":"inline_code","text":"x<y"}
		]}]}`)
	want := `<p><strong><em><del><code>叠加</code></del></em></strong>普通<code>x&lt;y</code></p>`
	if got := renderDoc(t, doc); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestRenderPageReferences(t *testing.T) {
	doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
		{"id":"00000000-0000-7000-8000-000000000001","type":"paragraph","content":[
			{"type":"page_reference","target_page_id":"00000000-0000-7000-8000-000000000099",
				"display_text":"安比"},
			{"type":"page_reference","target_page_id":"00000000-0000-7000-8000-000000000099",
				"target_heading_block_id":"00000000-0000-7000-8000-000000000088","display_text":"章节"},
			{"type":"page_reference","resolution_status":"unresolved",
				"target_namespace":"main","normalized_title":"不存在的页"}
		]}]}`)
	want := `<p>` +
		`<a href="/pages/00000000-0000-7000-8000-000000000099" data-page-ref>安比</a>` +
		`<a href="/pages/00000000-0000-7000-8000-000000000099#00000000-0000-7000-8000-000000000088" data-page-ref>章节</a>` +
		`<a class="unresolved" data-unresolved-ref data-target-namespace="main" data-target-title="不存在的页">不存在的页</a>` +
		`</p>`
	got := renderDoc(t, doc)
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if strings.Contains(got, `class="unresolved" href`) {
		t.Fatalf("未解析引用不得包含 href: %s", got)
	}
}

func TestRenderExternalLink(t *testing.T) {
	doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
		{"id":"00000000-0000-7000-8000-000000000001","type":"paragraph","content":[
			{"type":"external_link","url":"https://example.com/?a=1&b=2","display_text":"示例"}
		]}]}`)
	want := `<p><a href="https://example.com/?a=1&amp;b=2" rel="noopener noreferrer nofollow" target="_blank">示例</a></p>`
	if got := renderDoc(t, doc); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestRenderKnowledgeReferences(t *testing.T) {
	doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
		{"id":"00000000-0000-7000-8000-000000000001","type":"paragraph","content":[
			{"type":"entity_reference","entity_id":"00000000-0000-7000-8000-000000000011","display_text":"安比<&"},
			{"type":"claim_reference","claim_id":"00000000-0000-7000-8000-000000000012","display_text":"生日<值>"},
			{"type":"citation_reference","citation_id":"00000000-0000-7000-8000-000000000013","display_text":"设定集\"第 42 页"}
		]},
		{"id":"00000000-0000-7000-8000-000000000002","type":"quote","children":[
			{"id":"00000000-0000-7000-8000-000000000003","type":"paragraph","content":[
				{"type":"citation_reference","citation_id":"00000000-0000-7000-8000-000000000014"},
				{"type":"citation_reference","citation_id":"00000000-0000-7000-8000-000000000013","display_text":"重复引用"}
			]}
		]}
	]}`)

	want := `<p>` +
		`<a href="/entities/00000000-0000-7000-8000-000000000011" data-entity-ref>安比&lt;&amp;</a>` +
		`<a href="/claims/00000000-0000-7000-8000-000000000012" data-claim-ref="00000000-0000-7000-8000-000000000012">生日&lt;值&gt;</a>` +
		`<sup data-citation-ref="00000000-0000-7000-8000-000000000013" title="设定集&#34;第 42 页"><a href="/citations/00000000-0000-7000-8000-000000000013">[1]</a></sup>` +
		`</p><blockquote><p>` +
		`<sup data-citation-ref="00000000-0000-7000-8000-000000000014" title="00000000-0000-7000-8000-000000000014"><a href="/citations/00000000-0000-7000-8000-000000000014">[2]</a></sup>` +
		`<sup data-citation-ref="00000000-0000-7000-8000-000000000013" title="重复引用"><a href="/citations/00000000-0000-7000-8000-000000000013">[1]</a></sup>` +
		`</p></blockquote>`
	if got := renderDoc(t, doc); got != want {
		t.Fatalf("知识引用 HTML 快照不匹配:\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderUnsafeURLDegradesToText(t *testing.T) {
	cases := []string{
		"javascript:alert(1)",
		"JaVaScRiPt:alert(1)",
		"data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==",
		"vbscript:msgbox(1)",
	}
	for _, raw := range cases {
		doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
			{"id":"00000000-0000-7000-8000-000000000001","type":"paragraph","content":[
				{"type":"external_link","url":`+quoteJSON(raw)+`,"display_text":"点我"}
			]}]}`)
		got := renderDoc(t, doc)
		if want := `<p>点我</p>`; got != want {
			t.Fatalf("url=%q: got %s, want %s（应降级为纯文本）", raw, got, want)
		}
	}
}

// quoteJSON 把字符串包成 JSON 字符串字面量（本测试用例的 URL 不含需转义的特殊字符，
// 除 data: 用例中的 < > 外均为安全字符；< > 在 JSON 字符串中原样合法）。
func quoteJSON(s string) string {
	return `"` + s + `"`
}

func TestRenderXSS(t *testing.T) {
	t.Run("文本中的 script 标签被转义", func(t *testing.T) {
		doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
			{"id":"00000000-0000-7000-8000-000000000001","type":"paragraph","content":[
				{"type":"text","text":"<script>alert(1)</script>"}
			]}]}`)
		got := renderDoc(t, doc)
		if strings.Contains(got, "<script>") {
			t.Fatalf("输出含未转义 <script>: %s", got)
		}
		want := `<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>`
		if got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})

	t.Run("display_text 中的事件属性注入被转义", func(t *testing.T) {
		doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
			{"id":"00000000-0000-7000-8000-000000000001","type":"paragraph","content":[
				{"type":"external_link","url":"https://example.com",
					"display_text":"\"><img src=x onerror=alert(1)>"}
			]}]}`)
		got := renderDoc(t, doc)
		if strings.Contains(got, "onerror=") && !strings.Contains(got, "onerror=alert(1)&gt;") {
			t.Fatalf("输出疑似含事件属性注入: %s", got)
		}
		if strings.Contains(got, "<img") {
			t.Fatalf("输出含未转义 <img>: %s", got)
		}
	})

	t.Run("URL 中的引号注入被转义", func(t *testing.T) {
		doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
			{"id":"00000000-0000-7000-8000-000000000001","type":"paragraph","content":[
				{"type":"external_link","url":"https://example.com/\"><script>alert(1)</script>",
					"display_text":"x"}
			]}]}`)
		got := renderDoc(t, doc)
		if strings.Contains(got, "<script>") || strings.Contains(got, `"><`) {
			t.Fatalf("URL 属性注入未被转义: %s", got)
		}
	})

	t.Run("未解析引用与 code language 中的属性注入被转义", func(t *testing.T) {
		doc := parseDoc(t, `{"type":"document","schema_version":1,"children":[
			{"id":"00000000-0000-7000-8000-000000000001","type":"code",
				"content":"<script>alert(1)</script>","language":"go\"><script>alert(2)</script>"},
			{"id":"00000000-0000-7000-8000-000000000002","type":"paragraph","content":[
				{"type":"page_reference","resolution_status":"unresolved",
					"target_namespace":"main\"><script>alert(3)</script>",
					"normalized_title":"t\"><script>alert(4)</script>"}
			]}]}`)
		got := renderDoc(t, doc)
		if strings.Contains(got, "<script>") {
			t.Fatalf("输出含未转义 <script>: %s", got)
		}
	})
}

func TestRenderErrors(t *testing.T) {
	if _, err := RenderHTML(nil); err == nil {
		t.Fatal("nil doc 应返回错误")
	}
	// 未知 Block 类型：绕过 Schema 校验直接构造。
	doc := &ast.Document{Type: "document", SchemaVersion: 1, Children: []*ast.Block{
		{ID: "00000000-0000-7000-8000-000000000001", Type: "mystery"},
	}}
	if _, err := RenderHTML(doc); err == nil {
		t.Fatal("未知 Block 类型应返回错误")
	}
	// 未知 Inline 类型。
	doc = &ast.Document{Type: "document", SchemaVersion: 1, Children: []*ast.Block{
		{ID: "00000000-0000-7000-8000-000000000001", Type: ast.BlockParagraph,
			Content: []byte(`[{"type":"hologram"}]`)},
	}}
	if _, err := RenderHTML(doc); err == nil {
		t.Fatal("未知 Inline 类型应返回错误")
	}
}
