// 投影查询服务（M3-T03，设计 §17.3：关系查询必须走投影表，不扫 AST）。
// 只读路径：匿名阅读端点经本服务查 page_link_projection（反链）与
// document_outline_projection/page_anchor（目录）。
package projection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/page"
)

// ErrReferenceTargetNotFound 知识/证据反向查询目标不存在。
var (
	ErrReferenceTargetNotFound = errors.New("projection: 引用目标不存在")
	ErrSectionNotFound         = errors.New("projection: 章节分片不存在")
)

// Backlink 一条反向链接：指向目标页的已解析引用来源。
type Backlink struct {
	SourcePageID  uuid.UUID
	SourceTitle   string
	SourceBlockID uuid.UUID
	DisplayText   string
}

// BacklinkPage 一页反链（按 source_page_id, source_block_id, source_node_id 升序）。
// NextCursor 为 nil 表示没有更多。
type BacklinkPage struct {
	Items      []Backlink
	NextCursor *string
}

// OutlineItem 目录条目：heading 层级、纯文本标题、锚点 slug 与序号路径。
type OutlineItem struct {
	HeadingBlockID       uuid.UUID
	ParentHeadingBlockID *uuid.UUID
	Level                int
	Title                string
	Slug                 string
	PositionKey          string
}

// SectionSummary is one non-overlapping lazy-delivery chunk.
type SectionSummary struct {
	Key            string
	Position       int
	HeadingBlockID *uuid.UUID
	Level          *int
	Title          string
	SizeBytes      int
}

// SectionManifest is ready only when every row belongs to the Page's current
// Revision and was built by the current renderer.
type SectionManifest struct {
	Ready           bool
	RevisionID      uuid.UUID
	RendererVersion string
	CitationOrder   []string
	Items           []SectionSummary
}

// RenderedSection carries one AST fragment and its safe server-rendered cache.
type RenderedSection struct {
	SectionSummary
	RevisionID      uuid.UUID
	AST             json.RawMessage
	HTML            string
	RendererVersion string
}

// ReferenceUsage 是 Entity/Claim/Citation 引用在页面 Current Revision 中的位置。
type ReferenceUsage struct {
	PageID      uuid.UUID
	PageTitle   string
	RevisionID  uuid.UUID
	BlockID     uuid.UUID
	NodeID      string
	MentionText string
	ClaimID     *uuid.UUID
}

// ReferenceUsagePage 是一页知识/证据反向使用位置。
type ReferenceUsagePage struct {
	Items      []ReferenceUsage
	NextCursor *string
}

// SourceUsage is one current Page whose Revision cites a Source. Counts keep
// the backlink summary compact without discarding occurrence-level audit data.
type SourceUsage struct {
	PageID        uuid.UUID
	PageTitle     string
	RevisionID    uuid.UUID
	UsageCount    int64
	BlockCount    int64
	CitationCount int64
}

type SourceUsagePage struct {
	Items              []SourceUsage
	NextCursor         *string
	TotalUsageCount    int64
	TotalPageCount     int64
	TotalBlockCount    int64
	TotalCitationCount int64
}

// SourceUsageLocation is one auditable CitationReference occurrence inside a
// selected current Page. It is fetched only when the grouped Page is expanded.
type SourceUsageLocation struct {
	BlockID    uuid.UUID
	NodeID     string
	CitationID uuid.UUID
	ClaimID    *uuid.UUID
}

type SourceUsageLocationPage struct {
	Items      []SourceUsageLocation
	NextCursor *string
}

// ComponentUsage is one current ComponentBlock dependency. ComponentVersion
// and EntityID make the rendered dependency explicit without scanning AST.
type ComponentUsage struct {
	PageID           uuid.UUID
	PageTitle        string
	RevisionID       uuid.UUID
	BlockID          uuid.UUID
	ComponentVersion int
	EntityID         uuid.UUID
}

type ComponentUsagePage struct {
	Items      []ComponentUsage
	NextCursor *string
}

// Queries 投影只读查询服务（阅读 API 使用，与 Worker 侧 Builder 解耦）。
type Queries struct {
	pool *pgxpool.Pool
}

// NewQueries 装配投影查询服务。
func NewQueries(pool *pgxpool.Pool) *Queries {
	return &Queries{pool: pool}
}

// backlinkRow 扫描用内部行：Backlink + 行键 source_node_id（游标用，不透出 API）。
type backlinkRow struct {
	Backlink
	sourceNodeID string
}

// Backlinks 游标分页列出指向 pageID 的已解析引用（反链，设计 §17.3）：
// JOIN page 取来源页标题并排除软删除来源页。页面不存在返回 page.ErrPageNotFound，
// 游标无法解析返回 page.ErrInvalidCursor；limit 边界同历史列表。
func (q *Queries) Backlinks(
	ctx context.Context,
	wikiID, pageID uuid.UUID,
	cursor string,
	limit int,
) (*BacklinkPage, error) {
	if err := q.ensurePageExists(ctx, pageID); err != nil {
		return nil, err
	}
	var after *backlinkRow
	if cursor != "" {
		c, err := decodeBacklinkCursor(cursor)
		if err != nil {
			return nil, err
		}
		after = &c
	}
	if limit <= 0 {
		limit = page.DefaultHistoryPageSize
	}
	if limit > page.MaxHistoryPageSize {
		limit = page.MaxHistoryPageSize
	}

	// 多取一条判断是否还有下一页；$2..$4 为游标下界（首页传 NULL）。
	var afterPageID, afterBlockID *uuid.UUID
	var afterNodeID *string
	if after != nil {
		afterPageID = &after.SourcePageID
		afterBlockID = &after.SourceBlockID
		afterNodeID = &after.sourceNodeID
	}
	rows, err := q.pool.Query(ctx, `
		SELECT p.id, p.display_title, l.source_block_id, l.source_node_id, l.display_text
		FROM page_link_projection l
		JOIN page p ON p.id = l.source_page_id
		WHERE l.target_page_id = $1
		  AND l.resolution_status = 'resolved'
		  AND p.wiki_id = $2
		  AND p.deleted_at IS NULL
		  AND ($3::uuid IS NULL
		       OR (l.source_page_id, l.source_block_id, l.source_node_id) > ($3, $4, $5))
		ORDER BY l.source_page_id, l.source_block_id, l.source_node_id
		LIMIT $6`,
		pageID, wikiID, afterPageID, afterBlockID, afterNodeID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("projection: 查询反链失败: %w", err)
	}
	defer rows.Close()

	var scanned []backlinkRow
	for rows.Next() {
		var r backlinkRow
		if err := rows.Scan(&r.SourcePageID, &r.SourceTitle, &r.SourceBlockID,
			&r.sourceNodeID, &r.DisplayText); err != nil {
			return nil, fmt.Errorf("projection: 扫描反链行失败: %w", err)
		}
		scanned = append(scanned, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: 迭代反链行失败: %w", err)
	}

	result := &BacklinkPage{Items: make([]Backlink, 0, min(len(scanned), limit))}
	if len(scanned) > limit {
		last := scanned[limit-1]
		cursor := encodeBacklinkCursor(last)
		result.NextCursor = &cursor
		scanned = scanned[:limit]
	}
	for _, r := range scanned {
		result.Items = append(result.Items, r.Backlink)
	}
	return result, nil
}

// Outline 读取页面目录（document_outline_projection JOIN page_anchor 取 slug，
// 两表由 OutlineBuilder 同事务写入）。按 position_key 数值序（'1.10' 排在 '1.2' 后）。
// 页面不存在返回 page.ErrPageNotFound；页面未发布过返回空目录。
func (q *Queries) Outline(ctx context.Context, pageID uuid.UUID) ([]OutlineItem, error) {
	if err := q.ensurePageExists(ctx, pageID); err != nil {
		return nil, err
	}
	rows, err := q.pool.Query(ctx, `
		SELECT o.heading_block_id, o.parent_heading_block_id, o.level, o.title,
		       a.current_slug, o.position_key
		FROM document_outline_projection o
		JOIN page_anchor a ON a.page_id = o.page_id AND a.heading_block_id = o.heading_block_id
		WHERE o.page_id = $1
		ORDER BY string_to_array(o.position_key, '.')::int[]`, pageID)
	if err != nil {
		return nil, fmt.Errorf("projection: 查询目录失败: %w", err)
	}
	defer rows.Close()

	items := []OutlineItem{}
	for rows.Next() {
		var it OutlineItem
		if err := rows.Scan(&it.HeadingBlockID, &it.ParentHeadingBlockID, &it.Level,
			&it.Title, &it.Slug, &it.PositionKey); err != nil {
			return nil, fmt.Errorf("projection: 扫描目录行失败: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: 迭代目录行失败: %w", err)
	}
	return items, nil
}

// Sections returns the current lazy-delivery manifest. A missing or stale
// disposable projection is Ready=false, allowing callers to fall back to the
// authoritative full ContentSnapshot.
func (q *Queries) Sections(
	ctx context.Context,
	pageID uuid.UUID,
	rendererVersion string,
) (*SectionManifest, error) {
	if err := q.ensurePageExists(ctx, pageID); err != nil {
		return nil, err
	}
	rows, err := q.pool.Query(ctx, `
		SELECT rs.revision_id,rs.renderer_version,rs.section_key,rs.position,
			rs.heading_block_id,rs.level,rs.title,rs.size_bytes,rs.citation_order
		FROM page p
		JOIN rendered_section rs
		  ON rs.page_id=p.id AND rs.revision_id=p.current_revision_id
		WHERE p.id=$1 AND p.deleted_at IS NULL
		  AND rs.renderer_version=$2
		ORDER BY rs.position`,
		pageID, rendererVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("projection: query section manifest: %w", err)
	}
	defer rows.Close()

	result := &SectionManifest{Items: []SectionSummary{}, CitationOrder: []string{}}
	var citationRaw []byte
	for rows.Next() {
		var item SectionSummary
		var revisionID uuid.UUID
		var version string
		var raw []byte
		if err := rows.Scan(
			&revisionID, &version, &item.Key, &item.Position,
			&item.HeadingBlockID, &item.Level, &item.Title,
			&item.SizeBytes, &raw,
		); err != nil {
			return nil, fmt.Errorf("projection: scan section manifest: %w", err)
		}
		if !result.Ready {
			result.Ready = true
			result.RevisionID = revisionID
			result.RendererVersion = version
			citationRaw = raw
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: iterate section manifest: %w", err)
	}
	if result.Ready {
		if err := json.Unmarshal(citationRaw, &result.CitationOrder); err != nil {
			return nil, fmt.Errorf("projection: decode section citation order: %w", err)
		}
	}
	return result, nil
}

// Section returns one current section. Stale renderer/revision rows are treated
// as unavailable and never leak into the read path.
func (q *Queries) Section(
	ctx context.Context,
	pageID uuid.UUID,
	sectionKey, rendererVersion string,
) (*RenderedSection, error) {
	if err := q.ensurePageExists(ctx, pageID); err != nil {
		return nil, err
	}
	var item RenderedSection
	err := q.pool.QueryRow(ctx, `
		SELECT rs.revision_id,rs.section_key,rs.position,rs.heading_block_id,
			rs.level,rs.title,rs.size_bytes,rs.ast_json,rs.html_content,
			rs.renderer_version
		FROM page p
		JOIN rendered_section rs
		  ON rs.page_id=p.id AND rs.revision_id=p.current_revision_id
		WHERE p.id=$1 AND p.deleted_at IS NULL
		  AND rs.section_key=$2 AND rs.renderer_version=$3`,
		pageID, sectionKey, rendererVersion,
	).Scan(
		&item.RevisionID, &item.Key, &item.Position,
		&item.HeadingBlockID, &item.Level, &item.Title,
		&item.SizeBytes, &item.AST, &item.HTML, &item.RendererVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("projection: query section %q: %w", sectionKey, err)
	}
	return &item, nil
}

// LocateSection resolves any stable Block ID (including nested blocks) to its
// containing top-level delivery section.
func (q *Queries) LocateSection(
	ctx context.Context,
	pageID, blockID uuid.UUID,
	rendererVersion string,
) (string, error) {
	if err := q.ensurePageExists(ctx, pageID); err != nil {
		return "", err
	}
	var key string
	err := q.pool.QueryRow(ctx, `
		SELECT b.section_key
		FROM page p
		JOIN rendered_section_block b
		  ON b.page_id=p.id AND b.revision_id=p.current_revision_id
		JOIN rendered_section rs
		  ON rs.page_id=b.page_id AND rs.section_key=b.section_key
		     AND rs.revision_id=b.revision_id
		WHERE p.id=$1 AND p.deleted_at IS NULL AND b.block_id=$2
		  AND rs.renderer_version=$3`,
		pageID, blockID, rendererVersion,
	).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSectionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("projection: locate section for block %s: %w", blockID, err)
	}
	return key, nil
}

// EntityMentions 返回正文中引用 entityID 的位置；结果携带来源 Revision。
func (q *Queries) EntityMentions(
	ctx context.Context,
	wikiID, entityID uuid.UUID,
	cursor string,
	limit int,
) (*ReferenceUsagePage, error) {
	return q.referenceUsages(ctx, wikiID, "entity", entityID, cursor, limit)
}

// ClaimUsages 返回正文中引用 claimID 的位置；结果携带来源 Revision。
func (q *Queries) ClaimUsages(
	ctx context.Context,
	wikiID, claimID uuid.UUID,
	cursor string,
	limit int,
) (*ReferenceUsagePage, error) {
	return q.referenceUsages(ctx, wikiID, "claim", claimID, cursor, limit)
}

// CitationUsages 返回正文中引用 citationID 的位置；结果携带来源 Revision。
func (q *Queries) CitationUsages(
	ctx context.Context,
	wikiID, citationID uuid.UUID,
	cursor string,
	limit int,
) (*ReferenceUsagePage, error) {
	return q.referenceUsages(ctx, wikiID, "citation", citationID, cursor, limit)
}

// SourceUsages groups current CitationReference occurrences by Page. Totals
// always describe the complete Source backlink set, not only the loaded page.
func (q *Queries) SourceUsages(
	ctx context.Context,
	wikiID, sourceID uuid.UUID,
	cursor string,
	limit int,
) (*SourceUsagePage, error) {
	if err := q.ensureReferenceTargetExists(
		ctx, wikiID, "source", sourceID,
	); err != nil {
		return nil, err
	}
	afterPageID, err := sourceUsageCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = usageLimit(limit)
	result := &SourceUsagePage{Items: []SourceUsage{}}
	if err := q.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT u.page_id),
		       COUNT(DISTINCT (u.page_id, u.block_id)),
		       COUNT(DISTINCT u.citation_id)
		FROM citation_usage u
		JOIN citation c ON c.id=u.citation_id
		JOIN source_version sv ON sv.id=c.source_version_id
		JOIN page p ON p.id=u.page_id
		WHERE sv.source_id=$1 AND p.deleted_at IS NULL
		  AND p.wiki_id=$2
		  AND p.current_revision_id=u.revision_id`, sourceID, wikiID).Scan(
		&result.TotalUsageCount, &result.TotalPageCount,
		&result.TotalBlockCount, &result.TotalCitationCount,
	); err != nil {
		return nil, fmt.Errorf("projection: 统计 Source 使用位置失败: %w", err)
	}
	rows, err := q.pool.Query(ctx, `
		SELECT p.id,p.display_title,u.revision_id,COUNT(*),
		       COUNT(DISTINCT u.block_id),COUNT(DISTINCT u.citation_id)
		FROM citation_usage u
		JOIN citation c ON c.id=u.citation_id
		JOIN source_version sv ON sv.id=c.source_version_id
		JOIN page p ON p.id=u.page_id
		WHERE sv.source_id=$1 AND p.deleted_at IS NULL
		  AND p.wiki_id=$2
		  AND p.current_revision_id=u.revision_id
		  AND ($3::uuid IS NULL OR u.page_id>$3)
		GROUP BY p.id,p.display_title,u.revision_id
		ORDER BY p.id LIMIT $4`, sourceID, wikiID, afterPageID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("projection: 查询 Source 页面聚合失败: %w", err)
	}
	defer rows.Close()
	scanned := []SourceUsage{}
	for rows.Next() {
		var item SourceUsage
		if err := rows.Scan(
			&item.PageID, &item.PageTitle, &item.RevisionID, &item.UsageCount,
			&item.BlockCount, &item.CitationCount,
		); err != nil {
			return nil, fmt.Errorf("projection: 扫描 Source 页面聚合失败: %w", err)
		}
		scanned = append(scanned, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: 遍历 Source 页面聚合失败: %w", err)
	}
	if len(scanned) > limit {
		last := scanned[limit-1]
		next := encodeSourceUsageCursor(last.PageID)
		result.NextCursor = &next
		scanned = scanned[:limit]
	}
	result.Items = append(result.Items, scanned...)
	return result, nil
}

// SourceUsageLocations returns occurrence-level audit details for one Page in
// a Source usage summary. The nested page is current-Wiki scoped before query.
func (q *Queries) SourceUsageLocations(
	ctx context.Context,
	wikiID, sourceID, pageID uuid.UUID,
	cursor string,
	limit int,
) (*SourceUsageLocationPage, error) {
	if err := q.ensureReferenceTargetExists(ctx, wikiID, "source", sourceID); err != nil {
		return nil, err
	}
	if err := q.ensureWikiPageExists(ctx, wikiID, pageID); err != nil {
		return nil, err
	}
	afterPageID, afterBlockID, afterNodeID, err := usageCursor(cursor)
	if err != nil {
		return nil, err
	}
	if afterPageID != nil && *afterPageID != pageID {
		return nil, fmt.Errorf("%w: Source 使用位置游标页面不匹配", page.ErrInvalidCursor)
	}
	limit = usageLimit(limit)
	rows, err := q.pool.Query(ctx, `
		SELECT u.block_id,u.node_id,u.citation_id,u.claim_id
		FROM citation_usage u
		JOIN citation c ON c.id=u.citation_id
		JOIN source_version sv ON sv.id=c.source_version_id
		JOIN page p ON p.id=u.page_id
		WHERE sv.source_id=$1 AND u.page_id=$2
		  AND p.wiki_id=$3 AND p.deleted_at IS NULL
		  AND p.current_revision_id=u.revision_id
		  AND ($4::uuid IS NULL OR (u.block_id,u.node_id)>($4,$5))
		ORDER BY u.block_id,u.node_id LIMIT $6`,
		sourceID, pageID, wikiID, afterBlockID, afterNodeID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("projection: 查询 Source 页面使用明细失败: %w", err)
	}
	defer rows.Close()
	scanned := []SourceUsageLocation{}
	for rows.Next() {
		var item SourceUsageLocation
		if err := rows.Scan(&item.BlockID, &item.NodeID, &item.CitationID, &item.ClaimID); err != nil {
			return nil, fmt.Errorf("projection: 扫描 Source 页面使用明细失败: %w", err)
		}
		scanned = append(scanned, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: 遍历 Source 页面使用明细失败: %w", err)
	}
	result := &SourceUsageLocationPage{
		Items: make([]SourceUsageLocation, 0, min(len(scanned), limit)),
	}
	if len(scanned) > limit {
		last := scanned[limit-1]
		next := encodeUsageCursor(pageID, last.BlockID, last.NodeID)
		result.NextCursor = &next
		scanned = scanned[:limit]
	}
	result.Items = append(result.Items, scanned...)
	return result, nil
}

// ComponentUsages returns current ComponentBlock dependencies for a Component,
// optionally constrained to one immutable component version.
func (q *Queries) ComponentUsages(
	ctx context.Context,
	wikiID, componentID uuid.UUID,
	version *int,
	cursor string,
	limit int,
) (*ComponentUsagePage, error) {
	if err := q.ensureReferenceTargetExists(
		ctx, wikiID, "component", componentID,
	); err != nil {
		return nil, err
	}
	if version != nil && *version <= 0 {
		return nil, fmt.Errorf("%w: component version 非法", page.ErrInvalidCursor)
	}
	afterPageID, afterBlockID, afterNodeID, err := usageCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = usageLimit(limit)
	rows, err := q.pool.Query(ctx, `
		SELECT p.id,p.display_title,d.revision_id,d.block_id,
		       d.component_version,d.entity_id
		FROM component_dependency d
		JOIN page p ON p.id=d.page_id
		WHERE d.component_id=$1
		  AND ($2::integer IS NULL OR d.component_version=$2)
		  AND p.wiki_id=$3
		  AND p.deleted_at IS NULL
		  AND p.current_revision_id=d.revision_id
		  AND ($4::uuid IS NULL
		       OR (d.page_id,d.block_id,d.component_version::text)>($4,$5,$6))
		ORDER BY d.page_id,d.block_id,d.component_version::text LIMIT $7`,
		componentID, version, wikiID, afterPageID, afterBlockID, afterNodeID,
		limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("projection: 查询 Component 使用位置失败: %w", err)
	}
	defer rows.Close()
	scanned := []ComponentUsage{}
	for rows.Next() {
		var item ComponentUsage
		if err := rows.Scan(
			&item.PageID, &item.PageTitle, &item.RevisionID,
			&item.BlockID, &item.ComponentVersion, &item.EntityID,
		); err != nil {
			return nil, fmt.Errorf("projection: 扫描 Component 使用位置失败: %w", err)
		}
		scanned = append(scanned, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: 遍历 Component 使用位置失败: %w", err)
	}
	result := &ComponentUsagePage{
		Items: make([]ComponentUsage, 0, min(len(scanned), limit)),
	}
	if len(scanned) > limit {
		last := scanned[limit-1]
		next := encodeUsageCursor(
			last.PageID, last.BlockID,
			fmt.Sprintf("%d", last.ComponentVersion),
		)
		result.NextCursor = &next
		scanned = scanned[:limit]
	}
	result.Items = append(result.Items, scanned...)
	return result, nil
}

func (q *Queries) referenceUsages(
	ctx context.Context,
	wikiID uuid.UUID,
	kind string,
	targetID uuid.UUID,
	cursor string,
	limit int,
) (*ReferenceUsagePage, error) {
	if err := q.ensureReferenceTargetExists(
		ctx, wikiID, kind, targetID,
	); err != nil {
		return nil, err
	}
	var afterPageID, afterBlockID *uuid.UUID
	var afterNodeID *string
	if cursor != "" {
		decoded, err := decodeBacklinkCursor(cursor)
		if err != nil {
			return nil, err
		}
		afterPageID = &decoded.SourcePageID
		afterBlockID = &decoded.SourceBlockID
		afterNodeID = &decoded.sourceNodeID
	}
	if limit <= 0 {
		limit = page.DefaultHistoryPageSize
	}
	if limit > page.MaxHistoryPageSize {
		limit = page.MaxHistoryPageSize
	}

	queries := map[string]string{
		"entity": `
			SELECT p.id, p.display_title, u.revision_id, u.block_id, u.node_id,
			       u.mention_text, NULL::uuid
			FROM entity_mention_projection u JOIN page p ON p.id = u.page_id
			WHERE u.entity_id = $1 AND p.wiki_id = $2 AND p.deleted_at IS NULL
			  AND p.current_revision_id = u.revision_id
			  AND ($3::uuid IS NULL OR (u.page_id, u.block_id, u.node_id) > ($3, $4, $5))
			ORDER BY u.page_id, u.block_id, u.node_id LIMIT $6`,
		"claim": `
			SELECT p.id, p.display_title, u.revision_id, u.block_id, u.node_id,
			       ''::text, NULL::uuid
			FROM claim_usage u JOIN page p ON p.id = u.page_id
			WHERE u.claim_id = $1 AND p.wiki_id = $2 AND p.deleted_at IS NULL
			  AND p.current_revision_id = u.revision_id
			  AND ($3::uuid IS NULL OR (u.page_id, u.block_id, u.node_id) > ($3, $4, $5))
			ORDER BY u.page_id, u.block_id, u.node_id LIMIT $6`,
		"citation": `
			SELECT p.id, p.display_title, u.revision_id, u.block_id, u.node_id,
			       ''::text, u.claim_id
			FROM citation_usage u JOIN page p ON p.id = u.page_id
			WHERE u.citation_id = $1 AND p.wiki_id = $2 AND p.deleted_at IS NULL
			  AND p.current_revision_id = u.revision_id
			  AND ($3::uuid IS NULL OR (u.page_id, u.block_id, u.node_id) > ($3, $4, $5))
			ORDER BY u.page_id, u.block_id, u.node_id LIMIT $6`,
	}
	rows, err := q.pool.Query(ctx, queries[kind], targetID, wikiID,
		afterPageID, afterBlockID, afterNodeID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("projection: 查询 %s 使用位置失败: %w", kind, err)
	}
	defer rows.Close()

	var scanned []ReferenceUsage
	for rows.Next() {
		var usage ReferenceUsage
		if err := rows.Scan(&usage.PageID, &usage.PageTitle, &usage.RevisionID,
			&usage.BlockID, &usage.NodeID, &usage.MentionText, &usage.ClaimID); err != nil {
			return nil, fmt.Errorf("projection: 扫描 %s 使用位置失败: %w", kind, err)
		}
		scanned = append(scanned, usage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: 迭代 %s 使用位置失败: %w", kind, err)
	}

	result := &ReferenceUsagePage{Items: make([]ReferenceUsage, 0, min(len(scanned), limit))}
	if len(scanned) > limit {
		last := scanned[limit-1]
		cursor := encodeBacklinkCursor(backlinkRow{
			Backlink:     Backlink{SourcePageID: last.PageID, SourceBlockID: last.BlockID},
			sourceNodeID: last.NodeID,
		})
		result.NextCursor = &cursor
		scanned = scanned[:limit]
	}
	result.Items = append(result.Items, scanned...)
	return result, nil
}

func (q *Queries) ensureReferenceTargetExists(
	ctx context.Context,
	wikiID uuid.UUID,
	kind string,
	id uuid.UUID,
) error {
	var exists bool
	switch kind {
	case "entity":
		if err := q.pool.QueryRow(ctx,
			`SELECT EXISTS (
				SELECT 1 FROM entity WHERE id=$1 AND wiki_id=$2
			)`,
			id, wikiID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("projection: 检查 Entity 目标失败: %w", err)
		}
	case "claim":
		if err := q.pool.QueryRow(ctx,
			`SELECT EXISTS (
				SELECT 1
				FROM claim c
				JOIN entity e ON e.id=c.subject_entity_id
				WHERE c.id=$1 AND e.wiki_id=$2
			)`,
			id, wikiID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("projection: 检查 Claim 目标失败: %w", err)
		}
	default:
		tables := map[string]string{
			"citation": "citation", "source": "source", "component": "component",
		}
		table, ok := tables[kind]
		if !ok {
			return fmt.Errorf("projection: 未知引用目标类型 %q", kind)
		}
		if err := q.pool.QueryRow(ctx,
			fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)", table), id).Scan(&exists); err != nil {
			return fmt.Errorf("projection: 检查 %s 目标失败: %w", kind, err)
		}
	}
	if !exists {
		return fmt.Errorf("%w: %s id=%s", ErrReferenceTargetNotFound, kind, id)
	}
	return nil
}

func usageLimit(limit int) int {
	if limit <= 0 {
		return page.DefaultHistoryPageSize
	}
	if limit > page.MaxHistoryPageSize {
		return page.MaxHistoryPageSize
	}
	return limit
}

func usageCursor(
	cursor string,
) (pageID, blockID *uuid.UUID, nodeID *string, err error) {
	if cursor == "" {
		return nil, nil, nil, nil
	}
	decoded, err := decodeBacklinkCursor(cursor)
	if err != nil {
		return nil, nil, nil, err
	}
	return &decoded.SourcePageID, &decoded.SourceBlockID,
		&decoded.sourceNodeID, nil
}

func sourceUsageCursor(cursor string) (*uuid.UUID, error) {
	if cursor == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: Source usage cursor 非 base64url", page.ErrInvalidCursor)
	}
	pageID, err := uuid.Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: Source usage cursor 页面非法", page.ErrInvalidCursor)
	}
	return &pageID, nil
}

func encodeSourceUsageCursor(pageID uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(pageID.String()))
}

func encodeUsageCursor(pageID, blockID uuid.UUID, nodeID string) string {
	return encodeBacklinkCursor(backlinkRow{
		Backlink: Backlink{
			SourcePageID: pageID, SourceBlockID: blockID,
		},
		sourceNodeID: nodeID,
	})
}

// ensurePageExists 断言页面存在（含软删除页，410/404 语义由 handler 层区分）。
func (q *Queries) ensurePageExists(ctx context.Context, pageID uuid.UUID) error {
	var deletedAt any
	err := q.pool.QueryRow(ctx,
		`SELECT deleted_at FROM page WHERE id = $1`, pageID).Scan(&deletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: id=%s", page.ErrPageNotFound, pageID)
	}
	if err != nil {
		return fmt.Errorf("projection: 查询页面失败: %w", err)
	}
	return nil
}

func (q *Queries) ensureWikiPageExists(ctx context.Context, wikiID, pageID uuid.UUID) error {
	var exists bool
	if err := q.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM page WHERE id=$1 AND wiki_id=$2 AND deleted_at IS NULL
	)`, pageID, wikiID).Scan(&exists); err != nil {
		return fmt.Errorf("projection: 检查 Wiki 页面失败: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: page id=%s", ErrReferenceTargetNotFound, pageID)
	}
	return nil
}

// encodeBacklinkCursor 把行键 (source_page_id, source_block_id, source_node_id)
// 编码为不透明游标（base64url 的 "pid:bid:nid"）。
func encodeBacklinkCursor(r backlinkRow) string {
	raw := r.SourcePageID.String() + ":" + r.SourceBlockID.String() + ":" + r.sourceNodeID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeBacklinkCursor 解析游标；无法解析返回 page.ErrInvalidCursor。
// source_node_id 是行内节点下标的十进制形式（见迁移 000006），不做数值校验，
// 无法匹配时仅表现为空页——与历史列表对非法游标严格报错不同，这里结构合法即可。
func decodeBacklinkCursor(cursor string) (backlinkRow, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return backlinkRow{}, fmt.Errorf("%w: 非 base64url", page.ErrInvalidCursor)
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 || parts[2] == "" {
		return backlinkRow{}, fmt.Errorf("%w: 结构非法", page.ErrInvalidCursor)
	}
	pid, err := uuid.Parse(parts[0])
	if err != nil {
		return backlinkRow{}, fmt.Errorf("%w: source_page_id 非法", page.ErrInvalidCursor)
	}
	bid, err := uuid.Parse(parts[1])
	if err != nil {
		return backlinkRow{}, fmt.Errorf("%w: source_block_id 非法", page.ErrInvalidCursor)
	}
	return backlinkRow{
		Backlink:     Backlink{SourcePageID: pid, SourceBlockID: bid},
		sourceNodeID: parts[2],
	}, nil
}
