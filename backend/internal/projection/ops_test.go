package projection_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

func insertOpsEvent(t *testing.T, d *testkit.DB, status string, attempt int, createdAgo time.Duration) uuid.UUID {
	t.Helper()
	id := d.NewID(t)
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO outbox_event (
			id, aggregate_type, aggregate_id, event_type, payload_json,
			status, attempt_count, next_attempt_at, created_at, claimed_at, last_error)
		VALUES ($1, 'page', $2, 'test.ops', '{}'::jsonb, $3, $4, now(),
			now() - $5::interval,
			CASE WHEN $3 = 'claimed' THEN now() ELSE NULL END,
			CASE WHEN $4 > 0 OR $3 = 'dead' THEN 'injected failure' ELSE NULL END)`,
		id, d.NewID(t), status, attempt, createdAgo.String()); err != nil {
		t.Fatalf("插入运维指标测试事件失败: %v", err)
	}
	return id
}

func TestCollectOperationalMetrics(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)

	pageID, rev1 := makePublishedPage(t, d, "ops-metrics", nil)
	rev2 := appendRevision(t, d, pageID, &rev1)
	insertOpsEvent(t, d, "pending", 0, 2*time.Minute)
	insertOpsEvent(t, d, "pending", 2, time.Minute)
	insertOpsEvent(t, d, "claimed", 1, 30*time.Second)
	insertOpsEvent(t, d, "dead", 10, 3*time.Minute)

	if err := projection.UpsertState(context.Background(), d.Pool,
		projection.ErrorState("page", pageID, "error_projection", rev2, context.DeadlineExceeded)); err != nil {
		t.Fatal(err)
	}
	if err := projection.UpsertState(context.Background(), d.Pool,
		projection.OKState("page", pageID, "stale_projection", rev1)); err != nil {
		t.Fatal(err)
	}

	m, err := projection.CollectOperationalMetrics(context.Background(), d.Pool)
	if err != nil {
		t.Fatalf("CollectOperationalMetrics 返回错误: %v", err)
	}
	if m.Pending != 2 || m.Claimed != 1 || m.Backlog != 3 || m.Retrying != 1 || m.Dead != 1 {
		t.Fatalf("Outbox 指标不符: %+v", m)
	}
	if m.OldestBacklogAge < 90*time.Second {
		t.Fatalf("最老积压延迟 = %v，期望至少 90s", m.OldestBacklogAge)
	}
	if m.ProjectionErrors != 1 || m.StaleProjectionStates != 1 {
		t.Fatalf("Projection 指标不符: %+v", m)
	}
}

func TestCollectM9OperationalMetricsBacklog(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ctx := context.Background()

	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO outbox_event (
			id,aggregate_type,aggregate_id,event_type,payload_json,status,next_attempt_at)
		VALUES ($1,'claim',$2,'claim.changed','{}'::jsonb,'pending',now())`,
		d.NewID(t), d.NewID(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO external_resource (
			id,original_url,normalized_url,domain,path,status,next_check_at)
		VALUES ($1,'https://example.com/m9','https://example.com/m9',
			'example.com','/m9','unknown',now() - interval '2 minutes')`,
		d.NewID(t)); err != nil {
		t.Fatal(err)
	}

	metrics, err := projection.CollectM9OperationalMetrics(ctx, d.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.PendingClaimChanges != 1 || metrics.DueExternalLinks != 1 {
		t.Fatalf("M9 backlog metrics = %+v, want one claim and one link", metrics)
	}
	if metrics.OldestExternalLinkDueAge < time.Minute {
		t.Fatalf("oldest external link due age = %v, want at least one minute",
			metrics.OldestExternalLinkDueAge)
	}
}

func TestCheckConsistencyMissingErrorStaleAndHealthy(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	pageID, rev1 := makePublishedPage(t, d, "ops-consistency", nil)

	reg := projection.NewRegistry()
	reg.Register(newFakeBuilder(d.Pool, "a"))
	reg.Register(newFakeBuilder(d.Pool, "b"))

	report, err := projection.CheckConsistency(context.Background(), d.Pool, reg, 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.SampledPages != 1 || report.ExpectedStates != 2 || len(report.Issues) != 2 {
		t.Fatalf("缺状态报告不符: %+v", report)
	}
	for _, issue := range report.Issues {
		if issue.Kind != projection.ConsistencyMissingState {
			t.Fatalf("issue = %+v，期望 missing_state", issue)
		}
	}

	if err := projection.UpsertState(context.Background(), d.Pool,
		projection.OKState("page", pageID, "a", rev1)); err != nil {
		t.Fatal(err)
	}
	if err := projection.UpsertState(context.Background(), d.Pool,
		projection.ErrorState("page", pageID, "b", rev1, context.Canceled)); err != nil {
		t.Fatal(err)
	}
	rev2 := appendRevision(t, d, pageID, &rev1)
	report, err = projection.CheckConsistency(context.Background(), d.Pool, reg, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Issues) != 2 || report.Issues[0].Kind != projection.ConsistencyStaleSource || report.Issues[1].Kind != projection.ConsistencyErrorState {
		t.Fatalf("陈旧/失败报告不符: %+v", report)
	}

	for _, typ := range []string{"a", "b"} {
		if err := projection.UpsertState(context.Background(), d.Pool,
			projection.OKState("page", pageID, typ, rev2)); err != nil {
			t.Fatal(err)
		}
	}
	report, err = projection.CheckConsistency(context.Background(), d.Pool, reg, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Healthy() || report.ConsistentStates != 2 {
		t.Fatalf("健康报告不符: %+v", report)
	}
}

func TestReplayDead(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	deadID := insertOpsEvent(t, d, "dead", 10, time.Minute)
	_ = insertOpsEvent(t, d, "pending", 0, 0)

	count, err := projection.ReplayDead(context.Background(), d.Pool)
	if err != nil || count != 1 {
		t.Fatalf("ReplayDead = (%d, %v)，期望 (1, nil)", count, err)
	}
	var status string
	var attempt int
	var lastError *string
	if err := d.Pool.QueryRow(context.Background(), `
		SELECT status, attempt_count, last_error FROM outbox_event WHERE id = $1`, deadID).
		Scan(&status, &attempt, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempt != 0 || lastError != nil {
		t.Fatalf("重放后 = status:%s attempt:%d last_error:%v", status, attempt, lastError)
	}
}

// M3-T07 故障注入：新版消息重复投递两次，旧版消息延迟到达；最终投影仍唯一且指向 Current。
func TestFaultInjectionDelayedAndDuplicateMessages(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	ensureFakeTable(t, d)

	pageID, rev1 := makePublishedPage(t, d, "ops-fault-injection", nil)
	rev2 := appendRevision(t, d, pageID, &rev1)
	firstV2 := insertPublishEvent(t, d, pageID, rev2, 3*time.Minute)
	duplicateV2 := insertPublishEvent(t, d, pageID, rev2, 2*time.Minute)
	delayedV1 := insertPublishEvent(t, d, pageID, rev1, time.Minute)

	fake := newFakeBuilder(d.Pool, "fault_projection")
	reg := projection.NewRegistry()
	reg.Register(fake)
	cancel, done := runFrameworkConsumer(t, d, reg)
	defer func() { cancel(); <-done }()

	waitFor(t, 5*time.Second, "重复新版与延迟旧版消息处理完", func() bool {
		return eventStatus(t, d, firstV2) == "done" &&
			eventStatus(t, d, duplicateV2) == "done" &&
			eventStatus(t, d, delayedV1) == "done"
	})
	got, ok := fakeProjectionRevision(t, d, "fault_projection", pageID)
	if !ok || got != rev2 {
		t.Fatalf("最终投影 = (%s, %v)，期望唯一行指向 Current %s", got, ok, rev2)
	}
	if fake.handleCount() != 2 {
		t.Fatalf("HandleEvent 次数 = %d，期望两个 v2 重复消息均幂等处理、v1 被跳过", fake.handleCount())
	}
}
