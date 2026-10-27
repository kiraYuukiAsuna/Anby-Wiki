package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
)

func TestRequireAuthenticatedWiki(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := RequireAuthenticatedWiki(next)

	t.Run("knowledge route rejects anonymous request", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/pages", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", response.Code, http.StatusUnauthorized)
		}
	})

	t.Run("knowledge route accepts principal with wiki", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/pages", nil)
		request = request.WithContext(authdomain.WithPrincipal(
			request.Context(),
			authdomain.Principal{ActorID: uuid.New(), WikiID: uuid.New()},
		))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", response.Code, http.StatusNoContent)
		}
	})

	t.Run("account route accepts principal without wiki", func(t *testing.T) {
		request := httptest.NewRequest(
			http.MethodGet, "/api/v1/auth/cli/tokens", nil,
		)
		request = request.WithContext(authdomain.WithPrincipal(
			request.Context(),
			authdomain.Principal{ActorID: uuid.New(), Method: "session"},
		))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", response.Code, http.StatusNoContent)
		}
	})

	for _, path := range []string{
		"/api/v1/auth/register",
		"/api/v1/auth/login",
		"/api/v1/auth/cli/exchange",
		"/api/v1/auth/logout",
	} {
		t.Run(path+" remains public", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusNoContent {
				t.Fatalf("status=%d want=%d", response.Code, http.StatusNoContent)
			}
		})
	}

	t.Run("session inspection remains public", func(t *testing.T) {
		request := httptest.NewRequest(
			http.MethodGet, "/api/v1/auth/session", nil,
		)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", response.Code, http.StatusNoContent)
		}
	})
}
