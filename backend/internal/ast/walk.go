// AST 遍历与定位。
package ast

import "fmt"

// WalkNode 描述 Walk 访问到的单个节点：Block 与 Inline 恰有一个非 nil。
type WalkNode struct {
	// Block 非 nil 时当前节点是 Block；Inline 非 nil 时当前节点是 InlineNode。
	Block  *Block
	Inline *InlineNode
	// Parent 是父 Block：Block 的父 Block（顶层为 nil）；Inline 所属的 Block。
	Parent *Block
	// Index 是节点在父容器中的下标（父 children 或父 content）。
	Index int
	// Depth 是 Block 嵌套深度（顶层为 0）；Inline 与所属 Block 同深度。
	Depth int
}

// Walk 深度优先（前序）遍历 doc 中所有 Block 与 InlineNode：
// 每个 Block 先访问自身，再依次访问其行内 content（heading/paragraph），
// 最后递归访问 children。fn 返回 false 时中止整个遍历。
// Block 行内 content 无法解码时返回错误（已通过 Validate 的文档不会触发）。
func Walk(doc *Document, fn func(n WalkNode) bool) error {
	if doc == nil {
		return fmt.Errorf("ast: Walk 的 doc 为 nil")
	}
	w := &walker{fn: fn}
	w.blocks(doc.Children, nil, 0)
	return w.err
}

type walker struct {
	fn      func(WalkNode) bool
	err     error
	stopped bool
}

func (w *walker) blocks(blocks []*Block, parent *Block, depth int) {
	for i, b := range blocks {
		if w.stopped || w.err != nil {
			return
		}
		if !w.fn(WalkNode{Block: b, Parent: parent, Index: i, Depth: depth}) {
			w.stopped = true
			return
		}
		w.inline(b, depth)
		w.blocks(b.Children, b, depth+1)
	}
}

func (w *walker) inline(b *Block, depth int) {
	if len(b.Content) == 0 || b.Content[0] != '[' {
		return // code 的字符串 content 或无 content，无行内节点
	}
	nodes, err := b.InlineContent()
	if err != nil {
		w.err = err
		return
	}
	for i, n := range nodes {
		if w.stopped {
			return
		}
		if !w.fn(WalkNode{Inline: n, Parent: b, Index: i, Depth: depth}) {
			w.stopped = true
			return
		}
	}
}

// Location 是 FindBlock 的定位结果。
type Location struct {
	Block *Block
	// Parent 是父 Block；nil 表示 Block 位于文档顶层。
	Parent *Block
	// Index 是 Block 在父 children（或文档顶层 children）中的下标。
	Index int
	// Path 是从文档顶层起的索引路径，最后一个元素等于 Index。
	Path []int
}

// FindBlock 按 ID 深度优先定位 Block，未找到返回 (nil, false)。
func FindBlock(doc *Document, id string) (*Location, bool) {
	if doc == nil {
		return nil, false
	}
	return findIn(doc.Children, nil, nil, id)
}

func findIn(blocks []*Block, parent *Block, prefix []int, id string) (*Location, bool) {
	for i, b := range blocks {
		path := append(append([]int{}, prefix...), i)
		if b.ID == id {
			return &Location{Block: b, Parent: parent, Index: i, Path: path}, true
		}
		if loc, ok := findIn(b.Children, b, path, id); ok {
			return loc, true
		}
	}
	return nil, false
}

// Blocks 按 Walk 的前序顺序收集 doc 中所有 Block 的平铺列表。
func Blocks(doc *Document) []*Block {
	if doc == nil {
		return nil
	}
	var out []*Block
	var collect func(blocks []*Block)
	collect = func(blocks []*Block) {
		for _, b := range blocks {
			out = append(out, b)
			collect(b.Children)
		}
	}
	collect(doc.Children)
	return out
}
