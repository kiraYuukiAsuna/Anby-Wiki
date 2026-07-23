// 投影查询 API 的 HTTP 层集成测试（M3-T03）：httptest + 真实数据库（testkit）。
// 覆盖：发布→RebuildPage 构建投影→backlinks（内容、游标分页、匿名可读）→
// outline（层级/slug/position_key）→404/410/400 错误语义。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

// projectionRegistry 注册 M3-T03 的两个投影 Builder（与 cmd/worker 一致）。
func projectionRegistry(tdb *testkit.DB) *projection.Registry {
	reg := projection.NewRegistry()
	reg.Register(projection.NewPageLinksBuilder(tdb.Pool))
	reg.Register(projection.NewOutlineBuilder(tdb.Pool))
	return reg
}

// rebuildPageProjections 同步重建一页投影（HTTP 测试不依赖 Worker 时序）。
func rebuildPageProjections(t *testing.T, tdb *testkit.DB, pageID uuid.UUID) {
	t.Helper()
	rebuilder := projection.NewRebuilder(tdb.Pool, projectionRegistry(tdb), nil)
	rebuilt, err := rebuilder.RebuildPage(context.Background(), pageID)
	if err != nil || !rebuilt {
		t.Fatalf("RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}
}

// publishASTBody 经 HTTP 发布一段 AST JSON（首发布）。
func publishASTBody(t *testing.T, router http.Handler, actorHeader map[string]string, pageID string, astJSON string) {
	t.Helper()
	var doc map[string]any
	mustJSON(t, astJSON, &doc)
	code, body := doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/revisions", pageID),
		map[string]any{"ast": doc, "summary": "init"}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("发布失败: %d %v", code, body)
	}
}

// mustJSON 解析 JSON 文本到 v。
func mustJSON(t *testing.T, raw string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		t.Fatalf("AST JSON 非法: %v", err)
	}
}

// projectionScenario 搭建：目标页 + 两个源页（各含指向目标页的已解析引用与 heading），
// 返回 (目标页 ID, 源页 A ID, 源页 A 引用 Block ID, 源页 A heading Block ID)。
func projectionScenario(t *testing.T, tdb *testkit.DB, router http.Handler, actorHeader map[string]string) (targetID, srcAID, srcABlockID, srcAHeadID string) {
	t.Helper()
	create := func(title string) string {
		code, body := doJSON(t, router, http.MethodPost, "/api/v1/pages",
			map[string]any{"namespace": "main", "title": title}, actorHeader)
		if code != http.StatusCreated {
			t.Fatalf("创建页面失败: %d %v", code, body)
		}
		return body["id"].(string)
	}
	targetID = create("Projection Target")
	srcAID = create("Projection Source A")
	srcBID := create("Projection Source B")

	srcABlock := uuid.NewString() // 引用所在 Block（仅测试内引用，非 v7 亦可过 schema：format uuid）
	srcAHead := uuid.NewString()
	srcBBlock := uuid.NewString()

	srcAAST := fmt.Sprintf(`{"type":"document","schema_version":1,"children":[
		{"id":%q,"type":"heading","level":1,"content":[{"type":"text","text":"Overview"}]},
		{"id":%q,"type":"paragraph","content":[
			{"type":"text","text":"see "},
			{"type":"page_reference","target_page_id":%q,"display_text":"目标页"},
			{"type":"page_reference","resolution_status":"unresolved","target_namespace":"main","normalized_title":"Ghost Page"}]},
		{"id":%q,"type":"heading","level":2,"content":[{"type":"text","text":"Overview"}]}
	]}`, srcAHead, srcABlock, targetID, uuid.NewString())
	srcBAST := fmt.Sprintf(`{"type":"document","schema_version":1,"children":[
		{"id":%q,"type":"paragraph","content":[
			{"type":"page_reference","target_page_id":%q,"display_text":"另一个来源"}]}
	]}`, srcBBlock, targetID)

	publishASTBody(t, router, actorHeader, srcAID, srcAAST)
	publishASTBody(t, router, actorHeader, srcBID, srcBAST)

	for _, id := range []string{srcAID, srcBID} {
		rebuildPageProjections(t, tdb, uuid.MustParse(id))
	}
	return targetID, srcAID, srcABlock, srcAHead
}

// TestBacklinksEndpoint 反链端点：内容正确（来源页/Block/展示文本）、匿名可读、
// 未解析引用不出现在反链中、游标分页翻页。
func TestBacklinksEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	targetID, srcAID, srcABlockID, _ := projectionScenario(t, tdb, router, actorHeader)

	// 匿名可读：不带 X-Actor-ID。page_size=1 强制分页。
	url := fmt.Sprintf("/api/v1/pages/%s/backlinks?page_size=1", targetID)
	code, page1 := doJSON(t, router, http.MethodGet, url, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("第一页 status = %d: %v", code, page1)
	}
	items1, _ := page1["items"].([]any)
	if len(items1) != 1 {
		t.Fatalf("第一页 items = %v, 期望 1 条", page1["items"])
	}
	next, _ := page1["next_cursor"].(string)
	if next == "" {
		t.Fatalf("两页数据应有 next_cursor: %v", page1)
	}

	code, page2 := doJSON(t, router, http.MethodGet, url+"&cursor="+next, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("第二页 status = %d: %v", code, page2)
	}
	items2, _ := page2["items"].([]any)
	if len(items2) != 1 || page2["next_cursor"] != nil {
		t.Fatalf("第二页应恰 1 条且无后续游标: %v", page2)
	}

	// 两页合并：恰含两个源页各一条已解析引用（未解析引用不出现）。
	seen := map[string]map[string]any{}
	for _, it := range append(items1, items2...) {
		m := it.(map[string]any)
		seen[m["source_page_id"].(string)] = m
	}
	if len(seen) != 2 {
		t.Fatalf("反链来源页数 = %d, 期望 2: %v", len(seen), seen)
	}
	a := seen[srcAID]
	if a["source_title"] != "Projection Source A" || a["source_block_id"] != srcABlockID || a["display_text"] != "目标页" {
		t.Fatalf("源页 A 反链内容异常: %v", a)
	}
}

// TestOutlineEndpoint 目录端点：层级/parent/slug/position_key 正确，匿名可读。
func TestOutlineEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	_, srcAID, _, srcAHeadID := projectionScenario(t, tdb, router, actorHeader)

	code, body := doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/outline", srcAID), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d: %v", code, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("目录条目数 = %d, 期望 2: %v", len(items), body)
	}
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if first["heading_block_id"] != srcAHeadID || first["level"] != float64(1) ||
		first["slug"] != "overview" || first["position_key"] != "1" || first["parent_heading_block_id"] != nil {
		t.Fatalf("首条目异常: %v", first)
	}
	if second["level"] != float64(2) || second["slug"] != "overview-2" ||
		second["position_key"] != "1.1" || second["parent_heading_block_id"] != srcAHeadID {
		t.Fatalf("次条目异常: %v", second)
	}
}

func TestResolveAnchorEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "anchor-reader")
	headers := map[string]string{"X-Actor-ID": actor.String()}
	_, sourcePageID, _, headingID := projectionScenario(t, tdb, router, headers)

	path := fmt.Sprintf("/api/v1/pages/%s/anchors/overview", sourcePageID)
	code, body := doJSON(t, router, http.MethodGet, path, nil, nil)
	if code != http.StatusOK || body["page_id"] != sourcePageID ||
		body["block_id"] != headingID || body["current_slug"] != "overview" ||
		body["via_alias"] != false || body["via_redirect"] != false {
		t.Fatalf("current anchor = %d %v", code, body)
	}

	var revisionID uuid.UUID
	if err := tdb.Pool.QueryRow(context.Background(), `SELECT current_revision_id
		FROM page WHERE id=$1`, sourcePageID).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO page_anchor_alias
		(page_id,alias_slug,heading_block_id,source_revision_id)
		VALUES ($1,'legacy-overview',$2,$3)`,
		sourcePageID, headingID, revisionID); err != nil {
		t.Fatal(err)
	}
	code, body = doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/anchors/legacy-overview", sourcePageID), nil, nil)
	if code != http.StatusOK || body["block_id"] != headingID ||
		body["via_alias"] != true || body["via_redirect"] != false {
		t.Fatalf("legacy anchor = %d %v", code, body)
	}

	code, body = doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/anchors/missing", sourcePageID), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("missing anchor = %d %v", code, body)
	}
}

// TestProjectionEndpointsErrors 错误语义：不存在 404、软删除 410、非法游标 400、非法 id 400。
func TestProjectionEndpointsErrors(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)

	random := uuid.NewString()
	for _, path := range []string{
		"/api/v1/pages/" + random + "/backlinks",
		"/api/v1/pages/" + random + "/outline",
	} {
		if code, body := doJSON(t, router, http.MethodGet, path, nil, nil); code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, 期望 404: %v", path, code, body)
		}
	}
	if code, body := doJSON(t, router, http.MethodGet, "/api/v1/pages/not-a-uuid/outline", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("非法 id = %d, 期望 400: %v", code, body)
	}

	// 软删除页 → 410 gone。
	deleted := tdb.MakePage(t, testkit.MainNamespaceID, "deleted page", "Deleted Page", testkit.SystemActorID)
	tdb.SoftDeletePage(t, deleted)
	if code, body := doJSON(t, router, http.MethodGet,
		"/api/v1/pages/"+deleted.String()+"/backlinks", nil, nil); code != http.StatusGone {
		t.Fatalf("软删除页 backlinks = %d, 期望 410: %v", code, body)
	}

	// 非法游标 → 400 validation_failed（页面须存在：用种子场景外的新页）。
	live := tdb.MakePage(t, testkit.MainNamespaceID, "live page", "Live Page", testkit.SystemActorID)
	if code, body := doJSON(t, router, http.MethodGet,
		"/api/v1/pages/"+live.String()+"/backlinks?cursor=!!!not-base64!!!", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("非法游标 = %d, 期望 400: %v", code, body)
	}
}

type apiKnowledgeTargets struct {
	entityID, claimID, citationID uuid.UUID
}

func makeAPIKnowledgeTargets(t *testing.T, d *testkit.DB) apiKnowledgeTargets {
	t.Helper()
	ctx := context.Background()
	entityID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO entity (id, wiki_id, entity_type_id, canonical_key, status, created_by)
		VALUES ($1, $2, $3, $4, 'active', $5)`, entityID, testkit.DefaultWikiID,
		testkit.EntityTypeCharacterID, "api-entity-"+entityID.String(), testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	claimID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO claim (id, subject_entity_id, property_id, value_type, value_json,
			status, verification_status, origin_type, created_by)
		VALUES ($1, $2, $3, 'date', '"2024-07-04"'::jsonb,
			'published', 'human_verified', 'human', $4)`,
		claimID, entityID, testkit.PropertyReleaseDateID, testkit.SystemActorID); err != nil {
		t.Fatal(err)
	}
	return apiKnowledgeTargets{entityID: entityID, claimID: claimID,
		citationID: d.MakeCitation(t, testkit.SystemActorID)}
}

func publishKnowledgeUsagePage(
	t *testing.T,
	d *testkit.DB,
	router http.Handler,
	headers map[string]string,
	title string,
	targets apiKnowledgeTargets,
	includeAll bool,
) (pageID, revisionID, blockID string) {
	t.Helper()
	code, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": title}, headers)
	if code != http.StatusCreated {
		t.Fatalf("创建知识引用页失败: %d %v", code, created)
	}
	pageID = created["id"].(string)
	blockID = uuid.NewString()
	content := []any{
		map[string]any{"type": "entity_reference", "entity_id": targets.entityID.String(), "display_text": "安比"},
	}
	if includeAll {
		content = append(content,
			map[string]any{"type": "claim_reference", "claim_id": targets.claimID.String(), "display_text": "生日"},
			map[string]any{"type": "citation_reference", "citation_id": targets.citationID.String(), "display_text": "设定集"},
		)
	}
	code, published := doJSON(t, router, http.MethodPost,
		"/api/v1/pages/"+pageID+"/revisions",
		map[string]any{"ast": map[string]any{
			"type": "document", "schema_version": 1,
			"children": []any{map[string]any{"id": blockID, "type": "paragraph", "content": content}},
		}}, headers)
	if code != http.StatusCreated {
		t.Fatalf("发布知识引用页失败: %d %v", code, published)
	}
	revisionID = published["id"].(string)
	reg := projection.NewRegistry()
	reg.Register(projection.NewEntityMentionsBuilder(d.Pool))
	reg.Register(projection.NewClaimUsageBuilder(d.Pool))
	reg.Register(projection.NewCitationUsageBuilder(d.Pool))
	if rebuilt, err := projection.NewRebuilder(d.Pool, reg, nil).
		RebuildPage(context.Background(), uuid.MustParse(pageID)); err != nil || !rebuilt {
		t.Fatalf("重建知识投影 = (%v, %v)", rebuilt, err)
	}
	return pageID, revisionID, blockID
}

// TestKnowledgeUsageEndpoints 反向查询只走投影，分页并携带来源 Revision/Block/Node。
func TestKnowledgeUsageEndpoints(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	headers := map[string]string{"X-Actor-ID": actor.String()}
	targets := makeAPIKnowledgeTargets(t, tdb)
	pageA, revisionA, blockA := publishKnowledgeUsagePage(
		t, tdb, router, headers, "Knowledge Usage API A", targets, true)
	_, _, _ = publishKnowledgeUsagePage(
		t, tdb, router, headers, "Knowledge Usage API B", targets, false)

	entityPath := "/api/v1/entities/" + targets.entityID.String() + "/mentions?page_size=1"
	code, first := doJSON(t, router, http.MethodGet, entityPath, nil, nil)
	if code != http.StatusOK || len(first["items"].([]any)) != 1 || first["next_cursor"] == nil {
		t.Fatalf("entity mentions 第一页异常: %d %v", code, first)
	}
	code, second := doJSON(t, router, http.MethodGet,
		entityPath+"&cursor="+first["next_cursor"].(string), nil, nil)
	if code != http.StatusOK || len(second["items"].([]any)) != 1 || second["next_cursor"] != nil {
		t.Fatalf("entity mentions 第二页异常: %d %v", code, second)
	}

	assertUsage := func(path string, wantMention any) map[string]any {
		t.Helper()
		code, body := doJSON(t, router, http.MethodGet, path, nil, nil)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d %v", path, code, body)
		}
		items := body["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("GET %s items = %v", path, items)
		}
		item := items[0].(map[string]any)
		if item["page_id"] != pageA || item["revision_id"] != revisionA ||
			item["block_id"] != blockA || item["mention_text"] != wantMention {
			t.Fatalf("GET %s 定位字段异常: %v", path, item)
		}
		return item
	}
	claim := assertUsage("/api/v1/claims/"+targets.claimID.String()+"/usages", nil)
	if claim["node_id"] != "1" || claim["claim_id"] != nil {
		t.Fatalf("claim usage 异常: %v", claim)
	}
	citation := assertUsage("/api/v1/citations/"+targets.citationID.String()+"/usages", nil)
	if citation["node_id"] != "2" || citation["claim_id"] != nil {
		t.Fatalf("citation usage 异常: %v", citation)
	}
}

func TestKnowledgeUsageEndpointsErrors(t *testing.T) {
	_, router, _ := setupWriteAPI(t)
	for _, path := range []string{
		"/api/v1/entities/not-a-uuid/mentions",
		"/api/v1/claims/not-a-uuid/usages",
		"/api/v1/citations/not-a-uuid/usages",
	} {
		if code, body := doJSON(t, router, http.MethodGet, path, nil, nil); code != http.StatusBadRequest {
			t.Fatalf("GET %s = %d，期望 400: %v", path, code, body)
		}
	}
	random := uuid.NewString()
	for _, path := range []string{
		"/api/v1/entities/" + random + "/mentions",
		"/api/v1/claims/" + random + "/usages",
		"/api/v1/citations/" + random + "/usages",
	} {
		if code, body := doJSON(t, router, http.MethodGet, path, nil, nil); code != http.StatusNotFound {
			t.Fatalf("GET %s = %d，期望 404: %v", path, code, body)
		}
	}
}
