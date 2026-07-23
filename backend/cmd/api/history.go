// 页面历史 API handlers（M1-T07）：Revision 列表/详情、两版结构 Diff、回滚。
// 读端点匿名可读；回滚是写操作，需认证 Actor（ai 不允许直接写）。
package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

// HistoryAPI 历史 API 依赖集合：Page 领域服务。
type HistoryAPI struct {
	pages *page.Service
}

// NewHistoryAPI 装配历史 API。
func NewHistoryAPI(pages *page.Service) *HistoryAPI {
	return &HistoryAPI{pages: pages}
}

// ---- 请求/响应 DTO（与 contracts/openapi/openapi.yaml 对应，契约为准）----

// revisionListResponse Revision 历史列表（CursorPage 信封：items + next_cursor）。
type revisionListResponse struct {
	Items      []revisionResponse `json:"items"`
	NextCursor *string            `json:"next_cursor"`
}

// revisionDetailResponse 单版详情：Revision 元信息 + canonical AST（不含 html）。
type revisionDetailResponse struct {
	Revision revisionResponse `json:"revision"`
	AST      json.RawMessage  `json:"ast_json"`
}

type rollbackRequest struct {
	TargetRevisionID uuid.UUID `json:"target_revision_id"`
	Summary          string    `json:"summary"`
}

// rollbackResponse 回滚产生的新 Revision + rolled_back_to。
type rollbackResponse struct {
	revisionResponse
	RolledBackTo uuid.UUID `json:"rolled_back_to"`
}

// ---- handlers ----

// listRevisions GET /api/v1/pages/{id}/revisions?cursor=&page_size=：
// 按 created_at DESC, id DESC 游标分页（匿名可读）。
func (a *HistoryAPI) listRevisions(w http.ResponseWriter, r *http.Request) {
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.pages.ListRevisions(r.Context(), pageID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	resp := revisionListResponse{Items: make([]revisionResponse, len(result.Items)), NextCursor: result.NextCursor}
	for i := range result.Items {
		resp.Items[i] = toRevisionResponse(&result.Items[i])
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// getRevision GET /api/v1/pages/{id}/revisions/{rid}：单版详情（含 ast_json，匿名可读）。
func (a *HistoryAPI) getRevision(w http.ResponseWriter, r *http.Request) {
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	revisionID, err := uuid.Parse(chi.URLParam(r, "rid"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "路径 rid 不是合法 UUID")
		return
	}
	rev, snap, err := a.pages.GetRevision(r.Context(), pageID, revisionID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, revisionDetailResponse{
		Revision: toRevisionResponse(rev),
		AST:      snap.AST,
	})
}

// diffRevisions GET /api/v1/pages/{id}/diff?from=&to=：两版结构 Diff（匿名可读）。
// 响应直接序列化 ast.DocumentDiff（changes: added/removed/changed/moved）。
func (a *HistoryAPI) diffRevisions(w http.ResponseWriter, r *http.Request) {
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	fromID, err := uuid.Parse(q.Get("from"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "from 为必填查询参数且必须是合法 UUID")
		return
	}
	toID, err := uuid.Parse(q.Get("to"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "to 为必填查询参数且必须是合法 UUID")
		return
	}
	diff, err := a.pages.DiffRevisions(r.Context(), pageID, fromID, toID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, diff)
}

// rollback POST /api/v1/pages/{id}/rollback：以目标旧版内容发布新 Revision（201）。
// 旧 Revision 与旧快照不动；审计事件为 revision.rolled_back（payload 含 rolled_back_to）。
func (a *HistoryAPI) rollback(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	var req rollbackRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TargetRevisionID == uuid.Nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "target_revision_id 为必填字段")
		return
	}
	rev, err := a.pages.Rollback(r.Context(), page.RollbackParams{
		PageID:           pageID,
		TargetRevisionID: req.TargetRevisionID,
		ActorID:          actorID,
		Summary:          req.Summary,
	})
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, rollbackResponse{
		revisionResponse: toRevisionResponse(rev),
		RolledBackTo:     req.TargetRevisionID,
	})
}

// pageSizeFrom 解析 page_size 查询参数：缺省默认 20；非整数或越界（1..100）写 400 并返回 false。
func pageSizeFrom(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("page_size")
	if raw == "" {
		return page.DefaultHistoryPageSize, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > page.MaxHistoryPageSize {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest,
			"page_size 必须是 1.."+strconv.Itoa(page.MaxHistoryPageSize)+" 的整数")
		return 0, false
	}
	return n, true
}
