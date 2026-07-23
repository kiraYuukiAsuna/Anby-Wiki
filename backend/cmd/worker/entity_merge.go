package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/projection"
)

type entityMergeRepairService interface {
	CreateEntityMergeRepairProposals(
		context.Context,
		uuid.UUID,
		uuid.UUID,
	) (*governance.EntityMergeRepairResult, error)
}

type entityMergeRepairHandler struct {
	service entityMergeRepairService
}

type entityMergedPayload struct {
	MergeID uuid.UUID `json:"merge_id"`
	ActorID uuid.UUID `json:"actor_id"`
}

func newEntityMergeRepairHandler(service entityMergeRepairService) projection.Handler {
	return &entityMergeRepairHandler{service: service}
}

func (h *entityMergeRepairHandler) Handle(ctx context.Context, event projection.Event) error {
	if event.AggregateType != "entity_merge" ||
		event.EventType != knowledge.OutboxEventEntityMerged ||
		event.AggregateID == uuid.Nil {
		return fmt.Errorf("worker: invalid entity.merged envelope")
	}
	var payload entityMergedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("worker: decode entity.merged payload: %w", err)
	}
	if payload.MergeID == uuid.Nil || payload.ActorID == uuid.Nil ||
		payload.MergeID != event.AggregateID {
		return fmt.Errorf("worker: invalid entity.merged payload")
	}
	if _, err := h.service.CreateEntityMergeRepairProposals(
		ctx, payload.MergeID, payload.ActorID,
	); err != nil {
		return fmt.Errorf("worker: create Entity merge repair Proposals: %w", err)
	}
	return nil
}
