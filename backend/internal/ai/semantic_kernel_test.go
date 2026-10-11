package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSemanticKernelProviderMapsStructuredOutputFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"invalid_structured_output","message":"invalid","temporary":false}`))
	}))
	t.Cleanup(server.Close)

	provider, err := NewSemanticKernelProvider(
		server.URL,
		"internal-token",
		SemanticKernelConfigResolverFunc(func(context.Context) (*SemanticKernelConfig, error) {
			return &SemanticKernelConfig{
				Provider: "deepseek", BaseURL: "https://api.deepseek.com",
				APIKey: "secret", Model: "model", ResponseFormat: "json_object",
				RequestTimeoutSeconds: 30, MaxAttempts: 2,
			}, nil
		}),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = provider.Generate(context.Background(), ProviderRequest{
		SystemPrompt: "system", UserPrompt: "user", JSONSchema: []byte(`{"type":"object"}`),
	})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput, got %v", err)
	}
	if errors.Is(err, ErrProvider) {
		t.Fatalf("structured output failure must not be classified as provider failure: %v", err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "invalid_structured_output" {
		t.Fatalf("expected stable kernel error code, got %v", err)
	}
}

func TestSemanticKernelProviderEncodesImageInputs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Images []struct {
				MediaType string `json:"media_type"`
				Data      string `json:"data_base64"`
			} `json:"images"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Images) != 1 ||
			body.Images[0].MediaType != "image/png" ||
			body.Images[0].Data != base64.StdEncoding.EncodeToString([]byte("png")) {
			t.Fatalf("images=%#v", body.Images)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version":1,
			"provider":"deepseek",
			"model":"vision-model",
			"json":{},
			"input_tokens":1,
			"output_tokens":1
		}`))
	}))
	t.Cleanup(server.Close)

	provider, err := NewSemanticKernelProvider(
		server.URL,
		"internal-token",
		SemanticKernelConfigResolverFunc(func(context.Context) (*SemanticKernelConfig, error) {
			return &SemanticKernelConfig{
				Provider: "deepseek", BaseURL: "https://api.deepseek.com",
				APIKey: "secret", Model: "vision-model", ResponseFormat: "json_object",
				RequestTimeoutSeconds: 30, MaxAttempts: 1,
			}, nil
		}),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = provider.Generate(context.Background(), ProviderRequest{
		SystemPrompt: "system", UserPrompt: "user",
		JSONSchema: []byte(`{"type":"object"}`),
		Images:     []ImageInput{{MediaType: "image/png", Data: []byte("png")}},
	})
	if err != nil {
		t.Fatal(err)
	}
}
