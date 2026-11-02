package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/google/uuid"
)

type gatewayPromptResolver struct {
	prompt *Prompt
}

func TestGatewayAuditsEveryPhysicalAttemptIncludingUnreportedCancellation(t *testing.T) {
	var usage []*Usage
	receipts := []ProviderAttempt{
		{ID: uuid.New(), Provider: "deepseek", Model: "test", Status: UsageInvalidOutput,
			ErrorCode: "output_truncated", UsageReported: true, InputTokens: 10, OutputTokens: 8192},
		{ID: uuid.New(), Provider: "deepseek", Model: "test", Status: UsageFailed,
			ErrorCode: "interrupted", UsageReported: false},
	}
	gateway := NewGateway(gatewayPromptResolver{prompt: &Prompt{
		Key: "test", Version: 1, User: "request", OutputSchema: json.RawMessage(`{"type":"object"}`),
	}}, gatewayUsageRecorderFunc(func(_ context.Context, u *Usage) error {
		usage = append(usage, u)
		return nil
	}), id.NewGenerator(), map[string]Provider{"test": gatewayProviderFunc(func(context.Context, ProviderRequest) (*ProviderResponse, error) {
		return &ProviderResponse{Attempts: receipts}, context.Canceled
	})}, GatewayConfig{MaxAttempts: 1})
	_, err := gateway.Generate(context.Background(), Request{Provider: "test", Model: "test", PromptKey: "test"})
	if !errors.Is(err, context.Canceled) || len(usage) != 2 {
		t.Fatalf("err=%v usage=%#v", err, usage)
	}
	if usage[0].ID != receipts[0].ID || usage[0].OutputTokens != 8192 ||
		usage[1].ErrorCode != "interrupted"+UsageUnreportedSuffix ||
		usage[0].AttemptCount != 1 || usage[1].AttemptCount != 1 {
		t.Fatalf("incomplete physical usage: %#v", usage)
	}
}

func TestGatewayCancellationDuringBackoffDoesNotInflateAttempts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls, records := 0, 0
	gateway := NewGateway(gatewayPromptResolver{prompt: &Prompt{
		Key: "test", Version: 1, User: "request", OutputSchema: json.RawMessage(`{"type":"object"}`),
	}}, gatewayUsageRecorderFunc(func(_ context.Context, u *Usage) error {
		records++
		if u.AttemptCount != 1 {
			t.Fatalf("attempt count=%d", u.AttemptCount)
		}
		return nil
	}), id.NewGenerator(), map[string]Provider{"test": gatewayProviderFunc(func(context.Context, ProviderRequest) (*ProviderResponse, error) {
		calls++
		cancel()
		return nil, &ProviderError{Code: "unavailable", Temporary: true, Err: ErrProvider}
	})}, GatewayConfig{MaxAttempts: 5, RetryBase: time.Second})
	_, err := gateway.Generate(ctx, Request{Provider: "test", Model: "test", PromptKey: "test"})
	if !errors.Is(err, context.Canceled) || calls != 1 || records != 1 {
		t.Fatalf("err=%v calls=%d records=%d", err, calls, records)
	}
}

func (r gatewayPromptResolver) ActivePrompt(context.Context, string) (*Prompt, error) {
	return r.prompt, nil
}

type gatewayProviderFunc func(context.Context, ProviderRequest) (*ProviderResponse, error)

func (f gatewayProviderFunc) Generate(ctx context.Context, request ProviderRequest) (*ProviderResponse, error) {
	return f(ctx, request)
}

type gatewayUsageRecorderFunc func(context.Context, *Usage) error

func (f gatewayUsageRecorderFunc) InsertUsage(ctx context.Context, usage *Usage) error {
	return f(ctx, usage)
}

func TestGatewayUsageRecordingSurvivesCallerCancellationWithoutMaskingCallError(t *testing.T) {
	causalErr := errors.New("causal provider failure")
	usageErr := errors.New("usage store failure")
	ctx, cancel := context.WithCancel(context.Background())
	usageContextWasLive := false
	var recorded *Usage

	gateway := NewGateway(
		gatewayPromptResolver{prompt: &Prompt{
			Key: "test-prompt", Version: 1, User: "request",
			OutputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		gatewayUsageRecorderFunc(func(ctx context.Context, usage *Usage) error {
			usageContextWasLive = ctx.Err() == nil
			recorded = usage
			return usageErr
		}),
		id.NewGenerator(),
		map[string]Provider{"test": gatewayProviderFunc(func(context.Context, ProviderRequest) (*ProviderResponse, error) {
			cancel()
			return nil, causalErr
		})},
		GatewayConfig{Timeout: time.Second, MaxAttempts: 1, MaxConcurrent: 1},
	)

	_, err := gateway.Generate(ctx, Request{Provider: "test", Model: "test-model", PromptKey: "test-prompt"})
	if !errors.Is(err, causalErr) {
		t.Fatalf("error=%v, want causal provider failure", err)
	}
	if errors.Is(err, usageErr) {
		t.Fatalf("usage failure masked or replaced the causal error: %v", err)
	}
	if !usageContextWasLive {
		t.Fatal("usage recorder received the canceled caller context")
	}
	if recorded == nil || recorded.Status != UsageFailed || recorded.AttemptCount != 1 {
		t.Fatalf("unexpected usage record: %#v", recorded)
	}
}

func TestGatewayPassesValidatedImageInputsToProvider(t *testing.T) {
	var received ProviderRequest
	gateway := NewGateway(
		gatewayPromptResolver{prompt: &Prompt{
			Key: "vision", Version: 1, User: "analyze",
			OutputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		gatewayUsageRecorderFunc(func(context.Context, *Usage) error { return nil }),
		id.NewGenerator(),
		map[string]Provider{"test": gatewayProviderFunc(func(
			_ context.Context,
			request ProviderRequest,
		) (*ProviderResponse, error) {
			received = request
			return &ProviderResponse{JSON: json.RawMessage(`{}`)}, nil
		})},
		GatewayConfig{Timeout: time.Second, MaxAttempts: 1, MaxConcurrent: 1},
	)

	_, err := gateway.Generate(context.Background(), Request{
		Provider: "test", Model: "vision-model", PromptKey: "vision",
		Images: []ImageInput{{MediaType: "image/jpeg", Data: []byte{1, 2, 3}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(received.Images) != 1 ||
		received.Images[0].MediaType != "image/jpeg" ||
		string(received.Images[0].Data) != string([]byte{1, 2, 3}) {
		t.Fatalf("provider images=%#v", received.Images)
	}
}

func TestGatewayRejectsInvalidImageInputBeforeProvider(t *testing.T) {
	called := false
	gateway := NewGateway(
		gatewayPromptResolver{prompt: &Prompt{
			Key: "vision", Version: 1, User: "analyze",
			OutputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		gatewayUsageRecorderFunc(func(context.Context, *Usage) error { return nil }),
		id.NewGenerator(),
		map[string]Provider{"test": gatewayProviderFunc(func(
			context.Context,
			ProviderRequest,
		) (*ProviderResponse, error) {
			called = true
			return &ProviderResponse{JSON: json.RawMessage(`{}`)}, nil
		})},
		GatewayConfig{Timeout: time.Second, MaxAttempts: 1, MaxConcurrent: 1},
	)

	_, err := gateway.Generate(context.Background(), Request{
		Provider: "test", Model: "vision-model", PromptKey: "vision",
		Images: []ImageInput{{MediaType: "image/gif", Data: []byte{1}}},
	})
	if !errors.Is(err, ErrInvalidPrompt) {
		t.Fatalf("error=%v, want ErrInvalidPrompt", err)
	}
	if called {
		t.Fatal("provider called for invalid image input")
	}
}
