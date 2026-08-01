package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type addEntityLabelRequest struct {
	Language    string `json:"language"`
	Label       string `json:"label"`
	Description string `json:"description"`
	IsPrimary   bool   `json:"is_primary"`
}

type setPrimaryEntityLabelRequest struct {
	Language string `json:"language"`
	Label    string `json:"label"`
}

type addEntityAliasRequest struct {
	Language  string `json:"language"`
	Alias     string `json:"alias"`
	AliasType string `json:"alias_type"`
}

type writePageEntityBindingRequest struct {
	EntityID uuid.UUID `json:"entity_id"`
	Role     string    `json:"role"`
	Language string    `json:"language"`
}

type pageEntityBindingResponse struct {
	PageID    uuid.UUID `json:"page_id"`
	EntityID  uuid.UUID `json:"entity_id"`
	Role      string    `json:"role"`
	Language  string    `json:"language"`
	CreatedAt time.Time `json:"created_at"`
}

type pageEntityBindingListResponse struct {
	Items []pageEntityBindingResponse `json:"items"`
}

type updateClaimVerificationRequest struct {
	VerificationStatus string `json:"verification_status"`
}

func (a *KnowledgeReadAPI) addEntityLabel(
	w http.ResponseWriter,
	r *http.Request,
) {
	entityID, actorID, ok := a.requireEntityMutation(
		w, r, governance.ActionEdit,
	)
	if !ok {
		return
	}
	var request addEntityLabelRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	label, err := a.knowledge.AddLabel(
		r.Context(),
		entityID,
		knowledge.LabelInput{
			Language: request.Language, Label: request.Label,
			Description: request.Description, IsPrimary: request.IsPrimary,
		},
		actorID,
	)
	if err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, entityLabelResponse{
		Language: label.Language, Label: label.Label,
		Description: label.Description, IsPrimary: label.IsPrimary,
	})
}

func (a *KnowledgeReadAPI) removeEntityLabel(
	w http.ResponseWriter,
	r *http.Request,
) {
	entityID, actorID, ok := a.requireEntityMutation(
		w, r, governance.ActionEdit,
	)
	if !ok {
		return
	}
	if err := a.knowledge.RemoveLabel(
		r.Context(),
		entityID,
		r.URL.Query().Get("language"),
		r.URL.Query().Get("label"),
		actorID,
	); err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *KnowledgeReadAPI) setPrimaryEntityLabel(
	w http.ResponseWriter,
	r *http.Request,
) {
	entityID, actorID, ok := a.requireEntityMutation(
		w, r, governance.ActionEdit,
	)
	if !ok {
		return
	}
	var request setPrimaryEntityLabelRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := a.knowledge.SetPrimaryLabel(
		r.Context(),
		entityID,
		request.Language,
		request.Label,
		actorID,
	); err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *KnowledgeReadAPI) addEntityAlias(
	w http.ResponseWriter,
	r *http.Request,
) {
	entityID, actorID, ok := a.requireEntityMutation(
		w, r, governance.ActionEdit,
	)
	if !ok {
		return
	}
	var request addEntityAliasRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	alias, err := a.knowledge.AddAlias(
		r.Context(),
		entityID,
		knowledge.AliasInput{
			Language: request.Language, Alias: request.Alias,
			AliasType: request.AliasType,
		},
		actorID,
	)
	if err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, entityAliasResponse{
		ID: alias.ID, Language: alias.Language,
		Alias: alias.Alias, AliasType: alias.AliasType,
	})
}

func (a *KnowledgeReadAPI) removeEntityAlias(
	w http.ResponseWriter,
	r *http.Request,
) {
	entityID, actorID, ok := a.requireEntityMutation(
		w, r, governance.ActionEdit,
	)
	if !ok {
		return
	}
	aliasID, ok := federationPathID(w, r, "alias_id")
	if !ok {
		return
	}
	if err := a.knowledge.RemoveAlias(
		r.Context(), entityID, aliasID, actorID,
	); err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *KnowledgeReadAPI) listPageEntityBindings(
	w http.ResponseWriter,
	r *http.Request,
) {
	pageID, ok := pageIDFrom(w, r)
	if !ok {
		return
	}
	items, err := a.knowledge.ListPageBindings(
		r.Context(), a.wikiID, pageID,
	)
	if err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	response := pageEntityBindingListResponse{
		Items: make([]pageEntityBindingResponse, len(items)),
	}
	for i := range items {
		response.Items[i] = toPageEntityBindingResponse(items[i])
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (a *KnowledgeReadAPI) writePageEntityBinding(
	w http.ResponseWriter,
	r *http.Request,
) {
	pageID, actorID, ok := a.requireKnowledgeMutation(
		w, r, governance.ActionEdit, true,
	)
	if !ok {
		return
	}
	var request writePageEntityBindingRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	binding, err := a.knowledge.BindPage(
		r.Context(),
		knowledge.BindPageParams{
			WikiID: a.wikiID, PageID: pageID, EntityID: request.EntityID,
			Role: request.Role, Language: request.Language, ActorID: actorID,
		},
	)
	if err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	httpx.WriteJSON(
		w, http.StatusOK, toPageEntityBindingResponse(*binding),
	)
}

func (a *KnowledgeReadAPI) removePageEntityBinding(
	w http.ResponseWriter,
	r *http.Request,
) {
	pageID, actorID, ok := a.requireKnowledgeMutation(
		w, r, governance.ActionEdit, true,
	)
	if !ok {
		return
	}
	entityID, ok := federationPathID(w, r, "entity_id")
	if !ok {
		return
	}
	if err := a.knowledge.UnbindPage(
		r.Context(),
		a.wikiID,
		pageID,
		entityID,
		r.URL.Query().Get("role"),
		actorID,
	); err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *KnowledgeReadAPI) updateClaimVerification(
	w http.ResponseWriter,
	r *http.Request,
) {
	claimID, actorID, ok := a.requireKnowledgeMutation(
		w, r, governance.ActionReview, false,
	)
	if !ok {
		return
	}
	claim, err := a.knowledge.GetClaim(r.Context(), claimID)
	if err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	if _, err := a.currentWikiEntity(
		r.Context(), claim.SubjectEntityID,
	); err != nil {
		httpx.WriteError(
			w, r, http.StatusNotFound, httpx.CodeNotFound, "Claim 不存在",
		)
		return
	}
	var request updateClaimVerificationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if _, err := a.knowledge.UpdateVerificationStatus(
		r.Context(),
		knowledge.UpdateVerificationStatusParams{
			ClaimID: claimID, Status: request.VerificationStatus,
			ActorID: actorID,
		},
	); err != nil {
		knowledgeMutationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *KnowledgeReadAPI) requireEntityMutation(
	w http.ResponseWriter,
	r *http.Request,
	action string,
) (uuid.UUID, uuid.UUID, bool) {
	entityID, actorID, ok := a.requireKnowledgeMutation(
		w, r, action, false,
	)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	_, err := a.currentWikiEntity(r.Context(), entityID)
	if err != nil {
		knowledgeMutationError(w, r, err)
		return uuid.Nil, uuid.Nil, false
	}
	return entityID, actorID, true
}

func (a *KnowledgeReadAPI) requireKnowledgeMutation(
	w http.ResponseWriter,
	r *http.Request,
	action string,
	pageScoped bool,
) (uuid.UUID, uuid.UUID, bool) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	targetID, ok := pageIDFrom(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	if a.authorization == nil {
		a.writeError(w, r, governance.ErrPermissionDenied)
		return uuid.Nil, uuid.Nil, false
	}
	var pageID *uuid.UUID
	if pageScoped {
		pageID = &targetID
	}
	if err := a.authorization.Check(
		r.Context(), actorID, a.wikiID, action, pageID,
	); err != nil {
		a.writeError(w, r, err)
		return uuid.Nil, uuid.Nil, false
	}
	return targetID, actorID, true
}

func toPageEntityBindingResponse(
	binding knowledge.PageEntityBinding,
) pageEntityBindingResponse {
	return pageEntityBindingResponse{
		PageID: binding.PageID, EntityID: binding.EntityID,
		Role: binding.Role, Language: binding.Language,
		CreatedAt: binding.CreatedAt,
	}
}

func knowledgeMutationError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, knowledge.ErrInvalidLabel),
		errors.Is(err, knowledge.ErrInvalidBindingRole),
		errors.Is(err, knowledge.ErrBindingWikiMismatch),
		errors.Is(err, knowledge.ErrInvalidVerificationStatus):
		httpx.WriteError(
			w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error(),
		)
	case errors.Is(err, governance.ErrPermissionDenied),
		errors.Is(err, page.ErrInvalidActor),
		errors.Is(err, page.ErrActorNotAllowed),
		errors.Is(err, knowledge.ErrVerificationForbidden):
		httpx.WriteError(
			w, r, http.StatusForbidden, httpx.CodeForbidden, err.Error(),
		)
	case errors.Is(err, page.ErrPageNotFound),
		errors.Is(err, knowledge.ErrEntityNotFound),
		errors.Is(err, knowledge.ErrClaimNotFound),
		errors.Is(err, knowledge.ErrLabelNotFound),
		errors.Is(err, knowledge.ErrAliasNotFound),
		errors.Is(err, knowledge.ErrBindingNotFound):
		httpx.WriteError(
			w, r, http.StatusNotFound, httpx.CodeNotFound, err.Error(),
		)
	case errors.Is(err, knowledge.ErrDuplicateEntityKey),
		errors.Is(err, knowledge.ErrDuplicatePrimaryLabel),
		errors.Is(err, knowledge.ErrNoPrimaryLabel),
		errors.Is(err, knowledge.ErrLabelExists),
		errors.Is(err, knowledge.ErrDuplicateAlias),
		errors.Is(err, knowledge.ErrBindingExists),
		errors.Is(err, knowledge.ErrEntityMerged):
		httpx.WriteError(
			w, r, http.StatusConflict, httpx.CodeConflict, err.Error(),
		)
	default:
		httpx.WriteError(
			w, r, http.StatusInternalServerError, httpx.CodeInternal, "内部错误",
		)
	}
}
