package evidence

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
	DefaultSourcePageSize = 24
	MaxSourcePageSize     = 100
)

type sourceListCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

type sourceChunkCursor struct {
	Ordinal int       `json:"o"`
	ID      uuid.UUID `json:"i"`
}

func encodeSourceCursor(value any) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeSourceCursor(raw string, target any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, target)
}

func escapeSourceLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// ListSources returns logical evidence sources without reading any source chunk
// body. Filtering is deliberately limited to indexed metadata fields.
func (r *Repository) ListSources(
	ctx context.Context, sourceType, query, cursor string, limit int,
) (*SourcePage, error) {
	var afterTime *time.Time
	afterID := uuid.Nil
	if cursor != "" {
		var after sourceListCursor
		if err := decodeSourceCursor(cursor, &after); err != nil ||
			after.CreatedAt.IsZero() || after.ID == uuid.Nil {
			return nil, ErrInvalidSourceCursor
		}
		afterTime = &after.CreatedAt
		afterID = after.ID
	}
	pattern := ""
	if query != "" {
		pattern = "%" + escapeSourceLikePattern(query) + "%"
	}
	rows, err := r.pool.Query(ctx, `SELECT `+sourceColumns+`
		FROM source
		WHERE ($1='' OR source_type=$1)
		  AND ($2='' OR title ILIKE $2 ESCAPE '\'
		    OR COALESCE(author,'') ILIKE $2 ESCAPE '\'
		    OR COALESCE(publisher,'') ILIKE $2 ESCAPE '\')
		  AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4))
		ORDER BY created_at DESC,id DESC
		LIMIT $5`, sourceType, pattern, afterTime, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询来源目录失败: %w", err)
	}
	defer rows.Close()
	result := &SourcePage{Items: make([]Source, 0, limit)}
	for rows.Next() {
		value, err := scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("evidence: 扫描来源目录失败: %w", err)
		}
		result.Items = append(result.Items, *value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeSourceCursor(sourceListCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (r *Repository) ListSourceVersionsPage(
	ctx context.Context, sourceID uuid.UUID, cursor string, limit int,
) (*SourceVersionPage, error) {
	var afterTime *time.Time
	afterID := uuid.Nil
	if cursor != "" {
		var after sourceListCursor
		if err := decodeSourceCursor(cursor, &after); err != nil ||
			after.CreatedAt.IsZero() || after.ID == uuid.Nil {
			return nil, ErrInvalidSourceCursor
		}
		afterTime = &after.CreatedAt
		afterID = after.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+sourceVersionColumns+`
		FROM source_version
		WHERE source_id=$1
		  AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3))
		ORDER BY created_at DESC,id DESC
		LIMIT $4`, sourceID, afterTime, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询来源版本目录失败: %w", err)
	}
	defer rows.Close()
	result := &SourceVersionPage{Items: make([]SourceVersion, 0, limit)}
	for rows.Next() {
		value, err := scanSourceVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("evidence: 扫描来源版本目录失败: %w", err)
		}
		result.Items = append(result.Items, *value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeSourceCursor(sourceListCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (r *Repository) ListSourceChunksPage(
	ctx context.Context, sourceVersionID uuid.UUID, cursor string, limit int,
) (*SourceChunkPage, error) {
	afterOrdinal := -1
	afterID := uuid.Nil
	if cursor != "" {
		var after sourceChunkCursor
		if err := decodeSourceCursor(cursor, &after); err != nil ||
			after.Ordinal < 0 || after.ID == uuid.Nil {
			return nil, ErrInvalidSourceCursor
		}
		afterOrdinal = after.Ordinal
		afterID = after.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+sourceChunkColumns+`
		FROM source_chunk
		WHERE source_version_id=$1
		  AND ($2<0 OR (ordinal,id)>($2,$3))
		ORDER BY ordinal,id
		LIMIT $4`, sourceVersionID, afterOrdinal, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询来源分片目录失败: %w", err)
	}
	defer rows.Close()
	result := &SourceChunkPage{Items: make([]SourceChunk, 0, limit)}
	for rows.Next() {
		var value SourceChunk
		if err := rows.Scan(
			&value.ID, &value.SourceVersionID, &value.Ordinal, &value.LocatorJSON,
			&value.TextContent, &value.TextHash, &value.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("evidence: 扫描来源分片目录失败: %w", err)
		}
		result.Items = append(result.Items, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeSourceCursor(sourceChunkCursor{Ordinal: last.Ordinal, ID: last.ID})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (s *Service) ListSources(
	ctx context.Context, sourceType, query, cursor string, limit int,
) (*SourcePage, error) {
	sourceType = strings.TrimSpace(sourceType)
	query = strings.TrimSpace(query)
	if sourceType != "" && !IsValidSourceType(sourceType) {
		return nil, ErrInvalidSourceInput
	}
	if len([]rune(query)) > 200 {
		return nil, ErrInvalidSourceInput
	}
	if limit <= 0 {
		limit = DefaultSourcePageSize
	}
	if limit > MaxSourcePageSize {
		limit = MaxSourcePageSize
	}
	return s.repo.ListSources(ctx, sourceType, query, cursor, limit)
}

func (s *Service) GetSourceDetail(ctx context.Context, sourceID uuid.UUID) (*SourceDetail, error) {
	source, err := s.repo.GetSourceByID(ctx, nil, sourceID)
	if err != nil {
		return nil, err
	}
	result := &SourceDetail{Source: *source}
	if source.ExternalResourceID != nil {
		resource, err := s.repo.GetExternalResourceByID(ctx, nil, *source.ExternalResourceID)
		if err != nil {
			return nil, err
		}
		result.ExternalResource = resource
	}
	return result, nil
}

func (s *Service) ListSourceVersions(
	ctx context.Context, sourceID uuid.UUID, cursor string, limit int,
) (*SourceVersionPage, error) {
	if _, err := s.repo.GetSourceByID(ctx, nil, sourceID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultSourcePageSize
	}
	if limit > MaxSourcePageSize {
		limit = MaxSourcePageSize
	}
	return s.repo.ListSourceVersionsPage(ctx, sourceID, cursor, limit)
}

func (s *Service) ListSourceChunksPage(
	ctx context.Context, sourceVersionID uuid.UUID, cursor string, limit int,
) (*SourceChunkPage, error) {
	if _, err := s.repo.GetSourceVersionByID(ctx, nil, sourceVersionID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	return s.repo.ListSourceChunksPage(ctx, sourceVersionID, cursor, limit)
}
