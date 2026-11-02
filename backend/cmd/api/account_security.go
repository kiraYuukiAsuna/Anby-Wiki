package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/google/uuid"
)

type securityTokenRequest struct {
	Token    string `json:"token"`
	Purpose  string `json:"purpose"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

type securityProofRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func securityPrincipal(w http.ResponseWriter, r *http.Request) (authdomain.Principal, bool) {
	principal, ok := authdomain.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "需要登录")
		return principal, false
	}
	return principal, true
}

func (a *AuthAPI) getSecurity(w http.ResponseWriter, r *http.Request) {
	principal, ok := securityPrincipal(w, r)
	if !ok {
		return
	}
	state, err := a.service.GetSecurity(r.Context(), principal)
	if err != nil {
		securityError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, state)
}

func (a *AuthAPI) mutateSecurity(w http.ResponseWriter, r *http.Request) {
	principal, ok := securityPrincipal(w, r)
	if !ok {
		return
	}
	var input authdomain.SecurityInput
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	result, err := a.service.MutateSecurity(r.Context(), principal, input)
	if err != nil {
		securityError(w, r, err)
		return
	}
	if result.Session != nil {
		if !a.recordSessionDevice(w, r, *result.Session) {
			return
		}
		a.setSessionCookie(w, *result.Session)
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *AuthAPI) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Identifier string `json:"identifier"`
	}
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	if len(input.Identifier) > 254 || input.Identifier == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "请输入用户名或邮箱")
		return
	}
	if err := a.service.ForgotPassword(r.Context(), input.Identifier); err != nil {
		securityError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"message": "如果该账号已有验证邮箱，恢复邮件将发送至该邮箱。"})
}

func (a *AuthAPI) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var input securityTokenRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	if err := a.service.VerifyEmail(r.Context(), input.Token, input.Purpose); err != nil {
		securityError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AuthAPI) resetPassword(w http.ResponseWriter, r *http.Request) {
	var input securityTokenRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	if err := a.service.ResetPassword(r.Context(), input.Token, input.Password, input.Code); err != nil {
		securityError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AuthAPI) verifyMFA(w http.ResponseWriter, r *http.Request) {
	var input securityTokenRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	session, err := a.service.CompleteMFALogin(r.Context(), input.Token, input.Code)
	if err != nil {
		securityError(w, r, err)
		return
	}
	if !a.recordSessionDevice(w, r, session) {
		return
	}
	a.setSessionCookie(w, session)
	writeAuthResult(w, http.StatusOK, session)
}

func (a *AuthAPI) getMailConfig(w http.ResponseWriter, r *http.Request) {
	principal, ok := securityPrincipal(w, r)
	if !ok {
		return
	}
	config, err := a.service.GetMailConfig(r.Context(), principal)
	if err != nil {
		securityError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, config)
}

func (a *AuthAPI) updateMailConfig(w http.ResponseWriter, r *http.Request) {
	principal, ok := securityPrincipal(w, r)
	if !ok {
		return
	}
	var input authdomain.MailConfigInput
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	config, err := a.service.UpdateMailConfig(r.Context(), principal, input)
	if err != nil {
		securityError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, config)
}

func (a *AuthAPI) testMailConfig(w http.ResponseWriter, r *http.Request) {
	principal, ok := securityPrincipal(w, r)
	if !ok {
		return
	}
	var input securityProofRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	if err := a.service.TestMail(r.Context(), principal, input.Password, input.Code); err != nil {
		securityError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *AuthAPI) getSystemState(w http.ResponseWriter, r *http.Request) {
	principal, ok := securityPrincipal(w, r)
	if !ok {
		return
	}
	state, err := a.service.GetSystemState(r.Context(), principal)
	if err != nil {
		securityError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, state)
}

func (a *AuthAPI) setSystemAdmin(w http.ResponseWriter, r *http.Request) {
	principal, ok := securityPrincipal(w, r)
	if !ok {
		return
	}
	var input struct {
		ActorID  uuid.UUID `json:"actor_id"`
		Grant    bool      `json:"grant"`
		Password string    `json:"password"`
		Code     string    `json:"code"`
	}
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	if err := a.service.SetSystemAdmin(r.Context(), principal, input.ActorID, input.Grant, input.Password, input.Code); err != nil {
		securityError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func securityError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *authdomain.ValidationError
	switch {
	case errors.As(err, &validation):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, validation.Message)
	case errors.Is(err, authdomain.ErrSecurityCooldown):
		retryAfter := 10 * time.Minute
		var cooldown *authdomain.SecurityCooldownError
		if errors.As(err, &cooldown) {
			retryAfter = cooldown.RetryAfter
		}
		w.Header().Set("Retry-After", strconv.FormatInt(int64((retryAfter+time.Second-1)/time.Second), 10))
		httpx.WriteError(w, r, http.StatusTooManyRequests, httpx.CodeRateLimited, "安全操作过于频繁，请稍后重试")
	case errors.Is(err, authdomain.ErrInvalidCredentials), errors.Is(err, authdomain.ErrSecurityToken):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "密码、验证码或链接无效；验证码不可重复使用")
	case errors.Is(err, authdomain.ErrUnauthenticated):
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "会话已失效，请重新登录")
	case errors.Is(err, authdomain.ErrSecurityForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "需要本人浏览器会话或系统管理员权限")
	case errors.Is(err, authdomain.ErrLastSystemAdmin):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "必须保留至少一名有效的系统管理员")
	case errors.Is(err, authdomain.ErrAccountExists):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "邮箱已被使用")
	case errors.Is(err, authdomain.ErrSecurityUnavailable):
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeInternal, "邮件或账号安全配置不可用，请联系系统管理员")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "账号安全操作失败")
	}
}
