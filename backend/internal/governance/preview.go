package governance

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/page"
)

type PreviewService struct {
	governance *Repository
	pages      *page.Service
	pagePatch  *PagePatchEngine
}

func NewPreviewService(repo *Repository, pages *page.Service, pagePatch *PagePatchEngine) *PreviewService {
	return &PreviewService{governance: repo, pages: pages, pagePatch: pagePatch}
}

type PreviewDocument struct {
	RevisionID  *uuid.UUID      `json:"revision_id"`
	ContentHash string          `json:"content_hash"`
	AST         json.RawMessage `json:"ast"`
}

type PreviewImpact struct {
	OperationCount int `json:"operation_count"`
	AddedBlocks    int `json:"added_blocks"`
	RemovedBlocks  int `json:"removed_blocks"`
	ChangedBlocks  int `json:"changed_blocks"`
	MovedBlocks    int `json:"moved_blocks"`
}

type ProposalPreview struct {
	ProposalID     uuid.UUID           `json:"proposal_id"`
	TargetType     string              `json:"target_type"`
	RiskLevel      string              `json:"risk_level"`
	Stale          bool                `json:"stale"`
	Base           PreviewDocument     `json:"base"`
	Current        PreviewDocument     `json:"current"`
	Proposed       PreviewDocument     `json:"proposed"`
	BaseToCurrent  ast.DocumentDiff    `json:"base_to_current"`
	BaseToProposed ast.DocumentDiff    `json:"base_to_proposed"`
	Evidence       []OperationEvidence `json:"evidence"`
	Impact         PreviewImpact       `json:"impact"`
}

// PreviewPageProposal 仅执行只读查询和纯函数 Patch；不会创建 Revision/Claim、
// Audit/Outbox 或移动 current pointer。
func (s *PreviewService) PreviewPageProposal(ctx context.Context, proposalID uuid.UUID) (*ProposalPreview, error) {
	p, err := s.governance.GetProposal(ctx, nil, proposalID)
	if err != nil {
		return nil, err
	}
	if p.TargetType != TargetPage || p.TargetID == nil || p.BaseRevisionID == nil {
		return nil, fmt.Errorf("%w: 页面预览要求 target_id 与 base_revision_id", ErrInvalidProposal)
	}
	baseRev, baseSnap, err := s.pages.GetRevision(ctx, *p.TargetID, *p.BaseRevisionID)
	if err != nil {
		return nil, err
	}
	currentRev, currentSnap, err := s.pages.CurrentContent(ctx, *p.TargetID)
	if err != nil {
		return nil, err
	}
	if currentRev == nil || currentSnap == nil {
		return nil, fmt.Errorf("%w: 页面尚未发布", ErrInvalidProposal)
	}
	baseDoc, err := ast.Parse(baseSnap.AST)
	if err != nil {
		return nil, err
	}
	currentDoc, err := ast.Parse(currentSnap.AST)
	if err != nil {
		return nil, err
	}
	records, err := s.governance.ListOperations(ctx, nil, proposalID)
	if err != nil {
		return nil, err
	}
	ops := make([]OperationV1, len(records))
	var evidence []OperationEvidence
	for i := range records {
		op, err := OperationFromRecord(&records[i])
		if err != nil {
			return nil, err
		}
		ops[i] = *op
		evidence = append(evidence, op.Evidence...)
	}
	proposedDoc, err := s.pagePatch.Apply(baseDoc, *p.TargetID, ops)
	if err != nil {
		return nil, err
	}
	proposedAST, err := ast.CanonicalJSON(proposedDoc)
	if err != nil {
		return nil, err
	}
	proposedHash, err := ast.ContentHash(proposedDoc)
	if err != nil {
		return nil, err
	}
	baseCurrent, err := ast.Diff(baseDoc, currentDoc)
	if err != nil {
		return nil, err
	}
	baseProposed, err := ast.Diff(baseDoc, proposedDoc)
	if err != nil {
		return nil, err
	}
	impact := PreviewImpact{OperationCount: len(records)}
	for _, change := range baseProposed.Changes {
		switch change.Type {
		case ast.ChangeAdded:
			impact.AddedBlocks++
		case ast.ChangeRemoved:
			impact.RemovedBlocks++
		case ast.ChangeChanged:
			impact.ChangedBlocks++
		case ast.ChangeMoved:
			impact.MovedBlocks++
		}
	}
	return &ProposalPreview{
		ProposalID: proposalID, TargetType: p.TargetType, RiskLevel: p.RiskLevel,
		Stale:         currentRev.ID != baseRev.ID,
		Base:          PreviewDocument{RevisionID: &baseRev.ID, ContentHash: baseSnap.ContentHash, AST: baseSnap.AST},
		Current:       PreviewDocument{RevisionID: &currentRev.ID, ContentHash: currentSnap.ContentHash, AST: currentSnap.AST},
		Proposed:      PreviewDocument{RevisionID: nil, ContentHash: proposedHash, AST: proposedAST},
		BaseToCurrent: *baseCurrent, BaseToProposed: *baseProposed,
		Evidence: evidence, Impact: impact,
	}, nil
}

func OperationFromRecord(r *OperationRecord) (*OperationV1, error) {
	var base OperationBase
	var target OperationTarget
	var evidence []OperationEvidence
	var risk OperationRisk
	if err := json.Unmarshal(r.Base, &base); err != nil {
		return nil, fmt.Errorf("%w: base: %v", ErrInvalidOperation, err)
	}
	if err := json.Unmarshal(r.Target, &target); err != nil {
		return nil, fmt.Errorf("%w: target: %v", ErrInvalidOperation, err)
	}
	if err := json.Unmarshal(r.Evidence, &evidence); err != nil {
		return nil, fmt.Errorf("%w: evidence: %v", ErrInvalidOperation, err)
	}
	if err := json.Unmarshal(r.Risk, &risk); err != nil {
		return nil, fmt.Errorf("%w: risk: %v", ErrInvalidOperation, err)
	}
	return &OperationV1{
		SchemaVersion: r.SchemaVersion, OperationType: r.OperationType,
		Base: base, Target: target, ExpectedHash: r.ExpectedHash,
		Evidence: evidence, Risk: risk, Payload: r.Payload,
	}, nil
}
