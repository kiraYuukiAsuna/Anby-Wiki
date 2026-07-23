package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

// TestM4EvidenceChain 是 MT-M4-EVIDENCE-CHAIN 的里程碑验收：
// Page Current AST → ClaimUsage → Claim → ClaimSource → Citation →
// SourceChunk → SourceVersion → Source；Supersede 后旧 Claim、旧证据绑定和页面
// 对旧事实的稳定 ID 引用仍可匿名读取审计，新 Claim 通过 superseded_by 可达。
func TestM4EvidenceChain(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "m4-evidence-chain")
	headers := map[string]string{"X-Actor-ID": actorID.String()}

	pageRepo := page.NewRepository(tdb.Pool)
	txm := db.NewTxManager(tdb.Pool)
	ids := id.NewGenerator()
	evidenceRepo := evidence.NewRepository(tdb.Pool)
	evidenceService := evidence.NewService(evidenceRepo, pageRepo, nil, "test", txm, ids)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids).
		WithCitationChecker(evidenceRepo)

	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "character", CanonicalKey: "m4-evidence-anby",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "安比·德玛拉", IsPrimary: true}},
		ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	oldClaim, err := knowledgeService.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: entity.ID, PropertyKey: "release_date", Value: knowledge.DateValue("2024-07-04"),
		OriginType: knowledge.OriginHuman, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}

	source, err := evidenceService.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage, URL: "https://example.com/official/anby",
		Title: "安比官方角色档案", Publisher: "New Eridu Archive", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	pageNumber := int32(42)
	versionResult, err := evidenceService.AddSourceVersion(ctx, evidence.AddSourceVersionParams{
		SourceID: source.ID, VersionHash: "sha256:m4-evidence-chain-v1",
		FetchedAt: time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
		Chunks:    []evidence.ChunkInput{{Ordinal: 0, Locator: evidence.Locator{Page: &pageNumber}, TextContent: "官方记录：首次公开日期为 2024 年 7 月 4 日。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunkID := versionResult.Chunks[0].ID
	citation, err := evidenceService.CreateCitation(ctx, evidence.CreateCitationParams{
		SourceVersionID: versionResult.Version.ID, SourceChunkID: &chunkID,
		Quotation: "首次公开日期为 2024 年 7 月 4 日", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgeService.AddClaimSource(ctx, knowledge.AddClaimSourceParams{
		ClaimID: oldClaim.ID, CitationID: citation.ID, SupportType: knowledge.SupportTypeSupports,
	}); err != nil {
		t.Fatal(err)
	}

	code, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "M4 Evidence Chain"}, headers)
	if code != http.StatusCreated {
		t.Fatalf("创建页面=%d %v", code, created)
	}
	pageID := created["id"].(string)
	blockID := uuid.NewString()
	code, published := doJSON(t, router, http.MethodPost, "/api/v1/pages/"+pageID+"/revisions",
		map[string]any{"ast": map[string]any{
			"type": "document", "schema_version": 1,
			"children": []any{map[string]any{
				"id": blockID, "type": "paragraph",
				"content": []any{map[string]any{
					"type": "claim_reference", "claim_id": oldClaim.ID.String(), "display_text": "首次公开日期",
				}},
			}},
		}}, headers)
	if code != http.StatusCreated {
		t.Fatalf("发布页面=%d %v", code, published)
	}
	revisionID := published["id"].(string)

	registry := projection.NewRegistry()
	registry.Register(projection.NewClaimUsageBuilder(tdb.Pool))
	if rebuilt, err := projection.NewRebuilder(tdb.Pool, registry, nil).
		RebuildPage(ctx, uuid.MustParse(pageID)); err != nil || !rebuilt {
		t.Fatalf("重建 ClaimUsage=(%v,%v)", rebuilt, err)
	}

	// 由页面反向定位到 Claim，且投影携带 Current Revision / Block / Node。
	code, usageBody := doJSON(t, router, http.MethodGet,
		"/api/v1/claims/"+oldClaim.ID.String()+"/usages", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("claim usage=%d %v", code, usageBody)
	}
	usage := usageBody["items"].([]any)[0].(map[string]any)
	if usage["page_id"] != pageID || usage["revision_id"] != revisionID ||
		usage["block_id"] != blockID || usage["node_id"] != "0" {
		t.Fatalf("claim usage 定位链异常: %v", usage)
	}

	// 由 Claim 详情沿 claim_source 到 Citation。
	code, oldBody := doJSON(t, router, http.MethodGet,
		"/api/v1/claims/"+oldClaim.ID.String(), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("old claim=%d %v", code, oldBody)
	}
	sources := oldBody["sources"].([]any)
	if len(sources) != 1 || sources[0].(map[string]any)["citation_id"] != citation.ID.String() {
		t.Fatalf("claim→citation 绑定异常: %v", sources)
	}

	// 由 Citation 详情完整定位到不可变 Chunk、Version 与 Source。
	code, citationBody := doJSON(t, router, http.MethodGet,
		"/api/v1/citations/"+citation.ID.String(), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("citation=%d %v", code, citationBody)
	}
	if citationBody["source_version_id"] != versionResult.Version.ID.String() ||
		citationBody["source_chunk_id"] != chunkID.String() ||
		citationBody["source_chunk"].(map[string]any)["id"] != chunkID.String() ||
		citationBody["source"].(map[string]any)["id"] != source.ID.String() {
		t.Fatalf("citation 定位链异常: %v", citationBody)
	}

	// Supersede 只追加新 Claim 并终结旧 Claim；不篡改旧证据链或页面稳定引用。
	newClaim, err := knowledgeService.SupersedeClaim(ctx, knowledge.SupersedeClaimParams{
		ClaimID: oldClaim.ID, SubjectEntityID: entity.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2024-07-05"), ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	code, oldAfter := doJSON(t, router, http.MethodGet,
		"/api/v1/claims/"+oldClaim.ID.String(), nil, nil)
	if code != http.StatusOK || oldAfter["status"] != knowledge.ClaimStatusSuperseded ||
		oldAfter["superseded_by"] != newClaim.ID.String() || len(oldAfter["sources"].([]any)) != 1 {
		t.Fatalf("Supersede 后旧 Claim 审计异常: %d %v", code, oldAfter)
	}
	code, newBody := doJSON(t, router, http.MethodGet,
		"/api/v1/claims/"+newClaim.ID.String(), nil, nil)
	if code != http.StatusOK || newBody["status"] != knowledge.ClaimStatusPublished ||
		newBody["value"] != "2024-07-05" || len(newBody["sources"].([]any)) != 0 {
		t.Fatalf("Supersede 新 Claim 异常: %d %v", code, newBody)
	}
	code, usageAfter := doJSON(t, router, http.MethodGet,
		"/api/v1/claims/"+oldClaim.ID.String()+"/usages", nil, nil)
	if code != http.StatusOK || len(usageAfter["items"].([]any)) != 1 {
		t.Fatalf("Supersede 后旧页面引用不可审计: %d %v", code, usageAfter)
	}
	code, pageBody := doJSON(t, router, http.MethodGet, "/api/v1/pages/"+pageID, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("读取页面=%d %v", code, pageBody)
	}
	astJSON := pageBody["content"].(map[string]any)["ast_json"].(map[string]any)
	ref := astJSON["children"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if ref["claim_id"] != oldClaim.ID.String() {
		t.Fatalf("Supersede 不应改写已发布 AST 引用: %v", ref)
	}
}
