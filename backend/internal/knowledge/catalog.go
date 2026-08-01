package knowledge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultEntityCatalogLimit = 20
	maxEntityCatalogLimit     = 100
)

// EntityCatalogParams 定义站点实体目录的只读筛选。
// Status 支持 active（默认）、merged 与 all；all 仍排除软删除实体。
type EntityCatalogParams struct {
	WikiID  uuid.UUID
	Query   string
	TypeKey string
	Status  string
	Cursor  string
	Limit   int
}

type entityCatalogCursor struct {
	UpdatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

func encodeEntityCatalogCursor(value entityCatalogCursor) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeEntityCatalogCursor(raw string) (entityCatalogCursor, error) {
	var value entityCatalogCursor
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(decoded, &value); err != nil {
		return value, err
	}
	return value, nil
}

// ListEntities 返回 Wiki 隔离的实体目录。查询同时匹配 canonical key、标签与别名；
// 排序只依赖 (updated_at,id)，翻页期间不会因显示标签或计数变化而重复跳项。
func (s *Service) ListEntities(
	ctx context.Context,
	params EntityCatalogParams,
) (*EntityCatalogPage, error) {
	if params.WikiID == uuid.Nil {
		return nil, fmt.Errorf("%w: wiki_id 为空", ErrInvalidEntityFilter)
	}
	status := strings.TrimSpace(strings.ToLower(params.Status))
	if status == "" {
		status = StatusActive
	}
	switch status {
	case StatusActive, StatusMerged, "all":
	default:
		return nil, fmt.Errorf("%w: status=%q", ErrInvalidEntityFilter, status)
	}

	typeKey := strings.TrimSpace(strings.ToLower(params.TypeKey))
	if len(typeKey) > 100 {
		return nil, fmt.Errorf("%w: type_key 过长", ErrInvalidEntityFilter)
	}

	queryKey := ""
	queryPattern := ""
	if strings.TrimSpace(params.Query) != "" {
		var err error
		queryKey, err = normalizeKey(params.Query)
		if err != nil {
			return nil, err
		}
		queryPattern = likePattern(queryKey)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = defaultEntityCatalogLimit
	}
	if limit > maxEntityCatalogLimit {
		limit = maxEntityCatalogLimit
	}
	return s.repo.ListEntityCatalog(
		ctx, params.WikiID, queryKey, queryPattern, typeKey, status, params.Cursor, limit,
	)
}

// ListEntityCatalog 执行实体目录查询。所有筛选值均参数绑定；ILIKE 通配符已由
// likePattern 转义，因此用户输入的 % 与 _ 只按普通字符匹配。
func (r *Repository) ListEntityCatalog(
	ctx context.Context,
	wikiID uuid.UUID,
	queryKey, queryPattern, typeKey, status, cursor string,
	limit int,
) (*EntityCatalogPage, error) {
	var afterTime *time.Time
	afterID := uuid.Nil
	if cursor != "" {
		after, err := decodeEntityCatalogCursor(cursor)
		if err != nil || after.UpdatedAt.IsZero() || after.ID == uuid.Nil {
			return nil, ErrInvalidEntityCursor
		}
		afterTime = &after.UpdatedAt
		afterID = after.ID
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			e.id,e.wiki_id,e.canonical_key,e.status,e.merged_into_entity_id,
			t.id,t.type_key,t.name,t.created_at,
			COALESCE(display_label.label,e.canonical_key),
			COALESCE(display_label.language,''),
			COALESCE(display_label.description,''),
			(SELECT count(*)::int FROM entity_label lc WHERE lc.entity_id=e.id),
			(SELECT count(*)::int FROM entity_alias ac WHERE ac.entity_id=e.id),
			(SELECT count(*)::int FROM claim cc
			 WHERE cc.subject_entity_id=e.id
			   AND cc.status IN ('proposed','published','deprecated')),
			(SELECT count(DISTINCT pb.page_id)::int FROM page_entity_binding pb
			 WHERE pb.entity_id=e.id),
			e.created_at,e.updated_at
		FROM entity e
		JOIN entity_type t ON t.id=e.entity_type_id
		LEFT JOIN LATERAL (
			SELECT l.language,l.label,l.description
			FROM entity_label l
			WHERE l.entity_id=e.id
			ORDER BY
				l.is_primary DESC,
				CASE
					WHEN l.language IN ('zh-Hans','zh-CN','zh') THEN 0
					WHEN l.language='en' THEN 1
					ELSE 2
				END,
				l.language,l.label
			LIMIT 1
		) display_label ON true
		WHERE e.wiki_id=$1
		  AND ($2='' OR t.type_key=$2)
		  AND (
			($3='all' AND e.status IN ('active','merged'))
			OR e.status=$3
		  )
		  AND (
			$4='' OR
			e.canonical_key ILIKE $5 ESCAPE '\' OR
			EXISTS (
				SELECT 1 FROM entity_label sl
				WHERE sl.entity_id=e.id AND sl.label ILIKE $5 ESCAPE '\'
			) OR
			EXISTS (
				SELECT 1 FROM entity_alias sa
				WHERE sa.entity_id=e.id AND sa.normalized_alias ILIKE $5 ESCAPE '\'
			)
		  )
		  AND ($6::timestamptz IS NULL OR (e.updated_at,e.id)<($6,$7))
		ORDER BY e.updated_at DESC,e.id DESC
		LIMIT $8`,
		wikiID, typeKey, status, queryKey, queryPattern, afterTime, afterID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("knowledge: 查询实体目录失败: %w", err)
	}
	defer rows.Close()

	result := &EntityCatalogPage{Items: make([]EntityCatalogItem, 0, limit)}
	for rows.Next() {
		var item EntityCatalogItem
		if err := rows.Scan(
			&item.ID, &item.WikiID, &item.CanonicalKey, &item.Status,
			&item.MergedIntoEntityID, &item.EntityType.ID, &item.EntityType.TypeKey,
			&item.EntityType.Name, &item.EntityType.CreatedAt, &item.DisplayLabel,
			&item.DisplayLanguage, &item.Description, &item.LabelCount,
			&item.AliasCount, &item.ClaimCount, &item.PageCount,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("knowledge: 扫描实体目录失败: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: 遍历实体目录失败: %w", err)
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeEntityCatalogCursor(entityCatalogCursor{
			UpdatedAt: last.UpdatedAt,
			ID:        last.ID,
		})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}
