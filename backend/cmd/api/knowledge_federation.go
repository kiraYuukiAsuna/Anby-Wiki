package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type federatedWikiResponse struct {
	ID                uuid.UUID       `json:"id"`
	WikiID            uuid.UUID       `json:"wiki_id"`
	WikiKey           string          `json:"wiki_key"`
	DisplayName       string          `json:"display_name"`
	BaseURL           string          `json:"base_url"`
	EntityURLTemplate string          `json:"entity_url_template"`
	TrustLevel        string          `json:"trust_level"`
	Status            string          `json:"status"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedBy         uuid.UUID       `json:"created_by"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type federatedWikiListResponse struct {
	Items []federatedWikiResponse `json:"items"`
}

type createFederatedWikiRequest struct {
	WikiKey           string          `json:"wiki_key"`
	DisplayName       string          `json:"display_name"`
	BaseURL           string          `json:"base_url"`
	EntityURLTemplate string          `json:"entity_url_template"`
	TrustLevel        string          `json:"trust_level"`
	Status            string          `json:"status"`
	Metadata          json.RawMessage `json:"metadata"`
}

type updateFederatedWikiRequest struct {
	DisplayName       string          `json:"display_name"`
	BaseURL           string          `json:"base_url"`
	EntityURLTemplate string          `json:"entity_url_template"`
	TrustLevel        string          `json:"trust_level"`
	Status            string          `json:"status"`
	Metadata          json.RawMessage `json:"metadata"`
}

type federationLinkResponse struct {
	ID                 uuid.UUID       `json:"id"`
	WikiID             uuid.UUID       `json:"wiki_id"`
	LocalEntityID      uuid.UUID       `json:"local_entity_id"`
	LocalCanonicalKey  string          `json:"local_canonical_key"`
	LocalLabel         string          `json:"local_label"`
	LocalStatus        string          `json:"local_status"`
	RemoteWikiID       uuid.UUID       `json:"remote_wiki_id"`
	RemoteWikiKey      string          `json:"remote_wiki_key"`
	RemoteWikiName     string          `json:"remote_wiki_name"`
	RemoteWikiStatus   string          `json:"remote_wiki_status"`
	RemoteTrustLevel   string          `json:"remote_trust_level"`
	RemoteEntityID     string          `json:"remote_entity_id"`
	RemoteCanonicalKey string          `json:"remote_canonical_key"`
	RemoteLabel        string          `json:"remote_label"`
	RemoteEntityURL    string          `json:"remote_entity_url"`
	RelationType       string          `json:"relation_type"`
	VerificationStatus string          `json:"verification_status"`
	Status             string          `json:"status"`
	Metadata           json.RawMessage `json:"metadata"`
	CreatedBy          uuid.UUID       `json:"created_by"`
	UpdatedBy          uuid.UUID       `json:"updated_by"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type federationLinkListResponse struct {
	Items      []federationLinkResponse `json:"items"`
	NextCursor *string                  `json:"next_cursor"`
}

type createFederationLinkRequest struct {
	RemoteWikiID       uuid.UUID       `json:"remote_wiki_id"`
	RemoteEntityID     string          `json:"remote_entity_id"`
	RemoteCanonicalKey string          `json:"remote_canonical_key"`
	RemoteLabel        string          `json:"remote_label"`
	RelationType       string          `json:"relation_type"`
	VerificationStatus string          `json:"verification_status"`
	Metadata           json.RawMessage `json:"metadata"`
}

type updateFederationLinkRequest struct {
	RemoteCanonicalKey string          `json:"remote_canonical_key"`
	RemoteLabel        string          `json:"remote_label"`
	RelationType       string          `json:"relation_type"`
	VerificationStatus string          `json:"verification_status"`
	Status             string          `json:"status"`
	Metadata           json.RawMessage `json:"metadata"`
}

func (a *KnowledgeReadAPI) listFederatedWikis(
	w http.ResponseWriter,
	r *http.Request,
) {
	includeDisabled := r.URL.Query().Get("include_disabled") == "true"
	items, err := a.knowledge.ListFederatedWikis(
		r.Context(), a.wikiID, includeDisabled,
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	response := federatedWikiListResponse{
		Items: make([]federatedWikiResponse, len(items)),
	}
	for index, item := range items {
		response.Items[index] = toFederatedWikiResponse(item)
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (a *KnowledgeReadAPI) registerFederatedWiki(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := a.requireFederationManager(w, r)
	if !ok {
		return
	}
	var request createFederatedWikiRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	item, err := a.knowledge.RegisterFederatedWiki(
		r.Context(), knowledge.RegisterFederatedWikiParams{
			WikiID: a.wikiID, WikiKey: request.WikiKey,
			DisplayName: request.DisplayName, BaseURL: request.BaseURL,
			EntityURLTemplate: request.EntityURLTemplate,
			TrustLevel:        request.TrustLevel, Status: request.Status,
			Metadata: request.Metadata, ActorID: actorID,
		},
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toFederatedWikiResponse(*item))
}

func (a *KnowledgeReadAPI) updateFederatedWiki(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := a.requireFederationManager(w, r)
	if !ok {
		return
	}
	remoteWikiID, ok := federationPathID(w, r, "id")
	if !ok {
		return
	}
	var request updateFederatedWikiRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	item, err := a.knowledge.UpdateFederatedWiki(
		r.Context(), a.wikiID, remoteWikiID,
		knowledge.UpdateFederatedWikiParams{
			DisplayName: request.DisplayName, BaseURL: request.BaseURL,
			EntityURLTemplate: request.EntityURLTemplate,
			TrustLevel:        request.TrustLevel, Status: request.Status,
			Metadata: request.Metadata, ActorID: actorID,
		},
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toFederatedWikiResponse(*item))
}

func (a *KnowledgeReadAPI) listFederationLinks(
	w http.ResponseWriter,
	r *http.Request,
) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	var remoteWikiID *uuid.UUID
	if raw := r.URL.Query().Get("remote_wiki_id"); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(
				w, r, http.StatusBadRequest, httpx.CodeBadRequest,
				"remote_wiki_id 不是合法 UUID",
			)
			return
		}
		remoteWikiID = &value
	}
	page, err := a.knowledge.ListFederationLinks(
		r.Context(), knowledge.FederationLinkListParams{
			WikiID: a.wikiID, RemoteWikiID: remoteWikiID,
			Status: r.URL.Query().Get("status"), Query: r.URL.Query().Get("q"),
			Cursor: r.URL.Query().Get("cursor"), Limit: limit,
		},
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, federationPageResponse(page))
}

func (a *KnowledgeReadAPI) listEntityFederationLinks(
	w http.ResponseWriter,
	r *http.Request,
) {
	entityID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	items, err := a.knowledge.ListEntityFederationLinks(
		r.Context(), a.wikiID, entityID, r.URL.Query().Get("status"),
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	response := federationLinkListResponse{
		Items: make([]federationLinkResponse, len(items)),
	}
	for index, item := range items {
		response.Items[index] = toFederationLinkResponse(item)
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (a *KnowledgeReadAPI) createFederationLink(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := a.requireFederationManager(w, r)
	if !ok {
		return
	}
	entityID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	var request createFederationLinkRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	item, err := a.knowledge.CreateFederationLink(
		r.Context(), knowledge.CreateFederationLinkParams{
			WikiID: a.wikiID, LocalEntityID: entityID,
			RemoteWikiID:       request.RemoteWikiID,
			RemoteEntityID:     request.RemoteEntityID,
			RemoteCanonicalKey: request.RemoteCanonicalKey,
			RemoteLabel:        request.RemoteLabel,
			RelationType:       request.RelationType,
			VerificationStatus: request.VerificationStatus,
			Metadata:           request.Metadata, ActorID: actorID,
		},
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toFederationLinkResponse(*item))
}

func (a *KnowledgeReadAPI) updateFederationLink(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := a.requireFederationManager(w, r)
	if !ok {
		return
	}
	linkID, ok := federationPathID(w, r, "id")
	if !ok {
		return
	}
	var request updateFederationLinkRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	item, err := a.knowledge.UpdateFederationLink(
		r.Context(), a.wikiID, linkID,
		knowledge.UpdateFederationLinkParams{
			RemoteCanonicalKey: request.RemoteCanonicalKey,
			RemoteLabel:        request.RemoteLabel,
			RelationType:       request.RelationType,
			VerificationStatus: request.VerificationStatus,
			Status:             request.Status, Metadata: request.Metadata,
			ActorID: actorID,
		},
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toFederationLinkResponse(*item))
}

func (a *KnowledgeReadAPI) requireFederationManager(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return uuid.Nil, false
	}
	if a.authorization == nil {
		a.writeError(w, r, governance.ErrPermissionDenied)
		return uuid.Nil, false
	}
	if err := a.authorization.Check(
		r.Context(), actorID, a.wikiID, governance.ActionManage, nil,
	); err != nil {
		a.writeError(w, r, err)
		return uuid.Nil, false
	}
	return actorID, true
}

func federationPathID(
	w http.ResponseWriter,
	r *http.Request,
	name string,
) (uuid.UUID, bool) {
	value, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(
			w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			name+" 不是合法 UUID",
		)
		return uuid.Nil, false
	}
	return value, true
}

func toFederatedWikiResponse(item knowledge.FederatedWiki) federatedWikiResponse {
	return federatedWikiResponse{
		ID: item.ID, WikiID: item.WikiID, WikiKey: item.WikiKey,
		DisplayName: item.DisplayName, BaseURL: item.BaseURL,
		EntityURLTemplate: item.EntityURLTemplate,
		TrustLevel:        item.TrustLevel, Status: item.Status,
		Metadata: item.Metadata, CreatedBy: item.CreatedBy,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func toFederationLinkResponse(
	item knowledge.FederationLinkView,
) federationLinkResponse {
	return federationLinkResponse{
		ID: item.ID, WikiID: item.WikiID, LocalEntityID: item.LocalEntityID,
		LocalCanonicalKey: item.LocalCanonicalKey, LocalLabel: item.LocalLabel,
		LocalStatus: item.LocalStatus, RemoteWikiID: item.RemoteWikiID,
		RemoteWikiKey: item.RemoteWikiKey, RemoteWikiName: item.RemoteWikiName,
		RemoteWikiStatus:   item.RemoteWikiStatus,
		RemoteTrustLevel:   item.RemoteTrustLevel,
		RemoteEntityID:     item.RemoteEntityID,
		RemoteCanonicalKey: item.RemoteCanonicalKey,
		RemoteLabel:        item.RemoteLabel, RemoteEntityURL: item.RemoteEntityURL,
		RelationType:       item.RelationType,
		VerificationStatus: item.VerificationStatus,
		Status:             item.Status, Metadata: item.Metadata,
		CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func federationPageResponse(
	page *knowledge.FederationLinkPage,
) federationLinkListResponse {
	response := federationLinkListResponse{
		Items:      make([]federationLinkResponse, len(page.Items)),
		NextCursor: page.NextCursor,
	}
	for index, item := range page.Items {
		response.Items[index] = toFederationLinkResponse(item)
	}
	return response
}
