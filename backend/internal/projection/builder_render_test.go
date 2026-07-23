// RenderedPage 投影 Builder 集成测试（M3-T05，真实 PostgreSQL）。
// 覆盖：发布→事件分发后 rendered_page 行内容正确（html 与实时渲染一致、
// revision/renderer_version/content_hash 与权威一致）、重发新版投影覆盖更新、
// 删除投影行后 RebuildPage 恢复（INV-03）、同一事件重复投递幂等。
package projection_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/internal/render"
	"github.com/anby/wiki/backend/testkit"
)

// newRenderRegistry 只注册 RenderedPageBuilder 的注册表（隔离本投影的断言）。
func newRenderRegistry(d *testkit.DB) *projection.Registry {
	reg := projection.NewRegistry()
	reg.Register(projection.NewRenderedPageBuilder(d.Pool))
	return reg
}

// renderedRow rendered_page 的一行（created_at 不参与等价断言——覆盖写会刷新它）。
type renderedRow struct {
	revisionID      string
	rendererVersion string
	html            string
	contentHash     string
}

// readRenderedRow 读取页面的 rendered_page 行；无行返回 ok=false。
func readRenderedRow(t *testing.T, d *testkit.DB, pageID uuid.UUID) (renderedRow, bool) {
	t.Helper()
	var row renderedRow
	err := d.Pool.QueryRow(context.Background(), `
		SELECT revision_id, renderer_version, html_content, content_hash
		FROM rendered_page WHERE page_id = $1`, pageID).
		Scan(&row.revisionID, &row.rendererVersion, &row.html, &row.contentHash)
	if err != nil {
		return renderedRow{}, false
	}
	return row, true
}

// liveRender 从权威 AST 实时渲染（阅读路径兜底语义），并取快照的 content_hash。
func liveRender(t *testing.T, d *testkit.DB, revID uuid.UUID) (html, hash string) {
	t.Helper()
	doc, err := projection.RevisionAST(context.Background(), d.Pool, revID)
	if err != nil {
		t.Fatalf("读取权威 AST 失败: %v", err)
	}
	html, err = render.RenderHTML(doc)
	if err != nil {
		t.Fatalf("实时渲染失败: %v", err)
	}
	if err := d.Pool.QueryRow(context.Background(), `
		SELECT cs.content_hash FROM revision r
		JOIN content_snapshot cs ON cs.id = r.content_snapshot_id
		WHERE r.id = $1`, revID).Scan(&hash); err != nil {
		t.Fatalf("读取快照 content_hash 失败: %v", err)
	}
	return html, hash
}

// setupRenderedPage 建页、发布 v1 并同步分发事件，返回 (pageID, revID)。
func setupRenderedPage(t *testing.T, d *testkit.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "alice")
	pageID := d.MakePage(t, testkit.MainNamespaceID, "rendered page", "Rendered Page", testkit.SystemActorID)
	revID := publishAST(t, svc, pageID, actorID, document(
		headingBlock(d.NewID(t).String(), 1, textNode("Title")),
		paragraphBlock(d.NewID(t).String(), textNode("正文 v1")),
	))
	dispatchPublishEvent(t, d, newRenderRegistry(d), pageID, revID)
	return pageID, revID
}

// TestRenderedPageBuilderProjectionRow 发布并分发事件后：rendered_page 有行，
// html 与实时渲染一致，revision/renderer_version/content_hash 与权威一致。
func TestRenderedPageBuilderProjectionRow(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	pageID, revID := setupRenderedPage(t, d)

	row, ok := readRenderedRow(t, d, pageID)
	if !ok {
		t.Fatal("事件分发后 rendered_page 应有行")
	}
	wantHTML, wantHash := liveRender(t, d, revID)
	if row.revisionID != revID.String() {
		t.Fatalf("revision_id = %s, 期望 %s", row.revisionID, revID)
	}
	if row.rendererVersion != render.RendererVersion {
		t.Fatalf("renderer_version = %q, 期望 %q", row.rendererVersion, render.RendererVersion)
	}
	if row.html != wantHTML {
		t.Fatalf("投影 html 与实时渲染不一致\n got: %q\nwant: %q", row.html, wantHTML)
	}
	if row.contentHash != wantHash {
		t.Fatalf("content_hash = %q, 期望与快照一致 %q", row.contentHash, wantHash)
	}
}

// TestRenderedPageUpdatedOnRepublish 重发新版并分发后，投影覆盖为新 Revision 的 html。
func TestRenderedPageUpdatedOnRepublish(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	pageID, revID := setupRenderedPage(t, d)
	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "alice")

	rev2 := republishAST(t, svc, pageID, actorID, revID, document(
		headingBlock(d.NewID(t).String(), 1, textNode("Title")),
		paragraphBlock(d.NewID(t).String(), textNode("正文 v2 更新")),
	))
	dispatchPublishEvent(t, d, newRenderRegistry(d), pageID, rev2)

	row, ok := readRenderedRow(t, d, pageID)
	if !ok {
		t.Fatal("重发后 rendered_page 应有行")
	}
	wantHTML, wantHash := liveRender(t, d, rev2)
	if row.revisionID != rev2.String() || row.html != wantHTML || row.contentHash != wantHash {
		t.Fatalf("投影未更新到 v2\n got: %+v\nwant rev=%s html=%q hash=%q",
			row, rev2, wantHTML, wantHash)
	}
}

// TestRenderedPageRebuildRestores 删除投影行后 RebuildPage 恢复等价结果（INV-03）。
func TestRenderedPageRebuildRestores(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	pageID, _ := setupRenderedPage(t, d)

	before, ok := readRenderedRow(t, d, pageID)
	if !ok {
		t.Fatal("分发后 rendered_page 应有行")
	}
	if _, err := d.Pool.Exec(context.Background(),
		`DELETE FROM rendered_page WHERE page_id = $1`, pageID); err != nil {
		t.Fatalf("删除投影行失败: %v", err)
	}

	rebuilder := projection.NewRebuilder(d.Pool, newRenderRegistry(d), nil)
	rebuilt, err := rebuilder.RebuildPage(context.Background(), pageID)
	if err != nil || !rebuilt {
		t.Fatalf("RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}

	after, ok := readRenderedRow(t, d, pageID)
	if !ok {
		t.Fatal("重建后 rendered_page 应恢复行")
	}
	if after != before {
		t.Fatalf("重建后投影与删除前不一致\n got: %+v\nwant: %+v", after, before)
	}
}

// TestRenderedPageIdempotentDuplicateEvent 同一事件重复分发，投影行不变（幂等）。
func TestRenderedPageIdempotentDuplicateEvent(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	pageID, revID := setupRenderedPage(t, d)

	before, ok := readRenderedRow(t, d, pageID)
	if !ok {
		t.Fatal("分发后 rendered_page 应有行")
	}
	dispatchPublishEvent(t, d, newRenderRegistry(d), pageID, revID)

	after, ok := readRenderedRow(t, d, pageID)
	if !ok {
		t.Fatal("重放后 rendered_page 应有行")
	}
	if after != before {
		t.Fatalf("重复分发后投影不一致\n got: %+v\nwant: %+v", after, before)
	}
}

// TestRenderedPageHashMatchesASTContentHash content_hash 列与 ast.ContentHash
// 直接计算结果一致（与快照同源的另一佐证）。
func TestRenderedPageHashMatchesASTContentHash(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	pageID, revID := setupRenderedPage(t, d)

	doc, err := projection.RevisionAST(context.Background(), d.Pool, revID)
	if err != nil {
		t.Fatalf("读取权威 AST 失败: %v", err)
	}
	want, err := ast.ContentHash(doc)
	if err != nil {
		t.Fatalf("ast.ContentHash 失败: %v", err)
	}
	row, ok := readRenderedRow(t, d, pageID)
	if !ok {
		t.Fatal("分发后 rendered_page 应有行")
	}
	if row.contentHash != want {
		t.Fatalf("content_hash = %q, 期望 %q", row.contentHash, want)
	}
}
