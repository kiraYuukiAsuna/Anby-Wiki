package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

// PublishParams ties a normal Page publish to one active WorkingDocument.
type PublishParams struct {
	DocumentID         uuid.UUID
	PageID             uuid.UUID
	ActorID            uuid.UUID
	ExpectedRevisionID *uuid.UUID
	AST                json.RawMessage
	Summary            string
	IsMinor            bool
}

// Publisher composes collaboration lifecycle updates with the authoritative
// Page Service publish transaction.
type Publisher struct {
	txm   *db.TxManager
	ids   *id.Generator
	pages *page.Service
}

func NewPublisher(txm *db.TxManager, ids *id.Generator, pages *page.Service) *Publisher {
	return &Publisher{txm: txm, ids: ids, pages: pages}
}

// Publish creates a Revision and rebases the active WorkingDocument to it in
// one transaction. Any validation or stale-base failure leaves the document
// and its update log unchanged.
func (p *Publisher) Publish(ctx context.Context, params PublishParams) (*page.Revision, error) {
	var revision *page.Revision
	err := p.txm.InTx(ctx, func(tx pgx.Tx) error {
		// Keep lock order aligned with Open: Page first, WorkingDocument second.
		var currentRevisionID *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT current_revision_id FROM page
			WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, params.PageID).
			Scan(&currentRevisionID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return page.ErrPageNotFound
			}
			return err
		}

		var documentPageID uuid.UUID
		var baseRevisionID *uuid.UUID
		var status string
		if err := tx.QueryRow(ctx, `SELECT page_id,base_revision_id,status
			FROM working_document WHERE id=$1 FOR UPDATE`, params.DocumentID).
			Scan(&documentPageID, &baseRevisionID, &status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrDocumentNotFound
			}
			return err
		}
		if documentPageID != params.PageID {
			return ErrDocumentPageMismatch
		}
		if status != "active" {
			return ErrDocumentInactive
		}
		if !sameRevision(baseRevisionID, params.ExpectedRevisionID) ||
			!sameRevision(currentRevisionID, params.ExpectedRevisionID) {
			return page.ErrStaleRevision
		}

		var err error
		revision, err = p.pages.PublishInTx(ctx, tx, page.PublishParams{
			PageID: params.PageID, ActorID: params.ActorID,
			ExpectedRevisionID: params.ExpectedRevisionID,
			AST:                params.AST, Summary: params.Summary, IsMinor: params.IsMinor,
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE working_document
			SET base_revision_id=$2, updated_at=now() WHERE id=$1`,
			params.DocumentID, revision.ID); err != nil {
			return err
		}
		auditID, err := p.ids.New()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_event
			(id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
			VALUES ($1,$2,'working_document.rebased','working_document',$3,
				jsonb_build_object('page_id',$4::uuid,'revision_id',$5::uuid))`,
			auditID, params.ActorID, params.DocumentID, params.PageID, revision.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collaboration: publish working document: %w", err)
	}
	return revision, nil
}

func sameRevision(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
