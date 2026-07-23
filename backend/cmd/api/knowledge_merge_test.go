package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestKnowledgeAPI_MergeEntityAuthorizationAndIdempotency(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	admin := tdb.MakeActor(t, "human", "entity-merge-admin")
	editor := tdb.MakeActor(t, "human", "entity-merge-editor")
	assignAPIRole(t, tdb, admin, "admin")
	assignAPIRole(t, tdb, editor, "editor")

	txm := db.NewTxManager(tdb.Pool)
	knowledgeService := knowledge.NewService(
		knowledge.NewRepository(tdb.Pool), page.NewRepository(tdb.Pool), txm, id.NewGenerator(),
	)
	source, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "api-merge-source",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "合并源", IsPrimary: true}},
		ActorID: admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "api-merge-target",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "合并目标", IsPrimary: true}},
		ActorID: admin,
	})
	if err != nil {
		t.Fatal(err)
	}

	writeAPI, readAPI, historyAPI, projectionAPI, searchAPI, knowledgeAPI, governanceAPI, err :=
		assembleAPIs(tdb.Pool)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)),
		Deps{Service: "wiki-api", Version: "test"}, writeAPI, readAPI, historyAPI,
		projectionAPI, searchAPI, knowledgeAPI, governanceAPI)
	url := "/api/v1/entities/" + source.ID.String() + "/merge"
	body := map[string]any{"target_entity_id": target.ID.String(), "reason": "管理员确认重复"}

	status, response := doJSON(t, router, http.MethodPost, url, body, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d body=%v", status, response)
	}
	status, response = doJSON(t, router, http.MethodPost, url, body,
		map[string]string{"X-Actor-ID": editor.String()})
	if status != http.StatusForbidden {
		t.Fatalf("editor status=%d body=%v", status, response)
	}
	headers := map[string]string{"X-Actor-ID": admin.String()}
	status, response = doJSON(t, router, http.MethodPost, url, body, headers)
	if status != http.StatusOK || response["source_entity_id"] != source.ID.String() ||
		response["target_entity_id"] != target.ID.String() || response["idempotent"] != false {
		t.Fatalf("merge status=%d body=%v", status, response)
	}
	mergeID := response["id"].(string)

	status, response = doJSON(t, router, http.MethodPost, url, body, headers)
	if status != http.StatusOK || response["id"] != mergeID || response["idempotent"] != true {
		t.Fatalf("idempotent merge status=%d body=%v", status, response)
	}
	var outboxCount int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event
		WHERE aggregate_type='entity_merge' AND aggregate_id=$1 AND event_type=$2`,
		mergeID, knowledge.OutboxEventEntityMerged).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("entity.merged outbox count=%d, want 1", outboxCount)
	}
}
