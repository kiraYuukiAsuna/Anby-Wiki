// 纯函数 Patch：v1 最小操作集。
//
// ApplyPatch 不修改输入文档（深拷贝语义），返回应用后的新文档；
// 结果经 Validate 校验，容器规则（如 list 只容 list_item）由 Schema 裁决。
package ast

import (
	"encoding/json"
	"fmt"
)

// PatchOpType 是 v1 支持的 Patch 操作。
type PatchOpType string

const (
	// OpReplaceBlock 用 Block 整体替换 ID 指定的 Block（ID 可变）。
	OpReplaceBlock PatchOpType = "replace_block"
	// OpInsertBlock 在 ParentID（空串表示文档顶层）的 Index 处插入 Block。
	OpInsertBlock PatchOpType = "insert_block"
	// OpDeleteBlock 删除 ID 指定的 Block 及其子树。
	OpDeleteBlock PatchOpType = "delete_block"
	// OpMoveBlock 将 ID 指定的 Block 移动到 ParentID（空串表示文档顶层）的
	// Index 处；Index 指摘除该 Block 之后新父 children 中的目标下标。
	OpMoveBlock PatchOpType = "move_block"
)

// Patch 是单个 Patch 操作。各操作使用的字段：
//   - replace_block：ID + Block；
//   - insert_block：ParentID + Index + Block；
//   - delete_block：ID；
//   - move_block：ID + ParentID + Index。
type Patch struct {
	Op       PatchOpType
	ID       string
	ParentID string
	Index    int
	Block    *Block
}

// PatchError 是 ApplyPatch 返回的错误，带操作与原因。
type PatchError struct {
	Op     PatchOpType
	Reason string
	// Err 是底层错误（如结果未通过 Schema 校验），可为 nil。
	Err error
}

func (e *PatchError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("ast: patch %s 失败: %s: %v", e.Op, e.Reason, e.Err)
	}
	return fmt.Sprintf("ast: patch %s 失败: %s", e.Op, e.Reason)
}

func (e *PatchError) Unwrap() error { return e.Err }

func patchErrorf(op PatchOpType, format string, args ...any) *PatchError {
	return &PatchError{Op: op, Reason: fmt.Sprintf(format, args...)}
}

// ApplyPatch 将 p 应用到 doc 的深拷贝上，返回新文档；doc 本身不被修改。
// 目标/父 Block 不存在、索引越界、结果破坏容器规则或 Schema 时返回 *PatchError。
func ApplyPatch(doc *Document, p Patch) (*Document, error) {
	if doc == nil {
		return nil, patchErrorf(p.Op, "输入文档为 nil")
	}
	out, err := cloneDocument(doc)
	if err != nil {
		return nil, &PatchError{Op: p.Op, Reason: "深拷贝输入文档失败", Err: err}
	}

	switch p.Op {
	case OpReplaceBlock:
		err = applyReplace(out, p)
	case OpInsertBlock:
		err = applyInsert(out, p)
	case OpDeleteBlock:
		err = applyDelete(out, p)
	case OpMoveBlock:
		err = applyMove(out, p)
	default:
		err = patchErrorf(p.Op, "未知操作 %q", string(p.Op))
	}
	if err != nil {
		return nil, err
	}

	if err := Validate(out); err != nil {
		return nil, &PatchError{Op: p.Op, Reason: "结果未通过 AST v1 Schema 校验", Err: err}
	}
	return out, nil
}

func applyReplace(doc *Document, p Patch) error {
	if p.Block == nil {
		return patchErrorf(p.Op, "缺少替换 Block")
	}
	loc, ok := FindBlock(doc, p.ID)
	if !ok {
		return patchErrorf(p.Op, "目标 Block 不存在: %s", p.ID)
	}
	nb, err := cloneBlock(p.Block)
	if err != nil {
		return &PatchError{Op: p.Op, Reason: "深拷贝替换 Block 失败", Err: err}
	}
	(*siblingsOf(doc, loc))[loc.Index] = nb
	return nil
}

func applyInsert(doc *Document, p Patch) error {
	if p.Block == nil {
		return patchErrorf(p.Op, "缺少插入 Block")
	}
	parent, err := findParent(doc, p.Op, p.ParentID)
	if err != nil {
		return err
	}
	children := childrenOf(doc, parent)
	if p.Index < 0 || p.Index > len(*children) {
		return patchErrorf(p.Op, "索引越界: index=%d, children 长度=%d", p.Index, len(*children))
	}
	nb, err := cloneBlock(p.Block)
	if err != nil {
		return &PatchError{Op: p.Op, Reason: "深拷贝插入 Block 失败", Err: err}
	}
	*children = insertAt(*children, p.Index, nb)
	return nil
}

func applyDelete(doc *Document, p Patch) error {
	loc, ok := FindBlock(doc, p.ID)
	if !ok {
		return patchErrorf(p.Op, "目标 Block 不存在: %s", p.ID)
	}
	siblings := siblingsOf(doc, loc)
	*siblings = append((*siblings)[:loc.Index], (*siblings)[loc.Index+1:]...)
	return nil
}

func applyMove(doc *Document, p Patch) error {
	loc, ok := FindBlock(doc, p.ID)
	if !ok {
		return patchErrorf(p.Op, "目标 Block 不存在: %s", p.ID)
	}
	// 先摘除：移动到自身子树内时，目标父已随子树脱离文档，findParent 报父不存在。
	moving := loc.Block
	siblings := siblingsOf(doc, loc)
	*siblings = append((*siblings)[:loc.Index], (*siblings)[loc.Index+1:]...)

	parent, err := findParent(doc, p.Op, p.ParentID)
	if err != nil {
		return err
	}
	children := childrenOf(doc, parent)
	if p.Index < 0 || p.Index > len(*children) {
		return patchErrorf(p.Op, "索引越界: index=%d, children 长度=%d", p.Index, len(*children))
	}
	*children = insertAt(*children, p.Index, moving)
	return nil
}

// findParent 按 ParentID 定位父 Block；空串表示文档顶层（返回 nil parent）。
func findParent(doc *Document, op PatchOpType, parentID string) (*Block, error) {
	if parentID == "" {
		return nil, nil
	}
	loc, ok := FindBlock(doc, parentID)
	if !ok {
		return nil, patchErrorf(op, "父 Block 不存在: %s", parentID)
	}
	return loc.Block, nil
}

// childrenOf 返回父 children 切片的指针；parent 为 nil 时指向文档顶层。
func childrenOf(doc *Document, parent *Block) *[]*Block {
	if parent == nil {
		return &doc.Children
	}
	return &parent.Children
}

// siblingsOf 返回 loc.Block 所在 children 切片的指针。
func siblingsOf(doc *Document, loc *Location) *[]*Block {
	return childrenOf(doc, loc.Parent)
}

func insertAt(s []*Block, i int, b *Block) []*Block {
	s = append(s, nil)
	copy(s[i+1:], s[i:])
	s[i] = b
	return s
}

func cloneDocument(doc *Document) (*Document, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("ast: 序列化文档失败: %w", err)
	}
	var out Document
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ast: 解码文档失败: %w", err)
	}
	return &out, nil
}

func cloneBlock(b *Block) (*Block, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("ast: 序列化 Block 失败: %w", err)
	}
	var out Block
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ast: 解码 Block 失败: %w", err)
	}
	return &out, nil
}
