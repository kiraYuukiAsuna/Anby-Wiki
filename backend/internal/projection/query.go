// 投影查询服务（M3-T03，设计 §17.3：关系查询必须走投影表，不扫 AST）。
// 只读路径：匿名阅读端点经本服务查 page_link_projection（反链）与
// document_outline_projection/page_anchor（目录）。
package projection

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/page"
)

// ErrReferenceTargetNotFound 知识/证据反向查询目标不存在。
var ErrReferenceTargetNotFound = errors.New("projection: 引用目标不存在")

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
func (q *Queries) Backlinks(ctx context.Context, pageID uuid.UUID, cursor string, limit int) (*BacklinkPage, error) {
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
		  AND p.deleted_at IS NULL
		  AND ($2::uuid IS NULL
		       OR (l.source_page_id, l.source_block_id, l.source_node_id) > ($2, $3, $4))
		ORDER BY l.source_page_id, l.source_block_id, l.source_node_id
		LIMIT $5`,
		pageID, afterPageID, afterBlockID, afterNodeID, limit+1)
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

// EntityMentions 返回正文中引用 entityID 的位置；结果携带来源 Revision。
func (q *Queries) EntityMentions(ctx context.Context, entityID uuid.UUID, cursor string, limit int) (*ReferenceUsagePage, error) {
	return q.referenceUsages(ctx, "entity", entityID, cursor, limit)
}

// ClaimUsages 返回正文中引用 claimID 的位置；结果携带来源 Revision。
func (q *Queries) ClaimUsages(ctx context.Context, claimID uuid.UUID, cursor string, limit int) (*ReferenceUsagePage, error) {
	return q.referenceUsages(ctx, "claim", claimID, cursor, limit)
}

// CitationUsages 返回正文中引用 citationID 的位置；结果携带来源 Revision。
func (q *Queries) CitationUsages(ctx context.Context, citationID uuid.UUID, cursor string, limit int) (*ReferenceUsagePage, error) {
	return q.referenceUsages(ctx, "citation", citationID, cursor, limit)
}

func (q *Queries) referenceUsages(ctx context.Context, kind string, targetID uuid.UUID, cursor string, limit int) (*ReferenceUsagePage, error) {
	if err := q.ensureReferenceTargetExists(ctx, kind, targetID); err != nil {
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
			WHERE u.entity_id = $1 AND p.deleted_at IS NULL
			  AND p.current_revision_id = u.revision_id
			  AND ($2::uuid IS NULL OR (u.page_id, u.block_id, u.node_id) > ($2, $3, $4))
			ORDER BY u.page_id, u.block_id, u.node_id LIMIT $5`,
		"claim": `
			SELECT p.id, p.display_title, u.revision_id, u.block_id, u.node_id,
			       ''::text, NULL::uuid
			FROM claim_usage u JOIN page p ON p.id = u.page_id
			WHERE u.claim_id = $1 AND p.deleted_at IS NULL
			  AND p.current_revision_id = u.revision_id
			  AND ($2::uuid IS NULL OR (u.page_id, u.block_id, u.node_id) > ($2, $3, $4))
			ORDER BY u.page_id, u.block_id, u.node_id LIMIT $5`,
		"citation": `
			SELECT p.id, p.display_title, u.revision_id, u.block_id, u.node_id,
			       ''::text, u.claim_id
			FROM citation_usage u JOIN page p ON p.id = u.page_id
			WHERE u.citation_id = $1 AND p.deleted_at IS NULL
			  AND p.current_revision_id = u.revision_id
			  AND ($2::uuid IS NULL OR (u.page_id, u.block_id, u.node_id) > ($2, $3, $4))
			ORDER BY u.page_id, u.block_id, u.node_id LIMIT $5`,
	}
	rows, err := q.pool.Query(ctx, queries[kind], targetID,
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

func (q *Queries) ensureReferenceTargetExists(ctx context.Context, kind string, id uuid.UUID) error {
	tables := map[string]string{"entity": "entity", "claim": "claim", "citation": "citation"}
	table, ok := tables[kind]
	if !ok {
		return fmt.Errorf("projection: 未知引用目标类型 %q", kind)
	}
	var exists bool
	if err := q.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE id = $1)", table), id).Scan(&exists); err != nil {
		return fmt.Errorf("projection: 检查 %s 目标失败: %w", kind, err)
	}
	if !exists {
		return fmt.Errorf("%w: %s id=%s", ErrReferenceTargetNotFound, kind, id)
	}
	return nil
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
