package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/anby/wiki/backend/internal/platform/id"
)

type PromptResolver interface {
	ActivePrompt(ctx context.Context, key string) (*Prompt, error)
}

type UsageRecorder interface {
	InsertUsage(ctx context.Context, usage *Usage) error
}

type GatewayConfig struct {
	Timeout       time.Duration
	MaxAttempts   int
	RetryBase     time.Duration
	MaxConcurrent int
}

type Gateway struct {
	prompts   PromptResolver
	usage     UsageRecorder
	ids       *id.Generator
	providers map[string]Provider
	config    GatewayConfig
	limit     chan struct{}
}

func NewGateway(prompts PromptResolver, usage UsageRecorder, ids *id.Generator, providers map[string]Provider, cfg GatewayConfig) *Gateway {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.RetryBase <= 0 {
		cfg.RetryBase = 100 * time.Millisecond
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 4
	}
	return &Gateway{prompts: prompts, usage: usage, ids: ids, providers: providers,
		config: cfg, limit: make(chan struct{}, cfg.MaxConcurrent)}
}

func (g *Gateway) Generate(ctx context.Context, request Request) (*Result, error) {
	ctx, span := otel.Tracer("github.com/anby/wiki/backend/ai").Start(ctx, "ai.generate")
	defer span.End()
	span.SetAttributes(
		attribute.String("ai.provider", request.Provider),
		attribute.String("ai.model", request.Model),
	)
	provider := g.providers[request.Provider]
	if provider == nil {
		span.SetStatus(codes.Error, "provider_not_registered")
		return nil, fmt.Errorf("%w: provider=%s", ErrProvider, request.Provider)
	}
	prompt, err := g.prompts.ActivePrompt(ctx, request.PromptKey)
	if err != nil {
		return nil, err
	}
	system, err := renderPrompt(prompt.System, request.Variables)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPrompt, err)
	}
	user, err := renderPrompt(prompt.User, request.Variables)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPrompt, err)
	}

	select {
	case g.limit <- struct{}{}:
		defer func() { <-g.limit }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	started := time.Now()
	status, errorCode := UsageFailed, "provider_error"
	var response *ProviderResponse
	var callErr error
	attempts := 0
	for attempts < g.config.MaxAttempts {
		attempts++
		callCtx, cancel := context.WithTimeout(ctx, g.config.Timeout)
		response, callErr = provider.Generate(callCtx, ProviderRequest{
			Model: request.Model, SystemPrompt: system, UserPrompt: user, JSONSchema: prompt.OutputSchema,
		})
		deadline := errors.Is(callCtx.Err(), context.DeadlineExceeded)
		cancel()
		if callErr == nil && !deadline {
			break
		}
		if deadline || errors.Is(callErr, context.DeadlineExceeded) {
			callErr, status, errorCode = ErrTimeout, UsageTimeout, "timeout"
		}
		var providerErr *ProviderError
		retry := errors.As(callErr, &providerErr) && providerErr.Temporary
		if !retry || attempts == g.config.MaxAttempts {
			break
		}
		timer := time.NewTimer(g.config.RetryBase * time.Duration(1<<(attempts-1)))
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			callErr = ctx.Err()
			attempts = g.config.MaxAttempts
		}
	}

	if callErr == nil {
		if err := validateStructuredOutput(prompt, response.JSON); err != nil {
			callErr, status, errorCode = err, UsageInvalidOutput, "schema_validation"
		} else {
			status, errorCode = UsageSucceeded, ""
		}
	}
	usageID, idErr := g.ids.New()
	if idErr != nil {
		return nil, idErr
	}
	u := &Usage{ID: usageID, ImportJobID: request.ImportJobID, ImportRunID: request.ImportRunID,
		Provider: request.Provider, Model: request.Model, PromptKey: prompt.Key,
		PromptVersion: prompt.Version, AttemptCount: attempts, Latency: time.Since(started),
		Status: status, ErrorCode: errorCode}
	if response != nil {
		u.InputTokens, u.OutputTokens = response.InputTokens, response.OutputTokens
	}
	if err := g.usage.InsertUsage(ctx, u); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "usage_record_failed")
		return nil, fmt.Errorf("ai: 记录用量失败: %w", err)
	}
	span.SetAttributes(
		attribute.String("ai.result", status),
		attribute.Int("ai.attempt_count", attempts),
	)
	if callErr != nil {
		span.RecordError(callErr)
		span.SetStatus(codes.Error, errorCode)
		return nil, callErr
	}
	return &Result{JSON: response.JSON, PromptKey: prompt.Key, PromptVersion: prompt.Version,
		Provider: request.Provider, Model: request.Model, InputTokens: response.InputTokens,
		OutputTokens: response.OutputTokens}, nil
}

func validateStructuredOutput(prompt *Prompt, output []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(prompt.OutputSchema))
	if err != nil {
		return fmt.Errorf("%w: Prompt Schema 无效: %v", ErrInvalidOutput, err)
	}
	compiler := jsonschema.NewCompiler()
	url := "https://anby.wiki/prompts/" + prompt.Key + "/output.schema.json"
	if err := compiler.AddResource(url, doc); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(output))
	if err != nil {
		return fmt.Errorf("%w: 非法 JSON", ErrInvalidOutput)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	return nil
}
