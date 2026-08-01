package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type SourceAPI struct {
	service *evidence.Service
}

func NewSourceAPI(service *evidence.Service) *SourceAPI {
	return &SourceAPI{service: service}
}

type sourceListResponse struct {
	Items      []sourceResponse `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

type sourceDetailCatalogResponse struct {
	sourceResponse
	ExternalResource *externalResourceCatalogResponse `json:"external_resource"`
}

type externalResourceCatalogResponse struct {
	ID                  uuid.UUID  `json:"id"`
	OriginalURL         string     `json:"original_url"`
	NormalizedURL       string     `json:"normalized_url"`
	CanonicalURL        *string    `json:"canonical_url"`
	Domain              string     `json:"domain"`
	HTTPStatus          *int32     `json:"http_status"`
	Status              string     `json:"status"`
	LastCheckedAt       *time.Time `json:"last_checked_at"`
	LastSuccessAt       *time.Time `json:"last_success_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

type sourceVersionCatalogResponse struct {
	ID               uuid.UUID  `json:"id"`
	SourceID         uuid.UUID  `json:"source_id"`
	VersionHash      string     `json:"version_hash"`
	RawAssetID       *uuid.UUID `json:"raw_asset_id"`
	ExtractedAssetID *uuid.UUID `json:"extracted_asset_id"`
	FetchedAt        time.Time  `json:"fetched_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

type sourceVersionListResponse struct {
	Items      []sourceVersionCatalogResponse `json:"items"`
	NextCursor *string                        `json:"next_cursor"`
}

type sourceChunkListResponse struct {
	Items      []sourceChunkResponse `json:"items"`
	NextCursor *string               `json:"next_cursor"`
}

type createSourceRequest struct {
	SourceType         string          `json:"source_type"`
	ExternalResourceID *uuid.UUID      `json:"external_resource_id"`
	URL                string          `json:"url"`
	AssetID            *uuid.UUID      `json:"asset_id"`
	Title              string          `json:"title"`
	Author             string          `json:"author"`
	Publisher          string          `json:"publisher"`
	PublishedAt        *time.Time      `json:"published_at"`
	Metadata           json.RawMessage `json:"metadata"`
}

type locatorRequest struct {
	Page        *int32              `json:"page"`
	Section     *string             `json:"section"`
	CharStart   *int32              `json:"char_start"`
	CharEnd     *int32              `json:"char_end"`
	ImageRegion *imageRegionRequest `json:"image_region"`
	OCR         *ocrInfoRequest     `json:"ocr"`
}

type imageRegionRequest struct {
	X           int32  `json:"x"`
	Y           int32  `json:"y"`
	Width       int32  `json:"width"`
	Height      int32  `json:"height"`
	ImageWidth  int32  `json:"image_width"`
	ImageHeight int32  `json:"image_height"`
	Unit        string `json:"unit"`
}

type ocrInfoRequest struct {
	Engine     string   `json:"engine"`
	Languages  []string `json:"languages"`
	Confidence *float64 `json:"confidence"`
}

type createCitationRequest struct {
	SourceVersionID uuid.UUID       `json:"source_version_id"`
	SourceChunkID   *uuid.UUID      `json:"source_chunk_id"`
	Locator         *locatorRequest `json:"locator"`
	Quotation       string          `json:"quotation"`
}

func toSourceResponse(value evidence.Source) sourceResponse {
	return sourceResponse{
		ID: value.ID, SourceType: value.SourceType, Title: value.Title,
		Author: value.Author, Publisher: value.Publisher,
		PublishedAt: value.PublishedAt, Metadata: value.MetadataJSON,
	}
}

func toSourceVersionCatalogResponse(value evidence.SourceVersion) sourceVersionCatalogResponse {
	return sourceVersionCatalogResponse{
		ID: value.ID, SourceID: value.SourceID, VersionHash: value.VersionHash,
		RawAssetID: value.RawAssetID, ExtractedAssetID: value.ExtractedAssetID,
		FetchedAt: value.FetchedAt, CreatedAt: value.CreatedAt,
	}
}

func (a *SourceAPI) list(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.ListSources(
		r.Context(), r.URL.Query().Get("source_type"), r.URL.Query().Get("q"),
		r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		sourceError(w, r, err)
		return
	}
	items := make([]sourceResponse, len(result.Items))
	for index := range result.Items {
		items[index] = toSourceResponse(result.Items[index])
	}
	httpx.WriteJSON(w, http.StatusOK, sourceListResponse{
		Items: items, NextCursor: result.NextCursor,
	})
}

func (a *SourceAPI) create(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var body createSourceRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	value, err := a.service.CreateSource(r.Context(), evidence.CreateSourceParams{
		SourceType: body.SourceType, ExternalResourceID: body.ExternalResourceID,
		URL: body.URL, AssetID: body.AssetID, Title: body.Title,
		Author: body.Author, Publisher: body.Publisher, PublishedAt: body.PublishedAt,
		Metadata: body.Metadata, ActorID: actorID,
	})
	if err != nil {
		sourceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toSourceResponse(*value))
}

func (a *SourceAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := sourcePathID(w, r, "id")
	if !ok {
		return
	}
	value, err := a.service.GetSourceDetail(r.Context(), id)
	if err != nil {
		sourceError(w, r, err)
		return
	}
	response := sourceDetailCatalogResponse{sourceResponse: toSourceResponse(value.Source)}
	if value.ExternalResource != nil {
		resource := value.ExternalResource
		response.ExternalResource = &externalResourceCatalogResponse{
			ID: resource.ID, OriginalURL: resource.OriginalURL,
			NormalizedURL: resource.NormalizedURL, CanonicalURL: resource.CanonicalURL,
			Domain: resource.Domain, HTTPStatus: resource.HTTPStatus, Status: resource.Status,
			LastCheckedAt: resource.LastCheckedAt, LastSuccessAt: resource.LastSuccessAt,
			ConsecutiveFailures: resource.ConsecutiveFailures,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (a *SourceAPI) versions(w http.ResponseWriter, r *http.Request) {
	id, ok := sourcePathID(w, r, "id")
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.ListSourceVersions(
		r.Context(), id, r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		sourceError(w, r, err)
		return
	}
	items := make([]sourceVersionCatalogResponse, len(result.Items))
	for index := range result.Items {
		items[index] = toSourceVersionCatalogResponse(result.Items[index])
	}
	httpx.WriteJSON(w, http.StatusOK, sourceVersionListResponse{
		Items: items, NextCursor: result.NextCursor,
	})
}

func (a *SourceAPI) chunks(w http.ResponseWriter, r *http.Request) {
	id, ok := sourcePathID(w, r, "id")
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.ListSourceChunksPage(
		r.Context(), id, r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		sourceError(w, r, err)
		return
	}
	items := make([]sourceChunkResponse, len(result.Items))
	for index := range result.Items {
		value := result.Items[index]
		items[index] = sourceChunkResponse{
			ID: value.ID, Ordinal: value.Ordinal, Locator: value.LocatorJSON,
			TextContent: value.TextContent, TextHash: value.TextHash,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, sourceChunkListResponse{
		Items: items, NextCursor: result.NextCursor,
	})
}

func (a *SourceAPI) createCitation(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var body createCitationRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	var locator *evidence.Locator
	if body.Locator != nil {
		locator = &evidence.Locator{
			Page: body.Locator.Page, Section: body.Locator.Section,
			CharStart: body.Locator.CharStart, CharEnd: body.Locator.CharEnd,
		}
		if body.Locator.ImageRegion != nil {
			region := body.Locator.ImageRegion
			locator.ImageRegion = &evidence.ImageRegion{
				X: region.X, Y: region.Y, Width: region.Width, Height: region.Height,
				ImageWidth: region.ImageWidth, ImageHeight: region.ImageHeight,
				Unit: region.Unit,
			}
		}
		if body.Locator.OCR != nil {
			ocr := body.Locator.OCR
			locator.OCR = &evidence.OCRInfo{
				Engine: ocr.Engine, Languages: ocr.Languages,
				Confidence: ocr.Confidence,
			}
		}
	}
	value, err := a.service.CreateCitation(r.Context(), evidence.CreateCitationParams{
		SourceVersionID: body.SourceVersionID, SourceChunkID: body.SourceChunkID,
		Locator: locator, Quotation: body.Quotation, ActorID: actorID,
	})
	if err != nil {
		sourceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id": value.ID, "source_version_id": value.SourceVersionID,
		"source_chunk_id": value.SourceChunkID, "locator": value.LocatorJSON,
		"quotation": value.Quotation, "quotation_hash": value.QuotationHash,
		"created_by": value.CreatedBy, "created_at": value.CreatedAt,
	})
}

func sourcePathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			strings.ReplaceAll(name, "_", " ")+" 不是合法 UUID")
		return uuid.Nil, false
	}
	return id, true
}

func sourceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, evidence.ErrSourceNotFound),
		errors.Is(err, evidence.ErrSourceVersionNotFound),
		errors.Is(err, evidence.ErrSourceChunkNotFound),
		errors.Is(err, evidence.ErrCitationNotFound),
		errors.Is(err, evidence.ErrExternalResourceNotFound),
		errors.Is(err, evidence.ErrAssetNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, evidence.ErrInvalidSourceCursor),
		errors.Is(err, evidence.ErrInvalidSourceInput),
		errors.Is(err, evidence.ErrInvalidURL),
		errors.Is(err, evidence.ErrInvalidLocator),
		errors.Is(err, evidence.ErrInvalidChunkOrdinal),
		errors.Is(err, evidence.ErrChunkVersionMismatch),
		errors.Is(err, evidence.ErrQuotationMismatch):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
	default:
		serviceError(w, r, err)
	}
}
