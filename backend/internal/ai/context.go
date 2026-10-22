package ai

import (
	"context"

	"github.com/google/uuid"
)

type wikiIDKey struct{}

func WithWikiID(ctx context.Context, wikiID uuid.UUID) context.Context {
	return context.WithValue(ctx, wikiIDKey{}, wikiID)
}

func WikiIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	wikiID, ok := ctx.Value(wikiIDKey{}).(uuid.UUID)
	return wikiID, ok && wikiID != uuid.Nil
}
