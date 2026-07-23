package ai

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
)

type Repository struct{ pool db.Querier }

func NewRepository(pool db.Querier) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(tx pgx.Tx) db.Querier {
	if tx != nil {
		return tx
	}
	return r.pool
}

func scanPrompt(row pgx.Row) (*Prompt, error) {
	var p Prompt
	err := row.Scan(&p.ID, &p.Key, &p.Version, &p.System, &p.User, &p.OutputSchema,
		&p.ContentHash, &p.Active, &p.CreatedAt)
	return &p, err
}

func (r *Repository) ActivePrompt(ctx context.Context, key string) (*Prompt, error) {
	p, err := scanPrompt(r.pool.QueryRow(ctx, `SELECT id,prompt_key,version,system_template,
		user_template,output_schema_json,content_hash,active,created_at
		FROM prompt_template WHERE prompt_key=$1 AND active`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: key=%s", ErrPromptNotFound, key)
	}
	return p, err
}

func (r *Repository) PromptVersion(ctx context.Context, key string, version int) (*Prompt, error) {
	p, err := scanPrompt(r.pool.QueryRow(ctx, `SELECT id,prompt_key,version,system_template,
		user_template,output_schema_json,content_hash,active,created_at
		FROM prompt_template WHERE prompt_key=$1 AND version=$2`, key, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: key=%s version=%d", ErrPromptNotFound, key, version)
	}
	return p, err
}

func (r *Repository) InsertPrompt(ctx context.Context, tx pgx.Tx, p *Prompt) error {
	return r.q(tx).QueryRow(ctx, `INSERT INTO prompt_template
		(id,prompt_key,version,system_template,user_template,output_schema_json,content_hash,active)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,false) RETURNING created_at`,
		p.ID, p.Key, p.Version, p.System, p.User, p.OutputSchema, p.ContentHash).Scan(&p.CreatedAt)
}

func (r *Repository) ActivatePrompt(ctx context.Context, tx pgx.Tx, id uuid.UUID, key string) error {
	if _, err := r.q(tx).Exec(ctx, `UPDATE prompt_template SET active=false WHERE prompt_key=$1 AND active`, key); err != nil {
		return err
	}
	command, err := r.q(tx).Exec(ctx, `UPDATE prompt_template SET active=true WHERE id=$1 AND prompt_key=$2`, id, key)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrPromptNotFound
	}
	return nil
}

func (r *Repository) ActivatePromptIfNone(ctx context.Context, tx pgx.Tx, id uuid.UUID, key string) (bool, error) {
	command, err := r.q(tx).Exec(ctx, `UPDATE prompt_template SET active=true
		WHERE id=$1 AND prompt_key=$2
		AND NOT EXISTS (SELECT 1 FROM prompt_template WHERE prompt_key=$2 AND active)`, id, key)
	if err != nil {
		return false, err
	}
	return command.RowsAffected() == 1, nil
}

func (r *Repository) InsertUsage(ctx context.Context, u *Usage) error {
	return r.pool.QueryRow(ctx, `INSERT INTO ai_request_usage
		(id,import_job_id,import_run_id,provider,model,prompt_key,prompt_version,attempt_count,
		 input_tokens,output_tokens,latency_ms,status,error_code)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING created_at`,
		u.ID, u.ImportJobID, u.ImportRunID, u.Provider, u.Model, u.PromptKey, u.PromptVersion,
		u.AttemptCount, u.InputTokens, u.OutputTokens, u.Latency.Milliseconds(), u.Status,
		nullString(u.ErrorCode)).Scan(&u.CreatedAt)
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
