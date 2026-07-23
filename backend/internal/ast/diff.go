// 结构 Diff：按 Block ID 对齐 base 与 current。
//
// 语义：
//   - added/removed：ID 只在单侧出现（子树随父级增删时，父级与子级各自出现一条）；
//   - changed：同一 ID 的 Block 自身字段（type/level/kind/language/content 等，
//     不含 children）发生变化，附字段级 before/after 摘要（值的 canonical JSON）；
//   - moved：同一 ID 的 Block 父级或下标变化；
//   - ID 相同且位置未变的 Block 不产生任何条目。
//
// 假定 Block ID 在文档内唯一（与 ADR-0008 的编辑器生成约定一致）。
package ast

import (
	"encoding/json"
	"fmt"
	"sort"
)

// ChangeType 是 Diff 条目的变更类别。
type ChangeType string

const (
	ChangeAdded   ChangeType = "added"
	ChangeRemoved ChangeType = "removed"
	ChangeChanged ChangeType = "changed"
	ChangeMoved   ChangeType = "moved"
)

// FieldChange 是单字段的 before/after 摘要；Before/After 是字段值的
// canonical JSON 文本，字段在单侧不存在时为 "(absent)"。
type FieldChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// BlockChange 是一条 Block 级变更。同一 Block 字段与位置都变时
// 产生 changed 与 moved 两条独立条目。
type BlockChange struct {
	Type    ChangeType `json:"type"`
	BlockID string     `json:"block_id"`
	// ParentID 是变更后（added/changed/moved）或变更前（removed）的父 Block ID，
	// 顶层为空串。
	ParentID string `json:"parent_id"`
	// Path 是变更后（added）或变更前（removed）的索引路径。
	Path []int `json:"path,omitempty"`
	// BeforePath/AfterPath 仅 moved 使用。
	BeforePath []int `json:"before_path,omitempty"`
	AfterPath  []int `json:"after_path,omitempty"`
	// Fields 仅 changed 使用，按字段名排序。
	Fields []FieldChange `json:"fields,omitempty"`
}

// DocumentDiff 是 Diff 的输出，可 JSON 序列化。Changes 按 current 前序
// （removed 按 base 前序追加在后）排列，确定性输出。
type DocumentDiff struct {
	Changes []BlockChange `json:"changes"`
}

type blockInfo struct {
	block    *Block
	parentID string // 顶层为 ""
	path     []int
}

// Diff 对齐 base 与 current，报告 Block 级 added/removed/changed/moved。
func Diff(base, current *Document) (*DocumentDiff, error) {
	if base == nil || current == nil {
		return nil, fmt.Errorf("ast: Diff 的输入文档不能为 nil")
	}
	baseIdx := indexByID(base)
	curIdx := indexByID(current)

	d := &DocumentDiff{Changes: []BlockChange{}}
	// 前序遍历 current：added / changed / moved。
	var walkCur func(blocks []*Block, parentID string) error
	walkCur = func(blocks []*Block, parentID string) error {
		for _, b := range blocks {
			cur := curIdx[b.ID]
			old, ok := baseIdx[b.ID]
			if !ok {
				d.Changes = append(d.Changes, BlockChange{
					Type:     ChangeAdded,
					BlockID:  b.ID,
					ParentID: parentID,
					Path:     cur.path,
				})
			} else {
				fields, err := diffSelfFields(old.block, b)
				if err != nil {
					return err
				}
				if len(fields) > 0 {
					d.Changes = append(d.Changes, BlockChange{
						Type:     ChangeChanged,
						BlockID:  b.ID,
						ParentID: parentID,
						Fields:   fields,
					})
				}
				if old.parentID != cur.parentID || !pathEqual(old.path, cur.path) {
					d.Changes = append(d.Changes, BlockChange{
						Type:       ChangeMoved,
						BlockID:    b.ID,
						ParentID:   parentID,
						BeforePath: old.path,
						AfterPath:  cur.path,
					})
				}
			}
			if err := walkCur(b.Children, b.ID); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walkCur(current.Children, ""); err != nil {
		return nil, err
	}

	// 前序遍历 base：removed。
	var walkBase func(blocks []*Block, parentID string)
	walkBase = func(blocks []*Block, parentID string) {
		for _, b := range blocks {
			if _, ok := curIdx[b.ID]; !ok {
				d.Changes = append(d.Changes, BlockChange{
					Type:     ChangeRemoved,
					BlockID:  b.ID,
					ParentID: parentID,
					Path:     baseIdx[b.ID].path,
				})
			}
			walkBase(b.Children, b.ID)
		}
	}
	walkBase(base.Children, "")
	return d, nil
}

func indexByID(doc *Document) map[string]blockInfo {
	idx := make(map[string]blockInfo)
	var walk func(blocks []*Block, parentID string, prefix []int)
	walk = func(blocks []*Block, parentID string, prefix []int) {
		for i, b := range blocks {
			path := append(append([]int{}, prefix...), i)
			idx[b.ID] = blockInfo{block: b, parentID: parentID, path: path}
			walk(b.Children, b.ID, path)
		}
	}
	walk(doc.Children, "", nil)
	return idx
}

func pathEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// selfFields 返回 Block 自身字段（去掉 children）的 JSON 投影。
func selfFields(b *Block) (map[string]any, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("ast: 序列化 Block %s 失败: %w", b.ID, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("ast: 解码 Block %s 失败: %w", b.ID, err)
	}
	delete(m, "children")
	return m, nil
}

// diffSelfFields 比较两个同 ID Block 的自身字段，返回字段级差异（按字段名排序）。
func diffSelfFields(before, after *Block) ([]FieldChange, error) {
	bm, err := selfFields(before)
	if err != nil {
		return nil, err
	}
	am, err := selfFields(after)
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for k := range bm {
		keys[k] = true
	}
	for k := range am {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	var out []FieldChange
	for _, k := range sorted {
		bv, bok := bm[k]
		av, aok := am[k]
		bc, err := canonicalValue(bv, bok)
		if err != nil {
			return nil, err
		}
		ac, err := canonicalValue(av, aok)
		if err != nil {
			return nil, err
		}
		if bc != ac {
			out = append(out, FieldChange{Field: k, Before: bc, After: ac})
		}
	}
	return out, nil
}

func canonicalValue(v any, present bool) (string, error) {
	if !present {
		return "(absent)", nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("ast: 序列化字段值失败: %w", err)
	}
	canon, err := CanonicalizeJSON(raw)
	if err != nil {
		return "", err
	}
	return string(canon), nil
}
