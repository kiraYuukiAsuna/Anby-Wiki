package evidence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const assetWithRevisionColumns = `a.id,a.wiki_id,a.name,a.current_revision_id,a.status,a.created_at,a.updated_at,
	ar.id,ar.asset_id,ar.storage_key,ar.content_hash,ar.mime_type,ar.size_bytes,
	ar.width,ar.height,ar.metadata_json,ar.actor_id,ar.created_at`

func scanAssetWithRevision(row pgx.Row) (*AssetWithRevision, error) {
	var value AssetWithRevision
	err := row.Scan(
		&value.Asset.ID, &value.Asset.WikiID, &value.Asset.Name,
		&value.Asset.CurrentRevisionID, &value.Asset.Status, &value.Asset.CreatedAt,
		&value.Asset.UpdatedAt, &value.Revision.ID, &value.Revision.AssetID,
		&value.Revision.StorageKey, &value.Revision.ContentHash,
		&value.Revision.MimeType, &value.Revision.SizeBytes, &value.Revision.Width,
		&value.Revision.Height, &value.Revision.MetadataJSON,
		&value.Revision.ActorID, &value.Revision.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

type assetListCursor struct {
	UpdatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

func encodeAssetCursor(value assetListCursor) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeAssetCursor(cursor string) (assetListCursor, error) {
	var value assetListCursor
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	return value, nil
}

func (r *Repository) ListAssets(
	ctx context.Context, wikiID uuid.UUID, kind, cursor string, limit int,
) (*AssetPage, error) {
	var afterTime *time.Time
	afterID := uuid.Nil
	if cursor != "" {
		after, err := decodeAssetCursor(cursor)
		if err != nil || after.UpdatedAt.IsZero() || after.ID == uuid.Nil {
			return nil, ErrInvalidAssetCursor
		}
		afterTime = &after.UpdatedAt
		afterID = after.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+assetWithRevisionColumns+`
		FROM asset a
		JOIN asset_revision ar ON ar.id=a.current_revision_id
		WHERE a.wiki_id=$1 AND a.status='active'
		  AND (
		    $2='' OR
		    ($2='image' AND ar.mime_type LIKE 'image/%') OR
		    ($2='video' AND ar.mime_type LIKE 'video/%') OR
		    ($2='other' AND ar.mime_type NOT LIKE 'image/%' AND ar.mime_type NOT LIKE 'video/%')
		  )
		  AND ($3::timestamptz IS NULL OR (a.updated_at,a.id)<($3,$4))
		ORDER BY a.updated_at DESC,a.id DESC
		LIMIT $5`, wikiID, kind, afterTime, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询资产目录失败: %w", err)
	}
	defer rows.Close()
	result := &AssetPage{Items: make([]AssetWithRevision, 0, limit)}
	for rows.Next() {
		value, err := scanAssetWithRevision(rows)
		if err != nil {
			return nil, fmt.Errorf("evidence: 扫描资产目录失败: %w", err)
		}
		result.Items = append(result.Items, *value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1].Asset
		next := encodeAssetCursor(assetListCursor{UpdatedAt: last.UpdatedAt, ID: last.ID})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (r *Repository) GetAssetRevisionForWiki(
	ctx context.Context, wikiID, revisionID uuid.UUID,
) (*AssetWithRevision, error) {
	value, err := scanAssetWithRevision(r.pool.QueryRow(ctx, `SELECT `+assetWithRevisionColumns+`
		FROM asset a
		JOIN asset_revision ar ON ar.asset_id=a.id
		WHERE a.wiki_id=$1 AND a.status='active' AND ar.id=$2`,
		wikiID, revisionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAssetRevisionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("evidence: 查询资产版本失败: %w", err)
	}
	return value, nil
}
