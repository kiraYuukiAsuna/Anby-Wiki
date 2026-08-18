package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

type createCLIAuthCodeRequest struct {
	Name         string `json:"name"`
	TokenTTLDays int    `json:"token_ttl_days"`
}

type exchangeCLIAuthCodeRequest struct {
	Code string `json:"code"`
}

type cliAuthCodeResponse struct {
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	ExpiresAt      time.Time `json:"expires_at"`
	TokenExpiresAt time.Time `json:"token_expires_at"`
}

type cliTokenResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Status      string     `json:"status"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type cliAuthExchangeResponse struct {
	Token       string           `json:"token"`
	ActorID     uuid.UUID        `json:"actor_id"`
	DisplayName string           `json:"display_name"`
	TokenInfo   cliTokenResponse `json:"token_info"`
}

func (a *AuthAPI) createCLIAuthCode(w http.ResponseWriter, r *http.Request) {
	principal, ok := browserSessionPrincipal(w, r)
	if !ok {
		return
	}
	var request createCLIAuthCodeRequest
	if !decodeAuthJSON(w, r, &request) {
		return
	}
	result, err := a.service.CreateCLIAuthorizationCode(
		r.Context(), principal.ActorID, request.Name, request.TokenTTLDays,
	)
	switch {
	case errors.Is(err, authdomain.ErrInvalidCLIInput):
		httpx.WriteError(
			w, r, http.StatusBadRequest, httpx.CodeValidationFailed,
			"名称须为 1–120 个字符，Token 有效期须为 1–365 天",
		)
	case errors.Is(err, authdomain.ErrUnauthenticated):
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "需要登录")
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法创建 CLI 授权码")
	default:
		httpx.WriteJSON(w, http.StatusCreated, cliAuthCodeResponse{
			Code: result.Code, Name: result.Name,
			ExpiresAt: result.ExpiresAt, TokenExpiresAt: result.TokenExpiresAt,
		})
	}
}

func (a *AuthAPI) exchangeCLIAuthCode(w http.ResponseWriter, r *http.Request) {
	var request exchangeCLIAuthCodeRequest
	if !decodeAuthJSON(w, r, &request) {
		return
	}
	result, err := a.service.ExchangeCLIAuthorizationCode(r.Context(), request.Code)
	if errors.Is(err, authdomain.ErrInvalidCLIAuthorizationCode) {
		httpx.WriteError(
			w, r, http.StatusUnauthorized, httpx.CodeUnauthorized,
			"CLI 授权码无效、已使用或已过期",
		)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法兑换 CLI 授权码")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cliAuthExchangeResponse{
		Token: result.Token, ActorID: result.ActorID,
		DisplayName: result.DisplayName, TokenInfo: toCLITokenResponse(result.TokenInfo),
	})
}

func (a *AuthAPI) listCLITokens(w http.ResponseWriter, r *http.Request) {
	principal, ok := browserSessionPrincipal(w, r)
	if !ok {
		return
	}
	items, err := a.service.ListCLITokens(r.Context(), principal.ActorID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法读取 CLI Token")
		return
	}
	response := make([]cliTokenResponse, len(items))
	for index := range items {
		response[index] = toCLITokenResponse(items[index])
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": response})
}

func (a *AuthAPI) revokeCLIToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := browserSessionPrincipal(w, r)
	if !ok {
		return
	}
	tokenID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "路径 id 不是合法 UUID")
		return
	}
	err = a.service.RevokeCLIToken(r.Context(), principal.ActorID, tokenID)
	if errors.Is(err, authdomain.ErrCLITokenNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "CLI Token 不存在")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法撤销 CLI Token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AuthAPI) revokeCurrentCLIToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := authdomain.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "需要 CLI Token")
		return
	}
	if principal.Method != "cli_token" || principal.CredentialID == uuid.Nil {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "仅 CLI Token 可调用")
		return
	}
	if err := a.service.RevokeCLIToken(
		r.Context(), principal.ActorID, principal.CredentialID,
	); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法撤销当前 CLI Token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func browserSessionPrincipal(
	w http.ResponseWriter,
	r *http.Request,
) (authdomain.Principal, bool) {
	principal, ok := authdomain.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "需要登录")
		return authdomain.Principal{}, false
	}
	if principal.Method != "session" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "需要浏览器登录会话")
		return authdomain.Principal{}, false
	}
	return principal, true
}

func toCLITokenResponse(value authdomain.CLIToken) cliTokenResponse {
	return cliTokenResponse{
		ID: value.ID, Name: value.Name, TokenPrefix: value.TokenPrefix,
		Status: value.Status, ExpiresAt: value.ExpiresAt,
		LastUsedAt: value.LastUsedAt, RevokedAt: value.RevokedAt,
		CreatedAt: value.CreatedAt,
	}
}
