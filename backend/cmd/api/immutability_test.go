// INV-01 / INV-02 门禁（追踪矩阵 §2，实施方案 §9.2 发布阻断项）：
// 直接操作数据库验证 000001 迁移的行级触发器与外键兜底——
// 即使绕过领域服务，已发布 Revision/ContentSnapshot/AuditEvent 的 UPDATE/DELETE
// 也必须被拒，page.current_revision_id 不得指向他页或不存在的 Revision。
//
// 关于 TRUNCATE 绕过：行级 BEFORE 触发器不对 TRUNCATE 生效（拦截需显式
// TRUNCATE 触发器，本库未创建，见 testkit/postgres.go 注释）。TRUNCATE 是
// 表属主级 DDL，不参与应用写入路径，仅 testkit 清库使用——这是已记录、
// 可接受的测试逃生口，不是应用层绕过渠道。除 TRUNCATE 外，应用角色能发出的
// 其余行级写入（UPDATE/DELETE）均被触发器覆盖，见下方用例。
package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/testkit"
)

// setupPublishedPage 经 HTTP 发布一个版本，返回 (testkit, pageID, revID)。
func setupPublishedPage(t *testing.T, title string) (*testkit.DB, string, string) {
	t.Helper()
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageID, revID := createAndPublish(t, router, actorHeader, title, "正文")
	return tdb, pageID, revID
}

// mustRejectExec 执行原生 SQL 并断言被拒绝（触发器/约束），错误信息须含 wantFrag。
func mustRejectExec(t *testing.T, tdb *testkit.DB, sql, wantFrag string) {
	t.Helper()
	if _, err := tdb.Pool.Exec(context.Background(), sql); err == nil {
		t.Fatalf("应被拒绝但执行成功: %s", sql)
	} else if !strings.Contains(err.Error(), wantFrag) {
		t.Fatalf("错误信息不含 %q: %v（SQL: %s）", wantFrag, err, sql)
	}
}

// TestImmutableTablesRejectMutation INV-02：revision/content_snapshot/audit_event
// 的 UPDATE/DELETE 被不可变触发器拒绝，数据保持原样。
func TestImmutableTablesRejectMutation(t *testing.T) {
	tdb, _, revID := setupPublishedPage(t, "Immutable Target")
	ctx := context.Background()

	var snapID, auditID string
	if err := tdb.Pool.QueryRow(ctx,
		"SELECT content_snapshot_id::text FROM revision WHERE id = $1", revID).Scan(&snapID); err != nil {
		t.Fatal(err)
	}
	if err := tdb.Pool.QueryRow(ctx,
		"SELECT id::text FROM audit_event WHERE event_type = 'revision.published' LIMIT 1").Scan(&auditID); err != nil {
		t.Fatalf("发布后应有审计事件: %v", err)
	}

	// revision：UPDATE / DELETE 均被拒。
	mustRejectExec(t, tdb, fmt.Sprintf(
		"UPDATE revision SET summary = '篡改' WHERE id = '%s'", revID), "不允许")
	mustRejectExec(t, tdb, fmt.Sprintf(
		"DELETE FROM revision WHERE id = '%s'", revID), "不允许")

	// content_snapshot：UPDATE / DELETE 均被拒。
	mustRejectExec(t, tdb, fmt.Sprintf(
		"UPDATE content_snapshot SET size_bytes = 0 WHERE id = '%s'", snapID), "不允许")
	mustRejectExec(t, tdb, fmt.Sprintf(
		"DELETE FROM content_snapshot WHERE id = '%s'", snapID), "不允许")

	// audit_event：只增不改，UPDATE / DELETE 均被拒。
	mustRejectExec(t, tdb, fmt.Sprintf(
		"UPDATE audit_event SET event_type = 'forged' WHERE id = '%s'", auditID), "不允许")
	mustRejectExec(t, tdb, fmt.Sprintf(
		"DELETE FROM audit_event WHERE id = '%s'", auditID), "不允许")

	// 反例对照：outbox_event 设计上允许 Worker 更新状态（领取/完成/死信），
	// 不挂不可变触发器——断言 UPDATE 成功，防止误加触发器卡死消费链路。
	if _, err := tdb.Pool.Exec(ctx,
		"UPDATE outbox_event SET status = 'done', processed_at = now() WHERE status = 'pending'"); err != nil {
		t.Fatalf("outbox_event 状态更新应被允许: %v", err)
	}

	// 失败的篡改不改变数据：summary 原样、行数不变。
	var summary string
	if err := tdb.Pool.QueryRow(ctx,
		"SELECT summary FROM revision WHERE id = $1", revID).Scan(&summary); err != nil {
		t.Fatal(err)
	}
	if summary != "init" {
		t.Fatalf("revision 被篡改: summary=%q", summary)
	}
	for _, table := range []string{"revision", "content_snapshot"} {
		var n int
		if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("%s 行数 = %d, want 1（DELETE 应被拒）", table, n)
		}
	}
	// audit_event 期望 2 行：page.created（M3-T04 起 CreatePage 同事务写入）
	// + revision.published，两条 DELETE 都应被拒。
	var auditRows int
	if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_event").Scan(&auditRows); err != nil {
		t.Fatal(err)
	}
	if auditRows != 2 {
		t.Fatalf("audit_event 行数 = %d, want 2（page.created + revision.published，DELETE 应被拒）", auditRows)
	}
}

// TestPageCurrentRevisionConstraint INV-01：current_revision_id 必须属于本页面——
// 指向他页或不存在的 Revision 均被 page_current_revision_check 触发器拒绝
// （BEFORE 触发器先于 page_current_revision_fk 外键执行，外键为兜底）。
func TestPageCurrentRevisionConstraint(t *testing.T) {
	tdb, router, _ := setupWriteAPI(t)
	actor := tdb.MakeActor(t, "human", "alice")
	actorHeader := map[string]string{"X-Actor-ID": actor.String()}
	pageAID, revAID := createAndPublish(t, router, actorHeader, "Page A", "a")
	_, revBID := createAndPublish(t, router, actorHeader, "Page B", "b")

	// 指向他页 Revision：触发器拒绝。
	mustRejectExec(t, tdb, fmt.Sprintf(
		"UPDATE page SET current_revision_id = '%s' WHERE id = '%s'", revBID, pageAID),
		"不属于该页面")

	// 指向不存在的 Revision：BEFORE 触发器先于外键执行，同样被
	// page_current_revision_check 拒绝（外键 page_current_revision_fk 是
	// 触发器被移除时的兜底，正常路径不可达）。
	mustRejectExec(t, tdb, fmt.Sprintf(
		"UPDATE page SET current_revision_id = '%s' WHERE id = '%s'", uuid.New(), pageAID),
		"不属于该页面")

	// current 指针未被移动。
	var current string
	if err := tdb.Pool.QueryRow(context.Background(),
		"SELECT current_revision_id::text FROM page WHERE id = $1", pageAID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != revAID {
		t.Fatalf("current 指针被移动: %s, want %s", current, revAID)
	}
}
