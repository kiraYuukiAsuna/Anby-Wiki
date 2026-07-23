package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/anby/wiki/backend/internal/collection"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestCollectionAPIListDetailMembersAndErrors(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "collection-api-author")
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "collection-member", "Collection Member", actorID)
	snapshotID := tdb.NewID(t)
	revisionID := tdb.NewID(t)
	astJSON := `{"type":"document","schema_version":1,"children":[]}`
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO content_snapshot
		(id,content_hash,ast_json,schema_version,size_bytes)
		VALUES ($1,$2,$3::jsonb,1,$4)`,
		snapshotID, "sha256:"+snapshotID.String(), astJSON, len(astJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO revision
		(id,page_id,content_snapshot_id,actor_id) VALUES ($1,$2,$3,$4)`,
		revisionID, pageID, snapshotID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE page SET current_revision_id=$2 WHERE id=$1`,
		pageID, revisionID); err != nil {
		t.Fatal(err)
	}
	service := collection.NewService(
		collection.NewRepository(tdb.Pool), page.NewRepository(tdb.Pool),
		db.NewTxManager(tdb.Pool), id.NewGenerator(),
	)
	value, err := service.Create(ctx, collection.CreateParams{
		WikiID: testkit.DefaultWikiID, CollectionType: collection.TypeManual,
		Title: "Featured", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ReplaceManualMembers(ctx, value.ID, actorID, []collection.MemberInput{{
		MemberType: collection.MemberPage, PageID: &pageID,
		SortKey: "01", SourceRevisionID: revisionID,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, collection.CreateParams{
		WikiID: testkit.DefaultWikiID, CollectionType: collection.TypeManual,
		Title: "Later", ActorID: actorID,
	}); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{Service: "wiki-api", Version: "test"},
		nil, nil, nil, nil, nil,
		NewCollectionAPI(service, testkit.DefaultWikiID),
	)
	status, body := doJSON(t, router, http.MethodGet, "/api/v1/collections?page_size=1", nil, nil)
	if status != http.StatusOK || len(body["items"].([]any)) != 1 || body["next_cursor"] == nil {
		t.Fatalf("list status=%d body=%v", status, body)
	}
	cursor := body["next_cursor"].(string)
	status, body = doJSON(t, router, http.MethodGet,
		"/api/v1/collections?page_size=1&cursor="+cursor, nil, nil)
	if status != http.StatusOK || len(body["items"].([]any)) != 1 || body["next_cursor"] != nil {
		t.Fatalf("list second page status=%d body=%v", status, body)
	}
	status, body = doJSON(t, router, http.MethodGet,
		"/api/v1/collections/"+value.ID.String(), nil, nil)
	if status != http.StatusOK || body["title"] != "Featured" || body["query"] != nil {
		t.Fatalf("detail status=%d body=%v", status, body)
	}
	status, body = doJSON(t, router, http.MethodGet,
		"/api/v1/collections/"+value.ID.String()+"/members?page_size=1", nil, nil)
	items := body["items"].([]any)
	if status != http.StatusOK || len(items) != 1 ||
		items[0].(map[string]any)["source_revision_id"] != revisionID.String() {
		t.Fatalf("members status=%d body=%v", status, body)
	}
	status, _ = doJSON(t, router, http.MethodGet,
		"/api/v1/collections/"+value.ID.String()+"/members?cursor=bad", nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid cursor status=%d", status)
	}
	for _, path := range []string{
		"/api/v1/collections?cursor=e30",
		"/api/v1/collections/" + value.ID.String() + "/members?cursor=e30",
		"/api/v1/collections/not-a-uuid",
	} {
		status, _ = doJSON(t, router, http.MethodGet, path, nil, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid request %s status=%d", path, status)
		}
	}
	status, _ = doJSON(t, router, http.MethodGet,
		"/api/v1/collections/"+tdb.NewID(t).String(), nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("not found status=%d", status)
	}
}
