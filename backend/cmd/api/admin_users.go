package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type addWikiMemberRequest struct {
	Identifier string   `json:"identifier"`
	RoleKeys   []string `json:"role_keys"`
}

func (a *GovernanceAPI) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	items, err := a.userAdmin.List(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		governance.AdminUserListFilter{
			Search:          r.URL.Query().Get("search"),
			IncludeDisabled: r.URL.Query().Get("include_disabled") == "true",
		},
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (a *GovernanceAPI) grantAdminUserRole(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	targetActorID, ok := governancePathID(w, r, "actor_id")
	if !ok {
		return
	}
	roleKey := strings.TrimSpace(chi.URLParam(r, "role_key"))
	result, err := a.userAdmin.GrantRole(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		targetActorID,
		roleKey,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) revokeAdminUserRole(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	targetActorID, ok := governancePathID(w, r, "actor_id")
	if !ok {
		return
	}
	roleKey := strings.TrimSpace(chi.URLParam(r, "role_key"))
	result, err := a.userAdmin.RevokeRole(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		targetActorID,
		roleKey,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) deleteAdminUser(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	targetActorID, ok := governancePathID(w, r, "actor_id")
	if !ok {
		return
	}
	result, err := a.userAdmin.DeleteUser(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		targetActorID,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) listWikiMembers(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	items, err := a.userAdmin.ListWikiMembers(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		governance.AdminUserListFilter{
			Search:          r.URL.Query().Get("search"),
			IncludeDisabled: r.URL.Query().Get("include_disabled") == "true",
		},
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (a *GovernanceAPI) addWikiMember(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var payload addWikiMemberRequest
	if !decodeJSON(w, r, &payload) {
		return
	}
	result, err := a.userAdmin.AddWikiMember(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		payload.Identifier,
		payload.RoleKeys,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *GovernanceAPI) grantWikiMemberRole(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	targetActorID, ok := governancePathID(w, r, "actor_id")
	if !ok {
		return
	}
	roleKey := strings.TrimSpace(chi.URLParam(r, "role_key"))
	result, err := a.userAdmin.GrantWikiMemberRole(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		targetActorID,
		roleKey,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) revokeWikiMemberRole(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	targetActorID, ok := governancePathID(w, r, "actor_id")
	if !ok {
		return
	}
	roleKey := strings.TrimSpace(chi.URLParam(r, "role_key"))
	result, err := a.userAdmin.RevokeWikiMemberRole(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		targetActorID,
		roleKey,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *GovernanceAPI) removeWikiMember(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	targetActorID, ok := governancePathID(w, r, "actor_id")
	if !ok {
		return
	}
	result, err := a.userAdmin.RemoveWikiMember(
		r.Context(),
		wikiIDFromRequest(r),
		actorID,
		targetActorID,
	)
	if err != nil {
		governanceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func isAdminUserNotFound(err error) bool {
	return errors.Is(err, governance.ErrAdminUserNotFound) ||
		errors.Is(err, governance.ErrAdminRoleNotFound)
}

func isWikiMemberNotFound(err error) bool {
	return errors.Is(err, governance.ErrWikiMemberNotFound) ||
		errors.Is(err, governance.ErrAdminRoleNotFound)
}
