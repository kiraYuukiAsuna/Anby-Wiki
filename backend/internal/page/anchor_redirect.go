package page

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
)

var (
	ErrAnchorNotFound    = errors.New("page: anchor not found")
	ErrBlockRedirectLoop = errors.New("page: block redirect loop")
)

type AnchorTarget struct {
	PageID      uuid.UUID
	BlockID     uuid.UUID
	Slug        string
	ViaAlias    bool
	ViaRedirect bool
}

// CreateBlockRedirect records an explicit stable-ID migration after validating
// the actor and rejecting redirect cycles.
func (s *Service) CreateBlockRedirect(
	ctx context.Context,
	sourcePageID, sourceBlockID, targetPageID, targetBlockID, actorID uuid.UUID,
) error {
	if sourcePageID == targetPageID && sourceBlockID == targetBlockID {
		return ErrBlockRedirectLoop
	}
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		// Cycle validation and mutation must observe one linearized redirect graph.
		if _, err := tx.Exec(ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended('anby:block_redirect_graph', 0))`,
		); err != nil {
			return err
		}
		target, err := resolveBlockTarget(ctx, tx, targetPageID, targetBlockID)
		if err != nil {
			return err
		}
		if target.PageID == sourcePageID && target.BlockID == sourceBlockID {
			return ErrBlockRedirectLoop
		}
		_, err = tx.Exec(ctx, `INSERT INTO block_redirect
			(source_page_id,source_block_id,target_page_id,target_block_id,created_by)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (source_page_id,source_block_id) DO UPDATE SET
				target_page_id=excluded.target_page_id,
				target_block_id=excluded.target_block_id,
				created_by=excluded.created_by,
				created_at=now()`,
			sourcePageID, sourceBlockID, targetPageID, targetBlockID, actorID)
		return err
	})
}

// ResolveAnchor resolves current slug, historical alias and any BlockRedirect
// chain to the current page/block pair.
func (s *Service) ResolveAnchor(
	ctx context.Context,
	pageID uuid.UUID,
	slug string,
) (AnchorTarget, error) {
	var blockID uuid.UUID
	viaAlias := false
	err := s.repo.pool.QueryRow(ctx, `SELECT heading_block_id
		FROM page_anchor WHERE page_id=$1 AND current_slug=$2`, pageID, slug).
		Scan(&blockID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.repo.pool.QueryRow(ctx, `SELECT heading_block_id
			FROM page_anchor_alias WHERE page_id=$1 AND alias_slug=$2`, pageID, slug).
			Scan(&blockID)
		viaAlias = err == nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return AnchorTarget{}, ErrAnchorNotFound
	}
	if err != nil {
		return AnchorTarget{}, err
	}
	target, err := resolveBlockTarget(ctx, s.repo.pool, pageID, blockID)
	if err != nil {
		return AnchorTarget{}, err
	}
	target.ViaAlias = viaAlias
	return target, nil
}

func resolveBlockTarget(
	ctx context.Context,
	query db.Querier,
	pageID, blockID uuid.UUID,
) (AnchorTarget, error) {
	seen := make(map[[32]byte]bool)
	viaRedirect := false
	for range 16 {
		key := [32]byte{}
		copy(key[:16], pageID[:])
		copy(key[16:], blockID[:])
		if seen[key] {
			return AnchorTarget{}, ErrBlockRedirectLoop
		}
		seen[key] = true
		var targetPageID, targetBlockID uuid.UUID
		err := query.QueryRow(ctx, `SELECT target_page_id,target_block_id
			FROM block_redirect WHERE source_page_id=$1 AND source_block_id=$2`,
			pageID, blockID).Scan(&targetPageID, &targetBlockID)
		if errors.Is(err, pgx.ErrNoRows) {
			var slug string
			err = query.QueryRow(ctx, `SELECT a.current_slug FROM page_anchor a
				JOIN page p ON p.id=a.page_id
				WHERE a.page_id=$1 AND a.heading_block_id=$2 AND p.deleted_at IS NULL`,
				pageID, blockID).Scan(&slug)
			if errors.Is(err, pgx.ErrNoRows) {
				return AnchorTarget{}, ErrAnchorNotFound
			}
			if err != nil {
				return AnchorTarget{}, err
			}
			return AnchorTarget{
				PageID: pageID, BlockID: blockID, Slug: slug,
				ViaRedirect: viaRedirect,
			}, nil
		}
		if err != nil {
			return AnchorTarget{}, err
		}
		pageID, blockID = targetPageID, targetBlockID
		viaRedirect = true
	}
	return AnchorTarget{}, fmt.Errorf("%w: exceeds 16 hops", ErrBlockRedirectLoop)
}
