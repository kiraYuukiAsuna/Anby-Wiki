package main

import (
	"errors"
	"net/http"
	"time"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

// AuthAPI exposes the versioned browser OIDC and session contract.
type AuthAPI struct {
	service           *authdomain.Service
	provider          authdomain.OIDCProvider
	sessionCookie     string
	loginCookie       string
	secureCookies     bool
	postLoginRedirect string
}

func NewAuthAPI(service *authdomain.Service, provider authdomain.OIDCProvider, sessionCookie string, secureCookies bool, postLoginRedirect string) *AuthAPI {
	return &AuthAPI{
		service:           service,
		provider:          provider,
		sessionCookie:     sessionCookie,
		loginCookie:       sessionCookie + "_oidc",
		secureCookies:     secureCookies,
		postLoginRedirect: postLoginRedirect,
	}
}

func (a *AuthAPI) login(w http.ResponseWriter, r *http.Request) {
	if a.provider == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeInternal, "登录服务未配置")
		return
	}
	attempt, err := a.service.BeginLogin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法开始登录")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.loginCookie,
		Value:    attempt.BrowserSecret,
		Path:     "/api/v1/auth/callback",
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  attempt.ExpiresAt,
		MaxAge:   int(time.Until(attempt.ExpiresAt).Seconds()),
	})
	http.Redirect(w, r, a.provider.AuthorizationURL(attempt.State, attempt.Nonce, attempt.CodeChallenge), http.StatusFound)
}

func (a *AuthAPI) callback(w http.ResponseWriter, r *http.Request) {
	if a.provider == nil {
		a.clearLoginCookie(w)
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeInternal, "登录服务未配置")
		return
	}
	if r.URL.Query().Get("error") != "" {
		a.clearLoginCookie(w)
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "身份提供方拒绝登录")
		return
	}
	loginCookie, err := r.Cookie(a.loginCookie)
	if err != nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "登录事务无效或已过期")
		return
	}
	attempt, err := a.service.ConsumeLogin(r.Context(), r.URL.Query().Get("state"), loginCookie.Value)
	a.clearLoginCookie(w)
	if errors.Is(err, authdomain.ErrInvalidLogin) {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "登录事务无效或已过期")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法完成登录")
		return
	}
	identity, err := a.provider.ExchangeAndVerify(
		r.Context(), r.URL.Query().Get("code"), attempt.CodeVerifier, attempt.Nonce,
	)
	if err != nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "身份校验失败")
		return
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
	http.Redirect(w, r, a.postLoginRedirect, http.StatusFound)
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

func (a *AuthAPI) clearLoginCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.loginCookie,
		Path:     "/api/v1/auth/callback",
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
