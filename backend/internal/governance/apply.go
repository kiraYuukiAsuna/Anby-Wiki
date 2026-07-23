package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var ErrMergeConflict = errors.New("governance: Proposal 存在合并冲突")

type ApplyService struct {
	repo      *Repository
	pages     *page.Service
	pagePatch *PagePatchEngine
	knowledge *KnowledgePatchEngine
	conflicts *ConflictService
	txm       *db.TxManager
	ids       *id.Generator
	auth      *AuthorizationService
}

func (s *ApplyService) WithAuthorization(auth *AuthorizationService) *ApplyService {
	s.auth = auth
	return s
}

func (s *ApplyService) Conflicts() *ConflictService {
	return s.conflicts
}

func NewApplyService(repo *Repository, pages *page.Service, pagePatch *PagePatchEngine,
	knowledge *KnowledgePatchEngine, conflicts *ConflictService, txm *db.TxManager, ids *id.Generator) *ApplyService {
	return &ApplyService{repo: repo, pages: pages, pagePatch: pagePatch,
		knowledge: knowledge, conflicts: conflicts, txm: txm, ids: ids}
}

type ApplyResult struct {
	ProposalID    uuid.UUID   `json:"proposal_id"`
	ChangeBatchID uuid.UUID   `json:"change_batch_id"`
	RevisionID    *uuid.UUID  `json:"revision_id,omitempty"`
	ClaimIDs      []uuid.UUID `json:"claim_ids"`
	Idempotent    bool        `json:"idempotent"`
}

// Apply 是 Proposal 生效的唯一治理入口。Conflict 检测通过后，ChangeBatch、
// Page/Knowledge 领域写入、Audit、Outbox 与 Proposal 状态在单事务提交。
func (s *ApplyService) Apply(ctx context.Context, proposalID, actorID uuid.UUID) (*ApplyResult, error) {
	if existing, err := s.repo.GetChangeBatchByProposal(ctx, nil, proposalID); err != nil {
		return nil, err
	} else if existing != nil && (existing.Status == BatchApplied || existing.Status == BatchRolledBack) {
		return &ApplyResult{ProposalID: proposalID, ChangeBatchID: existing.ID, ClaimIDs: []uuid.UUID{}, Idempotent: true}, nil
	}
	actorType, status, err := s.repo.GetActor(ctx, nil, actorID)
	if err != nil || status != "active" {
		return nil, ErrInvalidActor
	}
	if actorType != "human" && actorType != "system" && actorType != "bot" {
		return nil, ErrActorNotAllowed
	}
	if s.conflicts != nil {
		conflicts, err := s.conflicts.DetectAndRecord(ctx, proposalID)
		if err != nil {
			return nil, err
		}
		if len(conflicts) > 0 {
			return nil, ErrMergeConflict
		}
	}

	p, err := s.repo.GetProposal(ctx, nil, proposalID)
	if err != nil {
		return nil, err
	}
	if s.auth != nil {
		if err := s.auth.Check(ctx, actorID, wikiIDForProposal(ctx, nil, s.repo, p), ActionApply, p.TargetID); err != nil {
			return nil, err
		}
	}
	records, err := s.repo.ListOperations(ctx, nil, proposalID)
	if err != nil {
		return nil, err
	}
	ops := make([]OperationV1, len(records))
	for i := range records {
		op, err := OperationFromRecord(&records[i])
		if err != nil {
			return nil, err
		}
		ops[i] = *op
	}

	var pageAST json.RawMessage
	var pageExpected *uuid.UUID
	if p.TargetType == TargetPage {
		if p.TargetID == nil {
			return nil, ErrInvalidProposal
		}
		currentRev, currentSnap, err := s.pages.CurrentContent(ctx, *p.TargetID)
		if err != nil || currentRev == nil {
			return nil, fmt.Errorf("governance: 读取 Apply Current 失败: %w", err)
		}
		currentDoc, err := ast.Parse(currentSnap.AST)
		if err != nil {
			return nil, err
		}
		resolvedConflicts, err := s.repo.ListMergeConflicts(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		effectiveOps, err := applyConflictResolutions(currentDoc, ops, resolvedConflicts)
		if err != nil {
			return nil, err
		}
		proposed, err := s.pagePatch.Apply(currentDoc, *p.TargetID, effectiveOps)
		if err != nil {
			return nil, err
		}
		pageAST, err = ast.CanonicalJSON(proposed)
		if err != nil {
			return nil, err
		}
		pageExpected = &currentRev.ID
	}

	result := &ApplyResult{ProposalID: proposalID, ClaimIDs: []uuid.UUID{}}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		locked, err := s.repo.GetProposalForUpdate(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if batch, err := s.repo.GetChangeBatchByProposal(ctx, tx, proposalID); err != nil {
			return err
		} else if batch != nil {
			result.ChangeBatchID = batch.ID
			result.Idempotent = batch.Status == BatchApplied || batch.Status == BatchRolledBack
			if result.Idempotent {
				return nil
			}
			return ErrInvalidTransition
		}
		if locked.Status != ProposalApproved {
			return fmt.Errorf("%w: apply status=%s", ErrInvalidTransition, locked.Status)
		}
		approved, err := s.repo.HasApprovalEvidence(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if !approved {
			return ErrApprovalRequired
		}
		batchID, err := s.ids.New()
		if err != nil {
			return err
		}
		batch := &ChangeBatch{ID: batchID, ImportJobID: locked.ImportJobID,
			ProposalID: proposalID, ActorID: actorID, Status: BatchApplying}
		if err := s.repo.InsertChangeBatch(ctx, tx, batch); err != nil {
			return err
		}
		result.ChangeBatchID = batchID
		if err := s.repo.UpdateProposalStatus(ctx, tx, proposalID, ProposalApproved, ProposalApplying); err != nil {
			return err
		}

		if locked.TargetType == TargetPage {
			rev, err := s.pages.PublishInTx(ctx, tx, page.PublishParams{
				PageID: *locked.TargetID, ActorID: actorID, ExpectedRevisionID: pageExpected,
				AST: pageAST, Summary: "Apply Proposal " + proposalID.String(), ChangeBatchID: &batchID,
			})
			if err != nil {
				return err
			}
			result.RevisionID = &rev.ID
		} else {
			if s.knowledge == nil {
				return fmt.Errorf("%w: knowledge patch 未装配", ErrUnsupportedOperation)
			}
			for i := range ops {
				applied, err := s.knowledge.ApplyOneInTx(ctx, tx, ops[i], actorID, &batchID)
				if err != nil {
					return err
				}
				if applied.ClaimID != nil {
					if ops[i].OperationType == OpCreateClaim || ops[i].OperationType == OpSupersedeClaim {
						if err := s.knowledge.PublishAppliedClaimInTx(ctx, tx, *applied.ClaimID); err != nil {
							return err
						}
					}
					result.ClaimIDs = append(result.ClaimIDs, *applied.ClaimID)
					if err := s.emitKnowledgeEvents(ctx, tx, actorID, batchID, proposalID, *applied.ClaimID, ops[i].OperationType); err != nil {
						return err
					}
				}
			}
		}
		if err := s.emitProposalAudit(ctx, tx, actorID, batchID, proposalID); err != nil {
			return err
		}
		if err := s.repo.UpdateChangeBatchStatus(ctx, tx, batchID, BatchApplying, BatchApplied); err != nil {
			return err
		}
		return s.repo.UpdateProposalStatus(ctx, tx, proposalID, ProposalApplying, ProposalApplied)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *ApplyService) emitProposalAudit(ctx context.Context, tx pgx.Tx, actorID, batchID, proposalID uuid.UUID) error {
	id, err := s.ids.New()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"proposal_id": proposalID, "change_batch_id": batchID})
	return s.repo.InsertAudit(ctx, tx, id, actorID, "proposal.applied", "proposal", proposalID, batchID, payload)
}

func (s *ApplyService) emitKnowledgeEvents(ctx context.Context, tx pgx.Tx, actorID, batchID, proposalID, claimID uuid.UUID, operationType string) error {
	payload, _ := json.Marshal(map[string]any{"proposal_id": proposalID, "change_batch_id": batchID,
		"claim_id": claimID, "operation_type": operationType})
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	if err := s.repo.InsertAudit(ctx, tx, auditID, actorID, "claim.changed", "claim", claimID, batchID, payload); err != nil {
		return err
	}
	outboxID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.repo.InsertOutbox(ctx, tx, outboxID, "claim", claimID, "claim.changed", payload)
}
