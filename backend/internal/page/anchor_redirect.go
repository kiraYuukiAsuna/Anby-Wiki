package page

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
)

var (
	ErrAnchorNotFound        = errors.New("page: anchor not found")
	ErrBlockRedirectLoop     = errors.New("page: block redirect loop")
	ErrBlockRedirectNotFound = errors.New("page: block redirect not found")
)

type AnchorTarget struct {
	PageID      uuid.UUID
	BlockID     uuid.UUID
	Slug        string
	ViaAlias    bool
	ViaRedirect bool
}

type BlockRedirect struct {
	SourcePageID      uuid.UUID `json:"source_page_id"`
	SourceBlockID     uuid.UUID `json:"source_block_id"`
	TargetPageID      uuid.UUID `json:"target_page_id"`
	TargetBlockID     uuid.UUID `json:"target_block_id"`
	TargetPageTitle   string    `json:"target_page_title"`
	TargetCurrentSlug string    `json:"target_current_slug"`
	CreatedBy         uuid.UUID `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
}

// CreateBlockRedirect records an explicit stable-ID migration after validating
// the actor and rejecting redirect cycles.
func (s *Service) CreateBlockRedirect(
	ctx context.Context,
	sourcePageID, sourceBlockID, targetPageID, targetBlockID, actorID uuid.UUID,
) error {
	_, err := s.UpsertBlockRedirect(
		ctx, sourcePageID, sourceBlockID, targetPageID, targetBlockID, actorID,
	)
	return err
}

// UpsertBlockRedirect is the auditable authoritative boundary used by the HTTP
// management surface. It preserves the previous edge in the audit payload.
func (s *Service) UpsertBlockRedirect(
	ctx context.Context,
	sourcePageID, sourceBlockID, targetPageID, targetBlockID, actorID uuid.UUID,
) (*BlockRedirect, error) {
	if sourcePageID == targetPageID && sourceBlockID == targetBlockID {
		return nil, ErrBlockRedirectLoop
	}
	var result *BlockRedirect
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		sourcePage, err := s.repo.GetPageByIDForUpdate(
			ctx, tx, sourcePageID,
		)
		if err != nil {
			return err
		}
		targetPage, err := s.repo.GetPageByID(ctx, tx, targetPageID)
		if err != nil {
			return err
		}
		if sourcePage.DeletedAt != nil || targetPage.DeletedAt != nil ||
			sourcePage.WikiID != targetPage.WikiID {
			return ErrAnchorNotFound
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
		resolvedTargetPage := targetPage
		if target.PageID != targetPageID {
			resolvedTargetPage, err = s.repo.GetPageByID(
				ctx, tx, target.PageID,
			)
			if err != nil {
				return err
			}
		}
		var previousPageID, previousBlockID *uuid.UUID
		err = tx.QueryRow(ctx, `SELECT target_page_id,target_block_id
			FROM block_redirect
			WHERE source_page_id=$1 AND source_block_id=$2`,
			sourcePageID, sourceBlockID,
		).Scan(&previousPageID, &previousBlockID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		redirect := &BlockRedirect{
			SourcePageID: sourcePageID, SourceBlockID: sourceBlockID,
			TargetPageID: target.PageID, TargetBlockID: target.BlockID,
			TargetPageTitle:   resolvedTargetPage.DisplayTitle,
			TargetCurrentSlug: target.Slug, CreatedBy: actorID,
		}
		err = tx.QueryRow(ctx, `INSERT INTO block_redirect
			(source_page_id,source_block_id,target_page_id,target_block_id,created_by)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (source_page_id,source_block_id) DO UPDATE SET
				target_page_id=excluded.target_page_id,
				target_block_id=excluded.target_block_id,
				created_by=excluded.created_by,
				created_at=now()
			RETURNING created_at`,
			sourcePageID, sourceBlockID,
			redirect.TargetPageID, redirect.TargetBlockID, actorID,
		).Scan(&redirect.CreatedAt)
		if err != nil {
			return err
		}
		if err := s.emitBlockRedirectAudit(
			ctx, tx, actorID, "block_redirect.updated", redirect,
			previousPageID, previousBlockID,
		); err != nil {
			return err
		}
		result = redirect
		return nil
	})
	return result, err
}

func (s *Service) ListBlockRedirects(
	ctx context.Context,
	sourcePageID uuid.UUID,
) ([]BlockRedirect, error) {
	source, err := s.repo.GetPageByID(ctx, nil, sourcePageID)
	if err != nil {
		return nil, err
	}
	if source.DeletedAt != nil {
		return nil, ErrPageNotFound
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT
		br.source_page_id,br.source_block_id,br.target_page_id,
		br.target_block_id,p.display_title,a.current_slug,
		br.created_by,br.created_at
		FROM block_redirect br
		JOIN page p ON p.id=br.target_page_id AND p.deleted_at IS NULL
		JOIN page_anchor a ON a.page_id=br.target_page_id
			AND a.heading_block_id=br.target_block_id
		WHERE br.source_page_id=$1
		ORDER BY br.created_at DESC,br.source_block_id`, sourcePageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	redirects := []BlockRedirect{}
	for rows.Next() {
		var redirect BlockRedirect
		if err := rows.Scan(
			&redirect.SourcePageID, &redirect.SourceBlockID,
			&redirect.TargetPageID, &redirect.TargetBlockID,
			&redirect.TargetPageTitle, &redirect.TargetCurrentSlug,
			&redirect.CreatedBy, &redirect.CreatedAt,
		); err != nil {
			return nil, err
		}
		redirects = append(redirects, redirect)
	}
	return redirects, rows.Err()
}

func (s *Service) DeleteBlockRedirect(
	ctx context.Context,
	sourcePageID, sourceBlockID, actorID uuid.UUID,
) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		if _, err := s.repo.GetPageByIDForUpdate(
			ctx, tx, sourcePageID,
		); err != nil {
			return err
		}
		var redirect BlockRedirect
		err := tx.QueryRow(ctx, `SELECT
			source_page_id,source_block_id,target_page_id,target_block_id,
			created_by,created_at
			FROM block_redirect
			WHERE source_page_id=$1 AND source_block_id=$2
			FOR UPDATE`, sourcePageID, sourceBlockID).Scan(
			&redirect.SourcePageID, &redirect.SourceBlockID,
			&redirect.TargetPageID, &redirect.TargetBlockID,
			&redirect.CreatedBy, &redirect.CreatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBlockRedirectNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM block_redirect
			WHERE source_page_id=$1 AND source_block_id=$2`,
			sourcePageID, sourceBlockID,
		); err != nil {
			return err
		}
		return s.emitBlockRedirectAudit(
			ctx, tx, actorID, "block_redirect.deleted",
			&redirect, &redirect.TargetPageID, &redirect.TargetBlockID,
		)
	})
}

func (s *Service) emitBlockRedirectAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	eventType string,
	redirect *BlockRedirect,
	previousPageID, previousBlockID *uuid.UUID,
) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"source_page_id":           redirect.SourcePageID,
		"source_block_id":          redirect.SourceBlockID,
		"target_page_id":           redirect.TargetPageID,
		"target_block_id":          redirect.TargetBlockID,
		"previous_target_page_id":  previousPageID,
		"previous_target_block_id": previousBlockID,
	})
	return s.repo.InsertAuditEvent(ctx, tx, &AuditEvent{
		ID: auditID, ActorID: actorID, EventType: eventType,
		AggregateType: AggregateTypePage,
		AggregateID:   redirect.SourcePageID, Payload: payload,
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
