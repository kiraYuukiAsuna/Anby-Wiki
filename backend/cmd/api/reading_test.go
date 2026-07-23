// 阅读 API 的 HTTP 层集成测试（M1-T06）：httptest + 真实数据库（testkit）。
// 覆盖：创建→发布→by-title 读取（ast+html+revision 元信息）→改名后旧标题 via_alias→
// ID 读取→未发布 content=null→不存在 404→重定向跟随→重定向环 422→目标软删除 410。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/render"
)

// byTitleURL 构造 by-title 请求路径。
func byTitleURL(namespace, title string) string {
	return "/api/v1/pages/by-title?namespace=" + url.QueryEscape(namespace) +
		"&title=" + url.QueryEscape(title)
}

// createAndPublish 创建页面并发布一段正文 AST，返回 (pageID, revisionID)。
func createAndPublish(t *testing.T, router http.Handler, actorHeader map[string]string, title, text string) (string, string) {
	t.Helper()
	code, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": title}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("创建页面失败: %d %v", code, created)
	}
	pageID := created["id"].(string)
	code, rev := doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/revisions", pageID),
		map[string]any{"ast": validASTBody(t, text), "summary": "init"}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("发布失败: %d %v", code, rev)
	}
	return pageID, rev["id"].(string)
}

func TestReadPageByTitle(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revID := createAndPublish(t, router, actorHeader, "Billy Kid", "正文内容")

	// 匿名可读：不带 X-Actor-ID。
	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "  billy   KID "), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	pageMap, _ := body["page"].(map[string]any)
	if pageMap["id"] != pageID || pageMap["normalized_title"] != "billy kid" {
		t.Fatalf("page 元信息异常: %v", pageMap)
	}
	if body["via_alias"] != false {
		t.Fatalf("活标题命中不应 via_alias: %v", body)
	}
	if _, ok := body["alias_title"]; ok {
		t.Fatalf("非别名命中不应有 alias_title: %v", body)
	}
	if _, ok := body["redirect"]; ok {
		t.Fatalf("无重定向不应有 redirect: %v", body)
	}

	content, _ := body["content"].(map[string]any)
	if content == nil {
		t.Fatalf("已发布页面 content 不应为 null: %v", body)
	}
	rev, _ := content["revision"].(map[string]any)
	if rev["id"] != revID || rev["page_id"] != pageID {
		t.Fatalf("revision 元信息异常: %v", rev)
	}
	if len(rev["content_hash"].(string)) != 64 || rev["schema_version"] != float64(1) {
		t.Fatalf("revision hash/schema_version 异常: %v", rev)
	}
	astJSON, _ := content["ast_json"].(map[string]any)
	if astJSON["type"] != "document" || astJSON["schema_version"] != float64(1) {
		t.Fatalf("ast_json 异常: %v", astJSON)
	}
	html, _ := content["html"].(string)
	if !strings.Contains(html, "<p>正文内容</p>") {
		t.Fatalf("html 未包含渲染正文: %q", html)
	}
	if content["renderer_version"] != render.RendererVersion {
		t.Fatalf("renderer_version 异常: %v", content)
	}
}

func TestReadPageViaAlias(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, _ := createAndPublish(t, router, actorHeader, "Old Title", "x")

	// 改名后旧标题进入别名。
	code, _ := doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/rename", pageID),
		map[string]any{"title": "New Title"}, actorHeader)
	if code != http.StatusOK {
		t.Fatalf("改名失败: %d", code)
	}

	// 旧标题经别名解析到同一页面：200 + via_alias=true + alias_title 回显。
	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "OLD   title"), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	if body["via_alias"] != true || body["alias_title"] != "OLD   title" {
		t.Fatalf("alias 字段异常: %v", body)
	}
	pageMap, _ := body["page"].(map[string]any)
	if pageMap["id"] != pageID || pageMap["normalized_title"] != "new title" {
		t.Fatalf("别名应解析到改名后的同一页面: %v", pageMap)
	}
	if body["content"] == nil {
		t.Fatalf("已发布页面 content 不应为 null: %v", body)
	}
}

func TestReadPageByID(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revID := createAndPublish(t, router, actorHeader, "By ID Page", "按 ID 读")

	code, body := doJSON(t, router, http.MethodGet, "/api/v1/pages/"+pageID, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	pageMap, _ := body["page"].(map[string]any)
	if pageMap["id"] != pageID {
		t.Fatalf("page 异常: %v", pageMap)
	}
	content, _ := body["content"].(map[string]any)
	rev, _ := content["revision"].(map[string]any)
	if rev["id"] != revID {
		t.Fatalf("revision 异常: %v", rev)
	}
	html, _ := content["html"].(string)
	if !strings.Contains(html, "按 ID 读") {
		t.Fatalf("html 异常: %q", html)
	}

	// 非法 UUID：400 bad_request。
	code, body = doJSON(t, router, http.MethodGet, "/api/v1/pages/not-a-uuid", nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeBadRequest)

	// 不存在：404 not_found。
	code, body = doJSON(t, router, http.MethodGet, "/api/v1/pages/"+uuid.New().String(), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
}

func TestReadUnpublishedPage(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")

	// 只创建不发布：200 但 content 为 null。
	code, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Draft Page"},
		map[string]string{"X-Actor-ID": actor.String()})
	if code != http.StatusCreated {
		t.Fatalf("创建失败: %d %v", code, created)
	}

	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "draft page"), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	if body["content"] != nil {
		t.Fatalf("未发布页面 content 应为 null: %v", body)
	}
	pageMap, _ := body["page"].(map[string]any)
	if pageMap["current_revision_id"] != nil {
		t.Fatalf("未发布页面 current_revision_id 应为 null: %v", pageMap)
	}
}

func TestReadPageNotFound(t *testing.T) {
	_, router, _ := setupWriteAPI(t)

	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "no such page"), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)

	// 缺查询参数：400 bad_request。
	code, body = doJSON(t, router, http.MethodGet, "/api/v1/pages/by-title?namespace=main", nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeBadRequest)

	// 命名空间不存在：404。
	code, body = doJSON(t, router, http.MethodGet, byTitleURL("no-such", "X"), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
}

func TestReadPageRedirectFollow(t *testing.T) {
	tdb, router, svc := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	sourceID, _ := createAndPublish(t, router, actorHeader, "Redirect Source", "源")
	targetID, _ := createAndPublish(t, router, actorHeader, "Redirect Target", "目标正文")

	sourceUUID := uuid.MustParse(sourceID)
	targetUUID := uuid.MustParse(targetID)
	if err := svc.CreateRedirect(context.Background(), sourceUUID, targetUUID); err != nil {
		t.Fatalf("CreateRedirect 失败: %v", err)
	}

	// by-title 命中重定向源：跟随到目标页，带 redirect 信息。
	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "redirect source"), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	pageMap, _ := body["page"].(map[string]any)
	if pageMap["id"] != targetID || pageMap["normalized_title"] != "redirect target" {
		t.Fatalf("应跟随到目标页: %v", pageMap)
	}
	redirect, _ := body["redirect"].(map[string]any)
	if redirect["from_page_id"] != sourceID || redirect["from_title"] != "Redirect Source" {
		t.Fatalf("redirect 信息异常: %v", redirect)
	}
	content, _ := body["content"].(map[string]any)
	html, _ := content["html"].(string)
	if !strings.Contains(html, "目标正文") {
		t.Fatalf("应渲染目标页内容: %q", html)
	}

	// by-ID 同样跟随。
	code, body = doJSON(t, router, http.MethodGet, "/api/v1/pages/"+sourceID, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	pageMap, _ = body["page"].(map[string]any)
	if pageMap["id"] != targetID {
		t.Fatalf("by-ID 应跟随到目标页: %v", pageMap)
	}
}

func TestReadPageRedirectLoop(t *testing.T) {
	tdb, router, svc := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	aID, _ := createAndPublish(t, router, actorHeader, "Loop A", "a")
	bID, _ := createAndPublish(t, router, actorHeader, "Loop B", "b")

	ctx := context.Background()
	if err := svc.CreateRedirect(ctx, uuid.MustParse(aID), uuid.MustParse(bID)); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateRedirect(ctx, uuid.MustParse(bID), uuid.MustParse(aID)); err != nil {
		t.Fatal(err)
	}

	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "loop a"), nil, nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeValidationFailed)
}

func TestReadPageRedirectTargetDeleted(t *testing.T) {
	tdb, router, svc := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	sourceID, _ := createAndPublish(t, router, actorHeader, "Gone Source", "源")
	targetID, _ := createAndPublish(t, router, actorHeader, "Gone Target", "目标")

	if err := svc.CreateRedirect(context.Background(),
		uuid.MustParse(sourceID), uuid.MustParse(targetID)); err != nil {
		t.Fatal(err)
	}
	tdb.SoftDeletePage(t, uuid.MustParse(targetID))

	// 重定向目标已软删除：410 gone。
	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "gone source"), nil, nil)
	if code != http.StatusGone {
		t.Fatalf("status = %d, want 410: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeGone)

	// 直接按 ID 读软删除页：410 gone。
	code, body = doJSON(t, router, http.MethodGet, "/api/v1/pages/"+targetID, nil, nil)
	if code != http.StatusGone {
		t.Fatalf("status = %d, want 410: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeGone)
}

// TestReadResponseErrorSchema 校验阅读端点错误体是合法 JSON 且符合 Error schema 必填字段。
func TestReadResponseErrorSchema(t *testing.T) {
	_, router, _ := setupWriteAPI(t)
	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "missing"), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if _, err := json.Marshal(body); err != nil {
		t.Fatalf("错误体不可序列化: %v", err)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)
}
