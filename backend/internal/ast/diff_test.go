package ast

import (
	"encoding/json"
	"reflect"
	"testing"
)

// diffBase / diffCurrent 覆盖四类变更，且各类别互不干扰：
//   - keep：内容位置均未变（不报告）；
//   - edit：index 不变、content 变化（changed）；
//   - move1：从 wrap（quote）内移到根部（moved），wrap 自身不报告；
//   - gone：位于 base 末尾被删除（removed），不影响其他块下标；
//   - new1：新增在 current 末尾（added）。
const diffBase = `{
  "type": "document",
  "schema_version": 1,
  "children": [
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f01","type":"paragraph","content":[{"type":"text","text":"keep"}]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f02","type":"paragraph","content":[{"type":"text","text":"before"}]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f06","type":"quote","children":[
      {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f03","type":"paragraph","content":[{"type":"text","text":"moveme"}]}
    ]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f04","type":"paragraph","content":[{"type":"text","text":"gone"}]}
  ]
}`

const diffCurrent = `{
  "type": "document",
  "schema_version": 1,
  "children": [
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f01","type":"paragraph","content":[{"type":"text","text":"keep"}]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f02","type":"paragraph","content":[{"type":"text","text":"after"}]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f06","type":"quote","children":[]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f03","type":"paragraph","content":[{"type":"text","text":"moveme"}]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f05","type":"code","language":"go","content":"package main\n"}
  ]
}`

const (
	idKeep = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f01"
	idEdit = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f02"
	idMove = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f03"
	idGone = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f04"
	idNew  = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f05"
	idWrap = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4f06"
)

func changesByID(d *DocumentDiff) map[string][]BlockChange {
	out := map[string][]BlockChange{}
	for _, c := range d.Changes {
		out[c.BlockID] = append(out[c.BlockID], c)
	}
	return out
}

// TestDiffCategories 断言 added/removed/changed/moved 四类识别，且未变块不报告。
func TestDiffCategories(t *testing.T) {
	base := mustParse(t, diffBase)
	cur := mustParse(t, diffCurrent)
	d, err := Diff(base, cur)
	if err != nil {
		t.Fatalf("Diff 失败: %v", err)
	}
	byID := changesByID(d)

	if cs := byID[idKeep]; len(cs) != 0 {
		t.Fatalf("keep 块不应有任何变更: %+v", cs)
	}
	if cs := byID[idWrap]; len(cs) != 0 {
		t.Fatalf("wrap 块自身未变，不应报告（子块移动记在子块上）: %+v", cs)
	}

	cs := byID[idEdit]
	if len(cs) != 1 || cs[0].Type != ChangeChanged {
		t.Fatalf("edit 块应有一条 changed: %+v", cs)
	}
	if len(cs[0].Fields) != 1 || cs[0].Fields[0].Field != "content" {
		t.Fatalf("changed 字段级摘要不符: %+v", cs[0].Fields)
	}
	fc := cs[0].Fields[0]
	if fc.Before != `[{"text":"before","type":"text"}]` || fc.After != `[{"text":"after","type":"text"}]` {
		t.Fatalf("before/after 摘要不符: %+v", fc)
	}

	if cs := byID[idMove]; len(cs) != 1 || cs[0].Type != ChangeMoved {
		t.Fatalf("move1 应有一条 moved: %+v", cs)
	} else {
		if cs[0].ParentID != "" {
			t.Fatalf("moved 后父应为顶层: %+v", cs[0])
		}
		if !reflect.DeepEqual(cs[0].BeforePath, []int{2, 0}) || !reflect.DeepEqual(cs[0].AfterPath, []int{3}) {
			t.Fatalf("moved 路径不符: %+v", cs[0])
		}
	}

	if cs := byID[idGone]; len(cs) != 1 || cs[0].Type != ChangeRemoved {
		t.Fatalf("gone 块应有一条 removed: %+v", cs)
	} else if !reflect.DeepEqual(cs[0].Path, []int{3}) {
		t.Fatalf("removed 路径不符: %+v", cs[0])
	}

	if cs := byID[idNew]; len(cs) != 1 || cs[0].Type != ChangeAdded {
		t.Fatalf("new 块应有一条 added: %+v", cs)
	} else if !reflect.DeepEqual(cs[0].Path, []int{4}) {
		t.Fatalf("added 路径不符: %+v", cs[0])
	}
}

// TestDiffNoop 相同文档无变更；结果可 JSON 序列化。
func TestDiffNoop(t *testing.T) {
	base := mustParse(t, diffBase)
	cur := mustParse(t, diffBase)
	d, err := Diff(base, cur)
	if err != nil {
		t.Fatalf("Diff 失败: %v", err)
	}
	if len(d.Changes) != 0 {
		t.Fatalf("相同文档不应有变更: %+v", d.Changes)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("DocumentDiff 序列化失败: %v", err)
	}
	if string(raw) != `{"changes":[]}` {
		t.Fatalf("空 Diff 序列化不符: %s", raw)
	}
}

// TestDiffHeadingLevelChanged 数字字段（level）变化的字段级摘要。
func TestDiffHeadingLevelChanged(t *testing.T) {
	base := mustParse(t, `{"type":"document","schema_version":1,"children":[{"id":"`+idKeep+`","type":"heading","level":1,"content":[]}]}`)
	cur := mustParse(t, `{"type":"document","schema_version":1,"children":[{"id":"`+idKeep+`","type":"heading","level":2,"content":[]}]}`)
	d, err := Diff(base, cur)
	if err != nil {
		t.Fatalf("Diff 失败: %v", err)
	}
	if len(d.Changes) != 1 || d.Changes[0].Type != ChangeChanged {
		t.Fatalf("应只有一条 changed: %+v", d.Changes)
	}
	fs := d.Changes[0].Fields
	if len(fs) != 1 || fs[0].Field != "level" || fs[0].Before != "1" || fs[0].After != "2" {
		t.Fatalf("level 字段摘要不符: %+v", fs)
	}
}
