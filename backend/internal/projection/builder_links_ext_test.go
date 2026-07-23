// 外链使用投影集成测试（M3-T06，真实 PostgreSQL）。
package projection_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

func externalLinkNode(rawURL, display string) string {
	return fmt.Sprintf(`{"type":"external_link","url":%q,"display_text":%q}`, rawURL, display)
}

func newExternalLinksRegistry(d *testkit.DB) *projection.Registry {
	registry := projection.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry.Register(projection.NewExternalLinksBuilder(d.Pool, logger))
	return registry
}

type externalLinksFixture struct {
	pageID  uuid.UUID
	revID   uuid.UUID
	blockID uuid.UUID
}

func setupExternalLinks(t *testing.T, d *testkit.DB) *externalLinksFixture {
	t.Helper()
	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "alice")
	fixture := &externalLinksFixture{
		pageID:  d.MakePage(t, testkit.MainNamespaceID, "external links", "External Links", testkit.SystemActorID),
		blockID: d.NewID(t),
	}
	fixture.revID = publishAST(t, svc, fixture.pageID, actorID, document(
		paragraphBlock(fixture.blockID.String(), strings.Join([]string{
			textNode("links: "),
			externalLinkNode("HTTPS://Example.COM:443/docs/../Guide/?utm_source=x&b=2&a=1#frag", "first"),
			externalLinkNode("https://example.com/Guide?a=1&b=2&utm_medium=y", "same"),
			externalLinkNode("javascript:alert(1)", "unsafe"),
			externalLinkNode("HTTP://Other.EXAMPLE:80/", "other"),
		}, ",")),
	))
	dispatchPublishEvent(t, d, newExternalLinksRegistry(d), fixture.pageID, fixture.revID)
	return fixture
}

func dumpExternalLinkUsage(t *testing.T, d *testkit.DB, pageID uuid.UUID) []string {
	t.Helper()
	rows, err := d.Pool.Query(context.Background(), `
		SELECT er.normalized_url, elu.revision_id, elu.block_id, elu.node_id, elu.link_role
		FROM external_link_usage elu
		JOIN external_resource er ON er.id = elu.external_resource_id
		WHERE elu.page_id = $1`, pageID)
	if err != nil {
		t.Fatalf("读取 external_link_usage 失败: %v", err)
	}
	defer rows.Close()
	return dumpRows(t, rows, 5)
}

func externalResourceCount(t *testing.T, d *testkit.DB) int {
	t.Helper()
	var count int
	if err := d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM external_resource`).Scan(&count); err != nil {
		t.Fatalf("统计 external_resource 失败: %v", err)
	}
	return count
}

func TestExternalLinksBuilderNormalizesDeduplicatesAndSkipsInvalid(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	fixture := setupExternalLinks(t, d)

	// 两个等价 example URL 共用一条资源；另一个 host 一条；javascript: 被跳过。
	if got := externalResourceCount(t, d); got != 2 {
		t.Fatalf("external_resource 行数 = %d，期望 2", got)
	}
	want := []string{
		fmt.Sprintf("https://example.com/Guide?a=1&b=2|%s|%s|1|inline", fixture.revID, fixture.blockID),
		fmt.Sprintf("https://example.com/Guide?a=1&b=2|%s|%s|2|inline", fixture.revID, fixture.blockID),
		fmt.Sprintf("http://other.example/|%s|%s|4|inline", fixture.revID, fixture.blockID),
	}
	assertRowsEqual(t, "external_link_usage", dumpExternalLinkUsage(t, d, fixture.pageID), want)

	var original string
	if err := d.Pool.QueryRow(context.Background(), `
		SELECT original_url FROM external_resource
		WHERE normalized_url = 'https://example.com/Guide?a=1&b=2'`).Scan(&original); err != nil {
		t.Fatalf("读取 original_url 失败: %v", err)
	}
	if !strings.HasPrefix(original, "HTTPS://Example.COM") {
		t.Fatalf("original_url = %q，期望保留首次出现原文", original)
	}
}

func TestExternalLinksBuilderRepublishRetainsUnreferencedResource(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	fixture := setupExternalLinks(t, d)
	beforeResources := externalResourceCount(t, d)
	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "bob")
	newBlockID := d.NewID(t)
	rev2 := republishAST(t, svc, fixture.pageID, actorID, fixture.revID, document(
		paragraphBlock(newBlockID.String(), externalLinkNode("https://new.example/path", "new")),
	))
	dispatchPublishEvent(t, d, newExternalLinksRegistry(d), fixture.pageID, rev2)

	want := []string{
		fmt.Sprintf("https://new.example/path|%s|%s|0|inline", rev2, newBlockID),
	}
	assertRowsEqual(t, "v2 external_link_usage", dumpExternalLinkUsage(t, d, fixture.pageID), want)
	if got := externalResourceCount(t, d); got != beforeResources+1 {
		t.Fatalf("资源行数 = %d，期望 %d（不再引用的资源仍保留）", got, beforeResources+1)
	}
}

func TestExternalLinksBuilderRebuildRestoresProjection(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	fixture := setupExternalLinks(t, d)
	before := dumpExternalLinkUsage(t, d, fixture.pageID)
	if _, err := d.Pool.Exec(context.Background(),
		`DELETE FROM external_link_usage WHERE page_id = $1`, fixture.pageID); err != nil {
		t.Fatalf("删除 usage 失败: %v", err)
	}

	rebuilder := projection.NewRebuilder(d.Pool, newExternalLinksRegistry(d), nil)
	rebuilt, err := rebuilder.RebuildPage(context.Background(), fixture.pageID)
	if err != nil || !rebuilt {
		t.Fatalf("RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}
	assertRowsEqual(t, "重建后的 external_link_usage", dumpExternalLinkUsage(t, d, fixture.pageID), before)
}

func TestExternalLinksBuilderDuplicateEventIsIdempotent(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	fixture := setupExternalLinks(t, d)
	before := dumpExternalLinkUsage(t, d, fixture.pageID)
	dispatchPublishEvent(t, d, newExternalLinksRegistry(d), fixture.pageID, fixture.revID)
	assertRowsEqual(t, "重复事件后的 external_link_usage", dumpExternalLinkUsage(t, d, fixture.pageID), before)
}
