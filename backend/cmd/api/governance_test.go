package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/anby/wiki/backend/testkit"
)

func TestGovernanceAPI_CreateOperationAndReadOnlyPreview(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	writeAPI, readAPI, historyAPI, projectionAPI, searchAPI, knowledgeAPI, governanceAPI, err := assembleAPIs(tdb.Pool)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{Service: "wiki-api", Version: "test"}, writeAPI, readAPI, historyAPI,
		projectionAPI, searchAPI, knowledgeAPI, governanceAPI)
	headers := map[string]string{"X-Actor-ID": testkit.SystemActorID.String()}

	status, pageBody := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Proposal API"}, headers)
	if status != http.StatusCreated {
		t.Fatalf("create page status=%d body=%v", status, pageBody)
	}
	pageID := pageBody["id"].(string)
	baseAST := validASTBody(t, "base")
	status, revisionBody := doJSON(t, router, http.MethodPost,
		"/api/v1/pages/"+pageID+"/revisions", map[string]any{"ast": baseAST}, headers)
	if status != http.StatusCreated {
		t.Fatalf("publish base status=%d body=%v", status, revisionBody)
	}
	baseRevisionID := revisionBody["id"].(string)

	proposalHeaders := map[string]string{
		"X-Actor-ID":      testkit.SystemActorID.String(),
		"Idempotency-Key": "governance-api-preview",
	}
	status, proposalBody := doJSON(t, router, http.MethodPost, "/api/v1/proposals",
		map[string]any{
			"target_type":      "page",
			"target_id":        pageID,
			"base_revision_id": baseRevisionID,
			"risk_level":       "low",
		}, proposalHeaders)
	if status != http.StatusCreated {
		t.Fatalf("create proposal status=%d body=%v", status, proposalBody)
	}
	proposalID := proposalBody["id"].(string)

	operation := map[string]any{
		"schema_version": 1,
		"operation_type": "insert_block",
		"base":           map[string]any{"revision_id": baseRevisionID},
		"target":         map[string]any{"page_id": pageID},
		"expected_hash":  nil,
		"evidence":       []any{map[string]any{"note": "API contract evidence"}},
		"risk":           map[string]any{"level": "low", "reasons": []any{}},
		"payload": map[string]any{
			"parent_block_id": nil,
			"index":           1,
			"block": map[string]any{
				"type": "paragraph", "id": "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4aff", "content": []any{},
			},
		},
	}
	status, operationBody := doJSON(t, router, http.MethodPost,
		"/api/v1/proposals/"+proposalID+"/operations", operation, nil)
	if status != http.StatusCreated || operationBody["sequence"] != float64(1) {
		t.Fatalf("add operation status=%d body=%v", status, operationBody)
	}

	before := governanceWriteCounts(t, tdb)
	status, previewBody := doJSON(t, router, http.MethodGet,
		"/api/v1/proposals/"+proposalID+"/preview", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("preview status=%d body=%v", status, previewBody)
	}
	if previewBody["stale"] != false {
		t.Fatalf("preview stale=%v", previewBody["stale"])
	}
	impact := previewBody["impact"].(map[string]any)
	if impact["operation_count"] != float64(1) || impact["added_blocks"] != float64(1) {
		t.Fatalf("impact=%v", impact)
	}
	after := governanceWriteCounts(t, tdb)
	if before != after {
		t.Fatalf("preview 产生权威写入 before=%s after=%s", before, after)
	}

	status, getBody := doJSON(t, router, http.MethodGet,
		"/api/v1/proposals/"+proposalID, nil, nil)
	if status != http.StatusOK || len(getBody["operations"].([]any)) != 1 {
		t.Fatalf("get proposal status=%d body=%v", status, getBody)
	}
}

func governanceWriteCounts(t *testing.T, tdb *testkit.DB) string {
	t.Helper()
	var revisions, audits, outbox, batches int
	if err := tdb.Pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM revision),
		(SELECT count(*) FROM audit_event),
		(SELECT count(*) FROM outbox_event),
		(SELECT count(*) FROM change_batch)`).Scan(&revisions, &audits, &outbox, &batches); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d/%d/%d/%d", revisions, audits, outbox, batches)
}

func TestGovernanceAPI_BulkReviewAuditFlow(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	_, _, _, _, _, _, governanceAPI, err := assembleAPIs(tdb.Pool)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{Service: "wiki-api", Version: "test"}, nil, nil, nil, nil, nil, governanceAPI)
	headers := map[string]string{"X-Actor-ID": testkit.SystemActorID.String()}

	proposalIDs := make([]string, 2)
	for i := range proposalIDs {
		proposalID := tdb.NewID(t)
		proposalIDs[i] = proposalID.String()
		if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO proposal
			(id,target_type,status,risk_level,created_by,idempotency_key)
			VALUES ($1,'page','in_review','medium',$2,$3)`,
			proposalID, testkit.SystemActorID, fmt.Sprintf("api-bulk-%d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO review_task
			(id,proposal_id,status) VALUES ($1,$2,'pending')`, tdb.NewID(t), proposalID); err != nil {
			t.Fatal(err)
		}
	}
	status, created := doJSON(t, router, http.MethodPost, "/api/v1/bulk-review-batches",
		map[string]any{
			"proposal_ids": proposalIDs, "sample_percent": 50, "force_full": false, "wave_size": 1,
		}, headers)
	if status != http.StatusCreated {
		t.Fatalf("create status=%d body=%v", status, created)
	}
	batchID := created["id"].(string)
	items := created["items"].([]any)
	var sampledProposalID string
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["selected_for_review"].(bool) {
			sampledProposalID = item["proposal_id"].(string)
		}
	}
	status, decided := doJSON(t, router, http.MethodPost,
		"/api/v1/bulk-review-batches/"+batchID+"/proposals/"+sampledProposalID+"/decision",
		map[string]any{"approve": true, "reason": "API sampled"}, headers)
	if status != http.StatusOK || decided["status"] != "reviewing" {
		t.Fatalf("decide status=%d body=%v", status, decided)
	}
	status, finalized := doJSON(t, router, http.MethodPost,
		"/api/v1/bulk-review-batches/"+batchID+"/finalize", map[string]any{}, headers)
	if status != http.StatusOK || finalized["status"] != "ready" {
		t.Fatalf("finalize status=%d body=%v", status, finalized)
	}
	status, _ = doJSON(t, router, http.MethodGet,
		"/api/v1/bulk-review-batches/"+batchID, nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous get status=%d want=%d", status, http.StatusUnauthorized)
	}
	noRole := tdb.MakeActor(t, "human", "bulk-no-role")
	status, _ = doJSON(t, router, http.MethodGet,
		"/api/v1/bulk-review-batches/"+batchID, nil,
		map[string]string{"X-Actor-ID": noRole.String()})
	if status != http.StatusForbidden {
		t.Fatalf("no-role get status=%d want=%d", status, http.StatusForbidden)
	}
	noRoleHeaders := map[string]string{"X-Actor-ID": noRole.String()}
	for _, endpoint := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/bulk-review-batches/" + batchID + "/audit-events"},
		{http.MethodPost, "/api/v1/bulk-review-batches/" + batchID + "/pause"},
		{http.MethodPost, "/api/v1/bulk-review-batches/" + batchID + "/resume"},
		{http.MethodPost, "/api/v1/bulk-review-batches/" + batchID + "/apply-next-wave"},
	} {
		status, _ = doJSON(t, router, endpoint.method, endpoint.path, map[string]any{}, noRoleHeaders)
		if status != http.StatusForbidden {
			t.Fatalf("no-role %s %s status=%d want=%d",
				endpoint.method, endpoint.path, status, http.StatusForbidden)
		}
	}
	status, audit := doJSON(t, router, http.MethodGet,
		"/api/v1/bulk-review-batches/"+batchID+"/audit-events", nil, headers)
	if status != http.StatusOK || len(audit["items"].([]any)) < 3 {
		t.Fatalf("audit status=%d body=%v", status, audit)
	}
}
