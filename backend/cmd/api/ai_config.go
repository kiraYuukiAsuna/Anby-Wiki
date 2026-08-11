package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/aiconfig"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/google/uuid"
)

type AIConfigAPI struct {
	service  *aiconfig.Service
	provider ai.Provider
	wikiID   uuid.UUID
}

func NewAIConfigAPI(service *aiconfig.Service, provider ai.Provider, wikiID uuid.UUID) *AIConfigAPI {
	return &AIConfigAPI{service: service, provider: provider, wikiID: wikiID}
}

type updateAIConfigRequest struct {
	Enabled               bool   `json:"enabled"`
	Provider              string `json:"provider"`
	BaseURL               string `json:"base_url"`
	Model                 string `json:"model"`
	ResponseFormat        string `json:"response_format"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	MaxAttempts           int    `json:"max_attempts"`
	APIKey                string `json:"api_key"`
}

type aiConfigResponse struct {
	Version               int        `json:"version"`
	Enabled               bool       `json:"enabled"`
	Provider              string     `json:"provider"`
	BaseURL               string     `json:"base_url"`
	Model                 string     `json:"model"`
	ResponseFormat        string     `json:"response_format"`
	RequestTimeoutSeconds int        `json:"request_timeout_seconds"`
	MaxAttempts           int        `json:"max_attempts"`
	APIKeyConfigured      bool       `json:"api_key_configured"`
	UpdatedBy             *uuid.UUID `json:"updated_by,omitempty"`
	UpdatedAt             *time.Time `json:"updated_at,omitempty"`
}

type aiConfigTestResponse struct {
	OK        bool   `json:"ok"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	LatencyMS int64  `json:"latency_ms"`
}

func (a *AIConfigAPI) get(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	value, err := a.service.Get(r.Context(), a.wikiID, actorID)
	if err != nil {
		aiConfigError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAIConfigResponse(value))
}

func (a *AIConfigAPI) update(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	var request updateAIConfigRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	value, err := a.service.Update(r.Context(), aiconfig.UpdateParams{
		WikiID: a.wikiID, ActorID: actorID, Enabled: request.Enabled,
		Provider: request.Provider, BaseURL: request.BaseURL, Model: request.Model,
		ResponseFormat:        request.ResponseFormat,
		RequestTimeoutSeconds: request.RequestTimeoutSeconds,
		MaxAttempts:           request.MaxAttempts, APIKey: request.APIKey,
	})
	if err != nil {
		aiConfigError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAIConfigResponse(value))
}

func (a *AIConfigAPI) test(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actorIDFrom(w, r)
	if !ok {
		return
	}
	if _, err := a.service.Get(r.Context(), a.wikiID, actorID); err != nil {
		aiConfigError(w, r, err)
		return
	}
	runtime, err := a.service.Runtime(r.Context(), a.wikiID)
	if err != nil {
		aiConfigError(w, r, err)
		return
	}
	timeout := time.Duration(runtime.RequestTimeoutSeconds*runtime.MaxAttempts+10) * time.Second
	if timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	started := time.Now()
	result, err := a.provider.Generate(ctx, ai.ProviderRequest{
		SystemPrompt: "You are a connectivity check. Return only JSON conforming exactly to the supplied schema.",
		UserPrompt:   `Return {"ok":true}.`,
		JSONSchema:   json.RawMessage(`{"type":"object","additionalProperties":false,"required":["ok"],"properties":{"ok":{"const":true}}}`),
	})
	if err != nil {
		aiConfigError(w, r, err)
		return
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal(result.JSON, &body) != nil || !body.OK {
		aiConfigError(w, r, ai.ErrInvalidOutput)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, aiConfigTestResponse{
		OK: true, Provider: result.Provider, Model: result.Model,
		LatencyMS: time.Since(started).Milliseconds(),
	})
}

func toAIConfigResponse(value *aiconfig.Config) aiConfigResponse {
	return aiConfigResponse{
		Version: value.Version, Enabled: value.Enabled, Provider: value.Provider,
		BaseURL: value.BaseURL, Model: value.Model, ResponseFormat: value.ResponseFormat,
		RequestTimeoutSeconds: value.RequestTimeoutSeconds, MaxAttempts: value.MaxAttempts,
		APIKeyConfigured: value.APIKeyConfigured,
		UpdatedBy:        value.UpdatedBy, UpdatedAt: value.UpdatedAt,
	}
}

func aiConfigError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, governance.ErrPermissionDenied):
		governanceError(w, r, err)
	case errors.Is(err, aiconfig.ErrInvalidConfig), errors.Is(err, aiconfig.ErrCredentialMissing):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
	case errors.Is(err, aiconfig.ErrNotConfigured), errors.Is(err, aiconfig.ErrDisabled):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "请先保存并启用 AI 配置")
	case errors.Is(err, ai.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		httpx.WriteError(w, r, http.StatusGatewayTimeout, httpx.CodeInternal, "模型配置测试超时")
	case errors.Is(err, ai.ErrProvider), errors.Is(err, ai.ErrInvalidOutput):
		httpx.WriteError(w, r, http.StatusBadGateway, httpx.CodeInternal, "模型配置测试失败")
	default:
		serviceError(w, r, err)
	}
}
