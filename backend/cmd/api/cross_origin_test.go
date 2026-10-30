package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/anby/wiki/backend/internal/clicontract"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

func TestRouterCrossOriginProtection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(logger, Deps{Environment: "production"}, &WriteAPI{}, nil, nil, nil, nil, &AuthAPI{}).(chi.Router)
	calls := 0
	router.Handle("/write-probe", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name          string
		method        string
		path          string
		host          string
		site          string
		origin        string
		forwardedHost string
		withCookie    bool
		status        int
		reachesProbe  bool
	}{
		{name: "same origin through rewrite", method: http.MethodPost, host: "api:8080", site: "same-origin", origin: "http://wiki.test:3000", status: 204, reachesProbe: true},
		{name: "direct client navigation", method: http.MethodPost, site: "none", status: 204, reachesProbe: true},
		{name: "CLI without browser headers", method: http.MethodDelete, status: 204, reachesProbe: true},
		{name: "Origin fallback same host", method: http.MethodPut, origin: "https://wiki.test", status: 204, reachesProbe: true},
		{name: "Origin fallback same host and port", method: http.MethodPatch, host: "wiki.test:3000", origin: "http://wiki.test:3000", status: 204, reachesProbe: true},
		{name: "cross site with session cookie", method: http.MethodPost, site: "cross-site", origin: "https://evil.test", withCookie: true, status: 403},
		{name: "same site other origin", method: http.MethodPost, site: "same-site", origin: "https://other.wiki.test", status: 403},
		{name: "metadata takes precedence over matching Origin", method: http.MethodPost, site: "cross-site", origin: "https://wiki.test", status: 403},
		{name: "Origin fallback cross host", method: http.MethodPut, origin: "https://evil.test", status: 403},
		{name: "Origin fallback cross port", method: http.MethodPost, host: "wiki.test:3000", origin: "http://wiki.test:3001", status: 403},
		{name: "untrusted forwarded host cannot allow Origin", method: http.MethodPost, origin: "https://evil.test", forwardedHost: "evil.test", status: 403},
		{name: "opaque Origin", method: http.MethodPost, origin: "null", status: 403},
		{name: "invalid metadata", method: http.MethodPost, site: "invalid", status: 403},
		{name: "safe read cross site", method: http.MethodGet, site: "cross-site", origin: "https://evil.test", status: 204, reachesProbe: true},
		{name: "safe head cross site", method: http.MethodHead, site: "cross-site", status: 204, reachesProbe: true},
		{name: "safe options cross site", method: http.MethodOptions, site: "cross-site", status: 204, reachesProbe: true},
		{name: "register protected before any session exists", method: http.MethodPost, path: "/api/v1/auth/register", site: "cross-site", status: 403},
		{name: "login protected before any session exists", method: http.MethodPost, path: "/api/v1/auth/login", site: "cross-site", status: 403},
		{name: "logout remains protected", method: http.MethodPost, path: "/api/v1/auth/logout", site: "cross-site", status: 403},
		{name: "CLI exchange browser request remains protected", method: http.MethodPost, path: "/api/v1/auth/cli/exchange", site: "cross-site", status: 403},
		{name: "non-browser request still needs authentication", method: http.MethodPost, path: "/api/v1/pages", status: 401},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.path
			if path == "" {
				path = "/write-probe"
			}
			request := httptest.NewRequest(test.method, "https://wiki.test"+path, nil)
			if test.host != "" {
				request.Host = test.host
			}
			request.Header.Set(requestIDHeader, "cross-origin-test")
			request.Header.Set("Sec-Fetch-Site", test.site)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("X-Forwarded-Host", test.forwardedHost)
			if test.withCookie {
				request.AddCookie(&http.Cookie{Name: "anby_session", Value: "test-session"})
			}
			before := calls
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d", response.Code, test.status)
			}
			if (calls > before) != test.reachesProbe {
				t.Fatalf("business handler reached=%v want=%v", calls > before, test.reachesProbe)
			}
			if test.status == http.StatusForbidden {
				var failure httpx.Error
				if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
					t.Fatal(err)
				}
				if failure.Code != httpx.CodeForbidden || failure.RequestID != response.Header().Get(requestIDHeader) ||
					failure.RequestID != "cross-origin-test" || failure.Message == "" {
					t.Fatalf("invalid CSRF failure: %+v", failure)
				}
			}
		})
	}
}

func TestWriteOperationsDeclareCrossOriginDenial(t *testing.T) {
	contract, err := clicontract.Load()
	if err != nil {
		t.Fatal(err)
	}
	errorBody := map[string]any{
		"code": httpx.CodeForbidden, "message": "不允许跨来源写请求", "request_id": "cross-origin-test",
	}
	for _, operation := range contract.List() {
		if operation.Method == http.MethodGet || operation.Method == http.MethodHead || operation.Method == http.MethodOptions {
			continue
		}
		declared := false
		for _, response := range operation.Responses {
			if response.Status == "403" {
				declared = true
				break
			}
		}
		if !declared {
			t.Errorf("%s does not declare 403", operation.ID)
		}
	}
	if err := contract.ValidateResponse("login", 403, "application/json", errorBody); err != nil {
		t.Fatal(err)
	}
}
