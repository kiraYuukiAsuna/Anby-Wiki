package page

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/platform/storage"
)

type SnapshotStorageConfig struct {
	Store       storage.Store
	Environment string
	MaxBytes    int64
	Retention   time.Duration
	BatchSize   int
}

// WithSnapshotStorage enables transparent cold hydration and archive runs.
func (s *Service) WithSnapshotStorage(
	config SnapshotStorageConfig,
) *Service {
	s.snapshotStore = config.Store
	s.snapshotEnv = strings.TrimSpace(config.Environment)
	if config.MaxBytes > 0 {
		s.snapshotMaxBytes = config.MaxBytes
	}
	if config.Retention > 0 {
		s.archiveRetention = config.Retention
	}
	if config.BatchSize > 0 {
		s.archiveBatchSize = config.BatchSize
	}
	return s
}

type RevisionStorageStats struct {
	ArchiveAvailable   bool
	Retention          time.Duration
	DefaultBatchSize   int
	HotSnapshots       int64
	ColdSnapshots      int64
	HotBytes           int64
	ColdBytes          int64
	EligibleSnapshots  int64
	OldestHotCreatedAt *time.Time
}

type SnapshotArchiveResult struct {
	Examined      int
	Archived      int
	Skipped       int
	ArchivedBytes int64
}

func (s *Service) SnapshotStorageStats(
	ctx context.Context,
) (*RevisionStorageStats, error) {
	cutoff := time.Now().UTC().Add(-s.archiveRetention)
	result := &RevisionStorageStats{
		ArchiveAvailable: s.snapshotStore != nil && s.snapshotEnv != "",
		Retention:        s.archiveRetention, DefaultBatchSize: s.archiveBatchSize,
	}
	err := s.repo.q(nil).QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE storage_tier='hot'),
			count(*) FILTER (WHERE storage_tier='cold'),
			COALESCE(sum(size_bytes) FILTER (WHERE storage_tier='hot'),0),
			COALESCE(sum(size_bytes) FILTER (WHERE storage_tier='cold'),0),
			count(*) FILTER (
				WHERE storage_tier='hot' AND created_at < $1
				  AND NOT EXISTS (
					SELECT 1 FROM page p
					JOIN revision r ON r.id=p.current_revision_id
					WHERE r.content_snapshot_id=content_snapshot.id
				  )
			),
			min(created_at) FILTER (WHERE storage_tier='hot')
		FROM content_snapshot`, cutoff,
	).Scan(
		&result.HotSnapshots, &result.ColdSnapshots,
		&result.HotBytes, &result.ColdBytes,
		&result.EligibleSnapshots, &result.OldestHotCreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("page: query Revision storage stats: %w", err)
	}
	return result, nil
}

// ArchiveRevisionSnapshots moves eligible non-current hot snapshots to
// content-addressed object storage. Object upload happens before the guarded
// physical tier transition, so a DB row never points at an absent object.
func (s *Service) ArchiveRevisionSnapshots(
	ctx context.Context,
	limit int,
) (*SnapshotArchiveResult, error) {
	if s.snapshotStore == nil || s.snapshotEnv == "" {
		return nil, ErrSnapshotStorageUnavailable
	}
	if limit <= 0 {
		limit = s.archiveBatchSize
	}
	if limit > 500 {
		limit = 500
	}
	cutoff := time.Now().UTC().Add(-s.archiveRetention)
	rows, err := s.repo.q(nil).Query(ctx, `
		SELECT cs.id
		FROM content_snapshot cs
		WHERE cs.storage_tier='hot' AND cs.created_at < $1
		  AND EXISTS (
			SELECT 1 FROM revision r WHERE r.content_snapshot_id=cs.id
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM page p
			JOIN revision r ON r.id=p.current_revision_id
			WHERE r.content_snapshot_id=cs.id
		  )
		ORDER BY cs.created_at,cs.id
		LIMIT $2`, cutoff, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("page: list archive candidates: %w", err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("page: iterate archive candidates: %w", err)
	}
	result := &SnapshotArchiveResult{Examined: len(ids)}
	for _, snapshotID := range ids {
		archived, size, err := s.archiveSnapshot(ctx, snapshotID)
		if err != nil {
			return result, err
		}
		if archived {
			result.Archived++
			result.ArchivedBytes += size
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

func (s *Service) archiveSnapshot(
	ctx context.Context,
	snapshotID uuid.UUID,
) (bool, int64, error) {
	var raw []byte
	var contentHash string
	var sizeBytes int
	err := s.repo.q(nil).QueryRow(ctx, `
		SELECT ast_json,content_hash,size_bytes
		FROM content_snapshot
		WHERE id=$1 AND storage_tier='hot'`, snapshotID,
	).Scan(&raw, &contentHash, &sizeBytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("page: read archive candidate %s: %w", snapshotID, err)
	}
	canonical, err := ast.CanonicalizeJSON(raw)
	if err != nil {
		return false, 0, fmt.Errorf("%w: snapshot=%s: %v", ErrSnapshotIntegrity, snapshotID, err)
	}
	if int64(len(canonical)) > s.snapshotMaxBytes ||
		len(canonical) != sizeBytes ||
		contentHashOf(canonical) != contentHash {
		return false, 0, fmt.Errorf(
			"%w: snapshot=%s size/hash mismatch", ErrSnapshotIntegrity, snapshotID,
		)
	}
	key, err := storage.ContentKey(
		s.snapshotEnv, "revision-snapshots", contentHash,
	)
	if err != nil {
		return false, 0, err
	}
	meta, headErr := s.snapshotStore.Head(ctx, key)
	switch {
	case headErr == nil:
		if meta.Size != int64(sizeBytes) {
			return false, 0, fmt.Errorf(
				"%w: object=%s size=%d expected=%d",
				ErrSnapshotIntegrity, key, meta.Size, sizeBytes,
			)
		}
	case errors.Is(headErr, storage.ErrNotFound):
		if err := s.snapshotStore.Put(
			ctx, key, bytes.NewReader(canonical), int64(sizeBytes),
			"application/vnd.anby.ast+json",
		); err != nil {
			return false, 0, err
		}
		meta, err = s.snapshotStore.Head(ctx, key)
		if err != nil || meta.Size != int64(sizeBytes) {
			return false, 0, fmt.Errorf(
				"%w: verify uploaded object %s", ErrSnapshotIntegrity, key,
			)
		}
	default:
		return false, 0, headErr
	}

	transitioned := false
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var tier, lockedHash string
		var lockedSize int
		err := tx.QueryRow(ctx, `
			SELECT storage_tier,content_hash,size_bytes
			FROM content_snapshot WHERE id=$1 FOR UPDATE`, snapshotID,
		).Scan(&tier, &lockedHash, &lockedSize)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRevisionNotFound
		}
		if err != nil {
			return err
		}
		if tier == SnapshotTierCold {
			return nil
		}
		if lockedHash != contentHash || lockedSize != sizeBytes {
			return ErrSnapshotIntegrity
		}
		var isCurrent bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM page p
				JOIN revision r ON r.id=p.current_revision_id
				WHERE r.content_snapshot_id=$1
			)`, snapshotID,
		).Scan(&isCurrent); err != nil {
			return err
		}
		if isCurrent {
			return nil
		}
		tag, err := tx.Exec(ctx, `
			UPDATE content_snapshot
			SET ast_json=NULL,storage_tier='cold',storage_key=$2,archived_at=now()
			WHERE id=$1 AND storage_tier='hot'`, snapshotID, key,
		)
		if err != nil {
			return fmt.Errorf("page: transition snapshot %s to cold: %w", snapshotID, err)
		}
		transitioned = tag.RowsAffected() == 1
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	return transitioned, int64(sizeBytes), nil
}

func (s *Service) hydrateSnapshot(
	ctx context.Context,
	snapshot *ContentSnapshot,
) error {
	if snapshot == nil {
		return nil
	}
	if snapshot.StorageTier == "" || snapshot.StorageTier == SnapshotTierHot {
		if len(snapshot.AST) == 0 {
			return ErrSnapshotIntegrity
		}
		return nil
	}
	if snapshot.StorageTier != SnapshotTierCold ||
		snapshot.StorageKey == "" || s.snapshotStore == nil {
		return ErrSnapshotStorageUnavailable
	}
	if snapshot.SizeBytes < 0 ||
		int64(snapshot.SizeBytes) > s.snapshotMaxBytes {
		return ErrSnapshotIntegrity
	}
	body, err := s.snapshotStore.Get(ctx, snapshot.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf(
				"%w: object missing", ErrSnapshotStorageUnavailable,
			)
		}
		return fmt.Errorf("%w: %v", ErrSnapshotStorageUnavailable, err)
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, int64(snapshot.SizeBytes)+1))
	if err != nil {
		return fmt.Errorf("%w: read object: %v", ErrSnapshotStorageUnavailable, err)
	}
	if len(raw) != snapshot.SizeBytes ||
		contentHashOf(raw) != snapshot.ContentHash {
		return ErrSnapshotIntegrity
	}
	if err := ast.ValidateJSON(raw); err != nil {
		return fmt.Errorf("%w: invalid AST", ErrSnapshotIntegrity)
	}
	snapshot.AST = raw
	return nil
}

func contentHashOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
