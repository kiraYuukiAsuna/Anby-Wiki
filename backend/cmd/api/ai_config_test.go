package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
)

func TestClassifyAIConfigProviderError(t *testing.T) {
	tests := []struct {
		code        string
		wantReason  string
		wantMessage string
	}{
		{"authentication_failed", "authentication_failed", "模型供应商拒绝了 API Key"},
		{"rate_limited", "rate_limited", "模型供应商正在限流，请稍后重试"},
		{"invalid_request", "invalid_request", "模型地址、模型 ID 或请求格式不被供应商接受"},
		{"provider_unavailable", "provider_unavailable", "模型供应商暂时不可用"},
		{"invalid_structured_output", "structured_output_invalid", "模型未返回兼容的结构化输出"},
		{"unknown", "provider_error", "模型配置测试失败"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			reason, message := classifyAIConfigProviderError(&ai.ProviderError{Code: test.code})
			if reason != test.wantReason || message != test.wantMessage {
				t.Fatalf("got (%q,%q), want (%q,%q)",
					reason, message, test.wantReason, test.wantMessage)
			}
		})
	}
}

func TestAPIAccountAIConfigIsolationE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	if baseURL == "" || password == "" {
		t.Skip("set API_E2E_BASE_URL and API_E2E_PASSWORD to an isolated test stack")
	}
	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	username := "model-a-" + runID
	first, firstActor := registerE2EUser(t, baseURL, username, username+"@example.invalid", password)
	second, _ := registerE2EUser(t, baseURL, "model-b-"+runID, "model-b-"+runID+"@example.invalid", password)
	endpoint := baseURL + "/api/v1/account/ai-config"

	checkConfig := func(client *http.Client, headers http.Header, model string, keyConfigured bool, tokens, characters int) {
		t.Helper()
		config := requestE2EMapHeaders(t, client, http.MethodGet, endpoint, nil, headers, http.StatusOK)
		if config["model"] != model || config["api_key_configured"] != keyConfigured ||
			config["max_input_tokens"] != float64(tokens) || config["chunk_characters"] != float64(characters) {
			t.Fatalf("unexpected model configuration: %#v", config)
		}
		for _, field := range []string{"api_key", "api_key_ciphertext"} {
			if _, exposed := config[field]; exposed {
				t.Fatalf("model configuration exposed %s", field)
			}
		}
	}
	body := map[string]any{
		"enabled": true, "provider": "openai-compatible",
		"base_url": "https://api-e2e.invalid/v1", "model": "other-account",
		"response_format": "json_schema", "max_input_tokens": 128000,
		"chunk_characters": 32000, "auto_approve_import_plans": true,
		"request_timeout_seconds": 5, "max_attempts": 1,
		"api_key": "isolated-model-test-key-b",
	}
	requestE2EMap(t, second, http.MethodPut, endpoint, body, http.StatusOK)
	page := requestE2EMap(t, second, http.MethodPost, baseURL+"/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Model scope " + runID, "language": "en", "content_model": "block-v1"}, http.StatusCreated)
	pageID := e2eUUID(t, page, "id")
	requestE2EMap(t, second, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions", baseURL, pageID),
		map[string]any{"ast": documentE2E(uuid.New(), "Model scope", "Shared page"), "summary": "model scope fixture"},
		http.StatusCreated)
	requestE2EMap(t, second, http.MethodPut,
		fmt.Sprintf("%s/api/v1/pages/%s/shares", baseURL, pageID),
		map[string]any{"identifier": username, "role_key": "viewer"}, http.StatusOK)
	session := requestE2EMap(t, second, http.MethodGet, baseURL+"/api/v1/auth/session", nil, http.StatusOK)
	selected := http.Header{"X-Wiki-ID": []string{e2eUUID(t, session, "wiki_id").String()}}
	selectedSession := requestE2EMapHeaders(t, first, http.MethodGet, baseURL+"/api/v1/auth/session", nil, selected, http.StatusOK)
	if selectedSession["wiki_id"] != session["wiki_id"] {
		t.Fatal("collaboration Wiki was not selected")
	}
	ownSession := requestE2EMap(t, first, http.MethodGet, baseURL+"/api/v1/auth/session", nil, http.StatusOK)
	if selectedSession["home_wiki_id"] != ownSession["wiki_id"] ||
		ownSession["home_wiki_id"] != ownSession["wiki_id"] {
		t.Fatal("session lost account Wiki when selecting a collaboration Wiki")
	}

	checkConfig(first, selected, "", false, 128000, 32000)
	requestE2EMapHeaders(t, first, http.MethodGet, baseURL+"/api/v1/admin/ai-config", nil, selected, http.StatusForbidden)
	// An empty own configuration must not borrow the selected Wiki's key.
	body["model"], body["enabled"], body["api_key"] = "own-account", false, ""
	body["max_input_tokens"], body["chunk_characters"] = 12800, 12800
	requestE2EMapHeaders(t, first, http.MethodPut, endpoint, body, selected, http.StatusBadRequest)
	body["api_key"] = "isolated-model-test-key-a"
	saved := requestE2EMapHeaders(t, first, http.MethodPut, endpoint, body, selected, http.StatusOK)
	if e2eUUID(t, saved, "updated_by") != firstActor || saved["api_key_configured"] != true {
		t.Fatal("configuration was not saved for the current account")
	}
	checkConfig(first, selected, "own-account", true, 12800, 12800)
	checkConfig(first, nil, "own-account", true, 12800, 12800)
	checkConfig(second, nil, "other-account", true, 128000, 32000)
	// The connection test must see the disabled own configuration even though
	// the selected collaboration Wiki has an enabled model.
	requestE2EMapHeaders(t, first, http.MethodPost, endpoint+"/test", map[string]any{}, selected, http.StatusConflict)

	body["api_key"] = ""
	body["max_input_tokens"], body["chunk_characters"] = 20000000, 20000000
	requestE2EMapHeaders(t, first, http.MethodPut, endpoint, body, selected, http.StatusOK)
	checkConfig(first, selected, "own-account", true, 20000000, 20000000)
	for _, invalid := range []struct {
		field string
		value int
	}{
		{"max_input_tokens", 12799}, {"max_input_tokens", 20000001},
		{"chunk_characters", 12799}, {"chunk_characters", 20000001},
	} {
		body[invalid.field] = invalid.value
		requestE2EMapHeaders(t, first, http.MethodPut, endpoint, body, selected, http.StatusBadRequest)
		body[invalid.field] = 20000000
	}
	checkConfig(first, nil, "own-account", true, 20000000, 20000000)
	checkConfig(second, nil, "other-account", true, 128000, 32000)
}
