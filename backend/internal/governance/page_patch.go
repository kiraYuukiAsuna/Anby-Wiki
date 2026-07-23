package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
)

var (
	ErrPatchTargetNotFound  = errors.New("governance: Patch Target 不存在")
	ErrPatchTargetModified  = errors.New("governance: Patch Target 已修改")
	ErrPatchTargetMismatch  = errors.New("governance: Patch Target 不属于当前页面")
	ErrUnsupportedOperation = errors.New("governance: Patch Engine 不支持该 Operation")
)

// PagePatchEngine 只做纯函数 AST 变换，不读写数据库。
type PagePatchEngine struct{}

func NewPagePatchEngine() *PagePatchEngine { return &PagePatchEngine{} }

// Apply 按 sequence 顺序应用页面 Operation。输入 doc 始终不被修改；任一操作失败
// 都不返回部分结果。
func (e *PagePatchEngine) Apply(doc *ast.Document, pageID uuid.UUID, operations []OperationV1) (*ast.Document, error) {
	if doc == nil {
		return nil, fmt.Errorf("%w: document=nil", ErrInvalidOperation)
	}
	current := doc
	for i := range operations {
		next, err := e.applyOne(current, pageID, &operations[i])
		if err != nil {
			return nil, fmt.Errorf("operation[%d] %s: %w", i, operations[i].OperationType, err)
		}
		current = next
	}
	return current, nil
}

func (e *PagePatchEngine) applyOne(doc *ast.Document, pageID uuid.UUID, op *OperationV1) (*ast.Document, error) {
	if op.Target.PageID == nil || *op.Target.PageID != pageID {
		return nil, ErrPatchTargetMismatch
	}
	if err := checkExpectedHash(doc, op); err != nil {
		return nil, err
	}

	switch op.OperationType {
	case OpInsertBlock:
		var p struct {
			ParentBlockID *string    `json:"parent_block_id"`
			Index         int        `json:"index"`
			Block         *ast.Block `json:"block"`
		}
		if err := decodePayload(op, &p); err != nil || p.Block == nil {
			return nil, invalidPayload(err)
		}
		parent := ""
		if p.ParentBlockID != nil {
			parent = *p.ParentBlockID
		}
		return ast.ApplyPatch(doc, ast.Patch{Op: ast.OpInsertBlock, ParentID: parent, Index: p.Index, Block: p.Block})

	case OpDeleteBlock:
		blockID, err := requiredBlockID(op)
		if err != nil {
			return nil, err
		}
		return ast.ApplyPatch(doc, ast.Patch{Op: ast.OpDeleteBlock, ID: blockID})

	case OpMoveBlock:
		blockID, err := requiredBlockID(op)
		if err != nil {
			return nil, err
		}
		var p struct {
			ParentBlockID *string `json:"parent_block_id"`
			Index         int     `json:"index"`
		}
		if err := decodePayload(op, &p); err != nil {
			return nil, err
		}
		parent := ""
		if p.ParentBlockID != nil {
			parent = *p.ParentBlockID
		}
		return ast.ApplyPatch(doc, ast.Patch{Op: ast.OpMoveBlock, ID: blockID, ParentID: parent, Index: p.Index})

	case OpReplaceBlock:
		blockID, err := requiredBlockID(op)
		if err != nil {
			return nil, err
		}
		var p struct {
			Block *ast.Block `json:"block"`
		}
		if err := decodePayload(op, &p); err != nil || p.Block == nil {
			return nil, invalidPayload(err)
		}
		return ast.ApplyPatch(doc, ast.Patch{Op: ast.OpReplaceBlock, ID: blockID, Block: p.Block})

	case OpInsertPageReference, OpInsertEntityReference, OpInsertClaimReference, OpInsertCitationReference:
		return insertReference(doc, op)

	case OpRetargetPageReference, OpRetargetEntityReference, OpRetargetClaimReference,
		OpRetargetCitationReference, OpRetargetExternalLink:
		return retargetReference(doc, op)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOperation, op.OperationType)
	}
}

func requiredBlockID(op *OperationV1) (string, error) {
	if op.Target.BlockID == nil || *op.Target.BlockID == "" {
		return "", ErrPatchTargetNotFound
	}
	if _, err := uuid.Parse(*op.Target.BlockID); err != nil {
		// AST v1 Block ID 必须是 UUID，但让 ApplyPatch 返回目标不存在会掩盖非法目标。
		return "", fmt.Errorf("%w: block_id=%q", ErrInvalidOperation, *op.Target.BlockID)
	}
	return *op.Target.BlockID, nil
}

func checkExpectedHash(doc *ast.Document, op *OperationV1) error {
	if op.ExpectedHash == nil {
		return nil
	}
	var got string
	var err error
	if op.Target.BlockID != nil {
		loc, ok := ast.FindBlock(doc, *op.Target.BlockID)
		if !ok {
			return fmt.Errorf("%w: block_id=%s", ErrPatchTargetNotFound, *op.Target.BlockID)
		}
		got, err = BlockHash(loc.Block)
	} else {
		got, err = ast.ContentHash(doc)
	}
	if err != nil {
		return err
	}
	if got != *op.ExpectedHash {
		return fmt.Errorf("%w: expected=%s current=%s", ErrPatchTargetModified, *op.ExpectedHash, got)
	}
	return nil
}

// BlockHash 返回稳定 Block JSON 的 SHA-256，用于 Block 级乐观锁。
func BlockHash(block *ast.Block) (string, error) {
	raw, err := json.Marshal(block)
	if err != nil {
		return "", err
	}
	canonical, err := ast.CanonicalizeJSON(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func insertReference(doc *ast.Document, op *OperationV1) (*ast.Document, error) {
	blockID, err := requiredBlockID(op)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Index                int        `json:"index"`
		TargetPageID         *uuid.UUID `json:"target_page_id"`
		TargetHeadingBlockID string     `json:"target_heading_block_id"`
		TargetNamespace      string     `json:"target_namespace"`
		NormalizedTitle      string     `json:"normalized_title"`
		ExpectedEntityType   string     `json:"expected_entity_type"`
		DisplayText          string     `json:"display_text"`
	}
	if err := decodePayload(op, &payload); err != nil {
		return nil, err
	}
	var node *ast.InlineNode
	switch op.OperationType {
	case OpInsertPageReference:
		if payload.TargetPageID != nil {
			node = &ast.InlineNode{Type: ast.InlinePageReference, TargetPageID: payload.TargetPageID.String(),
				TargetHeadingBlockID: payload.TargetHeadingBlockID, DisplayText: payload.DisplayText}
		} else if payload.TargetNamespace != "" && payload.NormalizedTitle != "" {
			node = &ast.InlineNode{Type: ast.InlinePageReference, ResolutionStatus: ast.ResolutionUnresolved,
				TargetNamespace: payload.TargetNamespace, NormalizedTitle: payload.NormalizedTitle,
				ExpectedEntityType: payload.ExpectedEntityType}
		} else {
			return nil, ErrInvalidOperation
		}
	case OpInsertEntityReference:
		if op.Target.EntityID == nil {
			return nil, ErrInvalidOperation
		}
		node, err = ast.NewEntityRefNode(op.Target.EntityID.String(), payload.DisplayText)
	case OpInsertClaimReference:
		if op.Target.ClaimID == nil {
			return nil, ErrInvalidOperation
		}
		node, err = ast.NewClaimRefNode(op.Target.ClaimID.String(), payload.DisplayText)
	case OpInsertCitationReference:
		if op.Target.CitationID == nil {
			return nil, ErrInvalidOperation
		}
		node, err = ast.NewCitationRefNode(op.Target.CitationID.String(), payload.DisplayText)
	}
	if err != nil {
		return nil, err
	}
	return mutateInline(doc, blockID, payload.Index, false, "", node)
}

func retargetReference(doc *ast.Document, op *OperationV1) (*ast.Document, error) {
	blockID, err := requiredBlockID(op)
	if err != nil {
		return nil, err
	}
	if op.Target.NodeID == nil {
		return nil, ErrPatchTargetNotFound
	}
	index, err := strconv.Atoi(*op.Target.NodeID)
	if err != nil || index < 0 {
		return nil, fmt.Errorf("%w: node_id=%q", ErrInvalidOperation, *op.Target.NodeID)
	}
	var p struct {
		TargetPageID         *uuid.UUID `json:"target_page_id"`
		TargetHeadingBlockID string     `json:"target_heading_block_id"`
		DisplayText          string     `json:"display_text"`
		URL                  string     `json:"url"`
	}
	if err := decodePayload(op, &p); err != nil {
		return nil, err
	}
	var want ast.InlineType
	var replacement *ast.InlineNode
	switch op.OperationType {
	case OpRetargetPageReference:
		want = ast.InlinePageReference
		if p.TargetPageID == nil {
			return nil, ErrInvalidOperation
		}
		replacement = &ast.InlineNode{Type: want, TargetPageID: p.TargetPageID.String(),
			TargetHeadingBlockID: p.TargetHeadingBlockID, DisplayText: p.DisplayText}
	case OpRetargetEntityReference:
		want = ast.InlineEntityReference
		if op.Target.EntityID == nil {
			return nil, ErrInvalidOperation
		}
		replacement, err = ast.NewEntityRefNode(op.Target.EntityID.String(), p.DisplayText)
	case OpRetargetClaimReference:
		want = ast.InlineClaimReference
		if op.Target.ClaimID == nil {
			return nil, ErrInvalidOperation
		}
		replacement, err = ast.NewClaimRefNode(op.Target.ClaimID.String(), p.DisplayText)
	case OpRetargetCitationReference:
		want = ast.InlineCitationReference
		if op.Target.CitationID == nil {
			return nil, ErrInvalidOperation
		}
		replacement, err = ast.NewCitationRefNode(op.Target.CitationID.String(), p.DisplayText)
	case OpRetargetExternalLink:
		want = ast.InlineExternalLink
		if p.URL == "" {
			return nil, ErrInvalidOperation
		}
		replacement = &ast.InlineNode{Type: want, URL: p.URL, DisplayText: p.DisplayText}
	}
	if err != nil {
		return nil, err
	}
	return mutateInline(doc, blockID, index, true, want, replacement)
}

func mutateInline(doc *ast.Document, blockID string, index int, replace bool, want ast.InlineType, node *ast.InlineNode) (*ast.Document, error) {
	loc, ok := ast.FindBlock(doc, blockID)
	if !ok {
		return nil, fmt.Errorf("%w: block_id=%s", ErrPatchTargetNotFound, blockID)
	}
	nodes, err := loc.Block.InlineContent()
	if err != nil {
		return nil, err
	}
	if replace {
		if index < 0 || index >= len(nodes) {
			return nil, fmt.Errorf("%w: node_id=%d", ErrPatchTargetNotFound, index)
		}
		if nodes[index].Type != want {
			return nil, fmt.Errorf("%w: node type=%s want=%s", ErrPatchTargetModified, nodes[index].Type, want)
		}
		nodes[index] = node
	} else {
		if index < 0 || index > len(nodes) {
			return nil, fmt.Errorf("%w: node index=%d", ErrPatchTargetNotFound, index)
		}
		nodes = append(nodes, nil)
		copy(nodes[index+1:], nodes[index:])
		nodes[index] = node
	}
	content, err := json.Marshal(nodes)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(loc.Block)
	if err != nil {
		return nil, err
	}
	var block ast.Block
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, err
	}
	block.Content = content
	return ast.ApplyPatch(doc, ast.Patch{Op: ast.OpReplaceBlock, ID: blockID, Block: &block})
}

func decodePayload(op *OperationV1, out any) error {
	if err := json.Unmarshal(op.Payload, out); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOperation, err)
	}
	return nil
}

func invalidPayload(err error) error {
	if err != nil {
		return err
	}
	return ErrInvalidOperation
}
