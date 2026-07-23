// 未解析链接 Resolver 集成测试（M3-T04，真实 PostgreSQL）。
// 覆盖：page.created / page.renamed 触发解析、AST 权威内容不变、
// 同名页面+别名歧义保持 unresolved、created 先于 published 的乱序兜底
// （published 路径 ResolvePageLinks hook）、旧 Revision 投影的版本防护、
// 同一事件重复投递的幂等性。
package projection_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

// deliverOutboxEvent 从 outbox_event 表取出该聚合最新一条指定类型的事件，
// 原样（真实 payload）投递给 handler——等价于 Worker 消费该事件。
func deliverOutboxEvent(t *testing.T, d *testkit.DB, h projection.Handler, aggregateID uuid.UUID, eventType string) {
	t.Helper()
	var e projection.Event
	err := d.Pool.QueryRow(context.Background(), `
		SELECT id, aggregate_id, event_type, payload_json
		FROM outbox_event
		WHERE aggregate_id = $1 AND event_type = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, aggregateID, eventType).
		Scan(&e.ID, &e.AggregateID, &e.EventType, &e.Payload)
	if err != nil {
		t.Fatalf("读取 outbox 事件 %s/%s 失败: %v", aggregateID, eventType, err)
	}
	e.AggregateType = "page"
	e.IdempotencyKey = e.ID.String()
	if err := h.Handle(context.Background(), e); err != nil {
		t.Fatalf("投递事件 %s 失败: %v", eventType, err)
	}
}

// snapshotAST 读取指定 Revision 的权威 AST 字节（用于断言 Resolver 不改 AST）。
func snapshotAST(t *testing.T, d *testkit.DB, revisionID uuid.UUID) []byte {
	t.Helper()
	var raw []byte
	err := d.Pool.QueryRow(context.Background(), `
		SELECT cs.ast_json::text
		FROM revision r JOIN content_snapshot cs ON cs.id = r.content_snapshot_id
		WHERE r.id = $1`, revisionID).Scan(&raw)
	if err != nil {
		t.Fatalf("读取 revision %s 的 AST 失败: %v", revisionID, err)
	}
	return raw
}

// publishUnresolvedSource 搭建含一条未解析引用「targetTitle」（main 命名空间）的
// 源页：直接 SQL 建页（不需要它的 created 事件）→ 发布 v1 → 分发 published 事件
// 生成投影，返回 (sourcePageID, blockID, revisionID)。
func publishUnresolvedSource(t *testing.T, d *testkit.DB, targetTitle string) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "alice")
	sourceID := d.MakePage(t, testkit.MainNamespaceID, "resolver source", "Resolver Source", testkit.SystemActorID)
	blockID := d.NewID(t)
	revID := publishAST(t, svc, sourceID, actorID, document(
		paragraphBlock(blockID.String(), textNode("see ")+","+unresolvedRef("main", targetTitle)),
	))
	dispatchPublishEvent(t, d, newPageBuilderRegistry(d), sourceID, revID)
	return sourceID, blockID, revID
}

// wantUnresolvedRow / wantResolvedRow 生成 dumpLinkRows 的期望行。
func wantUnresolvedRow(revID, blockID uuid.UUID, title string) string {
	return fmt.Sprintf("%s|%s|%s|-|%s|%s|-|unresolved|%s",
		revID, blockID, "1", testkit.MainNamespaceID, title, title)
}

func wantResolvedRow(revID, blockID, targetID uuid.UUID, title string) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|-|resolved|%s",
		revID, blockID, "1", targetID, testkit.MainNamespaceID, title, title)
}

// createPageViaService 经领域服务建页（写 page.created outbox，供 Resolver 消费）。
func createPageViaService(t *testing.T, d *testkit.DB, title string) uuid.UUID {
	t.Helper()
	p, err := newPageService(d).CreatePage(context.Background(), page.CreatePageParams{
		WikiID:      testkit.DefaultWikiID,
		NamespaceID: testkit.MainNamespaceID,
		Title:       title,
		ActorID:     d.MakeActor(t, "human", "bob"),
	})
	if err != nil {
		t.Fatalf("CreatePage(%q) 失败: %v", title, err)
	}
	return p.ID
}

// TestResolverResolvesOnPageCreated 发布含未解析引用「金星」的页面 A →
// 创建页面「金星」→ 消费 page.created → A 的投影行变 resolved 指向新页；
// 权威 AST（content_snapshot.ast_json）保持原样；同一事件重复投递结果一致（幂等）。
func TestResolverResolvesOnPageCreated(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	sourceID, blockID, revID := publishUnresolvedSource(t, d, "金星")
	astBefore := snapshotAST(t, d, revID)
	assertRowsEqual(t, "解析前 page_link_projection",
		dumpLinkRows(t, d, sourceID), []string{wantUnresolvedRow(revID, blockID, "金星")})

	targetID := createPageViaService(t, d, "金星")
	resolver := projection.NewLinkResolver(d.Pool, nil)
	deliverOutboxEvent(t, d, resolver, targetID, "page.created")

	want := []string{wantResolvedRow(revID, blockID, targetID, "金星")}
	assertRowsEqual(t, "解析后 page_link_projection", dumpLinkRows(t, d, sourceID), want)

	// 幂等：同一事件重放，投影不变。
	deliverOutboxEvent(t, d, resolver, targetID, "page.created")
	assertRowsEqual(t, "重放后 page_link_projection", dumpLinkRows(t, d, sourceID), want)

	// AST 不变：Resolver 只影响投影展示层，权威内容保持未解析形态。
	if got := snapshotAST(t, d, revID); string(got) != string(astBefore) {
		t.Fatalf("Resolver 处理后 AST 被修改\nbefore: %s\n after: %s", astBefore, got)
	}
}

// TestResolverResolvesOnPageRenamed 改名事件同样触发新标题的解析：
// 页面「晨星」改名为「金星」→ 消费 page.renamed → 投影行 resolved 指向该页
// （Page ID 稳定，target_page_id 与标题无关）。
func TestResolverResolvesOnPageRenamed(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	sourceID, blockID, revID := publishUnresolvedSource(t, d, "金星")
	targetID := createPageViaService(t, d, "晨星")

	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "carol")
	if _, err := svc.RenamePage(context.Background(), targetID, "金星", actorID); err != nil {
		t.Fatalf("RenamePage 失败: %v", err)
	}
	deliverOutboxEvent(t, d, projection.NewLinkResolver(d.Pool, nil), targetID, "page.renamed")

	assertRowsEqual(t, "改名解析后 page_link_projection",
		dumpLinkRows(t, d, sourceID), []string{wantResolvedRow(revID, blockID, targetID, "金星")})
}

// TestResolverAmbiguousStaysUnresolved 同名页面 + 另一页别名同时命中
// 「金星」（两个候选）→ 保持 unresolved，不做猜测性解析。
func TestResolverAmbiguousStaysUnresolved(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	sourceID, blockID, revID := publishUnresolvedSource(t, d, "金星")
	titleHitID := createPageViaService(t, d, "金星")

	// 直接 SQL 给另一页挂别名「金星」（服务层的标题占用检查会阻止该状态，
	// 这里模拟数据层可能存在的同名页面+别名歧义）。
	aliasPageID := createPageViaService(t, d, "启明星")
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO page_alias (id, wiki_id, namespace_id, normalized_title, page_id, alias_type)
		VALUES ($1, $2, $3, '金星', $4, 'import')`,
		d.NewID(t), testkit.DefaultWikiID, testkit.MainNamespaceID, aliasPageID); err != nil {
		t.Fatalf("插入歧义别名失败: %v", err)
	}

	deliverOutboxEvent(t, d, projection.NewLinkResolver(d.Pool, nil), titleHitID, "page.created")

	assertRowsEqual(t, "歧义时保持 unresolved",
		dumpLinkRows(t, d, sourceID), []string{wantUnresolvedRow(revID, blockID, "金星")})
}

// TestResolverOutOfOrderCreatedBeforePublished 乱序：page.created 先于引用所在
// Revision 的 page.revision_published 被消费——created 处理时无投影行（正常），
// published 分发后的 ResolvePageLinks hook 兜底完成解析，最终一致。
func TestResolverOutOfOrderCreatedBeforePublished(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "alice")
	resolver := projection.NewLinkResolver(d.Pool, nil)

	// 目标页先创建，其 created 事件先被消费——此刻还没有任何引用投影行。
	targetID := createPageViaService(t, d, "金星")
	deliverOutboxEvent(t, d, resolver, targetID, "page.created")

	// 源页随后发布，published 事件走与 cmd/worker 相同的「分发 + hook」路径。
	sourceID := d.MakePage(t, testkit.MainNamespaceID, "resolver source", "Resolver Source", testkit.SystemActorID)
	blockID := d.NewID(t)
	revID := publishAST(t, svc, sourceID, actorID, document(
		paragraphBlock(blockID.String(), textNode("see ")+","+unresolvedRef("main", "金星")),
	))
	wrapped := resolver.WrapPublishedHandler(
		projection.NewRevisionPublishedHandler(d.Pool, newPageBuilderRegistry(d), nil))
	deliverOutboxEvent(t, d, wrapped, sourceID, "page.revision_published")

	assertRowsEqual(t, "乱序后 page_link_projection",
		dumpLinkRows(t, d, sourceID), []string{wantResolvedRow(revID, blockID, targetID, "金星")})
}

// TestResolverVersionGuardSkipsStaleRevision 版本防护（设计 §15）：
// 源页发 v2（移除引用）后、v2 事件尚未分发时，迟到的 page.created 不得更新
// v1 时期的投影行（source_revision 已非 current）；v2 分发后投影随新 AST 清空。
func TestResolverVersionGuardSkipsStaleRevision(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	svc := newPageService(d)
	actorID := d.MakeActor(t, "human", "alice")
	sourceID, blockID, rev1 := publishUnresolvedSource(t, d, "金星")

	// v2 移除该引用；current 指针已指向 v2，但 v2 事件尚未分发
	// （投影仍是 v1 的行——正是乱序事件需要防护的状态）。
	rev2 := republishAST(t, svc, sourceID, actorID, rev1, document(
		paragraphBlock(blockID.String(), textNode("no reference anymore")),
	))

	targetID := createPageViaService(t, d, "金星")
	deliverOutboxEvent(t, d, projection.NewLinkResolver(d.Pool, nil), targetID, "page.created")

	// 防护生效：v1 的行（source_revision 已非 current）保持 unresolved。
	assertRowsEqual(t, "版本防护期间 page_link_projection",
		dumpLinkRows(t, d, sourceID), []string{wantUnresolvedRow(rev1, blockID, "金星")})

	// v2 事件分发后投影按新 AST 重建，引用行消失。
	dispatchPublishEvent(t, d, newPageBuilderRegistry(d), sourceID, rev2)
	assertRowsEqual(t, "v2 分发后 page_link_projection", dumpLinkRows(t, d, sourceID), nil)
}

// TestResolverIdempotentDuplicateDeliveries 同一 page.created 事件重复投递
// （至少一次语义下的重放）：多次处理结果与一次处理一致。
func TestResolverIdempotentDuplicateDeliveries(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	sourceID, _, _ := publishUnresolvedSource(t, d, "金星")
	targetID := createPageViaService(t, d, "金星")
	resolver := projection.NewLinkResolver(d.Pool, nil)

	deliverOutboxEvent(t, d, resolver, targetID, "page.created")
	first := dumpLinkRows(t, d, sourceID)
	for i := 0; i < 3; i++ {
		deliverOutboxEvent(t, d, resolver, targetID, "page.created")
	}
	assertRowsEqual(t, "重复投递后 page_link_projection", dumpLinkRows(t, d, sourceID), first)
}

// TestResolvePageLinksHookSkipsUnpublishedOrStale ResolvePageLinks 的防护：
// 页面从未发布（无 current Revision）或全部行属旧 Revision 时不报错、不更新。
func TestResolvePageLinksHookSkipsUnpublishedOrStale(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	resolver := projection.NewLinkResolver(d.Pool, nil)
	unpublished := d.MakePage(t, testkit.MainNamespaceID, "never published", "Never Published", testkit.SystemActorID)
	if err := resolver.ResolvePageLinks(context.Background(), unpublished); err != nil {
		t.Fatalf("未发布页面 ResolvePageLinks 应无错返回: %v", err)
	}
	nonexistent := d.NewID(t)
	if err := resolver.ResolvePageLinks(context.Background(), nonexistent); err != nil {
		t.Fatalf("不存在页面 ResolvePageLinks 应无错返回: %v", err)
	}
}

// 确保 payload 解析失败的路径有明确错误（防御性：载荷缺 normalized_title）。
func TestResolverRejectsMalformedPayload(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	resolver := projection.NewLinkResolver(d.Pool, nil)
	event := projection.Event{
		ID:             d.NewID(t),
		IdempotencyKey: d.NewID(t).String(),
		AggregateType:  "page",
		AggregateID:    d.NewID(t),
		EventType:      "page.created",
		Payload:        json.RawMessage(`{"page_id":"x","namespace_id":"not-a-uuid"}`),
	}
	if err := resolver.Handle(context.Background(), event); err == nil {
		t.Fatal("非法 payload 应返回错误（事件进入退避重试而非静默吞掉）")
	}
}
