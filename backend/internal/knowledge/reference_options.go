package knowledge

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type ClaimOption struct {
	ID          uuid.UUID `json:"id"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
}
type ClaimOptionPage struct {
	Items      []ClaimOption `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

// ListClaimOptions exposes only searchable labels within the selected Wiki.
func (s *Service) ListClaimOptions(ctx context.Context, wikiID uuid.UUID, query string, after uuid.UUID, limit int) (*ClaimOptionPage, error) {
	query = strings.TrimSpace(query)
	if wikiID == uuid.Nil || len([]rune(query)) > 255 || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("invalid reference search")
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT c.id, coalesce((SELECT l.label FROM entity_label l WHERE l.entity_id=e.id AND l.is_primary ORDER BY l.language LIMIT 1),e.canonical_key) || ' · ' || p.name,
 left(coalesce(target.canonical_key,c.value_json::text,''),240)
 FROM claim c JOIN entity e ON e.id=c.subject_entity_id JOIN property p ON p.id=c.property_id
 LEFT JOIN entity target ON target.id=c.target_entity_id
 WHERE e.wiki_id=$1 AND e.status='active' AND c.status='published' AND c.id>$3
 AND ($2='' OR position(lower($2) in lower(e.canonical_key || ' ' || p.name || ' ' || coalesce(c.value_json::text,'') || ' ' || coalesce(target.canonical_key,'')))>0
 OR EXISTS(SELECT 1 FROM entity_label l WHERE l.entity_id=e.id AND position(lower($2) in lower(l.label))>0))
 ORDER BY c.id LIMIT $4`, wikiID, query, after, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &ClaimOptionPage{Items: make([]ClaimOption, 0, limit)}
	for rows.Next() {
		var item ClaimOption
		if err := rows.Scan(&item.ID, &item.Label, &item.Description); err != nil {
			return nil, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.NextCursor = result.Items[limit-1].ID.String()
	}
	return result, nil
}
