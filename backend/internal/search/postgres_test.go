package search_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/search"
	"github.com/anby/wiki/backend/testkit"
)

func TestPostgresAdapter_SearchFieldsHighlightAndRebuild(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()
	actor := testkit.SystemActorID
	firstPage := d.MakePage(t, testkit.MainNamespaceID, "anby-demara", "Anby Demara", actor)
	secondPage := d.MakePage(t, testkit.MainNamespaceID, "new-eridu", "New Eridu", actor)
	makeRevision := func(pageID string) string {
		t.Helper()
		snapshotID := d.NewID(t)
		revisionID := d.NewID(t)
		if _, err := d.Pool.Exec(ctx, `
			INSERT INTO content_snapshot (id, schema_version, ast_json, content_hash, size_bytes)
			VALUES ($1, 1, '{"type":"document","schema_version":1,"children":[]}', $2, 56)`,
			snapshotID, snapshotID.String()); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Pool.Exec(ctx, `
			INSERT INTO revision (id, page_id, content_snapshot_id, actor_id)
			VALUES ($1, $2, $3, $4)`,
			revisionID, pageID, snapshotID, actor); err != nil {
			t.Fatal(err)
		}
		return revisionID.String()
	}
	firstRevision := makeRevision(firstPage.String())
	secondRevision := makeRevision(secondPage.String())

	adapter := search.NewPostgresAdapter(d.Pool)
	first := search.SearchDocument{
		PageID: firstPage, WikiID: testkit.DefaultWikiID, Namespace: "main",
		Language: "zh-Hans", SourceRevisionID: uuid.MustParse(firstRevision),
		DisplayTitle: "Anby Demara", NormalizedTitle: "anby-demara",
		Aliases: []string{"Soldier 0"}, Body: "A quiet swordswoman from the Cunning Hares. 安比是狡兔屋的成员。",
		EntityType: "character", EntityTerms: []string{"安比·德玛拉", "Anby"},
	}
	second := search.SearchDocument{
		PageID: secondPage, WikiID: testkit.DefaultWikiID, Namespace: "main",
		Language: "zh-Hans", SourceRevisionID: uuid.MustParse(secondRevision),
		DisplayTitle: "New Eridu", NormalizedTitle: "new-eridu",
		Body: "The last surviving metropolis.", EntityType: "place",
		EntityTerms: []string{"新艾利都"},
	}
	if err := adapter.Index(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Index(ctx, second); err != nil {
		t.Fatal(err)
	}

	assertHit := func(query search.Query, wantPage string, wantField search.Field) {
		t.Helper()
		query.WikiID = testkit.DefaultWikiID
		query.Namespace = "main"
		hits, total, err := adapter.Search(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(hits) != 1 || hits[0].PageID.String() != wantPage || hits[0].MatchedOn != wantField {
			t.Fatalf("Search(%+v) = total %d hits %+v", query, total, hits)
		}
		if !strings.Contains(hits[0].Highlight, "[[") {
			t.Fatalf("highlight missing markers: %q", hits[0].Highlight)
		}
	}
	assertHit(search.Query{Text: "Anby", Fields: []search.Field{search.FieldTitle}}, firstPage.String(), search.FieldTitle)
	assertHit(search.Query{Text: "Soldier", Fields: []search.Field{search.FieldAlias}}, firstPage.String(), search.FieldAlias)
	assertHit(search.Query{Text: "swordswoman", Fields: []search.Field{search.FieldBody}}, firstPage.String(), search.FieldBody)
	assertHit(search.Query{Text: "狡兔屋", Fields: []search.Field{search.FieldBody}}, firstPage.String(), search.FieldBody)
	assertHit(search.Query{Text: "Anby", Fields: []search.Field{search.FieldEntity}, EntityType: "character"}, firstPage.String(), search.FieldEntity)

	first.Body = "Updated body token"
	if err := adapter.Index(ctx, first); err != nil {
		t.Fatal(err)
	}
	hits, total, err := adapter.Search(ctx, search.Query{
		Text: "swordswoman", WikiID: testkit.DefaultWikiID, Fields: []search.Field{search.FieldBody},
	})
	if err != nil || total != 0 || len(hits) != 0 {
		t.Fatalf("idempotent update left old body: total=%d hits=%+v err=%v", total, hits, err)
	}

	if err := adapter.Rebuild(ctx, []search.SearchDocument{second}); err != nil {
		t.Fatal(err)
	}
	hits, total, err = adapter.Search(ctx, search.Query{Text: "Anby", WikiID: testkit.DefaultWikiID})
	if err != nil || total != 0 || len(hits) != 0 {
		t.Fatalf("rebuild retained removed document: total=%d hits=%+v err=%v", total, hits, err)
	}
}
