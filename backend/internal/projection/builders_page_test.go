// 页面链接与目录投影 Builder 集成测试（M3-T03，真实 PostgreSQL）。
// 覆盖：已解析引用（目标页+锚点）/未解析引用（命名空间解析成功与失败）两态落库、
// heading 层级树（parent 链、position_key、slug）、事件路径幂等、
// 重发新版投影更新、删除投影行后 RebuildPage 等价恢复（INV-03 具体投影落点）。
package projection_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

// ---------------------------------------------------------------------------
// 场景搭建助手
// ---------------------------------------------------------------------------

// newPageService 装配真实 Page 领域服务（发布走完整事务，产出 outbox 事件）。
func newPageService(d *testkit.DB) *page.Service {
	return page.NewService(page.NewRepository(d.Pool), db.NewTxManager(d.Pool), id.NewGenerator())
}

// newPageBuilderRegistry 注册 M3-T03 的两个投影 Builder。
func newPageBuilderRegistry(d *testkit.DB) *projection.Registry {
	reg := projection.NewRegistry()
	reg.Register(projection.NewPageLinksBuilder(d.Pool))
	reg.Register(projection.NewOutlineBuilder(d.Pool))
	return reg
}

// publishAST 经领域服务发布一段 AST（首次发布），返回 Revision ID。
func publishAST(t *testing.T, svc *page.Service, pageID, actorID uuid.UUID, astJSON string) uuid.UUID {
	t.Helper()
	rev, err := svc.Publish(context.Background(), page.PublishParams{
		PageID:  pageID,
		ActorID: actorID,
		AST:     json.RawMessage(astJSON),
		Summary: "test",
	})
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}
	return rev.ID
}

// republishAST 在已有版本上发布新 AST（乐观锁基线 = 当前 Revision）。
func republishAST(t *testing.T, svc *page.Service, pageID, actorID, expected uuid.UUID, astJSON string) uuid.UUID {
	t.Helper()
	rev, err := svc.Publish(context.Background(), page.PublishParams{
		PageID:             pageID,
		ActorID:            actorID,
		ExpectedRevisionID: &expected,
		AST:                json.RawMessage(astJSON),
		Summary:            "test v2",
	})
	if err != nil {
		t.Fatalf("重发失败: %v", err)
	}
	return rev.ID
}

// dispatchPublishEvent 同步走一遍事件分发（NewRevisionPublishedHandler），
// 等价于 Worker 消费该页的 page.revision_published 事件。
func dispatchPublishEvent(t *testing.T, d *testkit.DB, reg *projection.Registry, pageID, revID uuid.UUID) {
	t.Helper()
	handler := projection.NewRevisionPublishedHandler(d.Pool, reg, nil)
	payload := fmt.Sprintf(`{"page_id":%q,"revision_id":%q,"schema_version":1}`, pageID, revID)
	event := projection.Event{
		ID:             d.NewID(t),
		IdempotencyKey: d.NewID(t).String(),
		AggregateType:  "page",
		AggregateID:    pageID,
		EventType:      "page.revision_published",
		Payload:        json.RawMessage(payload),
	}
	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatalf("事件分发失败: %v", err)
	}
}

// headingBlock/paragraphBlock 构造 AST Block JSON 片段。
func headingBlock(id string, level int, content string) string {
	return fmt.Sprintf(`{"id":%q,"type":"heading","level":%d,"content":[%s]}`, id, level, content)
}

func paragraphBlock(id string, content string) string {
	return fmt.Sprintf(`{"id":%q,"type":"paragraph","content":[%s]}`, id, content)
}

func textNode(text string) string {
	return fmt.Sprintf(`{"type":"text","text":%q}`, text)
}

func resolvedRef(targetPageID, anchorBlockID, display string) string {
	anchor := ""
	if anchorBlockID != "" {
		anchor = fmt.Sprintf(`,"target_heading_block_id":%q`, anchorBlockID)
	}
	return fmt.Sprintf(`{"type":"page_reference","target_page_id":%q%s,"display_text":%q}`,
		targetPageID, anchor, display)
}

func unresolvedRef(namespace, title string) string {
	return fmt.Sprintf(`{"type":"page_reference","resolution_status":"unresolved","target_namespace":%q,"normalized_title":%q}`,
		namespace, title)
}

func document(blocks ...string) string {
	return `{"type":"document","schema_version":1,"children":[` + strings.Join(blocks, ",") + `]}`
}

// ---------------------------------------------------------------------------
// 投影行读取助手（dump 为排序后的稳定字符串，便于整体比对）
// ---------------------------------------------------------------------------

func dumpLinkRows(t *testing.T, d *testkit.DB, pageID uuid.UUID) []string {
	t.Helper()
	rows, err := d.Pool.Query(context.Background(), `
		SELECT source_revision_id, source_block_id, source_node_id,
		       COALESCE(target_page_id::text, '-'), COALESCE(target_namespace_id::text, '-'),
		       COALESCE(target_title, '-'), COALESCE(target_anchor_block_id::text, '-'),
		       resolution_status, display_text
		FROM page_link_projection WHERE source_page_id = $1`, pageID)
	if err != nil {
		t.Fatalf("读取链接投影失败: %v", err)
	}
	defer rows.Close()
	return dumpRows(t, rows, 9)
}

func dumpOutlineRows(t *testing.T, d *testkit.DB, pageID uuid.UUID) []string {
	t.Helper()
	rows, err := d.Pool.Query(context.Background(), `
		SELECT revision_id, heading_block_id, COALESCE(parent_heading_block_id::text, '-'),
		       level, title, position_key
		FROM document_outline_projection WHERE page_id = $1`, pageID)
	if err != nil {
		t.Fatalf("读取大纲投影失败: %v", err)
	}
	defer rows.Close()
	return dumpRows(t, rows, 6)
}

func dumpAnchorRows(t *testing.T, d *testkit.DB, pageID uuid.UUID) []string {
	t.Helper()
	rows, err := d.Pool.Query(context.Background(), `
		SELECT revision_id, heading_block_id, COALESCE(parent_heading_block_id::text, '-'),
		       level, title, current_slug, position_key
		FROM page_anchor WHERE page_id = $1`, pageID)
	if err != nil {
		t.Fatalf("读取锚点失败: %v", err)
	}
	defer rows.Close()
	return dumpRows(t, rows, 7)
}

// dumpRows 把查询结果逐行格式化为 "col|col|..." 并排序（确定性比对）。
func dumpRows(t *testing.T, rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}, cols int,
) []string {
	t.Helper()
	var out []string
	for rows.Next() {
		vals := make([]string, cols)
		dest := make([]any, cols)
		for i := range dest {
			dest[i] = &vals[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatalf("扫描投影行失败: %v", err)
		}
		out = append(out, strings.Join(vals, "|"))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("迭代投影行失败: %v", err)
	}
	sort.Strings(out)
	return out
}

func assertRowsEqual(t *testing.T, what string, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("%s 行数 = %d, 期望 %d\n got: %v\nwant: %v", what, len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s 第 %d 行不一致\n got: %s\nwant: %s", what, i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// 用例
// ---------------------------------------------------------------------------

// pageProjectionFixture 搭建目标页（带锚点 heading）与源页（引用 + 嵌套 heading），
// 发布源页 v1 并同步分发事件，返回各 ID 供断言。
type pageProjectionFixture struct {
	sourcePageID uuid.UUID
	sourceRevID  uuid.UUID
	targetPageID uuid.UUID
	targetHeadID uuid.UUID // 目标页被引用的 heading Block
	h1, h2a, h2b uuid.UUID
	h3           uuid.UUID
	p1, p2       uuid.UUID
}

// sourceASTv1 源页 v1 AST：
//   - h1 "Intro"（L1）→ p1 含已解析引用（目标页+锚点）
//   - h2a "Details"（L2）
//   - p2 含两条未解析引用（main 命中命名空间 / nonexistent 解析失败）
//   - h2b "Intro"（L2，重复标题 → slug 后缀）→ h3 "Deep 深"（L3）
func sourceASTv1(f *pageProjectionFixture) string {
	return document(
		headingBlock(f.h1.String(), 1, textNode("Intro")),
		paragraphBlock(f.p1.String(),
			textNode("see ")+","+resolvedRef(f.targetPageID.String(), f.targetHeadID.String(), "Target Page")+","+textNode("!")),
		headingBlock(f.h2a.String(), 2, textNode("Details")),
		paragraphBlock(f.p2.String(),
			unresolvedRef("main", "Missing Page")+","+unresolvedRef("nonexistent", "Other Missing")),
		headingBlock(f.h2b.String(), 2, textNode("Intro")),
		headingBlock(f.h3.String(), 3, textNode("Deep 深")),
	)
}

func setupPageProjection(t *testing.T, d *testkit.DB) *pageProjectionFixture {
	t.Helper()
	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "alice")

	f := &pageProjectionFixture{
		targetHeadID: d.NewID(t),
		h1:           d.NewID(t),
		h2a:          d.NewID(t),
		h2b:          d.NewID(t),
		h3:           d.NewID(t),
		p1:           d.NewID(t),
		p2:           d.NewID(t),
	}

	// 目标页：一个 heading 作为被引用锚点。
	f.targetPageID = d.MakePage(t, testkit.MainNamespaceID, "target page", "Target Page", testkit.SystemActorID)
	publishAST(t, svc, f.targetPageID, actorID, document(
		headingBlock(f.targetHeadID.String(), 1, textNode("Section One")),
	))

	f.sourcePageID = d.MakePage(t, testkit.MainNamespaceID, "source page", "Source Page", testkit.SystemActorID)
	f.sourceRevID = publishAST(t, svc, f.sourcePageID, actorID, sourceASTv1(f))

	dispatchPublishEvent(t, d, newPageBuilderRegistry(d), f.sourcePageID, f.sourceRevID)
	return f
}

// TestPageLinksBuilderBothResolutionStates 已解析/未解析引用两态落库：
// 已解析带 target_page_id+锚点；未解析命名空间命中落 id、未命中落 NULL；
// source_node_id 为行内下标十进制（稳定）。
func TestPageLinksBuilderBothResolutionStates(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	f := setupPageProjection(t, d)

	want := []string{
		fmt.Sprintf("%s|%s|%s|%s|-|-|%s|resolved|Target Page",
			f.sourceRevID, f.p1, "1", f.targetPageID, f.targetHeadID),
		fmt.Sprintf("%s|%s|%s|-|%s|%s|-|unresolved|Missing Page",
			f.sourceRevID, f.p2, "0", testkit.MainNamespaceID, "Missing Page"),
		fmt.Sprintf("%s|%s|%s|-|-|%s|-|unresolved|Other Missing",
			f.sourceRevID, f.p2, "1", "Other Missing"),
	}
	assertRowsEqual(t, "page_link_projection", dumpLinkRows(t, d, f.sourcePageID), want)
}

// TestOutlineBuilderTreeAndSlugs heading 层级树：parent 链、position_key、
// slug（重复标题 -2、CJK 保留）；page_anchor 与大纲同事务写入。
func TestOutlineBuilderTreeAndSlugs(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	f := setupPageProjection(t, d)

	wantOutline := []string{
		fmt.Sprintf("%s|%s|-|%d|%s|%s", f.sourceRevID, f.h1, 1, "Intro", "1"),
		fmt.Sprintf("%s|%s|%s|%d|%s|%s", f.sourceRevID, f.h2a, f.h1, 2, "Details", "1.1"),
		fmt.Sprintf("%s|%s|%s|%d|%s|%s", f.sourceRevID, f.h2b, f.h1, 2, "Intro", "1.2"),
		fmt.Sprintf("%s|%s|%s|%d|%s|%s", f.sourceRevID, f.h3, f.h2b, 3, "Deep 深", "1.2.1"),
	}
	assertRowsEqual(t, "document_outline_projection", dumpOutlineRows(t, d, f.sourcePageID), wantOutline)

	wantAnchors := []string{
		fmt.Sprintf("%s|%s|-|%d|%s|%s|%s", f.sourceRevID, f.h1, 1, "Intro", "intro", "1"),
		fmt.Sprintf("%s|%s|%s|%d|%s|%s|%s", f.sourceRevID, f.h2a, f.h1, 2, "Details", "details", "1.1"),
		fmt.Sprintf("%s|%s|%s|%d|%s|%s|%s", f.sourceRevID, f.h2b, f.h1, 2, "Intro", "intro-2", "1.2"),
		fmt.Sprintf("%s|%s|%s|%d|%s|%s|%s", f.sourceRevID, f.h3, f.h2b, 3, "Deep 深", "deep-深", "1.2.1"),
	}
	assertRowsEqual(t, "page_anchor", dumpAnchorRows(t, d, f.sourcePageID), wantAnchors)
}

func TestOutlineBuilderPreservesHistoricalSlugAlias(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	f := setupPageProjection(t, d)
	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "anchor-renamer")
	revisionID := republishAST(
		t, svc, f.sourcePageID, actorID, f.sourceRevID,
		document(headingBlock(f.h1.String(), 1, textNode("Renamed Section"))),
	)
	dispatchPublishEvent(t, d, newPageBuilderRegistry(d), f.sourcePageID, revisionID)

	var blockID uuid.UUID
	if err := d.Pool.QueryRow(context.Background(), `SELECT heading_block_id
		FROM page_anchor_alias WHERE page_id=$1 AND alias_slug='intro'`,
		f.sourcePageID).Scan(&blockID); err != nil {
		t.Fatal(err)
	}
	if blockID != f.h1 {
		t.Fatalf("alias block=%s want=%s", blockID, f.h1)
	}
	var slug string
	if err := d.Pool.QueryRow(context.Background(), `SELECT current_slug FROM page_anchor
		WHERE page_id=$1 AND heading_block_id=$2`, f.sourcePageID, f.h1).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug != "renamed-section" {
		t.Fatalf("slug=%s", slug)
	}
}

// TestBuildersIdempotentOnDuplicateEvent 同一事件处理两次结果一致（Builder 幂等）。
func TestBuildersIdempotentOnDuplicateEvent(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	f := setupPageProjection(t, d)

	linksBefore := dumpLinkRows(t, d, f.sourcePageID)
	outlineBefore := dumpOutlineRows(t, d, f.sourcePageID)
	anchorsBefore := dumpAnchorRows(t, d, f.sourcePageID)
	if len(linksBefore) == 0 || len(outlineBefore) == 0 {
		t.Fatal("首次分发后投影不应为空")
	}

	// 同一事件再投递一次（至少一次投递语义下的重放）。
	dispatchPublishEvent(t, d, newPageBuilderRegistry(d), f.sourcePageID, f.sourceRevID)
	assertRowsEqual(t, "重放后 page_link_projection", dumpLinkRows(t, d, f.sourcePageID), linksBefore)
	assertRowsEqual(t, "重放后 document_outline_projection", dumpOutlineRows(t, d, f.sourcePageID), outlineBefore)
	assertRowsEqual(t, "重放后 page_anchor", dumpAnchorRows(t, d, f.sourcePageID), anchorsBefore)
}

// TestProjectionUpdatedOnRepublish 重发新版（引用变化）→ 投影全量更新：
// source_revision_id 指向新版本，被移除的引用行消失。
func TestProjectionUpdatedOnRepublish(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	f := setupPageProjection(t, d)
	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "alice")

	// v2：p2 只保留 main 未解析引用；其余结构不变。
	astV2 := document(
		headingBlock(f.h1.String(), 1, textNode("Intro")),
		paragraphBlock(f.p1.String(),
			textNode("see ")+","+resolvedRef(f.targetPageID.String(), f.targetHeadID.String(), "Target Page")),
		headingBlock(f.h2a.String(), 2, textNode("Details")),
		paragraphBlock(f.p2.String(), unresolvedRef("main", "Missing Page")),
		headingBlock(f.h2b.String(), 2, textNode("Intro")),
		headingBlock(f.h3.String(), 3, textNode("Deep 深")),
	)
	rev2 := republishAST(t, svc, f.sourcePageID, actorID, f.sourceRevID, astV2)
	dispatchPublishEvent(t, d, newPageBuilderRegistry(d), f.sourcePageID, rev2)

	wantLinks := []string{
		fmt.Sprintf("%s|%s|%s|%s|-|-|%s|resolved|Target Page",
			rev2, f.p1, "1", f.targetPageID, f.targetHeadID),
		fmt.Sprintf("%s|%s|%s|-|%s|%s|-|unresolved|Missing Page",
			rev2, f.p2, "0", testkit.MainNamespaceID, "Missing Page"),
	}
	assertRowsEqual(t, "v2 page_link_projection", dumpLinkRows(t, d, f.sourcePageID), wantLinks)

	// 大纲同步指向 v2。
	for _, row := range dumpOutlineRows(t, d, f.sourcePageID) {
		if !strings.HasPrefix(row, rev2.String()+"|") {
			t.Fatalf("v2 后大纲行 revision 前缀异常: %s（期望 %s）", row, rev2)
		}
	}
}

// TestRebuildPageRestoresPageProjections 删除三张投影表行后 RebuildPage 恢复
// 等价结果（INV-03 具体投影落点）。
func TestRebuildPageRestoresPageProjections(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	f := setupPageProjection(t, d)

	linksBefore := dumpLinkRows(t, d, f.sourcePageID)
	outlineBefore := dumpOutlineRows(t, d, f.sourcePageID)
	anchorsBefore := dumpAnchorRows(t, d, f.sourcePageID)

	keyCol := map[string]string{
		"page_link_projection":        "source_page_id",
		"document_outline_projection": "page_id",
		"page_anchor":                 "page_id",
	}
	for table, col := range keyCol {
		if _, err := d.Pool.Exec(context.Background(),
			fmt.Sprintf(`DELETE FROM %s WHERE %s = $1`, table, col), f.sourcePageID); err != nil {
			t.Fatalf("清空 %s 失败: %v", table, err)
		}
	}

	rebuilder := projection.NewRebuilder(d.Pool, newPageBuilderRegistry(d), nil)
	rebuilt, err := rebuilder.RebuildPage(context.Background(), f.sourcePageID)
	if err != nil || !rebuilt {
		t.Fatalf("RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}

	assertRowsEqual(t, "重建后 page_link_projection", dumpLinkRows(t, d, f.sourcePageID), linksBefore)
	assertRowsEqual(t, "重建后 document_outline_projection", dumpOutlineRows(t, d, f.sourcePageID), outlineBefore)
	assertRowsEqual(t, "重建后 page_anchor", dumpAnchorRows(t, d, f.sourcePageID), anchorsBefore)
}
