package governance

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var ErrInvalidResolution = errors.New("governance: invalid conflict resolution")

type ResolveConflictParams struct {
	ProposalID uuid.UUID
	ConflictID uuid.UUID
	ActorID    uuid.UUID
	Choice     string
	Reason     string
}

type ConflictResolutionService struct {
	repo *Repository
	txm  *db.TxManager
	ids  *id.Generator
	auth *AuthorizationService
}

func NewConflictResolutionService(
	repo *Repository,
	txm *db.TxManager,
	ids *id.Generator,
) *ConflictResolutionService {
	return &ConflictResolutionService{repo: repo, txm: txm, ids: ids}
}

func (s *ConflictResolutionService) WithAuthorization(
	auth *AuthorizationService,
) *ConflictResolutionService {
	s.auth = auth
	return s
}

func (s *ConflictResolutionService) Resolve(
	ctx context.Context,
	params ResolveConflictParams,
) (*Proposal, error) {
	if params.Choice != ResolutionChooseCurrent &&
		params.Choice != ResolutionChooseProposed &&
		params.Choice != ResolutionDismiss {
		return nil, ErrInvalidResolution
	}
	actorType, status, err := s.repo.GetActor(ctx, nil, params.ActorID)
	if err != nil || status != "active" || actorType != "human" {
		return nil, ErrActorNotAllowed
	}
	proposal, err := s.repo.GetProposal(ctx, nil, params.ProposalID)
	if err != nil {
		return nil, err
	}
	if s.auth != nil {
		if err := s.auth.Check(
			ctx, params.ActorID, wikiIDForProposal(ctx, nil, s.repo, proposal),
			ActionApply, proposal.TargetID,
		); err != nil {
			return nil, err
		}
	}
	var result *Proposal
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		locked, err := s.repo.GetProposalForUpdate(ctx, tx, params.ProposalID)
		if err != nil {
			return err
		}
		if locked.Status != ProposalConflicted {
			return ErrInvalidTransition
		}
		resolution, _ := json.Marshal(map[string]any{
			"choice": params.Choice, "reason": params.Reason,
		})
		conflictStatus := ConflictResolved
		if params.Choice == ResolutionDismiss {
			conflictStatus = ConflictDismissed
		}
		if err := s.repo.ResolveMergeConflict(
			ctx, tx, params.ProposalID, params.ConflictID, params.ActorID,
			conflictStatus, resolution,
		); err != nil {
			return err
		}
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		auditPayload, _ := json.Marshal(map[string]any{
			"proposal_id": params.ProposalID, "conflict_id": params.ConflictID,
			"choice": params.Choice,
		})
		if err := s.repo.InsertAuditWithoutBatch(
			ctx, tx, auditID, params.ActorID, "proposal.conflict_resolved",
			"proposal", params.ProposalID, auditPayload,
		); err != nil {
			return err
		}
		open, err := s.repo.CountOpenMergeConflicts(ctx, tx, params.ProposalID)
		if err != nil {
			return err
		}
		if open == 0 {
			if err := s.repo.UpdateProposalStatus(
				ctx, tx, params.ProposalID, ProposalConflicted, ProposalApproved,
			); err != nil {
				return err
			}
		}
		result, err = s.repo.GetProposal(ctx, tx, params.ProposalID)
		return err
	})
	return result, err
}
