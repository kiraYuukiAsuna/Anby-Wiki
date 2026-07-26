package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

// AuthAPI exposes the versioned session contract.
//
// EARLY STAGE: the only login path is a shared bootstrap token exchanged for a
// server-side session. There is no identity provider and therefore no real
// authentication of *who* the caller is: anyone holding the token can act as
// the named actor. This is acceptable only for a closed early-stage
// deployment and must be replaced with a real identity provider before the
// wiki is publicly reachable. Tracked in Docs/OutstandingIssues.md.
type AuthAPI struct {
	service           *authdomain.Service
	sessionCookie     string
	secureCookies     bool
	devLoginEnabled   bool
	devLoginTokenHash [sha256.Size]byte
	hasDevLoginToken  bool
}

func NewAuthAPI(service *authdomain.Service, sessionCookie string, secureCookies bool, devLoginEnabled bool, devLoginToken string) *AuthAPI {
	api := &AuthAPI{
		service:         service,
		sessionCookie:   sessionCookie,
		secureCookies:   secureCookies,
		devLoginEnabled: devLoginEnabled,
	}
	if token := strings.TrimSpace(devLoginToken); token != "" {
		api.devLoginTokenHash = sha256.Sum256([]byte(token))
		api.hasDevLoginToken = true
	}
	return api
}

// devLoginRequest is the bootstrap login payload.
type devLoginRequest struct {
	// Token is the shared bootstrap secret.
	Token string `json:"token"`
	// DisplayName names the actor created on first use.
	DisplayName string `json:"display_name"`
}

// devLogin exchanges the bootstrap token for a session cookie.
func (a *AuthAPI) devLogin(w http.ResponseWriter, r *http.Request) {
	if !a.devLoginEnabled || !a.hasDevLoginToken {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "登录方式未启用")
		return
	}
	var payload devLoginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&payload); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "请求体无效")
		return
	}
	supplied := sha256.Sum256([]byte(strings.TrimSpace(payload.Token)))
	// Constant-time compare so the token cannot be recovered by timing.
	if subtle.ConstantTimeCompare(supplied[:], a.devLoginTokenHash[:]) != 1 {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "令牌无效")
		return
	}
	displayName := strings.TrimSpace(payload.DisplayName)
	if displayName == "" {
		displayName = "Bootstrap user"
	}
	if utf8.RuneCountInString(displayName) > 128 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed,
			"显示名不得超过 128 个字符")
		return
	}
	// A stable synthetic subject keeps repeat logins mapped to one Actor.
	identity := authdomain.Identity{
		Issuer:      authdomain.BootstrapIssuer,
		Subject:     "bootstrap-user",
		DisplayName: displayName,
	}
	session, err := a.service.EstablishSession(r.Context(), identity)
	if errors.Is(err, authdomain.ErrUnauthenticated) {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "Actor 已停用")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法创建会话")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.sessionCookie,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"actor_id":     session.ActorID.String(),
		"display_name": session.DisplayName,
	})
}

func (a *AuthAPI) session(w http.ResponseWriter, r *http.Request) {
	principal, ok := authdomain.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "需要登录")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"actor_id":     principal.ActorID.String(),
		"actor_type":   principal.ActorType,
		"display_name": principal.DisplayName,
		"method":       principal.Method,
	})
}

func (a *AuthAPI) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(a.sessionCookie); err == nil {
		if err := a.service.RevokeSession(r.Context(), cookie.Value); err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法退出登录")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.sessionCookie,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}
