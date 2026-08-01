package evidence

import (
	"context"
	"io"
	"strings"

	"github.com/google/uuid"
)

const (
	DefaultAssetPageSize = 24
	MaxAssetPageSize     = 100
)

func (s *Service) ListAssets(
	ctx context.Context, wikiID uuid.UUID, kind, cursor string, limit int,
) (*AssetPage, error) {
	kind = strings.TrimSpace(kind)
	if kind != "" && kind != "image" && kind != "video" && kind != "other" {
		return nil, ErrInvalidAssetInput
	}
	if limit <= 0 {
		limit = DefaultAssetPageSize
	}
	if limit > MaxAssetPageSize {
		limit = MaxAssetPageSize
	}
	return s.repo.ListAssets(ctx, wikiID, kind, cursor, limit)
}

func (s *Service) GetAssetRevision(
	ctx context.Context, wikiID, revisionID uuid.UUID,
) (*AssetWithRevision, error) {
	return s.repo.GetAssetRevisionForWiki(ctx, wikiID, revisionID)
}

type OpenAssetRevisionResult struct {
	Value   *AssetWithRevision
	Content io.ReadCloser
}

func (s *Service) OpenAssetRevision(
	ctx context.Context, wikiID, revisionID uuid.UUID,
) (*OpenAssetRevisionResult, error) {
	if s.store == nil {
		return nil, ErrAssetStorageUnavailable
	}
	value, err := s.repo.GetAssetRevisionForWiki(ctx, wikiID, revisionID)
	if err != nil {
		return nil, err
	}
	content, err := s.store.Get(ctx, value.Revision.StorageKey)
	if err != nil {
		return nil, err
	}
	return &OpenAssetRevisionResult{Value: value, Content: content}, nil
}
