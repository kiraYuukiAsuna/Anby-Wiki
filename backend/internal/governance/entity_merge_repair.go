package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
)

var entityMergeRepairSystemActorID = uuid.MustParse("00000000-0000-7000-8000-000000000201")

type EntityMergeRepairResult struct {
	MergeID    uuid.UUID
	Proposals  []Proposal
	Operations int
	Idempotent bool
}

// CreateEntityMergeRepairProposals creates one atomic draft Proposal per affected
// Current Revision. It reuses the v1 retarget operations and never edits AST directly.
func (s *Service) CreateEntityMergeRepairProposals(
	ctx context.Context,
	mergeID, originalActorID uuid.UUID,
) (*EntityMergeRepairResult, error) {
	if mergeID == uuid.Nil || originalActorID == uuid.Nil {
		return nil, ErrInvalidProposal
	}
	result := &EntityMergeRepairResult{MergeID: mergeID, Proposals: []Proposal{}}
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		status, err := s.repo.entityMergeRepairStatus(ctx, tx, mergeID)
		if err != nil {
			return err
		}
		if status == "rolled_back" {
			result.Idempotent = true
			return nil
		}
		if status != "applied" {
			return ErrInvalidProposal
		}
		if err := s.checkProposalActor(ctx, tx, entityMergeRepairSystemActorID); err != nil {
			return err
		}
		references, err := s.repo.listEntityMergeRepairReferences(ctx, tx, mergeID)
		if err != nil {
			return err
		}
		for start := 0; start < len(references); {
			end := start + 1
			for end < len(references) && references[end].PageID == references[start].PageID {
				end++
			}
			proposal, operationCount, idempotent, err := s.createEntityMergePageRepair(
				ctx, tx, mergeID, originalActorID, references[start:end],
			)
			if err != nil {
				return err
			}
			result.Proposals = append(result.Proposals, *proposal)
			result.Operations += operationCount
			result.Idempotent = result.Idempotent || idempotent
			start = end
		}
		return nil
	})
	return result, err
}

func (s *Service) createEntityMergePageRepair(
	ctx context.Context,
	tx pgx.Tx,
	mergeID, originalActorID uuid.UUID,
	references []entityMergeRepairReference,
) (*Proposal, int, bool, error) {
	pageID, revisionID := references[0].PageID, references[0].RevisionID
	for _, ref := range references {
		if ref.RevisionID != revisionID {
			return nil, 0, false, ErrRepairProjectionStale
		}
	}
	rawAST, err := s.repo.currentRepairAST(ctx, tx, pageID, revisionID)
	if err != nil {
		return nil, 0, false, err
	}
	document, err := ast.Parse(rawAST)
	if err != nil {
		return nil, 0, false, fmt.Errorf("governance: parse Entity merge repair AST: %w", err)
	}
	operations, err := buildEntityMergeRetargetOperations(document, revisionID, references)
	if err != nil {
		return nil, 0, false, err
	}
	key := "entity-merge-repair:" + mergeID.String() + ":" + pageID.String() + ":" + revisionID.String()
	proposalID, err := s.ids.New()
	if err != nil {
		return nil, 0, false, err
	}
	candidate := &Proposal{
		ID: proposalID, TargetType: TargetPage, TargetID: &pageID,
		BaseRevisionID: &revisionID, Status: ProposalDraft, RiskLevel: RiskHigh,
		CreatedBy: entityMergeRepairSystemActorID, IdempotencyKey: key,
	}
	inserted, err := s.repo.InsertProposal(ctx, tx, candidate)
	if err != nil {
		return nil, 0, false, err
	}
	if !inserted {
		existing, err := s.repo.GetProposalByIdempotency(
			ctx, tx, entityMergeRepairSystemActorID, key,
		)
		if err != nil {
			return nil, 0, false, err
		}
		if existing.TargetID == nil || *existing.TargetID != pageID ||
			existing.BaseRevisionID == nil || *existing.BaseRevisionID != revisionID {
			return nil, 0, false, ErrIdempotencyMismatch
		}
		count, err := s.repo.CountOperations(ctx, tx, existing.ID)
		return existing, count, true, err
	}
	for i := range operations {
		if err := s.insertOperationV1InTx(ctx, tx, candidate.ID, i+1, operations[i]); err != nil {
			return nil, 0, false, err
		}
	}
	auditID, err := s.ids.New()
	if err != nil {
		return nil, 0, false, err
	}
	auditPayload, _ := json.Marshal(map[string]any{
		"merge_id": mergeID, "original_actor_id": originalActorID,
		"system_actor_id": entityMergeRepairSystemActorID,
	})
	if err := s.repo.InsertAuditWithoutBatch(
		ctx, tx, auditID, originalActorID, "proposal.entity_merge_repair_created",
		"proposal", candidate.ID, auditPayload,
	); err != nil {
		return nil, 0, false, err
	}
	created, err := s.repo.GetProposal(ctx, tx, candidate.ID)
	return created, len(operations), false, err
}

func buildEntityMergeRetargetOperations(
	document *ast.Document,
	revisionID uuid.UUID,
	references []entityMergeRepairReference,
) ([]OperationV1, error) {
	operations := make([]OperationV1, 0, len(references))
	hashedBlocks := make(map[string]bool)
	for _, ref := range references {
		loc, ok := ast.FindBlock(document, ref.BlockID)
		if !ok {
			return nil, ErrRepairProjectionStale
		}
		nodes, err := loc.Block.InlineContent()
		if err != nil {
			return nil, err
		}
		index, err := strconv.Atoi(ref.NodeID)
		if err != nil || index < 0 || index >= len(nodes) {
			return nil, ErrRepairProjectionStale
		}
		node := nodes[index]
		target := OperationTarget{
			PageID: &ref.PageID, BlockID: &ref.BlockID, NodeID: &ref.NodeID,
		}
		operationType := ""
		switch ref.Kind {
		case "entity":
			if node.Type != ast.InlineEntityReference || node.EntityID != ref.OldID.String() {
				return nil, ErrRepairProjectionStale
			}
			target.EntityID = &ref.NewID
			operationType = OpRetargetEntityReference
		case "claim":
			if node.Type != ast.InlineClaimReference || node.ClaimID != ref.OldID.String() {
				return nil, ErrRepairProjectionStale
			}
			target.ClaimID = &ref.NewID
			operationType = OpRetargetClaimReference
		default:
			return nil, ErrUnsupportedOperation
		}
		var expectedHash *string
		if !hashedBlocks[ref.BlockID] {
			hash, err := BlockHash(loc.Block)
			if err != nil {
				return nil, err
			}
			expectedHash = &hash
			hashedBlocks[ref.BlockID] = true
		}
		payload, _ := json.Marshal(map[string]string{"display_text": node.DisplayText})
		operations = append(operations, OperationV1{
			SchemaVersion: OperationVersion, OperationType: operationType,
			Base: OperationBase{RevisionID: &revisionID}, Target: target,
			ExpectedHash: expectedHash, Evidence: []OperationEvidence{},
			Risk: OperationRisk{
				Level:   RiskHigh,
				Reasons: []string{"Entity merge reference repair requires review"},
			},
			Payload: payload,
		})
	}
	return operations, nil
}

func (s *Service) insertOperationV1InTx(
	ctx context.Context,
	tx pgx.Tx,
	proposalID uuid.UUID,
	sequence int,
	op OperationV1,
) error {
	raw, err := json.Marshal(op)
	if err != nil {
		return err
	}
	if _, err := ParseOperationV1(raw); err != nil {
		return err
	}
	base, _ := json.Marshal(op.Base)
	target, _ := json.Marshal(op.Target)
	evidence, _ := json.Marshal(op.Evidence)
	risk, _ := json.Marshal(op.Risk)
	operationID, err := s.ids.New()
	if err != nil {
		return err
	}
	return s.repo.InsertOperation(ctx, tx, &OperationRecord{
		ID: operationID, ProposalID: proposalID, Sequence: sequence,
		SchemaVersion: op.SchemaVersion, OperationType: op.OperationType,
		TargetPageID: op.Target.PageID, TargetBlockID: op.Target.BlockID,
		TargetNodeID: op.Target.NodeID, TargetEntityID: op.Target.EntityID,
		TargetClaimID: op.Target.ClaimID, Target: target,
		ExpectedHash: op.ExpectedHash, Base: base, Evidence: evidence,
		Risk: risk, Payload: op.Payload,
	})
}
