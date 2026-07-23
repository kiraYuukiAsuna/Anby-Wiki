// 投影查询 API handlers（M3-T03）：反向链接与文档目录，匿名可读。
// 数据来自 page_link_projection / document_outline_projection / page_anchor
// 投影表（设计 §17.3：关系查询走投影表，不扫 AST），投影由 Worker 异步构建，
// 新发布内容与投影之间存在最终一致窗口。
package main

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/projection"
)

// ProjectionAPI 投影查询 API 依赖集合：Page 领域服务（存在性/软删除判定）
// 与投影只读查询服务。
type ProjectionAPI struct {
	pages   *page.Service
	queries *projection.Queries
}

// NewProjectionAPI 装配投影查询 API。
func NewProjectionAPI(pages *page.Service, queries *projection.Queries) *ProjectionAPI {
	return &ProjectionAPI{pages: pages, queries: queries}
}

// ---- 响应 DTO（与 contracts/openapi/openapi.yaml 对应，契约为准）----

// backlinkResponse 一条反向链接：来源页 + 所在 Block + 展示文本。
type backlinkResponse struct {
	SourcePageID  uuid.UUID `json:"source_page_id"`
	SourceTitle   string    `json:"source_title"`
	SourceBlockID uuid.UUID `json:"source_block_id"`
	DisplayText   string    `json:"display_text"`
}

// backlinkListResponse 反链列表（CursorPage 信封：items + next_cursor）。
type backlinkListResponse struct {
	Items      []backlinkResponse `json:"items"`
	NextCursor *string            `json:"next_cursor"`
}

// outlineItemResponse 目录条目（slug 供阅读页锚点跳转，position_key 为序号路径）。
type outlineItemResponse struct {
	HeadingBlockID       uuid.UUID  `json:"heading_block_id"`
	ParentHeadingBlockID *uuid.UUID `json:"parent_heading_block_id"`
	Level                int        `json:"level"`
	Title                string     `json:"title"`
	Slug                 string     `json:"slug"`
	PositionKey          string     `json:"position_key"`
}

// outlineResponse 文档目录（items 按文档顺序；未发布过为空数组）。
type outlineResponse struct {
	Items []outlineItemResponse `json:"items"`
}

type anchorTargetResponse struct {
	PageID      uuid.UUID `json:"page_id"`
	BlockID     uuid.UUID `json:"block_id"`
	CurrentSlug string    `json:"current_slug"`
	ViaAlias    bool      `json:"via_alias"`
	ViaRedirect bool      `json:"via_redirect"`
}

// referenceUsageResponse 知识/证据引用来源位置；Revision 用于审计与定位。
type referenceUsageResponse struct {
	PageID      uuid.UUID  `json:"page_id"`
	PageTitle   string     `json:"page_title"`
	RevisionID  uuid.UUID  `json:"revision_id"`
	BlockID     uuid.UUID  `json:"block_id"`
	NodeID      string     `json:"node_id"`
	MentionText *string    `json:"mention_text"`
	ClaimID     *uuid.UUID `json:"claim_id"`
}

type referenceUsageListResponse struct {
	Items      []referenceUsageResponse `json:"items"`
	NextCursor *string                  `json:"next_cursor"`
}

// ---- handlers ----

// listBacklinks GET /api/v1/pages/{id}/backlinks?cursor=&page_size=：
// 指向该页的已解析引用来源，游标分页（匿名可读）。软删除页返回 410 gone。
func (a *ProjectionAPI) listBacklinks(w http.ResponseWriter, r *http.Request) {
	pageID, ok := a.livePageIDFrom(w, r)
	if !ok {
		return
	}
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := a.queries.Backlinks(r.Context(), pageID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	resp := backlinkListResponse{
		Items:      make([]backlinkResponse, len(result.Items)),
		NextCursor: result.NextCursor,
	}
	for i, b := range result.Items {
		resp.Items[i] = backlinkResponse{
			SourcePageID:  b.SourcePageID,
			SourceTitle:   b.SourceTitle,
			SourceBlockID: b.SourceBlockID,
			DisplayText:   b.DisplayText,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// getOutline GET /api/v1/pages/{id}/outline：文档目录（匿名可读），
// 含层级、纯文本标题、锚点 slug 与 position_key，供阅读页 TOC。软删除页返回 410 gone。
func (a *ProjectionAPI) getOutline(w http.ResponseWriter, r *http.Request) {
	pageID, ok := a.livePageIDFrom(w, r)
	if !ok {
		return
	}
	items, err := a.queries.Outline(r.Context(), pageID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	resp := outlineResponse{Items: make([]outlineItemResponse, len(items))}
	for i, it := range items {
		resp.Items[i] = outlineItemResponse{
			HeadingBlockID:       it.HeadingBlockID,
			ParentHeadingBlockID: it.ParentHeadingBlockID,
			Level:                it.Level,
			Title:                it.Title,
			Slug:                 it.Slug,
			PositionKey:          it.PositionKey,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// resolveAnchor GET /api/v1/pages/{id}/anchors/{slug} resolves a current or
// historical slug to the live stable Block ID, following explicit migrations.
func (a *ProjectionAPI) resolveAnchor(w http.ResponseWriter, r *http.Request) {
	pageID, ok := a.livePageIDFrom(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "slug 不能为空")
		return
	}
	target, err := a.pages.ResolveAnchor(r.Context(), pageID, slug)
	if errors.Is(err, page.ErrAnchorNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
		return
	}
	if errors.Is(err, page.ErrBlockRedirectLoop) {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidationFailed, err.Error())
		return
	}
	if err != nil {
		serviceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, anchorTargetResponse{
		PageID: target.PageID, BlockID: target.BlockID, CurrentSlug: target.Slug,
		ViaAlias: target.ViaAlias, ViaRedirect: target.ViaRedirect,
	})
}

// listEntityMentions GET /api/v1/entities/{id}/mentions。
func (a *ProjectionAPI) listEntityMentions(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	a.listReferenceUsages(w, r, func(limit int) (*projection.ReferenceUsagePage, error) {
		return a.queries.EntityMentions(r.Context(), id, r.URL.Query().Get("cursor"), limit)
	})
}

// listClaimUsages GET /api/v1/claims/{id}/usages。
func (a *ProjectionAPI) listClaimUsages(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	a.listReferenceUsages(w, r, func(limit int) (*projection.ReferenceUsagePage, error) {
		return a.queries.ClaimUsages(r.Context(), id, r.URL.Query().Get("cursor"), limit)
	})
}

// listCitationUsages GET /api/v1/citations/{id}/usages。
func (a *ProjectionAPI) listCitationUsages(w http.ResponseWriter, r *http.Request) {
	id, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	a.listReferenceUsages(w, r, func(limit int) (*projection.ReferenceUsagePage, error) {
		return a.queries.CitationUsages(r.Context(), id, r.URL.Query().Get("cursor"), limit)
	})
}

func (a *ProjectionAPI) listReferenceUsages(
	w http.ResponseWriter,
	r *http.Request,
	query func(limit int) (*projection.ReferenceUsagePage, error),
) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return
	}
	result, err := query(limit)
	if errors.Is(err, projection.ErrReferenceTargetNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error())
		return
	}
	if err != nil {
		serviceError(w, r, err)
		return
	}
	resp := referenceUsageListResponse{
		Items:      make([]referenceUsageResponse, len(result.Items)),
		NextCursor: result.NextCursor,
	}
	for i, usage := range result.Items {
		var mentionText *string
		if usage.MentionText != "" {
			text := usage.MentionText
			mentionText = &text
		}
		resp.Items[i] = referenceUsageResponse{
			PageID: usage.PageID, PageTitle: usage.PageTitle,
			RevisionID: usage.RevisionID, BlockID: usage.BlockID, NodeID: usage.NodeID,
			MentionText: mentionText, ClaimID: usage.ClaimID,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// livePageIDFrom 解析路径 {id} 并做存在性/软删除判定：
// 非法 UUID 400、不存在 404、已软删除 410（与 getPageByID 语义一致）。
func (a *ProjectionAPI) livePageIDFrom(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return uuid.Nil, false
	}
	p, err := a.pages.GetPage(r.Context(), pageID)
	if err != nil {
		serviceError(w, r, err)
		return uuid.Nil, false
	}
	if p.DeletedAt != nil {
		httpx.WriteError(w, r, http.StatusGone, httpx.CodeGone, "页面已删除: id="+p.ID.String())
		return uuid.Nil, false
	}
	return pageID, true
}
