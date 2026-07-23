package page

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

// 本文件是 publish_internal_test.go 的缺口补充（INV-11 / 失败原子性门禁，
// 追踪矩阵 §2）。publish_internal_test.go 已覆盖「首发布」提交前失败的回滚
// 原子性，此处补齐另外两条关键路径：
//  1. 页面已有 current 时的二次发布失败——current 必须仍指向旧版；
//  2. 回滚（runPublishTx 的 useCurrentExpected + 快照去重分支）失败——
//     同样不留孤立 snapshot/revision/audit/outbox，current 不移动。
// 注入点同为包内钩子 beforePublishCommit（publish.go，生产恒为 nil）。

// atomicAST 构造带固定 Block ID 的合法 AST（两版 text 不同，快照必然不同行）。
func atomicAST(text string) []byte {
	return []byte(`{"type":"document","schema_version":1,"children":[` +
		`{"id":"00000000-0000-7000-8000-00000000a001","type":"paragraph",` +
		`"content":[{"type":"text","text":"` + text + `"}]}]}`)
}

// setupAtomicPage 建服务、actor 与空页面。
func setupAtomicPage(t *testing.T) (*testkit.DB, *Service, uuid.UUID, uuid.UUID) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	svc := NewService(NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator())
	actorID := tdb.MakeActor(t, "human", "alice")
	p, err := svc.CreatePage(context.Background(), CreatePageParams{
		WikiID:      testkit.DefaultWikiID,
		NamespaceID: testkit.MainNamespaceID,
		Title:       "Atomic Target",
		ActorID:     actorID,
	})
	if err != nil {
		t.Fatalf("CreatePage 失败: %v", err)
	}
	return tdb, svc, p.ID, actorID
}

// mustPublish 发布并断言成功，返回 Revision ID。
func mustPublish(t *testing.T, svc *Service, pageID, actorID uuid.UUID, expected *uuid.UUID, text string) uuid.UUID {
	t.Helper()
	rev, err := svc.Publish(context.Background(), PublishParams{
		PageID:             pageID,
		ActorID:            actorID,
		ExpectedRevisionID: expected,
		AST:                atomicAST(text),
	})
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}
	return rev.ID
}

// injectLateFailure 在发布事务全部写入后、提交前注入失败，返回复位函数。
func injectLateFailure(t *testing.T) {
	t.Helper()
	beforePublishCommit = func(context.Context, pgx.Tx) error { return errInjectedBeforeCommit }
	t.Cleanup(func() { beforePublishCommit = nil })
}

// countRows 返回表行数。
func countRows(t *testing.T, tdb *testkit.DB, table string) int {
	t.Helper()
	var n int
	if err := tdb.Pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// assertNoResidue 断言四张写入表行数与 current 指针仍停留在基线（失败原子性）。
func assertNoResidue(t *testing.T, tdb *testkit.DB, pageID uuid.UUID, base map[string]int, wantCurrent uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	for _, table := range []string{"content_snapshot", "revision", "audit_event", "outbox_event"} {
		if got := countRows(t, tdb, table); got != base[table] {
			t.Fatalf("%s 行数 = %d, want %d（失败不应残留）", table, got, base[table])
		}
	}
	var current *uuid.UUID
	if err := tdb.Pool.QueryRow(ctx,
		"SELECT current_revision_id FROM page WHERE id = $1", pageID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current == nil || *current != wantCurrent {
		t.Fatalf("current 指针 = %v, want %s（失败不应移动）", current, wantCurrent)
	}
}

// rowCounts 抓取四张写入表行数快照。
func rowCounts(t *testing.T, tdb *testkit.DB) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{"content_snapshot", "revision", "audit_event", "outbox_event"} {
		counts[table] = countRows(t, tdb, table)
	}
	return counts
}

// TestPublish_LateFailureKeepsCurrent 二次发布提交前失败：无孤立 snapshot/revision，
// current 仍指向 v1，audit/outbox 无残留。
func TestPublish_LateFailureKeepsCurrent(t *testing.T) {
	tdb, svc, pageID, actorID := setupAtomicPage(t)
	rev1ID := mustPublish(t, svc, pageID, actorID, nil, "v1")
	base := rowCounts(t, tdb)

	injectLateFailure(t)
	_, err := svc.Publish(context.Background(), PublishParams{
		PageID:             pageID,
		ActorID:            actorID,
		ExpectedRevisionID: &rev1ID,
		AST:                atomicAST("v2 将失败"),
	})
	if !errors.Is(err, errInjectedBeforeCommit) {
		t.Fatalf("err = %v, want 注入错误", err)
	}
	assertNoResidue(t, tdb, pageID, base, rev1ID)
}

// TestRollback_LateFailureAtomicity 回滚路径（快照去重 + useCurrentExpected 分支）
// 提交前失败：同样整体回滚，current 仍指向 v2。
func TestRollback_LateFailureAtomicity(t *testing.T) {
	tdb, svc, pageID, actorID := setupAtomicPage(t)
	rev1ID := mustPublish(t, svc, pageID, actorID, nil, "v1")
	rev2ID := mustPublish(t, svc, pageID, actorID, &rev1ID, "v2")
	base := rowCounts(t, tdb)

	injectLateFailure(t)
	_, err := svc.Rollback(context.Background(), RollbackParams{
		PageID:           pageID,
		TargetRevisionID: rev1ID,
		ActorID:          actorID,
	})
	if !errors.Is(err, errInjectedBeforeCommit) {
		t.Fatalf("err = %v, want 注入错误", err)
	}
	assertNoResidue(t, tdb, pageID, base, rev2ID)
}
