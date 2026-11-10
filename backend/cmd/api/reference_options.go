package main

import (
	"net/http"
	"strings"

	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/google/uuid"
)

func referenceSearchParams(w http.ResponseWriter, r *http.Request) (string, uuid.UUID, int, bool) {
	limit, ok := pageSizeFrom(w, r)
	if !ok {
		return "", uuid.Nil, 0, false
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	after := uuid.Nil
	var err error
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		after, err = uuid.Parse(cursor)
	}
	if err != nil || len([]rune(query)) > 255 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "搜索参数无效")
		return "", uuid.Nil, 0, false
	}
	return query, after, limit, true
}

func (a *KnowledgeReadAPI) listClaimOptions(w http.ResponseWriter, r *http.Request) {
	query, after, limit, ok := referenceSearchParams(w, r)
	if !ok {
		return
	}
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if a.authorization == nil {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "无权读取目录")
		return
	}
	allowed, err := a.authorization.HasWikiAccess(r.Context(), actorID, wikiIDFromRequest(r))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !allowed {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "无权读取目录")
		return
	}
	result, err := a.knowledge.ListClaimOptions(r.Context(), wikiIDFromRequest(r), query, after, limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *KnowledgeReadAPI) listCitationOptions(w http.ResponseWriter, r *http.Request) {
	query, after, limit, ok := referenceSearchParams(w, r)
	if !ok {
		return
	}
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if a.authorization == nil {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "无权读取目录")
		return
	}
	allowed, err := a.authorization.HasWikiAccess(r.Context(), actorID, wikiIDFromRequest(r))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !allowed {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "无权读取目录")
		return
	}
	result, err := a.evidence.ListCitationOptions(r.Context(), wikiIDFromRequest(r), query, after, limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) listMemberOptions(w http.ResponseWriter, r *http.Request) {
	query, after, limit, ok := referenceSearchParams(w, r)
	if !ok {
		return
	}
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if a.auth == nil {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "无权读取目录")
		return
	}
	allowed, err := a.auth.HasWikiAccess(r.Context(), actorID, wikiIDFromRequest(r))
	if err != nil {
		governanceError(w, r, err)
		return
	}
	if !allowed {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "无权读取目录")
		return
	}
	result, err := a.userAdmin.ListMemberOptions(r.Context(), wikiIDFromRequest(r), query, after, limit)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.WriteJSON(w, http.StatusOK, result)
}
