package ast

import (
	"fmt"
	"reflect"
	"testing"
)

// walkTestDoc 是遍历/定位测试共用的文档：
//
//	children[0] heading h1（2 个行内节点）
//	children[1] bullet_list l1
//	  └─ list_item i1
//	     └─ paragraph p1（1 个行内节点）
//	children[2] code c1（字符串 content，无行内节点）
const walkTestDoc = `{
  "type": "document",
  "schema_version": 1,
  "children": [
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d01","type":"heading","level":2,"content":[
      {"type":"text","text":"标题"},
      {"type":"text","text":"加粗","marks":["bold"]}
    ]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d02","type":"bullet_list","children":[
      {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d03","type":"list_item","children":[
        {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d04","type":"paragraph","content":[
          {"type":"text","text":"条目"}
        ]}
      ]}
    ]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d05","type":"code","content":"package main\n"}
  ]
}`

func mustParse(t *testing.T, data string) *Document {
	t.Helper()
	doc, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	return doc
}

func walkDescriptor(n WalkNode) string {
	if n.Block != nil {
		return fmt.Sprintf("block:%s@%d", n.Block.Type, n.Depth)
	}
	return fmt.Sprintf("inline:%s[%d]", n.Inline.Type, n.Index)
}

// TestWalkOrder 断言前序顺序：Block 自身 → 行内 content → 子树。
func TestWalkOrder(t *testing.T) {
	doc := mustParse(t, walkTestDoc)
	var got []string
	err := Walk(doc, func(n WalkNode) bool {
		got = append(got, walkDescriptor(n))
		return true
	})
	if err != nil {
		t.Fatalf("Walk 失败: %v", err)
	}
	want := []string{
		"block:heading@0",
		"inline:text[0]",
		"inline:text[1]",
		"block:bullet_list@0",
		"block:list_item@1",
		"block:paragraph@2",
		"inline:text[0]",
		"block:code@0",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("遍历顺序不符:\n got: %v\nwant: %v", got, want)
	}
}

// TestWalkEarlyStop fn 返回 false 时中止整个遍历。
func TestWalkEarlyStop(t *testing.T) {
	doc := mustParse(t, walkTestDoc)
	count := 0
	err := Walk(doc, func(n WalkNode) bool {
		count++
		return false
	})
	if err != nil {
		t.Fatalf("Walk 失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("提前终止后仍访问了 %d 个节点", count)
	}
}

// TestWalkInlineParent 行内节点的 Parent 是所属 Block。
func TestWalkInlineParent(t *testing.T) {
	doc := mustParse(t, walkTestDoc)
	err := Walk(doc, func(n WalkNode) bool {
		if n.Inline != nil && n.Parent == nil {
			t.Fatalf("行内节点缺少父 Block")
		}
		return true
	})
	if err != nil {
		t.Fatalf("Walk 失败: %v", err)
	}
}

// TestFindBlock 定位嵌套 Block，断言父链与索引路径。
func TestFindBlock(t *testing.T) {
	doc := mustParse(t, walkTestDoc)
	loc, ok := FindBlock(doc, "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d04")
	if !ok {
		t.Fatalf("未找到嵌套 paragraph")
	}
	if loc.Parent == nil || loc.Parent.ID != "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d03" {
		t.Fatalf("父 Block 不符: %+v", loc.Parent)
	}
	if loc.Index != 0 {
		t.Fatalf("Index 不符: %d", loc.Index)
	}
	if !reflect.DeepEqual(loc.Path, []int{1, 0, 0}) {
		t.Fatalf("Path 不符: %v", loc.Path)
	}
}

// TestFindBlockTopLevel 顶层 Block 的 Parent 为 nil。
func TestFindBlockTopLevel(t *testing.T) {
	doc := mustParse(t, walkTestDoc)
	loc, ok := FindBlock(doc, "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d05")
	if !ok {
		t.Fatalf("未找到顶层 code")
	}
	if loc.Parent != nil || loc.Index != 2 || !reflect.DeepEqual(loc.Path, []int{2}) {
		t.Fatalf("顶层定位不符: %+v", loc)
	}
}

func TestFindBlockNotFound(t *testing.T) {
	doc := mustParse(t, walkTestDoc)
	if _, ok := FindBlock(doc, "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4ffff"); ok {
		t.Fatalf("不存在的 ID 不应被定位")
	}
}

// TestBlocks 平铺列表按前序排列。
func TestBlocks(t *testing.T) {
	doc := mustParse(t, walkTestDoc)
	blocks := Blocks(doc)
	var got []BlockType
	for _, b := range blocks {
		got = append(got, b.Type)
	}
	want := []BlockType{BlockHeading, BlockBulletList, BlockListItem, BlockParagraph, BlockCode}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Blocks 顺序不符:\n got: %v\nwant: %v", got, want)
	}
}
