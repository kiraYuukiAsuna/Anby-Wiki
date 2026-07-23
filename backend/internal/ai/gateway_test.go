package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type promptStub struct{ prompt *ai.Prompt }

func (s promptStub) ActivePrompt(context.Context, string) (*ai.Prompt, error) { return s.prompt, nil }

type usageStub struct {
	mu    sync.Mutex
	items []*ai.Usage
}

func (s *usageStub) InsertUsage(_ context.Context, usage *ai.Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, usage)
	return nil
}

type providerFunc func(context.Context, ai.ProviderRequest) (*ai.ProviderResponse, error)

func (f providerFunc) Generate(ctx context.Context, request ai.ProviderRequest) (*ai.ProviderResponse, error) {
	return f(ctx, request)
}

func testPrompt() *ai.Prompt {
	return &ai.Prompt{ID: uuid.New(), Key: "extract", Version: 3,
		System: "Only JSON for {{.source}}", User: "Text: {{.text}}",
		OutputSchema: json.RawMessage(`{"type":"object","required":["items"],"properties":{"items":{"type":"array"}},"additionalProperties":false}`),
		Active:       true}
}

func TestGateway_StructuredRetryVersionAndUsage(t *testing.T) {
	var calls atomic.Int32
	provider := providerFunc(func(_ context.Context, request ai.ProviderRequest) (*ai.ProviderResponse, error) {
		if calls.Add(1) == 1 {
			return nil, &ai.ProviderError{Code: "overloaded", Temporary: true, Err: errors.New("retry")}
		}
		if request.Model != "test-model" || request.UserPrompt != "Text: hello" {
			t.Fatalf("request=%+v", request)
		}
		return &ai.ProviderResponse{JSON: json.RawMessage(`{"items":[]}`), InputTokens: 11, OutputTokens: 4}, nil
	})
	usage := &usageStub{}
	gateway := ai.NewGateway(promptStub{testPrompt()}, usage, id.NewGenerator(),
		map[string]ai.Provider{"fake": provider}, ai.GatewayConfig{
			Timeout: time.Second, MaxAttempts: 3, RetryBase: time.Millisecond, MaxConcurrent: 1,
		})
	result, err := gateway.Generate(context.Background(), ai.Request{
		Provider: "fake", Model: "test-model", PromptKey: "extract",
		Variables: map[string]any{"source": "golden", "text": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PromptVersion != 3 || result.InputTokens != 11 || calls.Load() != 2 {
		t.Fatalf("result=%+v calls=%d", result, calls.Load())
	}
	if len(usage.items) != 1 || usage.items[0].Status != ai.UsageSucceeded || usage.items[0].AttemptCount != 2 {
		t.Fatalf("usage=%+v", usage.items)
	}
}

func TestGateway_TimeoutAndInvalidOutputAreRecorded(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider ai.Provider
		wantErr  error
		status   string
	}{
		{
			name: "timeout",
			provider: providerFunc(func(ctx context.Context, _ ai.ProviderRequest) (*ai.ProviderResponse, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}),
			wantErr: ai.ErrTimeout, status: ai.UsageTimeout,
		},
		{
			name: "invalid schema output",
			provider: providerFunc(func(context.Context, ai.ProviderRequest) (*ai.ProviderResponse, error) {
				return &ai.ProviderResponse{JSON: json.RawMessage(`{"items":"forged"}`)}, nil
			}),
			wantErr: ai.ErrInvalidOutput, status: ai.UsageInvalidOutput,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			usage := &usageStub{}
			gateway := ai.NewGateway(promptStub{testPrompt()}, usage, id.NewGenerator(),
				map[string]ai.Provider{"fake": test.provider}, ai.GatewayConfig{
					Timeout: 5 * time.Millisecond, MaxAttempts: 1, MaxConcurrent: 1,
				})
			_, err := gateway.Generate(context.Background(), ai.Request{Provider: "fake", Model: "m",
				PromptKey: "extract", Variables: map[string]any{"source": "s", "text": "t"}})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("err=%v", err)
			}
			if len(usage.items) != 1 || usage.items[0].Status != test.status {
				t.Fatalf("usage=%+v", usage.items)
			}
		})
	}
}
