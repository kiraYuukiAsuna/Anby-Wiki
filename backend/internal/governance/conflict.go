package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type ConflictService struct {
	repo      *Repository
	pages     *page.Service
	knowledge *knowledge.Service
	txm       *db.TxManager
	ids       *id.Generator
}

func NewConflictService(repo *Repository, pages *page.Service, knowledgeService *knowledge.Service, txm *db.TxManager, ids *id.Generator) *ConflictService {
	return &ConflictService{repo: repo, pages: pages, knowledge: knowledgeService, txm: txm, ids: ids}
}

// DetectAndRecord 执行 Revision / Block hash / Claim state 三层检测。只要存在冲突，
// 全部冲突与 Proposal→conflicted 在一个事务内记录，调用方不得继续 Apply。
func (s *ConflictService) DetectAndRecord(ctx context.Context, proposalID uuid.UUID) ([]MergeConflict, error) {
	p, err := s.repo.GetProposal(ctx, nil, proposalID)
	if err != nil {
		return nil, err
	}
	if p.Status != ProposalApproved {
		return nil, fmt.Errorf("%w: conflict check status=%s", ErrInvalidTransition, p.Status)
	}
	records, err := s.repo.ListOperations(ctx, nil, proposalID)
	if err != nil {
		return nil, err
	}
	var conflicts []MergeConflict
	if p.TargetType == TargetPage {
		conflicts, err = s.detectPage(ctx, p, records)
	} else {
		conflicts, err = s.detectClaims(ctx, p, records)
	}
	if err != nil || len(conflicts) == 0 {
		return conflicts, err
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		locked, err := s.repo.GetProposalForUpdate(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if locked.Status != ProposalApproved {
			return ErrInvalidTransition
		}
		for i := range conflicts {
			conflictID, err := s.ids.New()
			if err != nil {
				return err
			}
			conflicts[i].ID = conflictID
			if err := s.repo.InsertMergeConflict(ctx, tx, &conflicts[i]); err != nil {
				return err
			}
		}
		return s.repo.UpdateProposalStatus(ctx, tx, proposalID, ProposalApproved, ProposalConflicted)
	})
	return conflicts, err
}

func (s *ConflictService) detectPage(ctx context.Context, p *Proposal, records []OperationRecord) ([]MergeConflict, error) {
	if p.TargetID == nil || p.BaseRevisionID == nil {
		return []MergeConflict{{ProposalID: p.ID, ConflictType: ConflictRevision}}, nil
	}
	_, baseSnap, err := s.pages.GetRevision(ctx, *p.TargetID, *p.BaseRevisionID)
	if err != nil {
		return nil, err
	}
	currentRev, currentSnap, err := s.pages.CurrentContent(ctx, *p.TargetID)
	if err != nil {
		return nil, err
	}
	if currentRev == nil || currentRev.ID == *p.BaseRevisionID {
		return nil, nil
	}
	baseDoc, err := ast.Parse(baseSnap.AST)
	if err != nil {
		return nil, err
	}
	currentDoc, err := ast.Parse(currentSnap.AST)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]bool)
	existing, err := s.repo.ListMergeConflicts(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	for _, conflict := range existing {
		if conflict.TargetBlockID != nil && conflict.Status != ConflictOpen &&
			conflict.CurrentRevisionID != nil && *conflict.CurrentRevisionID == currentRev.ID {
			resolved[*conflict.TargetBlockID] = true
		}
	}
	var conflicts []MergeConflict
	for i := range records {
		op, err := OperationFromRecord(&records[i])
		if err != nil {
			return nil, err
		}
		if op.Target.BlockID != nil && resolved[*op.Target.BlockID] {
			continue
		}
		if op.ExpectedHash == nil || op.Target.BlockID == nil {
			conflicts = append(conflicts, MergeConflict{
				ProposalID: p.ID, PageID: p.TargetID, ConflictType: ConflictRevision,
				TargetBlockID: op.Target.BlockID, BaseRevisionID: p.BaseRevisionID,
				CurrentRevisionID: &currentRev.ID, ProposedValue: op.Payload, Status: ConflictOpen,
			})
			continue
		}
		baseLoc, baseOK := ast.FindBlock(baseDoc, *op.Target.BlockID)
		currentLoc, currentOK := ast.FindBlock(currentDoc, *op.Target.BlockID)
		if currentOK {
			currentHash, _ := BlockHash(currentLoc.Block)
			if currentHash == *op.ExpectedHash {
				continue // 其他 Block 的并发修改可三方合并。
			}
		}
		var baseJSON, currentJSON json.RawMessage
		if baseOK {
			baseJSON, _ = json.Marshal(baseLoc.Block)
		}
		if currentOK {
			currentJSON, _ = json.Marshal(currentLoc.Block)
		}
		conflicts = append(conflicts, MergeConflict{
			ProposalID: p.ID, PageID: p.TargetID, ConflictType: ConflictBlockHash,
			TargetBlockID: op.Target.BlockID, BaseRevisionID: p.BaseRevisionID,
			CurrentRevisionID: &currentRev.ID, BaseValue: baseJSON,
			CurrentValue: currentJSON, ProposedValue: op.Payload, Status: ConflictOpen,
		})
	}
	return conflicts, nil
}

func (s *ConflictService) detectClaims(ctx context.Context, p *Proposal, records []OperationRecord) ([]MergeConflict, error) {
	if s.knowledge == nil {
		return nil, nil
	}
	var conflicts []MergeConflict
	for i := range records {
		op, err := OperationFromRecord(&records[i])
		if err != nil {
			return nil, err
		}
		if op.OperationType != OpSupersedeClaim && op.OperationType != OpAddClaimSource {
			continue
		}
		if op.Target.ClaimID == nil {
			continue
		}
		claim, err := s.knowledge.GetClaim(ctx, *op.Target.ClaimID)
		if err != nil {
			return nil, err
		}
		hash, _ := ClaimHash(claim)
		if op.ExpectedHash != nil && hash == *op.ExpectedHash && claim.Status == knowledge.ClaimStatusPublished {
			continue
		}
		current, _ := json.Marshal(claim)
		conflicts = append(conflicts, MergeConflict{
			ProposalID: p.ID, ConflictType: ConflictClaimState, TargetClaimID: op.Target.ClaimID,
			CurrentValue: current, ProposedValue: op.Payload, Status: ConflictOpen,
		})
	}
	return conflicts, nil
}

// ClaimHash 是 Claim 冲突检测的稳定状态摘要；只纳入影响语义/状态机的字段。
func ClaimHash(claim *knowledge.Claim) (string, error) {
	raw, err := json.Marshal(struct {
		ID                 uuid.UUID       `json:"id"`
		SubjectEntityID    uuid.UUID       `json:"subject_entity_id"`
		PropertyID         uuid.UUID       `json:"property_id"`
		ValueJSON          json.RawMessage `json:"value"`
		Status             string          `json:"status"`
		VerificationStatus string          `json:"verification_status"`
		SupersededBy       *uuid.UUID      `json:"superseded_by"`
	}{claim.ID, claim.SubjectEntityID, claim.PropertyID, claim.ValueJSON,
		claim.Status, claim.VerificationStatus, claim.SupersededBy})
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
