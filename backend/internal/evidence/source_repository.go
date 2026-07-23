package evidence

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// source_repository.go —— external_resource / source / source_version /
// source_chunk / citation 的数据访问（M4-T05）。事务约定与 repository.go 相同：
// 每个方法接收可 nil 的 pgx.Tx，nil 时在连接池上自动提交执行。

const externalResourceColumns = `id, original_url, normalized_url, canonical_url, domain, path,
	http_status, content_hash, status, redirect_target_id, last_checked_at, last_success_at,
	next_check_at, lease_token, consecutive_failures, created_at, updated_at`

const claimedExternalResourceColumns = `resource.id, resource.original_url, resource.normalized_url,
	resource.canonical_url, resource.domain, resource.path, resource.http_status, resource.content_hash,
	resource.status, resource.redirect_target_id, resource.last_checked_at, resource.last_success_at,
	resource.next_check_at, resource.lease_token, resource.consecutive_failures, resource.created_at, resource.updated_at`

func scanExternalResource(row pgx.Row) (*ExternalResource, error) {
	var e ExternalResource
	err := row.Scan(
		&e.ID, &e.OriginalURL, &e.NormalizedURL, &e.CanonicalURL, &e.Domain, &e.Path,
		&e.HTTPStatus, &e.ContentHash, &e.Status, &e.RedirectTargetID,
		&e.LastCheckedAt, &e.LastSuccessAt, &e.NextCheckAt, &e.LeaseToken, &e.ConsecutiveFailures,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) getExternalResource(ctx context.Context, tx pgx.Tx, where string, arg any) (*ExternalResource, error) {
	e, err := scanExternalResource(r.q(tx).QueryRow(ctx, `
		SELECT `+externalResourceColumns+` FROM external_resource WHERE `+where, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %v", ErrExternalResourceNotFound, arg)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 external_resource 失败: %w", err)
	}
	return e, nil
}

// GetExternalResourceByNormalizedURL 按 normalized_url 查 external_resource，
// 未命中返回 ErrExternalResourceNotFound。
func (r *Repository) GetExternalResourceByNormalizedURL(ctx context.Context, tx pgx.Tx, normalizedURL string) (*ExternalResource, error) {
	return r.getExternalResource(ctx, tx, `normalized_url = $1`, normalizedURL)
}

// GetExternalResourceByID 按 ID 查 external_resource，未命中返回 ErrExternalResourceNotFound。
func (r *Repository) GetExternalResourceByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*ExternalResource, error) {
	return r.getExternalResource(ctx, tx, `id = $1`, id)
}

// InsertExternalResource 插入规范化外部资源，created_at/updated_at 由 DB 默认值回填。
// normalized_url 唯一索引冲突返回 ErrExternalResourceExists（服务层转幂等返回）。
func (r *Repository) InsertExternalResource(ctx context.Context, tx pgx.Tx, e *ExternalResource) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO external_resource (id, original_url, normalized_url, domain, path, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING next_check_at, consecutive_failures, created_at, updated_at`,
		e.ID, e.OriginalURL, e.NormalizedURL, e.Domain, e.Path, e.Status,
	).Scan(&e.NextCheckAt, &e.ConsecutiveFailures, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: normalized_url=%q", ErrExternalResourceExists, e.NormalizedURL)
		}
		return fmt.Errorf("evidence: 插入 external_resource 失败: %w", err)
	}
	return nil
}

// InsertExternalResourceIfAbsent 以 normalized_url 唯一键做无异常 upsert。
// 返回 inserted=false 表示已有并发行抢先创建；ON CONFLICT 不使调用方事务失效。
func (r *Repository) InsertExternalResourceIfAbsent(ctx context.Context, tx pgx.Tx, e *ExternalResource) (bool, error) {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO external_resource (id, original_url, normalized_url, domain, path, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (normalized_url) DO NOTHING
		RETURNING next_check_at, consecutive_failures, created_at, updated_at`,
		e.ID, e.OriginalURL, e.NormalizedURL, e.Domain, e.Path, e.Status,
	).Scan(&e.NextCheckAt, &e.ConsecutiveFailures, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("evidence: upsert external_resource 失败: %w", err)
	}
	return true, nil
}

// UpdateExternalResourceStatus 更新健康检查字段并返回更新后的资源。
func (r *Repository) UpdateExternalResourceStatus(
	ctx context.Context,
	id uuid.UUID,
	status string,
	httpStatus *int32,
	contentHash, canonicalURL *string,
	redirectTargetID *uuid.UUID,
) (*ExternalResource, error) {
	resource, err := scanExternalResource(r.q(nil).QueryRow(ctx, `
		UPDATE external_resource SET
			status = $2,
			http_status = $3,
			content_hash = $4,
			canonical_url = $5,
			redirect_target_id = $6,
			last_checked_at = now(),
			last_success_at = CASE WHEN $2 IN ('ok', 'redirect') THEN now() ELSE last_success_at END,
			updated_at = now()
		WHERE id = $1
		RETURNING `+externalResourceColumns,
		id, status, httpStatus, contentHash, canonicalURL, redirectTargetID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrExternalResourceNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 更新 external_resource 状态失败: %w", err)
	}
	return resource, nil
}

// ClaimDueExternalResources atomically leases due resources. SKIP LOCKED lets
// multiple workers share the queue without probing the same row concurrently.
func (r *Repository) ClaimDueExternalResources(
	ctx context.Context,
	limit int,
	leaseSeconds int64,
	leaseToken uuid.UUID,
) ([]ExternalResource, error) {
	rows, err := r.q(nil).Query(ctx, `
		WITH due AS (
			SELECT id
			FROM external_resource
			WHERE next_check_at <= now()
			ORDER BY next_check_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE external_resource AS resource
		SET next_check_at = now() + make_interval(secs => $2),
			lease_token = $3,
			updated_at = now()
		FROM due
		WHERE resource.id = due.id
		RETURNING `+claimedExternalResourceColumns, limit, leaseSeconds, leaseToken)
	if err != nil {
		return nil, fmt.Errorf("evidence: 领取到期 external_resource 失败: %w", err)
	}
	defer rows.Close()
	var resources []ExternalResource
	for rows.Next() {
		resource, err := scanExternalResource(rows)
		if err != nil {
			return nil, fmt.Errorf("evidence: 扫描到期 external_resource 失败: %w", err)
		}
		resources = append(resources, *resource)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evidence: 遍历到期 external_resource 失败: %w", err)
	}
	return resources, nil
}

// CompleteExternalResourceCheck records one probe and replaces its lease with
// the next success interval or bounded failure backoff.
func (r *Repository) CompleteExternalResourceCheck(
	ctx context.Context,
	id uuid.UUID,
	status string,
	httpStatus *int32,
	contentHash, canonicalURL *string,
	redirectTargetID *uuid.UUID,
	failures int,
	nextSeconds int64,
	leaseToken uuid.UUID,
) (*ExternalResource, error) {
	resource, err := scanExternalResource(r.q(nil).QueryRow(ctx, `
		UPDATE external_resource SET
			status = $2,
			http_status = $3,
			content_hash = $4,
			canonical_url = $5,
			redirect_target_id = $6,
			consecutive_failures = $7,
			next_check_at = now() + make_interval(secs => $8),
			last_checked_at = now(),
			last_success_at = CASE WHEN $2 IN ('ok', 'redirect') THEN now() ELSE last_success_at END,
			updated_at = now()
		WHERE id = $1 AND lease_token = $9
		RETURNING `+externalResourceColumns,
		id, status, httpStatus, contentHash, canonicalURL, redirectTargetID, failures, nextSeconds, leaseToken))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrExternalResourceLeaseLost, id)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 完成 external_resource 检查失败: %w", err)
	}
	return resource, nil
}

// RetryExternalResourceCheck replaces a still-owned lease with a short retry.
// The token CAS prevents an expired worker from moving a newer worker's schedule.
func (r *Repository) RetryExternalResourceCheck(
	ctx context.Context,
	id, leaseToken uuid.UUID,
	nextSeconds int64,
) error {
	tag, err := r.q(nil).Exec(ctx, `
		UPDATE external_resource
		SET next_check_at = now() + make_interval(secs => $3),
			updated_at = now()
		WHERE id = $1 AND lease_token = $2`,
		id, leaseToken, nextSeconds)
	if err != nil {
		return fmt.Errorf("evidence: 重排 external_resource 检查失败: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: id=%s", ErrExternalResourceLeaseLost, id)
	}
	return nil
}

const sourceColumns = `id, source_type, external_resource_id, asset_id, title,
	author, publisher, published_at, metadata_json, created_at`

func scanSource(row pgx.Row) (*Source, error) {
	var s Source
	err := row.Scan(
		&s.ID, &s.SourceType, &s.ExternalResourceID, &s.AssetID, &s.Title,
		&s.Author, &s.Publisher, &s.PublishedAt, &s.MetadataJSON, &s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSourceByID 按 ID 查 source，未命中返回 ErrSourceNotFound。
func (r *Repository) GetSourceByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Source, error) {
	s, err := scanSource(r.q(tx).QueryRow(ctx, `
		SELECT `+sourceColumns+` FROM source WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrSourceNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 source 失败: %w", err)
	}
	return s, nil
}

// InsertSource 插入来源，created_at 由 DB 默认值回填。
// metadata_json 为 nil 时落库 '{}'。
func (r *Repository) InsertSource(ctx context.Context, tx pgx.Tx, s *Source) error {
	metadata := s.MetadataJSON
	if metadata == nil {
		metadata = []byte("{}")
	}
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO source (id, source_type, external_resource_id, asset_id, title,
			author, publisher, published_at, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
		RETURNING created_at`,
		s.ID, s.SourceType, s.ExternalResourceID, s.AssetID, s.Title,
		s.Author, s.Publisher, s.PublishedAt, metadata,
	).Scan(&s.CreatedAt)
	if err != nil {
		return fmt.Errorf("evidence: 插入 source 失败: %w", err)
	}
	return nil
}

const sourceVersionColumns = `id, source_id, version_hash, raw_asset_id, extracted_asset_id,
	fetched_at, created_at`

func scanSourceVersion(row pgx.Row) (*SourceVersion, error) {
	var v SourceVersion
	err := row.Scan(
		&v.ID, &v.SourceID, &v.VersionHash, &v.RawAssetID, &v.ExtractedAssetID,
		&v.FetchedAt, &v.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetSourceVersionByID 按 ID 查 source_version，未命中返回 ErrSourceVersionNotFound。
func (r *Repository) GetSourceVersionByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*SourceVersion, error) {
	v, err := scanSourceVersion(r.q(tx).QueryRow(ctx, `
		SELECT `+sourceVersionColumns+` FROM source_version WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrSourceVersionNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 source_version 失败: %w", err)
	}
	return v, nil
}

// GetSourceVersionByHash 按 (source_id, version_hash) 查 source_version，
// 未命中返回 ErrSourceVersionNotFound。
func (r *Repository) GetSourceVersionByHash(ctx context.Context, tx pgx.Tx, sourceID uuid.UUID, versionHash string) (*SourceVersion, error) {
	v, err := scanSourceVersion(r.q(tx).QueryRow(ctx, `
		SELECT `+sourceVersionColumns+` FROM source_version
		WHERE source_id = $1 AND version_hash = $2`, sourceID, versionHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: source=%s version_hash=%q", ErrSourceVersionNotFound, sourceID, versionHash)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 source_version 失败: %w", err)
	}
	return v, nil
}

// InsertSourceVersion 插入来源版本，created_at 由 DB 默认值回填。
// (source_id, version_hash) 唯一索引冲突返回 ErrSourceVersionExists（服务层转幂等返回）。
func (r *Repository) InsertSourceVersion(ctx context.Context, tx pgx.Tx, v *SourceVersion) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO source_version (id, source_id, version_hash, raw_asset_id, extracted_asset_id, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`,
		v.ID, v.SourceID, v.VersionHash, v.RawAssetID, v.ExtractedAssetID, v.FetchedAt,
	).Scan(&v.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: source=%s version_hash=%q", ErrSourceVersionExists, v.SourceID, v.VersionHash)
		}
		return fmt.Errorf("evidence: 插入 source_version 失败: %w", err)
	}
	return nil
}

const sourceChunkColumns = `id, source_version_id, ordinal, locator_json, text_content, text_hash, created_at`

// InsertSourceChunk 插入来源分片，created_at 由 DB 默认值回填。
func (r *Repository) InsertSourceChunk(ctx context.Context, tx pgx.Tx, c *SourceChunk) error {
	locator := c.LocatorJSON
	if locator == nil {
		locator = []byte("{}")
	}
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO source_chunk (id, source_version_id, ordinal, locator_json, text_content, text_hash)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6)
		RETURNING created_at`,
		c.ID, c.SourceVersionID, c.Ordinal, locator, c.TextContent, c.TextHash,
	).Scan(&c.CreatedAt)
	if err != nil {
		return fmt.Errorf("evidence: 插入 source_chunk 失败: %w", err)
	}
	return nil
}

// GetSourceChunkByID 按 ID 查 source_chunk，未命中返回 ErrSourceChunkNotFound。
func (r *Repository) GetSourceChunkByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*SourceChunk, error) {
	var c SourceChunk
	err := r.q(tx).QueryRow(ctx, `
		SELECT `+sourceChunkColumns+` FROM source_chunk WHERE id = $1`, id,
	).Scan(&c.ID, &c.SourceVersionID, &c.Ordinal, &c.LocatorJSON, &c.TextContent, &c.TextHash, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrSourceChunkNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 source_chunk 失败: %w", err)
	}
	return &c, nil
}

// ListSourceChunks 列出 source_version 的全部分片（按 ordinal 排序）。
func (r *Repository) ListSourceChunks(ctx context.Context, tx pgx.Tx, sourceVersionID uuid.UUID) ([]SourceChunk, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT `+sourceChunkColumns+` FROM source_chunk
		WHERE source_version_id = $1 ORDER BY ordinal`, sourceVersionID)
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 source_chunk 列表失败: %w", err)
	}
	defer rows.Close()

	chunks := []SourceChunk{}
	for rows.Next() {
		var c SourceChunk
		if err := rows.Scan(&c.ID, &c.SourceVersionID, &c.Ordinal, &c.LocatorJSON, &c.TextContent, &c.TextHash, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("evidence: 扫描 source_chunk 失败: %w", err)
		}
		chunks = append(chunks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evidence: 遍历 source_chunk 列表失败: %w", err)
	}
	return chunks, nil
}

// GetActiveAssetByID 按 ID 查 active 状态的 asset，未命中（含已软删除）返回 ErrAssetNotFound。
func (r *Repository) GetActiveAssetByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Asset, error) {
	a, err := scanAsset(r.q(tx).QueryRow(ctx, `
		SELECT `+assetColumns+` FROM asset
		WHERE id = $1 AND status = 'active'`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrAssetNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 asset 失败: %w", err)
	}
	return a, nil
}

// GetAssetRevisionByID 按 ID 查 asset_revision，未命中返回 ErrAssetRevisionNotFound。
func (r *Repository) GetAssetRevisionByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*AssetRevision, error) {
	rev, err := scanAssetRevision(r.q(tx).QueryRow(ctx, `
		SELECT `+assetRevisionColumns+` FROM asset_revision WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrAssetRevisionNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 asset_revision 失败: %w", err)
	}
	return rev, nil
}

// InsertCitation 插入证据引用，created_at 由 DB 默认值回填。citation 不可变（000007 触发器）。
func (r *Repository) InsertCitation(ctx context.Context, tx pgx.Tx, c *Citation) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO citation (id, source_version_id, source_chunk_id, locator_json,
			quotation, quotation_hash, created_by)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
		RETURNING created_at`,
		c.ID, c.SourceVersionID, c.SourceChunkID, c.LocatorJSON,
		c.Quotation, c.QuotationHash, c.CreatedBy,
	).Scan(&c.CreatedAt)
	if err != nil {
		return fmt.Errorf("evidence: 插入 citation 失败: %w", err)
	}
	return nil
}

// GetCitationByID 按稳定 ID 读取不可变 Citation。
func (r *Repository) GetCitationByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Citation, error) {
	var c Citation
	err := r.q(tx).QueryRow(ctx, `
		SELECT id, source_version_id, source_chunk_id, locator_json,
		       quotation, quotation_hash, created_by, created_at
		FROM citation WHERE id = $1`, id).Scan(
		&c.ID, &c.SourceVersionID, &c.SourceChunkID, &c.LocatorJSON,
		&c.Quotation, &c.QuotationHash, &c.CreatedBy, &c.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrCitationNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询 citation 失败: %w", err)
	}
	return &c, nil
}

// citationExists 按 ID 判断 citation 是否存在。
func (r *Repository) citationExists(ctx context.Context, tx pgx.Tx, id uuid.UUID) (bool, error) {
	var exists bool
	if err := r.q(tx).QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM citation WHERE id = $1)`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("evidence: 查询 citation 存在性失败: %w", err)
	}
	return exists, nil
}

// CitationExists 按 ID 判断 citation 是否存在（连接池上自动提交）。
// 这是 evidence 暴露给 knowledge 模块的只读接口——knowledge 侧定义
// CitationChecker 接口，本方法签名与之匹配，装配层直接注入 *Repository，
// 保持 knowledge 不 import evidence、evidence 不 import knowledge 的单向结构。
func (r *Repository) CitationExists(ctx context.Context, id uuid.UUID) (bool, error) {
	return r.citationExists(ctx, nil, id)
}
