// MT-M1-INVALID-ACTOR 等负路径：M1 里程碑级拒绝语义验收（追踪矩阵 §3、§2 INV-05/INV-09）。
// 全部经 HTTP 层断言契约 Error 模型（code/message/request_id），
// 与 M1-T04~T07 的端点级负路径互补：这里按里程碑验收项成组收口。
// 覆盖：非法 AST（缺 block id / 错误 schema_version / list 直挂 paragraph）→ 400；
// 陈旧 expected_revision_id → 409 stale_revision（INV-09）；AI actor 发布 → 403（INV-05）；
// 不存在 actor 创建页面 → 403；重定向环 → 422；不存在页面/标题 → 404；
// 缺认证身份 → 401；软删除页面 by-id → 410。
package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/platform/httpx"
)

// TestM1InvalidASTRejected 非法 AST 发布一律 400 validation_failed。
func TestM1InvalidASTRejected(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}

	_, created := doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Invalid AST Target"}, actorHeader)
	pageID := created["id"].(string)
	revURL := fmt.Sprintf("/api/v1/pages/%s/revisions", pageID)

	cases := map[string]map[string]any{
		// 缺 block id：每个 Block 必须持有稳定 ID（ADR-0008）。
		"缺 block id": {
			"type": "document", "schema_version": 1,
			"children": []any{
				map[string]any{"type": "paragraph",
					"content": []any{textInline("无 id")}},
			},
		},
		// 错误 schema_version：v1 只接受 1。
		"错误 schema_version": {
			"type": "document", "schema_version": 2,
			"children": []any{
				paragraphBlock(newBlockID(t), textInline("x")),
			},
		},
		// list 直挂 paragraph：容器规则要求 list → list_item → 正文 Block。
		"list 直挂 paragraph": {
			"type": "document", "schema_version": 1,
			"children": []any{
				map[string]any{"id": newBlockID(t), "type": "bullet_list",
					"children": []any{paragraphBlock(newBlockID(t), textInline("越级"))}},
			},
		},
	}
	for name, badAST := range cases {
		code, body := doJSON(t, router, http.MethodPost, revURL,
			map[string]any{"ast": badAST}, actorHeader)
		if code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400: %v", name, code, body)
		}
		assertErrorShape(t, body, httpx.CodeValidationFailed)
	}

	// 全部拒绝后页面仍未发布（拒绝不产生任何 Revision）。
	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "invalid ast target"), nil, nil)
	if code != http.StatusOK || body["content"] != nil {
		t.Fatalf("非法发布不应产生内容: %d %v", code, body)
	}
}

// TestM1StaleAndActorRejected 陈旧基线 409（INV-09）、AI/不存在 actor 403（INV-05）、
// 缺认证身份 401。
func TestM1StaleAndActorRejected(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, rev1ID := createAndPublish(t, router, actorHeader, "Stale Target", "v1")
	revURL := fmt.Sprintf("/api/v1/pages/%s/revisions", pageID)

	// 陈旧 expected_revision_id：v1 已发布后再以 nil（首发布语义）基线发布 → 409。
	code, body := doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"ast": validASTBody(t, "并发覆盖")}, actorHeader)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeStaleRevision)

	// 推进到 v2 后再用 v1 基线发布 → 同样 409（陈旧 Base 不能静默覆盖 Current，INV-09）。
	v2 := publishBody(t, router, pageID, rev1ID, validASTBody(t, "v2"), actorHeader)
	code, body = doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"expected_revision_id": rev1ID, "ast": validASTBody(t, "v3 基于旧基线")}, actorHeader)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeStaleRevision)

	// AI actor 直接发布 → 403 forbidden（INV-05：AI 无直接发布权限）。
	ai := tdb.MakeActor(t, "ai", "gpt")
	code, body = doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"expected_revision_id": v2["id"], "ast": validASTBody(t, "ai 直发")},
		map[string]string{"X-Actor-ID": ai.String()})
	if code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeForbidden)

	// 不存在的 actor 创建页面 → 403 forbidden。
	code, body = doJSON(t, router, http.MethodPost, "/api/v1/pages",
		map[string]any{"namespace": "main", "title": "Ghost Page"},
		map[string]string{"X-Actor-ID": uuid.New().String()})
	if code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeForbidden)

	// 缺认证身份 → 401 unauthorized。
	code, body = doJSON(t, router, http.MethodPost, revURL,
		map[string]any{"expected_revision_id": v2["id"], "ast": validASTBody(t, "x")}, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeUnauthorized)

	// 负路径全部拒绝后历史仍只有 v1/v2 两版。
	code, list := doJSON(t, router, http.MethodGet, revURL, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d: %v", code, list)
	}
	if items, _ := list["items"].([]any); len(items) != 2 {
		t.Fatalf("负路径不应产生新 Revision: %v", list)
	}
}

// TestM1RedirectLoopRejected 重定向环（A→B→A，经领域服务搭建）读取 → 422。
func TestM1RedirectLoopRejected(t *testing.T) {
	tdb, router, svc := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	aID, _ := createAndPublish(t, router, actorHeader, "Loop Node A", "a")
	bID, _ := createAndPublish(t, router, actorHeader, "Loop Node B", "b")

	ctx := context.Background()
	if err := svc.CreateRedirect(ctx, uuid.MustParse(aID), uuid.MustParse(bID)); err != nil {
		t.Fatalf("CreateRedirect A→B 失败: %v", err)
	}
	if err := svc.CreateRedirect(ctx, uuid.MustParse(bID), uuid.MustParse(aID)); err != nil {
		t.Fatalf("CreateRedirect B→A 失败: %v", err)
	}

	// by-title 与 by-id 两个入口都必须检出环，返回 422 validation_failed。
	for _, path := range []string{byTitleURL("main", "loop node a"), "/api/v1/pages/" + bID} {
		code, body := doJSON(t, router, http.MethodGet, path, nil, nil)
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("GET %s status = %d, want 422: %v", path, code, body)
		}
		assertErrorShape(t, body, httpx.CodeValidationFailed)
	}
}

// TestM1NotFoundAndGone 不存在页面/标题 404；软删除页面 by-id 410。
func TestM1NotFoundAndGone(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}

	// 不存在标题 → 404。
	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", "不存在的标题"), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)

	// 不存在页面 ID → 404。
	code, body = doJSON(t, router, http.MethodGet, "/api/v1/pages/"+uuid.New().String(), nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeNotFound)

	// 软删除页面 by-id → 410 gone（读端点现状，里程碑记录以此为准）。
	pageID, _ := createAndPublish(t, router, actorHeader, "Doomed Page", "bye")
	tdb.SoftDeletePage(t, uuid.MustParse(pageID))
	code, body = doJSON(t, router, http.MethodGet, "/api/v1/pages/"+pageID, nil, nil)
	if code != http.StatusGone {
		t.Fatalf("status = %d, want 410: %v", code, body)
	}
	assertErrorShape(t, body, httpx.CodeGone)
}
