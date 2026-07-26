package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func setupAuthAPI(t *testing.T) (*testkit.DB, *authdomain.Service, http.Handler) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	service := authdomain.NewService(
		tdb.Pool,
		db.NewTxManager(tdb.Pool),
		id.NewGenerator(),
		time.Hour,
	)
	authenticator := authdomain.NewAuthenticator(service, "anby_session", true)
	router := NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{
			Service:             "wiki-api",
			Version:             "test",
			Environment:         "test",
			Authenticator:       authenticator,
			SessionCookie:       "anby_session",
			TrustedOrigins:      []string{"https://wiki.example.com"},
			AllowDevActorHeader: true,
		},
		nil, nil, nil, nil, nil,
		NewAuthAPI(service, "anby_session", false, true),
	)
	return tdb, service, router
}

func TestAuthSessionEndpoint(t *testing.T) {
	tdb, _, router := setupAuthAPI(t)

	status, body := doJSON(t, router, http.MethodGet, "/api/v1/auth/session", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous session status=%d body=%v", status, body)
	}
	assertErrorShape(t, body, httpx.CodeUnauthorized)

	actorID := tdb.MakeActor(t, "human", "Alice")
	status, body = doJSON(t, router, http.MethodGet, "/api/v1/auth/session", nil,
		map[string]string{authdomain.DevActorHeader: actorID.String()})
	if status != http.StatusOK {
		t.Fatalf("authenticated session status=%d body=%v", status, body)
	}
	if body["actor_id"] != actorID.String() ||
		body["actor_type"] != "human" ||
		body["display_name"] != "Alice" ||
		body["method"] != "development_header" {
		t.Fatalf("session DTO=%v", body)
	}
}

func TestAuthLogoutRevokesCookieSession(t *testing.T) {
	_, service, router := setupAuthAPI(t)
	session, err := service.EstablishSession(context.Background(), authdomain.Identity{
		Issuer:      "https://id.example.com",
		Subject:     "logout-user",
		DisplayName: "Logout User",
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: "anby_session", Value: session.Token})
	request.Header.Set("Origin", "https://wiki.example.com")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "anby_session" || cookies[0].MaxAge != -1 {
		t.Fatalf("logout clear cookies=%v", cookies)
	}
	if _, err := service.ResolveSession(context.Background(), session.Token); err != authdomain.ErrUnauthenticated {
		t.Fatalf("logout 后 session 仍有效: %v", err)
	}
}

func TestAuthLogoutRejectsUntrustedBrowserWithoutRevokingSession(t *testing.T) {
	_, service, router := setupAuthAPI(t)
	session, err := service.EstablishSession(context.Background(), authdomain.Identity{
		Issuer: "https://id.example.com", Subject: "csrf-logout-user", DisplayName: "CSRF Logout User",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, headers := range []map[string]string{
		nil,
		{"Origin": "https://evil.example"},
		{"Referer": "https://evil.example/logout"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		request.AddCookie(&http.Cookie{Name: "anby_session", Value: session.Token})
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("logout status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		if _, err := service.ResolveSession(context.Background(), session.Token); err != nil {
			t.Fatalf("CSRF 拒绝后 session 被撤销: %v", err)
		}
	}
}

func TestRegisterAndLoginIssueIndependentSessions(t *testing.T) {
	_, service, router := setupAuthAPI(t)
	first := doAuthRequest(t, router, "/api/v1/auth/register", map[string]any{
		"username": "alice", "email": "alice@example.com",
		"password": "correct horse battery staple", "display_name": "Alice",
	}, http.StatusCreated)
	second := doAuthRequest(t, router, "/api/v1/auth/login", map[string]any{
		"identifier": "ALICE@EXAMPLE.COM", "password": "correct horse battery staple",
	}, http.StatusOK)
	if first.actorID != second.actorID {
		t.Fatalf("account actor changed from %s to %s", first.actorID, second.actorID)
	}
	if first.token == second.token {
		t.Fatal("register and login must issue distinct session tokens")
	}
	principal, err := service.ResolveSession(context.Background(), second.token)
	if err != nil || principal.ActorID.String() != first.actorID {
		t.Fatalf("resolve principal=%+v err=%v", principal, err)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	_, _, router := setupAuthAPI(t)
	doAuthRequest(t, router, "/api/v1/auth/register", map[string]any{
		"username": "alice", "email": "alice@example.com",
		"password": "correct horse battery staple",
	}, http.StatusCreated)
	status, body := doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		map[string]any{"identifier": "alice", "password": "wrong password"},
		map[string]string{"Origin": "https://wiki.example.com"})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong password status=%d body=%v", status, body)
	}
	assertErrorShape(t, body, httpx.CodeUnauthorized)
}

func TestRegistrationDisabledReturnsForbidden(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	service := authdomain.NewService(tdb.Pool, db.NewTxManager(tdb.Pool), id.NewGenerator(), time.Hour)
	router := NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{
			Service: "wiki-api", Version: "test", Environment: "test",
			SessionCookie:  "anby_session",
			TrustedOrigins: []string{"https://wiki.example.com"},
		},
		nil, nil, nil, nil, nil,
		NewAuthAPI(service, "anby_session", false, false),
	)
	status, body := doJSON(t, router, http.MethodPost, "/api/v1/auth/register",
		map[string]any{
			"username": "alice", "email": "alice@example.com",
			"password": "correct horse battery staple",
		},
		map[string]string{"Origin": "https://wiki.example.com"})
	if status != http.StatusForbidden {
		t.Fatalf("disabled registration status=%d body=%v", status, body)
	}
}

type authResult struct {
	actorID     string
	displayName string
	token       string
}

func doAuthRequest(t *testing.T, router http.Handler, path string, body map[string]any, wantStatus int) authResult {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://wiki.example.com")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
	}
	var resultBody map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &resultBody); err != nil {
		t.Fatal(err)
	}
	var sessionToken string
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "anby_session" {
			sessionToken = cookie.Value
		}
	}
	if sessionToken == "" {
		t.Fatal("auth response did not set a session cookie")
	}
	return authResult{
		actorID: resultBody["actor_id"], displayName: resultBody["display_name"], token: sessionToken,
	}
}
