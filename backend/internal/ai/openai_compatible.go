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

const maxProviderResponseBytes = 4 << 20

// OpenAICompatibleProvider adapts the widely supported chat/completions JSON
// protocol to the provider-neutral Gateway boundary. Credentials and provider
// response bodies are deliberately never included in returned errors.
type OpenAICompatibleProvider struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewOpenAICompatibleProvider(baseURL, apiKey string, client *http.Client) (*OpenAICompatibleProvider, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, fmt.Errorf("%w: invalid provider base URL", ErrProvider)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("%w: missing provider credential", ErrProvider)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &OpenAICompatibleProvider{endpoint: parsed.String() + "/chat/completions", apiKey: apiKey, client: client}, nil
}

type chatCompletionRequest struct {
	Model          string                 `json:"model"`
	Messages       []chatMessage          `json:"messages"`
	ResponseFormat chatJSONResponseFormat `json:"response_format"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatJSONResponseFormat struct {
	Type       string         `json:"type"`
	JSONSchema chatJSONSchema `json:"json_schema"`
}

type chatJSONSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *OpenAICompatibleProvider) Generate(ctx context.Context, request ProviderRequest) (*ProviderResponse, error) {
	if strings.TrimSpace(request.Model) == "" || len(request.JSONSchema) == 0 {
		return nil, &ProviderError{Code: "invalid_request", Err: ErrProvider}
	}
	payload, err := json.Marshal(chatCompletionRequest{
		Model: request.Model,
		Messages: []chatMessage{
			{Role: "system", Content: request.SystemPrompt},
			{Role: "user", Content: request.UserPrompt},
		},
		ResponseFormat: chatJSONResponseFormat{Type: "json_schema", JSONSchema: chatJSONSchema{
			Name: "anby_structured_output", Strict: true, Schema: request.JSONSchema,
		}},
	})
	if err != nil {
		return nil, &ProviderError{Code: "encode_request", Err: ErrProvider}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, &ProviderError{Code: "build_request", Err: ErrProvider}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	response, err := p.client.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, &ProviderError{Code: "transport", Temporary: true, Err: ErrProvider}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		temporary := response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return nil, &ProviderError{Code: fmt.Sprintf("http_%d", response.StatusCode), Temporary: temporary, Err: ErrProvider}
	}
	limited := io.LimitReader(response.Body, maxProviderResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil || len(body) > maxProviderResponseBytes {
		return nil, &ProviderError{Code: "response_too_large", Err: ErrProvider}
	}
	var decoded chatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded.Choices) == 0 {
		return nil, &ProviderError{Code: "invalid_response", Err: ErrProvider}
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if !json.Valid([]byte(content)) {
		return nil, &ProviderError{Code: "invalid_json", Err: ErrInvalidOutput}
	}
	return &ProviderResponse{JSON: json.RawMessage(content), InputTokens: decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens}, nil
}
