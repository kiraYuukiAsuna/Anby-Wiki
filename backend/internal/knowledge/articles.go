package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// EntityArticlesParams scopes reader destinations to a Wiki and its current
// access grants. A nil PageIDs permits all pages; an empty slice permits none.
type EntityArticlesParams struct {
	WikiID   uuid.UUID
	EntityID uuid.UUID
	Language string
	PageIDs  []uuid.UUID
}

type EntityArticle struct {
	PageID       uuid.UUID
	DisplayTitle string
	Namespace    string
	Language     string
}

type EntityArticleResolution struct {
	EntityID uuid.UUID
	Items    []EntityArticle
}

// ResolveEntityArticles keeps the semantic Entity identity in the document
// while resolving its primary bindings to published reader destinations.
func (s *Service) ResolveEntityArticles(
	ctx context.Context,
	params EntityArticlesParams,
) (*EntityArticleResolution, error) {
	language := strings.TrimSpace(params.Language)
	if len(language) > maxLanguageLength {
		return nil, ErrInvalidEntityFilter
	}
	entity, err := s.GetEntity(ctx, params.EntityID)
	if err != nil {
		return nil, err
	}
	if entity.WikiID != params.WikiID || entity.Status == StatusDeleted {
		return nil, ErrEntityNotFound
	}
	entity, err = s.ResolveEntity(ctx, entity.ID)
	if errors.Is(err, ErrInvalidEntityMerge) {
		return nil, ErrEntityNotFound
	}
	if err != nil {
		return nil, err
	}
	if entity.WikiID != params.WikiID {
		return nil, ErrEntityNotFound
	}
	items, err := s.repo.listEntityArticles(
		ctx, params.WikiID, entity.ID, language, params.PageIDs,
	)
	if err != nil {
		return nil, err
	}
	return &EntityArticleResolution{EntityID: entity.ID, Items: items}, nil
}

func (r *Repository) listEntityArticles(
	ctx context.Context,
	wikiID, entityID uuid.UUID,
	language string,
	pageIDs []uuid.UUID,
) ([]EntityArticle, error) {
	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE identities(id) AS (
			SELECT id FROM entity WHERE id=$1 AND wiki_id=$3 AND status='active'
			UNION
			SELECT e.id FROM entity e
			JOIN identities i ON e.merged_into_entity_id=i.id
			WHERE e.wiki_id=$3 AND e.status='merged'
		)
		SELECT p.id,p.display_title,n.namespace_key,p.language
		FROM page_entity_binding b
		JOIN identities i ON i.id=b.entity_id
		JOIN page p ON p.id=b.page_id
		JOIN namespace n ON n.id=p.namespace_id
		WHERE b.binding_role=$2
		  AND p.wiki_id=$3 AND p.deleted_at IS NULL AND p.status='active'
		  AND p.current_revision_id IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM page_redirect pr WHERE pr.source_page_id=p.id)
		  AND ($4 OR p.id=ANY($5::uuid[]))
		ORDER BY CASE WHEN p.language=$6 THEN 0 ELSE 1 END,
		         p.display_title,p.language,p.id`,
		entityID, BindingRolePrimary, wikiID, pageIDs == nil, pageIDs, language,
	)
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询实体条目失败: %w", err)
	}
	defer rows.Close()
	items := []EntityArticle{}
	for rows.Next() {
		var item EntityArticle
		if err := rows.Scan(&item.PageID, &item.DisplayTitle, &item.Namespace, &item.Language); err != nil {
			return nil, fmt.Errorf("knowledge: 扫描实体条目失败: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: 遍历实体条目失败: %w", err)
	}
	return items, nil
}
