package page

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/anby/wiki/backend/internal/platform/db"
)

// pgUniqueViolation 唯一约束违例的 SQLSTATE。
const pgUniqueViolation = "23505"

// Repository Page 模块的数据访问，手写 SQL 内联以便逐行审查（ADR-0002）。
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

const pageColumns = `id, wiki_id, namespace_id, normalized_title, display_title,
	language, content_model, status, current_revision_id, primary_entity_id,
	created_by, created_at, updated_at, deleted_at`

func scanPage(row pgx.Row) (*Page, error) {
	var p Page
	err := row.Scan(
		&p.ID, &p.WikiID, &p.NamespaceID, &p.NormalizedTitle, &p.DisplayTitle,
		&p.Language, &p.ContentModel, &p.Status, &p.CurrentRevisionID, &p.PrimaryEntityID,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// InsertPage 插入页面。唯一索引 page_live_title_key 冲突时返回 ErrTitleConflict。
func (r *Repository) InsertPage(ctx context.Context, tx pgx.Tx, p *Page) error {
	_, err := r.q(tx).Exec(ctx, `
		INSERT INTO page (id, wiki_id, namespace_id, normalized_title, display_title,
			language, content_model, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		p.ID, p.WikiID, p.NamespaceID, p.NormalizedTitle, p.DisplayTitle,
		p.Language, p.ContentModel, p.Status, p.CreatedBy,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: wiki=%s namespace=%s title=%q", ErrTitleConflict, p.WikiID, p.NamespaceID, p.NormalizedTitle)
		}
		return fmt.Errorf("page: 插入页面失败: %w", err)
	}
	return nil
}

// GetPageByID 按 ID 查页面（含已软删除），未命中返回 ErrPageNotFound。
func (r *Repository) GetPageByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Page, error) {
	p, err := scanPage(r.q(tx).QueryRow(ctx, `
		SELECT `+pageColumns+` FROM page WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrPageNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("page: 查询页面失败: %w", err)
	}
	return p, nil
}

// GetPageByIDForUpdate 按 ID 查页面并加行锁（供改名事务使用）。
func (r *Repository) GetPageByIDForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Page, error) {
	p, err := scanPage(r.q(tx).QueryRow(ctx, `
		SELECT `+pageColumns+` FROM page WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrPageNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("page: 锁定页面失败: %w", err)
	}
	return p, nil
}

// GetLivePageByTitle 按规范化标题查活页面（deleted_at IS NULL），未命中返回 ErrPageNotFound。
func (r *Repository) GetLivePageByTitle(ctx context.Context, tx pgx.Tx, wikiID, namespaceID uuid.UUID, normalizedTitle string) (*Page, error) {
	p, err := scanPage(r.q(tx).QueryRow(ctx, `
		SELECT `+pageColumns+` FROM page
		WHERE wiki_id = $1 AND namespace_id = $2 AND normalized_title = $3 AND deleted_at IS NULL`,
		wikiID, namespaceID, normalizedTitle))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: title=%q", ErrPageNotFound, normalizedTitle)
	}
	if err != nil {
		return nil, fmt.Errorf("page: 按标题查询页面失败: %w", err)
	}
	return p, nil
}

// UpdatePageTitle 更新页面标题并刷新 updated_at。
func (r *Repository) UpdatePageTitle(ctx context.Context, tx pgx.Tx, id uuid.UUID, normalizedTitle, displayTitle string) error {
	tag, err := r.q(tx).Exec(ctx, `
		UPDATE page
		SET normalized_title = $2, display_title = $3, updated_at = now()
		WHERE id = $1`,
		id, normalizedTitle, displayTitle)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return fmt.Errorf("%w: title=%q", ErrTitleConflict, normalizedTitle)
		}
		return fmt.Errorf("page: 更新标题失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", ErrPageNotFound, id)
	}
	return nil
}

// InsertAlias 写入页面别名。
func (r *Repository) InsertAlias(ctx context.Context, tx pgx.Tx, a *Alias) error {
	_, err := r.q(tx).Exec(ctx, `
		INSERT INTO page_alias (id, wiki_id, namespace_id, normalized_title, page_id, alias_type)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		a.ID, a.WikiID, a.NamespaceID, a.NormalizedTitle, a.PageID, a.AliasType,
	)
	if err != nil {
		return fmt.Errorf("page: 写入别名失败: %w", err)
	}
	return nil
}

// GetAliasByTitle 按规范化标题查别名，未命中返回 errAliasNotFound。
func (r *Repository) GetAliasByTitle(ctx context.Context, tx pgx.Tx, wikiID, namespaceID uuid.UUID, normalizedTitle string) (*Alias, error) {
	var a Alias
	err := r.q(tx).QueryRow(ctx, `
		SELECT id, wiki_id, namespace_id, normalized_title, page_id, alias_type, created_at
		FROM page_alias
		WHERE wiki_id = $1 AND namespace_id = $2 AND normalized_title = $3
		ORDER BY created_at DESC
		LIMIT 1`,
		wikiID, namespaceID, normalizedTitle,
	).Scan(&a.ID, &a.WikiID, &a.NamespaceID, &a.NormalizedTitle, &a.PageID, &a.AliasType, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errAliasNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("page: 查询别名失败: %w", err)
	}
	return &a, nil
}

// DeleteAlias 删除别名（改名改回原标题时回收该页面自己的旧别名）。
func (r *Repository) DeleteAlias(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	if _, err := r.q(tx).Exec(ctx, `DELETE FROM page_alias WHERE id = $1`, id); err != nil {
		return fmt.Errorf("page: 删除别名失败: %w", err)
	}
	return nil
}

// GetNamespaceIDByKey 按 (wiki_id, namespace_key) 查命名空间 ID。
func (r *Repository) GetNamespaceIDByKey(ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, namespaceKey string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.q(tx).QueryRow(ctx, `
		SELECT id FROM namespace WHERE wiki_id = $1 AND namespace_key = $2`,
		wikiID, namespaceKey,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("%w: key=%q", ErrNamespaceNotFound, namespaceKey)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("page: 查询命名空间失败: %w", err)
	}
	return id, nil
}

// CheckWriteActor 校验写操作 Actor：存在、active、类型属于 human/bot/system。
// 不存在/停用返回 ErrInvalidActor；类型不允许（含 ai）返回 ErrActorNotAllowed。
// 导出供其他领域模块（knowledge 等）复用同一 Actor 准入规则，规则只允许维护这一份。
func (r *Repository) CheckWriteActor(ctx context.Context, tx pgx.Tx, actorID uuid.UUID) error {
	actor, err := r.GetActorByID(ctx, tx, actorID)
	if errors.Is(err, errActorNotFound) {
		return fmt.Errorf("%w: id=%s 不存在", ErrInvalidActor, actorID)
	}
	if err != nil {
		return err
	}
	if actor.Status != StatusActive {
		return fmt.Errorf("%w: id=%s 状态 %q", ErrInvalidActor, actorID, actor.Status)
	}
	if !allowedWriteActorTypes[actor.ActorType] {
		return fmt.Errorf("%w: actor_type=%q", ErrActorNotAllowed, actor.ActorType)
	}
	return nil
}

// GetActorByID 按 ID 查 Actor，未命中返回 errActorNotFound。
func (r *Repository) GetActorByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Actor, error) {
	var a Actor
	err := r.q(tx).QueryRow(ctx, `
		SELECT id, actor_type, display_name, status FROM actor WHERE id = $1`, id,
	).Scan(&a.ID, &a.ActorType, &a.DisplayName, &a.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errActorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("page: 查询 Actor 失败: %w", err)
	}
	return &a, nil
}

// UpsertRedirect 写入/覆盖站内重定向（source → target_page_id）。
func (r *Repository) UpsertRedirect(ctx context.Context, tx pgx.Tx, sourcePageID, targetPageID uuid.UUID) error {
	_, err := r.q(tx).Exec(ctx, `
		INSERT INTO page_redirect (source_page_id, target_page_id)
		VALUES ($1, $2)
		ON CONFLICT (source_page_id)
		DO UPDATE SET target_page_id = EXCLUDED.target_page_id`,
		sourcePageID, targetPageID,
	)
	if err != nil {
		return fmt.Errorf("page: 写入重定向失败: %w", err)
	}
	return nil
}

// GetRedirectTarget 查 source 的站内重定向目标。
// 无重定向记录返回 (nil, nil)；记录存在但非站内目标（target_page_id 为 NULL）
// 同样返回 (nil, nil)——跨命名空间/跨 wiki 目标不在本 Task 跟随范围。
func (r *Repository) GetRedirectTarget(ctx context.Context, tx pgx.Tx, sourcePageID uuid.UUID) (*uuid.UUID, error) {
	var target *uuid.UUID
	err := r.q(tx).QueryRow(ctx, `
		SELECT target_page_id FROM page_redirect WHERE source_page_id = $1`, sourcePageID,
	).Scan(&target)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("page: 查询重定向失败: %w", err)
	}
	return target, nil
}

// GetWikiIDBySiteKey 按 site_key 查站点 ID（API 启动时解析默认站点并缓存），
// 未命中返回 ErrWikiNotFound。
func (r *Repository) GetWikiIDBySiteKey(ctx context.Context, tx pgx.Tx, siteKey string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.q(tx).QueryRow(ctx, `
		SELECT id FROM wiki_site WHERE site_key = $1`, siteKey,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("%w: site_key=%q", ErrWikiNotFound, siteKey)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("page: 查询站点失败: %w", err)
	}
	return id, nil
}

// GetCurrentRevisionWithSnapshot 读取页面当前 Revision 及其 ContentSnapshot（M1-T06 阅读路径）。
// 页面已创建但未发布过（current_revision_id 为 NULL）返回 (nil, nil, nil)；
// 页面是否存在由调用方先查 Page 判定。
func (r *Repository) GetCurrentRevisionWithSnapshot(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) (*Revision, *ContentSnapshot, error) {
	var rev Revision
	var snap ContentSnapshot
	err := r.q(tx).QueryRow(ctx, `
		SELECT r.id, r.page_id, r.parent_revision_id, r.content_snapshot_id, r.actor_id,
			r.change_batch_id, r.summary, r.is_minor, r.visibility, r.created_at,
			s.id, s.schema_version, s.ast_json, s.content_hash, s.size_bytes
		FROM page p
		JOIN revision r ON r.id = p.current_revision_id
		JOIN content_snapshot s ON s.id = r.content_snapshot_id
		WHERE p.id = $1`, pageID,
	).Scan(
		&rev.ID, &rev.PageID, &rev.ParentRevisionID, &rev.ContentSnapshotID, &rev.ActorID, &rev.ChangeBatchID,
		&rev.Summary, &rev.IsMinor, &rev.Visibility, &rev.CreatedAt,
		&snap.ID, &snap.SchemaVersion, &snap.AST, &snap.ContentHash, &snap.SizeBytes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("page: 查询当前 Revision 失败: %w", err)
	}
	rev.ContentHash = snap.ContentHash
	rev.SchemaVersion = snap.SchemaVersion
	return &rev, &snap, nil
}

// GetRenderedHTML 查询 RenderedPage 投影（M3-T05，设计 §17.1，只读）。
// 命中条件：page_id 匹配 且 revision_id 是给定 Revision 且 renderer_version 匹配——
// 三者任一不满足都返回 ok=false（投影缺失/落后/渲染器升版），调用方实时渲染兜底。
func (r *Repository) GetRenderedHTML(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID, rendererVersion string) (html string, ok bool, err error) {
	err = r.q(tx).QueryRow(ctx, `
		SELECT html_content FROM rendered_page
		WHERE page_id = $1 AND revision_id = $2 AND renderer_version = $3`,
		pageID, revisionID, rendererVersion).Scan(&html)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("page: 查询渲染投影失败: %w", err)
	}
	return html, true, nil
}

// revisionSnapshotColumns 历史查询共用的 revision + snapshot 列（content_hash/schema_version 冗余自 snapshot）。
const revisionSnapshotColumns = `r.id, r.page_id, r.parent_revision_id, r.content_snapshot_id, r.actor_id,
	r.change_batch_id, r.summary, r.is_minor, r.visibility, r.created_at,
	s.id, s.schema_version, s.ast_json, s.content_hash, s.size_bytes`

// scanRevisionSnapshot 扫描一行 revision JOIN content_snapshot。
// snapAST 为 nil 时丢弃 AST 字节（列表场景只取元信息）。
func scanRevisionSnapshot(rev *Revision, snap *ContentSnapshot, snapAST any) []any {
	return []any{
		&rev.ID, &rev.PageID, &rev.ParentRevisionID, &rev.ContentSnapshotID, &rev.ActorID, &rev.ChangeBatchID,
		&rev.Summary, &rev.IsMinor, &rev.Visibility, &rev.CreatedAt,
		&snap.ID, &snap.SchemaVersion, snapAST, &snap.ContentHash, &snap.SizeBytes,
	}
}

// ListRevisions 按 (created_at DESC, id DESC) 游标分页列出页面 Revision（M1-T07 历史）。
// afterCreatedAt/afterID 同时为 nil 时取首页；否则取严格早于该位置的后续条目。
// 每行的 ContentHash/SchemaVersion 冗余自关联 snapshot（AST 字节不取出）。
func (r *Repository) ListRevisions(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, afterCreatedAt *time.Time, afterID *uuid.UUID, limit int) ([]Revision, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT `+revisionSnapshotColumns+`
		FROM revision r
		JOIN content_snapshot s ON s.id = r.content_snapshot_id
		WHERE r.page_id = $1
		  AND ($2::timestamptz IS NULL OR (r.created_at, r.id) < ($2::timestamptz, $3::uuid))
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT $4`,
		pageID, afterCreatedAt, afterID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("page: 查询 Revision 历史失败: %w", err)
	}
	defer rows.Close()

	revs := []Revision{}
	for rows.Next() {
		var rev Revision
		var snap ContentSnapshot
		if err := rows.Scan(scanRevisionSnapshot(&rev, &snap, new([]byte))...); err != nil {
			return nil, fmt.Errorf("page: 扫描 Revision 历史失败: %w", err)
		}
		rev.ContentHash = snap.ContentHash
		rev.SchemaVersion = snap.SchemaVersion
		revs = append(revs, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("page: 遍历 Revision 历史失败: %w", err)
	}
	return revs, nil
}

// GetRevisionWithSnapshot 读取指定页面下的单个 Revision 及其 ContentSnapshot（含 AST）。
// Revision 不存在或不属于该页面返回 ErrRevisionNotFound（不泄露跨页存在性）。
func (r *Repository) GetRevisionWithSnapshot(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) (*Revision, *ContentSnapshot, error) {
	var rev Revision
	var snap ContentSnapshot
	err := r.q(tx).QueryRow(ctx, `
		SELECT `+revisionSnapshotColumns+`
		FROM revision r
		JOIN content_snapshot s ON s.id = r.content_snapshot_id
		WHERE r.id = $1 AND r.page_id = $2`,
		revisionID, pageID,
	).Scan(scanRevisionSnapshot(&rev, &snap, &snap.AST)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, fmt.Errorf("%w: page=%s revision=%s", ErrRevisionNotFound, pageID, revisionID)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("page: 查询 Revision 失败: %w", err)
	}
	rev.ContentHash = snap.ContentHash
	rev.SchemaVersion = snap.SchemaVersion
	return &rev, &snap, nil
}

// GetSnapshotByHash 按 (content_hash, schema_version) 查重已有快照（回滚去重用，M1-T07）。
// 未命中返回 (nil, nil)；多条命中（理论上 hash 相同即内容相同）任取其一。
func (r *Repository) GetSnapshotByHash(ctx context.Context, tx pgx.Tx, contentHash string, schemaVersion int) (*ContentSnapshot, error) {
	var snap ContentSnapshot
	err := r.q(tx).QueryRow(ctx, `
		SELECT id, schema_version, ast_json, content_hash, size_bytes
		FROM content_snapshot
		WHERE content_hash = $1 AND schema_version = $2
		LIMIT 1`,
		contentHash, schemaVersion,
	).Scan(&snap.ID, &snap.SchemaVersion, &snap.AST, &snap.ContentHash, &snap.SizeBytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("page: 按 hash 查询快照失败: %w", err)
	}
	return &snap, nil
}

// InsertContentSnapshot 写入内容快照（发布事务第 1 步）。
func (r *Repository) InsertContentSnapshot(ctx context.Context, tx pgx.Tx, s *ContentSnapshot) error {
	_, err := r.q(tx).Exec(ctx, `
		INSERT INTO content_snapshot (id, schema_version, ast_json, content_hash, size_bytes)
		VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.SchemaVersion, s.AST, s.ContentHash, s.SizeBytes,
	)
	if err != nil {
		return fmt.Errorf("page: 写入内容快照失败: %w", err)
	}
	return nil
}

// InsertRevision 写入 Revision（发布事务第 2 步），created_at 由 DB 默认值回填。
func (r *Repository) InsertRevision(ctx context.Context, tx pgx.Tx, rev *Revision) error {
	err := r.q(tx).QueryRow(ctx, `
		INSERT INTO revision (id, page_id, parent_revision_id, content_snapshot_id,
			actor_id, change_batch_id, summary, is_minor, visibility)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`,
		rev.ID, rev.PageID, rev.ParentRevisionID, rev.ContentSnapshotID,
		rev.ActorID, rev.ChangeBatchID, rev.Summary, rev.IsMinor, rev.Visibility,
	).Scan(&rev.CreatedAt)
	if err != nil {
		return fmt.Errorf("page: 写入 Revision 失败: %w", err)
	}
	return nil
}

// UpdateCurrentRevision 移动页面当前 Revision 指针（发布事务第 3 步）。
// 触发器 page_current_revision_check 校验目标 Revision 属于本页面（INV-01）。
func (r *Repository) UpdateCurrentRevision(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	tag, err := r.q(tx).Exec(ctx, `
		UPDATE page SET current_revision_id = $2, updated_at = now() WHERE id = $1`,
		pageID, revisionID,
	)
	if err != nil {
		return fmt.Errorf("page: 更新当前 Revision 指针失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", ErrPageNotFound, pageID)
	}
	return nil
}

// InsertAuditEvent 写入审计事件（发布事务第 4 步）。
func (r *Repository) InsertAuditEvent(ctx context.Context, tx pgx.Tx, e *AuditEvent) error {
	_, err := r.q(tx).Exec(ctx, `
		INSERT INTO audit_event (id, actor_id, event_type, aggregate_type, aggregate_id, change_batch_id, payload_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.ID, e.ActorID, e.EventType, e.AggregateType, e.AggregateID, e.ChangeBatchID, e.Payload,
	)
	if err != nil {
		return fmt.Errorf("page: 写入审计事件失败: %w", err)
	}
	return nil
}

// InsertOutboxEvent 写入 Outbox 事件（发布事务第 5 步），
// status/next_attempt_at 使用 DB 默认值（pending / now()）。
func (r *Repository) InsertOutboxEvent(ctx context.Context, tx pgx.Tx, e *OutboxEvent) error {
	_, err := r.q(tx).Exec(ctx, `
		INSERT INTO outbox_event (id, aggregate_type, aggregate_id, event_type, payload_json)
		VALUES ($1, $2, $3, $4, $5)`,
		e.ID, e.AggregateType, e.AggregateID, e.EventType, e.Payload,
	)
	if err != nil {
		return fmt.Errorf("page: 写入 Outbox 事件失败: %w", err)
	}
	return nil
}

// SearchPages 在 wiki+namespace 内搜索活页面（M2-T04 编辑器引用选择器，只读）。
// 标题与别名两路 ILIKE 匹配（% _ \ 已由调用方转义，模式含 ESCAPE '\'）；
// 排序键 rank：规范化标题精确命中(0) > 前缀(1) > 包含(2)；
// 同一页面标题与别名同时命中时取最优 rank 的一路（matched_on: title/alias）。
// exactNorm 为规范化后的完整查询词，containsPattern/prefixPattern 为已转义的 ILIKE 模式。
func (r *Repository) SearchPages(ctx context.Context, tx pgx.Tx, wikiID, namespaceID uuid.UUID, exactNorm, containsPattern, prefixPattern string, limit int) ([]PageSearchHit, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT id, display_title, namespace_key, matched_on FROM (
			SELECT DISTINCT ON (p.id)
				p.id, p.display_title, n.namespace_key, m.matched_on, m.rank
			FROM (
				SELECT p2.id AS page_id, 'title'::text AS matched_on,
					CASE WHEN p2.normalized_title = $3 THEN 0
						WHEN p2.normalized_title ILIKE $5 ESCAPE '\' THEN 1
						ELSE 2 END AS rank
				FROM page p2
				WHERE p2.wiki_id = $1 AND p2.namespace_id = $2 AND p2.deleted_at IS NULL
					AND p2.normalized_title ILIKE $4 ESCAPE '\'
				UNION ALL
				SELECT a.page_id, 'alias'::text,
					CASE WHEN a.normalized_title = $3 THEN 0
						WHEN a.normalized_title ILIKE $5 ESCAPE '\' THEN 1
						ELSE 2 END
				FROM page_alias a
				JOIN page p3 ON p3.id = a.page_id AND p3.deleted_at IS NULL
				WHERE a.wiki_id = $1 AND a.namespace_id = $2
					AND a.normalized_title ILIKE $4 ESCAPE '\'
			) m
			JOIN page p ON p.id = m.page_id
			JOIN namespace n ON n.id = p.namespace_id
			ORDER BY p.id, m.rank, m.matched_on
		) deduped
		ORDER BY rank, lower(display_title), id
		LIMIT $6`,
		wikiID, namespaceID, exactNorm, containsPattern, prefixPattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("page: 搜索页面失败: %w", err)
	}
	defer rows.Close()

	hits := []PageSearchHit{}
	for rows.Next() {
		var h PageSearchHit
		if err := rows.Scan(&h.ID, &h.DisplayTitle, &h.NamespaceKey, &h.MatchedOn); err != nil {
			return nil, fmt.Errorf("page: 扫描搜索结果失败: %w", err)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("page: 遍历搜索结果失败: %w", err)
	}
	return hits, nil
}
