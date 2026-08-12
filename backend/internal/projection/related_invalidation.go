package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RelatedPagesInvalidator translates non-Revision relationship events into
// bounded page projection rebuilds. The source tables stay authoritative; a
// failed rebuild is safely retryable through the same Outbox event.
type RelatedPagesInvalidator struct {
	pool    *pgxpool.Pool
	builder *RelatedPagesBuilder
}

func NewRelatedPagesInvalidator(pool *pgxpool.Pool) *RelatedPagesInvalidator {
	return &RelatedPagesInvalidator{
		pool: pool, builder: NewRelatedPagesBuilder(pool),
	}
}

func (h *RelatedPagesInvalidator) HandlePublishedPage(
	ctx context.Context,
	pageID uuid.UUID,
) error {
	rows, err := h.pool.Query(ctx, `
		SELECT DISTINCT candidate.page_id
		FROM (
		  SELECT $1::uuid AS page_id
		  UNION
		  SELECT link.target_page_id
		  FROM page_link_projection link
		  JOIN page source ON source.id=link.source_page_id
		  WHERE link.source_page_id=$1
		    AND link.source_revision_id=source.current_revision_id
		    AND link.resolution_status='resolved'
		  UNION
		  SELECT link.source_page_id
		  FROM page_link_projection link
		  JOIN page source ON source.id=link.source_page_id
		  WHERE link.target_page_id=$1
		    AND link.source_revision_id=source.current_revision_id
		    AND link.resolution_status='resolved'
		  UNION
		  SELECT related.source_page_id
		  FROM page_related_projection related
		  WHERE related.target_page_id=$1
		) candidate
		JOIN page ON page.id=candidate.page_id
		WHERE page.deleted_at IS NULL
		  AND page.current_revision_id IS NOT NULL
		ORDER BY candidate.page_id`, pageID)
	if err != nil {
		return fmt.Errorf("projection: query pages affected by published links: %w", err)
	}
	pageIDs, err := scanUUIDRows(rows)
	if err != nil {
		return err
	}
	return h.rebuildPages(ctx, pageIDs)
}

func (h *RelatedPagesInvalidator) HandleCollectionEvent(
	ctx context.Context,
	event Event,
) error {
	var payload struct {
		CollectionID uuid.UUID   `json:"collection_id"`
		PageID       *uuid.UUID  `json:"page_id"`
		EntityID     *uuid.UUID  `json:"entity_id"`
		PageIDs      []uuid.UUID `json:"page_ids"`
		EntityIDs    []uuid.UUID `json:"entity_ids"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("projection: decode Collection invalidation: %w", err)
	}
	if payload.CollectionID == uuid.Nil {
		return fmt.Errorf("projection: Collection invalidation missing collection_id")
	}
	rows, err := h.pool.Query(ctx, `
		SELECT DISTINCT affected.page_id
		FROM (
		  SELECT membership.page_id
		  FROM collection_membership membership
		  WHERE membership.collection_id=$1
		    AND membership.member_type='page'
		  UNION
		  SELECT page.id
		  FROM collection_membership membership
		  JOIN page ON page.primary_entity_id=membership.entity_id
		  WHERE membership.collection_id=$1
		    AND membership.member_type='entity'
		  UNION
		  SELECT $2::uuid
		  UNION
		  SELECT page.id FROM page WHERE page.primary_entity_id=$3
		) affected
		JOIN page ON page.id=affected.page_id
		WHERE affected.page_id IS NOT NULL
		  AND page.deleted_at IS NULL
		  AND page.current_revision_id IS NOT NULL
		ORDER BY affected.page_id`,
		payload.CollectionID, payload.PageID, payload.EntityID)
	if err != nil {
		return fmt.Errorf("projection: query Collection-related pages: %w", err)
	}
	pageIDs, err := scanUUIDRows(rows)
	if err != nil {
		return err
	}
	pageIDs = append(pageIDs, payload.PageIDs...)
	entityIDs := append([]uuid.UUID{}, payload.EntityIDs...)
	if payload.EntityID != nil {
		entityIDs = append(entityIDs, *payload.EntityID)
	}
	entityPages, err := h.pagesAffectedByEntities(ctx, entityIDs)
	if err != nil {
		return err
	}
	pageIDs = append(pageIDs, entityPages...)
	return h.rebuildPages(ctx, pageIDs)
}

func (h *RelatedPagesInvalidator) HandleBindingEvent(
	ctx context.Context,
	event Event,
) error {
	var payload struct {
		PageID           uuid.UUID  `json:"page_id"`
		EntityID         uuid.UUID  `json:"entity_id"`
		PreviousEntityID *uuid.UUID `json:"previous_entity_id"`
		RestoredEntityID *uuid.UUID `json:"restored_entity_id"`
		PreviousBinding  *struct {
			EntityID uuid.UUID `json:"entity_id"`
		} `json:"previous_binding"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("projection: decode page binding invalidation: %w", err)
	}
	if payload.PageID == uuid.Nil {
		payload.PageID = event.AggregateID
	}
	entityIDs := []uuid.UUID{payload.EntityID}
	if payload.PreviousEntityID != nil {
		entityIDs = append(entityIDs, *payload.PreviousEntityID)
	}
	if payload.RestoredEntityID != nil {
		entityIDs = append(entityIDs, *payload.RestoredEntityID)
	}
	if payload.PreviousBinding != nil {
		entityIDs = append(entityIDs, payload.PreviousBinding.EntityID)
	}
	pageIDs, err := h.pagesAffectedByEntities(ctx, entityIDs)
	if err != nil {
		return err
	}
	pageIDs = append(pageIDs, payload.PageID)
	peers, err := h.SnapshotPagePeers(ctx, payload.PageID)
	if err != nil {
		return err
	}
	pageIDs = append(pageIDs, peers...)
	return h.rebuildPages(ctx, uniqueSortedUUIDs(pageIDs))
}

// HandleEntityIDs refreshes pages whose primary subject is one of the changed
// entities, one graph hop away, or shares a materialized Collection with it.
func (h *RelatedPagesInvalidator) HandleEntityIDs(
	ctx context.Context,
	entityIDs ...uuid.UUID,
) error {
	pageIDs, err := h.pagesAffectedByEntities(ctx, entityIDs)
	if err != nil {
		return err
	}
	return h.rebuildPages(ctx, pageIDs)
}

func (h *RelatedPagesInvalidator) pagesAffectedByEntities(
	ctx context.Context,
	entityIDs []uuid.UUID,
) ([]uuid.UUID, error) {
	entityIDs = nonNilUUIDs(entityIDs)
	if len(entityIDs) == 0 {
		return nil, nil
	}
	rows, err := h.pool.Query(ctx, `
		WITH entities AS (
		  SELECT unnest($1::uuid[]) AS id
		  UNION
		  SELECT CASE
		    WHEN edge.subject_entity_id=ANY($1::uuid[]) THEN edge.target_entity_id
		    ELSE edge.subject_entity_id
		  END
		  FROM entity_edge_projection edge
		  WHERE edge.subject_entity_id=ANY($1::uuid[])
		     OR edge.target_entity_id=ANY($1::uuid[])
		), collections AS (
		  SELECT DISTINCT membership.collection_id
		  FROM collection_membership membership
		  WHERE membership.entity_id IN (SELECT id FROM entities)
		), affected AS (
		  SELECT page.id
		  FROM page
		  WHERE page.primary_entity_id IN (SELECT id FROM entities)
		  UNION
		  SELECT membership.page_id
		  FROM collection_membership membership
		  WHERE membership.collection_id IN (SELECT collection_id FROM collections)
		    AND membership.member_type='page'
		  UNION
		  SELECT page.id
		  FROM collection_membership membership
		  JOIN page ON page.primary_entity_id=membership.entity_id
		  WHERE membership.collection_id IN (SELECT collection_id FROM collections)
		    AND membership.member_type='entity'
		  UNION
		  SELECT related.source_page_id
		  FROM page_related_projection related
		  JOIN page target ON target.id=related.target_page_id
		  WHERE target.primary_entity_id IN (SELECT id FROM entities)
		  UNION
		  SELECT related.target_page_id
		  FROM page_related_projection related
		  JOIN page source ON source.id=related.source_page_id
		  WHERE source.primary_entity_id IN (SELECT id FROM entities)
		)
		SELECT DISTINCT affected.id
		FROM affected
		JOIN page ON page.id=affected.id
		WHERE page.deleted_at IS NULL
		  AND page.current_revision_id IS NOT NULL
		ORDER BY affected.id`, entityIDs)
	if err != nil {
		return nil, fmt.Errorf("projection: query Entity-related pages: %w", err)
	}
	return scanUUIDRows(rows)
}

// SnapshotPagePeers captures currently materialized recommendation neighbors
// before a page binding/deletion mutates or clears those disposable rows.
func (h *RelatedPagesInvalidator) SnapshotPagePeers(
	ctx context.Context,
	pageIDs ...uuid.UUID,
) ([]uuid.UUID, error) {
	pageIDs = nonNilUUIDs(pageIDs)
	if len(pageIDs) == 0 {
		return nil, nil
	}
	rows, err := h.pool.Query(ctx, `
		SELECT DISTINCT peer.page_id
		FROM (
		  SELECT source_page_id AS page_id
		  FROM page_related_projection
		  WHERE target_page_id=ANY($1::uuid[])
		  UNION
		  SELECT target_page_id
		  FROM page_related_projection
		  WHERE source_page_id=ANY($1::uuid[])
		) peer
		JOIN page ON page.id=peer.page_id
		WHERE page.deleted_at IS NULL
		  AND page.current_revision_id IS NOT NULL
		ORDER BY peer.page_id`, pageIDs)
	if err != nil {
		return nil, fmt.Errorf("projection: snapshot related-page peers: %w", err)
	}
	return scanUUIDRows(rows)
}

// RebuildPageIDs refreshes an explicit bounded set captured before another
// projection cleanup. It is safe to call with deleted/unpublished IDs.
func (h *RelatedPagesInvalidator) RebuildPageIDs(
	ctx context.Context,
	pageIDs ...uuid.UUID,
) error {
	return h.rebuildPages(ctx, pageIDs)
}

func (h *RelatedPagesInvalidator) rebuildPages(
	ctx context.Context,
	pageIDs []uuid.UUID,
) error {
	for _, pageID := range uniqueSortedUUIDs(pageIDs) {
		if err := h.rebuildPage(ctx, pageID); err != nil {
			return err
		}
	}
	return nil
}

func (h *RelatedPagesInvalidator) rebuildPage(
	ctx context.Context,
	pageID uuid.UUID,
) error {
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var revisionID *uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT current_revision_id
		FROM page
		WHERE id=$1 AND deleted_at IS NULL
		FOR UPDATE`, pageID).Scan(&revisionID); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	if revisionID == nil {
		return nil
	}
	if err := h.builder.Rebuild(ctx, tx, pageID, *revisionID); err != nil {
		return err
	}
	if err := UpsertState(
		ctx, tx,
		OKState(AggregateTypePage, pageID, h.builder.Type(), *revisionID),
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanUUIDRows(rows pgx.Rows) ([]uuid.UUID, error) {
	defer rows.Close()
	var values []uuid.UUID
	for rows.Next() {
		var value uuid.UUID
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func nonNilUUIDs(values []uuid.UUID) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value != uuid.Nil {
			result = append(result, value)
		}
	}
	return uniqueSortedUUIDs(result)
}

func uniqueSortedUUIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(values))
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].String() < result[j].String()
	})
	return result
}
