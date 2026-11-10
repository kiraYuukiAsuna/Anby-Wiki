package governance

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type MemberOption struct {
	ID          uuid.UUID `json:"id"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
}
type MemberOptionPage struct {
	Items      []MemberOption `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

// ListMemberOptions exposes only searchable labels within the selected Wiki.
func (s *UserAdminService) ListMemberOptions(ctx context.Context, wikiID uuid.UUID, query string, after uuid.UUID, limit int) (*MemberOptionPage, error) {
	query = strings.TrimSpace(query)
	if wikiID == uuid.Nil || len([]rune(query)) > 255 || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("invalid reference search")
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT a.id,a.display_name,coalesce(la.username,'')
 FROM actor a LEFT JOIN local_account la ON la.actor_id=a.id
 WHERE a.status='active' AND (la.id IS NULL OR la.status='active') AND a.actor_type='human' AND a.id>$3
 AND EXISTS(SELECT 1 FROM actor_role ar WHERE ar.actor_id=a.id AND ar.wiki_id=$1)
 AND ($2='' OR position(lower($2) in lower(a.display_name || ' ' || coalesce(la.username,'')))>0)
 ORDER BY a.id LIMIT $4`, wikiID, query, after, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &MemberOptionPage{Items: make([]MemberOption, 0, limit)}
	for rows.Next() {
		var item MemberOption
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
