package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/ai"
)

func TestOpenAICompatibleProvider_StructuredRequestAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request path=%s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		format := body["response_format"].(map[string]any)
		if format["type"] != "json_schema" || format["json_schema"].(map[string]any)["strict"] != true {
			t.Fatalf("response_format=%v", format)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[]}"}}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`))
	}))
	defer server.Close()

	provider, err := ai.NewOpenAICompatibleProvider(server.URL+"/v1", "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Generate(context.Background(), ai.ProviderRequest{Model: "model",
		SystemPrompt: "system", UserPrompt: "user", JSONSchema: json.RawMessage(`{"type":"object"}`)})
	if err != nil || string(result.JSON) != `{"items":[]}` || result.InputTokens != 12 || result.OutputTokens != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestDeepSeekProvider_UsesDocumentedJSONObjectMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request path=%s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		format := body["response_format"].(map[string]any)
		if format["type"] != "json_object" {
			t.Fatalf("response_format=%v", format)
		}
		if _, exists := format["json_schema"]; exists {
			t.Fatalf("DeepSeek 请求不得发送 json_schema: %v", format)
		}
		thinking := body["thinking"].(map[string]any)
		if thinking["type"] != "disabled" {
			t.Fatalf("thinking=%v", thinking)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[]}"}}],"usage":{"prompt_tokens":8,"completion_tokens":3}}`))
	}))
	defer server.Close()

	provider, err := ai.NewDeepSeekProvider(server.URL, "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Generate(context.Background(), ai.ProviderRequest{Model: "deepseek-v4-flash",
		SystemPrompt: "return JSON", UserPrompt: "extract", JSONSchema: json.RawMessage(`{"type":"object"}`)})
	if err != nil || string(result.JSON) != `{"items":[]}` || result.InputTokens != 8 || result.OutputTokens != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestOpenAICompatibleProvider_SafeRetryClassification(t *testing.T) {
	for _, test := range []struct {
		status    int
		temporary bool
	}{
		{status: http.StatusBadRequest},
		{status: http.StatusTooManyRequests, temporary: true},
		{status: http.StatusBadGateway, temporary: true},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "sensitive provider response", test.status)
		}))
		provider, err := ai.NewOpenAICompatibleProvider(server.URL, "secret", server.Client())
		if err != nil {
			t.Fatal(err)
		}
		_, err = provider.Generate(context.Background(), ai.ProviderRequest{Model: "model", JSONSchema: json.RawMessage(`{"type":"object"}`)})
		server.Close()
		var providerErr *ai.ProviderError
		if !errors.As(err, &providerErr) || providerErr.Temporary != test.temporary ||
			!strings.Contains(providerErr.Error(), fmt.Sprintf("http_%d", test.status)) ||
			strings.Contains(providerErr.Error(), "sensitive provider response") {
			t.Fatalf("status=%d err=%+v", test.status, err)
		}
	}
}
