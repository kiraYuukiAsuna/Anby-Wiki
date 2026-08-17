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
	ExpectedSequence   *int64
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
		var latestSequence int64
		if err := tx.QueryRow(ctx, `SELECT page_id,base_revision_id,status,latest_sequence
			FROM working_document WHERE id=$1 FOR UPDATE`, params.DocumentID).
			Scan(&documentPageID, &baseRevisionID, &status, &latestSequence); err != nil {
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
		if !sameSequence(params.ExpectedSequence, latestSequence) {
			expectedSequence := int64(-1)
			if params.ExpectedSequence != nil {
				expectedSequence = *params.ExpectedSequence
			}
			return fmt.Errorf(
				"%w: expected=%d actual=%d",
				ErrSequenceMismatch, expectedSequence, latestSequence,
			)
		}

		published, publishErr := p.pages.PublishInTx(ctx, tx, page.PublishParams{
			PageID: params.PageID, ActorID: params.ActorID,
			ExpectedRevisionID: params.ExpectedRevisionID,
			AST:                params.AST, Summary: params.Summary, IsMinor: params.IsMinor,
		})
		if publishErr != nil {
			return publishErr
		}
		revision = published
		if _, execErr := tx.Exec(ctx, `UPDATE working_document
			SET base_revision_id=$2, updated_at=now() WHERE id=$1`,
			params.DocumentID, revision.ID); execErr != nil {
			return execErr
		}
		auditID, idErr := p.ids.New()
		if idErr != nil {
			return idErr
		}
		if _, execErr := tx.Exec(ctx, `INSERT INTO audit_event
			(id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
			VALUES ($1,$2,'working_document.rebased','working_document',$3,
				jsonb_build_object('page_id',$4::uuid,'revision_id',$5::uuid))`,
			auditID, params.ActorID, params.DocumentID, params.PageID, revision.ID); execErr != nil {
			return execErr
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

func sameSequence(expected *int64, actual int64) bool {
	return expected != nil && *expected >= 0 && *expected == actual
}
