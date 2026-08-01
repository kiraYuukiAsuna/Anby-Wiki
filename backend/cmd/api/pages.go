// 页面写 API handlers（M1-T05）：创建页面、改名、发布 Revision。
// handler 层保持薄：解析/校验输入 → 调领域服务 → 按契约 Error 模型映射错误。
// Actor 身份只从认证中间件写入的 request context 读取。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

// WriteAPI 写 API 依赖集合：Page 领域服务与启动时解析缓存的默认站点 ID。
type WriteAPI struct {
	pages                  *page.Service
	wikiID                 uuid.UUID
	auth                   *governance.AuthorizationService
	collaborationPublisher *collaboration.Publisher
}

// NewWriteAPI 装配写 API。wikiID 为种子里 site_key='default' 的站点，
// 由 main 启动时解析一次并缓存。
func NewWriteAPI(pages *page.Service, wikiID uuid.UUID) *WriteAPI {
	return &WriteAPI{pages: pages, wikiID: wikiID}
}

// WithAuthorization 在生产装配中启用 Role 与 PageProtection 检查。
// 未注入时只保留 Page 领域服务本身的 Actor 类型边界。
func (a *WriteAPI) WithAuthorization(auth *governance.AuthorizationService) *WriteAPI {
	a.auth = auth
	return a
}

func (a *WriteAPI) WithCollaborationPublisher(publisher *collaboration.Publisher) *WriteAPI {
	a.collaborationPublisher = publisher
	return a
}

func (a *WriteAPI) authorize(w http.ResponseWriter, r *http.Request, actorID uuid.UUID, action string, pageID *uuid.UUID) bool {
	if pageID != nil {
		if _, err := getPageInWiki(
			r.Context(), a.pages, a.wikiID, *pageID,
		); err != nil {
			serviceError(w, r, err)
			return false
		}
	}
	if a.auth == nil {
		return true
	}
	if err := a.auth.Check(r.Context(), actorID, a.wikiID, action, pageID); err != nil {
		governanceError(w, r, err)
		return false
	}
	return true
}

func (a *WriteAPI) authorizeCreate(w http.ResponseWriter, r *http.Request, actorID, namespaceID uuid.UUID, normalizedTitle string) bool {
	if a.auth == nil {
		return true
	}
	if err := a.auth.CheckCreate(r.Context(), actorID, a.wikiID, namespaceID, normalizedTitle); err != nil {
		governanceError(w, r, err)
		return false
	}
	return true
}

// ---- 请求/响应 DTO（与 contracts/openapi/openapi.yaml 对应，契约为准）----

type createPageRequest struct {
	Namespace    string `json:"namespace"`
	Title        string `json:"title"`
	Language     string `json:"language"`
	ContentModel string `json:"content_model"`
}

type renamePageRequest struct {
	Title string `json:"title"`
}

type pageRedirectTargetRequest struct {
	Kind          string     `json:"kind"`
	TargetPageID  *uuid.UUID `json:"target_page_id"`
	Namespace     string     `json:"namespace"`
	TargetTitle   string     `json:"target_title"`
	AnchorBlockID *uuid.UUID `json:"anchor_block_id"`
	ExternalURL   string     `json:"external_url"`
}

type createPageRedirectRequest struct {
	Target pageRedirectTargetRequest `json:"target"`
}

type pageRedirectTargetResponse struct {
	Kind            string     `json:"kind"`
	TargetPageID    *uuid.UUID `json:"target_page_id,omitempty"`
	TargetPageTitle string     `json:"target_page_title,omitempty"`
	NamespaceID     *uuid.UUID `json:"namespace_id,omitempty"`
	Namespace       string     `json:"namespace,omitempty"`
	TargetTitle     string     `json:"target_title,omitempty"`
	AnchorBlockID   *uuid.UUID `json:"anchor_block_id,omitempty"`
	ExternalURL     string     `json:"external_url,omitempty"`
}

type pageRedirectResponse struct {
	SourcePageID uuid.UUID                  `json:"source_page_id"`
	Target       pageRedirectTargetResponse `json:"target"`
	CreatedBy    uuid.UUID                  `json:"created_by"`
	UpdatedBy    uuid.UUID                  `json:"updated_by"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`
}

type upsertBlockRedirectRequest struct {
	TargetPageID  uuid.UUID `json:"target_page_id"`
	TargetBlockID uuid.UUID `json:"target_block_id"`
}

type publishRevisionRequest struct {
	ExpectedRevisionID *uuid.UUID      `json:"expected_revision_id"`
	WorkingDocumentID  *uuid.UUID      `json:"working_document_id"`
	AST                json.RawMessage `json:"ast"`
	Summary            string          `json:"summary"`
	IsMinor            bool            `json:"is_minor"`
}

type pageResponse struct {
	ID                uuid.UUID  `json:"id"`
	WikiID            uuid.UUID  `json:"wiki_id"`
	NamespaceID       uuid.UUID  `json:"namespace_id"`
	NormalizedTitle   string     `json:"normalized_title"`
	DisplayTitle      string     `json:"display_title"`
	Language          string     `json:"language"`
	ContentModel      string     `json:"content_model"`
	Status            string     `json:"status"`
	CurrentRevisionID *uuid.UUID `json:"current_revision_id"`
	CreatedBy         uuid.UUID  `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type revisionResponse struct {
	ID                uuid.UUID  `json:"id"`
	PageID            uuid.UUID  `json:"page_id"`
	ParentRevisionID  *uuid.UUID `json:"parent_revision_id"`
	ContentSnapshotID uuid.UUID  `json:"content_snapshot_id"`
	ActorID           uuid.UUID  `json:"actor_id"`
	Summary           string     `json:"summary"`
	IsMinor           bool       `json:"is_minor"`
	Visibility        string     `json:"visibility"`
	ContentHash       string     `json:"content_hash"`
	SchemaVersion     int        `json:"schema_version"`
	StorageTier       string     `json:"storage_tier"`
	ArchivedAt        *time.Time `json:"archived_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

func toPageResponse(p *page.Page) pageResponse {
	return pageResponse{
		ID:                p.ID,
		WikiID:            p.WikiID,
		NamespaceID:       p.NamespaceID,
		NormalizedTitle:   p.NormalizedTitle,
		DisplayTitle:      p.DisplayTitle,
		Language:          p.Language,
		ContentModel:      p.ContentModel,
		Status:            p.Status,
		CurrentRevisionID: p.CurrentRevisionID,
		CreatedBy:         p.CreatedBy,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

func toRevisionResponse(r *page.Revision) revisionResponse {
	return revisionResponse{
		ID:                r.ID,
		PageID:            r.PageID,
		ParentRevisionID:  r.ParentRevisionID,
		ContentSnapshotID: r.ContentSnapshotID,
		ActorID:           r.ActorID,
		Summary:           r.Summary,
		IsMinor:           r.IsMinor,
		Visibility:        r.Visibility,
		ContentHash:       r.ContentHash,
		SchemaVersion:     r.SchemaVersion,
		StorageTier:       r.StorageTier,
		ArchivedAt:        r.ArchivedAt,
		CreatedAt:         r.CreatedAt,
	}
}

func toPageRedirectResponse(item *page.PageRedirect) pageRedirectResponse {
	return pageRedirectResponse{
		SourcePageID: item.SourcePageID,
		Target: pageRedirectTargetResponse{
			Kind: item.Target.Kind, TargetPageID: item.Target.PageID,
			TargetPageTitle: item.Target.TargetPageTitle,
			NamespaceID:     item.Target.NamespaceID,
			Namespace:       item.Target.NamespaceKey, TargetTitle: item.Target.Title,
			AnchorBlockID: item.Target.AnchorBlockID,
			ExternalURL:   item.Target.TargetInterwiki,
		},
		CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

// ---- handlers ----

// createPage POST /api/v1/pages：在默认站点创建页面（201 Page）。
func (a *WriteAPI) createPage(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var req createPageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Namespace == "" || req.Title == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "namespace 与 title 为必填字段")
		return
	}
	namespaceID, err := a.pages.NamespaceID(r.Context(), a.wikiID, req.Namespace)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	normalizedTitle, err := page.NormalizeTitle(req.Title)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	if !a.authorizeCreate(w, r, actorID, namespaceID, normalizedTitle) {
		return
	}
	p, err := a.pages.CreatePage(r.Context(), page.CreatePageParams{
		WikiID:       a.wikiID,
		NamespaceID:  namespaceID,
		Title:        req.Title,
		Language:     req.Language,
		ContentModel: req.ContentModel,
		ActorID:      actorID,
	})
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toPageResponse(p))
}

// renamePage POST /api/v1/pages/{id}/rename：页面改名（200 Page）。
func (a *WriteAPI) renamePage(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	var req renamePageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Title == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "title 为必填字段")
		return
	}
	if !a.authorize(w, r, actorID, governance.ActionRename, &pageID) {
		return
	}
	p, err := a.pages.RenamePage(r.Context(), pageID, req.Title, actorID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPageResponse(p))
}

// getRedirect GET /api/v1/pages/{id}/redirect：读取当前权威重定向。
func (a *WriteAPI) getRedirect(w http.ResponseWriter, r *http.Request) {
	sourcePageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	if _, err := getPageInWiki(
		r.Context(), a.pages, a.wikiID, sourcePageID,
	); err != nil {
		serviceError(w, r, err)
		return
	}
	item, err := a.pages.GetRedirect(r.Context(), sourcePageID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPageRedirectResponse(item))
}

// createRedirect POST /api/v1/pages/{id}/redirect：创建或更新判别式重定向。
func (a *WriteAPI) createRedirect(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	sourcePageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	var req createPageRedirectRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !a.authorize(w, r, actorID, governance.ActionEdit, &sourcePageID) {
		return
	}
	target := page.RedirectTarget{
		Kind: req.Target.Kind, PageID: req.Target.TargetPageID,
		Title:           req.Target.TargetTitle,
		AnchorBlockID:   req.Target.AnchorBlockID,
		TargetInterwiki: req.Target.ExternalURL,
	}
	if req.Target.Kind == page.RedirectTargetUnresolved {
		if req.Target.Namespace == "" {
			httpx.WriteError(
				w, r, http.StatusBadRequest, httpx.CodeBadRequest,
				"unresolved 目标的 namespace 为必填字段",
			)
			return
		}
		namespaceID, err := a.pages.NamespaceID(
			r.Context(), a.wikiID, req.Target.Namespace,
		)
		if err != nil {
			serviceError(w, r, err)
			return
		}
		target.NamespaceID = &namespaceID
	}
	item, err := a.pages.CreateRedirect(
		r.Context(), sourcePageID, target, actorID,
	)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPageRedirectResponse(item))
}

// deleteRedirect DELETE /api/v1/pages/{id}/redirect：删除当前重定向。
func (a *WriteAPI) deleteRedirect(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	sourcePageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	if !a.authorize(w, r, actorID, governance.ActionEdit, &sourcePageID) {
		return
	}
	if err := a.pages.DeleteRedirect(r.Context(), sourcePageID, actorID); err != nil {
		serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *WriteAPI) listBlockRedirects(w http.ResponseWriter, r *http.Request) {
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	if _, err := getPageInWiki(
		r.Context(), a.pages, a.wikiID, pageID,
	); err != nil {
		serviceError(w, r, err)
		return
	}
	items, err := a.pages.ListBlockRedirects(r.Context(), pageID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *WriteAPI) upsertBlockRedirect(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	blockID, err := uuid.Parse(chi.URLParam(r, "block_id"))
	if err != nil {
		httpx.WriteError(
			w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			"block_id 不是合法 UUID",
		)
		return
	}
	if !a.authorize(w, r, actorID, governance.ActionEdit, &pageID) {
		return
	}
	var req upsertBlockRedirectRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	redirect, err := a.pages.UpsertBlockRedirect(
		r.Context(), pageID, blockID,
		req.TargetPageID, req.TargetBlockID, actorID,
	)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, redirect)
}

func (a *WriteAPI) deleteBlockRedirect(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	blockID, err := uuid.Parse(chi.URLParam(r, "block_id"))
	if err != nil {
		httpx.WriteError(
			w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			"block_id 不是合法 UUID",
		)
		return
	}
	if !a.authorize(w, r, actorID, governance.ActionEdit, &pageID) {
		return
	}
	if err := a.pages.DeleteBlockRedirect(
		r.Context(), pageID, blockID, actorID,
	); err != nil {
		serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// publishRevision POST /api/v1/pages/{id}/revisions：原子发布 Revision（201 Revision）。
// 基线过期返回 409 stale_revision。
func (a *WriteAPI) publishRevision(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	var req publishRevisionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// ast 必须是 JSON 对象（Document 根节点）；领域服务再做完整 Schema 校验。
	if len(req.AST) == 0 || req.AST[0] != '{' {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "ast 为必填字段且必须是 JSON 对象")
		return
	}
	if !a.authorize(w, r, actorID, governance.ActionEdit, &pageID) {
		return
	}
	var rev *page.Revision
	var err error
	if req.WorkingDocumentID != nil {
		if a.collaborationPublisher == nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "协作发布服务未配置")
			return
		}
		rev, err = a.collaborationPublisher.Publish(r.Context(), collaboration.PublishParams{
			DocumentID: *req.WorkingDocumentID, PageID: pageID, ActorID: actorID,
			ExpectedRevisionID: req.ExpectedRevisionID, AST: req.AST,
			Summary: req.Summary, IsMinor: req.IsMinor,
		})
	} else {
		rev, err = a.pages.Publish(r.Context(), page.PublishParams{
			PageID: pageID, ActorID: actorID,
			ExpectedRevisionID: req.ExpectedRevisionID, AST: req.AST,
			Summary: req.Summary, IsMinor: req.IsMinor,
		})
	}
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toRevisionResponse(rev))
}

// ---- 输入解析与错误映射 ----

// actorIDFrom 读取认证中间件写入的 Actor；未认证时写 401。
func actorIDFrom(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	principal, ok := authdomain.PrincipalFrom(r.Context())
	if !ok || principal.ActorID == uuid.Nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "需要登录")
		return uuid.Nil, false
	}
	return principal.ActorID, true
}

// pageIDFrom 解析路径参数 {id}；非法时写 400 并返回 false。
func pageIDFrom(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "路径 id 不是合法 UUID")
		return uuid.Nil, false
	}
	return id, true
}

// getPageInWiki scopes opaque Page IDs to the configured site. Foreign-Wiki
// IDs intentionally have the same error semantics as missing IDs.
func getPageInWiki(
	ctx context.Context,
	pages *page.Service,
	wikiID, pageID uuid.UUID,
) (*page.Page, error) {
	value, err := pages.GetPage(ctx, pageID)
	if err != nil {
		return nil, err
	}
	if value.WikiID != wikiID {
		return nil, fmt.Errorf("%w: id=%s", page.ErrPageNotFound, pageID)
	}
	return value, nil
}

// decodeJSON 解码请求体（拒绝未知字段）；失败时写 400 并返回 false。
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "请求体不是合法 JSON: "+err.Error())
		return false
	}
	return true
}

// serviceError 把领域哨兵错误映射为契约 Error 响应。
func serviceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, page.ErrInvalidTitle), errors.Is(err, page.ErrInvalidAST), errors.Is(err, page.ErrInvalidCursor),
		errors.Is(err, page.ErrInvalidRedirectTarget):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
	case errors.Is(err, page.ErrInvalidActor), errors.Is(err, page.ErrActorNotAllowed):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, err.Error())
	case errors.Is(err, page.ErrPageNotFound), errors.Is(err, page.ErrNamespaceNotFound),
		errors.Is(err, page.ErrRevisionNotFound), errors.Is(err, page.ErrAnchorNotFound),
		errors.Is(err, page.ErrBlockRedirectNotFound), errors.Is(err, page.ErrRedirectNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, collaboration.ErrDocumentNotFound), errors.Is(err, collaboration.ErrDocumentPageMismatch):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, page.ErrStaleRevision):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeStaleRevision, err.Error())
	case errors.Is(err, collaboration.ErrDocumentInactive):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, page.ErrTitleConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, page.ErrRedirectLoop), errors.Is(err, page.ErrRedirectTooDeep),
		errors.Is(err, page.ErrBlockRedirectLoop):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, err.Error())
	case errors.Is(err, page.ErrRedirectTargetDeleted):
		httpx.WriteError(w, r, http.StatusGone, httpx.CodeGone, err.Error())
	case errors.Is(err, page.ErrSnapshotStorageUnavailable):
		httpx.WriteError(
			w, r, http.StatusServiceUnavailable, httpx.CodeInternal,
			"历史版本暂时无法从冷存储读取，请稍后重试",
		)
	case errors.Is(err, page.ErrSnapshotIntegrity):
		httpx.WriteError(
			w, r, http.StatusInternalServerError, httpx.CodeInternal,
			"历史版本完整性校验失败，已拒绝返回内容",
		)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "内部错误")
	}
}
