package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/anby/wiki/backend/internal/platform/db"
)

// pgUniqueViolation 唯一约束违例的 SQLSTATE。
const pgUniqueViolation = "23505"

// constraintLabelPrimary 主标签部分唯一索引名（000003 迁移）。
const constraintLabelPrimary = "entity_label_primary_key"

// Repository knowledge 模块的数据访问，手写 SQL 内联以便逐行审查（ADR-0002）。
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

const entityColumns = `id, wiki_id, entity_type_id, canonical_key, status,
	merged_into_entity_id, created_by, created_at, updated_at`

func scanEntity(row pgx.Row) (*Entity, error) {
	var e Entity
	err := row.Scan(
		&e.ID, &e.WikiID, &e.EntityTypeID, &e.CanonicalKey, &e.Status,
		&e.MergedIntoEntityID, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// InsertEntity 插入实体，created_at/updated_at 由 DB 默认值回填。
// 唯一索引 entity_wiki_canonical_key_key 冲突时返回 ErrDuplicateEntityKey。
func (r *Repository) InsertEntity(ctx context.Context, tx pgx.Tx, e *Entity) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO entity (id, wiki_id, entity_type_id, canonical_key, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`,
		e.ID, e.WikiID, e.EntityTypeID, e.CanonicalKey, e.Status, e.CreatedBy,
	).Scan(&e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: wiki=%s canonical_key=%q", ErrDuplicateEntityKey, e.WikiID, e.CanonicalKey)
		}
		return fmt.Errorf("knowledge: 插入实体失败: %w", err)
	}
	return nil
}

// GetEntityByID 按 ID 查实体（含 merged/deleted，由调用方判断 Status），
// 未命中返回 ErrEntityNotFound。
func (r *Repository) GetEntityByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Entity, error) {
	e, err := scanEntity(r.q(tx).QueryRow(ctx, `
		SELECT `+entityColumns+` FROM entity WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrEntityNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询实体失败: %w", err)
	}
	return e, nil
}

// GetEntityByIDForUpdate 按 ID 查实体并加行锁。
// 标签/别名写操作先取该锁：既保证 merged 校验与后续写入原子，
// 也序列化同一实体的并发写（主标签唯一/别名去重的服务层前置检查依赖它）。
func (r *Repository) GetEntityByIDForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Entity, error) {
	e, err := scanEntity(r.q(tx).QueryRow(ctx, `
		SELECT `+entityColumns+` FROM entity WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrEntityNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 锁定实体失败: %w", err)
	}
	return e, nil
}

// GetEntityByCanonicalKey 按 (wiki_id, canonical_key) 查实体，未命中返回 ErrEntityNotFound。
func (r *Repository) GetEntityByCanonicalKey(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, canonicalKey string) (*Entity, error) {
	e, err := scanEntity(r.q(tx).QueryRow(ctx, `
		SELECT `+entityColumns+` FROM entity WHERE wiki_id = $1 AND canonical_key = $2`,
		wikiID, canonicalKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: canonical_key=%q", ErrEntityNotFound, canonicalKey)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 按 canonical_key 查询实体失败: %w", err)
	}
	return e, nil
}

// GetEntityTypeByKey 按 type_key 解析实体类型（000004 种子），未命中返回 ErrEntityTypeNotFound。
func (r *Repository) GetEntityTypeByKey(ctx context.Context, tx pgx.Tx, typeKey string) (*EntityType, error) {
	var t EntityType
	err := r.q(tx).QueryRow(ctx, `
		SELECT id, type_key, name, created_at FROM entity_type WHERE type_key = $1`, typeKey,
	).Scan(&t.ID, &t.TypeKey, &t.Name, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: type_key=%q", ErrEntityTypeNotFound, typeKey)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询实体类型失败: %w", err)
	}
	return &t, nil
}

// GetEntityTypeByID 按稳定 ID 读取实体类型（详情页只读路径）。
func (r *Repository) GetEntityTypeByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*EntityType, error) {
	var et EntityType
	err := r.q(tx).QueryRow(ctx, `
		SELECT id, type_key, name, created_at FROM entity_type WHERE id = $1`, id).
		Scan(&et.ID, &et.TypeKey, &et.Name, &et.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrEntityTypeNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询 entity_type 失败: %w", err)
	}
	return &et, nil
}

const labelColumns = `entity_id, language, label, description, is_primary`

func scanLabel(row pgx.Row) (*EntityLabel, error) {
	var l EntityLabel
	err := row.Scan(&l.EntityID, &l.Language, &l.Label, &l.Description, &l.IsPrimary)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// InsertLabel 插入标签。主标签部分唯一索引冲突返回 ErrDuplicatePrimaryLabel；
// (entity, language, label) 主键冲突返回 ErrLabelExists。
func (r *Repository) InsertLabel(ctx context.Context, tx pgx.Tx, l *EntityLabel) error {
	_, err := r.q(tx).Exec(ctx, `
		INSERT INTO entity_label (entity_id, language, label, description, is_primary)
		VALUES ($1, $2, $3, $4, $5)`,
		l.EntityID, l.Language, l.Label, l.Description, l.IsPrimary,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			if pgErr.ConstraintName == constraintLabelPrimary {
				return fmt.Errorf("%w: entity=%s language=%q", ErrDuplicatePrimaryLabel, l.EntityID, l.Language)
			}
			return fmt.Errorf("%w: entity=%s language=%q label=%q", ErrLabelExists, l.EntityID, l.Language, l.Label)
		}
		return fmt.Errorf("knowledge: 写入标签失败: %w", err)
	}
	return nil
}

// GetLabel 按 (entity_id, language, label) 查标签，未命中返回 ErrLabelNotFound。
func (r *Repository) GetLabel(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, language, label string) (*EntityLabel, error) {
	l, err := scanLabel(r.q(tx).QueryRow(ctx, `
		SELECT `+labelColumns+` FROM entity_label
		WHERE entity_id = $1 AND language = $2 AND label = $3`,
		entityID, language, label))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: entity=%s language=%q label=%q", ErrLabelNotFound, entityID, language, label)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询标签失败: %w", err)
	}
	return l, nil
}

// GetPrimaryLabel 查实体某语言的主标签，未设置返回 ErrLabelNotFound。
func (r *Repository) GetPrimaryLabel(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, language string) (*EntityLabel, error) {
	l, err := scanLabel(r.q(tx).QueryRow(ctx, `
		SELECT `+labelColumns+` FROM entity_label
		WHERE entity_id = $1 AND language = $2 AND is_primary`,
		entityID, language))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: entity=%s language=%q 无主标签", ErrLabelNotFound, entityID, language)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询主标签失败: %w", err)
	}
	return l, nil
}

// ListLabels 列出实体全部标签（按 language, label 排序，便于断言）。
func (r *Repository) ListLabels(ctx context.Context, tx pgx.Tx, entityID uuid.UUID) ([]EntityLabel, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT `+labelColumns+` FROM entity_label
		WHERE entity_id = $1 ORDER BY language, label`, entityID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询标签列表失败: %w", err)
	}
	defer rows.Close()

	labels := []EntityLabel{}
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, fmt.Errorf("knowledge: 扫描标签失败: %w", err)
		}
		labels = append(labels, *l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: 遍历标签失败: %w", err)
	}
	return labels, nil
}

// DeleteLabel 删除标签，未命中返回 ErrLabelNotFound。
func (r *Repository) DeleteLabel(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, language, label string) error {
	tag, err := r.q(tx).Exec(ctx, `
		DELETE FROM entity_label WHERE entity_id = $1 AND language = $2 AND label = $3`,
		entityID, language, label)
	if err != nil {
		return fmt.Errorf("knowledge: 删除标签失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: entity=%s language=%q label=%q", ErrLabelNotFound, entityID, language, label)
	}
	return nil
}

// ClearPrimaryLabel 取消实体某语言当前的主标签（SetPrimaryLabel 事务的第 1 步）。
func (r *Repository) ClearPrimaryLabel(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, language string) error {
	if _, err := r.q(tx).Exec(ctx, `
		UPDATE entity_label SET is_primary = false
		WHERE entity_id = $1 AND language = $2 AND is_primary`,
		entityID, language); err != nil {
		return fmt.Errorf("knowledge: 取消主标签失败: %w", err)
	}
	return nil
}

// SetLabelPrimary 把指定标签置为主标签（SetPrimaryLabel 事务的第 2 步）。
// 标签不存在返回 ErrLabelNotFound；部分唯一索引兜底并发，冲突返回 ErrDuplicatePrimaryLabel。
func (r *Repository) SetLabelPrimary(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, language, label string) error {
	tag, err := r.q(tx).Exec(ctx, `
		UPDATE entity_label SET is_primary = true
		WHERE entity_id = $1 AND language = $2 AND label = $3`,
		entityID, language, label)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: entity=%s language=%q", ErrDuplicatePrimaryLabel, entityID, language)
		}
		return fmt.Errorf("knowledge: 设置主标签失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: entity=%s language=%q label=%q", ErrLabelNotFound, entityID, language, label)
	}
	return nil
}

// CountPrimaryLabels 统计实体的主标签数（RemoveLabel 守护"至少保留一个主标签"）。
func (r *Repository) CountPrimaryLabels(ctx context.Context, tx pgx.Tx, entityID uuid.UUID) (int, error) {
	var n int
	if err := r.q(tx).QueryRow(ctx, `
		SELECT count(*) FROM entity_label WHERE entity_id = $1 AND is_primary`,
		entityID).Scan(&n); err != nil {
		return 0, fmt.Errorf("knowledge: 统计主标签失败: %w", err)
	}
	return n, nil
}

const aliasColumns = `id, entity_id, language, alias, normalized_alias, alias_type, created_at`

func scanAlias(row pgx.Row) (*EntityAlias, error) {
	var a EntityAlias
	err := row.Scan(&a.ID, &a.EntityID, &a.Language, &a.Alias, &a.NormalizedAlias, &a.AliasType, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// InsertAlias 插入别名，created_at 由 DB 默认值回填。
// (entity, normalized_alias) 无 DB 唯一约束，去重由服务层在实体行锁内前置检查。
func (r *Repository) InsertAlias(ctx context.Context, tx pgx.Tx, a *EntityAlias) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO entity_alias (id, entity_id, language, alias, normalized_alias, alias_type)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`,
		a.ID, a.EntityID, a.Language, a.Alias, a.NormalizedAlias, a.AliasType,
	).Scan(&a.CreatedAt)
	if err != nil {
		return fmt.Errorf("knowledge: 写入别名失败: %w", err)
	}
	return nil
}

// GetAliasByNormalized 按 (entity_id, normalized_alias) 查别名，未命中返回 ErrAliasNotFound。
func (r *Repository) GetAliasByNormalized(ctx context.Context, tx pgx.Tx, entityID uuid.UUID, normalizedAlias string) (*EntityAlias, error) {
	a, err := scanAlias(r.q(tx).QueryRow(ctx, `
		SELECT `+aliasColumns+` FROM entity_alias
		WHERE entity_id = $1 AND normalized_alias = $2
		ORDER BY created_at DESC
		LIMIT 1`,
		entityID, normalizedAlias))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: entity=%s alias=%q", ErrAliasNotFound, entityID, normalizedAlias)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询别名失败: %w", err)
	}
	return a, nil
}

// ListAliases 列出实体全部别名（按 created_at, id 排序，便于断言）。
func (r *Repository) ListAliases(ctx context.Context, tx pgx.Tx, entityID uuid.UUID) ([]EntityAlias, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT `+aliasColumns+` FROM entity_alias
		WHERE entity_id = $1 ORDER BY created_at, id`, entityID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询别名列表失败: %w", err)
	}
	defer rows.Close()

	aliases := []EntityAlias{}
	for rows.Next() {
		a, err := scanAlias(rows)
		if err != nil {
			return nil, fmt.Errorf("knowledge: 扫描别名失败: %w", err)
		}
		aliases = append(aliases, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: 遍历别名失败: %w", err)
	}
	return aliases, nil
}

// DeleteAlias 按 (entity_id, alias_id) 删除别名，未命中返回 ErrAliasNotFound。
func (r *Repository) DeleteAlias(ctx context.Context, tx pgx.Tx, entityID, aliasID uuid.UUID) error {
	tag, err := r.q(tx).Exec(ctx, `
		DELETE FROM entity_alias WHERE entity_id = $1 AND id = $2`,
		entityID, aliasID)
	if err != nil {
		return fmt.Errorf("knowledge: 删除别名失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: entity=%s alias_id=%s", ErrAliasNotFound, entityID, aliasID)
	}
	return nil
}

// 搜索共用的 FROM / WHERE 片段（SQL 只由本文件常量拼接，过滤值全部走参数绑定）：
// $1=wiki_id，$2=类型过滤（NULL 不过滤），$3=include_merged。
// 状态过滤：默认仅 active；include_merged 时含 merged；deleted 永远排除。
// 搜索 SQL 关联 entity_type/entity_label/entity_alias（均含 created_at），
// 实体列必须用 e. 限定避免歧义。
const (
	searchEntityColumns = `e.id, e.wiki_id, e.entity_type_id, e.canonical_key, e.status,
	e.merged_into_entity_id, e.created_by, e.created_at, e.updated_at`
	searchFrom = `
	FROM entity e
	JOIN entity_type t ON t.id = e.entity_type_id`
	searchWhere = `
	WHERE e.wiki_id = $1
	  AND ($2::text IS NULL OR t.type_key = $2)
	  AND (e.status = 'active' OR ($3::bool AND e.status = 'merged'))`
	searchTail = `
		ORDER BY e.id
		LIMIT $5`
)

// SearchExactCanonical canonical_key 精确命中。
func (r *Repository) SearchExactCanonical(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, typeKey *string, includeMerged bool, key string, limit int) ([]Entity, error) {
	return r.searchEntities(ctx, tx, `
		SELECT `+searchEntityColumns+searchFrom+searchWhere+`
		  AND e.canonical_key = $4`+searchTail,
		wikiID, typeKey, includeMerged, key, limit)
}

// SearchExactLabel 标签规范化相等命中（标签落库时已 NFC + 折叠空白，
// lower(label) 与规范化键严格可比，见 normalize.go）。
func (r *Repository) SearchExactLabel(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, typeKey *string, includeMerged bool, key string, limit int) ([]Entity, error) {
	return r.searchEntities(ctx, tx, `
		SELECT DISTINCT `+searchEntityColumns+searchFrom+`
	JOIN entity_label l ON l.entity_id = e.id AND lower(l.label) = $4`+searchWhere+searchTail,
		wikiID, typeKey, includeMerged, key, limit)
}

// SearchExactAlias normalized_alias 精确命中。
func (r *Repository) SearchExactAlias(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, typeKey *string, includeMerged bool, key string, limit int) ([]Entity, error) {
	return r.searchEntities(ctx, tx, `
		SELECT DISTINCT `+searchEntityColumns+searchFrom+`
	JOIN entity_alias a ON a.entity_id = e.id AND a.normalized_alias = $4`+searchWhere+searchTail,
		wikiID, typeKey, includeMerged, key, limit)
}

// SearchFuzzyCanonical canonical_key 前缀/包含命中（ILIKE，不引入 pg_trgm）。
func (r *Repository) SearchFuzzyCanonical(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, typeKey *string, includeMerged bool, pattern string, limit int) ([]Entity, error) {
	return r.searchEntities(ctx, tx, `
		SELECT `+searchEntityColumns+searchFrom+searchWhere+`
		  AND e.canonical_key ILIKE $4 ESCAPE '\'`+searchTail,
		wikiID, typeKey, includeMerged, pattern, limit)
}

// SearchFuzzyLabel 标签前缀/包含命中（ILIKE）。
func (r *Repository) SearchFuzzyLabel(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, typeKey *string, includeMerged bool, pattern string, limit int) ([]Entity, error) {
	return r.searchEntities(ctx, tx, `
		SELECT DISTINCT `+searchEntityColumns+searchFrom+`
	JOIN entity_label l ON l.entity_id = e.id AND l.label ILIKE $4 ESCAPE '\'`+searchWhere+searchTail,
		wikiID, typeKey, includeMerged, pattern, limit)
}

// SearchFuzzyAlias normalized_alias 前缀/包含命中（ILIKE）。
func (r *Repository) SearchFuzzyAlias(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, typeKey *string, includeMerged bool, pattern string, limit int) ([]Entity, error) {
	return r.searchEntities(ctx, tx, `
		SELECT DISTINCT `+searchEntityColumns+searchFrom+`
	JOIN entity_alias a ON a.entity_id = e.id AND a.normalized_alias ILIKE $4 ESCAPE '\'`+searchWhere+searchTail,
		wikiID, typeKey, includeMerged, pattern, limit)
}

// searchEntities 执行搜索 SQL 并扫描实体列表。
func (r *Repository) searchEntities(ctx context.Context, tx pgx.Tx, sql string, wikiID uuid.UUID, typeKey *string, includeMerged bool, match string, limit int) ([]Entity, error) {
	rows, err := r.q(tx).Query(ctx, sql, wikiID, typeKey, includeMerged, match, limit)
	if err != nil {
		return nil, fmt.Errorf("knowledge: 搜索实体失败: %w", err)
	}
	defer rows.Close()

	entities := []Entity{}
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, fmt.Errorf("knowledge: 扫描搜索结果失败: %w", err)
		}
		entities = append(entities, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: 遍历搜索结果失败: %w", err)
	}
	return entities, nil
}

// likePattern 构造 ILIKE 包含模式：转义 \ % _ 后两侧加 %。
func likePattern(key string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(key) + "%"
}

// InsertBinding 写入页面-实体绑定，created_at 由 DB 默认值回填。
// 主键 (page, entity, role) 冲突返回 ErrBindingExists。
func (r *Repository) InsertBinding(ctx context.Context, tx pgx.Tx, b *PageEntityBinding) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO page_entity_binding (page_id, entity_id, binding_role, language)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`,
		b.PageID, b.EntityID, b.Role, b.Language,
	).Scan(&b.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: page=%s entity=%s role=%q", ErrBindingExists, b.PageID, b.EntityID, b.Role)
		}
		return fmt.Errorf("knowledge: 写入绑定失败: %w", err)
	}
	return nil
}

// DeleteBinding 删除绑定，未命中返回 ErrBindingNotFound。
func (r *Repository) DeleteBinding(ctx context.Context, tx pgx.Tx, pageID, entityID uuid.UUID, role string) error {
	tag, err := r.q(tx).Exec(ctx, `
		DELETE FROM page_entity_binding
		WHERE page_id = $1 AND entity_id = $2 AND binding_role = $3`,
		pageID, entityID, role)
	if err != nil {
		return fmt.Errorf("knowledge: 删除绑定失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: page=%s entity=%s role=%q", ErrBindingNotFound, pageID, entityID, role)
	}
	return nil
}

// HasPageBinding reports whether page and entity have any active binding role.
// It is intentionally read-only so import matching can use page context without
// bypassing the Knowledge service for authoritative writes.
func (r *Repository) HasPageBinding(ctx context.Context, tx pgx.Tx, pageID, entityID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.q(tx).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM page_entity_binding
			WHERE page_id = $1 AND entity_id = $2
		)`, pageID, entityID).Scan(&exists); err != nil {
		return false, fmt.Errorf("knowledge: 查询页面实体绑定失败: %w", err)
	}
	return exists, nil
}
