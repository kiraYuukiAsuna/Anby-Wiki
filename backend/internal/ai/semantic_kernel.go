package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const semanticKernelProtocolVersion = 1

type SemanticKernelConfig struct {
	Provider              string
	BaseURL               string
	APIKey                string
	Model                 string
	ResponseFormat        string
	RequestTimeoutSeconds int
	MaxAttempts           int
}

type SemanticKernelConfigResolver interface {
	ResolveSemanticKernelConfig(context.Context) (*SemanticKernelConfig, error)
}

type SemanticKernelConfigResolverFunc func(context.Context) (*SemanticKernelConfig, error)

func (f SemanticKernelConfigResolverFunc) ResolveSemanticKernelConfig(ctx context.Context) (*SemanticKernelConfig, error) {
	return f(ctx)
}

type SemanticKernelProvider struct {
	endpoint string
	token    string
	resolver SemanticKernelConfigResolver
	client   *http.Client
}

func NewSemanticKernelProvider(
	baseURL, token string,
	resolver SemanticKernelConfigResolver,
	client *http.Client,
) (*SemanticKernelProvider, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("ai: AI_KERNEL_URL 非法")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("ai: AI_KERNEL_INTERNAL_TOKEN 不能为空")
	}
	if resolver == nil {
		return nil, fmt.Errorf("ai: Semantic Kernel 配置解析器不能为空")
	}
	if client == nil {
		client = &http.Client{}
	}
	return &SemanticKernelProvider{
		endpoint: strings.TrimRight(parsed.String(), "/") + "/v1/generate",
		token:    token, resolver: resolver, client: client,
	}, nil
}

type semanticKernelGenerateRequest struct {
	Version      int                  `json:"version"`
	Config       semanticKernelConfig `json:"config"`
	SystemPrompt string               `json:"system_prompt"`
	UserPrompt   string               `json:"user_prompt"`
	JSONSchema   json.RawMessage      `json:"json_schema"`
}

type semanticKernelConfig struct {
	Provider              string `json:"provider"`
	BaseURL               string `json:"base_url"`
	APIKey                string `json:"api_key"`
	Model                 string `json:"model"`
	ResponseFormat        string `json:"response_format"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	MaxAttempts           int    `json:"max_attempts"`
}

type semanticKernelGenerateResponse struct {
	Version      int             `json:"version"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	JSON         json.RawMessage `json:"json"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
}

type semanticKernelError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Temporary bool   `json:"temporary"`
}

func (p *SemanticKernelProvider) Generate(ctx context.Context, request ProviderRequest) (*ProviderResponse, error) {
	config, err := p.resolver.ResolveSemanticKernelConfig(ctx)
	if err != nil {
		return nil, &ProviderError{Code: "configuration_unavailable", Err: err}
	}
	body, err := json.Marshal(semanticKernelGenerateRequest{
		Version: semanticKernelProtocolVersion,
		Config: semanticKernelConfig{
			Provider: config.Provider, BaseURL: config.BaseURL, APIKey: config.APIKey,
			Model: config.Model, ResponseFormat: config.ResponseFormat,
			RequestTimeoutSeconds: config.RequestTimeoutSeconds, MaxAttempts: config.MaxAttempts,
		},
		SystemPrompt: request.SystemPrompt, UserPrompt: request.UserPrompt,
		JSONSchema: request.JSONSchema,
	})
	if err != nil {
		return nil, fmt.Errorf("ai: 构造 Semantic Kernel 请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai: 构造 Semantic Kernel HTTP 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Anby-Internal-Token", p.token)
	response, err := p.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, &ProviderError{Code: "kernel_unavailable", Temporary: true, Err: err}
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, &ProviderError{Code: "kernel_response_read", Temporary: true, Err: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var kernelErr semanticKernelError
		if json.Unmarshal(payload, &kernelErr) != nil || kernelErr.Code == "" {
			kernelErr = semanticKernelError{Code: "kernel_http_error", Message: http.StatusText(response.StatusCode)}
		}
		if response.StatusCode == http.StatusGatewayTimeout || kernelErr.Code == "timeout" {
			return nil, ErrTimeout
		}
		if kernelErr.Code == "invalid_schema" || kernelErr.Code == "invalid_structured_output" ||
			kernelErr.Code == "output_truncated" {
			return nil, &ProviderError{Code: kernelErr.Code, Err: ErrInvalidOutput}
		}
		return nil, &ProviderError{
			Code: kernelErr.Code, Temporary: kernelErr.Temporary,
			Err: fmt.Errorf("%w: %s", ErrProvider, kernelErr.Message),
		}
	}
	var value semanticKernelGenerateResponse
	if err := json.Unmarshal(payload, &value); err != nil || value.Version != semanticKernelProtocolVersion || len(value.JSON) == 0 {
		return nil, &ProviderError{Code: "kernel_invalid_response", Err: ErrInvalidOutput}
	}
	return &ProviderResponse{
		JSON: value.JSON, Provider: value.Provider, Model: value.Model,
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
	}, nil
}
