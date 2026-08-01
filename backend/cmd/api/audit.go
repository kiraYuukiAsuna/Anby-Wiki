package main

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type createChangeTagRequest struct {
	TagKey      string `json:"tag_key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type assignChangeTagRequest struct {
	TargetType string    `json:"target_type"`
	TargetID   uuid.UUID `json:"target_id"`
}

func (a *GovernanceAPI) listChangeTags(w http.ResponseWriter, r *http.Request) {
	tags, err := a.audit.ListTags(r.Context())
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": tags})
}

func (a *GovernanceAPI) createChangeTag(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var body createChangeTagRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	tag, err := a.audit.CreateTag(
		r.Context(), actorID, body.TagKey, body.Name, body.Description,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, tag)
}

func (a *GovernanceAPI) assignChangeTag(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	tagID, ok := governancePathID(w, r, "id")
	if !ok {
		return
	}
	var body assignChangeTagRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	inserted, err := a.audit.AssignTag(
		r.Context(), actorID, tagID, body.TargetType, body.TargetID,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{
		"assigned":   true,
		"idempotent": !inserted,
	})
}

func (a *GovernanceAPI) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	var changeBatchID *uuid.UUID
	if raw := r.URL.Query().Get("change_batch_id"); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
				"change_batch_id 不是合法 UUID")
			return
		}
		changeBatchID = &value
	}
	page, err := a.audit.List(r.Context(), actorID, governance.AuditListFilter{
		EventType:     r.URL.Query().Get("event_type"),
		AggregateType: r.URL.Query().Get("aggregate_type"),
		ChangeBatchID: changeBatchID,
		TagKey:        r.URL.Query().Get("tag_key"),
		Cursor:        r.URL.Query().Get("cursor"),
		Limit:         limit,
	})
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func isAuditNotFound(err error) bool {
	return errors.Is(err, governance.ErrChangeTagNotFound) ||
		errors.Is(err, governance.ErrAuditEventNotFound) ||
		errors.Is(err, governance.ErrAuditTargetNotFound)
}
