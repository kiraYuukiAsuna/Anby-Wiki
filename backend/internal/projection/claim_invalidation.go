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

type claimChangedPayload struct {
	ClaimID            uuid.UUID  `json:"claim_id"`
	SubjectEntityID    uuid.UUID  `json:"subject_entity_id"`
	ReplacementClaimID *uuid.UUID `json:"replacement_claim_id,omitempty"`
}

// ClaimChangedHandler 只重建 claim_usage/component_dependency 命中的页面渲染。
// 定位查询只读投影表，不扫描 AST JSON。
type ClaimChangedHandler struct {
	pool         *pgxpool.Pool
	claimUsage   *KnowledgeUsageBuilder
	dependencies *ComponentDependencyBuilder
	rendered     *RenderedPageBuilder
	logger       *slog.Logger
}

func NewClaimChangedHandler(pool *pgxpool.Pool, logger *slog.Logger) *ClaimChangedHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClaimChangedHandler{
		pool:         pool,
		claimUsage:   NewClaimUsageBuilder(pool),
		dependencies: NewComponentDependencyBuilder(pool),
		rendered:     NewRenderedPageBuilder(pool),
		logger:       logger,
	}
}

func (h *ClaimChangedHandler) Handle(ctx context.Context, event Event) error {
	var payload claimChangedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("projection: decode claim.changed payload: %w", err)
	}
	if payload.ClaimID == uuid.Nil || payload.SubjectEntityID == uuid.Nil {
		return fmt.Errorf("projection: claim.changed payload missing stable IDs")
	}
	claimIDs := []uuid.UUID{payload.ClaimID}
	if payload.ReplacementClaimID != nil {
		claimIDs = append(claimIDs, *payload.ReplacementClaimID)
	}
	pageIDs, err := h.affectedPages(ctx, claimIDs, payload.SubjectEntityID)
	if err != nil {
		return err
	}
	for _, pageID := range pageIDs {
		if err := h.rebuildPage(ctx, pageID); err != nil {
			return err
		}
	}
	h.logger.Info("Claim 变化精准重渲染完成",
		slog.String("event_id", event.ID.String()),
		slog.String("claim_id", payload.ClaimID.String()),
		slog.Int("affected_pages", len(pageIDs)),
	)
	return nil
}

func (h *ClaimChangedHandler) affectedPages(
	ctx context.Context, claimIDs []uuid.UUID, entityID uuid.UUID,
) ([]uuid.UUID, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT DISTINCT affected.page_id
		FROM (
			SELECT cu.page_id
			FROM claim_usage cu
			JOIN page p ON p.id = cu.page_id
			WHERE cu.claim_id = ANY($1::uuid[])
			  AND p.current_revision_id = cu.revision_id
			  AND p.deleted_at IS NULL
			UNION
			SELECT cd.page_id
			FROM component_dependency cd
			JOIN page p ON p.id = cd.page_id
			WHERE cd.entity_id = $2
			  AND p.current_revision_id = cd.revision_id
			  AND p.deleted_at IS NULL
		) affected
		ORDER BY affected.page_id`, claimIDs, entityID)
	if err != nil {
		return nil, fmt.Errorf("projection: query pages affected by claim change: %w", err)
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

func (h *ClaimChangedHandler) rebuildPage(ctx context.Context, pageID uuid.UUID) error {
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
		FOR UPDATE`, pageID).Scan(&revisionID); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	if revisionID == nil {
		return nil
	}
	for _, builder := range []Builder{h.dependencies, h.claimUsage, h.rendered} {
		if err := builder.Rebuild(ctx, tx, pageID, *revisionID); err != nil {
			return fmt.Errorf(
				"projection: claim change rebuild %s for page %s: %w",
				builder.Type(), pageID, err,
			)
		}
	}
	return tx.Commit(ctx)
}
