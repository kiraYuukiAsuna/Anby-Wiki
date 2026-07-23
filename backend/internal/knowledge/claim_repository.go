package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Claim 相关数据访问，约定同 repository.go（手写 SQL、可 nil 的 pgx.Tx）。

const propertyColumns = `id, property_key, name, value_type, subject_type_id, target_type_id,
	is_multivalued, schema_json, created_at`

func scanProperty(row pgx.Row) (*Property, error) {
	var p Property
	err := row.Scan(
		&p.ID, &p.PropertyKey, &p.Name, &p.ValueType, &p.SubjectTypeID, &p.TargetTypeID,
		&p.IsMultivalued, &p.SchemaJSON, &p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPropertyByKey 按 property_key 解析 Property（000004 种子），未命中返回 ErrPropertyNotFound。
func (r *Repository) GetPropertyByKey(ctx context.Context, tx pgx.Tx, propertyKey string) (*Property, error) {
	p, err := scanProperty(r.q(tx).QueryRow(ctx, `
		SELECT `+propertyColumns+` FROM property WHERE property_key = $1`, propertyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: property_key=%q", ErrPropertyNotFound, propertyKey)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询 property 失败: %w", err)
	}
	return p, nil
}

// GetPropertyByID 按 ID 查 Property（状态机单值守卫等按外键回查的场景），
// 未命中返回 ErrPropertyNotFound。
func (r *Repository) GetPropertyByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Property, error) {
	p, err := scanProperty(r.q(tx).QueryRow(ctx, `
		SELECT `+propertyColumns+` FROM property WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrPropertyNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询 property 失败: %w", err)
	}
	return p, nil
}

const claimColumns = `id, subject_entity_id, property_id, value_type, value_json, target_entity_id,
	qualifiers_json, rank, status, verification_status, valid_from, valid_to,
	origin_type, change_batch_id, created_by, created_at, superseded_by`

func scanClaim(row pgx.Row) (*Claim, error) {
	var c Claim
	err := row.Scan(
		&c.ID, &c.SubjectEntityID, &c.PropertyID, &c.ValueType, &c.ValueJSON, &c.TargetEntityID,
		&c.QualifiersJSON, &c.Rank, &c.Status, &c.VerificationStatus, &c.ValidFrom, &c.ValidTo,
		&c.OriginType, &c.ChangeBatchID, &c.CreatedBy, &c.CreatedAt, &c.SupersededBy,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// InsertClaimChangedEvent 与 Claim 权威写入同事务追加投影失效事件。
func (r *Repository) InsertClaimChangedEvent(
	ctx context.Context,
	tx pgx.Tx,
	eventID, claimID, subjectEntityID uuid.UUID,
	replacementClaimID *uuid.UUID,
) error {
	payload, err := json.Marshal(struct {
		ClaimID            uuid.UUID  `json:"claim_id"`
		SubjectEntityID    uuid.UUID  `json:"subject_entity_id"`
		ReplacementClaimID *uuid.UUID `json:"replacement_claim_id,omitempty"`
	}{claimID, subjectEntityID, replacementClaimID})
	if err != nil {
		return fmt.Errorf("knowledge: encode claim changed event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_event
			(id, aggregate_type, aggregate_id, event_type, payload_json)
		VALUES ($1, $2, $3, $4, $5)`,
		eventID, AggregateTypeClaim, claimID, OutboxEventClaimChanged, payload,
	); err != nil {
		return fmt.Errorf("knowledge: insert claim changed event: %w", err)
	}
	return nil
}

// InsertClaim 插入 claim，created_at 由 DB 默认值回填。
// 唯一约束不存在于 claim 表，多值/单值约束由服务层在 subject 行锁内前置检查。
func (r *Repository) InsertClaim(ctx context.Context, tx pgx.Tx, c *Claim) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO claim (id, subject_entity_id, property_id, value_type, value_json,
			target_entity_id, qualifiers_json, rank, status, verification_status,
			valid_from, valid_to, origin_type, change_batch_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING created_at`,
		c.ID, c.SubjectEntityID, c.PropertyID, c.ValueType, c.ValueJSON,
		c.TargetEntityID, c.QualifiersJSON, c.Rank, c.Status, c.VerificationStatus,
		c.ValidFrom, c.ValidTo, c.OriginType, c.ChangeBatchID, c.CreatedBy,
	).Scan(&c.CreatedAt)
	if err != nil {
		return fmt.Errorf("knowledge: 写入 claim 失败: %w", err)
	}
	return nil
}

// GetClaimByID 按 ID 查 claim（含全部状态，由调用方判断 Status），未命中返回 ErrClaimNotFound。
func (r *Repository) GetClaimByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Claim, error) {
	c, err := scanClaim(r.q(tx).QueryRow(ctx, `
		SELECT `+claimColumns+` FROM claim WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrClaimNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询 claim 失败: %w", err)
	}
	return c, nil
}

// GetClaimByIDForUpdate 按 ID 查 claim 并加行锁（状态机流转/Supersede 事务用），
// 未命中返回 ErrClaimNotFound。
func (r *Repository) GetClaimByIDForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Claim, error) {
	c, err := scanClaim(r.q(tx).QueryRow(ctx, `
		SELECT `+claimColumns+` FROM claim WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrClaimNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 锁定 claim 失败: %w", err)
	}
	return c, nil
}

// GetClaimPredecessor 查 superseded_by 指向给定 Claim 的旧值；无则返回 nil。
func (r *Repository) GetClaimPredecessor(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Claim, error) {
	c, err := scanClaim(r.q(tx).QueryRow(ctx, `SELECT `+claimColumns+` FROM claim WHERE superseded_by=$1 LIMIT 1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateClaimStatus 更新业务状态（状态机流转由服务层前置校验）。
func (r *Repository) UpdateClaimStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status string) error {
	tag, err := r.q(tx).Exec(ctx, `
		UPDATE claim SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("knowledge: 更新 claim 状态失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", ErrClaimNotFound, id)
	}
	return nil
}

// SetClaimSuperseded 把旧 claim 置为 superseded 并指向新 claim（SupersedeClaim 事务内）。
// superseded_by 指向同 subject+property 的 claim 这一规则由服务层保证（000003 注释）。
func (r *Repository) SetClaimSuperseded(ctx context.Context, tx pgx.Tx, oldID, newID uuid.UUID) error {
	tag, err := r.q(tx).Exec(ctx, `
		UPDATE claim SET status = $2, superseded_by = $3 WHERE id = $1`,
		oldID, ClaimStatusSuperseded, newID)
	if err != nil {
		return fmt.Errorf("knowledge: 更新 supersede 链失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", ErrClaimNotFound, oldID)
	}
	return nil
}

// UpdateClaimVerificationStatus 更新验证状态（权限由服务层前置校验）。
func (r *Repository) UpdateClaimVerificationStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, status string) error {
	tag, err := r.q(tx).Exec(ctx, `
		UPDATE claim SET verification_status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("knowledge: 更新 claim 验证状态失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", ErrClaimNotFound, id)
	}
	return nil
}

// CountPublishedClaims 统计同 subject+property 的 published claim 数（单值约束用）。
// 调用方须持有 subject 实体行锁以序列化并发创建。
func (r *Repository) CountPublishedClaims(ctx context.Context, tx pgx.Tx, subjectEntityID, propertyID uuid.UUID) (int, error) {
	var n int
	if err := r.q(tx).QueryRow(ctx, `
		SELECT count(*) FROM claim
		WHERE subject_entity_id = $1 AND property_id = $2 AND status = $3`,
		subjectEntityID, propertyID, ClaimStatusPublished).Scan(&n); err != nil {
		return 0, fmt.Errorf("knowledge: 统计 published claim 失败: %w", err)
	}
	return n, nil
}

// ListClaims 按 subject 列出 claim，可选 property/status/verification_status 过滤
// （propertyID 为 nil 不过滤），按 (created_at, id) 排序便于断言。
func (r *Repository) ListClaims(ctx context.Context, tx pgx.Tx, subjectEntityID uuid.UUID, propertyID *uuid.UUID, status, verificationStatus *string) ([]Claim, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT `+claimColumns+` FROM claim
		WHERE subject_entity_id = $1
		  AND ($2::uuid IS NULL OR property_id = $2)
		  AND ($3::text IS NULL OR status = $3)
		  AND ($4::text IS NULL OR verification_status = $4)
		ORDER BY created_at, id`,
		subjectEntityID, propertyID, status, verificationStatus)
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询 claim 列表失败: %w", err)
	}
	defer rows.Close()

	claims := []Claim{}
	for rows.Next() {
		c, err := scanClaim(rows)
		if err != nil {
			return nil, fmt.Errorf("knowledge: 扫描 claim 失败: %w", err)
		}
		claims = append(claims, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: 遍历 claim 列表失败: %w", err)
	}
	return claims, nil
}

// InsertClaimSource 写入 claim 来源，created_at 由 DB 默认值回填。
// 主键 (claim_id, citation_id) 冲突返回 ErrClaimSourceExists（幂等拒绝）。
func (r *Repository) InsertClaimSource(ctx context.Context, tx pgx.Tx, s *ClaimSource) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO claim_source (claim_id, citation_id, support_type)
		VALUES ($1, $2, $3)
		RETURNING created_at`,
		s.ClaimID, s.CitationID, s.SupportType,
	).Scan(&s.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: claim=%s citation=%s", ErrClaimSourceExists, s.ClaimID, s.CitationID)
		}
		return fmt.Errorf("knowledge: 写入 claim 来源失败: %w", err)
	}
	return nil
}

// ListClaimSources 列出 claim 的全部来源（按 created_at, citation_id 排序，便于断言）。
func (r *Repository) ListClaimSources(ctx context.Context, tx pgx.Tx, claimID uuid.UUID) ([]ClaimSource, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT claim_id, citation_id, support_type, created_at FROM claim_source
		WHERE claim_id = $1 ORDER BY created_at, citation_id`, claimID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询 claim 来源失败: %w", err)
	}
	defer rows.Close()

	sources := []ClaimSource{}
	for rows.Next() {
		var s ClaimSource
		if err := rows.Scan(&s.ClaimID, &s.CitationID, &s.SupportType, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("knowledge: 扫描 claim 来源失败: %w", err)
		}
		sources = append(sources, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: 遍历 claim 来源失败: %w", err)
	}
	return sources, nil
}
