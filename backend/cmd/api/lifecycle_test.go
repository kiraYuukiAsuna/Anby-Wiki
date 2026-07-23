// MT-M1-LIFECYCLE：M1 里程碑级端到端验收（实施方案 §5 M1 里程碑验收、
// 追踪矩阵 §3）。与 M1-T04~T07 的单元/端点测试不同，本文件把页面全生命周期
// 串成一条链路，全程只经 HTTP（httptest.NewServer + 真实 PostgreSQL，
// testkit 仅准备 actor），验证各 Task 组合后的端到端行为：
//
// 创建「测试页面 Alpha」→ 发布 v1（heading+paragraph+list+table+未解析引用）
// → 发布 v2（增删 Block）→ 改名「测试页面 Beta」→ 旧标题 by-title 读取
// （via_alias=true 且 page id 不变，INV-06）→ 新标题直读 → diff(v1,v2) 有结构变化
// → 回滚到 v1（产生新 Revision v3，旧版不动，INV-11）→ 当前内容与 v1 AST 相同
// → 历史列表共 3 版且顺序正确。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/testkit"
)

// startLifecycleServer 装配完整路由并挂在真实 HTTP Server 上（非 ServeHTTP 直调），
// 返回测试库与 Server；actor 由调用方用 testkit 自行准备。
func startLifecycleServer(t *testing.T) (*testkit.DB, *httptest.Server) {
	t.Helper()
	tdb, router, _ := setupWriteAPI(t)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return tdb, srv
}

// srvJSON 经真实 HTTP 往返发起 JSON 请求，返回状态码与解码后的响应体。
func srvJSON(t *testing.T, srv *httptest.Server, method, path string, body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s 请求失败: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s 读取响应失败: %v", method, path, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("%s %s 响应非合法 JSON: %v body=%s", method, path, err, string(data))
	}
	return resp.StatusCode, parsed
}

// textInline 构造 text 行内节点。
func textInline(text string) map[string]any {
	return map[string]any{"type": "text", "text": text}
}

// paragraphBlock 构造 paragraph Block。
func paragraphBlock(id string, content ...any) map[string]any {
	return map[string]any{"id": id, "type": "paragraph", "content": content}
}

// lifecycleASTs 构造里程碑链路用的 v1/v2 AST（Block ID 跨版本稳定，供 Diff 断言）。
// v1：heading + paragraph（含未解析 page_reference）+ bullet_list（两个 list_item）+ table。
// v2：heading 改文案、删一个 list_item、删整个 table、新增 quote——增/删/改三类结构变化。
func lifecycleASTs(t *testing.T) (v1, v2 map[string]any, headingID, tableID, quoteID string) {
	t.Helper()
	h1, p1 := newBlockID(t), newBlockID(t)
	l1, i1, i2 := newBlockID(t), newBlockID(t), newBlockID(t)
	ip1, ip2 := newBlockID(t), newBlockID(t)
	t1, r1, c1, cp1 := newBlockID(t), newBlockID(t), newBlockID(t), newBlockID(t)
	q1, qp1 := newBlockID(t), newBlockID(t)

	intro := paragraphBlock(p1,
		textInline("首段，含未解析引用："),
		map[string]any{
			"type":              "page_reference",
			"resolution_status": "unresolved",
			"target_namespace":  "main",
			"normalized_title":  "不存在的页面",
		})
	listWith := func(items ...any) map[string]any {
		return map[string]any{"id": l1, "type": "bullet_list", "children": items}
	}
	item1 := map[string]any{"id": i1, "type": "list_item",
		"children": []any{paragraphBlock(ip1, textInline("条目一"))}}
	item2 := map[string]any{"id": i2, "type": "list_item",
		"children": []any{paragraphBlock(ip2, textInline("条目二"))}}
	table := map[string]any{"id": t1, "type": "table", "children": []any{
		map[string]any{"id": r1, "type": "table_row", "children": []any{
			map[string]any{"id": c1, "type": "table_cell",
				"children": []any{paragraphBlock(cp1, textInline("单元格"))}},
		}},
	}}
	heading := func(text string) map[string]any {
		return map[string]any{"id": h1, "type": "heading", "level": 1,
			"content": []any{textInline(text)}}
	}

	v1 = map[string]any{
		"type": "document", "schema_version": 1,
		"children": []any{heading("测试页面 Alpha"), intro, listWith(item1, item2), table},
	}
	v2 = map[string]any{
		"type": "document", "schema_version": 1,
		"children": []any{
			heading("测试页面 Beta 版"), intro, listWith(item1),
			map[string]any{"id": q1, "type": "quote",
				"children": []any{paragraphBlock(qp1, textInline("引用块"))}},
		},
	}
	return v1, v2, h1, t1, q1
}

// mustPost 发起写请求并断言预期状态码，返回响应体。
func mustPost(t *testing.T, srv *httptest.Server, path string, body any, headers map[string]string, wantStatus int) map[string]any {
	t.Helper()
	code, resp := srvJSON(t, srv, http.MethodPost, path, body, headers)
	if code != wantStatus {
		t.Fatalf("POST %s status = %d, want %d: %v", path, code, wantStatus, resp)
	}
	return resp
}

// mustGet 发起读请求并断言 200，返回响应体。
func mustGet(t *testing.T, srv *httptest.Server, path string) map[string]any {
	t.Helper()
	code, resp := srvJSON(t, srv, http.MethodGet, path, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200: %v", path, code, resp)
	}
	return resp
}

// canonicalJSON 把解码后的 JSON 值重新序列化（map key 排序，确定性输出），
// 用于比较两个 AST 是否内容相同。
func canonicalJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestM1Lifecycle(t *testing.T) {
	tdb, srv := startLifecycleServer(t)
	actor := tdb.MakeActor(t, "human", "milestone-tester")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}

	// ---- 1. 创建页面「测试页面 Alpha」----
	created := mustPost(t, srv, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "测试页面 Alpha"}, actorHeader, http.StatusCreated)
	pageID := created["id"].(string)
	if created["current_revision_id"] != nil {
		t.Fatalf("新页面不应有 current_revision_id: %v", created)
	}
	revURL := fmt.Sprintf("/api/v1/pages/%s/revisions", pageID)

	// ---- 2. 发布 v1：heading + paragraph + list + table + 未解析引用 ----
	v1AST, v2AST, headingID, tableID, quoteID := lifecycleASTs(t)
	rev1 := mustPost(t, srv, revURL,
		map[string]any{"ast": v1AST, "summary": "v1 初版"}, actorHeader, http.StatusCreated)
	rev1ID := rev1["id"].(string)

	// 渲染直连快照：v1 的 HTML 应包含四种 Block 与未解析引用标记。
	read := mustGet(t, srv, "/api/v1/pages/"+pageID)
	html := read["content"].(map[string]any)["html"].(string)
	for _, want := range []string{"<h1", "<ul>", "<table>", "data-unresolved-ref", "不存在的页面"} {
		if !strings.Contains(html, want) {
			t.Fatalf("v1 HTML 缺少 %q: %s", want, html)
		}
	}

	// ---- 3. 发布 v2：增删 Block ----
	rev2 := mustPost(t, srv, revURL,
		map[string]any{"expected_revision_id": rev1ID, "ast": v2AST, "summary": "v2 增删块"},
		actorHeader, http.StatusCreated)
	rev2ID := rev2["id"].(string)
	if rev2["parent_revision_id"] != rev1ID {
		t.Fatalf("v2 parent 应为 v1: %v", rev2)
	}

	// ---- 4. 改名「测试页面 Beta」：Page ID 不变（INV-06）----
	renamed := mustPost(t, srv, fmt.Sprintf("/api/v1/pages/%s/rename", pageID),
		map[string]any{"title": "测试页面 Beta"}, actorHeader, http.StatusOK)
	if renamed["id"] != pageID || renamed["display_title"] != "测试页面 Beta" {
		t.Fatalf("改名响应异常: %v", renamed)
	}

	// ---- 5. 旧标题 by-title 读取：via_alias=true 且 page id 不变 ----
	oldRead := mustGet(t, srv, byTitleURL("main", "测试页面 Alpha"))
	if oldRead["via_alias"] != true || oldRead["alias_title"] != "测试页面 Alpha" {
		t.Fatalf("旧标题应经别名解析: %v", oldRead)
	}
	if oldRead["page"].(map[string]any)["id"] != pageID {
		t.Fatalf("旧标题应解析到同一页面: %v", oldRead)
	}

	// ---- 6. 新标题直读：via_alias=false ----
	newRead := mustGet(t, srv, byTitleURL("main", "测试页面 Beta"))
	if newRead["via_alias"] != false {
		t.Fatalf("新标题直读不应 via_alias: %v", newRead)
	}
	if newRead["page"].(map[string]any)["id"] != pageID {
		t.Fatalf("新标题应解析到同一页面: %v", newRead)
	}

	// ---- 7. diff(v1, v2)：有结构变化（heading 改、table 删、quote 增）----
	diff := mustGet(t, srv,
		fmt.Sprintf("/api/v1/pages/%s/diff?from=%s&to=%s", pageID, rev1ID, rev2ID))
	changes, _ := diff["changes"].([]any)
	if len(changes) == 0 {
		t.Fatalf("diff(v1,v2) 应有结构变化: %v", diff)
	}
	typeByID := map[string]map[string]bool{}
	for _, raw := range changes {
		ch := raw.(map[string]any)
		id := ch["block_id"].(string)
		if typeByID[id] == nil {
			typeByID[id] = map[string]bool{}
		}
		typeByID[id][ch["type"].(string)] = true
	}
	if !typeByID[headingID]["changed"] {
		t.Fatalf("heading 应标记 changed: %v", changes)
	}
	if !typeByID[tableID]["removed"] {
		t.Fatalf("table 应标记 removed: %v", changes)
	}
	if !typeByID[quoteID]["added"] {
		t.Fatalf("quote 应标记 added: %v", changes)
	}

	// ---- 8. 回滚到 v1：产生新 Revision（v3），旧版不动（INV-11/INV-02）----
	rb := mustPost(t, srv, fmt.Sprintf("/api/v1/pages/%s/rollback", pageID),
		map[string]any{"target_revision_id": rev1ID, "summary": "回滚到 v1"}, actorHeader, http.StatusCreated)
	rev3ID := rb["id"].(string)
	if rev3ID == rev1ID || rev3ID == rev2ID {
		t.Fatalf("回滚必须新建 Revision: %v", rb)
	}
	if rb["parent_revision_id"] != rev2ID || rb["rolled_back_to"] != rev1ID {
		t.Fatalf("回滚 parent/rolled_back_to 异常: %v", rb)
	}
	// 内容寻址去重：v3 复用 v1 的快照行。
	if rb["content_snapshot_id"] != rev1["content_snapshot_id"] ||
		rb["content_hash"] != rev1["content_hash"] {
		t.Fatalf("回滚版应复用 v1 快照: %v vs %v", rb, rev1)
	}

	// ---- 9. 当前内容与 v1 AST 相同，current 指向 v3 ----
	current := mustGet(t, srv, "/api/v1/pages/"+pageID)
	curContent := current["content"].(map[string]any)
	if curContent["revision"].(map[string]any)["id"] != rev3ID {
		t.Fatalf("当前 Revision 应为回滚版 v3: %v", curContent)
	}
	if current["page"].(map[string]any)["current_revision_id"] != rev3ID {
		t.Fatalf("current_revision_id 应指向 v3: %v", current)
	}
	v1Detail := mustGet(t, srv, fmt.Sprintf("/api/v1/pages/%s/revisions/%s", pageID, rev1ID))
	if canonicalJSON(t, curContent["ast_json"]) != canonicalJSON(t, v1Detail["ast_json"]) {
		t.Fatalf("回滚后当前 AST 应与 v1 相同:\ncurrent=%v\nv1=%v",
			curContent["ast_json"], v1Detail["ast_json"])
	}
	// 旧版本仍可读（INV-02：回滚不修改/删除旧 Revision）。
	mustGet(t, srv, fmt.Sprintf("/api/v1/pages/%s/revisions/%s", pageID, rev2ID))

	// ---- 10. 历史列表：共 3 版，倒序 [v3, v2, v1] ----
	list := mustGet(t, srv, revURL)
	items, _ := list["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("历史应共 3 版: %v", list)
	}
	wantOrder := []string{rev3ID, rev2ID, rev1ID}
	for i, want := range wantOrder {
		if got := items[i].(map[string]any)["id"]; got != want {
			t.Fatalf("历史第 %d 项 = %v, want %s: %v", i, got, want, items)
		}
	}
	if list["next_cursor"] != nil {
		t.Fatalf("3 版应一页取完: %v", list)
	}
	// parent 链完整：v3→v2→v1→nil。
	if items[1].(map[string]any)["parent_revision_id"] != rev1ID ||
		items[2].(map[string]any)["parent_revision_id"] != nil {
		t.Fatalf("parent 链异常: %v", items)
	}
}
