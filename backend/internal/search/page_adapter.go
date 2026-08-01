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

func (a *PageAdapter) Capabilities() Capabilities {
	return keywordCapabilities("page")
}

func (a *PageAdapter) Index(context.Context, SearchDocument) error { return nil }
func (a *PageAdapter) Delete(context.Context, uuid.UUID) error     { return nil }

func (a *PageAdapter) Rebuild(context.Context, []SearchDocument) error { return nil }

func (a *PageAdapter) Search(ctx context.Context, query Query) (Result, error) {
	query = normalizeQuery(query)
	if err := validateQuery(query); err != nil {
		return Result{}, err
	}
	if query.Mode != ModeKeyword {
		return Result{}, ErrSemanticUnavailable
	}
	if query.Text == "" {
		return emptyResult(query.Mode), nil
	}
	hits, err := a.pages.SearchPages(ctx, query.WikiID, query.Namespace, query.Text, query.Limit)
	if err != nil {
		if errors.Is(err, page.ErrNamespaceNotFound) {
			return Result{}, ErrNamespaceNotFound
		}
		return Result{}, err
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
	response := emptyResult(query.Mode)
	response.Hits = result
	response.Total = len(result)
	return response, nil
}

var _ SearchAdapter = (*PageAdapter)(nil)
