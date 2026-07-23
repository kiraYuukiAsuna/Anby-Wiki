// 页面写 API handlers（M1-T05）：创建页面、改名、发布 Revision。
// handler 层保持薄：解析/校验输入 → 调领域服务 → 按契约 Error 模型映射错误。
// Actor 身份只从认证中间件写入的 request context 读取。
package main

import (
	"encoding/json"
	"errors"
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
// 测试和迁移期工具可不注入，保持 Page 领域服务本身的 Actor 类型边界。
func (a *WriteAPI) WithAuthorization(auth *governance.AuthorizationService) *WriteAPI {
	a.auth = auth
	return a
}

func (a *WriteAPI) WithCollaborationPublisher(publisher *collaboration.Publisher) *WriteAPI {
	a.collaborationPublisher = publisher
	return a
}

func (a *WriteAPI) authorize(w http.ResponseWriter, r *http.Request, actorID uuid.UUID, action string, pageID *uuid.UUID) bool {
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
		CreatedAt:         r.CreatedAt,
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
	case errors.Is(err, page.ErrInvalidTitle), errors.Is(err, page.ErrInvalidAST), errors.Is(err, page.ErrInvalidCursor):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
	case errors.Is(err, page.ErrInvalidActor), errors.Is(err, page.ErrActorNotAllowed):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, err.Error())
	case errors.Is(err, page.ErrPageNotFound), errors.Is(err, page.ErrNamespaceNotFound), errors.Is(err, page.ErrRevisionNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, collaboration.ErrDocumentNotFound), errors.Is(err, collaboration.ErrDocumentPageMismatch):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
	case errors.Is(err, page.ErrStaleRevision):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeStaleRevision, err.Error())
	case errors.Is(err, collaboration.ErrDocumentInactive):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, page.ErrTitleConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, page.ErrRedirectLoop), errors.Is(err, page.ErrRedirectTooDeep):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, err.Error())
	case errors.Is(err, page.ErrRedirectTargetDeleted):
		httpx.WriteError(w, r, http.StatusGone, httpx.CodeGone, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "内部错误")
	}
}
