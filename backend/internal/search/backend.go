package search

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	BackendPostgres    = "postgres"
	BackendMeilisearch = "meilisearch"
)

type BackendConfig struct {
	Backend      string
	MeiliURL     string
	MeiliAPIKey  string
	MeiliIndex   string
	MeiliTimeout time.Duration
}

// NewBackend constructs the configured query/index adapter.
func NewBackend(pool *pgxpool.Pool, cfg BackendConfig) (SearchAdapter, error) {
	switch cfg.Backend {
	case BackendPostgres:
		return NewPostgresAdapter(pool), nil
	case BackendMeilisearch:
		adapter, err := NewMeilisearchAdapter(MeilisearchConfig{
			BaseURL: cfg.MeiliURL,
			APIKey:  cfg.MeiliAPIKey,
			Index:   cfg.MeiliIndex,
			HTTPClient: &http.Client{
				Timeout: cfg.MeiliTimeout,
			},
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
