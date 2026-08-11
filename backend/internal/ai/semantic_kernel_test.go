package ai

import (
	"context"
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
