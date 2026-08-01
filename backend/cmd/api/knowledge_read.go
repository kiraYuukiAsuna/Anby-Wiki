// Knowledge/Evidence 只读详情 API（M4-T08）。
// 写入仍只能经各领域 Service；本文件仅把稳定 ID、状态与证据定位链投射为契约 DTO。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/projection"
)

type KnowledgeReadAPI struct {
	knowledge     *knowledge.Service
	evidence      *evidence.Service
	authorization *governance.AuthorizationService
	consistency   *knowledge.FactConsistencyService
	entityGraph   *projection.EntityGraphService
	wikiID        uuid.UUID
}

func NewKnowledgeReadAPI(
	knowledgeService *knowledge.Service,
	evidenceService *evidence.Service,
	wikiID uuid.UUID,
) *KnowledgeReadAPI {
	return &KnowledgeReadAPI{knowledge: knowledgeService, evidence: evidenceService, wikiID: wikiID}
}

func (a *KnowledgeReadAPI) WithMergeAuthorization(
	authorization *governance.AuthorizationService,
) *KnowledgeReadAPI {
	a.authorization = authorization
	return a
}

func (a *KnowledgeReadAPI) WithFactConsistency(
	service *knowledge.FactConsistencyService,
) *KnowledgeReadAPI {
	a.consistency = service
	return a
}

func (a *KnowledgeReadAPI) WithEntityGraph(
	service *projection.EntityGraphService,
) *KnowledgeReadAPI {
	a.entityGraph = service
	return a
}

type entityTypeResponse struct {
	ID      uuid.UUID `json:"id"`
	TypeKey string    `json:"type_key"`
	Name    string    `json:"name"`
}

type entityLabelResponse struct {
	Language    string `json:"language"`
	Label       string `json:"label"`
	Description string `json:"description"`
	IsPrimary   bool   `json:"is_primary"`
}

type entityAliasResponse struct {
	ID        uuid.UUID `json:"id"`
	Language  string    `json:"language"`
	Alias     string    `json:"alias"`
	AliasType string    `json:"alias_type"`
}

type entityDetailResponse struct {
	ID                 uuid.UUID             `json:"id"`
	WikiID             uuid.UUID             `json:"wiki_id"`
	CanonicalKey       string                `json:"canonical_key"`
	Status             string                `json:"status"`
	MergedIntoEntityID *uuid.UUID            `json:"merged_into_entity_id"`
	EntityType         entityTypeResponse    `json:"entity_type"`
	Labels             []entityLabelResponse `json:"labels"`
	Aliases            []entityAliasResponse `json:"aliases"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
}

type entityCatalogItemResponse struct {
	ID                 uuid.UUID          `json:"id"`
	WikiID             uuid.UUID          `json:"wiki_id"`
	CanonicalKey       string             `json:"canonical_key"`
	Status             string             `json:"status"`
	MergedIntoEntityID *uuid.UUID         `json:"merged_into_entity_id"`
	EntityType         entityTypeResponse `json:"entity_type"`
	DisplayLabel       string             `json:"display_label"`
	DisplayLanguage    string             `json:"display_language"`
	Description        string             `json:"description"`
	LabelCount         int                `json:"label_count"`
	AliasCount         int                `json:"alias_count"`
	ClaimCount         int                `json:"claim_count"`
	PageCount          int                `json:"page_count"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

type entityCatalogPageResponse struct {
	Items      []entityCatalogItemResponse `json:"items"`
	NextCursor *string                     `json:"next_cursor"`
}

type propertyResponse struct {
	ID            uuid.UUID       `json:"id"`
	PropertyKey   string          `json:"property_key"`
	Name          string          `json:"name"`
	ValueType     string          `json:"value_type"`
	IsMultivalued bool            `json:"is_multivalued"`
	Schema        json.RawMessage `json:"schema"`
}

type claimSourceResponse struct {
	CitationID  uuid.UUID `json:"citation_id"`
	SupportType string    `json:"support_type"`
	CreatedAt   time.Time `json:"created_at"`
}

type claimDetailResponse struct {
	ID                 uuid.UUID             `json:"id"`
	SubjectEntityID    uuid.UUID             `json:"subject_entity_id"`
	Property           propertyResponse      `json:"property"`
	ValueType          string                `json:"value_type"`
	Value              json.RawMessage       `json:"value"`
	TargetEntityID     *uuid.UUID            `json:"target_entity_id"`
	Qualifiers         json.RawMessage       `json:"qualifiers"`
	Rank               string                `json:"rank"`
	Status             string                `json:"status"`
	VerificationStatus string                `json:"verification_status"`
	ValidFrom          *time.Time            `json:"valid_from"`
	ValidTo            *time.Time            `json:"valid_to"`
	OriginType         string                `json:"origin_type"`
	CreatedBy          uuid.UUID             `json:"created_by"`
	CreatedAt          time.Time             `json:"created_at"`
	SupersededBy       *uuid.UUID            `json:"superseded_by"`
	Sources            []claimSourceResponse `json:"sources"`
}

type sourceResponse struct {
	ID          uuid.UUID       `json:"id"`
	SourceType  string          `json:"source_type"`
	Title       string          `json:"title"`
	Author      *string         `json:"author"`
	Publisher   *string         `json:"publisher"`
	PublishedAt *time.Time      `json:"published_at"`
	Metadata    json.RawMessage `json:"metadata"`
}

type sourceVersionResponse struct {
	ID          uuid.UUID `json:"id"`
	VersionHash string    `json:"version_hash"`
	FetchedAt   time.Time `json:"fetched_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type sourceChunkResponse struct {
	ID          uuid.UUID       `json:"id"`
	Ordinal     int             `json:"ordinal"`
	Locator     json.RawMessage `json:"locator"`
	TextContent string          `json:"text_content"`
	TextHash    string          `json:"text_hash"`
}

type externalResourceResponse struct {
	ID            uuid.UUID `json:"id"`
	OriginalURL   string    `json:"original_url"`
	NormalizedURL string    `json:"normalized_url"`
	CanonicalURL  *string   `json:"canonical_url"`
	Status        string    `json:"status"`
}

type citationDetailResponse struct {
	ID               uuid.UUID                 `json:"id"`
	SourceVersionID  uuid.UUID                 `json:"source_version_id"`
	SourceChunkID    *uuid.UUID                `json:"source_chunk_id"`
	Locator          json.RawMessage           `json:"locator"`
	Quotation        *string                   `json:"quotation"`
	QuotationHash    *string                   `json:"quotation_hash"`
	CreatedBy        uuid.UUID                 `json:"created_by"`
	CreatedAt        time.Time                 `json:"created_at"`
	Source           sourceResponse            `json:"source"`
	SourceVersion    sourceVersionResponse     `json:"source_version"`
	SourceChunk      *sourceChunkResponse      `json:"source_chunk"`
	ExternalResource *externalResourceResponse `json:"external_resource"`
}

func (a *KnowledgeReadAPI) listEntities(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	page, err := a.knowledge.ListEntities(r.Context(), knowledge.EntityCatalogParams{
		WikiID: a.wikiID, Query: r.URL.Query().Get("q"),
		TypeKey: r.URL.Query().Get("type_key"), Status: r.URL.Query().Get("status"),
		Cursor: r.URL.Query().Get("cursor"), Limit: limit,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	items := make([]entityCatalogItemResponse, len(page.Items))
	for i, item := range page.Items {
		items[i] = entityCatalogItemResponse{
			ID: item.ID, WikiID: item.WikiID, CanonicalKey: item.CanonicalKey,
			Status: item.Status, MergedIntoEntityID: item.MergedIntoEntityID,
			EntityType: entityTypeResponse{
				ID: item.EntityType.ID, TypeKey: item.EntityType.TypeKey, Name: item.EntityType.Name,
			},
			DisplayLabel: item.DisplayLabel, DisplayLanguage: item.DisplayLanguage,
			Description: item.Description, LabelCount: item.LabelCount,
			AliasCount: item.AliasCount, ClaimCount: item.ClaimCount,
			PageCount: item.PageCount, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, entityCatalogPageResponse{
		Items: items, NextCursor: page.NextCursor,
	})
}

func (a *KnowledgeReadAPI) getEntity(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	entity, err := a.currentWikiEntity(r.Context(), id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	entityType, err := a.knowledge.GetEntityType(r.Context(), entity.EntityTypeID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	labels, err := a.knowledge.ListLabels(r.Context(), id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	aliases, err := a.knowledge.ListAliases(r.Context(), id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	resp := entityDetailResponse{
		ID: entity.ID, WikiID: entity.WikiID, CanonicalKey: entity.CanonicalKey,
		Status: entity.Status, MergedIntoEntityID: entity.MergedIntoEntityID,
		EntityType: entityTypeResponse{ID: entityType.ID, TypeKey: entityType.TypeKey, Name: entityType.Name},
		Labels:     make([]entityLabelResponse, len(labels)), Aliases: make([]entityAliasResponse, len(aliases)),
		CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
	for i, label := range labels {
		resp.Labels[i] = entityLabelResponse{Language: label.Language, Label: label.Label,
			Description: label.Description, IsPrimary: label.IsPrimary}
	}
	for i, alias := range aliases {
		resp.Aliases[i] = entityAliasResponse{ID: alias.ID, Language: alias.Language,
			Alias: alias.Alias, AliasType: alias.AliasType}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *KnowledgeReadAPI) getClaim(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	claim, err := a.knowledge.GetClaim(r.Context(), id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if _, err := a.currentWikiEntity(
		r.Context(), claim.SubjectEntityID,
	); err != nil {
		a.writeError(w, r, knowledge.ErrClaimNotFound)
		return
	}
	property, err := a.knowledge.GetProperty(r.Context(), claim.PropertyID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	sources, err := a.knowledge.ListClaimSources(r.Context(), id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	resp := claimDetailResponse{
		ID: claim.ID, SubjectEntityID: claim.SubjectEntityID,
		Property: propertyResponse{ID: property.ID, PropertyKey: property.PropertyKey,
			Name: property.Name, ValueType: property.ValueType,
			IsMultivalued: property.IsMultivalued, Schema: property.SchemaJSON},
		ValueType: claim.ValueType, Value: claim.ValueJSON, TargetEntityID: claim.TargetEntityID,
		Qualifiers: claim.QualifiersJSON, Rank: claim.Rank, Status: claim.Status,
		VerificationStatus: claim.VerificationStatus, ValidFrom: claim.ValidFrom, ValidTo: claim.ValidTo,
		OriginType: claim.OriginType, CreatedBy: claim.CreatedBy, CreatedAt: claim.CreatedAt,
		SupersededBy: claim.SupersededBy, Sources: make([]claimSourceResponse, len(sources)),
	}
	for i, source := range sources {
		resp.Sources[i] = claimSourceResponse{CitationID: source.CitationID,
			SupportType: source.SupportType, CreatedAt: source.CreatedAt}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// currentWikiEntity turns an opaque Entity ID into a site-scoped lookup.
// A foreign-Wiki ID is deliberately indistinguishable from a missing ID.
func (a *KnowledgeReadAPI) currentWikiEntity(
	ctx context.Context,
	id uuid.UUID,
) (*knowledge.Entity, error) {
	entity, err := a.knowledge.GetEntity(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity.WikiID != a.wikiID {
		return nil, fmt.Errorf("%w: id=%s", knowledge.ErrEntityNotFound, id)
	}
	return entity, nil
}

func (a *KnowledgeReadAPI) getCitation(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	detail, err := a.evidence.GetCitationDetail(r.Context(), id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	resp := citationDetailResponse{
		ID: detail.Citation.ID, SourceVersionID: detail.Citation.SourceVersionID,
		SourceChunkID: detail.Citation.SourceChunkID, Locator: detail.Citation.LocatorJSON,
		Quotation: detail.Citation.Quotation, QuotationHash: detail.Citation.QuotationHash,
		CreatedBy: detail.Citation.CreatedBy, CreatedAt: detail.Citation.CreatedAt,
		Source: sourceResponse{ID: detail.Source.ID, SourceType: detail.Source.SourceType,
			Title: detail.Source.Title, Author: detail.Source.Author, Publisher: detail.Source.Publisher,
			PublishedAt: detail.Source.PublishedAt, Metadata: detail.Source.MetadataJSON},
		SourceVersion: sourceVersionResponse{ID: detail.SourceVersion.ID,
			VersionHash: detail.SourceVersion.VersionHash, FetchedAt: detail.SourceVersion.FetchedAt,
			CreatedAt: detail.SourceVersion.CreatedAt},
	}
	if detail.SourceChunk != nil {
		resp.SourceChunk = &sourceChunkResponse{ID: detail.SourceChunk.ID,
			Ordinal: detail.SourceChunk.Ordinal, Locator: detail.SourceChunk.LocatorJSON,
			TextContent: detail.SourceChunk.TextContent, TextHash: detail.SourceChunk.TextHash}
	}
	if detail.ExternalResource != nil {
		resp.ExternalResource = &externalResourceResponse{ID: detail.ExternalResource.ID,
			OriginalURL:   detail.ExternalResource.OriginalURL,
			NormalizedURL: detail.ExternalResource.NormalizedURL,
			CanonicalURL:  detail.ExternalResource.CanonicalURL, Status: detail.ExternalResource.Status}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *KnowledgeReadAPI) listFactConsistencyIssues(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if a.authorization == nil {
		a.writeError(w, r, governance.ErrPermissionDenied)
		return
	}
	if err := a.authorization.Check(
		r.Context(), actorID, a.wikiID, governance.ActionReview, nil,
	); err != nil {
		a.writeError(w, r, err)
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.consistency.List(
		r.Context(), knowledge.FactConsistencyListParams{
			WikiID: a.wikiID, Status: r.URL.Query().Get("status"),
			IssueType: r.URL.Query().Get("issue_type"),
			Cursor:    r.URL.Query().Get("cursor"), Limit: limit,
		},
	)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *KnowledgeReadAPI) scanFactConsistency(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if a.authorization == nil {
		a.writeError(w, r, governance.ErrPermissionDenied)
		return
	}
	if err := a.authorization.Check(
		r.Context(), actorID, a.wikiID, governance.ActionManage, nil,
	); err != nil {
		a.writeError(w, r, err)
		return
	}
	result, err := a.consistency.ScanWiki(r.Context(), a.wikiID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *KnowledgeReadAPI) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, knowledge.ErrInvalidEntityCursor) ||
		errors.Is(err, knowledge.ErrInvalidEntityFilter) ||
		errors.Is(err, knowledge.ErrInvalidLabel) ||
		errors.Is(err, knowledge.ErrInvalidConsistencyFilter) ||
		errors.Is(err, knowledge.ErrInvalidConsistencyCursor) ||
		errors.Is(err, knowledge.ErrInvalidFederation) ||
		errors.Is(err, projection.ErrInvalidGraphQuery) {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	if errors.Is(err, governance.ErrPermissionDenied) ||
		errors.Is(err, governance.ErrInvalidActor) {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, err.Error())
		return
	}
	if errors.Is(err, knowledge.ErrEntityNotFound) || errors.Is(err, knowledge.ErrEntityTypeNotFound) ||
		errors.Is(err, knowledge.ErrClaimNotFound) || errors.Is(err, knowledge.ErrPropertyNotFound) ||
		errors.Is(err, knowledge.ErrFederatedWikiNotFound) ||
		errors.Is(err, knowledge.ErrFederationLinkNotFound) ||
		errors.Is(err, projection.ErrGraphEntityNotFound) ||
		errors.Is(err, projection.ErrGraphPropertyNotFound) ||
		errors.Is(err, evidence.ErrCitationNotFound) || errors.Is(err, evidence.ErrSourceNotFound) ||
		errors.Is(err, evidence.ErrSourceVersionNotFound) || errors.Is(err, evidence.ErrSourceChunkNotFound) ||
		errors.Is(err, evidence.ErrExternalResourceNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
		return
	}
	if errors.Is(err, knowledge.ErrFederatedWikiExists) ||
		errors.Is(err, knowledge.ErrFederationLinkExists) ||
		errors.Is(err, knowledge.ErrFederatedWikiDisabled) {
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
		return
	}
	httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "内部错误")
}
