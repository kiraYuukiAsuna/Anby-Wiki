package collaboration_test

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func setup(t *testing.T) (*testkit.DB, *collaboration.Service, uuid.UUID, uuid.UUID) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	actorID := tdb.MakeActor(t, "human", "collaborator")
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "working-document", "Working Document", actorID)
	service := collaboration.NewService(tdb.Pool, db.NewTxManager(tdb.Pool), id.NewGenerator())
	return tdb, service, pageID, actorID
}

func TestOpenIsPerPageAndAudited(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()

	first, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.PageID != pageID || first.CRDTCodec != "yjs-v1" {
		t.Fatalf("unexpected documents: first=%+v second=%+v", first, second)
	}
	var documents, audits int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM working_document WHERE page_id=$1`, pageID).
		Scan(&documents); err != nil {
		t.Fatal(err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event
		WHERE aggregate_id=$1 AND event_type='working_document.created'`, first.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if documents != 1 || audits != 1 {
		t.Fatalf("documents=%d audits=%d, want 1/1", documents, audits)
	}
}

func TestAppendAllocatesSequenceAndIsIdempotent(t *testing.T) {
	_, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	clientID, updateID := uuid.New(), uuid.New()
	first, err := service.Append(ctx, document.ID, actorID, clientID, updateID, []byte("update-1"))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Append(ctx, document.ID, actorID, clientID, updateID, []byte("update-1"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 || replayed.Sequence != first.Sequence || !bytes.Equal(replayed.Bytes, first.Bytes) {
		t.Fatalf("first=%+v replayed=%+v", first, replayed)
	}
	if _, err := service.Append(
		ctx, document.ID, actorID, clientID, updateID, []byte("different"),
	); !errors.Is(err, collaboration.ErrIdempotencyConflict) {
		t.Fatalf("idempotency mismatch err=%v", err)
	}
}

func TestConcurrentAppendAllocatesContiguousSequence(t *testing.T) {
	_, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}

	const count = 12
	sequences := make([]int, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for index := range count {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			update, err := service.Append(
				ctx, document.ID, actorID, uuid.New(), uuid.New(), []byte{byte(index + 1)},
			)
			errs[index] = err
			sequences[index] = int(update.Sequence)
		}(index)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Ints(sequences)
	for index, sequence := range sequences {
		if sequence != index+1 {
			t.Fatalf("sequences=%v", sequences)
		}
	}
}

func TestConcurrentAppendCASOnlyOneExpectedSequenceWins(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range 2 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			errs[index] = db.NewTxManager(tdb.Pool).InTx(ctx, func(tx pgx.Tx) error {
				_, err := service.AppendCASInTx(
					ctx, tx, document.ID, pageID, actorID, uuid.New(), uuid.New(),
					0, []byte{byte(index + 1)},
				)
				return err
			})
		}(index)
	}
	wait.Wait()
	var success, mismatch int
	for _, err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, collaboration.ErrSequenceMismatch):
			mismatch++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if success != 1 || mismatch != 1 {
		t.Fatalf("success=%d mismatch=%d errors=%v", success, mismatch, errs)
	}
}

func TestAppendRejectsDeletedPageAndOversizedUpdate(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Append(
		ctx, document.ID, actorID, uuid.New(), uuid.New(),
		make([]byte, collaboration.MaxUpdateBytes+1),
	); !errors.Is(err, collaboration.ErrInvalidUpdate) {
		t.Fatalf("oversized err=%v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE page SET deleted_at=now() WHERE id=$1`, pageID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Append(
		ctx, document.ID, actorID, uuid.New(), uuid.New(), []byte("after-delete"),
	); !errors.Is(err, collaboration.ErrDocumentNotFound) {
		t.Fatalf("deleted page append err=%v", err)
	}
	var updates int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM working_document_update
		WHERE document_id=$1`, document.ID).Scan(&updates)
	if updates != 0 {
		t.Fatalf("updates=%d", updates)
	}
}

func TestNearLimitUpdateSurvivesServiceRestart(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0x5a}, collaboration.MaxUpdateBytes-1)
	if _, err := service.Append(
		ctx, document.ID, actorID, uuid.New(), uuid.New(), payload,
	); err != nil {
		t.Fatal(err)
	}
	restarted := collaboration.NewService(
		tdb.Pool, db.NewTxManager(tdb.Pool), id.NewGenerator(),
	)
	recovery, err := restarted.LoadSince(ctx, document.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovery.Updates) != 1 {
		t.Fatalf("recovered updates=%d", len(recovery.Updates))
	}
	if !bytes.Equal(recovery.Updates[0].Bytes, payload) {
		t.Fatalf("recovered bytes=%d", len(recovery.Updates[0].Bytes))
	}
}

func TestSnapshotCompactionAndRecovery(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 3; index++ {
		if _, err := service.Append(
			ctx, document.ID, actorID, uuid.New(), uuid.New(), []byte{byte(index)},
		); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := service.SaveSnapshot(
		ctx, document.ID, actorID, 2, []byte("state-through-2"), true,
	)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := service.LoadRecovery(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Snapshot == nil || recovery.Snapshot.ID != snapshot.ID ||
		recovery.Snapshot.UpToSequence != 2 {
		t.Fatalf("snapshot=%+v", recovery.Snapshot)
	}
	if len(recovery.Updates) != 1 || recovery.Updates[0].Sequence != 3 {
		t.Fatalf("updates=%+v", recovery.Updates)
	}
	var remaining, audits int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM working_document_update
		WHERE document_id=$1`, document.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event
		WHERE aggregate_id=$1 AND event_type='working_document.snapshotted'`, document.ID).
		Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 || audits != 1 {
		t.Fatalf("remaining=%d audits=%d", remaining, audits)
	}
}

func TestWriteValidationAndActorBoundary(t *testing.T) {
	tdb, service, pageID, actorID := setup(t)
	ctx := context.Background()
	document, err := service.Open(ctx, pageID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	aiID := tdb.MakeActor(t, "ai", "assistant")
	if _, err := service.Append(
		ctx, document.ID, aiID, uuid.New(), uuid.New(), []byte("forbidden"),
	); !errors.Is(err, collaboration.ErrInvalidActor) {
		t.Fatalf("AI append err=%v", err)
	}
	if _, err := service.Append(
		ctx, document.ID, actorID, uuid.New(), uuid.New(), nil,
	); !errors.Is(err, collaboration.ErrInvalidUpdate) {
		t.Fatalf("empty update err=%v", err)
	}
	if _, err := service.SaveSnapshot(
		ctx, document.ID, actorID, 1, []byte("future"), false,
	); !errors.Is(err, collaboration.ErrInvalidSnapshot) {
		t.Fatalf("future snapshot err=%v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE working_document SET status='closed' WHERE id=$1`, document.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Append(
		ctx, document.ID, actorID, uuid.New(), uuid.New(), []byte("closed"),
	); !errors.Is(err, collaboration.ErrDocumentInactive) {
		t.Fatalf("closed append err=%v", err)
	}
}
