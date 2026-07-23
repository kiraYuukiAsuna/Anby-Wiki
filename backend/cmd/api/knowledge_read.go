// Knowledge/Evidence 只读详情 API（M4-T08）。
// 写入仍只能经各领域 Service；本文件仅把稳定 ID、状态与证据定位链投射为契约 DTO。
package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type KnowledgeReadAPI struct {
	knowledge     *knowledge.Service
	evidence      *evidence.Service
	authorization *governance.AuthorizationService
	wikiID        uuid.UUID
}

func NewKnowledgeReadAPI(knowledgeService *knowledge.Service, evidenceService *evidence.Service) *KnowledgeReadAPI {
	return &KnowledgeReadAPI{knowledge: knowledgeService, evidence: evidenceService}
}

func (a *KnowledgeReadAPI) WithMergeAuthorization(
	authorization *governance.AuthorizationService,
	wikiID uuid.UUID,
) *KnowledgeReadAPI {
	a.authorization = authorization
	a.wikiID = wikiID
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

func (a *KnowledgeReadAPI) getEntity(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	entity, err := a.knowledge.GetEntity(r.Context(), id)
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

func (a *KnowledgeReadAPI) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, knowledge.ErrEntityNotFound) || errors.Is(err, knowledge.ErrEntityTypeNotFound) ||
		errors.Is(err, knowledge.ErrClaimNotFound) || errors.Is(err, knowledge.ErrPropertyNotFound) ||
		errors.Is(err, evidence.ErrCitationNotFound) || errors.Is(err, evidence.ErrSourceNotFound) ||
		errors.Is(err, evidence.ErrSourceVersionNotFound) || errors.Is(err, evidence.ErrSourceChunkNotFound) ||
		errors.Is(err, evidence.ErrExternalResourceNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
		return
	}
	httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "内部错误")
}
