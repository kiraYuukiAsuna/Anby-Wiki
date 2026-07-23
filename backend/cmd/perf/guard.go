package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	databaseMarker = "anby-wiki-performance-only"
	confirmValue   = "ANBY_WIKI_PERF_ONLY"
)

func validateDatabaseIdentity(name, marker, environment string) error {
	if !strings.HasPrefix(name, "wiki_perf_") || name == "wiki" || name == "postgres" {
		return fmt.Errorf("refusing database %q: name must start with wiki_perf_", name)
	}
	if marker != databaseMarker {
		return fmt.Errorf("refusing database %q: marker must be %q", name, databaseMarker)
	}
	if environment == "production" || environment == "prod" {
		return errors.New("refusing production database")
	}
	return nil
}

func verifyPerformanceDatabase(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var name, marker, environment string
	err := pool.QueryRow(ctx, `
		SELECT current_database(),
		       COALESCE(shobj_description(oid, 'pg_database'), ''),
		       COALESCE(current_setting('app.environment', true), '')
		FROM pg_database WHERE datname = current_database()`).
		Scan(&name, &marker, &environment)
	if err != nil {
		return "", fmt.Errorf("inspect database identity: %w", err)
	}
	if err := validateDatabaseIdentity(name, marker, environment); err != nil {
		return "", err
	}
	var pages int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM page`).Scan(&pages); err != nil {
		return "", fmt.Errorf("database is not migrated: %w", err)
	}
	if pages != 0 {
		return "", fmt.Errorf("performance database is not empty (%d pages); recreate it", pages)
	}
	return name, nil
}
