// 历史 API 的 HTTP 层集成测试（M1-T07）：httptest + 真实数据库（testkit）。
// 覆盖：列表顺序与游标分页、单版详情与跨页 404、结构 Diff、回滚（201/400/403/404）。
package main

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/platform/httpx"
)

// astBodyWithBlocks 构造固定 Block ID 的发布请求 AST（Diff 断言需要跨版本稳定 ID）。
func astBodyWithBlocks(t *testing.T, blocks ...[2]string) map[string]any {
	t.Helper()
	children := []any{}
	for _, b := range blocks {
		children = append(children, map[string]any{
			"id":   b[0],
			"type": "paragraph",
			"content": []any{
				map[string]any{"type": "text", "text": b[1]},
			},
		})
	}
	return map[string]any{
		"type":           "document",
		"schema_version": 1,
		"children":       children,
	}
}

func newBlockID(t *testing.T) string {
	t.Helper()
	id, err := ast.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// publishBody 发布一个版本并断言 201，返回 Revision 响应体。
func publishBody(t *testing.T, router http.Handler, pageID string, expected any, astBody map[string]any, actorHeader map[string]string) map[string]any {
	t.Helper()
	req := map[string]any{"ast": astBody, "summary": "v"}
	if expected != nil {
		req["expected_revision_id"] = expected
	}
	code, rev := doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/revisions", pageID), req, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("发布失败: %d %v", code, rev)
	}
	return rev
}

// setupHistoryPage 创建页面并发布三个版本（v1/v3 用固定 Block，供 Diff 断言）。
// 返回 (pageID, [v1ID, v2ID, v3ID], blockA, blockB, blockC)。
func setupHistoryPage(t *testing.T, router http.Handler, actorHeader map[string]string, title string) (string, []string, string, string, string) {
	t.Helper()
	code, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": title}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("创建页面失败: %d %v", code, created)
	}
	pageID := created["id"].(string)

	blockA, blockB, blockC := newBlockID(t), newBlockID(t), newBlockID(t)
	v1 := publishBody(t, router, pageID, nil,
		astBodyWithBlocks(t, [2]string{blockA, "alpha"}, [2]string{blockB, "beta"}), actorHeader)
	v2 := publishBody(t, router, pageID, v1["id"],
		astBodyWithBlocks(t, [2]string{blockA, "alpha"}, [2]string{blockB, "beta 二"}), actorHeader)
	v3 := publishBody(t, router, pageID, v2["id"],
		astBodyWithBlocks(t, [2]string{blockA, "alpha 改"}, [2]string{blockC, "gamma"}), actorHeader)
	return pageID, []string{v1["id"].(string), v2["id"].(string), v3["id"].(string)}, blockA, blockB, blockC
}

func TestListRevisionsEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revIDs, _, _, _ := setupHistoryPage(t, router, actorHeader, "History List")
	listURL := fmt.Sprintf("/api/v1/pages/%s/revisions", pageID)

	// 匿名可读：page_size=2 翻页取全，倒序 [v3, v2] → [v1]。
	code, body := doJSON(t, router, http.MethodGet, listURL+"?page_size=2", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("第一页 items = %d, want 2: %v", len(items), body)
	}
	first, _ := items[0].(map[string]any)
	second, _ := items[1].(map[string]any)
	if first["id"] != revIDs[2] || second["id"] != revIDs[1] {
		t.Fatalf("第一页顺序异常: %v", items)
	}
	if len(first["content_hash"].(string)) != 64 || first["schema_version"] != float64(1) {
		t.Fatalf("列表项元信息异常: %v", first)
	}
	cursor, _ := body["next_cursor"].(string)
	if cursor == "" {
		t.Fatalf("第一页应有 next_cursor: %v", body)
	}

	code, body = doJSON(t, router, http.MethodGet, listURL+"?page_size=2&cursor="+cursor, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	items, _ = body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != revIDs[0] {
		t.Fatalf("第二页异常: %v", body)
	}
	if body["next_cursor"] != nil {
		t.Fatalf("末页 next_cursor 应为 null: %v", body)
	}

	// page_size 越界/非整数：400 bad_request。
	for _, q := range []string{"page_size=0", "page_size=101", "page_size=abc"} {
		code, body = doJSON(t, router, http.MethodGet, listURL+"?"+q, nil, nil)
		if code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400: %v", q, code, body)
		}
		assertErrorShape(t, body, httpx.CodeBadRequest)
	}
	// 游标非法：400 validation_failed。
	code, body = doJSON(t, router, http.MethodGet, listURL+"?cursor=bad-cursor", nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeValidationFailed)
	// 页面不存在：404。
	code, body = doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/revisions", uuid.New()), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
}

func TestGetRevisionEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revIDs, blockA, _, _ := setupHistoryPage(t, router, actorHeader, "History Detail")
	_, otherRevIDs, _, _, _ := setupHistoryPage(t, router, actorHeader, "History Other")

	// 匿名读单版详情：revision 元信息 + ast_json，不含 html。
	code, body := doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/revisions/%s", pageID, revIDs[0]), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	rev, _ := body["revision"].(map[string]any)
	if rev["id"] != revIDs[0] || rev["page_id"] != pageID || rev["parent_revision_id"] != nil {
		t.Fatalf("revision 异常: %v", rev)
	}
	astJSON, _ := body["ast_json"].(map[string]any)
	if astJSON["type"] != "document" || astJSON["schema_version"] != float64(1) {
		t.Fatalf("ast_json 异常: %v", astJSON)
	}
	children, _ := astJSON["children"].([]any)
	if len(children) == 0 || children[0].(map[string]any)["id"] != blockA {
		t.Fatalf("ast_json children 异常: %v", astJSON)
	}
	if _, ok := body["html"]; ok {
		t.Fatalf("详情端点不应含 html: %v", body)
	}

	// rid 非法：400。
	code, body = doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/revisions/not-a-uuid", pageID), nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeBadRequest)
	// 跨页：404（不泄露存在性）。
	code, body = doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/revisions/%s", pageID, otherRevIDs[0]), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
	// 页面不存在：404。
	code, body = doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/revisions/%s", uuid.New(), otherRevIDs[0]), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
}

func TestDiffRevisionsEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revIDs, blockA, blockB, blockC := setupHistoryPage(t, router, actorHeader, "History Diff")
	diffURL := fmt.Sprintf("/api/v1/pages/%s/diff", pageID)

	// 匿名读 diff(from=v1, to=v3)：A changed、B removed、C added。
	code, body := doJSON(t, router, http.MethodGet,
		fmt.Sprintf("%s?from=%s&to=%s", diffURL, revIDs[0], revIDs[2]), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	changes, _ := body["changes"].([]any)
	byID := map[string]string{}
	fieldsByID := map[string][]any{}
	for _, raw := range changes {
		ch := raw.(map[string]any)
		byID[ch["block_id"].(string)] = ch["type"].(string)
		if ch["type"] == "changed" {
			fieldsByID[ch["block_id"].(string)], _ = ch["fields"].([]any)
		}
	}
	if byID[blockA] != "changed" || byID[blockB] != "removed" || byID[blockC] != "added" {
		t.Fatalf("diff 异常: %v", changes)
	}
	if len(fieldsByID[blockA]) == 0 {
		t.Fatalf("changed 条目应带 fields: %v", changes)
	}

	// 同版 diff：changes 为空数组。
	code, body = doJSON(t, router, http.MethodGet,
		fmt.Sprintf("%s?from=%s&to=%s", diffURL, revIDs[0], revIDs[0]), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	if changes, _ := body["changes"].([]any); len(changes) != 0 {
		t.Fatalf("同版 diff 应为空: %v", body)
	}

	// 缺参数/非法 UUID：400 bad_request。
	for _, q := range []string{"", "from=" + revIDs[0], "from=bad&to=" + revIDs[0]} {
		code, body = doJSON(t, router, http.MethodGet, diffURL+"?"+q, nil, nil)
		if code != http.StatusBadRequest {
			t.Fatalf("q=%q status = %d, want 400: %v", q, code, body)
		}
		assertErrorShape(t, body, httpx.CodeBadRequest)
	}
	// Revision 不属于本页：404。
	_, otherRevIDs, _, _, _ := setupHistoryPage(t, router, actorHeader, "History Diff Other")
	code, body = doJSON(t, router, http.MethodGet,
		fmt.Sprintf("%s?from=%s&to=%s", diffURL, revIDs[0], otherRevIDs[0]), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
}

func TestRollbackEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revIDs, _, _, _ := setupHistoryPage(t, router, actorHeader, "History Rollback")
	rollbackURL := fmt.Sprintf("/api/v1/pages/%s/rollback", pageID)

	// 正路径：201，rolled_back_to=v1，parent=v3，默认 summary，snapshot 复用 v1。
	code, v1Detail := doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/revisions/%s", pageID, revIDs[0]), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("读取 v1 失败: %d", code)
	}
	v1Rev := v1Detail["revision"].(map[string]any)

	code, body := doJSON(t, router, http.MethodPost, rollbackURL,
		map[string]any{"target_revision_id": revIDs[0]}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %v", code, body)
	}
	if body["rolled_back_to"] != revIDs[0] {
		t.Fatalf("rolled_back_to 异常: %v", body)
	}
	if body["parent_revision_id"] != revIDs[2] {
		t.Fatalf("parent = %v, want v3 %s", body["parent_revision_id"], revIDs[2])
	}
	if body["content_hash"] != v1Rev["content_hash"] {
		t.Fatalf("回滚版 hash 应与 v1 相同: %v vs %v", body["content_hash"], v1Rev["content_hash"])
	}
	if body["content_snapshot_id"] != v1Rev["content_snapshot_id"] {
		t.Fatalf("应复用 v1 快照: %v vs %v", body["content_snapshot_id"], v1Rev["content_snapshot_id"])
	}
	if body["summary"] != "回滚到 "+revIDs[0] {
		t.Fatalf("默认 summary 异常: %v", body["summary"])
	}
	if body["id"] == revIDs[0] {
		t.Fatal("回滚必须新建 Revision")
	}

	// 列表现在 4 版，最新的是回滚版。
	code, list := doJSON(t, router, http.MethodGet,
		fmt.Sprintf("/api/v1/pages/%s/revisions", pageID), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	items, _ := list["items"].([]any)
	if len(items) != 4 || items[0].(map[string]any)["id"] != body["id"] {
		t.Fatalf("回滚后历史异常: %v", list)
	}

	// 覆盖 summary。
	code, custom := doJSON(t, router, http.MethodPost, rollbackURL,
		map[string]any{"target_revision_id": revIDs[0], "summary": "撤销破坏"}, actorHeader)
	if code != http.StatusCreated || custom["summary"] != "撤销破坏" {
		t.Fatalf("覆盖 summary 失败: %d %v", code, custom)
	}

	// 缺少认证身份：401 unauthorized。
	code, body = doJSON(t, router, http.MethodPost, rollbackURL,
		map[string]any{"target_revision_id": revIDs[0]}, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeUnauthorized)

	// ai actor：403 forbidden。
	ai := tdb.MakeActor(t, "ai", "gpt")
	code, body = doJSON(t, router, http.MethodPost, rollbackURL,
		map[string]any{"target_revision_id": revIDs[0]},
		map[string]string{"X-Actor-ID": ai.String()})
	if code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeForbidden)

	// 缺 target_revision_id：400。
	code, body = doJSON(t, router, http.MethodPost, rollbackURL, map[string]any{}, actorHeader)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeBadRequest)

	// 目标属于其他页面：404。
	_, otherRevIDs, _, _, _ := setupHistoryPage(t, router, actorHeader, "Rollback Other")
	code, body = doJSON(t, router, http.MethodPost, rollbackURL,
		map[string]any{"target_revision_id": otherRevIDs[0]}, actorHeader)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)

	// 页面不存在：404。
	code, body = doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/rollback", uuid.New()),
		map[string]any{"target_revision_id": revIDs[0]}, actorHeader)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
}
