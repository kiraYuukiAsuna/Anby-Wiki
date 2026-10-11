package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/anby/wiki/backend/internal/platform/id"
)

type gatewayPromptResolver struct {
	prompt *Prompt
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
