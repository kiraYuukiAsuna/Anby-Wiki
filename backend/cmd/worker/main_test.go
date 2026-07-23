// worker 重建命令（-rebuild-page / -rebuild-all）集成测试
// （真实 PostgreSQL，TEST_DATABASE_URL 未配置时 skip）。
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/platform/config"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

func TestAssembleImportRunner_BootstrapsAuthoritativePrompt(t *testing.T) {
	if !setupEnv(t) {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	d := testkit.Open(t)
	d.Reset(t)
	cfg := config.Config{Env: "test", S3Endpoint: "http://localhost:9000", S3Region: "us-east-1",
		S3Bucket: "test", S3AccessKey: "test", S3SecretKey: "test", AIProvider: "openai-compatible",
		AIBaseURL: "https://provider.invalid/v1", AIAPIKey: "secret", AIModel: "test-model"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := assembleImportRunner(context.Background(), d.Pool, cfg, logger); err != nil {
		t.Fatal(err)
	}
	if _, err := assembleImportRunner(context.Background(), d.Pool, cfg, logger); err != nil {
		t.Fatalf("重复装配应幂等: %v", err)
	}
	var active, versions int
	if err := d.Pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE active),count(*)
		FROM prompt_template WHERE prompt_key='source-extraction-v1'`).Scan(&active, &versions); err != nil {
		t.Fatal(err)
	}
	if active != 1 || versions != 1 {
		t.Fatalf("active=%d versions=%d", active, versions)
	}
}

// setupEnv 配置 run() 需要的环境变量（DATABASE_URL 取测试库，其余占位）。
// 返回 false 表示 TEST_DATABASE_URL 未配置（用例应 skip）。
func setupEnv(t *testing.T) bool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		return false
	}
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("S3_BUCKET", "test")
	t.Setenv("S3_ACCESS_KEY", "test")
	t.Setenv("S3_SECRET_KEY", "test")
	return true
}

func TestCheckExternalLinksFlagRunsEmptyBatch(t *testing.T) {
	if !setupEnv(t) {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	d := testkit.Open(t)
	d.Reset(t)
	if code := run([]string{"-check-external-links"}); code != exitOK {
		t.Fatalf("run(-check-external-links) 退出码 = %d，期望 0", code)
	}
}

// makeRichPublishedPage 搭建同时含 heading/page_reference/external_link 的页面，
// 用于验证 worker 注册的全部 M3 投影可整体删除后恢复。
func makeRichPublishedPage(t *testing.T, d *testkit.DB, title string) string {
	t.Helper()
	ctx := context.Background()
	pid := d.MakePage(t, testkit.MainNamespaceID, title, title, testkit.SystemActorID)
	astJSON := fmt.Sprintf(`{
		"type":"document","schema_version":1,"children":[
			{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4101","type":"heading","level":2,"content":[{"type":"text","text":"Overview"}]},
			{"id":"0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4102","type":"paragraph","content":[
				{"type":"page_reference","target_page_id":"%s","display_text":"Self"},
				{"type":"text","text":" / "},
				{"type":"external_link","url":"https://EXAMPLE.com:443/docs#fragment","display_text":"Docs"}
			]}
		]}`, pid)
	snapID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO content_snapshot (id, schema_version, ast_json, content_hash, size_bytes)
		VALUES ($1, 1, $2::jsonb, $3, $4)`, snapID, astJSON, snapID.String(), len(astJSON)); err != nil {
		t.Fatalf("插入 rich content_snapshot 失败: %v", err)
	}
	revID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO revision (id, page_id, content_snapshot_id, actor_id)
		VALUES ($1, $2, $3, $4)`, revID, pid, snapID, testkit.SystemActorID); err != nil {
		t.Fatalf("插入 rich revision 失败: %v", err)
	}
	if _, err := d.Pool.Exec(ctx,
		`UPDATE page SET current_revision_id = $2 WHERE id = $1`, pid, revID); err != nil {
		t.Fatalf("移动 rich current 指针失败: %v", err)
	}
	return pid.String()
}

type projectionSnapshot struct {
	links, outline, anchors, rendered, externalLinks, states int
	html, rendererVersion, contentHash                       string
}

func readProjectionSnapshot(t *testing.T, d *testkit.DB, pageID string) projectionSnapshot {
	t.Helper()
	var s projectionSnapshot
	if err := d.Pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM page_link_projection WHERE source_page_id = $1),
			(SELECT count(*) FROM document_outline_projection WHERE page_id = $1),
			(SELECT count(*) FROM page_anchor WHERE page_id = $1),
			(SELECT count(*) FROM rendered_page WHERE page_id = $1),
			(SELECT count(*) FROM external_link_usage WHERE page_id = $1),
			(SELECT count(*) FROM projection_state WHERE aggregate_type = 'page' AND aggregate_id = $1),
			COALESCE((SELECT html_content FROM rendered_page WHERE page_id = $1), ''),
			COALESCE((SELECT renderer_version FROM rendered_page WHERE page_id = $1), ''),
			COALESCE((SELECT content_hash FROM rendered_page WHERE page_id = $1), '')`, pageID).
		Scan(&s.links, &s.outline, &s.anchors, &s.rendered, &s.externalLinks, &s.states,
			&s.html, &s.rendererVersion, &s.contentHash); err != nil {
		t.Fatalf("读取投影快照失败: %v", err)
	}
	return s
}

// makePublishedPage 直接 SQL 搭建一个已发布页面，返回 pageID。
func makePublishedPage(t *testing.T, d *testkit.DB, title string) (pageID string) {
	t.Helper()
	ctx := context.Background()
	pid := d.MakePage(t, testkit.MainNamespaceID, title, title, testkit.SystemActorID)
	snapID := d.NewID(t)
	astJSON := `{"type":"document","schema_version":1,"children":[]}`
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO content_snapshot (id, schema_version, ast_json, content_hash, size_bytes)
		VALUES ($1, 1, $2::jsonb, $3, $4)`, snapID, astJSON, snapID.String(), len(astJSON)); err != nil {
		t.Fatalf("插入 content_snapshot 失败: %v", err)
	}
	revID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO revision (id, page_id, content_snapshot_id, actor_id)
		VALUES ($1, $2, $3, $4)`, revID, pid, snapID, testkit.SystemActorID); err != nil {
		t.Fatalf("插入 revision 失败: %v", err)
	}
	if _, err := d.Pool.Exec(ctx,
		`UPDATE page SET current_revision_id = $2 WHERE id = $1`, pid, revID); err != nil {
		t.Fatalf("移动 current 指针失败: %v", err)
	}
	return pid.String()
}

// TestRebuildPageFlag -rebuild-page 对已发布页面执行成功（退出码 0）。
func TestRebuildPageFlag(t *testing.T) {
	if !setupEnv(t) {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	d := testkit.Open(t)
	d.Reset(t)
	pageID := makePublishedPage(t, d, "worker-rebuild-page")

	if code := run([]string{"-rebuild-page", pageID}); code != exitOK {
		t.Fatalf("run(-rebuild-page) 退出码 = %d，期望 0", code)
	}
}

// TestRebuildPageFlagUnpublished -rebuild-page 对从未发布页面跳过（退出码 0）。
func TestRebuildPageFlagUnpublished(t *testing.T) {
	if !setupEnv(t) {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	d := testkit.Open(t)
	d.Reset(t)
	pid := d.MakePage(t, testkit.MainNamespaceID, "worker-unpublished", "worker-unpublished", testkit.SystemActorID)

	if code := run([]string{"-rebuild-page", pid.String()}); code != exitOK {
		t.Fatalf("run(-rebuild-page 未发布页) 退出码 = %d，期望 0", code)
	}
}

// TestRebuildPageFlagInvalidID 非法 Page ID → 退出码 2（参数错误）。
func TestRebuildPageFlagInvalidID(t *testing.T) {
	if !setupEnv(t) {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	if code := run([]string{"-rebuild-page", "not-a-uuid"}); code != exitUsage {
		t.Fatalf("run(-rebuild-page not-a-uuid) 退出码 = %d，期望 2", code)
	}
}

// TestRebuildPageFlagNotFound 页面不存在 → 退出码 1。
func TestRebuildPageFlagNotFound(t *testing.T) {
	if !setupEnv(t) {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	d := testkit.Open(t)
	d.Reset(t)

	if code := run([]string{"-rebuild-page", d.NewID(t).String()}); code != exitError {
		t.Fatalf("run(-rebuild-page 不存在页面) 退出码 = %d，期望 1", code)
	}
}

// TestRebuildAllFlag -rebuild-all 全量重放执行成功（退出码 0）。
func TestRebuildAllFlag(t *testing.T) {
	if !setupEnv(t) {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	d := testkit.Open(t)
	d.Reset(t)
	makePublishedPage(t, d, "worker-rebuild-all-1")
	d.MakePage(t, testkit.MainNamespaceID, "worker-rebuild-all-2", "worker-rebuild-all-2", testkit.SystemActorID)

	if code := run([]string{"-rebuild-all"}); code != exitOK {
		t.Fatalf("run(-rebuild-all) 退出码 = %d，期望 0", code)
	}
	var searchDocuments int
	if err := d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM search_document`).
		Scan(&searchDocuments); err != nil {
		t.Fatal(err)
	}
	if searchDocuments != 1 {
		t.Fatalf("search documents = %d, want 1 published page", searchDocuments)
	}
}

func TestRegisterBuildersIncludesSearch(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	registry := projection.NewRegistry()
	searchBuilder := registerBuilders(registry, d.Pool, nil)
	if searchBuilder == nil {
		t.Fatal("registerBuilders returned nil SearchBuilder")
	}
	if registry.Len() != 9 {
		t.Fatalf("registered builders = %d, want 9", registry.Len())
	}
	foundSearch := false
	foundComponentDependency := false
	for _, builder := range registry.Builders() {
		if builder.Type() == projection.ProjectionSearch {
			foundSearch = true
		}
		if builder.Type() == projection.ProjectionComponentDependency {
			foundComponentDependency = true
		}
	}
	if !foundSearch {
		t.Fatal("search builder not registered")
	}
	if !foundComponentDependency {
		t.Fatal("component dependency builder not registered")
	}
}

// TestM3AllPageProjectionsRebuild 是 MT-M3-RENDER-REBUILD / INV-03 的里程碑门禁：
// 删除一页全部 M3 投影及状态后，从 Current Revision 整体重建且结果等价。
func TestM3AllPageProjectionsRebuild(t *testing.T) {
	if !setupEnv(t) {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	d := testkit.Open(t)
	d.Reset(t)
	pageID := makeRichPublishedPage(t, d, "m3-all-projections-rebuild")

	registry := projection.NewRegistry()
	registry.Register(projection.NewPageLinksBuilder(d.Pool))
	registry.Register(projection.NewOutlineBuilder(d.Pool))
	registry.Register(projection.NewRenderedPageBuilder(d.Pool))
	registry.Register(projection.NewExternalLinksBuilder(d.Pool, nil))
	rebuilder := projection.NewRebuilder(d.Pool, registry, nil)
	if rebuilt, err := rebuilder.RebuildPage(context.Background(), uuid.MustParse(pageID)); err != nil || !rebuilt {
		t.Fatalf("首次 RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}
	before := readProjectionSnapshot(t, d, pageID)
	if before.links != 1 || before.outline != 1 || before.anchors != 1 || before.rendered != 1 || before.externalLinks != 1 || before.states != 4 {
		t.Fatalf("首次投影不完整: %+v", before)
	}

	deleteStatements := []string{
		`DELETE FROM page_link_projection WHERE source_page_id = $1`,
		`DELETE FROM document_outline_projection WHERE page_id = $1`,
		`DELETE FROM page_anchor WHERE page_id = $1`,
		`DELETE FROM rendered_page WHERE page_id = $1`,
		`DELETE FROM external_link_usage WHERE page_id = $1`,
		`DELETE FROM projection_state WHERE aggregate_type = 'page' AND aggregate_id = $1`,
	}
	for _, statement := range deleteStatements {
		if _, err := d.Pool.Exec(context.Background(), statement, pageID); err != nil {
			t.Fatalf("删除页面全部投影失败: %v", err)
		}
	}
	if empty := readProjectionSnapshot(t, d, pageID); empty.links+empty.outline+empty.anchors+empty.rendered+empty.externalLinks+empty.states != 0 {
		t.Fatalf("删除后仍有投影残留: %+v", empty)
	}

	if rebuilt, err := rebuilder.RebuildPage(context.Background(), uuid.MustParse(pageID)); err != nil || !rebuilt {
		t.Fatalf("恢复 RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}
	after := readProjectionSnapshot(t, d, pageID)
	if after != before {
		t.Fatalf("整体重建不等价\n before: %+v\n  after: %+v", before, after)
	}
}
