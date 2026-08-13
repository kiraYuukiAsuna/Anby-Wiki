package page

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const catalogColumns = `p.id,p.wiki_id,p.namespace_id,p.normalized_title,p.display_title,
	p.language,p.content_model,p.status,p.current_revision_id,p.primary_entity_id,
	p.created_by,p.created_at,p.updated_at,p.deleted_at,n.namespace_key`

func scanCatalogItem(row pgx.Row) (CatalogItem, error) {
	var item CatalogItem
	err := row.Scan(
		&item.Page.ID, &item.Page.WikiID, &item.Page.NamespaceID,
		&item.Page.NormalizedTitle, &item.Page.DisplayTitle, &item.Page.Language,
		&item.Page.ContentModel, &item.Page.Status, &item.Page.CurrentRevisionID,
		&item.Page.PrimaryEntityID, &item.Page.CreatedBy, &item.Page.CreatedAt,
		&item.Page.UpdatedAt, &item.Page.DeletedAt, &item.NamespaceKey,
	)
	return item, err
}

func (r *Repository) ListPublishedPages(
	ctx context.Context,
	wikiID uuid.UUID,
	sortKey string,
	after *catalogCursor,
	limit int,
) ([]CatalogItem, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*)
		FROM page p JOIN namespace n ON n.id=p.namespace_id
		WHERE p.wiki_id=$1 AND p.deleted_at IS NULL
		  AND p.current_revision_id IS NOT NULL AND n.is_content`, wikiID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("page: 统计已发布页面失败: %w", err)
	}

	var rows pgx.Rows
	var err error
	if sortKey == CatalogSortTitle {
		var afterTitle *string
		afterID := uuid.Nil
		if after != nil {
			afterTitle = &after.NormalizedTitle
			afterID = after.ID
		}
		rows, err = r.pool.Query(ctx, `SELECT `+catalogColumns+`
			FROM page p JOIN namespace n ON n.id=p.namespace_id
			WHERE p.wiki_id=$1 AND p.deleted_at IS NULL
			  AND p.current_revision_id IS NOT NULL AND n.is_content
			  AND ($2::text IS NULL OR (p.normalized_title,p.id)>($2,$3))
			ORDER BY p.normalized_title ASC,p.id ASC LIMIT $4`,
			wikiID, afterTitle, afterID, limit)
	} else {
		var afterTime *time.Time
		afterID := uuid.Nil
		if after != nil {
			afterTime = &after.UpdatedAt
			afterID = after.ID
		}
		rows, err = r.pool.Query(ctx, `SELECT `+catalogColumns+`
			FROM page p JOIN namespace n ON n.id=p.namespace_id
			WHERE p.wiki_id=$1 AND p.deleted_at IS NULL
			  AND p.current_revision_id IS NOT NULL AND n.is_content
			  AND ($2::timestamptz IS NULL OR (p.updated_at,p.id)<($2,$3))
			ORDER BY p.updated_at DESC,p.id DESC LIMIT $4`,
			wikiID, afterTime, afterID, limit)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("page: 列出已发布页面失败: %w", err)
	}
	defer rows.Close()

	items := make([]CatalogItem, 0, limit)
	for rows.Next() {
		item, scanErr := scanCatalogItem(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("page: 扫描页面目录失败: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
