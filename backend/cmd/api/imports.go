package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/importer"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type ImportAPI struct {
	jobs     *importer.Service
	evidence *evidence.Service
	wikiID   uuid.UUID
}

func NewImportAPI(jobs *importer.Service) *ImportAPI { return &ImportAPI{jobs: jobs} }

func (a *ImportAPI) WithUploads(evidenceService *evidence.Service, wikiID uuid.UUID) *ImportAPI {
	a.evidence, a.wikiID = evidenceService, wikiID
	return a
}

type createImportJobRequest struct {
	JobType string          `json:"job_type"`
	Config  json.RawMessage `json:"config"`
}

func (a *ImportAPI) createJob(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 Idempotency-Key 请求头")
		return
	}
	var request createImportJobRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.JobType == "" {
		request.JobType = "source_import"
	}
	job, err := a.jobs.Create(r.Context(), actorID, request.JobType, key, request.Config)
	if err != nil {
		importError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, job)
}

func (a *ImportAPI) createUploadJob(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 Idempotency-Key 请求头")
		return
	}
	if a.evidence == nil || a.wikiID == uuid.Nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeInternal, "上传存储未配置")
		return
	}
	maxBytes := importer.DefaultURLPolicy().MaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes + 1); err != nil {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "上传内容超过限制或表单非法")
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
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(content)) > maxBytes {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "上传内容超过限制")
		return
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || strings.HasPrefix(mimeType, "application/octet-stream") {
		mimeType = http.DetectContentType(content)
	}
	acquired, err := importer.ValidateUpload(r.Context(), importer.DefaultURLPolicy(), importer.SignatureScanner{},
		header.Filename, mimeType, content)
	if err != nil {
		importError(w, r, err)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if len([]rune(title)) > 255 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "来源标题超过 255 个字符")
		return
	}
	asset, err := a.evidence.StoreAsset(r.Context(), evidence.StoreAssetParams{WikiID: a.wikiID,
		Name: acquired.Filename, Content: bytes.NewReader(acquired.Content), MimeType: acquired.MIMEType, ActorID: actorID})
	if err != nil {
		serviceError(w, r, err)
		return
	}
	config, _ := json.Marshal(map[string]any{"source": map[string]any{
		"kind": "upload", "storage_key": asset.Revision.StorageKey, "filename": acquired.Filename,
		"mime_type": acquired.MIMEType, "content_hash": acquired.ContentHash,
	}, "title": title})
	job, err := a.jobs.Create(r.Context(), actorID, "source_import", key, config)
	if err != nil {
		importError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, job)
}

func (a *ImportAPI) getJob(w http.ResponseWriter, r *http.Request) {
	actorID, id, ok := importActorAndID(w, r)
	if !ok {
		return
	}
	detail, err := a.jobs.Detail(r.Context(), id)
	if err != nil {
		importError(w, r, err)
		return
	}
	if detail.Job.InitiatedBy != actorID {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "只能读取自己创建的导入任务")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func (a *ImportAPI) cancelJob(w http.ResponseWriter, r *http.Request) {
	a.mutateOwnedJob(w, r, a.jobs.Cancel)
}

func (a *ImportAPI) retryJob(w http.ResponseWriter, r *http.Request) {
	a.mutateOwnedJob(w, r, a.jobs.Retry)
}

func (a *ImportAPI) mutateOwnedJob(w http.ResponseWriter, r *http.Request, mutate func(context.Context, uuid.UUID) error) {
	actorID, id, ok := importActorAndID(w, r)
	if !ok {
		return
	}
	job, err := a.jobs.DetailJob(r.Context(), id)
	if err != nil {
		importError(w, r, err)
		return
	}
	if job.InitiatedBy != actorID {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "只能操作自己创建的导入任务")
		return
	}
	if err := mutate(r.Context(), id); err != nil {
		importError(w, r, err)
		return
	}
	detail, err := a.jobs.Detail(r.Context(), id)
	if err != nil {
		importError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func importActorAndID(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "路径 id 不是合法 UUID")
		return uuid.Nil, uuid.Nil, false
	}
	return actorID, id, true
}

func importError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, importer.ErrJobNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, importer.ErrIdempotencyMismatch), errors.Is(err, importer.ErrInvalidTransition), errors.Is(err, importer.ErrCancelled):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, importer.ErrInvalidJob):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, err.Error())
	case errors.Is(err, importer.ErrSourceTooLarge):
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, err.Error())
	case errors.Is(err, importer.ErrUnsupportedMIME), errors.Is(err, importer.ErrMalware):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, err.Error())
	default:
		serviceError(w, r, err)
	}
}
