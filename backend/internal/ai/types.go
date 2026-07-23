// Package ai 提供供应商无关的结构化模型网关与版本化 Prompt Registry（M6-T01）。
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrPromptNotFound = errors.New("ai: Prompt 不存在")
	ErrInvalidPrompt  = errors.New("ai: Prompt 非法")
	ErrProvider       = errors.New("ai: 模型供应商失败")
	ErrTimeout        = errors.New("ai: 模型调用超时")
	ErrInvalidOutput  = errors.New("ai: 结构化输出不符合 Schema")
)

type Prompt struct {
	ID           uuid.UUID
	Key          string
	Version      int
	System       string
	User         string
	OutputSchema json.RawMessage
	ContentHash  string
	Active       bool
	CreatedAt    time.Time
}

type Usage struct {
	ID            uuid.UUID
	ImportJobID   *uuid.UUID
	ImportRunID   *uuid.UUID
	Provider      string
	Model         string
	PromptKey     string
	PromptVersion int
	AttemptCount  int
	InputTokens   int
	OutputTokens  int
	Latency       time.Duration
	Status        string
	ErrorCode     string
	CreatedAt     time.Time
}

const (
	UsageSucceeded     = "succeeded"
	UsageFailed        = "failed"
	UsageTimeout       = "timeout"
	UsageInvalidOutput = "invalid_output"
)

// Request 是业务层唯一需要了解的模型请求；不暴露供应商 DTO。
type Request struct {
	Provider    string
	Model       string
	PromptKey   string
	Variables   map[string]any
	ImportJobID *uuid.UUID
	ImportRunID *uuid.UUID
}

type Result struct {
	JSON          json.RawMessage
	PromptKey     string
	PromptVersion int
	Provider      string
	Model         string
	InputTokens   int
	OutputTokens  int
}

// ProviderRequest/ProviderResponse 只存在 ai Adapter 边界内部。
type ProviderRequest struct {
	Model        string
	SystemPrompt string
	UserPrompt   string
	JSONSchema   json.RawMessage
}

type ProviderResponse struct {
	JSON         json.RawMessage
	InputTokens  int
	OutputTokens int
}

type Provider interface {
	Generate(ctx context.Context, request ProviderRequest) (*ProviderResponse, error)
}

type ProviderError struct {
	Code      string
	Temporary bool
	Err       error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "provider error: " + e.Code
}

func (e *ProviderError) Unwrap() error { return e.Err }
