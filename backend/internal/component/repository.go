package component

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/anby/wiki/backend/internal/platform/db"
)

type Repository struct {
	pool db.Querier
}

func NewRepository(pool db.Querier) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) q(tx pgx.Tx) db.Querier {
	if tx != nil {
		return tx
	}
	return r.pool
}

func scanComponent(row pgx.Row) (*Component, error) {
	var value Component
	if err := row.Scan(&value.ID, &value.ComponentKey, &value.Name,
		&value.CreatedBy, &value.CreatedAt, &value.UpdatedAt); err != nil {
		return nil, err
	}
	return &value, nil
}

func scanVersion(row pgx.Row) (*Version, error) {
	var value Version
	if err := row.Scan(&value.ComponentID, &value.Version, &value.PropsSchema,
		&value.RendererRef, &value.Status, &value.CreatedBy,
		&value.CreatedAt, &value.PublishedAt); err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *Repository) Insert(ctx context.Context, tx pgx.Tx, value *Component) error {
	err := r.q(tx).QueryRow(ctx, `INSERT INTO component
		(id,component_key,name,created_by) VALUES ($1,$2,$3,$4)
		RETURNING created_at,updated_at`,
		value.ID, value.ComponentKey, value.Name, value.CreatedBy).
		Scan(&value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%w: key=%s", ErrDuplicateKey, value.ComponentKey)
		}
		return fmt.Errorf("component: insert: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Component, error) {
	value, err := scanComponent(r.q(tx).QueryRow(ctx, `SELECT
		id,component_key,name,created_by,created_at,updated_at
		FROM component WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrNotFound, id)
	}
	return value, err
}

func (r *Repository) Lock(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Component, error) {
	value, err := scanComponent(tx.QueryRow(ctx, `SELECT
		id,component_key,name,created_by,created_at,updated_at
		FROM component WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrNotFound, id)
	}
	return value, err
}

func (r *Repository) NextVersion(ctx context.Context, tx pgx.Tx, componentID uuid.UUID) (int, error) {
	var version int
	err := tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1
		FROM component_version WHERE component_id=$1`, componentID).Scan(&version)
	return version, err
}

func (r *Repository) InsertVersion(ctx context.Context, tx pgx.Tx, value *Version) error {
	return tx.QueryRow(ctx, `INSERT INTO component_version
		(component_id,version,props_schema_json,renderer_ref,status,created_by)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING created_at,published_at`,
		value.ComponentID, value.Version, value.PropsSchema, value.RendererRef,
		value.Status, value.CreatedBy).Scan(&value.CreatedAt, &value.PublishedAt)
}

func (r *Repository) GetVersion(
	ctx context.Context, tx pgx.Tx, componentID uuid.UUID, version int,
) (*Version, error) {
	value, err := scanVersion(r.q(tx).QueryRow(ctx, `SELECT
		component_id,version,props_schema_json,renderer_ref,status,created_by,
		created_at,published_at
		FROM component_version WHERE component_id=$1 AND version=$2`,
		componentID, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: component=%s version=%d",
			ErrVersionNotFound, componentID, version)
	}
	return value, err
}

func (r *Repository) LockVersion(
	ctx context.Context, tx pgx.Tx, componentID uuid.UUID, version int,
) (*Version, error) {
	value, err := scanVersion(tx.QueryRow(ctx, `SELECT
		component_id,version,props_schema_json,renderer_ref,status,created_by,
		created_at,published_at
		FROM component_version WHERE component_id=$1 AND version=$2 FOR UPDATE`,
		componentID, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: component=%s version=%d",
			ErrVersionNotFound, componentID, version)
	}
	return value, err
}

func (r *Repository) UpdateDraft(
	ctx context.Context, tx pgx.Tx, componentID uuid.UUID, version int,
	schema []byte, rendererRef string,
) error {
	command, err := tx.Exec(ctx, `UPDATE component_version
		SET props_schema_json=$3,renderer_ref=$4
		WHERE component_id=$1 AND version=$2 AND status='draft'`,
		componentID, version, schema, rendererRef)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrVersionFrozen
	}
	return nil
}

func (r *Repository) SetStatus(
	ctx context.Context, tx pgx.Tx, componentID uuid.UUID, version int, status string,
) error {
	var command pgconn.CommandTag
	var err error
	if status == StatusPublished {
		command, err = tx.Exec(ctx, `UPDATE component_version
			SET status='published',published_at=now()
			WHERE component_id=$1 AND version=$2 AND status='draft'`,
			componentID, version)
	} else {
		command, err = tx.Exec(ctx, `UPDATE component_version
			SET status='deprecated'
			WHERE component_id=$1 AND version=$2 AND status='published'`,
			componentID, version)
	}
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}
