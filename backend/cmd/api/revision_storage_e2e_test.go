package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/ast"
)

func TestAPIRevisionStorageAccountIsolationE2E(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_E2E_BASE_URL"), "/")
	password := os.Getenv("API_E2E_PASSWORD")
	fixtureURL := os.Getenv("API_E2E_FIXTURE_DATABASE_URL")
	if baseURL == "" || password == "" || fixtureURL == "" {
		t.Skip("set API_E2E_BASE_URL, API_E2E_PASSWORD and API_E2E_FIXTURE_DATABASE_URL to an isolated test stack")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, fixtureURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	first, firstActor := registerE2EUser(t, baseURL, "archive-a-"+runID, "archive-a-"+runID+"@example.invalid", password)
	second, secondActor := registerE2EUser(t, baseURL, "archive-b-"+runID, "archive-b-"+runID+"@example.invalid", password)
	empty, _ := registerE2EUser(t, baseURL, "archive-empty-"+runID, "archive-empty-"+runID+"@example.invalid", password)
	sharedAST := documentE2E(uuid.New(), "Shared history", "Identical content across Wikis")

	type fixture struct {
		client      *http.Client
		actorID     uuid.UUID
		pageID      uuid.UUID
		wikiID      uuid.UUID
		revisionIDs []uuid.UUID
		snapshotIDs []uuid.UUID
		sizes       []int64
	}
	oldest := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
	var sharedSnapshotID uuid.UUID
	fixtures := []*fixture{{client: first, actorID: firstActor}, {client: second, actorID: secondActor}}
	for index, item := range fixtures {
		session := requestE2EMap(t, item.client, http.MethodGet, baseURL+"/api/v1/auth/session", nil, http.StatusOK)
		item.wikiID = e2eUUID(t, session, "wiki_id")
		created := requestE2EMap(t, item.client, http.MethodPost, baseURL+"/api/v1/pages",
			map[string]any{"namespace": "main", "title": fmt.Sprintf("Archive %d %s", index, runID),
				"language": "en", "content_model": "block-v1"}, http.StatusCreated)
		item.pageID = e2eUUID(t, created, "id")
		// Fixture-only INSERTs create old, immutable artifacts without changing
		// published content or disabling the database immutability triggers.
		for docIndex, doc := range []map[string]any{
			sharedAST, documentE2E(uuid.New(), "Own history", fmt.Sprintf("Account %d historical content", index)),
			documentE2E(uuid.New(), "Current", fmt.Sprintf("Account %d current content", index)),
		} {
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := ast.CanonicalizeJSON(raw)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(canonical)
			snapshotID := uuid.New()
			if docIndex == 0 && index > 0 {
				snapshotID = sharedSnapshotID
			} else {
				if _, err := conn.Exec(ctx, `INSERT INTO content_snapshot
					(id,schema_version,ast_json,content_hash,size_bytes,created_at)
					VALUES ($1,1,$2,$3,$4,$5)`, snapshotID, canonical, hex.EncodeToString(sum[:]), len(canonical), oldest); err != nil {
					t.Fatal(err)
				}
			}
			if docIndex == 0 {
				sharedSnapshotID = snapshotID
			}
			revisionID := uuid.New()
			if _, err := conn.Exec(ctx, `INSERT INTO revision
				(id,page_id,content_snapshot_id,actor_id,summary,created_at)
				VALUES ($1,$2,$3,$4,'archive isolation fixture',$5)`,
				revisionID, item.pageID, snapshotID, item.actorID, oldest); err != nil {
				t.Fatal(err)
			}
			item.revisionIDs = append(item.revisionIDs, revisionID)
			item.snapshotIDs = append(item.snapshotIDs, snapshotID)
			item.sizes = append(item.sizes, int64(len(canonical)))
		}
		// The real publishing transaction reuses the old current snapshot. Two
		// revisions referencing one snapshot must still contribute only once.
		published := requestE2EMap(t, item.client, http.MethodPost,
			fmt.Sprintf("%s/api/v1/pages/%s/rollback", baseURL, item.pageID),
			map[string]any{"target_revision_id": item.revisionIDs[2], "summary": "current fixture"}, http.StatusCreated)
		item.revisionIDs[2] = e2eUUID(t, published, "id")
	}
	a, b := fixtures[0], fixtures[1]
	// An orphan snapshot has no account owner and must never enter account stats.
	if _, err := conn.Exec(ctx, `INSERT INTO content_snapshot
		(id,schema_version,ast_json,content_hash,size_bytes,created_at)
		SELECT $1,schema_version,ast_json,content_hash,size_bytes,created_at FROM content_snapshot WHERE id=$2`,
		uuid.New(), a.snapshotIDs[1]); err != nil {
		t.Fatal(err)
	}
	requestE2EMap(t, second, http.MethodPut,
		fmt.Sprintf("%s/api/v1/pages/%s/shares", baseURL, b.pageID),
		map[string]any{"identifier": "archive-a-" + runID, "role_key": "viewer"}, http.StatusOK)
	selected := http.Header{"X-Wiki-ID": []string{b.wikiID.String()}}
	session := requestE2EMapHeaders(t, first, http.MethodGet, baseURL+"/api/v1/auth/session", nil, selected, http.StatusOK)
	if e2eUUID(t, session, "wiki_id") != b.wikiID {
		t.Fatal("collaboration Wiki was not selected")
	}
	checkStats := func(client *http.Client, headers http.Header, hot, cold, hotBytes, coldBytes, candidates int64, oldestHot *time.Time) {
		t.Helper()
		got := requestE2EMapHeaders(t, client, http.MethodGet, baseURL+"/api/v1/revision-storage", nil, headers, http.StatusOK)
		for key, want := range map[string]int64{
			"hot_snapshots": hot, "cold_snapshots": cold, "hot_bytes": hotBytes,
			"cold_bytes": coldBytes, "eligible_snapshots": candidates,
		} {
			if got[key] != float64(want) {
				t.Fatalf("%s=%v want=%d; stats=%#v", key, got[key], want, got)
			}
		}
		if oldestHot == nil {
			if got["oldest_hot_created_at"] != nil {
				t.Fatalf("empty Wiki leaked oldest snapshot: %#v", got)
			}
		} else {
			raw, ok := got["oldest_hot_created_at"].(string)
			createdAt, err := time.Parse(time.RFC3339, raw)
			if !ok || err != nil || !createdAt.Equal(*oldestHot) {
				t.Fatalf("oldest_hot_created_at=%v want=%s", got["oldest_hot_created_at"], oldestHot.Format(time.RFC3339))
			}
		}
		if got["archive_available"] != true {
			t.Fatal("test requires real object storage")
		}
	}
	aBytes := a.sizes[0] + a.sizes[1] + a.sizes[2]
	bBytes := b.sizes[0] + b.sizes[1] + b.sizes[2]
	checkStats(first, nil, 3, 0, aBytes, 0, 1, &oldest)
	checkStats(first, selected, 3, 0, aBytes, 0, 1, &oldest)
	checkStats(second, nil, 3, 0, bBytes, 0, 1, &oldest)
	checkStats(empty, nil, 0, 0, 0, 0, 0, nil)
	checkArchive := func(client *http.Client, headers http.Header, count, size int64) {
		t.Helper()
		result := requestE2EMapHeaders(t, client, http.MethodPost, baseURL+"/api/v1/revision-storage/archive",
			map[string]any{"limit": 500}, headers, http.StatusOK)
		for key, want := range map[string]int64{"examined": count, "archived": count, "skipped": 0, "archived_bytes": size} {
			if result[key] != float64(want) {
				t.Fatalf("%s=%v want=%d; archive=%#v", key, result[key], want, result)
			}
		}
	}
	checkArchive(empty, nil, 0, 0)
	checkArchive(first, selected, 1, a.sizes[1])
	checkStats(first, selected, 2, 1, aBytes-a.sizes[1], a.sizes[1], 0, &oldest)
	checkStats(second, nil, 3, 0, bBytes, 0, 1, &oldest)
	checkArchive(first, nil, 0, 0)
	checkArchive(second, nil, 1, b.sizes[1])
	checkStats(second, nil, 2, 1, bBytes-b.sizes[1], b.sizes[1], 0, &oldest)
	var sharedTier string
	if err := conn.QueryRow(ctx, `SELECT storage_tier FROM content_snapshot WHERE id=$1`, a.snapshotIDs[0]).Scan(&sharedTier); err != nil {
		t.Fatal(err)
	}
	if sharedTier != "hot" {
		t.Fatal("manual archive changed a snapshot shared with another Wiki")
	}
	// Cold history, Diff and compensating rollback still use the actual S3 object.
	history := requestE2EMap(t, first, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions/%s", baseURL, a.pageID, a.revisionIDs[1]), nil, http.StatusOK)
	revision, ok := history["revision"].(map[string]any)
	if !ok || revision["storage_tier"] != "cold" || history["ast_json"] == nil {
		t.Fatalf("history did not read a cold snapshot: %#v", history)
	}
	requestE2EMap(t, first, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/diff?from=%s&to=%s", baseURL, a.pageID, a.revisionIDs[1], a.revisionIDs[2]), nil, http.StatusOK)
	rolledBack := requestE2EMap(t, first, http.MethodPost,
		fmt.Sprintf("%s/api/v1/pages/%s/rollback", baseURL, a.pageID),
		map[string]any{"target_revision_id": a.revisionIDs[1], "summary": "cold history recovery"}, http.StatusCreated)
	restored := requestE2EMap(t, first, http.MethodGet,
		fmt.Sprintf("%s/api/v1/pages/%s/revisions/%s", baseURL, a.pageID, e2eUUID(t, rolledBack, "id")), nil, http.StatusOK)
	if !reflect.DeepEqual(history["ast_json"], restored["ast_json"]) {
		t.Fatal("rollback did not restore the archived AST")
	}
	checkStats(empty, nil, 0, 0, 0, 0, 0, nil)
}
