package search

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/page"
)

// PageAdapter preserves the pre-M7 title/alias selector behavior for callers
// that deliberately do not run the asynchronous search projection.
type PageAdapter struct {
	pages *page.Service
}

func NewPageAdapter(pages *page.Service) *PageAdapter {
	return &PageAdapter{pages: pages}
}

func (a *PageAdapter) Index(context.Context, SearchDocument) error { return nil }
func (a *PageAdapter) Delete(context.Context, uuid.UUID) error     { return nil }

func (a *PageAdapter) Rebuild(context.Context, []SearchDocument) error { return nil }

func (a *PageAdapter) Search(ctx context.Context, query Query) ([]Hit, int, error) {
	query = normalizeQuery(query)
	if query.Text == "" {
		return []Hit{}, 0, nil
	}
	hits, err := a.pages.SearchPages(ctx, query.WikiID, query.Namespace, query.Text, query.Limit)
	if err != nil {
		if errors.Is(err, page.ErrNamespaceNotFound) {
			return nil, 0, ErrNamespaceNotFound
		}
		return nil, 0, err
	}
	result := make([]Hit, 0, len(hits))
	for _, hit := range hits {
		result = append(result, Hit{
			PageID:       hit.ID,
			DisplayTitle: hit.DisplayTitle,
			Namespace:    hit.NamespaceKey,
			MatchedOn:    Field(hit.MatchedOn),
			Highlight:    hit.DisplayTitle,
		})
	}
	return result, len(result), nil
}

var _ SearchAdapter = (*PageAdapter)(nil)
