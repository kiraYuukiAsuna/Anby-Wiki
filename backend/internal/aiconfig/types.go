// Package aiconfig manages the encrypted, administrator-controlled AI runtime
// configuration. Provider credentials never leave this domain in plaintext
// except for the short-lived RuntimeConfig consumed by the internal kernel adapter.
package aiconfig

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotConfigured     = errors.New("aiconfig: AI 配置不存在")
	ErrDisabled          = errors.New("aiconfig: AI 导入未启用")
	ErrInvalidConfig     = errors.New("aiconfig: AI 配置非法")
	ErrMasterKeyInvalid  = errors.New("aiconfig: 主密钥非法")
	ErrCredentialMissing = errors.New("aiconfig: API Key 未配置")
)

const (
	ProviderOpenAICompatible = "openai-compatible"
	ProviderDeepSeek         = "deepseek"

	ResponseFormatJSONObject = "json_object"
	ResponseFormatJSONSchema = "json_schema"

	DefaultMaxInputTokens = 65536
	MinMaxInputTokens     = 4096
	MaxMaxInputTokens     = 2000000
)

// StoredConfig is the versioned JSON persisted inside wiki_site.settings_json.
// APIKeyCiphertext contains nonce+ciphertext encoded as base64; plaintext is
// deliberately absent from this type.
type StoredConfig struct {
	Version               int       `json:"version"`
	Enabled               bool      `json:"enabled"`
	Provider              string    `json:"provider"`
	BaseURL               string    `json:"base_url"`
	Model                 string    `json:"model"`
	ResponseFormat        string    `json:"response_format"`
	MaxInputTokens        int       `json:"max_input_tokens"`
	RequestTimeoutSeconds int       `json:"request_timeout_seconds"`
	MaxAttempts           int       `json:"max_attempts"`
	APIKeyCiphertext      string    `json:"api_key_ciphertext"`
	UpdatedBy             uuid.UUID `json:"updated_by"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// Config is the redacted shape returned to administrators.
type Config struct {
	Version               int        `json:"version"`
	Enabled               bool       `json:"enabled"`
	Provider              string     `json:"provider"`
	BaseURL               string     `json:"base_url"`
	Model                 string     `json:"model"`
	ResponseFormat        string     `json:"response_format"`
	MaxInputTokens        int        `json:"max_input_tokens"`
	RequestTimeoutSeconds int        `json:"request_timeout_seconds"`
	MaxAttempts           int        `json:"max_attempts"`
	APIKeyConfigured      bool       `json:"api_key_configured"`
	UpdatedBy             *uuid.UUID `json:"updated_by,omitempty"`
	UpdatedAt             *time.Time `json:"updated_at,omitempty"`
}

// UpdateParams accepts a replacement configuration. A blank APIKey preserves
// an existing credential; the first save requires a non-blank value.
type UpdateParams struct {
	WikiID                uuid.UUID
	ActorID               uuid.UUID
	Enabled               bool
	Provider              string
	BaseURL               string
	Model                 string
	ResponseFormat        string
	MaxInputTokens        int
	RequestTimeoutSeconds int
	MaxAttempts           int
	APIKey                string
}

// RuntimeConfig is decrypted only at the Worker/API-to-kernel boundary. It
// must never be logged, serialized to a public response, or retained in a job.
type RuntimeConfig struct {
	Enabled               bool
	Provider              string
	BaseURL               string
	Model                 string
	ResponseFormat        string
	MaxInputTokens        int
	RequestTimeoutSeconds int
	MaxAttempts           int
	APIKey                string
}
