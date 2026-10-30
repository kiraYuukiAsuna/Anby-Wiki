package main

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type createMachineActorRequest struct {
	ActorType   string `json:"actor_type"`
	DisplayName string `json:"display_name"`
}

type setMachineActorStatusRequest struct {
	Status string `json:"status"`
}

func (a *AuthAPI) createMachineActor(w http.ResponseWriter, r *http.Request) {
	principal, ok := browserSessionPrincipal(w, r)
	if !ok {
		return
	}
	var request createMachineActorRequest
	if !decodeAuthJSON(w, r, &request) {
		return
	}
	result, err := a.service.CreateMachineActor(r.Context(), authdomain.CreateMachineActorParams{
		WikiID: principal.WikiID, CreatedBy: principal.ActorID,
		ActorType: request.ActorType, DisplayName: request.DisplayName,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		machineActorError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *AuthAPI) listMachineActors(w http.ResponseWriter, r *http.Request) {
	principal, ok := browserSessionPrincipal(w, r)
	if !ok {
		return
	}
	items, err := a.service.ListMachineActors(r.Context(), principal.ActorID, principal.WikiID)
	if err != nil {
		machineActorError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *AuthAPI) listCLIMachineActors(w http.ResponseWriter, r *http.Request) {
	principal, ok := browserSessionPrincipal(w, r)
	if !ok {
		return
	}
	items, err := a.service.ListCLIMachineActors(r.Context(), principal.ActorID)
	if err != nil {
		machineActorError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *AuthAPI) setMachineActorStatus(w http.ResponseWriter, r *http.Request) {
	principal, ok := browserSessionPrincipal(w, r)
	if !ok {
		return
	}
	actorID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "路径 id 不是合法 UUID")
		return
	}
	var request setMachineActorStatusRequest
	if !decodeAuthJSON(w, r, &request) {
		return
	}
	result, err := a.service.SetMachineActorStatus(r.Context(), authdomain.SetMachineActorStatusParams{
		WikiID: principal.WikiID, ActorID: actorID, UpdatedBy: principal.ActorID, Status: request.Status,
	})
	if err != nil {
		machineActorError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func machineActorError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, authdomain.ErrInvalidMachineActor):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "名称须为 1–120 个字符，类型须为 ai 或 import，并提供 Idempotency-Key")
	case errors.Is(err, authdomain.ErrMachineActorForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "需要活跃的真人账号与当前 Wiki 管理员权限")
	case errors.Is(err, authdomain.ErrMachineActorNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "机器 Actor 不存在")
	case errors.Is(err, authdomain.ErrMachineActorConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "同一创建键已用于另一份 Actor 配置")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法管理机器 Actor")
	}
}
