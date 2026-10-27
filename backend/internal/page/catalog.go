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

	CatalogStatusPublished = "published"
	CatalogStatusDraft     = "draft"

	DefaultCatalogPageSize = 20
	MaxCatalogPageSize     = 100
)

// CatalogItem is the public discovery projection of one live Page.
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
	Status          string    `json:"p"`
	UpdatedAt       time.Time `json:"u,omitempty"`
	NormalizedTitle string    `json:"t,omitempty"`
	ID              uuid.UUID `json:"i"`
}

// ListCatalogPages returns live content pages for the requested publication
// state. The catalog is an anonymous read model; all authoritative mutations
// still go through the existing Page service methods.
func (s *Service) ListCatalogPages(
	ctx context.Context,
	wikiID uuid.UUID,
	status, sortKey, cursor string,
	limit int,
) (*CatalogPage, error) {
	return s.listCatalogPages(ctx, wikiID, uuid.Nil, status, sortKey, cursor, limit)
}

func (s *Service) ListSharedCatalogPages(
	ctx context.Context,
	wikiID, actorID uuid.UUID,
	status, sortKey, cursor string,
	limit int,
) (*CatalogPage, error) {
	if actorID == uuid.Nil {
		return nil, ErrWikiNotFound
	}
	return s.listCatalogPages(ctx, wikiID, actorID, status, sortKey, cursor, limit)
}

func (s *Service) listCatalogPages(
	ctx context.Context,
	wikiID, actorID uuid.UUID,
	status, sortKey, cursor string,
	limit int,
) (*CatalogPage, error) {
	if wikiID == uuid.Nil {
		return nil, ErrWikiNotFound
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = CatalogStatusPublished
	}
	if status != CatalogStatusPublished && status != CatalogStatusDraft {
		return nil, fmt.Errorf("%w: status=%q", ErrInvalidCursor, status)
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
		if err != nil || decoded.Sort != sortKey || decoded.Status != status ||
			decoded.ID == uuid.Nil ||
			(sortKey == CatalogSortRecent && decoded.UpdatedAt.IsZero()) ||
			(sortKey == CatalogSortTitle && decoded.NormalizedTitle == "") {
			return nil, fmt.Errorf("%w: 页面目录 cursor 非法", ErrInvalidCursor)
		}
		after = &decoded
	}

	var items []CatalogItem
	var total int
	var err error
	if actorID == uuid.Nil {
		items, total, err = s.repo.ListCatalogPages(ctx, wikiID, status, sortKey, after, limit+1)
	} else {
		items, total, err = s.repo.ListSharedCatalogPages(
			ctx, wikiID, actorID, status, sortKey, after, limit+1,
		)
	}
	if err != nil {
		return nil, err
	}
	result := &CatalogPage{Items: items, Total: total}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeCatalogCursor(catalogCursor{
			Sort: sortKey, Status: status, UpdatedAt: last.Page.UpdatedAt,
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
