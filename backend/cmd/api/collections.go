package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/collection"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type CollectionAPI struct {
	service *collection.Service
	wikiID  uuid.UUID
}

func NewCollectionAPI(service *collection.Service, wikiID uuid.UUID) *CollectionAPI {
	return &CollectionAPI{service: service, wikiID: wikiID}
}

type collectionResponse struct {
	ID                uuid.UUID       `json:"id"`
	WikiID            uuid.UUID       `json:"wiki_id"`
	CollectionType    string          `json:"collection_type"`
	Title             string          `json:"title"`
	DescriptionPageID *uuid.UUID      `json:"description_page_id"`
	Query             json.RawMessage `json:"query"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type collectionListResponse struct {
	Items      []collectionResponse `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

type membershipResponse struct {
	MemberType       string     `json:"member_type"`
	PageID           *uuid.UUID `json:"page_id"`
	EntityID         *uuid.UUID `json:"entity_id"`
	DisplayTitle     string     `json:"display_title"`
	SourceType       string     `json:"source_type"`
	SortKey          string     `json:"sort_key"`
	SourceRevisionID *uuid.UUID `json:"source_revision_id"`
	CreatedAt        time.Time  `json:"created_at"`
}

type membershipListResponse struct {
	Items      []membershipResponse `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

func toCollectionResponse(value collection.Collection) collectionResponse {
	query := value.QueryJSON
	if len(query) == 0 {
		query = json.RawMessage("null")
	}
	return collectionResponse{
		ID: value.ID, WikiID: value.WikiID, CollectionType: value.CollectionType,
		Title: value.Title, DescriptionPageID: value.DescriptionPageID, Query: query,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func (a *CollectionAPI) list(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.List(r.Context(), a.wikiID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	response := collectionListResponse{
		Items: make([]collectionResponse, len(result.Items)), NextCursor: result.NextCursor,
	}
	for index, item := range result.Items {
		response.Items[index] = toCollectionResponse(item)
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

type createCollectionRequest struct {
	CollectionType    string          `json:"collection_type"`
	Title             string          `json:"title"`
	DescriptionPageID *uuid.UUID      `json:"description_page_id"`
	Query             json.RawMessage `json:"query"`
}

func (a *CollectionAPI) create(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var request createCollectionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.Create(r.Context(), collection.CreateParams{
		WikiID: a.wikiID, CollectionType: request.CollectionType,
		Title: request.Title, DescriptionPageID: request.DescriptionPageID,
		Rule: request.Query, ActorID: actorID,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toCollectionResponse(*value))
}

func (a *CollectionAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	value, err := a.service.Get(r.Context(), a.wikiID, id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCollectionResponse(*value))
}

func (a *CollectionAPI) members(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.ListMembers(
		r.Context(), a.wikiID, id, r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	response := membershipListResponse{
		Items: make([]membershipResponse, len(result.Items)), NextCursor: result.NextCursor,
	}
	for index, item := range result.Items {
		response.Items[index] = membershipResponse{
			MemberType: item.MemberType, PageID: item.PageID, EntityID: item.EntityID,
			DisplayTitle: item.DisplayTitle, SourceType: item.SourceType,
			SortKey: item.SortKey, SourceRevisionID: item.SourceRevisionID,
			CreatedAt: item.CreatedAt,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

type manualMemberRequest struct {
	MemberType       string     `json:"member_type"`
	PageID           *uuid.UUID `json:"page_id"`
	EntityID         *uuid.UUID `json:"entity_id"`
	SortKey          string     `json:"sort_key"`
	SourceRevisionID uuid.UUID  `json:"source_revision_id"`
}

type replaceManualMembersRequest struct {
	Items []manualMemberRequest `json:"items"`
}

func (a *CollectionAPI) replaceMembers(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	var request replaceManualMembersRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	inputs := make([]collection.MemberInput, len(request.Items))
	for index, item := range request.Items {
		inputs[index] = collection.MemberInput{
			MemberType: item.MemberType, PageID: item.PageID, EntityID: item.EntityID,
			SortKey: item.SortKey, SourceRevisionID: item.SourceRevisionID,
		}
	}
	if err := a.service.ReplaceManualMembers(
		r.Context(), a.wikiID, id, actorID, inputs,
	); err != nil {
		a.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type rebuildCollectionRequest struct {
	SourceRevisionID uuid.UUID `json:"source_revision_id"`
}

func (a *CollectionAPI) rebuild(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	var request rebuildCollectionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	count, err := a.service.RebuildRule(
		r.Context(), a.wikiID, id, request.SourceRevisionID, actorID,
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"member_count": count})
}

func (a *CollectionAPI) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, collection.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "Collection 不存在")
	case errors.Is(err, collection.ErrInvalidCursor):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "cursor 无效")
	case errors.Is(err, collection.ErrInvalidDefinition),
		errors.Is(err, collection.ErrInvalidRule),
		errors.Is(err, collection.ErrInvalidMember):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity,
			httpx.CodeValidationFailed, err.Error())
	default:
		serviceError(w, r, err)
	}
}
