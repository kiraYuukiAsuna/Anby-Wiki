package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/wikicli"
)

func TestAPICrossOriginProtectionE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD")
	}
	username := "csrf_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	client, actorID := registerE2EUser(t, baseURL, username, username+"@example.invalid", password)
	original := requestE2EMap(t, client, http.MethodGet, baseURL+"/api/v1/account/profile", nil, http.StatusOK)
	changes := map[string]any{
		"email": original["email"], "display_name": "Same origin update", "wiki_name": original["wiki_name"],
	}
	sameOrigin := http.Header{"Sec-Fetch-Site": []string{"same-origin"}, "Origin": []string{baseURL}}
	updated := requestE2EMapHeaders(t, client, http.MethodPut, baseURL+"/api/v1/account/profile",
		changes, sameOrigin, http.StatusOK)
	if updated["display_name"] != changes["display_name"] {
		t.Fatal("same-origin profile write did not persist")
	}

	for _, headers := range []http.Header{
		{"Sec-Fetch-Site": []string{"cross-site"}, "Origin": []string{"https://evil.test"}},
		{"Sec-Fetch-Site": []string{"same-site"}, "Origin": []string{"https://other.wiki.test"}},
		{"Origin": []string{"https://evil.test"}, "X-Forwarded-Host": []string{"evil.test"}},
		{"Origin": []string{"null"}},
	} {
		failure := requestE2EMapHeaders(t, client, http.MethodPut, baseURL+"/api/v1/account/profile",
			map[string]any{"email": original["email"], "display_name": "Cross origin update", "wiki_name": original["wiki_name"]},
			headers, http.StatusForbidden)
		requestID, ok := failure["request_id"].(string)
		if failure["code"] != httpx.CodeForbidden || !ok || requestID == "" {
			t.Fatal("cross-origin denial lacks the standard error contract")
		}
		current := requestE2EMap(t, client, http.MethodGet, baseURL+"/api/v1/account/profile", nil, http.StatusOK)
		if current["display_name"] != updated["display_name"] || current["email"] != updated["email"] {
			t.Fatal("rejected cross-origin request changed authoritative profile data")
		}
	}

	crossSite := http.Header{"Sec-Fetch-Site": []string{"cross-site"}, "Origin": []string{"https://evil.test"}}
	requestE2EMapHeaders(t, client, http.MethodPost, baseURL+"/api/v1/auth/logout", nil,
		crossSite, http.StatusForbidden)
	requestE2EMap(t, client, http.MethodGet, baseURL+"/api/v1/auth/session", nil, http.StatusOK)
	anonymous := &http.Client{Timeout: collaborationE2ETimeout}
	requestE2EMapHeaders(t, anonymous, http.MethodPost, baseURL+"/api/v1/auth/login",
		map[string]any{"identifier": username, "password": password}, crossSite, http.StatusForbidden)
	requestE2EMap(t, anonymous, http.MethodGet, baseURL+"/api/v1/auth/session", nil, http.StatusUnauthorized)

	// A failed browser exchange must leave the one-time code redeemable by CLI.
	code := requestE2EMapHeaders(t, client, http.MethodPost, baseURL+"/api/v1/auth/cli/codes",
		map[string]any{"name": "CSRF CLI", "token_ttl_days": 30}, sameOrigin, http.StatusCreated)
	requestE2EMapHeaders(t, anonymous, http.MethodPost, baseURL+"/api/v1/auth/cli/exchange",
		map[string]any{"code": code["code"]}, crossSite, http.StatusForbidden)
	app, err := wikicli.New("csrf-e2e")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "cli.json")
	exchange, exitCode := app.Execute(context.Background(), wikicli.Input{
		Action: "auth.exchange", BaseURL: baseURL, ConfigPath: configPath, Code: code["code"].(string),
	})
	if !exchange.OK || exitCode != 0 {
		t.Fatal("non-browser CLI could not exchange the authorization code")
	}
	status, exitCode := app.Execute(context.Background(), wikicli.Input{Action: "auth.status", ConfigPath: configPath})
	if !status.OK || exitCode != 0 || status.Data.(map[string]any)["actor_id"] != actorID.String() {
		t.Fatal("CLI identity authentication did not survive CSRF protection")
	}
	logout, exitCode := app.Execute(context.Background(), wikicli.Input{Action: "auth.logout", ConfigPath: configPath})
	if !logout.OK || exitCode != 0 {
		t.Fatal("non-browser CLI could not revoke its credential")
	}
	requestE2ENoBody(t, client, http.MethodPost, baseURL+"/api/v1/auth/logout", http.StatusNoContent)
}
