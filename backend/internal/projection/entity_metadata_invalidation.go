package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type entityMetadataChangedPayload struct {
	EntityID uuid.UUID `json:"entity_id"`
}

// EntityMetadataChangedHandler invalidates every dynamic consumer of an
// Entity label or alias. It follows applied merge mappings backwards so pages
// that intentionally retain an old stable Entity ID still resolve to the
// current active identity.
type EntityMetadataChangedHandler struct {
	pool     *pgxpool.Pool
	rendered *RenderedPageBuilder
	sections *RenderedSectionsBuilder
	search   *SearchBuilder
	logger   *slog.Logger
}

func NewEntityMetadataChangedHandler(
	pool *pgxpool.Pool,
	search *SearchBuilder,
	logger *slog.Logger,
) *EntityMetadataChangedHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &EntityMetadataChangedHandler{
		pool: pool, rendered: NewRenderedPageBuilder(pool),
		sections: NewRenderedSectionsBuilder(pool), search: search,
		logger: logger,
	}
}

func (h *EntityMetadataChangedHandler) Handle(
	ctx context.Context,
	event Event,
) error {
	var payload entityMetadataChangedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("projection: decode entity metadata payload: %w", err)
	}
	if payload.EntityID == uuid.Nil {
		return fmt.Errorf("projection: entity metadata payload missing entity_id")
	}
	return h.HandleEntityIDs(ctx, event, payload.EntityID)
}

// HandleEntityIDs also serves entity.merged, whose payload names both source
// and target rather than a single entity_id.
func (h *EntityMetadataChangedHandler) HandleEntityIDs(
	ctx context.Context,
	event Event,
	entityIDs ...uuid.UUID,
) error {
	ids := make([]uuid.UUID, 0, len(entityIDs))
	seen := make(map[uuid.UUID]struct{}, len(entityIDs))
	for _, entityID := range entityIDs {
		if entityID == uuid.Nil {
			continue
		}
		if _, exists := seen[entityID]; exists {
			continue
		}
		seen[entityID] = struct{}{}
		ids = append(ids, entityID)
	}
	if len(ids) == 0 {
		return fmt.Errorf("projection: entity metadata invalidation has no entity IDs")
	}
	pageIDs, err := h.affectedPages(ctx, ids)
	if err != nil {
		return err
	}
	for _, pageID := range pageIDs {
		if err := h.rebuildDynamicPage(ctx, pageID); err != nil {
			return err
		}
		if h.search != nil {
			if err := h.search.HandlePageMetadataEvent(ctx, Event{
				ID:            event.ID,
				AggregateType: "page",
				AggregateID:   pageID,
				EventType:     event.EventType,
				Payload:       event.Payload,
			}); err != nil {
				return err
			}
		}
	}
	h.logger.Info(
		"Entity 元数据变化精准失效完成",
		slog.String("event_id", event.ID.String()),
		slog.Int("entity_count", len(ids)),
		slog.Int("affected_pages", len(pageIDs)),
	)
	return nil
}

func (h *EntityMetadataChangedHandler) affectedPages(
	ctx context.Context,
	entityIDs []uuid.UUID,
) ([]uuid.UUID, error) {
	rows, err := h.pool.Query(ctx, `
		WITH RECURSIVE referenced_entity(id, path) AS (
			SELECT seed.id, ARRAY[seed.id]
			FROM unnest($1::uuid[]) AS seed(id)
			UNION ALL
			SELECT merge.source_entity_id, refs.path || merge.source_entity_id
			FROM referenced_entity refs
			JOIN entity_merge merge
			  ON merge.target_entity_id = refs.id
			 AND merge.status = 'applied'
			WHERE NOT merge.source_entity_id = ANY(refs.path)
		),
		affected AS (
			SELECT p.id AS page_id
			FROM page p
			WHERE p.primary_entity_id IN (SELECT id FROM referenced_entity)
			  AND p.deleted_at IS NULL
			UNION
			SELECT dependency.page_id
			FROM component_dependency dependency
			JOIN page p ON p.id = dependency.page_id
			WHERE dependency.entity_id IN (SELECT id FROM referenced_entity)
			  AND p.current_revision_id = dependency.revision_id
			  AND p.deleted_at IS NULL
			UNION
			SELECT usage.page_id
			FROM claim_usage usage
			JOIN claim c ON c.id = usage.claim_id
			JOIN page p ON p.id = usage.page_id
			WHERE c.target_entity_id IN (SELECT id FROM referenced_entity)
			  AND p.current_revision_id = usage.revision_id
			  AND p.deleted_at IS NULL
		)
		SELECT DISTINCT page_id
		FROM affected
		ORDER BY page_id`,
		entityIDs,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"projection: query pages affected by Entity metadata: %w", err,
		)
	}
	defer rows.Close()
	var pageIDs []uuid.UUID
	for rows.Next() {
		var pageID uuid.UUID
		if err := rows.Scan(&pageID); err != nil {
			return nil, err
		}
		pageIDs = append(pageIDs, pageID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pageIDs, nil
}

func (h *EntityMetadataChangedHandler) rebuildDynamicPage(
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
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE`,
		pageID,
	).Scan(&revisionID); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	if revisionID == nil {
		return nil
	}
	for _, builder := range []Builder{h.rendered, h.sections} {
		if err := builder.Rebuild(
			ctx, tx, pageID, *revisionID,
		); err != nil {
			return fmt.Errorf(
				"projection: Entity metadata rebuild %s for page %s: %w",
				builder.Type(), pageID, err,
			)
		}
	}
	return tx.Commit(ctx)
}
