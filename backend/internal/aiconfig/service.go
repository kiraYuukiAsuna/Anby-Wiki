package aiconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type Service struct {
	repo   *Repository
	txm    *db.TxManager
	ids    *id.Generator
	auth   *governance.AuthorizationService
	cipher *credentialCipher
}

func NewService(
	repo *Repository,
	txm *db.TxManager,
	ids *id.Generator,
	auth *governance.AuthorizationService,
	masterKey string,
) (*Service, error) {
	cipher, err := newCredentialCipher(masterKey)
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, txm: txm, ids: ids, auth: auth, cipher: cipher}, nil
}

func (s *Service) Get(ctx context.Context, wikiID, actorID uuid.UUID) (*Config, error) {
	if err := s.auth.Check(ctx, actorID, wikiID, governance.ActionManage, nil); err != nil {
		return nil, err
	}
	stored, err := s.repo.Get(ctx, nil, wikiID)
	if errors.Is(err, ErrNotConfigured) {
		return defaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	return redact(stored), nil
}

func (s *Service) Runtime(ctx context.Context, wikiID uuid.UUID) (*RuntimeConfig, error) {
	stored, err := s.repo.Get(ctx, nil, wikiID)
	if err != nil {
		return nil, err
	}
	if err := validateStored(stored); err != nil {
		return nil, err
	}
	if !stored.Enabled {
		return nil, ErrDisabled
	}
	key, err := s.cipher.decrypt(stored.APIKeyCiphertext)
	if err != nil {
		return nil, err
	}
	return &RuntimeConfig{
		Enabled: true, Provider: stored.Provider, BaseURL: stored.BaseURL,
		Model: stored.Model, ResponseFormat: stored.ResponseFormat,
		RequestTimeoutSeconds: stored.RequestTimeoutSeconds,
		MaxAttempts:           stored.MaxAttempts, APIKey: key,
	}, nil
}

func (s *Service) Available(ctx context.Context, wikiID uuid.UUID) (bool, error) {
	stored, err := s.repo.Get(ctx, nil, wikiID)
	if errors.Is(err, ErrNotConfigured) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := validateStored(stored); err != nil {
		return false, err
	}
	if !stored.Enabled {
		return false, nil
	}
	if _, err := s.cipher.decrypt(stored.APIKeyCiphertext); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) Update(ctx context.Context, params UpdateParams) (*Config, error) {
	params.Provider = strings.TrimSpace(params.Provider)
	params.BaseURL = strings.TrimRight(strings.TrimSpace(params.BaseURL), "/")
	params.Model = strings.TrimSpace(params.Model)
	params.ResponseFormat = strings.TrimSpace(params.ResponseFormat)
	params.APIKey = strings.TrimSpace(params.APIKey)
	if params.WikiID == uuid.Nil || params.ActorID == uuid.Nil || validateValues(
		params.Provider, params.BaseURL, params.Model, params.ResponseFormat,
		params.RequestTimeoutSeconds, params.MaxAttempts,
	) != nil {
		return nil, ErrInvalidConfig
	}

	var result *StoredConfig
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(
			ctx, tx, params.ActorID, params.WikiID, governance.ActionManage, nil,
		); err != nil {
			return err
		}
		existing, err := s.repo.Get(ctx, tx, params.WikiID)
		if err != nil && !errors.Is(err, ErrNotConfigured) {
			return err
		}
		ciphertext := ""
		if existing != nil {
			ciphertext = existing.APIKeyCiphertext
		}
		credentialChanged := params.APIKey != ""
		if credentialChanged {
			ciphertext, err = s.cipher.encrypt(params.APIKey)
			if err != nil {
				return err
			}
		}
		if ciphertext == "" {
			return ErrCredentialMissing
		}
		now := time.Now().UTC()
		result = &StoredConfig{
			Version: 1, Enabled: params.Enabled, Provider: params.Provider,
			BaseURL: params.BaseURL, Model: params.Model,
			ResponseFormat:        params.ResponseFormat,
			RequestTimeoutSeconds: params.RequestTimeoutSeconds,
			MaxAttempts:           params.MaxAttempts, APIKeyCiphertext: ciphertext,
			UpdatedBy: params.ActorID, UpdatedAt: now,
		}
		if err := s.repo.Put(ctx, tx, params.WikiID, result); err != nil {
			return err
		}
		auditID, err := s.ids.New()
		if err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"version": result.Version, "enabled": result.Enabled,
			"provider": result.Provider, "model": result.Model,
			"response_format":         result.ResponseFormat,
			"request_timeout_seconds": result.RequestTimeoutSeconds,
			"max_attempts":            result.MaxAttempts,
			"credential_changed":      credentialChanged,
		})
		if err != nil {
			return err
		}
		return s.repo.InsertAudit(ctx, tx, auditID, params.ActorID, params.WikiID, payload)
	})
	if err != nil {
		return nil, err
	}
	return redact(result), nil
}

func defaultConfig() *Config {
	return &Config{
		Version: 1, Provider: ProviderDeepSeek,
		BaseURL: "https://api.deepseek.com", ResponseFormat: ResponseFormatJSONObject,
		RequestTimeoutSeconds: 180, MaxAttempts: 2,
	}
}

func redact(value *StoredConfig) *Config {
	updatedBy, updatedAt := value.UpdatedBy, value.UpdatedAt
	return &Config{
		Version: value.Version, Enabled: value.Enabled, Provider: value.Provider,
		BaseURL: value.BaseURL, Model: value.Model,
		ResponseFormat:        value.ResponseFormat,
		RequestTimeoutSeconds: value.RequestTimeoutSeconds,
		MaxAttempts:           value.MaxAttempts,
		APIKeyConfigured:      value.APIKeyCiphertext != "",
		UpdatedBy:             &updatedBy, UpdatedAt: &updatedAt,
	}
}

func validateStored(value *StoredConfig) error {
	if value == nil || value.Version != 1 || value.APIKeyCiphertext == "" {
		return ErrInvalidConfig
	}
	return validateValues(value.Provider, value.BaseURL, value.Model, value.ResponseFormat,
		value.RequestTimeoutSeconds, value.MaxAttempts)
}

func validateValues(provider, baseURL, model, responseFormat string, timeout, attempts int) error {
	if provider != ProviderOpenAICompatible && provider != ProviderDeepSeek {
		return ErrInvalidConfig
	}
	if model == "" || timeout < 5 || timeout > 300 || attempts < 1 || attempts > 5 {
		return ErrInvalidConfig
	}
	if responseFormat != ResponseFormatJSONObject && responseFormat != ResponseFormatJSONSchema {
		return ErrInvalidConfig
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: base_url", ErrInvalidConfig)
	}
	return nil
}
