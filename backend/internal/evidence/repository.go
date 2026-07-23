package evidence

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/anby/wiki/backend/internal/platform/db"
)

// pgUniqueViolation 唯一约束违例的 SQLSTATE。
const pgUniqueViolation = "23505"

// Repository evidence 模块的数据访问，手写 SQL 内联以便逐行审查（ADR-0002）。
// 每个方法接收可 nil 的 pgx.Tx：nil 时在连接池上自动提交执行，
// 非 nil 时加入调用方（领域服务）编排的事务。
type Repository struct {
	pool db.Querier
}

// NewRepository 创建基于连接池的 Repository。
func NewRepository(pool db.Querier) *Repository {
	return &Repository{pool: pool}
}

// q 返回本次调用实际使用的 Querier。
func (r *Repository) q(tx pgx.Tx) db.Querier {
	if tx != nil {
		return tx
	}
	return r.pool
}

const assetColumns = `id, wiki_id, name, current_revision_id, status, created_at, updated_at`

func scanAsset(row pgx.Row) (*Asset, error) {
	var a Asset
	err := row.Scan(
		&a.ID, &a.WikiID, &a.Name, &a.CurrentRevisionID, &a.Status,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetActiveAssetByName 按 (wiki_id, name) 查 active 状态的 asset，
// 未命中返回 ErrAssetNotFound。
func (r *Repository) GetActiveAssetByName(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, name string) (*Asset, error) {
	a, err := scanAsset(r.q(tx).QueryRow(ctx, `
		SELECT `+assetColumns+` FROM asset
		WHERE wiki_id = $1 AND name = $2 AND status = 'active'`, wikiID, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: wiki=%s name=%q", ErrAssetNotFound, wikiID, name)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 asset 失败: %w", err)
	}
	return a, nil
}

// GetActiveAssetByNameForUpdate 按 (wiki_id, name) 查 active asset 并加行锁，
// 用于序列化同一 asset 的并发上传；未命中返回 ErrAssetNotFound。
func (r *Repository) GetActiveAssetByNameForUpdate(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, name string) (*Asset, error) {
	a, err := scanAsset(r.q(tx).QueryRow(ctx, `
		SELECT `+assetColumns+` FROM asset
		WHERE wiki_id = $1 AND name = $2 AND status = 'active'
		FOR UPDATE`, wikiID, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: wiki=%s name=%q", ErrAssetNotFound, wikiID, name)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 锁定 asset 失败: %w", err)
	}
	return a, nil
}

// InsertAsset 插入资产，created_at/updated_at 由 DB 默认值回填。
// 部分唯一索引 asset_wiki_name_key 冲突时返回 ErrDuplicateAssetName。
func (r *Repository) InsertAsset(ctx context.Context, tx pgx.Tx, a *Asset) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO asset (id, wiki_id, name, status)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at`,
		a.ID, a.WikiID, a.Name, a.Status,
	).Scan(&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: wiki=%s name=%q", ErrDuplicateAssetName, a.WikiID, a.Name)
		}
		return fmt.Errorf("evidence: 插入 asset 失败: %w", err)
	}
	return nil
}

// SetAssetCurrentRevision 更新 asset.current_revision_id（并刷新 updated_at）。
// 上传（含去重命中）使该内容版本成为 current。
func (r *Repository) SetAssetCurrentRevision(ctx context.Context, tx pgx.Tx, assetID, revisionID uuid.UUID) error {
	if _, err := r.q(tx).Exec(ctx, `
		UPDATE asset SET current_revision_id = $2, updated_at = now()
		WHERE id = $1`, assetID, revisionID); err != nil {
		return fmt.Errorf("evidence: 更新 asset current_revision_id 失败: %w", err)
	}
	return nil
}

const assetRevisionColumns = `id, asset_id, storage_key, content_hash, mime_type,
	size_bytes, width, height, metadata_json, actor_id, created_at`

func scanAssetRevision(row pgx.Row) (*AssetRevision, error) {
	var rev AssetRevision
	err := row.Scan(
		&rev.ID, &rev.AssetID, &rev.StorageKey, &rev.ContentHash, &rev.MimeType,
		&rev.SizeBytes, &rev.Width, &rev.Height, &rev.MetadataJSON, &rev.ActorID,
		&rev.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rev, nil
}

// GetAssetRevisionByHash 查同 asset 下指定 content_hash 的最新一条 asset_revision，
// 未命中返回 ErrAssetRevisionNotFound。
func (r *Repository) GetAssetRevisionByHash(ctx context.Context, tx pgx.Tx, assetID uuid.UUID, contentHash string) (*AssetRevision, error) {
	rev, err := scanAssetRevision(r.q(tx).QueryRow(ctx, `
		SELECT `+assetRevisionColumns+` FROM asset_revision
		WHERE asset_id = $1 AND content_hash = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, assetID, contentHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: asset=%s content_hash=%s", ErrAssetRevisionNotFound, assetID, contentHash)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 asset_revision 失败: %w", err)
	}
	return rev, nil
}

// InsertAssetRevision 插入资产内容版本，created_at 由 DB 默认值回填。
// width/height/metadata_json 为 nil 时落库 NULL/'{}'。
func (r *Repository) InsertAssetRevision(ctx context.Context, tx pgx.Tx, rev *AssetRevision) error {
	metadata := rev.MetadataJSON
	if metadata == nil {
		metadata = []byte("{}")
	}
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO asset_revision (id, asset_id, storage_key, content_hash, mime_type,
			size_bytes, width, height, metadata_json, actor_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10)
		RETURNING created_at`,
		rev.ID, rev.AssetID, rev.StorageKey, rev.ContentHash, rev.MimeType,
		rev.SizeBytes, rev.Width, rev.Height, metadata, rev.ActorID,
	).Scan(&rev.CreatedAt)
	if err != nil {
		return fmt.Errorf("evidence: 插入 asset_revision 失败: %w", err)
	}
	return nil
}
