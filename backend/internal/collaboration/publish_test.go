package collaboration_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

func TestPublishRebasesWorkingDocumentAtomically(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	txm := db.NewTxManager(tdb.Pool)
	ids := id.NewGenerator()
	pages := page.NewService(page.NewRepository(tdb.Pool), txm, ids)
	publisher := collaboration.NewPublisher(txm, ids, pages)

	revision, err := publisher.Publish(ctx, collaboration.PublishParams{
		DocumentID: document.ID, PageID: pageID, ActorID: actorID,
		AST: validCollaborationAST("published"), Summary: "collaboration publish",
	})
	if err != nil {
		t.Fatal(err)
	}
	var currentRevisionID, baseRevisionID uuid.UUID
	var status string
	if err := tdb.Pool.QueryRow(ctx, `SELECT current_revision_id FROM page WHERE id=$1`, pageID).
		Scan(&currentRevisionID); err != nil {
		t.Fatal(err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT base_revision_id,status FROM working_document WHERE id=$1`, document.ID).
		Scan(&baseRevisionID, &status); err != nil {
		t.Fatal(err)
	}
	if currentRevisionID != revision.ID || baseRevisionID != revision.ID || status != "active" {
		t.Fatalf("revision=%s current=%s base=%s status=%s",
			revision.ID, currentRevisionID, baseRevisionID, status)
	}
	var audits int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event
		WHERE aggregate_id=$1 AND event_type='working_document.rebased'`, document.ID).
		Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("rebase audits=%d, want 1", audits)
	}
}

func TestPublishStaleBasePreservesWorkingDocument(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	txm := db.NewTxManager(tdb.Pool)
	ids := id.NewGenerator()
	pages := page.NewService(page.NewRepository(tdb.Pool), txm, ids)
	publisher := collaboration.NewPublisher(txm, ids, pages)
	if _, err := pages.Publish(ctx, page.PublishParams{
		PageID: pageID, ActorID: actorID, AST: validCollaborationAST("other"),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := publisher.Publish(ctx, collaboration.PublishParams{
		DocumentID: document.ID, PageID: pageID, ActorID: actorID,
		AST: validCollaborationAST("stale"),
	}); !errors.Is(err, page.ErrStaleRevision) {
		t.Fatalf("publish err=%v, want stale revision", err)
	}
	var baseRevisionID *uuid.UUID
	var status string
	if err := tdb.Pool.QueryRow(ctx, `SELECT base_revision_id,status
		FROM working_document WHERE id=$1`, document.ID).Scan(&baseRevisionID, &status); err != nil {
		t.Fatal(err)
	}
	if baseRevisionID != nil || status != "active" {
		t.Fatalf("base=%v status=%s, want nil/active", baseRevisionID, status)
	}
}

func TestConcurrentPublishOnlyOneRebases(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	txm := db.NewTxManager(tdb.Pool)
	ids := id.NewGenerator()
	publisher := collaboration.NewPublisher(
		txm, ids, page.NewService(page.NewRepository(tdb.Pool), txm, ids),
	)

	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range 2 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errs[index] = publisher.Publish(ctx, collaboration.PublishParams{
				DocumentID: document.ID, PageID: pageID, ActorID: actorID,
				AST: validCollaborationAST("concurrent"),
			})
		}(index)
	}
	wait.Wait()
	var succeeded, stale int
	for _, err := range errs {
		if err == nil {
			succeeded++
		} else if errors.Is(err, page.ErrStaleRevision) {
			stale++
		} else {
			t.Fatalf("unexpected publish error: %v", err)
		}
	}
	if succeeded != 1 || stale != 1 {
		t.Fatalf("succeeded=%d stale=%d", succeeded, stale)
	}
}

func validCollaborationAST(text string) json.RawMessage {
	blockID := "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4d41"
	value, _ := json.Marshal(map[string]any{
		"type": "document", "schema_version": 1,
		"children": []any{map[string]any{
			"id": blockID, "type": "paragraph",
			"content": []any{map[string]any{"type": "text", "text": text}},
		}},
	})
	return value
}
