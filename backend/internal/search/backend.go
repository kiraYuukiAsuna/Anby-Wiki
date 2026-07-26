package search

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BackendPostgres is the only supported search backend in the early stage.
// A dedicated engine adapter may be reintroduced behind SearchAdapter once
// capacity requirements justify the extra operational surface (ADR-0006).
const BackendPostgres = "postgres"

type BackendConfig struct {
	Backend string
}

// NewBackend constructs the configured query/index adapter.
func NewBackend(pool *pgxpool.Pool, cfg BackendConfig) (SearchAdapter, error) {
	switch cfg.Backend {
	case BackendPostgres:
		return NewPostgresAdapter(pool), nil
	default:
		return nil, fmt.Errorf("search: unsupported backend %q", cfg.Backend)
	}
}
