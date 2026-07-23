package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

// TestKnowledgeReadAPI_DetailChain 验证匿名详情接口能沿稳定 ID 展示
// Entity、Claim→Citation 绑定以及 Citation→Chunk→Version→Source→URL 定位链。
func TestKnowledgeReadAPI_DetailChain(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "knowledge-reader-fixture")

	pageRepo := page.NewRepository(tdb.Pool)
	txm := db.NewTxManager(tdb.Pool)
	ids := id.NewGenerator()
	evidenceRepo := evidence.NewRepository(tdb.Pool)
	evidenceService := evidence.NewService(evidenceRepo, pageRepo, nil, "test", txm, ids)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids).
		WithCitationChecker(evidenceRepo)

	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "character", CanonicalKey: "anby-demara",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "安比·德玛拉", Description: "测试实体", IsPrimary: true}},
		ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgeService.AddAlias(ctx, entity.ID, knowledge.AliasInput{Language: "en", Alias: "Anby Demara"}); err != nil {
		t.Fatal(err)
	}
	claim, err := knowledgeService.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: entity.ID, PropertyKey: "release_date", Value: knowledge.DateValue("2024-07-04"),
		OriginType: knowledge.OriginHuman, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}

	source, err := evidenceService.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage, URL: "https://example.com/anby?utm_source=test",
		Title: "安比角色档案", Author: "官方资料组", Metadata: []byte(`{"language":"zh-Hans"}`), ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	pageNumber := int32(7)
	section := "角色信息"
	version, err := evidenceService.AddSourceVersion(ctx, evidence.AddSourceVersionParams{
		SourceID: source.ID, VersionHash: "sha256:anby-profile-v1", FetchedAt: time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC),
		Chunks: []evidence.ChunkInput{{Ordinal: 0, Locator: evidence.Locator{Page: &pageNumber, Section: &section}, TextContent: "安比的首次公开日期为 2024 年 7 月 4 日。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunkID := version.Chunks[0].ID
	citation, err := evidenceService.CreateCitation(ctx, evidence.CreateCitationParams{
		SourceVersionID: version.Version.ID, SourceChunkID: &chunkID,
		Quotation: "2024 年 7 月 4 日", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgeService.AddClaimSource(ctx, knowledge.AddClaimSourceParams{
		ClaimID: claim.ID, CitationID: citation.ID, SupportType: knowledge.SupportTypeSupports,
	}); err != nil {
		t.Fatal(err)
	}

	status, entityBody := doJSON(t, router, http.MethodGet, "/api/v1/entities/"+entity.ID.String(), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("entity status=%d body=%v", status, entityBody)
	}
	if entityBody["canonical_key"] != "anby-demara" || entityBody["status"] != knowledge.StatusActive {
		t.Fatalf("entity detail=%v", entityBody)
	}
	if labels, ok := entityBody["labels"].([]any); !ok || len(labels) != 1 {
		t.Fatalf("entity labels=%v", entityBody["labels"])
	}
	if aliases, ok := entityBody["aliases"].([]any); !ok || len(aliases) != 1 {
		t.Fatalf("entity aliases=%v", entityBody["aliases"])
	}

	status, claimBody := doJSON(t, router, http.MethodGet, "/api/v1/claims/"+claim.ID.String(), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("claim status=%d body=%v", status, claimBody)
	}
	if claimBody["status"] != knowledge.ClaimStatusPublished || claimBody["value"] != "2024-07-04" {
		t.Fatalf("claim detail=%v", claimBody)
	}
	claimSources, ok := claimBody["sources"].([]any)
	if !ok || len(claimSources) != 1 || claimSources[0].(map[string]any)["citation_id"] != citation.ID.String() {
		t.Fatalf("claim sources=%v", claimBody["sources"])
	}

	status, citationBody := doJSON(t, router, http.MethodGet, "/api/v1/citations/"+citation.ID.String(), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("citation status=%d body=%v", status, citationBody)
	}
	if citationBody["quotation"] != "2024 年 7 月 4 日" {
		t.Fatalf("citation detail=%v", citationBody)
	}
	if got := citationBody["source"].(map[string]any)["title"]; got != "安比角色档案" {
		t.Fatalf("source title=%v", got)
	}
	if got := citationBody["source_chunk"].(map[string]any)["text_content"]; got != "安比的首次公开日期为 2024 年 7 月 4 日。" {
		t.Fatalf("chunk text=%v", got)
	}
	if got := citationBody["external_resource"].(map[string]any)["normalized_url"]; got != "https://example.com/anby" {
		t.Fatalf("normalized URL=%v", got)
	}
}

func TestKnowledgeReadAPI_InvalidAndMissingIDs(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	for _, path := range []string{
		"/api/v1/entities/not-a-uuid", "/api/v1/claims/not-a-uuid", "/api/v1/citations/not-a-uuid",
	} {
		status, body := doJSON(t, router, http.MethodGet, path, nil, nil)
		if status != http.StatusBadRequest || body["code"] != "bad_request" {
			t.Fatalf("%s status=%d body=%v", path, status, body)
		}
	}
	missing := tdb.NewID(t).String()
	for _, path := range []string{
		"/api/v1/entities/" + missing, "/api/v1/claims/" + missing, "/api/v1/citations/" + missing,
	} {
		status, body := doJSON(t, router, http.MethodGet, path, nil, nil)
		if status != http.StatusNotFound || body["code"] != "not_found" {
			t.Fatalf("%s status=%d body=%v", path, status, body)
		}
	}
}
