package projection

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PageDeletedHandler removes only disposable, current-page projections. The
// Page, Revision, Snapshot, aliases and audit history remain intact.
type PageDeletedHandler struct {
	pool *pgxpool.Pool
}

func NewPageDeletedHandler(pool *pgxpool.Pool) *PageDeletedHandler {
	return &PageDeletedHandler{pool: pool}
}

func (h *PageDeletedHandler) Handle(
	ctx context.Context,
	event Event,
) error {
	if event.AggregateType != "page" ||
		event.EventType != "page.deleted" ||
		event.AggregateID == uuid.Nil {
		return fmt.Errorf("projection: invalid page.deleted envelope")
	}
	var payload struct {
		PageID uuid.UUID `json:"page_id"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("projection: decode page.deleted payload: %w", err)
	}
	if payload.PageID != uuid.Nil && payload.PageID != event.AggregateID {
		return fmt.Errorf("projection: page.deleted aggregate mismatch")
	}

	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("projection: begin page deletion cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	deletions := []struct {
		table  string
		column string
	}{
		{"page_link_projection", "source_page_id"},
		{"document_outline_projection", "page_id"},
		{"page_anchor", "page_id"},
		{"page_anchor_alias", "page_id"},
		{"rendered_page", "page_id"},
		{"rendered_section", "page_id"},
		{"external_link_usage", "page_id"},
		{"entity_mention_projection", "page_id"},
		{"claim_usage", "page_id"},
		{"citation_usage", "page_id"},
		{"component_dependency", "page_id"},
		{"search_document", "page_id"},
	}
	for _, deletion := range deletions {
		if _, err := tx.Exec(
			ctx,
			"DELETE FROM "+deletion.table+" WHERE "+deletion.column+"=$1",
			event.AggregateID,
		); err != nil {
			return fmt.Errorf(
				"projection: clear %s for deleted page %s: %w",
				deletion.table, event.AggregateID, err,
			)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM projection_state
		WHERE aggregate_type='page' AND aggregate_id=$1`,
		event.AggregateID,
	); err != nil {
		return fmt.Errorf(
			"projection: clear state for deleted page %s: %w",
			event.AggregateID, err,
		)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("projection: commit page deletion cleanup: %w", err)
	}
	return nil
}
