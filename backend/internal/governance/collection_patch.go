package governance

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/collection"
)

type collectionMembershipPayload struct {
	SortKey          string    `json:"sort_key"`
	SourceRevisionID uuid.UUID `json:"source_revision_id"`
}

func isCollectionOperation(operationType string) bool {
	return operationType == OpAddCollectionMembership ||
		operationType == OpRemoveCollectionMembership
}

func (s *ApplyService) applyCollectionOperationInTx(
	ctx context.Context,
	tx pgx.Tx,
	proposal *Proposal,
	op OperationV1,
	actorID, changeBatchID uuid.UUID,
) (uuid.UUID, error) {
	if s.collections == nil {
		return uuid.Nil, fmt.Errorf("%w: Collection 领域服务未装配", ErrUnsupportedOperation)
	}
	if proposal.TargetType != TargetCollection || proposal.TargetID == nil ||
		op.Target.CollectionID == nil || *proposal.TargetID != *op.Target.CollectionID {
		return uuid.Nil, ErrInvalidOperation
	}
	if (op.Target.PageID == nil) == (op.Target.EntityID == nil) {
		return uuid.Nil, ErrInvalidOperation
	}
	wikiID := wikiIDForProposal(ctx, tx, s.repo, proposal)
	if wikiID == uuid.Nil {
		return uuid.Nil, ErrInvalidOperation
	}
	if s.auth != nil {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionEdit, nil); err != nil {
			return uuid.Nil, err
		}
	}
	input := collection.MemberInput{
		PageID: op.Target.PageID, EntityID: op.Target.EntityID,
	}
	if op.Target.PageID != nil {
		input.MemberType = collection.MemberPage
	} else {
		input.MemberType = collection.MemberEntity
	}
	add := op.OperationType == OpAddCollectionMembership
	if add {
		var payload collectionMembershipPayload
		if err := decodeOperationPayload(op.Payload, &payload); err != nil {
			return uuid.Nil, err
		}
		if strings.TrimSpace(payload.SortKey) == "" ||
			payload.SourceRevisionID == uuid.Nil {
			return uuid.Nil, ErrInvalidOperation
		}
		input.SortKey = payload.SortKey
		input.SourceRevisionID = payload.SourceRevisionID
	} else {
		var payload struct{}
		if err := decodeOperationPayload(op.Payload, &payload); err != nil {
			return uuid.Nil, err
		}
	}
	if _, err := s.collections.ApplyManualMemberInTx(
		ctx, tx, wikiID, *proposal.TargetID, actorID, input, add, &changeBatchID,
	); err != nil {
		return uuid.Nil, err
	}
	return *proposal.TargetID, nil
}
