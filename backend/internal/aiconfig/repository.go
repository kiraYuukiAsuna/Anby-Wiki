package aiconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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

func (r *Repository) Get(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID) (*StoredConfig, error) {
	var raw []byte
	query := `SELECT settings_json->'ai_runtime' FROM wiki_site WHERE id=$1`
	if tx != nil {
		query += ` FOR UPDATE`
	}
	err := r.q(tx).QueryRow(ctx, query, wikiID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotConfigured
	}
	if err != nil {
		return nil, fmt.Errorf("aiconfig: 读取站点设置失败: %w", err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ErrNotConfigured
	}
	var value StoredConfig
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("aiconfig: 已存配置损坏: %w", err)
	}
	// Version 1 configurations written before max_input_tokens existed remain
	// valid and adopt the conservative default without an eager data rewrite.
	if value.MaxInputTokens == 0 {
		value.MaxInputTokens = DefaultMaxInputTokens
	}
	return &value, nil
}

func (r *Repository) Put(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, value *StoredConfig) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("aiconfig: 序列化配置失败: %w", err)
	}
	tag, err := r.q(tx).Exec(ctx, `UPDATE wiki_site
		SET settings_json=jsonb_set(settings_json,'{ai_runtime}',$2::jsonb,true)
		WHERE id=$1`, wikiID, raw)
	if err != nil {
		return fmt.Errorf("aiconfig: 保存站点设置失败: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidConfig
	}
	return nil
}

func (r *Repository) InsertAudit(
	ctx context.Context,
	tx pgx.Tx,
	id, actorID, wikiID uuid.UUID,
	payload []byte,
) error {
	_, err := r.q(tx).Exec(ctx, `INSERT INTO audit_event
		(id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
		VALUES ($1,$2,'ai_runtime_config_updated','wiki_site',$3,$4::jsonb)`,
		id, actorID, wikiID, payload,
	)
	if err != nil {
		return fmt.Errorf("aiconfig: 写入审计事件失败: %w", err)
	}
	return nil
}
