package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

// AuthAPI exposes local-account registration, password login and opaque
// server-side sessions.
type AuthAPI struct {
	service             *authdomain.Service
	sessionCookie       string
	secureCookies       bool
	registrationEnabled bool
}

func NewAuthAPI(service *authdomain.Service, sessionCookie string, secureCookies, registrationEnabled bool) *AuthAPI {
	return &AuthAPI{
		service:             service,
		sessionCookie:       sessionCookie,
		secureCookies:       secureCookies,
		registrationEnabled: registrationEnabled,
	}
}

type registerRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

func (a *AuthAPI) register(w http.ResponseWriter, r *http.Request) {
	if !a.registrationEnabled {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "账号注册已关闭")
		return
	}
	var payload registerRequest
	if !decodeAuthJSON(w, r, &payload) {
		return
	}
	session, err := a.service.Register(r.Context(), authdomain.RegisterInput{
		Username:    payload.Username,
		Email:       payload.Email,
		Password:    payload.Password,
		DisplayName: payload.DisplayName,
	})
	var validationErr *authdomain.ValidationError
	switch {
	case errors.As(err, &validationErr):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, validationErr.Message)
		return
	case errors.Is(err, authdomain.ErrAccountExists):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "用户名或邮箱已被使用")
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法创建账号")
		return
	}
	a.setSessionCookie(w, session)
	writeAuthResult(w, http.StatusCreated, session)
}

func (a *AuthAPI) login(w http.ResponseWriter, r *http.Request) {
	var payload loginRequest
	if !decodeAuthJSON(w, r, &payload) {
		return
	}
	session, err := a.service.Login(r.Context(), authdomain.LoginInput{
		Identifier: payload.Identifier,
		Password:   payload.Password,
	})
	if errors.Is(err, authdomain.ErrInvalidCredentials) {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "账号或密码错误")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "无法创建会话")
		return
	}
	a.setSessionCookie(w, session)
	writeAuthResult(w, http.StatusOK, session)
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

func decodeAuthJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "请求体无效")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "请求体无效")
		return false
	}
	return true
}

func (a *AuthAPI) setSessionCookie(w http.ResponseWriter, session authdomain.Session) {
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
}

func writeAuthResult(w http.ResponseWriter, status int, session authdomain.Session) {
	httpx.WriteJSON(w, status, map[string]string{
		"actor_id":     session.ActorID.String(),
		"display_name": session.DisplayName,
	})
}
