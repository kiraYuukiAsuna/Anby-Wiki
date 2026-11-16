package main

import (
	"errors"
	"net/http"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/google/uuid"
)

type languageVariant struct {
	PageID   uuid.UUID `json:"page_id"`
	Language string    `json:"language"`
	Title    string    `json:"title"`
}
type languageVariants struct {
	Items  []languageVariant `json:"items"`
	Status string            `json:"status"`
}

func (a *ReadAPI) listPageTranslations(w http.ResponseWriter, r *http.Request) {
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	p, err := getPageInWiki(r.Context(), a.pages, wikiIDFromRequest(r), pageID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	if !a.authorizeReadPage(w, r, p.ID) {
		return
	}
	result := languageVariants{Items: []languageVariant{{p.ID, p.Language, p.DisplayTitle}}, Status: "none"}
	t, err := a.pages.Translation(r.Context(), pageID)
	if err != nil {
		serviceError(w, r, err)
		return
	}
	if t != nil {
		result.Status = t.Status
		other := t.SourcePageID
		if other == pageID {
			if t.TargetPageID == nil {
				httpx.WriteJSON(w, http.StatusOK, result)
				return
			}
			other = *t.TargetPageID
		}
		target, err := getPageInWiki(r.Context(), a.pages, p.WikiID, other)
		if err != nil {
			serviceError(w, r, err)
			return
		}
		actorID, ok := actorIDFrom(w, r)
		if !ok {
			return
		}
		if target.DeletedAt == nil && (a.auth == nil || a.auth.CheckReadPage(r.Context(), actorID, p.WikiID, target.ID) == nil) {
			result.Items = append(result.Items, languageVariant{target.ID, target.Language, target.DisplayTitle})
		}
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *WriteAPI) requestPageTranslation(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	if !a.authorize(w, r, actorID, governance.ActionEdit, &pageID) || !a.authorize(w, r, actorID, governance.ActionCreate, nil) {
		return
	}
	_, err := a.pages.RequestTranslation(r.Context(), pageID, actorID)
	if errors.Is(err, page.ErrInvalidTranslation) {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	if err != nil {
		serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
