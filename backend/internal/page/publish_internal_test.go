package page

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

// 注入到发布事务提交前的错误（验证回滚原子性）。
var errInjectedBeforeCommit = errors.New("page: 测试注入的提交前失败")

// TestPublish_RollbackOnLateFailure 验证失败原子性：事务内全部写入完成后、
// 提交前注入失败（beforePublishCommit，见 publish.go 注释），整个事务必须回滚——
// 不留孤立 snapshot/revision/audit/outbox，current 指针不移动。
func TestPublish_RollbackOnLateFailure(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	svc := NewService(NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator())
	ctx := context.Background()

	actorID := tdb.MakeActor(t, "human", "alice")
	p, err := svc.CreatePage(ctx, CreatePageParams{
		WikiID:      testkit.DefaultWikiID,
		NamespaceID: testkit.MainNamespaceID,
		Title:       "Rollback Target",
		ActorID:     actorID,
	})
	if err != nil {
		t.Fatalf("CreatePage 失败: %v", err)
	}

	beforePublishCommit = func(context.Context, pgx.Tx) error { return errInjectedBeforeCommit }
	t.Cleanup(func() { beforePublishCommit = nil })

	_, err = svc.Publish(ctx, PublishParams{
		PageID:  p.ID,
		ActorID: actorID,
		AST:     []byte(`{"type":"document","schema_version":1,"children":[]}`),
	})
	if !errors.Is(err, errInjectedBeforeCommit) {
		t.Fatalf("err = %v, want 注入错误", err)
	}

	// 无发布残留（CreatePage 的 page.created 审计/Outbox 行属创建动作，不在断言范围）。
	for _, table := range []string{"content_snapshot", "revision"} {
		var n int
		if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("回滚后 %s 残留 %d 行", table, n)
		}
	}
	for table, eventType := range map[string]string{
		"audit_event":  EventTypeRevisionPublished,
		"outbox_event": OutboxEventRevisionPublished,
	} {
		var n int
		if err := tdb.Pool.QueryRow(ctx,
			"SELECT count(*) FROM "+table+" WHERE event_type = $1", eventType).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("回滚后 %s 残留 %d 行 %s", table, n, eventType)
		}
	}
	var current *string
	if err := tdb.Pool.QueryRow(ctx,
		"SELECT current_revision_id::text FROM page WHERE id = $1", p.ID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != nil {
		t.Fatalf("回滚后 current 指针移动了: %v", *current)
	}
}
