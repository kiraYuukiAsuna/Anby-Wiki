// 页面阅读 API handlers（M1-T06）：按标题/别名/ID 读取当前 Revision；
// HTML 优先取 RenderedPage 投影（M3-T05），未命中时直连 ContentSnapshot 实时渲染兜底。
// 阅读端点匿名可读，无需 X-Actor-ID；重定向跟随与错误语义见契约 openapi.yaml。
package main

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/render"
)

// ReadAPI 读 API 依赖集合：Page 领域服务与启动时解析缓存的默认站点 ID。
type ReadAPI struct {
	pages  *page.Service
	wikiID uuid.UUID
}

// NewReadAPI 装配读 API。wikiID 与写 API 同为种子里 site_key='default' 的站点。
func NewReadAPI(pages *page.Service, wikiID uuid.UUID) *ReadAPI {
	return &ReadAPI{pages: pages, wikiID: wikiID}
}

// ---- 响应 DTO（与 contracts/openapi/openapi.yaml 对应，契约为准）----

// pageContentResponse 已发布页面的内容：当前 Revision 元信息 + canonical AST + 渲染 HTML。
type pageContentResponse struct {
	Revision        revisionResponse `json:"revision"`
	AST             json.RawMessage  `json:"ast_json"`
	HTML            string           `json:"html"`
	RendererVersion string           `json:"renderer_version"`
}

// redirectResponse 重定向跟随信息：响应返回的是落地页，from_* 回显请求命中的源页面。
type redirectResponse struct {
	FromPageID uuid.UUID `json:"from_page_id"`
	FromTitle  string    `json:"from_title"`
}

// pageWithContentResponse 阅读端点响应（PageWithContent）。
// Content 为 null 表示页面已创建但未发布过；Redirect 仅在跟随了重定向时出现。
type pageWithContentResponse struct {
	Page       pageResponse         `json:"page"`
	Content    *pageContentResponse `json:"content"`
	ViaAlias   bool                 `json:"via_alias"`
	AliasTitle string               `json:"alias_title,omitempty"`
	Redirect   *redirectResponse    `json:"redirect,omitempty"`
}

// ---- handlers ----

// getPageByTitle GET /api/v1/pages/by-title?namespace=&title=：
// 规范化标题解析（活页面优先，其次别名）；命中后跟随站内重定向。
func (a *ReadAPI) getPageByTitle(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	title := r.URL.Query().Get("title")
	if namespace == "" || title == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "namespace 与 title 为必填查询参数")
		return
	}
	resolved, err := a.pages.ResolveTitle(r.Context(), a.wikiID, namespace, title)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	aliasTitle := ""
	if resolved.ViaAlias {
		aliasTitle = title
	}
	a.respondPage(w, r, resolved.Page, resolved.ViaAlias, aliasTitle)
}

// getPageByID GET /api/v1/pages/{id}：按 ID 读取，软删除页返回 410 gone，
// 命中站内重定向源时跟随到落地页。
func (a *ReadAPI) getPageByID(w http.ResponseWriter, r *http.Request) {
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	p, err := a.pages.GetPage(r.Context(), pageID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	if p.DeletedAt != nil {
		httpx.WriteError(w, r, http.StatusGone, httpx.CodeGone, "页面已删除: id="+p.ID.String())
		return
	}
	a.respondPage(w, r, p, false, "")
}

// respondPage 跟随重定向并组装 PageWithContent 响应：
// 未发布过的页面 Content 为 null；渲染失败按内部错误处理
// （ContentSnapshot 入库时已通过 AST 校验，渲染失败即服务端缺陷）。
func (a *ReadAPI) respondPage(w http.ResponseWriter, r *http.Request, from *page.Page, viaAlias bool, aliasTitle string) {
	final, err := a.pages.ResolveRedirect(r.Context(), from.ID, 0)
	if err != nil {
		serviceError(w, r, err)
		return
	}

	resp := pageWithContentResponse{
		Page:       toPageResponse(final),
		ViaAlias:   viaAlias,
		AliasTitle: aliasTitle,
	}
	if final.ID != from.ID {
		resp.Redirect = &redirectResponse{FromPageID: from.ID, FromTitle: from.DisplayTitle}
	}

	rev, snap, err := a.pages.CurrentContent(r.Context(), final.ID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	if rev != nil {
		html, err := a.renderHTML(r, final.ID, rev.ID, snap)
		if err != nil {
			serviceError(w, r, err)
			return
		}
		resp.Content = &pageContentResponse{
			Revision:        toRevisionResponse(rev),
			AST:             snap.AST,
			HTML:            html,
			RendererVersion: render.RendererVersion,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// renderHTML 阅读路径的 HTML 来源决策（M3-T05，设计 §17.1）：
// 先查 RenderedPage 投影（page_id + revision_id + renderer_version 三者匹配才命中）；
// 命中返回投影 HTML；未命中（投影缺失/Revision 落后/渲染器升版）降级为
// 从 ContentSnapshot 实时渲染——投影是缓存语义，权威兜底永远在。
func (a *ReadAPI) renderHTML(r *http.Request, pageID, revisionID uuid.UUID, snap *page.ContentSnapshot) (string, error) {
	if html, ok, err := a.pages.RenderedHTML(r.Context(), pageID, revisionID); err != nil {
		return "", err
	} else if ok {
		return html, nil
	}
	doc, err := ast.Parse(snap.AST)
	if err != nil {
		return "", err
	}
	return render.RenderHTML(doc)
}
