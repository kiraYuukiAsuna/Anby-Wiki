// Projection 通用框架集成测试（真实 PostgreSQL，testkit.Open 未配置时 skip）。
// 覆盖：版本防护（旧 Revision 事件跳过，INV-04）、乱序事件最终投影、
// RebuildPage 等价重建（INV-03 框架侧）、RebuildAll 聚合报告、
// projection_state ok/error 两态、处理后断言（处理期间又有新发布）。
package projection_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

// 测试用合法 AST v1 文档（空 document）。
const testAST = `{"type":"document","schema_version":1,"children":[]}`

// ---------------------------------------------------------------------------
// 测试场景搭建助手
// ---------------------------------------------------------------------------

// ensureFakeTable 创建 fake Builder 的投影落点表（幂等）。
func ensureFakeTable(t *testing.T, d *testkit.DB) {
	t.Helper()
	if _, err := d.Pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS test_fake_projection (
			projection_type text NOT NULL,
			page_id         uuid NOT NULL,
			revision_id     uuid NOT NULL,
			PRIMARY KEY (projection_type, page_id)
		)`); err != nil {
		t.Fatalf("创建 test_fake_projection 失败: %v", err)
	}
	if _, err := d.Pool.Exec(context.Background(), `DELETE FROM test_fake_projection`); err != nil {
		t.Fatalf("清空 test_fake_projection 失败: %v", err)
	}
}

// makePublishedPage 直接 SQL 搭建一个已发布页面（绕过领域服务），返回 pageID 与 revisionID。
// parent 非 nil 时作为 parent_revision_id；插入后 page.current_revision_id 指向新 Revision。
// 注意：不产 outbox_event，事件由用例显式注入以保证时序确定。
func makePublishedPage(t *testing.T, d *testkit.DB, title string, parent *uuid.UUID) (pageID, revID uuid.UUID) {
	t.Helper()
	pageID = d.MakePage(t, testkit.MainNamespaceID, title, title, testkit.SystemActorID)
	revID = appendRevision(t, d, pageID, parent)
	return pageID, revID
}

// appendRevision 给已有页面追加一个 Revision 并移动 current 指针，返回新 Revision ID。
func appendRevision(t *testing.T, d *testkit.DB, pageID uuid.UUID, parent *uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	snapID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO content_snapshot (id, schema_version, ast_json, content_hash, size_bytes)
		VALUES ($1, 1, $2::jsonb, $3, $4)`,
		snapID, testAST, snapID.String(), len(testAST)); err != nil {
		t.Fatalf("插入 content_snapshot 失败: %v", err)
	}
	revID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO revision (id, page_id, parent_revision_id, content_snapshot_id, actor_id)
		VALUES ($1, $2, $3, $4, $5)`,
		revID, pageID, parent, snapID, testkit.SystemActorID); err != nil {
		t.Fatalf("插入 revision 失败: %v", err)
	}
	if _, err := d.Pool.Exec(ctx,
		`UPDATE page SET current_revision_id = $2 WHERE id = $1`, pageID, revID); err != nil {
		t.Fatalf("移动 current 指针失败: %v", err)
	}
	return revID
}

// setCurrentRevision 直接把页面 current 指针拨到指定 Revision（构造乱序场景用）。
func setCurrentRevision(t *testing.T, d *testkit.DB, pageID, revID uuid.UUID) {
	t.Helper()
	if _, err := d.Pool.Exec(context.Background(),
		`UPDATE page SET current_revision_id = $2 WHERE id = $1`, pageID, revID); err != nil {
		t.Fatalf("拨动 current 指针失败: %v", err)
	}
}

// insertPublishEvent 注入一条 page.revision_published 事件（pending），
// createdAgo 控制 created_at 以决定领取顺序，返回事件 ID。
func insertPublishEvent(t *testing.T, d *testkit.DB, pageID, revID uuid.UUID, createdAgo time.Duration) uuid.UUID {
	t.Helper()
	id := d.NewID(t)
	payload := fmt.Sprintf(
		`{"page_id":%q,"revision_id":%q,"schema_version":1}`, pageID.String(), revID.String())
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO outbox_event (id, aggregate_type, aggregate_id, event_type, payload_json, created_at)
		VALUES ($1, 'page', $2, 'page.revision_published', $3::jsonb, now() - $4::interval)`,
		id, pageID, payload, createdAgo.String()); err != nil {
		t.Fatalf("注入 page.revision_published 事件失败: %v", err)
	}
	return id
}

// ---------------------------------------------------------------------------
// 记录型 fake Builder
// ---------------------------------------------------------------------------

type rebuildCall struct {
	pageID     uuid.UUID
	revisionID uuid.UUID
}

// fakeBuilder 记录 HandleEvent/Rebuild 调用，并把投影写成
// test_fake_projection 的一行（projection_type, page_id）→ revision_id。
type fakeBuilder struct {
	typ  string
	pool *pgxpool.Pool

	mu           sync.Mutex
	handleCalls  int
	rebuildCalls []rebuildCall
	// failPages 命中的 pageID 时 Rebuild 返回错误（构造 error 状态场景）。
	failPages map[uuid.UUID]bool
	// beforeRebuild 在 Rebuild 落库前触发（构造「处理期间又有新发布」场景）。
	beforeRebuild func()
}

func newFakeBuilder(pool *pgxpool.Pool, typ string) *fakeBuilder {
	return &fakeBuilder{typ: typ, pool: pool, failPages: map[uuid.UUID]bool{}}
}

func (b *fakeBuilder) Type() string { return b.typ }

func (b *fakeBuilder) Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	b.mu.Lock()
	b.rebuildCalls = append(b.rebuildCalls, rebuildCall{pageID: pageID, revisionID: revisionID})
	fail := b.failPages[pageID]
	before := b.beforeRebuild
	b.mu.Unlock()

	if fail {
		return errors.New("fake builder boom")
	}
	if before != nil {
		before()
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO test_fake_projection (projection_type, page_id, revision_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (projection_type, page_id) DO UPDATE SET revision_id = EXCLUDED.revision_id`,
		b.typ, pageID, revisionID)
	return err
}

// HandleEvent 采用框架通用实现：从事务内读当前 Revision 并 Rebuild。
func (b *fakeBuilder) HandleEvent(ctx context.Context, event projection.Event) error {
	b.mu.Lock()
	b.handleCalls++
	b.mu.Unlock()
	return projection.HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}

func (b *fakeBuilder) handleCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.handleCalls
}

func (b *fakeBuilder) rebuildCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.rebuildCalls)
}

// ---------------------------------------------------------------------------
// 断言助手
// ---------------------------------------------------------------------------

// fakeProjectionRevision 读取 fake 投影行的 revision_id；无行返回 ok=false。
func fakeProjectionRevision(t *testing.T, d *testkit.DB, projType string, pageID uuid.UUID) (uuid.UUID, bool) {
	t.Helper()
	var revID uuid.UUID
	err := d.Pool.QueryRow(context.Background(),
		`SELECT revision_id FROM test_fake_projection WHERE projection_type = $1 AND page_id = $2`,
		projType, pageID).Scan(&revID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false
	}
	if err != nil {
		t.Fatalf("读取 fake 投影行失败: %v", err)
	}
	return revID, true
}

type stateRow struct {
	status           string
	sourceRevisionID uuid.UUID
	lastError        *string
}

// readState 读取 projection_state 行；无行返回 nil。
func readState(t *testing.T, d *testkit.DB, projType string, pageID uuid.UUID) *stateRow {
	t.Helper()
	var r stateRow
	err := d.Pool.QueryRow(context.Background(), `
		SELECT status, source_revision_id, last_error
		FROM projection_state
		WHERE aggregate_type = 'page' AND aggregate_id = $1 AND projection_type = $2`,
		pageID, projType).Scan(&r.status, &r.sourceRevisionID, &r.lastError)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatalf("读取 projection_state 失败: %v", err)
	}
	return &r
}

func eventStatus(t *testing.T, d *testkit.DB, eventID uuid.UUID) string {
	t.Helper()
	var status string
	if err := d.Pool.QueryRow(context.Background(),
		`SELECT status FROM outbox_event WHERE id = $1`, eventID).Scan(&status); err != nil {
		t.Fatalf("读取事件状态失败: %v", err)
	}
	return status
}

// runFrameworkConsumer 装配「注册表 + 版本防护分发」的 Consumer 并后台运行。
func runFrameworkConsumer(t *testing.T, d *testkit.DB, reg *projection.Registry) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	c := projection.New(d.Pool, testConfig())
	c.Register("page.revision_published",
		projection.NewRevisionPublishedHandler(d.Pool, reg, nil))
	return runConsumer(c)
}

// ---------------------------------------------------------------------------
// 用例
// ---------------------------------------------------------------------------

// TestRegistryContract 注册表契约（纯单测，不需要 DB）：
// 正常注册、按注册序枚举、nil/重复 Type 注册 panic。
func TestRegistryContract(t *testing.T) {
	reg := projection.NewRegistry()
	reg.Register(newFakeBuilder(nil, "a"))
	reg.Register(newFakeBuilder(nil, "b"))
	if reg.Len() != 2 {
		t.Fatalf("Len = %d, 期望 2", reg.Len())
	}
	got := reg.Builders()
	if got[0].Type() != "a" || got[1].Type() != "b" {
		t.Fatalf("Builders 顺序 = [%q %q], 期望 [a b]", got[0].Type(), got[1].Type())
	}

	assertPanics(t, "nil Builder", func() { projection.NewRegistry().Register(nil) })
	assertPanics(t, "重复 Type", func() {
		r := projection.NewRegistry()
		r.Register(newFakeBuilder(nil, "x"))
		r.Register(newFakeBuilder(nil, "x"))
	})
}

func assertPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s 应 panic", name)
		}
	}()
	fn()
}

// TestVersionGuardSkipsStaleEvent 版本防护（INV-04 核心语义）：
// 页面已发布 v2 后，把 v1 的事件重新塞入 outbox（pending）→ Consumer 处理 →
// v1 事件被跳过（done），fake Builder 未被调用，projection_state 无记录。
func TestVersionGuardSkipsStaleEvent(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ensureFakeTable(t, d)

	pageID, rev1 := makePublishedPage(t, d, "guard-stale", nil)
	_ = appendRevision(t, d, pageID, &rev1) // current 指向 v2
	staleEventID := insertPublishEvent(t, d, pageID, rev1, 0)

	fake := newFakeBuilder(d.Pool, "fake_links")
	reg := projection.NewRegistry()
	reg.Register(fake)
	cancel, done := runFrameworkConsumer(t, d, reg)
	defer func() { cancel(); <-done }()

	waitFor(t, 5*time.Second, "v1 旧事件处理完（done）", func() bool {
		return eventStatus(t, d, staleEventID) == "done"
	})
	if n := fake.handleCount(); n != 0 {
		t.Fatalf("过期事件触发了 Builder.HandleEvent %d 次，期望 0", n)
	}
	if n := fake.rebuildCount(); n != 0 {
		t.Fatalf("过期事件触发了 Rebuild %d 次，期望 0", n)
	}
	if _, ok := fakeProjectionRevision(t, d, "fake_links", pageID); ok {
		t.Fatal("过期事件不应写出投影行")
	}
	if st := readState(t, d, "fake_links", pageID); st != nil {
		t.Fatalf("被跳过的 Builder 不应写 projection_state，实际 %+v", st)
	}
}

// TestOutOfOrderEventsFinalProjection 乱序事件（INV-04 门禁）：
// v2 事件先到达、v1 事件后到达（created_at 更晚），最终投影必须反映 v2。
func TestOutOfOrderEventsFinalProjection(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ensureFakeTable(t, d)

	pageID, rev1 := makePublishedPage(t, d, "out-of-order", nil)
	rev2 := appendRevision(t, d, pageID, &rev1)                      // current = v2
	eventV2 := insertPublishEvent(t, d, pageID, rev2, 2*time.Minute) // 先被领取
	eventV1 := insertPublishEvent(t, d, pageID, rev1, 1*time.Minute) // 后到达的旧事件

	fake := newFakeBuilder(d.Pool, "fake_links")
	reg := projection.NewRegistry()
	reg.Register(fake)
	cancel, done := runFrameworkConsumer(t, d, reg)
	defer func() { cancel(); <-done }()

	waitFor(t, 5*time.Second, "两个事件都处理完", func() bool {
		return eventStatus(t, d, eventV2) == "done" && eventStatus(t, d, eventV1) == "done"
	})

	// v1 事件被版本防护跳过；v2 事件正常重建。
	if n := fake.handleCount(); n != 1 {
		t.Fatalf("Builder.HandleEvent 调用 %d 次，期望 1（仅 v2）", n)
	}
	got, ok := fakeProjectionRevision(t, d, "fake_links", pageID)
	if !ok {
		t.Fatal("投影行不存在")
	}
	if got != rev2 {
		t.Fatalf("最终投影 revision = %s，期望 v2 %s（INV-04：旧事件不得覆盖新投影）", got, rev2)
	}
	st := readState(t, d, "fake_links", pageID)
	if st == nil || st.status != "ok" || st.sourceRevisionID != rev2 {
		t.Fatalf("projection_state = %+v，期望 ok 且 source=v2", st)
	}
}

// TestVersionGuardPostCheck 处理后断言：事件处理期间页面又发布了更新的 Revision，
// 事件仍视为过期跳过（done、不重试、不写 projection_state）。
func TestVersionGuardPostCheck(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ensureFakeTable(t, d)

	pageID, rev1 := makePublishedPage(t, d, "guard-post", nil)
	// 预造 v2，但先把 current 拨回 v1（事件处理前 v1 仍是当前版本）。
	rev2 := appendRevision(t, d, pageID, &rev1)
	setCurrentRevision(t, d, pageID, rev1)
	eventID := insertPublishEvent(t, d, pageID, rev1, 0)

	fake := newFakeBuilder(d.Pool, "fake_links")
	// 处理过程中模拟并发发布：current 移动到 v2。
	fake.beforeRebuild = func() { setCurrentRevision(t, d, pageID, rev2) }
	reg := projection.NewRegistry()
	reg.Register(fake)
	cancel, done := runFrameworkConsumer(t, d, reg)
	defer func() { cancel(); <-done }()

	waitFor(t, 5*time.Second, "事件处理完（done）", func() bool {
		return eventStatus(t, d, eventID) == "done"
	})
	if st := readState(t, d, "fake_links", pageID); st != nil {
		t.Fatalf("处理后判定过期不应写 projection_state，实际 %+v", st)
	}
	// 事件 done 后不再重投：HandleEvent 只被调用一次。
	if n := fake.handleCount(); n != 1 {
		t.Fatalf("HandleEvent 调用 %d 次，期望 1（过期事件不重试循环）", n)
	}
}

// TestRebuildPageRestoresProjection RebuildPage 等价重建（INV-03 框架侧）：
// 删掉 fake Builder 写的投影行后，RebuildPage 能从权威 AST 恢复等价结果。
func TestRebuildPageRestoresProjection(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ensureFakeTable(t, d)

	pageID, rev1 := makePublishedPage(t, d, "rebuild-page", nil)

	fake := newFakeBuilder(d.Pool, "fake_links")
	reg := projection.NewRegistry()
	reg.Register(fake)
	rebuilder := projection.NewRebuilder(d.Pool, reg, nil)

	rebuilt, err := rebuilder.RebuildPage(context.Background(), pageID)
	if err != nil || !rebuilt {
		t.Fatalf("首次 RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}
	before, ok := fakeProjectionRevision(t, d, "fake_links", pageID)
	if !ok || before != rev1 {
		t.Fatalf("首次重建投影 = (%s, %v)，期望 (%s, true)", before, ok, rev1)
	}
	st := readState(t, d, "fake_links", pageID)
	if st == nil || st.status != "ok" || st.sourceRevisionID != rev1 || st.lastError != nil {
		t.Fatalf("projection_state = %+v，期望 ok、source=v1、无 last_error", st)
	}

	// 删掉投影行（投影可丢弃），RebuildPage 应恢复等价结果。
	if _, err := d.Pool.Exec(context.Background(),
		`DELETE FROM test_fake_projection WHERE projection_type = 'fake_links' AND page_id = $1`, pageID); err != nil {
		t.Fatalf("删除投影行失败: %v", err)
	}
	rebuilt, err = rebuilder.RebuildPage(context.Background(), pageID)
	if err != nil || !rebuilt {
		t.Fatalf("二次 RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}
	after, ok := fakeProjectionRevision(t, d, "fake_links", pageID)
	if !ok || after != before {
		t.Fatalf("恢复后投影 = (%s, %v)，期望与删除前等价 (%s, true)", after, ok, before)
	}
	if n := fake.rebuildCount(); n != 2 {
		t.Fatalf("Rebuild 调用 %d 次，期望 2", n)
	}
}

// TestRebuildPageUnpublishedSkipped 从未发布的页面：RebuildPage 跳过并返回 (false, nil)。
func TestRebuildPageUnpublishedSkipped(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ensureFakeTable(t, d)

	pageID := d.MakePage(t, testkit.MainNamespaceID, "never-published", "never-published", testkit.SystemActorID)
	reg := projection.NewRegistry()
	reg.Register(newFakeBuilder(d.Pool, "fake_links"))
	rebuilder := projection.NewRebuilder(d.Pool, reg, nil)

	rebuilt, err := rebuilder.RebuildPage(context.Background(), pageID)
	if err != nil || rebuilt {
		t.Fatalf("RebuildPage = (%v, %v)，期望 (false, nil)", rebuilt, err)
	}
	if st := readState(t, d, "fake_links", pageID); st != nil {
		t.Fatalf("未发布页面不应写 projection_state，实际 %+v", st)
	}
}

// TestProjectionStateOKAndError projection_state 记录 ok/error 两态：
// Builder 失败 → error + last_error + source_revision_id；修复后重建 → ok 且 last_error 清空。
func TestProjectionStateOKAndError(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ensureFakeTable(t, d)

	pageID, rev1 := makePublishedPage(t, d, "state-error", nil)
	fake := newFakeBuilder(d.Pool, "fake_links")
	fake.failPages[pageID] = true
	reg := projection.NewRegistry()
	reg.Register(fake)
	rebuilder := projection.NewRebuilder(d.Pool, reg, nil)

	if _, err := rebuilder.RebuildPage(context.Background(), pageID); err == nil {
		t.Fatal("Builder 失败时 RebuildPage 应返回错误")
	}
	st := readState(t, d, "fake_links", pageID)
	if st == nil || st.status != "error" || st.lastError == nil || st.sourceRevisionID != rev1 {
		t.Fatalf("失败后 projection_state = %+v，期望 error + last_error + source=v1", st)
	}
	// 失败事务回滚：投影行不得残留。
	if _, ok := fakeProjectionRevision(t, d, "fake_links", pageID); ok {
		t.Fatal("失败事务应回滚，不应残留投影行")
	}
	// Worker 投影失败不能反向回滚已经提交的权威发布。
	var current uuid.UUID
	var revisions int
	if err := d.Pool.QueryRow(context.Background(), `
		SELECT p.current_revision_id, count(r.id)
		FROM page p JOIN revision r ON r.page_id = p.id
		WHERE p.id = $1 GROUP BY p.current_revision_id`, pageID).Scan(&current, &revisions); err != nil {
		t.Fatalf("读取投影失败后的权威状态失败: %v", err)
	}
	if current != rev1 || revisions != 1 {
		t.Fatalf("投影失败改变了权威发布：current=%s revisions=%d，期望 %s/1", current, revisions, rev1)
	}

	fake.failPages[pageID] = false
	rebuilt, err := rebuilder.RebuildPage(context.Background(), pageID)
	if err != nil || !rebuilt {
		t.Fatalf("修复后 RebuildPage = (%v, %v)，期望 (true, nil)", rebuilt, err)
	}
	st = readState(t, d, "fake_links", pageID)
	if st == nil || st.status != "ok" || st.lastError != nil || st.sourceRevisionID != rev1 {
		t.Fatalf("修复后 projection_state = %+v，期望 ok、last_error 清空、source=v1", st)
	}
}

// TestRebuildAllReport 全量重放：两页已发布、一页从未发布（跳过不报错）、
// 一页 Builder 失败（收集错误继续），报告计数正确。
func TestRebuildAllReport(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ensureFakeTable(t, d)

	p1, _ := makePublishedPage(t, d, "all-ok-1", nil)
	p2, _ := makePublishedPage(t, d, "all-ok-2", nil)
	p3 := d.MakePage(t, testkit.MainNamespaceID, "all-unpublished", "all-unpublished", testkit.SystemActorID)
	p4, _ := makePublishedPage(t, d, "all-fail", nil)

	fake := newFakeBuilder(d.Pool, "fake_links")
	fake.failPages[p4] = true
	reg := projection.NewRegistry()
	reg.Register(fake)
	rebuilder := projection.NewRebuilder(d.Pool, reg, nil)

	report, err := rebuilder.RebuildAll(context.Background())
	if err != nil {
		t.Fatalf("RebuildAll 返回错误: %v", err)
	}
	if report.Total != 4 || report.Rebuilt != 2 || report.Skipped != 1 || report.Failed != 1 {
		t.Fatalf("报告 = %+v，期望 Total=4 Rebuilt=2 Skipped=1 Failed=1", report)
	}
	if len(report.Failures) != 1 || report.Failures[0].PageID != p4 {
		t.Fatalf("Failures = %+v，期望仅 p4", report.Failures)
	}
	for _, p := range []uuid.UUID{p1, p2} {
		if _, ok := fakeProjectionRevision(t, d, "fake_links", p); !ok {
			t.Fatalf("页面 %s 缺少重建后的投影行", p)
		}
		if st := readState(t, d, "fake_links", p); st == nil || st.status != "ok" {
			t.Fatalf("页面 %s projection_state = %+v，期望 ok", p, st)
		}
	}
	if st := readState(t, d, "fake_links", p3); st != nil {
		t.Fatalf("未发布页面 p3 不应有 projection_state，实际 %+v", st)
	}
	if st := readState(t, d, "fake_links", p4); st == nil || st.status != "error" {
		t.Fatalf("失败页面 p4 projection_state = %+v，期望 error", st)
	}
}
