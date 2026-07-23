// 知识引用（EntityReference / ClaimReference / CitationReference，M4-T06）的
// 行内节点构造器与插入编辑命令：纯函数，供 M5 Patch Engine 与 AI 使用。
//
// 两层 API：
//   - NewXxxRefNode：构造并校验单个行内节点（ID 必须是合法 UUID，
//     entity/claim 的 display_text 非空，citation 的 display_text 可省略）；
//   - InsertXxxRefPatch / InsertXxxRef：在 paragraph/heading Block 的
//     content 指定 index 处插入引用节点。Patch 变体返回 M1-T02 的
//     replace_block Patch 供组合；Insert 变体直接经 ApplyPatch 应用
//     （输入文档不被修改，结果经 Schema 校验）。
package ast

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// NewEntityRefNode 构造 entity_reference 行内节点（设计 §5.3）。
func NewEntityRefNode(entityID, displayText string) (*InlineNode, error) {
	if err := checkRefID("entity_id", entityID); err != nil {
		return nil, err
	}
	if err := checkRefDisplayText(displayText); err != nil {
		return nil, err
	}
	return &InlineNode{Type: InlineEntityReference, EntityID: entityID, DisplayText: displayText}, nil
}

// NewClaimRefNode 构造 claim_reference 行内节点（设计 §5.4）。
// display_text 是渲染兜底文本（v1 渲染 display_text；实时取值属 M9）。
func NewClaimRefNode(claimID, displayText string) (*InlineNode, error) {
	if err := checkRefID("claim_id", claimID); err != nil {
		return nil, err
	}
	if err := checkRefDisplayText(displayText); err != nil {
		return nil, err
	}
	return &InlineNode{Type: InlineClaimReference, ClaimID: claimID, DisplayText: displayText}, nil
}

// NewCitationRefNode 构造 citation_reference 行内节点（设计 §5.5）。
// displayText 可空（渲染层 title 回退为 citation 短 id）。
func NewCitationRefNode(citationID, displayText string) (*InlineNode, error) {
	if err := checkRefID("citation_id", citationID); err != nil {
		return nil, err
	}
	return &InlineNode{Type: InlineCitationReference, CitationID: citationID, DisplayText: displayText}, nil
}

func checkRefID(field, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("ast: %s 不是合法 UUID %q: %w", field, id, err)
	}
	return nil
}

func checkRefDisplayText(displayText string) error {
	if strings.TrimSpace(displayText) == "" {
		return fmt.Errorf("ast: display_text 不能为空")
	}
	return nil
}

// InsertEntityRefPatch 构建在 blockID（必须是 paragraph/heading）的 content
// 第 index 处插入 entity_reference 的 replace_block Patch。
func InsertEntityRefPatch(doc *Document, blockID string, index int, entityID, displayText string) (Patch, error) {
	node, err := NewEntityRefNode(entityID, displayText)
	if err != nil {
		return Patch{}, err
	}
	return insertInlineNodePatch(doc, blockID, index, node)
}

// InsertClaimRefPatch 同 InsertEntityRefPatch，插入 claim_reference。
func InsertClaimRefPatch(doc *Document, blockID string, index int, claimID, displayText string) (Patch, error) {
	node, err := NewClaimRefNode(claimID, displayText)
	if err != nil {
		return Patch{}, err
	}
	return insertInlineNodePatch(doc, blockID, index, node)
}

// InsertCitationRefPatch 同 InsertEntityRefPatch，插入 citation_reference（displayText 可空）。
func InsertCitationRefPatch(doc *Document, blockID string, index int, citationID, displayText string) (Patch, error) {
	node, err := NewCitationRefNode(citationID, displayText)
	if err != nil {
		return Patch{}, err
	}
	return insertInlineNodePatch(doc, blockID, index, node)
}

// InsertEntityRef 直接应用 InsertEntityRefPatch，返回新文档（doc 不被修改）。
func InsertEntityRef(doc *Document, blockID string, index int, entityID, displayText string) (*Document, error) {
	p, err := InsertEntityRefPatch(doc, blockID, index, entityID, displayText)
	if err != nil {
		return nil, err
	}
	return ApplyPatch(doc, p)
}

// InsertClaimRef 直接应用 InsertClaimRefPatch，返回新文档（doc 不被修改）。
func InsertClaimRef(doc *Document, blockID string, index int, claimID, displayText string) (*Document, error) {
	p, err := InsertClaimRefPatch(doc, blockID, index, claimID, displayText)
	if err != nil {
		return nil, err
	}
	return ApplyPatch(doc, p)
}

// InsertCitationRef 直接应用 InsertCitationRefPatch，返回新文档（doc 不被修改）。
func InsertCitationRef(doc *Document, blockID string, index int, citationID, displayText string) (*Document, error) {
	p, err := InsertCitationRefPatch(doc, blockID, index, citationID, displayText)
	if err != nil {
		return nil, err
	}
	return ApplyPatch(doc, p)
}

// insertInlineNodePatch 在 paragraph/heading Block 的 content 第 index 处
// 插入行内节点，返回 replace_block Patch（Block 为原 Block 的深拷贝 + 新 content）。
func insertInlineNodePatch(doc *Document, blockID string, index int, node *InlineNode) (Patch, error) {
	if doc == nil {
		return Patch{}, patchErrorf(OpReplaceBlock, "输入文档为 nil")
	}
	loc, ok := FindBlock(doc, blockID)
	if !ok {
		return Patch{}, patchErrorf(OpReplaceBlock, "目标 Block 不存在: %s", blockID)
	}
	blk := loc.Block
	if blk.Type != BlockParagraph && blk.Type != BlockHeading {
		return Patch{}, patchErrorf(OpReplaceBlock,
			"目标 Block %s 类型 %q 不持有行内 content（仅 paragraph/heading）", blockID, blk.Type)
	}
	nodes, err := blk.InlineContent()
	if err != nil {
		return Patch{}, &PatchError{Op: OpReplaceBlock, Reason: "解码目标 Block 行内 content 失败", Err: err}
	}
	if index < 0 || index > len(nodes) {
		return Patch{}, patchErrorf(OpReplaceBlock,
			"索引越界: index=%d, content 长度=%d", index, len(nodes))
	}
	nodes = append(nodes, nil)
	copy(nodes[index+1:], nodes[index:])
	nodes[index] = node

	content, err := json.Marshal(nodes)
	if err != nil {
		return Patch{}, &PatchError{Op: OpReplaceBlock, Reason: "序列化行内 content 失败", Err: err}
	}
	nb, err := cloneBlock(blk)
	if err != nil {
		return Patch{}, &PatchError{Op: OpReplaceBlock, Reason: "深拷贝目标 Block 失败", Err: err}
	}
	nb.Content = content
	return Patch{Op: OpReplaceBlock, ID: blockID, Block: nb}, nil
}
