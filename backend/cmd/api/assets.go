package main

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/platform/storage"
)

const maxAssetUploadBytes int64 = 50 << 20

type AssetAPI struct {
	service *evidence.Service
	wikiID  uuid.UUID
}

func NewAssetAPI(service *evidence.Service, wikiID uuid.UUID) *AssetAPI {
	return &AssetAPI{service: service, wikiID: wikiID}
}

type assetRevisionResponse struct {
	ID          uuid.UUID `json:"id"`
	AssetID     uuid.UUID `json:"asset_id"`
	ContentHash string    `json:"content_hash"`
	MimeType    string    `json:"mime_type"`
	SizeBytes   int64     `json:"size_bytes"`
	Width       *int32    `json:"width"`
	Height      *int32    `json:"height"`
	CreatedAt   string    `json:"created_at"`
}

type assetResponse struct {
	ID              uuid.UUID             `json:"id"`
	Name            string                `json:"name"`
	Status          string                `json:"status"`
	CurrentRevision assetRevisionResponse `json:"current_revision"`
	CreatedAt       string                `json:"created_at"`
	UpdatedAt       string                `json:"updated_at"`
}

type assetListResponse struct {
	Items      []assetResponse `json:"items"`
	NextCursor *string         `json:"next_cursor"`
}

func toAssetResponse(value evidence.AssetWithRevision) assetResponse {
	return assetResponse{
		ID: value.Asset.ID, Name: value.Asset.Name, Status: value.Asset.Status,
		CurrentRevision: assetRevisionResponse{
			ID: value.Revision.ID, AssetID: value.Revision.AssetID,
			ContentHash: value.Revision.ContentHash, MimeType: value.Revision.MimeType,
			SizeBytes: value.Revision.SizeBytes, Width: value.Revision.Width,
			Height:    value.Revision.Height,
			CreatedAt: value.Revision.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		},
		CreatedAt: value.Asset.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt: value.Asset.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

func (a *AssetAPI) list(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	page, err := a.service.ListAssets(
		r.Context(), a.wikiID, r.URL.Query().Get("kind"),
		r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		assetError(w, r, err)
		return
	}
	items := make([]assetResponse, len(page.Items))
	for index := range page.Items {
		items[index] = toAssetResponse(page.Items[index])
	}
	httpx.WriteJSON(w, http.StatusOK, assetListResponse{
		Items: items, NextCursor: page.NextCursor,
	})
}

func (a *AssetAPI) upload(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAssetUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAssetUploadBytes); err != nil {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest,
			"资产超过 50 MiB 或表单非法")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 file 表单字段")
		return
	}
	defer file.Close()
	if header.Size > maxAssetUploadBytes {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest,
			"资产超过 50 MiB")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSpace(header.Filename)
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	result, err := a.service.StoreAsset(r.Context(), evidence.StoreAssetParams{
		WikiID: a.wikiID, Name: name, Content: io.LimitReader(file, maxAssetUploadBytes+1),
		MimeType: mimeType, ActorID: actorID,
	})
	if err != nil {
		assetError(w, r, err)
		return
	}
	value := evidence.AssetWithRevision{Asset: *result.Asset, Revision: *result.Revision}
	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, toAssetResponse(value))
}

func (a *AssetAPI) getRevision(w http.ResponseWriter, r *http.Request) {
	revisionID, ok := assetRevisionIDFrom(w, r)
	if !ok {
		return
	}
	value, err := a.service.GetAssetRevision(r.Context(), a.wikiID, revisionID)
	if err != nil {
		assetError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAssetResponse(*value).CurrentRevision)
}

func (a *AssetAPI) content(w http.ResponseWriter, r *http.Request) {
	revisionID, ok := assetRevisionIDFrom(w, r)
	if !ok {
		return
	}
	result, err := a.service.OpenAssetRevision(r.Context(), a.wikiID, revisionID)
	if err != nil {
		assetError(w, r, err)
		return
	}
	defer result.Content.Close()
	revision := result.Value.Revision
	etag := `"` + revision.ContentHash + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", revision.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(revision.SizeBytes, 10))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if disposition := mime.FormatMediaType("inline", map[string]string{
		"filename": result.Value.Asset.Name,
	}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, result.Content)
}

func assetRevisionIDFrom(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "revision_id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			"asset revision id 不是合法 UUID")
		return uuid.Nil, false
	}
	return id, true
}

func assetError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, evidence.ErrAssetNotFound),
		errors.Is(err, evidence.ErrAssetRevisionNotFound),
		errors.Is(err, storage.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "资产版本不存在")
	case errors.Is(err, evidence.ErrInvalidAssetCursor),
		errors.Is(err, evidence.ErrInvalidAssetInput):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
	case errors.Is(err, evidence.ErrAssetStorageUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeInternal, "对象存储未配置")
	default:
		serviceError(w, r, err)
	}
}
