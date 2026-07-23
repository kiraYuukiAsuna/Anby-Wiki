// 写 API 的 HTTP 层测试：httptest + 真实数据库（testkit），
// 覆盖三个端点的正反路径与契约 Error 响应体。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
	wikisearch "github.com/anby/wiki/backend/internal/search"
	"github.com/anby/wiki/backend/testkit"
)

// setupWriteAPI 建连 + Reset + 装配完整路由（含中间件）。
// 返回领域服务以便测试直接搭建重定向等无端点的场景。
func setupWriteAPI(t *testing.T) (*testkit.DB, http.Handler, *page.Service) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	svc := page.NewService(
		page.NewRepository(tdb.Pool),
		db.NewTxManager(tdb.Pool),
		id.NewGenerator(),
	)
	pageRepo := page.NewRepository(tdb.Pool)
	evidenceRepo := evidence.NewRepository(tdb.Pool)
	evidenceService := evidence.NewService(evidenceRepo, pageRepo, nil, "test", db.NewTxManager(tdb.Pool), id.NewGenerator())
	knowledgeService := knowledge.NewService(knowledge.NewRepository(tdb.Pool), pageRepo, db.NewTxManager(tdb.Pool), id.NewGenerator()).
		WithCitationChecker(evidenceRepo)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(logger, Deps{
		Service: "wiki-api", Version: "test", SessionCookie: "anby_session",
		TrustedOrigins: []string{"https://wiki.example.com"},
	},
		NewWriteAPI(svc, testkit.DefaultWikiID), NewReadAPI(svc, testkit.DefaultWikiID), NewHistoryAPI(svc),
		NewProjectionAPI(svc, projection.NewQueries(tdb.Pool)), NewSearchAPI(wikisearch.NewPageAdapter(svc), testkit.DefaultWikiID),
		NewKnowledgeReadAPI(knowledgeService, evidenceService))
	return tdb, router, svc
}

func TestCreatePageRejectsUntrustedBrowserWithoutSideEffect(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	code, body := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Blocked CSRF Page"},
		map[string]string{
			"Cookie": "anby_session=attacker-controlled",
			"Origin": "https://evil.example",
		})
	if code != http.StatusForbidden {
		t.Fatalf("status=%d body=%v", code, body)
	}
	var count int
	if err := tdb.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM page WHERE normalized_title = 'blocked csrf page'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("CSRF 拒绝后创建了 %d 个页面", count)
	}
}

// doJSON 发起 JSON 请求并返回状态码与解码后的响应体。
func doJSON(t *testing.T, router http.Handler, method, path string, body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var parsed map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("%s %s 响应非合法 JSON: %v body=%s", method, path, err, rec.Body.String())
	}
	return rec.Code, parsed
}

// validASTBody 构造发布请求用的合法 AST。
func validASTBody(t *testing.T, text string) map[string]any {
	t.Helper()
	blockID, err := ast.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"type":           "document",
		"schema_version": 1,
		"children": []any{
			map[string]any{
				"id":   blockID,
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": text},
				},
			},
		},
	}
}

// assertErrorShape 断言响应体符合契约 Error 模型（code/message/request_id 齐全）。
func assertErrorShape(t *testing.T, body map[string]any, wantCode string) {
	t.Helper()
	if body["code"] != wantCode {
		t.Fatalf("error code = %v, want %q (body=%v)", body["code"], wantCode, body)
	}
	if msg, _ := body["message"].(string); msg == "" {
		t.Fatalf("error message 为空: %v", body)
	}
	if rid, _ := body["request_id"].(string); rid == "" {
		t.Fatalf("error request_id 为空: %v", body)
	}
}

func TestCreatePageEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}

	// 正路径：201 Page
	code, body := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Anby Demara"}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %v", code, body)
	}
	if body["normalized_title"] != "anby demara" || body["display_title"] != "Anby Demara" {
		t.Fatalf("标题字段异常: %v", body)
	}
	if body["wiki_id"] != testkit.DefaultWikiID.String() ||
		body["namespace_id"] != testkit.MainNamespaceID.String() {
		t.Fatalf("站点/命名空间字段异常: %v", body)
	}
	if body["current_revision_id"] != nil {
		t.Fatalf("新页面 current_revision_id 应为 null: %v", body)
	}

	// 标题冲突：409 conflict
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "  ANBY   demara "}, actorHeader)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeConflict)

	// 命名空间不存在：404 not_found
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "no-such", "title": "X"}, actorHeader)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)

	// 非法标题：400 validation_failed
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "   "}, actorHeader)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeValidationFailed)

	// 缺少认证身份：401 unauthorized
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "No Actor"}, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeUnauthorized)

	// 不存在 / ai actor：403 forbidden
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Ghost"},
		map[string]string{"X-Actor-ID": uuid.New().String()})
	if code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeForbidden)

	ai := tdb.MakeActor(t, "ai", "gpt")
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "AI Page"},
		map[string]string{"X-Actor-ID": ai.String()})
	if code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeForbidden)

	// 缺字段：400 bad_request
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main"}, actorHeader)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeBadRequest)
}

func TestRenamePageEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}

	_, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Before Rename"}, actorHeader)
	pageID := created["id"].(string)
	doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Occupied"}, actorHeader)

	// 正路径：200 Page，标题已更新
	code, body := doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/rename", pageID),
		map[string]any{"title": "After Rename"}, actorHeader)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	if body["id"] != pageID || body["normalized_title"] != "after rename" {
		t.Fatalf("改名响应异常: %v", body)
	}

	// 标题冲突：409
	code, body = doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/rename", pageID),
		map[string]any{"title": "OCCUPIED"}, actorHeader)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeConflict)

	// 页面不存在：404
	code, body = doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/rename", uuid.New().String()),
		map[string]any{"title": "Whatever"}, actorHeader)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)

	// 路径 id 非法：400
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages/not-a-uuid/rename",
		map[string]any{"title": "X"}, actorHeader)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeBadRequest)
}

func TestPublishRevisionEndpoint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}

	_, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Revision Target"}, actorHeader)
	pageID := created["id"].(string)
	revURL := fmt.Sprintf("/api/v1/pages/%s/revisions", pageID)

	// 首发布：201 Revision，parent 为 null，content_hash/created_at 齐全
	code, rev1 := doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"ast": validASTBody(t, "第一版"), "summary": "init"}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %v", code, rev1)
	}
	if rev1["parent_revision_id"] != nil {
		t.Fatalf("首发布 parent 应为 null: %v", rev1)
	}
	hash1, _ := rev1["content_hash"].(string)
	if len(hash1) != 64 {
		t.Fatalf("content_hash 异常: %v", rev1)
	}
	if ts, _ := rev1["created_at"].(string); ts == "" {
		t.Fatalf("created_at 缺失: %v", rev1)
	}
	if rev1["schema_version"] != float64(1) || rev1["visibility"] != "public" {
		t.Fatalf("version/visibility 异常: %v", rev1)
	}
	rev1ID := rev1["id"].(string)

	// 二次发布：expected=rev1，parent 链正确
	code, rev2 := doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"expected_revision_id": rev1ID, "ast": validASTBody(t, "第二版"), "is_minor": true},
		actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %v", code, rev2)
	}
	if rev2["parent_revision_id"] != rev1ID || rev2["is_minor"] != true {
		t.Fatalf("二次发布 parent/is_minor 异常: %v", rev2)
	}

	// 陈旧基线：409 stale_revision
	code, body := doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"expected_revision_id": rev1ID, "ast": validASTBody(t, "第三版")}, actorHeader)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeStaleRevision)

	// 已发布页面缺 expected：409 stale_revision
	code, body = doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"ast": validASTBody(t, "第三版")}, actorHeader)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeStaleRevision)

	// 非法 AST：400 validation_failed
	badAST := validASTBody(t, "x")
	badAST["schema_version"] = 2
	code, body = doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"expected_revision_id": rev2["id"], "ast": badAST}, actorHeader)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeValidationFailed)

	// ast 非对象：400 bad_request
	code, body = doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"expected_revision_id": rev2["id"], "ast": []any{1, 2}}, actorHeader)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeBadRequest)

	// 页面不存在：404
	code, body = doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/revisions", uuid.New().String()),
		map[string]any{"ast": validASTBody(t, "x")}, actorHeader)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)

	// 缺少认证身份：401
	code, body = doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"ast": validASTBody(t, "x")}, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeUnauthorized)
}

// TestErrorRequestIDEcho 断言错误响应的 request_id 与透传的 X-Request-ID 一致。
func TestErrorRequestIDEcho(t *testing.T) {
	_, router, _ := setupWriteAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pages",
		bytes.NewReader([]byte(`{"namespace":"main","title":"X"}`)))
	req.Header.Set("X-Request-ID", "test-request-id-123")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["request_id"] != "test-request-id-123" {
		t.Fatalf("request_id = %v, want 透传的 X-Request-ID", body["request_id"])
	}
	if rec.Header().Get("X-Request-ID") != "test-request-id-123" {
		t.Fatalf("响应头 X-Request-ID = %q", rec.Header().Get("X-Request-ID"))
	}
}
