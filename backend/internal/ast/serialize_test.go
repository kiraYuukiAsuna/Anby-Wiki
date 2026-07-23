package ast

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// canonicalFixtureNames 列出 fixtures/canonical 下的输入文档名（不含期望文件）。
func canonicalFixtureNames(t *testing.T) []string {
	t.Helper()
	dir := fixturesDir(t, "canonical")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取 canonical fixtures 目录失败: %v", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".json") && !strings.HasSuffix(n, ".canonical.json") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatalf("fixtures/canonical 目录为空")
	}
	return names
}

// TestCanonicalFixtures 遍历共享 canonical fixtures：
// CanonicalizeJSON(输入) 必须逐字节等于 {name}.canonical.json，
// 其 SHA-256 必须等于 {name}.sha256。vitest 侧对同一目录做同样断言。
func TestCanonicalFixtures(t *testing.T) {
	dir := fixturesDir(t, "canonical")
	for _, name := range canonicalFixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("读取输入 fixture 失败: %v", err)
			}
			base := strings.TrimSuffix(name, ".json")

			wantCanon, err := os.ReadFile(filepath.Join(dir, base+".canonical.json"))
			if err != nil {
				t.Fatalf("读取期望 canonical 失败: %v", err)
			}
			wantHashBytes, err := os.ReadFile(filepath.Join(dir, base+".sha256"))
			if err != nil {
				t.Fatalf("读取期望哈希失败: %v", err)
			}
			wantHash := strings.TrimSpace(string(wantHashBytes))

			canon, err := CanonicalizeJSON(input)
			if err != nil {
				t.Fatalf("CanonicalizeJSON 失败: %v", err)
			}
			if !bytes.Equal(canon, wantCanon) {
				t.Fatalf("canonical 字节不一致:\n got: %s\nwant: %s", canon, wantCanon)
			}
			sum := sha256.Sum256(canon)
			if got := hex.EncodeToString(sum[:]); got != wantHash {
				t.Fatalf("哈希不一致: got %s, want %s", got, wantHash)
			}

			// canonical 幂等。
			again, err := CanonicalizeJSON(canon)
			if err != nil {
				t.Fatalf("canonical 二次解析失败: %v", err)
			}
			if !bytes.Equal(again, canon) {
				t.Fatalf("canonical 不幂等")
			}

			// 经 Go 类型解析后哈希一致（fixture 均通过 Schema 校验）。
			doc, err := Parse(input)
			if err != nil {
				t.Fatalf("Parse 失败: %v", err)
			}
			h, err := ContentHash(doc)
			if err != nil {
				t.Fatalf("ContentHash 失败: %v", err)
			}
			if h != wantHash {
				t.Fatalf("ContentHash(doc) 与 fixture 哈希不一致: got %s, want %s", h, wantHash)
			}
		})
	}
}

// TestCanonicalMapOrderIndependence 字面顺序/空白不同但语义相同的 JSON
// 必须产出相同 canonical 字节与哈希。
func TestCanonicalMapOrderIndependence(t *testing.T) {
	a := `{"b":1,"a":[true,null,"x"],"c":{"y":2,"x":1}}`
	b := "{\n  \"c\" : { \"x\": 1, \"y\": 2 },\n  \"a\" : [ true, null, \"x\" ],\n  \"b\" : 1\n}"
	ca, err := CanonicalizeJSON([]byte(a))
	if err != nil {
		t.Fatalf("canonicalize a 失败: %v", err)
	}
	cb, err := CanonicalizeJSON([]byte(b))
	if err != nil {
		t.Fatalf("canonicalize b 失败: %v", err)
	}
	if !bytes.Equal(ca, cb) {
		t.Fatalf("键序/空白不同的等价 JSON canonical 不一致:\n a: %s\n b: %s", ca, cb)
	}
	want := `{"a":[true,null,"x"],"b":1,"c":{"x":1,"y":2}}`
	if string(ca) != want {
		t.Fatalf("canonical 结果不符:\n got: %s\nwant: %s", ca, want)
	}
}

// TestCanonicalStringEscaping 固定字符串转义规则：
// 仅转义引号、反斜杠与控制字符；非 ASCII 与 '<' '>' '&' 保留原始 UTF-8。
func TestCanonicalStringEscaping(t *testing.T) {
	in := `{"s":"<tag> & \"引号\" \\ 换行\n制表\t 退格\b 换页\f 回车\r 控制\u0001 汉字 👩‍💻"}`
	canon, err := CanonicalizeJSON([]byte(in))
	if err != nil {
		t.Fatalf("CanonicalizeJSON 失败: %v", err)
	}
	want := `{"s":"<tag> & \"引号\" \\ 换行\n制表\t 退格\b 换页\f 回车\r 控制\u0001 汉字 👩‍💻"}`
	if string(canon) != want {
		t.Fatalf("转义结果不符:\n got: %s\nwant: %s", canon, want)
	}
}

// TestCanonicalNumberRules 整数规范化与非整数拒绝。
func TestCanonicalNumberRules(t *testing.T) {
	canon, err := CanonicalizeJSON([]byte(`{"a":1.0,"b":-0,"c":10e1}`))
	if err != nil {
		t.Fatalf("整数型数字应被接受: %v", err)
	}
	if string(canon) != `{"a":1,"b":0,"c":100}` {
		t.Fatalf("整数规范化不符: %s", canon)
	}
	for _, bad := range []string{`{"a":1.5}`, `{"a":1e100}`} {
		if _, err := CanonicalizeJSON([]byte(bad)); err == nil {
			t.Fatalf("非整数/超范围数字应被拒绝: %s", bad)
		}
	}
}

// TestContentHashStableAcrossRuns 同一文档多次哈希结果一致，
// 且与 CanonicalizeJSON 路径一致。
func TestContentHashStableAcrossRuns(t *testing.T) {
	data := readFixture(t, "valid", "full_document.json")
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	h1, err := ContentHash(doc)
	if err != nil {
		t.Fatalf("ContentHash 失败: %v", err)
	}
	h2, err := ContentHash(doc)
	if err != nil {
		t.Fatalf("ContentHash 失败: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("同一文档两次哈希不一致: %s vs %s", h1, h2)
	}
	canon, err := CanonicalizeJSON(data)
	if err != nil {
		t.Fatalf("CanonicalizeJSON 失败: %v", err)
	}
	sum := sha256.Sum256(canon)
	if want := hex.EncodeToString(sum[:]); h1 != want {
		t.Fatalf("ContentHash 与 CanonicalizeJSON 路径不一致: %s vs %s", h1, want)
	}
}
