package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var ErrRollbackStale = errors.New("governance: ChangeBatch 之后已有新变更，拒绝静默回滚")

type RollbackService struct {
	repo      *Repository
	pages     *page.Service
	knowledge *KnowledgePatchEngine
	txm       *db.TxManager
	ids       *id.Generator
	auth      *AuthorizationService
}

func (s *RollbackService) WithAuthorization(auth *AuthorizationService) *RollbackService {
	s.auth = auth
	return s
}

func NewRollbackService(repo *Repository, pages *page.Service, knowledge *KnowledgePatchEngine,
	txm *db.TxManager, ids *id.Generator) *RollbackService {
	return &RollbackService{repo: repo, pages: pages, knowledge: knowledge, txm: txm, ids: ids}
}

type RollbackResult struct {
	ChangeBatchID        uuid.UUID   `json:"change_batch_id"`
	RevisionIDs          []uuid.UUID `json:"revision_ids"`
	CompensationClaimIDs []uuid.UUID `json:"compensation_claim_ids"`
	Idempotent           bool        `json:"idempotent"`
}

// Rollback 以补偿版本撤销 ChangeBatch；页面创建新 Revision，Claim 经
// deprecated/rejected 或反向 Supersede，绝不 UPDATE/DELETE 历史 Revision/Snapshot。
func (s *RollbackService) Rollback(ctx context.Context, batchID, actorID uuid.UUID) (*RollbackResult, error) {
	actorType, status, err := s.repo.GetActor(ctx, nil, actorID)
	if err != nil || status != "active" {
		return nil, ErrInvalidActor
	}
	if actorType != "human" && actorType != "system" {
		return nil, ErrActorNotAllowed
	}
	result := &RollbackResult{ChangeBatchID: batchID, RevisionIDs: []uuid.UUID{}, CompensationClaimIDs: []uuid.UUID{}}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.repo.GetChangeBatchForUpdate(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if batch.Status == BatchRolledBack {
			result.Idempotent = true
			return nil
		}
		if batch.Status != BatchApplied {
			return fmt.Errorf("%w: batch status=%s", ErrInvalidTransition, batch.Status)
		}
		p, err := s.repo.GetProposalForUpdate(ctx, tx, batch.ProposalID)
		if err != nil {
			return err
		}
		if p.Status != ProposalApplied {
			return ErrInvalidTransition
		}
		if s.auth != nil {
			if err := s.auth.CheckTx(ctx, tx, actorID, wikiIDForProposal(ctx, tx, s.repo, p), ActionBatchRollback, p.TargetID); err != nil {
				return err
			}
		}
		revisions, err := s.repo.ListBatchRevisions(ctx, tx, batchID)
		if err != nil {
			return err
		}
		for _, rev := range revisions {
			current, err := s.repo.CurrentPageRevisionID(ctx, tx, rev.PageID)
			if err != nil {
				return err
			}
			if current == nil || *current != rev.ID || rev.ParentRevisionID == nil {
				return fmt.Errorf("%w: page=%s batch_revision=%s current=%v", ErrRollbackStale, rev.PageID, rev.ID, current)
			}
		}
		if err := s.repo.UpdateChangeBatchStatus(ctx, tx, batchID, BatchApplied, BatchRollbackPending); err != nil {
			return err
		}
		for _, rev := range revisions {
			compensation, err := s.pages.RollbackInTx(ctx, tx, page.RollbackParams{
				PageID: rev.PageID, TargetRevisionID: *rev.ParentRevisionID, ActorID: actorID,
				Summary: "回滚 ChangeBatch " + batchID.String(), ChangeBatchID: &batchID,
			})
			if err != nil {
				return err
			}
			result.RevisionIDs = append(result.RevisionIDs, compensation.ID)
		}
		claimIDs, err := s.repo.ListBatchClaimIDs(ctx, tx, batchID)
		if err != nil {
			return err
		}
		if len(claimIDs) > 0 && s.knowledge == nil {
			return ErrUnsupportedOperation
		}
		for _, claimID := range claimIDs {
			compensationID, err := s.knowledge.RollbackClaimInTx(ctx, tx, claimID, actorID, batchID)
			if err != nil {
				return err
			}
			result.CompensationClaimIDs = append(result.CompensationClaimIDs, *compensationID)
		}
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"proposal_id": batch.ProposalID, "change_batch_id": batchID})
		if err := s.repo.InsertAudit(ctx, tx, auditID, actorID, "proposal.rolled_back", "proposal", batch.ProposalID, batchID, payload); err != nil {
			return err
		}
		if err := s.repo.UpdateChangeBatchStatus(ctx, tx, batchID, BatchRollbackPending, BatchRolledBack); err != nil {
			return err
		}
		return s.repo.UpdateProposalStatus(ctx, tx, p.ID, ProposalApplied, ProposalRolledBack)
	})
	return result, err
}
