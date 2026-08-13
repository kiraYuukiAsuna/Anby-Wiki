package page

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
	CatalogSortRecent = "recent"
	CatalogSortTitle  = "title"

	DefaultCatalogPageSize = 20
	MaxCatalogPageSize     = 100
)

// CatalogItem is the public discovery projection of one live, published Page.
// It stays in the Page domain because the source fields are authoritative Page
// identity and Current Revision metadata rather than a rebuildable search hit.
type CatalogItem struct {
	Page         Page
	NamespaceKey string
}

type CatalogPage struct {
	Items      []CatalogItem
	NextCursor *string
	Total      int
}

type catalogCursor struct {
	Sort            string    `json:"s"`
	UpdatedAt       time.Time `json:"u,omitempty"`
	NormalizedTitle string    `json:"t,omitempty"`
	ID              uuid.UUID `json:"i"`
}

// ListPublishedPages returns live pages that have a Current Revision. The
// catalog is an anonymous read model; all authoritative mutations still go
// through the existing Page service methods.
func (s *Service) ListPublishedPages(
	ctx context.Context,
	wikiID uuid.UUID,
	sortKey, cursor string,
	limit int,
) (*CatalogPage, error) {
	if wikiID == uuid.Nil {
		return nil, ErrWikiNotFound
	}
	sortKey = strings.TrimSpace(sortKey)
	if sortKey == "" {
		sortKey = CatalogSortRecent
	}
	if sortKey != CatalogSortRecent && sortKey != CatalogSortTitle {
		return nil, fmt.Errorf("%w: sort=%q", ErrInvalidCursor, sortKey)
	}
	if limit <= 0 {
		limit = DefaultCatalogPageSize
	}
	if limit > MaxCatalogPageSize {
		limit = MaxCatalogPageSize
	}

	var after *catalogCursor
	if cursor != "" {
		decoded, err := decodeCatalogCursor(cursor)
		if err != nil || decoded.Sort != sortKey || decoded.ID == uuid.Nil ||
			(sortKey == CatalogSortRecent && decoded.UpdatedAt.IsZero()) ||
			(sortKey == CatalogSortTitle && decoded.NormalizedTitle == "") {
			return nil, fmt.Errorf("%w: 页面目录 cursor 非法", ErrInvalidCursor)
		}
		after = &decoded
	}

	items, total, err := s.repo.ListPublishedPages(ctx, wikiID, sortKey, after, limit+1)
	if err != nil {
		return nil, err
	}
	result := &CatalogPage{Items: items, Total: total}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeCatalogCursor(catalogCursor{
			Sort: sortKey, UpdatedAt: last.Page.UpdatedAt,
			NormalizedTitle: last.Page.NormalizedTitle, ID: last.Page.ID,
		})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func encodeCatalogCursor(value catalogCursor) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCatalogCursor(value string) (catalogCursor, error) {
	var cursor catalogCursor
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor, err
	}
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return catalogCursor{}, err
	}
	return cursor, nil
}
