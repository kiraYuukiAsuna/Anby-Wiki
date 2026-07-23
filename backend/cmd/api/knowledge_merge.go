package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type mergeEntityRequest struct {
	TargetEntityID uuid.UUID `json:"target_entity_id"`
	Reason         string    `json:"reason"`
}

type entityMergeLabelMappingResponse struct {
	Language        string `json:"language"`
	SourceLabel     string `json:"source_label"`
	TargetLabel     string `json:"target_label"`
	Action          string `json:"action"`
	TargetIsPrimary bool   `json:"target_is_primary"`
}

type entityMergeClaimMappingResponse struct {
	OldClaimID uuid.UUID `json:"old_claim_id"`
	NewClaimID uuid.UUID `json:"new_claim_id"`
	OldStatus  string    `json:"old_status"`
	NewStatus  string    `json:"new_status"`
}

type entityMergeResponse struct {
	ID             uuid.UUID                         `json:"id"`
	SourceEntityID uuid.UUID                         `json:"source_entity_id"`
	TargetEntityID uuid.UUID                         `json:"target_entity_id"`
	ActorID        uuid.UUID                         `json:"actor_id"`
	Status         string                            `json:"status"`
	Reason         string                            `json:"reason"`
	CreatedAt      time.Time                         `json:"created_at"`
	Idempotent     bool                              `json:"idempotent"`
	LabelMappings  []entityMergeLabelMappingResponse `json:"label_mappings"`
	ClaimMappings  []entityMergeClaimMappingResponse `json:"claim_mappings"`
}

func (a *KnowledgeReadAPI) mergeEntity(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	sourceID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	var request mergeEntityRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if a.authorization == nil {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "Entity 合并未启用")
		return
	}
	if err := a.authorization.Check(
		r.Context(), actorID, a.wikiID, governance.ActionEntityMerge, nil,
	); err != nil {
		governanceError(w, r, err)
		return
	}
	result, err := a.knowledge.MergeEntity(r.Context(), knowledge.MergeEntityParams{
		SourceEntityID: sourceID,
		TargetEntityID: request.TargetEntityID,
		ActorID:        actorID,
		Reason:         request.Reason,
	})
	if err != nil {
		entityMergeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toEntityMergeResponse(result))
}

func toEntityMergeResponse(result *knowledge.MergeEntityResult) entityMergeResponse {
	labels := make([]entityMergeLabelMappingResponse, 0, len(result.LabelMappings))
	for _, mapping := range result.LabelMappings {
		labels = append(labels, entityMergeLabelMappingResponse{
			Language: mapping.Language, SourceLabel: mapping.SourceLabel,
			TargetLabel: mapping.TargetLabel, Action: mapping.Action,
			TargetIsPrimary: mapping.TargetIsPrimary,
		})
	}
	claims := make([]entityMergeClaimMappingResponse, 0, len(result.ClaimMappings))
	for _, mapping := range result.ClaimMappings {
		claims = append(claims, entityMergeClaimMappingResponse{
			OldClaimID: mapping.OldClaimID, NewClaimID: mapping.NewClaimID,
			OldStatus: mapping.OldStatus, NewStatus: mapping.NewStatus,
		})
	}
	return entityMergeResponse{
		ID: result.Merge.ID, SourceEntityID: result.Merge.SourceEntityID,
		TargetEntityID: result.Merge.TargetEntityID, ActorID: result.Merge.ActorID,
		Status: result.Merge.Status, Reason: result.Merge.Reason,
		CreatedAt: result.Merge.CreatedAt, Idempotent: result.Idempotent,
		LabelMappings: labels, ClaimMappings: claims,
	}
}

func entityMergeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, knowledge.ErrEntityNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, knowledge.ErrEntityMergeActorOnly):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, err.Error())
	case errors.Is(err, knowledge.ErrInvalidEntityMerge),
		errors.Is(err, knowledge.ErrEntityMergeCycle):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "内部错误")
	}
}
