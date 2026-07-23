package page_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestResolveHistoricalAnchorThroughBlockRedirect(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "anchor-migrator")
	service := page.NewService(
		page.NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator(),
	)
	sourcePage := tdb.MakePage(t, testkit.MainNamespaceID, "source-anchor", "Source Anchor", actorID)
	targetPage := tdb.MakePage(t, testkit.MainNamespaceID, "target-anchor", "Target Anchor", actorID)
	sourceBlock := tdb.NewID(t)
	targetBlock := tdb.NewID(t)
	sourceRevision := publishHeading(t, service, sourcePage, sourceBlock, actorID, "Old")
	targetRevision := publishHeading(t, service, targetPage, targetBlock, actorID, "New")
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO page_anchor
		(page_id,revision_id,heading_block_id,level,title,current_slug,position_key)
		VALUES ($1,$2,$3,1,'New','new','1')`,
		targetPage, targetRevision, targetBlock); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO page_anchor_alias
		(page_id,alias_slug,heading_block_id,source_revision_id)
		VALUES ($1,'old',$2,$3)`, sourcePage, sourceBlock, sourceRevision); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateBlockRedirect(
		ctx, sourcePage, sourceBlock, targetPage, targetBlock, actorID,
	); err != nil {
		t.Fatal(err)
	}
	target, err := service.ResolveAnchor(ctx, sourcePage, "old")
	if err != nil {
		t.Fatal(err)
	}
	if target.PageID != targetPage || target.BlockID != targetBlock || target.Slug != "new" {
		t.Fatalf("target=%+v", target)
	}
	if !target.ViaAlias || !target.ViaRedirect {
		t.Fatalf("resolution metadata=%+v", target)
	}
	if err := service.CreateBlockRedirect(
		ctx, targetPage, targetBlock, sourcePage, sourceBlock, actorID,
	); !errors.Is(err, page.ErrBlockRedirectLoop) {
		t.Fatalf("cycle err=%v", err)
	}
	if err := service.CreateBlockRedirect(
		ctx, sourcePage, tdb.NewID(t), targetPage, tdb.NewID(t), actorID,
	); !errors.Is(err, page.ErrAnchorNotFound) {
		t.Fatalf("dangling target err=%v", err)
	}
}

func TestCreateBlockRedirectSerializesOppositeConcurrentWrites(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "concurrent-anchor-migrator")
	service := page.NewService(
		page.NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator(),
	)
	pageA := tdb.MakePage(t, testkit.MainNamespaceID, "redirect-a", "Redirect A", actorID)
	pageB := tdb.MakePage(t, testkit.MainNamespaceID, "redirect-b", "Redirect B", actorID)
	blockA := tdb.NewID(t)
	blockB := tdb.NewID(t)
	revisionA := publishHeading(t, service, pageA, blockA, actorID, "A")
	revisionB := publishHeading(t, service, pageB, blockB, actorID, "B")
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO page_anchor
		(page_id,revision_id,heading_block_id,level,title,current_slug,position_key)
		VALUES ($1,$2,$3,1,'A','a','1'),($4,$5,$6,1,'B','b','1')`,
		pageA, revisionA, blockA, pageB, revisionB, blockB); err != nil {
		t.Fatal(err)
	}

	// Hold the graph lock so both requests queue before either validates the graph.
	blocker, err := tdb.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	if _, err := blocker.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended('anby:block_redirect_graph', 0))`,
	); err != nil {
		t.Fatal(err)
	}

	type result struct {
		direction string
		err       error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	run := func(direction string, sourcePage, sourceBlock, targetPage, targetBlock uuid.UUID) {
		ready.Done()
		<-start
		results <- result{
			direction: direction,
			err: service.CreateBlockRedirect(
				ctx, sourcePage, sourceBlock, targetPage, targetBlock, actorID,
			),
		}
	}
	go run("A->B", pageA, blockA, pageB, blockB)
	go run("B->A", pageB, blockB, pageA, blockA)
	ready.Wait()
	close(start)

	select {
	case got := <-results:
		t.Fatalf("%s completed before the redirect graph lock was released: %v", got.direction, got.err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	var success, loops int
	for range 2 {
		got := <-results
		switch {
		case got.err == nil:
			success++
		case errors.Is(got.err, page.ErrBlockRedirectLoop):
			loops++
		default:
			t.Fatalf("%s returned unexpected error: %v", got.direction, got.err)
		}
	}
	if success != 1 || loops != 1 {
		t.Fatalf("success=%d loop_errors=%d, want exactly one of each", success, loops)
	}

	var redirectCount int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM block_redirect`).Scan(&redirectCount); err != nil {
		t.Fatal(err)
	}
	if redirectCount != 1 {
		t.Fatalf("redirect rows=%d, want 1", redirectCount)
	}

	resolvedA, err := service.ResolveAnchor(ctx, pageA, "a")
	if err != nil {
		t.Fatalf("resolve A after concurrent writes: %v", err)
	}
	resolvedB, err := service.ResolveAnchor(ctx, pageB, "b")
	if err != nil {
		t.Fatalf("resolve B after concurrent writes: %v", err)
	}
	if resolvedA.ViaRedirect == resolvedB.ViaRedirect {
		t.Fatalf("resolution flags A=%+v B=%+v, want exactly one redirect", resolvedA, resolvedB)
	}
}

func publishHeading(
	t *testing.T,
	service *page.Service,
	pageID, blockID, actorID uuid.UUID,
	title string,
) uuid.UUID {
	t.Helper()
	ast, _ := json.Marshal(map[string]any{
		"type": "document", "schema_version": 1,
		"children": []any{map[string]any{
			"id": blockID, "type": "heading", "level": 1,
			"content": []any{map[string]any{"type": "text", "text": title}},
		}},
	})
	revision, err := service.Publish(context.Background(), page.PublishParams{
		PageID: pageID, ActorID: actorID, AST: ast,
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision.ID
}
