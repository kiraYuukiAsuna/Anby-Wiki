package main

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/anby/wiki/backend/internal/platform/httpx"
)

// BrowserWriteGuard protects state-changing requests authenticated by a
// browser session cookie. Header-authenticated development/test requests do
// not carry that cookie and intentionally remain outside this CSRF boundary.
func BrowserWriteGuard(sessionCookie string, trustedOrigins []string) func(http.Handler) http.Handler {
	trusted := make(map[string]struct{}, len(trustedOrigins))
	for _, raw := range trustedOrigins {
		if origin, ok := exactOrigin(raw); ok {
			trusted[origin] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isWriteMethod(r.Method) || !hasCookie(r, sessionCookie) {
				next.ServeHTTP(w, r)
				return
			}
			origin, ok := requestSourceOrigin(r)
			if !ok {
				httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "写请求来源不可信")
				return
			}
			if _, allowed := trusted[origin]; !allowed {
				httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "写请求来源不可信")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func hasCookie(r *http.Request, name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	_, err := r.Cookie(name)
	return err == nil
}

func requestSourceOrigin(r *http.Request) (string, bool) {
	if raw := strings.TrimSpace(r.Header.Get("Origin")); raw != "" {
		return exactOrigin(raw)
	}
	raw := strings.TrimSpace(r.Header.Get("Referer"))
	if raw == "" {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	return normalizedOrigin(parsed), true
}

func exactOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return normalizedOrigin(parsed), true
}

func normalizedOrigin(parsed *url.URL) string {
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
}
