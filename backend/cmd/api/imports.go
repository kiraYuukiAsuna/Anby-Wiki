package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/importer"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type ImportAPI struct {
	jobs        *importer.Service
	evidence    *evidence.Service
	uploadSlots chan struct{}
}

func NewImportAPI(jobs *importer.Service) *ImportAPI { return &ImportAPI{jobs: jobs} }

func (a *ImportAPI) WithUploads(
	evidenceService *evidence.Service,
	maxConcurrentUploads int,
) *ImportAPI {
	if maxConcurrentUploads < 1 {
		maxConcurrentUploads = 1
	}
	a.evidence = evidenceService
	a.uploadSlots = make(chan struct{}, maxConcurrentUploads)
	return a
}

const (
	importUploadFormMemoryBytes   = 1 << 20
	importUploadRetryAfterSeconds = 1
)

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
	job, err := a.jobs.Create(
		r.Context(), actorID, wikiIDFromRequest(r),
		request.JobType, key, request.Config,
	)
	if err != nil {
		importError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, job)
}

func (a *ImportAPI) listJobs(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.jobs.ListOwned(
		r.Context(), actorID, r.URL.Query().Get("status"),
		r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		importError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
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
	if a.evidence == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeInternal, "上传存储未配置")
		return
	}
	if !a.acquireUploadSlot() {
		w.Header().Set("Retry-After", strconv.Itoa(importUploadRetryAfterSeconds))
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeRateLimited,
			"上传处理繁忙，请稍后重试")
		return
	}
	defer a.releaseUploadSlot()
	maxBytes := importer.DefaultURLPolicy().MaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(importUploadFormMemoryBytes); err != nil {
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
	if header.Size > maxBytes {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "上传内容超过限制")
		return
	}
	mimeType := header.Header.Get("Content-Type")
	acquired, err := importer.ValidateUploadReader(
		r.Context(), importer.DefaultURLPolicy(), importer.SignatureScanner{},
		header.Filename, mimeType, file,
	)
	if err != nil {
		importError(w, r, err)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if len([]rune(title)) > 255 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "来源标题超过 255 个字符")
		return
	}
	instructions := strings.TrimSpace(r.FormValue("instructions"))
	if len([]rune(instructions)) > 4000 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "导入说明超过 4000 个字符")
		return
	}
	ingestionMode := strings.TrimSpace(r.FormValue("ingestion_mode"))
	if ingestionMode == "" {
		ingestionMode = importer.IngestionModeWiki
	}
	if ingestionMode != importer.IngestionModeWiki &&
		ingestionMode != importer.IngestionModeSourceOnly {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "导入模式不合法")
		return
	}
	routeMode := strings.TrimSpace(r.FormValue("route_mode"))
	if routeMode == "" {
		routeMode = importer.RouteModeAuto
	}
	if routeMode != importer.RouteModeAuto && routeMode != importer.RouteModeForceCreate &&
		routeMode != importer.RouteModeForceUpdate {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "页面路由模式不合法")
		return
	}
	if routeMode == importer.RouteModeForceCreate && title == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "强制创建页面时必须填写标题")
		return
	}
	var pageID *uuid.UUID
	if rawPageID := strings.TrimSpace(r.FormValue("page_id")); rawPageID != "" {
		parsedPageID, err := uuid.Parse(rawPageID)
		if err != nil {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "目标页面 ID 不合法")
			return
		}
		pageID = &parsedPageID
	}
	if routeMode == importer.RouteModeForceUpdate && pageID == nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "更新指定页面时必须选择目标页面")
		return
	}
	existing, err := a.jobs.FindUploadByKey(r.Context(), actorID, key, importer.UploadIdentity{
		Filename: acquired.Filename, MIMEType: acquired.MIMEType,
		ContentHash: acquired.ContentHash, IngestionMode: ingestionMode, Title: title,
		Instructions: instructions, RouteMode: routeMode, PageID: pageID,
	})
	if err == nil {
		httpx.WriteJSON(w, http.StatusCreated, existing)
		return
	}
	if !errors.Is(err, importer.ErrJobNotFound) {
		importError(w, r, err)
		return
	}
	asset, err := a.evidence.StoreAsset(r.Context(), evidence.StoreAssetParams{WikiID: wikiIDFromRequest(r),
		Name: acquired.Filename, Content: file, MimeType: acquired.MIMEType, ActorID: actorID})
	if err != nil {
		serviceError(w, r, err)
		return
	}
	config, _ := json.Marshal(map[string]any{"source": map[string]any{
		"kind": "upload", "storage_key": asset.Revision.StorageKey, "filename": acquired.Filename,
		"mime_type": acquired.MIMEType, "content_hash": acquired.ContentHash,
	}, "ingestion_mode": ingestionMode, "title": title,
		"instructions": instructions, "route_mode": routeMode,
		"page_id": pageID})
	job, err := a.jobs.Create(
		r.Context(), actorID, wikiIDFromRequest(r),
		"source_import", key, config,
	)
	if err != nil {
		importError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, job)
}

func (a *ImportAPI) acquireUploadSlot() bool {
	select {
	case a.uploadSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *ImportAPI) releaseUploadSlot() {
	<-a.uploadSlots
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
	if detail.Job.WikiID != wikiIDFromRequest(r) ||
		detail.Job.InitiatedBy != actorID {
		importError(w, r, importer.ErrJobNotFound)
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

type confirmImportPlanRequest struct {
	PlanID uuid.UUID `json:"plan_id"`
}

func (a *ImportAPI) confirmPlan(w http.ResponseWriter, r *http.Request) {
	actorID, id, ok := importActorAndID(w, r)
	if !ok {
		return
	}
	job, err := a.jobs.DetailJob(r.Context(), id)
	if err != nil {
		importError(w, r, err)
		return
	}
	if job.WikiID != wikiIDFromRequest(r) || job.InitiatedBy != actorID {
		importError(w, r, importer.ErrJobNotFound)
		return
	}
	var request confirmImportPlanRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.PlanID == uuid.Nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, "plan_id 不能为空")
		return
	}
	if err := a.jobs.ConfirmPlan(r.Context(), id, request.PlanID); err != nil {
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

type replanImportJobRequest struct {
	Title        string     `json:"title"`
	Instructions string     `json:"instructions"`
	RouteMode    string     `json:"route_mode"`
	PageID       *uuid.UUID `json:"page_id"`
}

func (a *ImportAPI) replanJob(w http.ResponseWriter, r *http.Request) {
	actorID, id, ok := importActorAndID(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "缺少 Idempotency-Key 请求头")
		return
	}
	parent, err := a.jobs.DetailJob(r.Context(), id)
	if err != nil {
		importError(w, r, err)
		return
	}
	if parent.WikiID != wikiIDFromRequest(r) || parent.InitiatedBy != actorID {
		importError(w, r, importer.ErrJobNotFound)
		return
	}
	var request replanImportJobRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	job, err := a.jobs.Replan(r.Context(), actorID, id, key, importer.ReplanInput{
		Title: request.Title, Instructions: request.Instructions,
		RouteMode: request.RouteMode, PageID: request.PageID,
	})
	if err != nil {
		importError(w, r, err)
		return
	}
	detail, err := a.jobs.Detail(r.Context(), job.ID)
	if err != nil {
		importError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
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
	if job.WikiID != wikiIDFromRequest(r) || job.InitiatedBy != actorID {
		importError(w, r, importer.ErrJobNotFound)
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
	case errors.Is(err, importer.ErrJobNotFound), errors.Is(err, importer.ErrImportPlanNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, importer.ErrInvalidCursor), errors.Is(err, importer.ErrInvalidStatus):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
	case errors.Is(err, importer.ErrIdempotencyMismatch), errors.Is(err, importer.ErrInvalidTransition),
		errors.Is(err, importer.ErrCancelled), errors.Is(err, importer.ErrQualityGate):
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
