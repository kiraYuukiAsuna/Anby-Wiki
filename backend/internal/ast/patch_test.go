package ast

import (
	"errors"
	"strings"
	"testing"
)

// patchTestDoc：children = [A paragraph, B bullet_list > B1 list_item > B2 paragraph, C paragraph]。
const patchTestDoc = `{
  "type": "document",
  "schema_version": 1,
  "children": [
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e01","type":"paragraph","content":[{"type":"text","text":"alpha"}]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e02","type":"bullet_list","children":[
      {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e03","type":"list_item","children":[
        {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e04","type":"paragraph","content":[{"type":"text","text":"item"}]}
      ]}
    ]},
    {"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e05","type":"paragraph","content":[{"type":"text","text":"omega"}]}
  ]
}`

const (
	idA  = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e01"
	idB  = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e02"
	idB1 = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e03"
	idB2 = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e04"
	idC  = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e05"
	idX  = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4e99"
)

func mustParseBlock(t *testing.T, data string) *Block {
	t.Helper()
	doc := mustParse(t, `{"type":"document","schema_version":1,"children":[`+data+`]}`)
	return doc.Children[0]
}

func requirePatchErr(t *testing.T, err error, wantOp PatchOpType, wantSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望 %s 返回错误", wantOp)
	}
	var pe *PatchError
	if !errors.As(err, &pe) {
		t.Fatalf("错误类型应为 *PatchError: %T (%v)", err, err)
	}
	if pe.Op != wantOp {
		t.Fatalf("错误操作不符: got %s, want %s", pe.Op, wantOp)
	}
	if !strings.Contains(pe.Error(), wantSubstr) {
		t.Fatalf("错误信息不含 %q: %v", wantSubstr, err)
	}
}

// TestApplyPatchReplace 正反样例：替换成功且不改原值；目标不存在/非法新块报错。
func TestApplyPatchReplace(t *testing.T) {
	doc := mustParse(t, patchTestDoc)
	before, err := CanonicalJSON(doc)
	if err != nil {
		t.Fatalf("CanonicalJSON 失败: %v", err)
	}

	nb := mustParseBlock(t, `{"id":"`+idX+`","type":"heading","level":3,"content":[{"type":"text","text":"新标题"}]}`)
	out, err := ApplyPatch(doc, Patch{Op: OpReplaceBlock, ID: idA, Block: nb})
	if err != nil {
		t.Fatalf("replace_block 失败: %v", err)
	}
	if out.Children[0].Type != BlockHeading || out.Children[0].ID != idX {
		t.Fatalf("替换结果不符: %+v", out.Children[0])
	}

	// 输入文档未被修改（深拷贝语义）。
	after, err := CanonicalJSON(doc)
	if err != nil {
		t.Fatalf("CanonicalJSON 失败: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("ApplyPatch 修改了输入文档")
	}

	// 目标不存在。
	if _, err := ApplyPatch(doc, Patch{Op: OpReplaceBlock, ID: idX, Block: nb}); err == nil {
		t.Fatalf("目标不存在应报错")
	} else {
		requirePatchErr(t, err, OpReplaceBlock, "目标 Block 不存在")
	}

	// 非法新块（divider 带 content）破坏 Schema。
	bad := mustParseBlock(t, `{"id":"`+idX+`","type":"divider"}`)
	bad.Content = []byte(`"x"`)
	if _, err := ApplyPatch(doc, Patch{Op: OpReplaceBlock, ID: idA, Block: bad}); err == nil {
		t.Fatalf("非法替换块应报错")
	} else {
		requirePatchErr(t, err, OpReplaceBlock, "Schema")
	}
}

// TestApplyPatchInsert 正反样例：根部/嵌套插入；容器规则、越界、父不存在报错。
func TestApplyPatchInsert(t *testing.T) {
	doc := mustParse(t, patchTestDoc)
	nb := mustParseBlock(t, `{"id":"`+idX+`","type":"paragraph","content":[{"type":"text","text":"新段落"}]}`)

	// 根部 index=1 插入。
	out, err := ApplyPatch(doc, Patch{Op: OpInsertBlock, ParentID: "", Index: 1, Block: nb})
	if err != nil {
		t.Fatalf("根部 insert 失败: %v", err)
	}
	if len(out.Children) != 4 || out.Children[1].ID != idX {
		t.Fatalf("根部插入结果不符")
	}

	// list_item 允许任意 Block：插入 B1 末尾（index == len 为追加）。
	out, err = ApplyPatch(doc, Patch{Op: OpInsertBlock, ParentID: idB1, Index: 1, Block: nb})
	if err != nil {
		t.Fatalf("list_item insert 失败: %v", err)
	}
	loc, ok := FindBlock(out, idB1)
	if !ok || len(loc.Block.Children) != 2 || loc.Block.Children[1].ID != idX {
		t.Fatalf("list_item 插入结果不符")
	}

	// 容器规则：paragraph 直插 bullet_list 被拒绝。
	if _, err := ApplyPatch(doc, Patch{Op: OpInsertBlock, ParentID: idB, Index: 0, Block: nb}); err == nil {
		t.Fatalf("paragraph 直插 bullet_list 应报错")
	} else {
		requirePatchErr(t, err, OpInsertBlock, "Schema")
	}

	// 索引越界。
	if _, err := ApplyPatch(doc, Patch{Op: OpInsertBlock, ParentID: "", Index: 4, Block: nb}); err == nil {
		t.Fatalf("索引越界应报错")
	} else {
		requirePatchErr(t, err, OpInsertBlock, "索引越界")
	}

	// 父不存在。
	if _, err := ApplyPatch(doc, Patch{Op: OpInsertBlock, ParentID: idX, Index: 0, Block: nb}); err == nil {
		t.Fatalf("父不存在应报错")
	} else {
		requirePatchErr(t, err, OpInsertBlock, "父 Block 不存在")
	}
}

// TestApplyPatchDelete 正反样例：删除嵌套/顶层块；目标不存在报错。
func TestApplyPatchDelete(t *testing.T) {
	doc := mustParse(t, patchTestDoc)

	out, err := ApplyPatch(doc, Patch{Op: OpDeleteBlock, ID: idB2})
	if err != nil {
		t.Fatalf("delete_block 失败: %v", err)
	}
	loc, ok := FindBlock(out, idB1)
	if !ok || len(loc.Block.Children) != 0 {
		t.Fatalf("嵌套删除结果不符")
	}

	out, err = ApplyPatch(doc, Patch{Op: OpDeleteBlock, ID: idC})
	if err != nil {
		t.Fatalf("顶层删除失败: %v", err)
	}
	if len(out.Children) != 2 || out.Children[1].ID != idB {
		t.Fatalf("顶层删除结果不符")
	}

	if _, err := ApplyPatch(doc, Patch{Op: OpDeleteBlock, ID: idX}); err == nil {
		t.Fatalf("目标不存在应报错")
	} else {
		requirePatchErr(t, err, OpDeleteBlock, "目标 Block 不存在")
	}
}

// TestApplyPatchMove 正反样例：根部移动、跨父移动；容器规则、移动到自身子树报错。
func TestApplyPatchMove(t *testing.T) {
	doc := mustParse(t, patchTestDoc)

	// C 移到根部最前。
	out, err := ApplyPatch(doc, Patch{Op: OpMoveBlock, ID: idC, ParentID: "", Index: 0})
	if err != nil {
		t.Fatalf("根部 move 失败: %v", err)
	}
	if len(out.Children) != 3 || out.Children[0].ID != idC {
		t.Fatalf("根部移动结果不符")
	}

	// A（paragraph）移入 list_item B1 末尾。
	out, err = ApplyPatch(doc, Patch{Op: OpMoveBlock, ID: idA, ParentID: idB1, Index: 1})
	if err != nil {
		t.Fatalf("跨父 move 失败: %v", err)
	}
	if len(out.Children) != 2 || out.Children[0].ID != idB {
		t.Fatalf("移动后根部不符")
	}
	loc, _ := FindBlock(out, idB1)
	if len(loc.Block.Children) != 2 || loc.Block.Children[1].ID != idA {
		t.Fatalf("跨父移动结果不符")
	}

	// 容器规则：paragraph 移入 bullet_list 被拒绝。
	if _, err := ApplyPatch(doc, Patch{Op: OpMoveBlock, ID: idA, ParentID: idB, Index: 0}); err == nil {
		t.Fatalf("paragraph 移入 bullet_list 应报错")
	} else {
		requirePatchErr(t, err, OpMoveBlock, "Schema")
	}

	// 移动到自身子树：父随子树脱离，报父不存在。
	if _, err := ApplyPatch(doc, Patch{Op: OpMoveBlock, ID: idB, ParentID: idB1, Index: 0}); err == nil {
		t.Fatalf("移入自身子树应报错")
	} else {
		requirePatchErr(t, err, OpMoveBlock, "父 Block 不存在")
	}

	// 索引越界与目标不存在。
	if _, err := ApplyPatch(doc, Patch{Op: OpMoveBlock, ID: idA, ParentID: "", Index: 3}); err == nil {
		t.Fatalf("索引越界应报错")
	} else {
		requirePatchErr(t, err, OpMoveBlock, "索引越界")
	}
	if _, err := ApplyPatch(doc, Patch{Op: OpMoveBlock, ID: idX, ParentID: "", Index: 0}); err == nil {
		t.Fatalf("目标不存在应报错")
	} else {
		requirePatchErr(t, err, OpMoveBlock, "目标 Block 不存在")
	}
}

// TestApplyPatchUnknownOp 未知操作报错。
func TestApplyPatchUnknownOp(t *testing.T) {
	doc := mustParse(t, patchTestDoc)
	if _, err := ApplyPatch(doc, Patch{Op: "frob", ID: idA}); err == nil {
		t.Fatalf("未知操作应报错")
	} else {
		requirePatchErr(t, err, "frob", "未知操作")
	}
}
