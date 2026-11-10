package evidence

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type CitationOption struct {
	ID          uuid.UUID `json:"id"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
}
type CitationOptionPage struct {
	Items      []CitationOption `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

// ListCitationOptions exposes only searchable labels within the selected Wiki.
func (s *Service) ListCitationOptions(ctx context.Context, wikiID uuid.UUID, query string, after uuid.UUID, limit int) (*CitationOptionPage, error) {
	query = strings.TrimSpace(query)
	if wikiID == uuid.Nil || len([]rune(query)) > 255 || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("invalid reference search")
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT c.id,s.title,left(coalesce(c.quotation,c.locator_json::text,''),240)
 FROM citation c JOIN source_version v ON v.id=c.source_version_id JOIN source s ON s.id=v.source_id
 WHERE s.wiki_id=$1 AND c.id>$3 AND ($2='' OR position(lower($2) in lower(s.title || ' ' || coalesce(c.quotation,'')))>0)
 ORDER BY c.id LIMIT $4`, wikiID, query, after, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &CitationOptionPage{Items: make([]CitationOption, 0, limit)}
	for rows.Next() {
		var item CitationOption
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
