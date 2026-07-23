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
	SourceRevisionID uuid.UUID  `json:"source_revision_id"`
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

func (a *CollectionAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	value, err := a.service.Get(r.Context(), id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if value.WikiID != a.wikiID {
		a.writeError(w, r, collection.ErrNotFound)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCollectionResponse(*value))
}

func (a *CollectionAPI) members(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	value, err := a.service.Get(r.Context(), id)
	if err != nil || value.WikiID != a.wikiID {
		if err == nil {
			err = collection.ErrNotFound
		}
		a.writeError(w, r, err)
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.ListMembers(
		r.Context(), id, r.URL.Query().Get("cursor"), limit,
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

func (a *CollectionAPI) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, collection.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "Collection 不存在")
	case errors.Is(err, collection.ErrInvalidCursor):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "cursor 无效")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "内部错误")
	}
}
