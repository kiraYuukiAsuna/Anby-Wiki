package governance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var ErrMergedASTMismatch = errors.New("governance: merged AST does not match proposal operations")

type MergeWorkingDocumentParams struct {
	ProposalID       uuid.UUID
	DocumentID       uuid.UUID
	ActorID          uuid.UUID
	ClientID         uuid.UUID
	ClientUpdateID   uuid.UUID
	ExpectedSequence int64
	CurrentAST       json.RawMessage
	MergedAST        json.RawMessage
	Update           []byte
}

type MergeWorkingDocumentResult struct {
	ProposalID    uuid.UUID
	ChangeBatchID uuid.UUID
	DocumentID    uuid.UUID
	Sequence      int64
	Idempotent    bool
}

// MergeWorkingDocumentService validates a client-generated three-way merge and
// atomically accepts its Yjs delta with Proposal/ChangeBatch state changes.
type MergeWorkingDocumentService struct {
	repo          *Repository
	pagePatch     *PagePatchEngine
	conflicts     *ConflictService
	collaboration *collaboration.Service
	txm           *db.TxManager
	ids           *id.Generator
	auth          *AuthorizationService
}

func NewMergeWorkingDocumentService(
	repo *Repository,
	pagePatch *PagePatchEngine,
	conflicts *ConflictService,
	collaborationService *collaboration.Service,
	txm *db.TxManager,
	ids *id.Generator,
) *MergeWorkingDocumentService {
	return &MergeWorkingDocumentService{
		repo: repo, pagePatch: pagePatch, conflicts: conflicts,
		collaboration: collaborationService, txm: txm, ids: ids,
	}
}

func (s *MergeWorkingDocumentService) WithAuthorization(
	auth *AuthorizationService,
) *MergeWorkingDocumentService {
	s.auth = auth
	return s
}

func (s *MergeWorkingDocumentService) Merge(
	ctx context.Context,
	params MergeWorkingDocumentParams,
) (*MergeWorkingDocumentResult, *collaboration.Update, error) {
	if params.ExpectedSequence < 0 || len(params.CurrentAST) == 0 ||
		len(params.MergedAST) == 0 || len(params.Update) == 0 {
		return nil, nil, ErrInvalidProposal
	}
	if batch, err := s.repo.GetChangeBatchByProposal(ctx, nil, params.ProposalID); err != nil {
		return nil, nil, err
	} else if batch != nil && batch.Status == BatchApplied {
		return &MergeWorkingDocumentResult{
			ProposalID: params.ProposalID, ChangeBatchID: batch.ID,
			DocumentID: params.DocumentID, Idempotent: true,
		}, nil, nil
	}
	proposal, err := s.repo.GetProposal(ctx, nil, params.ProposalID)
	if err != nil {
		return nil, nil, err
	}
	if proposal.Status != ProposalApproved || proposal.TargetType != TargetPage ||
		proposal.TargetID == nil {
		return nil, nil, ErrInvalidTransition
	}
	if s.auth != nil {
		wikiID := wikiIDForProposal(ctx, nil, s.repo, proposal)
		if err := s.auth.Check(ctx, params.ActorID, wikiID, ActionApply, proposal.TargetID); err != nil {
			return nil, nil, err
		}
	}
	if s.conflicts != nil {
		conflicts, err := s.conflicts.DetectAndRecord(ctx, params.ProposalID)
		if err != nil {
			return nil, nil, err
		}
		if len(conflicts) > 0 {
			return nil, nil, ErrMergeConflict
		}
	}

	records, err := s.repo.ListOperations(ctx, nil, params.ProposalID)
	if err != nil {
		return nil, nil, err
	}
	operations := make([]OperationV1, len(records))
	for index := range records {
		operation, err := OperationFromRecord(&records[index])
		if err != nil {
			return nil, nil, err
		}
		operations[index] = *operation
	}
	current, err := ast.Parse(params.CurrentAST)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: current AST: %v", ErrInvalidProposal, err)
	}
	resolvedConflicts, err := s.repo.ListMergeConflicts(ctx, proposal.ID)
	if err != nil {
		return nil, nil, err
	}
	effectiveOperations, err := applyConflictResolutions(
		current, operations, resolvedConflicts,
	)
	if err != nil {
		return nil, nil, err
	}
	expectedMerged, err := s.pagePatch.Apply(current, *proposal.TargetID, effectiveOperations)
	if err != nil {
		if errors.Is(err, ErrPatchTargetModified) || errors.Is(err, ErrPatchTargetNotFound) {
			return nil, nil, fmt.Errorf("%w: %v", ErrMergeConflict, err)
		}
		return nil, nil, err
	}
	expectedJSON, err := ast.CanonicalJSON(expectedMerged)
	if err != nil {
		return nil, nil, err
	}
	providedJSON, err := ast.CanonicalizeJSON(params.MergedAST)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: merged AST: %v", ErrInvalidProposal, err)
	}
	if !bytes.Equal(expectedJSON, providedJSON) {
		return nil, nil, ErrMergedASTMismatch
	}

	result := &MergeWorkingDocumentResult{
		ProposalID: params.ProposalID, DocumentID: params.DocumentID,
	}
	var accepted collaboration.Update
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		locked, err := s.repo.GetProposalForUpdate(ctx, tx, params.ProposalID)
		if err != nil {
			return err
		}
		if batch, err := s.repo.GetChangeBatchByProposal(ctx, tx, params.ProposalID); err != nil {
			return err
		} else if batch != nil {
			if batch.Status != BatchApplied {
				return ErrInvalidTransition
			}
			result.ChangeBatchID = batch.ID
			result.Idempotent = true
			return nil
		}
		if locked.Status != ProposalApproved {
			return ErrInvalidTransition
		}
		approved, err := s.repo.HasApprovalEvidence(ctx, tx, params.ProposalID)
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
		batch := &ChangeBatch{
			ID: batchID, ImportJobID: locked.ImportJobID, ProposalID: locked.ID,
			ActorID: params.ActorID, Status: BatchApplying,
		}
		if err := s.repo.InsertChangeBatch(ctx, tx, batch); err != nil {
			return err
		}
		result.ChangeBatchID = batchID
		if err := s.repo.UpdateProposalStatus(
			ctx, tx, locked.ID, ProposalApproved, ProposalApplying,
		); err != nil {
			return err
		}
		accepted, err = s.collaboration.AppendCASInTx(
			ctx, tx, params.DocumentID, *locked.TargetID, params.ActorID,
			params.ClientID, params.ClientUpdateID, params.ExpectedSequence, params.Update,
		)
		if err != nil {
			return err
		}
		result.Sequence = accepted.Sequence
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"proposal_id": params.ProposalID, "change_batch_id": batchID,
			"working_document_id": params.DocumentID, "sequence": accepted.Sequence,
		})
		if err := s.repo.InsertAudit(
			ctx, tx, auditID, params.ActorID, "proposal.merged_to_working_document",
			"proposal", params.ProposalID, batchID, payload,
		); err != nil {
			return err
		}
		if err := s.repo.UpdateChangeBatchStatus(
			ctx, tx, batchID, BatchApplying, BatchApplied,
		); err != nil {
			return err
		}
		return s.repo.UpdateProposalStatus(
			ctx, tx, locked.ID, ProposalApplying, ProposalApplied,
		)
	})
	if err != nil {
		return nil, nil, err
	}
	return result, &accepted, nil
}
