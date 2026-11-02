package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSemanticKernelProviderMapsStructuredOutputFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"invalid_structured_output","message":"model output failed schema validation at $.elements[0].region (required)","temporary":false}`))
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
	if !strings.Contains(err.Error(), "$.elements[0].region (required)") {
		t.Fatalf("safe validation diagnostic was discarded: %v", err)
	}
}

func TestSemanticKernelProviderClassifiesUnavailableKernel(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	endpoint := server.URL
	server.Close()

	provider, err := NewSemanticKernelProvider(
		endpoint,
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
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("unavailable kernel must be classified as provider failure: %v", err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "kernel_unavailable" || !providerErr.Temporary {
		t.Fatalf("unexpected unavailable kernel error: %v", err)
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
			"version":2,
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

func TestKernelStreamRetainsReceiptsWhenCallerCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"type":"attempt_started","attempt":{"id":"019a438e-c200-7000-8000-000000000001","provider":"deepseek","model":"test","status":"failed","error_code":"interrupted","usage_reported":false}}` + "\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	reader := &cancelAfterReceiptReader{reader: response.Body, cancel: cancel}
	result, err := readKernelStream(ctx, reader)
	response.Body.Close()
	if !errors.Is(err, context.Canceled) || len(result.Attempts) != 1 ||
		result.Attempts[0].UsageReported {
		t.Fatalf("lost canceled receipt: %#v err=%v", result, err)
	}
}

func TestKernelStreamRejectsBrokenReceiptSequence(t *testing.T) {
	attempt := `{"id":"019a438e-c200-7000-8000-000000000001","provider":"deepseek","model":"test","status":"failed","error_code":"interrupted","usage_reported":false}`
	started := `{"type":"attempt_started","attempt":` + attempt + "}\n"
	finished := `{"type":"attempt_finished","attempt":` + attempt + "}\n"
	result := `{"type":"result","status":503,"body":{"code":"provider_unavailable","temporary":true}}`
	for _, tc := range []struct {
		name, stream, code string
	}{
		{"duplicate_start", started + started, kernelCodeInvalidReceipt},
		{"finish_without_start", finished, kernelCodeInvalidReceipt},
		{"duplicate_finish", started + finished + finished, kernelCodeInvalidReceipt},
		{"unfinished_result", started + result, kernelCodeIncompleteReceipt},
		{"negative_tokens", strings.Replace(started, `"usage_reported":false`, `"input_tokens":-1`, 1), kernelCodeInvalidReceipt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readKernelStream(context.Background(), strings.NewReader(tc.stream))
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.Code != tc.code || !errors.Is(err, ErrInvalidOutput) {
				t.Fatalf("unexpected receipt error: %v", err)
			}
		})
	}
}

type cancelAfterReceiptReader struct {
	reader io.Reader
	cancel context.CancelFunc
	read   bool
}

func (r *cancelAfterReceiptReader) Read(data []byte) (int, error) {
	if r.read {
		r.cancel()
		return 0, context.Canceled
	}
	r.read = true
	return r.reader.Read(data)
}
