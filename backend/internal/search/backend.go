package search

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	BackendPostgres    = "postgres"
	BackendMeilisearch = "meilisearch"
)

type BackendConfig struct {
	Backend             string
	MeiliURL            string
	MeiliAPIKey         string
	MeiliIndex          string
	MeiliTimeout        time.Duration
	MeiliTaskTimeout    time.Duration
	SemanticEnabled     bool
	MeiliEmbedderName   string
	MeiliEmbedderSource string
	MeiliEmbedderModel  string
	MeiliEmbedderAPIKey string
}

// NewBackend constructs the configured query/index adapter.
func NewBackend(pool *pgxpool.Pool, cfg BackendConfig) (SearchAdapter, error) {
	switch cfg.Backend {
	case BackendPostgres:
		return NewPostgresAdapter(pool), nil
	case BackendMeilisearch:
		var embedderName string
		var embedderConfig map[string]any
		if cfg.SemanticEnabled {
			embedderName = strings.TrimSpace(cfg.MeiliEmbedderName)
			embedderConfig = map[string]any{
				"source":           strings.TrimSpace(cfg.MeiliEmbedderSource),
				"model":            strings.TrimSpace(cfg.MeiliEmbedderModel),
				"documentTemplate": "{{doc.display_title}}\n{{doc.body}}",
			}
			if cfg.MeiliEmbedderAPIKey != "" {
				embedderConfig["apiKey"] = cfg.MeiliEmbedderAPIKey
			}
		}
		adapter, err := NewMeilisearchAdapter(MeilisearchConfig{
			BaseURL: cfg.MeiliURL,
			APIKey:  cfg.MeiliAPIKey,
			Index:   cfg.MeiliIndex,
			HTTPClient: &http.Client{
				Timeout: cfg.MeiliTimeout,
			},
			TaskTimeout:      cfg.MeiliTaskTimeout,
			SemanticEmbedder: embedderName,
			EmbedderConfig:   embedderConfig,
		})
		if err != nil {
			return nil, err
		}
		adapter.namespaceExists = func(ctx context.Context, wikiID uuid.UUID, namespace string) (bool, error) {
			var exists bool
			err := pool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM namespace
					WHERE wiki_id = $1 AND namespace_key = $2
				)`, wikiID, namespace).Scan(&exists)
			return exists, err
		}
		return adapter, nil
	default:
		return nil, fmt.Errorf("search: unsupported backend %q", cfg.Backend)
	}
}
