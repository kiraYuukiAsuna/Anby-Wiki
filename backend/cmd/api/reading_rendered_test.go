// 阅读 API 的 RenderedPage 投影优先级与兜底语义集成测试（M3-T05，真实数据库）。
// 覆盖：投影命中时 html 直接来自投影（SQL 篡改标记串断言）；投影缺失 / Revision
// 落后于 current / renderer_version 不匹配时降级为实时渲染。响应结构不变。
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/render"
	"github.com/anby/wiki/backend/testkit"
)

// projectionMarker 篡改投影 html 用的标记串：API 返回它即证明走了投影。
const projectionMarker = "<p>PROJECTION-MARKER-7f3a</p>"

// upsertRenderedRow 直接 SQL 写入/覆盖 rendered_page 行（绕过 Worker，
// 构造投影命中/落后的场景）。
func upsertRenderedRow(t *testing.T, tdb *testkit.DB, pageID, revID, rendererVersion, html string) {
	t.Helper()
	if _, err := tdb.Pool.Exec(context.Background(), `
		INSERT INTO rendered_page (page_id, revision_id, renderer_version, html_content, content_hash)
		VALUES ($1, $2, $3, $4, 'marker-hash')
		ON CONFLICT (page_id) DO UPDATE SET
			revision_id = EXCLUDED.revision_id,
			renderer_version = EXCLUDED.renderer_version,
			html_content = EXCLUDED.html_content`,
		pageID, revID, rendererVersion, html); err != nil {
		t.Fatalf("写入渲染投影失败: %v", err)
	}
}

// readHTML GET by-title 并取出 content.html。
func readHTML(t *testing.T, router http.Handler, title string) string {
	t.Helper()
	code, body := doJSON(t, router, http.MethodGet, byTitleURL("main", title), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	content, _ := body["content"].(map[string]any)
	if content == nil {
		t.Fatalf("已发布页面 content 不应为 null: %v", body)
	}
	if content["renderer_version"] != render.RendererVersion {
		t.Fatalf("renderer_version = %v, 期望 %q", content["renderer_version"], render.RendererVersion)
	}
	html, _ := content["html"].(string)
	return html
}

// TestReadPagePrefersRenderedProjection 投影命中：html 来自投影（标记串断言），
// by-title 与 by-id 两路径一致；删除投影行后同一请求降级为实时渲染。
func TestReadPagePrefersRenderedProjection(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revID := createAndPublish(t, router, actorHeader, "Proj Page", "实时渲染正文")

	upsertRenderedRow(t, tdb, pageID, revID, render.RendererVersion, projectionMarker)

	if html := readHTML(t, router, "proj page"); html != projectionMarker {
		t.Fatalf("投影命中时应返回投影 html\n got: %q\nwant: %q", html, projectionMarker)
	}
	// by-id 同样走投影。
	code, body := doJSON(t, router, http.MethodGet, "/api/v1/pages/"+pageID, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	content, _ := body["content"].(map[string]any)
	if content["html"] != projectionMarker {
		t.Fatalf("by-id 投影命中时应返回投影 html: %v", content["html"])
	}

	// 删除投影行 → 实时渲染兜底（现状行为）。
	if _, err := tdb.Pool.Exec(context.Background(),
		`DELETE FROM rendered_page WHERE page_id = $1`, pageID); err != nil {
		t.Fatalf("删除投影行失败: %v", err)
	}
	if html := readHTML(t, router, "proj page"); !strings.Contains(html, "实时渲染正文") {
		t.Fatalf("删除投影后应实时渲染兜底: %q", html)
	}
}

// TestReadPageFallsBackWhenProjectionStale 投影 Revision 落后于页面 current
// 时不得返回旧 html，降级为实时渲染当前 Revision。
func TestReadPageFallsBackWhenProjectionStale(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revID := createAndPublish(t, router, actorHeader, "Stale Page", "v1 正文")

	// 投影停留在 v1（标记串），随后页面发布 v2，current 前移。
	upsertRenderedRow(t, tdb, pageID, revID, render.RendererVersion, projectionMarker)
	code, rev := doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/pages/%s/revisions", pageID),
		map[string]any{
			"ast":                  validASTBody(t, "v2 新正文"),
			"expected_revision_id": revID,
			"summary":              "v2",
		}, actorHeader)
	if code != http.StatusCreated {
		t.Fatalf("发布 v2 失败: %d %v", code, rev)
	}

	html := readHTML(t, router, "stale page")
	if html == projectionMarker || !strings.Contains(html, "v2 新正文") {
		t.Fatalf("投影落后时应实时渲染当前 Revision: %q", html)
	}
}

// TestReadPageFallsBackWhenRendererVersionMismatch 投影的 renderer_version 与当前
// RendererVersion 不匹配（渲染器升版后的旧行）时未命中，降级实时渲染。
func TestReadPageFallsBackWhenRendererVersionMismatch(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revID := createAndPublish(t, router, actorHeader, "Version Page", "版本兜底正文")

	upsertRenderedRow(t, tdb, pageID, revID, "v0-legacy", projectionMarker)

	html := readHTML(t, router, "version page")
	if html == projectionMarker || !strings.Contains(html, "版本兜底正文") {
		t.Fatalf("renderer_version 不匹配时应实时渲染兜底: %q", html)
	}
}
