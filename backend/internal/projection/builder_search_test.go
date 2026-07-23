package projection_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/anby/wiki/backend/internal/projection"
	wikisearch "github.com/anby/wiki/backend/internal/search"
	"github.com/anby/wiki/backend/testkit"
	"github.com/google/uuid"
)

func TestSearchBuilder_RebuildsCurrentRevisionMetadataAndBody(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()
	pageID := d.MakePage(t, testkit.MainNamespaceID, "search-builder", "Search Builder", testkit.SystemActorID)
	snapshotID := d.NewID(t)
	revisionID := d.NewID(t)
	aliasID := d.NewID(t)
	astJSON := `{"type":"document","schema_version":1,"children":[
		{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4201","type":"heading","level":2,
		 "content":[{"type":"text","text":"Builder Heading"}]},
		{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4202","type":"code","language":"go","content":"projection token"}
	]}`
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO content_snapshot (id, schema_version, ast_json, content_hash, size_bytes)
		VALUES ($1, 1, $2::jsonb, $3, $4)`,
		snapshotID, astJSON, snapshotID.String(), len(astJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO revision (id, page_id, content_snapshot_id, actor_id)
		VALUES ($1, $2, $3, $4)`, revisionID, pageID, snapshotID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx,
		`UPDATE page SET current_revision_id = $2 WHERE id = $1`, pageID, revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO page_alias (id, wiki_id, namespace_id, normalized_title, page_id, alias_type)
		VALUES ($1, $2, $3, 'old-builder', $4, 'rename')`,
		aliasID, testkit.DefaultWikiID, testkit.MainNamespaceID, pageID); err != nil {
		t.Fatal(err)
	}

	registry := projection.NewRegistry()
	registry.Register(projection.NewSearchBuilder(d.Pool, wikisearch.NewPostgresAdapter(d.Pool)))
	rebuilder := projection.NewRebuilder(d.Pool, registry, nil)
	rebuilt, err := rebuilder.RebuildPage(ctx, pageID)
	if err != nil || !rebuilt {
		t.Fatalf("RebuildPage = (%v, %v)", rebuilt, err)
	}

	var sourceRevision, title, aliases, body string
	if err := d.Pool.QueryRow(ctx, `
		SELECT source_revision_id::text, display_title, array_to_string(aliases, ','), body_text
		FROM search_document WHERE page_id = $1`, pageID).
		Scan(&sourceRevision, &title, &aliases, &body); err != nil {
		t.Fatal(err)
	}
	if sourceRevision != revisionID.String() || title != "Search Builder" ||
		aliases != "old-builder" || body != "Builder Heading projection token" {
		t.Fatalf("unexpected search document: revision=%s title=%q aliases=%q body=%q",
			sourceRevision, title, aliases, body)
	}

	if _, err := d.Pool.Exec(ctx, `DELETE FROM search_document WHERE page_id = $1`, pageID); err != nil {
		t.Fatal(err)
	}
	report, err := rebuilder.RebuildAll(ctx)
	if err != nil || report.Total != 1 || report.Rebuilt != 1 || report.Failed != 0 {
		t.Fatalf("RebuildAll = (%+v, %v)", report, err)
	}
	if err := d.Pool.QueryRow(ctx,
		`SELECT source_revision_id FROM search_document WHERE page_id = $1`, pageID).
		Scan(&sourceRevision); err != nil {
		t.Fatal(err)
	}
	if sourceRevision != revisionID.String() {
		t.Fatalf("full rebuild source revision = %s, want %s", sourceRevision, revisionID)
	}
}

func TestSearchBuilder_RemoteRetryAndOldRevisionCannotOverwrite(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()
	pageID := d.MakePage(t, testkit.MainNamespaceID, "remote-order", "Remote Order", testkit.SystemActorID)
	oldRevision := insertSearchRevision(t, d, pageID, "old body")
	if _, err := d.Pool.Exec(ctx,
		`UPDATE page SET current_revision_id = $2 WHERE id = $1`, pageID, oldRevision); err != nil {
		t.Fatal(err)
	}

	remote := &recordingSearchAdapter{fail: true}
	builder := projection.NewSearchBuilder(d.Pool, remote)
	oldEvent := searchRevisionEvent(pageID, oldRevision)
	if err := builder.HandleEvent(ctx, oldEvent); err == nil {
		t.Fatal("remote failure must return an error for Outbox retry")
	}
	var stagedRevision uuid.UUID
	if err := d.Pool.QueryRow(ctx,
		`SELECT source_revision_id FROM search_document WHERE page_id=$1`, pageID).
		Scan(&stagedRevision); err != nil || stagedRevision != oldRevision {
		t.Fatalf("staging must commit before remote retry: revision=%s err=%v", stagedRevision, err)
	}

	remote.setFail(false)
	if err := builder.HandleEvent(ctx, oldEvent); err != nil {
		t.Fatalf("same event retry should succeed: %v", err)
	}
	if got := remote.revisions(); len(got) != 1 || got[0] != oldRevision {
		t.Fatalf("retry indexed revisions=%v", got)
	}

	newRevision := insertSearchRevision(t, d, pageID, "new body")
	if _, err := d.Pool.Exec(ctx,
		`UPDATE page SET current_revision_id = $2 WHERE id = $1`, pageID, newRevision); err != nil {
		t.Fatal(err)
	}
	if err := builder.HandleEvent(ctx, oldEvent); err != nil {
		t.Fatalf("late old event should be a successful no-op: %v", err)
	}
	if got := remote.revisions(); len(got) != 1 {
		t.Fatalf("late old event polluted remote index: %v", got)
	}
	if err := builder.HandleEvent(ctx, searchRevisionEvent(pageID, newRevision)); err != nil {
		t.Fatalf("new event failed: %v", err)
	}
	if got := remote.revisions(); len(got) != 2 || got[1] != newRevision {
		t.Fatalf("remote revisions=%v, want old then new", got)
	}
}

func insertSearchRevision(t *testing.T, d *testkit.DB, pageID uuid.UUID, body string) uuid.UUID {
	t.Helper()
	snapshotID := d.NewID(t)
	revisionID := d.NewID(t)
	raw, err := json.Marshal(map[string]any{
		"type": "document", "schema_version": 1,
		"children": []any{map[string]any{
			"id": d.NewID(t).String(), "type": "paragraph",
			"content": []any{map[string]any{"type": "text", "text": body}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO content_snapshot (id, schema_version, ast_json, content_hash, size_bytes)
		VALUES ($1, 1, $2::jsonb, $3, $4)`,
		snapshotID, raw, snapshotID.String(), len(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO revision (id, page_id, content_snapshot_id, actor_id)
		VALUES ($1, $2, $3, $4)`,
		revisionID, pageID, snapshotID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	return revisionID
}

func searchRevisionEvent(pageID, revisionID uuid.UUID) projection.Event {
	payload, _ := json.Marshal(map[string]string{"revision_id": revisionID.String()})
	return projection.Event{
		ID: uuid.New(), AggregateType: projection.AggregateTypePage,
		AggregateID: pageID, EventType: "page.revision_published", Payload: payload,
	}
}

type recordingSearchAdapter struct {
	mu      sync.Mutex
	fail    bool
	indexed []uuid.UUID
}

func (a *recordingSearchAdapter) setFail(fail bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fail = fail
}

func (a *recordingSearchAdapter) Index(_ context.Context, doc wikisearch.SearchDocument) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail {
		return errors.New("remote unavailable")
	}
	a.indexed = append(a.indexed, doc.SourceRevisionID)
	return nil
}

func (a *recordingSearchAdapter) Delete(context.Context, uuid.UUID) error { return nil }

func (a *recordingSearchAdapter) Search(context.Context, wikisearch.Query) ([]wikisearch.Hit, int, error) {
	return nil, 0, nil
}

func (a *recordingSearchAdapter) Rebuild(context.Context, []wikisearch.SearchDocument) error {
	return nil
}

func (a *recordingSearchAdapter) revisions() []uuid.UUID {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]uuid.UUID(nil), a.indexed...)
}

var _ wikisearch.SearchAdapter = (*recordingSearchAdapter)(nil)
