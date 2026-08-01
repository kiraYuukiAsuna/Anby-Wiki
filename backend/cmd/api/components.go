package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ComponentAPI struct {
	service *component.Service
}

func NewComponentAPI(service *component.Service) *ComponentAPI {
	return &ComponentAPI{service: service}
}

type componentResponse struct {
	ID           uuid.UUID `json:"id"`
	ComponentKey string    `json:"component_key"`
	Name         string    `json:"name"`
	CreatedBy    uuid.UUID `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type componentVersionResponse struct {
	ComponentID uuid.UUID       `json:"component_id"`
	Version     int             `json:"version"`
	PropsSchema json.RawMessage `json:"props_schema"`
	RendererRef string          `json:"renderer_ref"`
	Status      string          `json:"status"`
	CreatedBy   uuid.UUID       `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	PublishedAt *time.Time      `json:"published_at"`
}

func toComponentResponse(value component.Component) componentResponse {
	return componentResponse{
		ID: value.ID, ComponentKey: value.ComponentKey, Name: value.Name,
		CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func toComponentVersionResponse(value component.Version) componentVersionResponse {
	return componentVersionResponse{
		ComponentID: value.ComponentID, Version: value.Version,
		PropsSchema: value.PropsSchema, RendererRef: value.RendererRef,
		Status: value.Status, CreatedBy: value.CreatedBy,
		CreatedAt: value.CreatedAt, PublishedAt: value.PublishedAt,
	}
}

func (a *ComponentAPI) list(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.List(r.Context(), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		componentError(w, r, err)
		return
	}
	items := make([]componentResponse, len(result.Items))
	for index, item := range result.Items {
		items[index] = toComponentResponse(item)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items, "next_cursor": result.NextCursor,
	})
}

type createComponentRequest struct {
	ComponentKey string `json:"component_key"`
	Name         string `json:"name"`
}

func (a *ComponentAPI) create(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var request createComponentRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.Create(r.Context(), component.CreateParams{
		ComponentKey: request.ComponentKey, Name: request.Name, ActorID: actorID,
	})
	if err != nil {
		componentError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toComponentResponse(*value))
}

func (a *ComponentAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := componentIDFrom(w, r)
	if !ok {
		return
	}
	value, err := a.service.Get(r.Context(), id)
	if err != nil {
		componentError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toComponentResponse(*value))
}

type componentVersionRequest struct {
	PropsSchema json.RawMessage `json:"props_schema"`
	RendererRef string          `json:"renderer_ref"`
}

func (a *ComponentAPI) listVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := componentIDFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.ListVersions(r.Context(), id)
	if err != nil {
		componentError(w, r, err)
		return
	}
	items := make([]componentVersionResponse, len(result.Items))
	for index, item := range result.Items {
		items[index] = toComponentVersionResponse(item)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *ComponentAPI) createVersion(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := componentIDFrom(w, r)
	if !ok {
		return
	}
	var request componentVersionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.CreateVersion(r.Context(), component.CreateVersionParams{
		ComponentID: id, PropsSchema: request.PropsSchema,
		RendererRef: request.RendererRef, ActorID: actorID,
	})
	if err != nil {
		componentError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toComponentVersionResponse(*value))
}

func (a *ComponentAPI) updateVersion(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := componentIDFrom(w, r)
	if !ok {
		return
	}
	version, ok := componentVersionFrom(w, r)
	if !ok {
		return
	}
	var request componentVersionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.UpdateDraft(
		r.Context(), id, version, request.PropsSchema, request.RendererRef, actorID,
	)
	if err != nil {
		componentError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toComponentVersionResponse(*value))
}

func (a *ComponentAPI) publishVersion(w http.ResponseWriter, r *http.Request) {
	a.transitionVersion(w, r, true)
}

func (a *ComponentAPI) deprecateVersion(w http.ResponseWriter, r *http.Request) {
	a.transitionVersion(w, r, false)
}

func (a *ComponentAPI) transitionVersion(
	w http.ResponseWriter, r *http.Request, publish bool,
) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := componentIDFrom(w, r)
	if !ok {
		return
	}
	version, ok := componentVersionFrom(w, r)
	if !ok {
		return
	}
	var (
		value *component.Version
		err   error
	)
	if publish {
		value, err = a.service.Publish(r.Context(), id, version, actorID)
	} else {
		value, err = a.service.Deprecate(r.Context(), id, version, actorID)
	}
	if err != nil {
		componentError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toComponentVersionResponse(*value))
}

type componentPreviewRequest struct {
	Props    json.RawMessage `json:"props"`
	EntityID *uuid.UUID      `json:"entity_id"`
}

func (a *ComponentAPI) preview(w http.ResponseWriter, r *http.Request) {
	id, ok := componentIDFrom(w, r)
	if !ok {
		return
	}
	version, ok := componentVersionFrom(w, r)
	if !ok {
		return
	}
	var request componentPreviewRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	html, err := a.service.Preview(
		r.Context(), id, version, request.EntityID, request.Props,
	)
	if err != nil {
		componentError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"html": html})
}

func componentIDFrom(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	value, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			"id 不是合法 UUID")
		return uuid.Nil, false
	}
	return value, true
}

func componentVersionFrom(w http.ResponseWriter, r *http.Request) (int, bool) {
	value, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || value <= 0 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			"version 必须是正整数")
		return 0, false
	}
	return value, true
}

func componentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, component.ErrNotFound),
		errors.Is(err, component.ErrVersionNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, component.ErrDuplicateKey),
		errors.Is(err, component.ErrVersionFrozen),
		errors.Is(err, component.ErrInvalidTransition):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, component.ErrInvalidCursor):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
	case errors.Is(err, component.ErrInvalidDefinition),
		errors.Is(err, component.ErrInvalidProps),
		errors.Is(err, component.ErrRendererNotFound):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity,
			httpx.CodeValidationFailed, err.Error())
	default:
		serviceError(w, r, err)
	}
}
