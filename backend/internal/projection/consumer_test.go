// Outbox 消费框架集成测试（真实 PostgreSQL，testkit.Open 未配置时 skip）。
// 覆盖：成功/失败退避/死信、崩溃恢复（租约过期重领）、并发 SKIP LOCKED、
// 重启不丢事件、长任务续租。
package projection_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

const testEventType = "page.revision_published"

// testConfig 测试基线配置：轮询快、关闭等待短。
func testConfig() projection.Config {
	return projection.Config{
		BatchSize:       20,
		PollInterval:    10 * time.Millisecond,
		LeaseDuration:   30 * time.Second,
		MaxAttempts:     10,
		BackoffBase:     1 * time.Second,
		ShutdownTimeout: 2 * time.Second,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// runConsumer 在后台运行 Consumer，返回取消函数与等待退出通道。
func runConsumer(c *projection.Consumer) (context.CancelFunc, <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = c.Run(ctx)
	}()
	return cancel, done
}

// insertEvent 直接插入一条 pending 的 outbox_event，返回事件 ID。
func insertEvent(t *testing.T, d *testkit.DB, eventType string) uuid.UUID {
	t.Helper()
	id := d.NewID(t)
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO outbox_event (id, aggregate_type, aggregate_id, event_type, payload_json)
		VALUES ($1, 'page', $2, $3, $4::jsonb)`,
		id, d.NewID(t), eventType,
		`{"page_id":"`+d.NewID(t).String()+`","revision_id":"`+d.NewID(t).String()+`","schema_version":1}`); err != nil {
		t.Fatalf("插入 outbox_event 失败: %v", err)
	}
	return id
}

// eventRow 是 outbox_event 一行的测试视图。
type eventRow struct {
	status        string
	attemptCount  int
	nextAttemptAt time.Time
	claimedAt     *time.Time
	processedAt   *time.Time
	lastError     *string
}

func readEvent(t *testing.T, d *testkit.DB, id uuid.UUID) eventRow {
	t.Helper()
	var r eventRow
	if err := d.Pool.QueryRow(context.Background(), `
		SELECT status, attempt_count, next_attempt_at, claimed_at, processed_at, last_error
		FROM outbox_event WHERE id = $1`, id).
		Scan(&r.status, &r.attemptCount, &r.nextAttemptAt, &r.claimedAt, &r.processedAt, &r.lastError); err != nil {
		t.Fatalf("读取 outbox_event 失败: %v", err)
	}
	return r
}

// waitFor 轮询等待条件成立，超时失败。
func waitFor(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待条件超时（%s）: %s", timeout, msg)
}

// countingHandler 记录每个事件 ID 的处理次数。
type countingHandler struct {
	mu       sync.Mutex
	counts   map[uuid.UUID]int
	err      error
	delay    time.Duration
	onHandle func()
}

func newCountingHandler() *countingHandler {
	return &countingHandler{counts: make(map[uuid.UUID]int)}
}

func (h *countingHandler) Handle(_ context.Context, e projection.Event) error {
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	h.mu.Lock()
	h.counts[e.ID]++
	h.mu.Unlock()
	if h.onHandle != nil {
		h.onHandle()
	}
	return h.err
}

func (h *countingHandler) count(id uuid.UUID) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[id]
}

func (h *countingHandler) total() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.counts {
		n += c
	}
	return n
}

// TestProcessSuccess 领取→处理成功→done，且 Handler 收到幂等键与载荷。
func TestProcessSuccess(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	eventID := insertEvent(t, d, testEventType)

	h := newCountingHandler()
	var got projection.Event
	c := projection.New(d.Pool, testConfig())
	c.Register(testEventType, projection.HandlerFunc(func(_ context.Context, e projection.Event) error {
		got = e
		return h.Handle(context.Background(), e)
	}))
	cancel, done := runConsumer(c)
	defer func() { cancel(); <-done }()

	waitFor(t, 5*time.Second, "事件标记 done", func() bool {
		return readEvent(t, d, eventID).status == "done"
	})
	row := readEvent(t, d, eventID)
	if row.processedAt == nil {
		t.Fatal("done 事件应写入 processed_at")
	}
	if row.attemptCount != 1 {
		t.Fatalf("attempt_count = %d, 期望 1", row.attemptCount)
	}
	if got.IdempotencyKey != eventID.String() {
		t.Fatalf("IdempotencyKey = %q, 期望事件 ID %q", got.IdempotencyKey, eventID)
	}
	if got.EventType != testEventType {
		t.Fatalf("EventType = %q, 期望 %q", got.EventType, testEventType)
	}
	if len(got.Payload) == 0 {
		t.Fatal("Payload 不应为空")
	}
	if h.count(eventID) != 1 {
		t.Fatalf("处理次数 = %d, 期望 1", h.count(eventID))
	}
}

// TestFailureBackoff 处理失败→置回 pending、记录 last_error，且退避随次数指数增长。
func TestFailureBackoff(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	eventID := insertEvent(t, d, testEventType)

	var mu sync.Mutex
	var failTimes []time.Time
	cfg := testConfig()
	cfg.MaxAttempts = 5
	cfg.BackoffBase = 1 * time.Second
	c := projection.New(d.Pool, cfg)
	c.Register(testEventType, projection.HandlerFunc(func(_ context.Context, _ projection.Event) error {
		mu.Lock()
		failTimes = append(failTimes, time.Now())
		mu.Unlock()
		return errors.New("boom")
	}))
	cancel, done := runConsumer(c)
	defer func() { cancel(); <-done }()

	failCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(failTimes)
	}
	failAt := func(i int) time.Time {
		mu.Lock()
		defer mu.Unlock()
		return failTimes[i]
	}

	// 第一次失败：pending、attempt_count=1、last_error 记录、退避约 1s。
	waitFor(t, 5*time.Second, "第一次失败落库", func() bool {
		return failCount() >= 1
	})
	waitFor(t, 5*time.Second, "第一次失败状态写库", func() bool {
		return readEvent(t, d, eventID).status == "pending" && readEvent(t, d, eventID).attemptCount == 1
	})
	row1 := readEvent(t, d, eventID)
	if row1.lastError == nil || *row1.lastError != "boom" {
		t.Fatalf("last_error = %v, 期望 boom", row1.lastError)
	}
	delay1 := row1.nextAttemptAt.Sub(failAt(0))
	if delay1 < 800*time.Millisecond || delay1 > 1300*time.Millisecond {
		t.Fatalf("第一次退避 = %v, 期望约 1s（±10%% jitter）", delay1)
	}

	// 强制到期后第二次失败：退避应指数增长到约 2s。
	if _, err := d.Pool.Exec(context.Background(),
		`UPDATE outbox_event SET next_attempt_at = now() - interval '1 second' WHERE id = $1`, eventID); err != nil {
		t.Fatalf("强制到期失败: %v", err)
	}
	waitFor(t, 5*time.Second, "第二次失败落库", func() bool {
		return failCount() >= 2
	})
	waitFor(t, 5*time.Second, "第二次失败状态写库", func() bool {
		return readEvent(t, d, eventID).attemptCount == 2 && readEvent(t, d, eventID).status == "pending"
	})
	row2 := readEvent(t, d, eventID)
	delay2 := row2.nextAttemptAt.Sub(failAt(1))
	// 第 1 次退避 ∈ [0.9,1.1]s，第 2 次 ∈ [1.8,2.2]s，区间不重叠。
	if delay2 <= delay1 {
		t.Fatalf("退避未指数增长：delay1=%v delay2=%v", delay1, delay2)
	}
}

// TestMaxAttemptsGoesDead 达到 MaxAttempts 后事件进入死信。
func TestMaxAttemptsGoesDead(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	eventID := insertEvent(t, d, testEventType)

	cfg := testConfig()
	cfg.MaxAttempts = 2
	cfg.BackoffBase = 20 * time.Millisecond
	h := newCountingHandler()
	h.err = errors.New("boom")
	c := projection.New(d.Pool, cfg)
	c.Register(testEventType, h)
	cancel, done := runConsumer(c)
	defer func() { cancel(); <-done }()

	waitFor(t, 5*time.Second, "事件进入死信", func() bool {
		return readEvent(t, d, eventID).status == "dead"
	})
	row := readEvent(t, d, eventID)
	if row.attemptCount != 2 {
		t.Fatalf("attempt_count = %d, 期望 2", row.attemptCount)
	}
	if row.lastError == nil || *row.lastError != "boom" {
		t.Fatalf("last_error = %v, 期望 boom", row.lastError)
	}
	if row.processedAt != nil {
		t.Fatal("死信事件不应写入 processed_at")
	}
	if h.count(eventID) != 2 {
		t.Fatalf("处理次数 = %d, 期望 2", h.count(eventID))
	}
}

// TestCrashRecovery 崩溃恢复：claimed 且租约过期的事件被重新领取；
// claimed 但租约未过期的事件不被抢占。
func TestCrashRecovery(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	staleID := insertEvent(t, d, testEventType)
	freshID := insertEvent(t, d, testEventType)

	// stale：模拟 Worker 崩溃留下的 claimed 事件（claimed_at 远超租约）。
	if _, err := d.Pool.Exec(context.Background(), `
		UPDATE outbox_event
		SET status = 'claimed', claimed_at = now() - interval '1 hour', attempt_count = 3
		WHERE id = $1`, staleID); err != nil {
		t.Fatalf("构造过期 claimed 事件失败: %v", err)
	}
	// fresh：其他存活 Worker 刚领取的事件，不得被抢占。
	if _, err := d.Pool.Exec(context.Background(), `
		UPDATE outbox_event
		SET status = 'claimed', claimed_at = now(), attempt_count = 1
		WHERE id = $1`, freshID); err != nil {
		t.Fatalf("构造新鲜 claimed 事件失败: %v", err)
	}

	h := newCountingHandler()
	c := projection.New(d.Pool, testConfig()) // LeaseDuration 30s
	c.Register(testEventType, h)
	cancel, done := runConsumer(c)

	waitFor(t, 5*time.Second, "过期 claimed 事件被重新领取并处理", func() bool {
		return readEvent(t, d, staleID).status == "done"
	})
	cancel()
	<-done

	if h.count(staleID) != 1 {
		t.Fatalf("过期事件处理次数 = %d, 期望 1", h.count(staleID))
	}
	if row := readEvent(t, d, staleID); row.attemptCount != 4 {
		t.Fatalf("过期事件 attempt_count = %d, 期望 4（3+1）", row.attemptCount)
	}
	if h.count(freshID) != 0 {
		t.Fatalf("租约未过期事件被抢占，处理次数 = %d", h.count(freshID))
	}
	if row := readEvent(t, d, freshID); row.status != "claimed" || row.attemptCount != 1 {
		t.Fatalf("租约未过期事件被改动：status=%s attempt_count=%d", row.status, row.attemptCount)
	}
}

// TestConcurrentConsumers 两个 Consumer 并发消费 50 个事件：
// 每个事件恰好被一个 Consumer 处理一次（SKIP LOCKED），-race 下验证。
func TestConcurrentConsumers(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	const n = 50
	ids := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, insertEvent(t, d, testEventType))
	}

	cfg := testConfig()
	cfg.BatchSize = 10
	h1, h2 := newCountingHandler(), newCountingHandler()
	c1 := projection.New(d.Pool, cfg)
	c1.Register(testEventType, h1)
	c2 := projection.New(d.Pool, cfg)
	c2.Register(testEventType, h2)

	cancel1, done1 := runConsumer(c1)
	cancel2, done2 := runConsumer(c2)
	waitFor(t, 20*time.Second, "全部 50 个事件 done", func() bool {
		var doneCount int
		if err := d.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM outbox_event WHERE status = 'done'`).Scan(&doneCount); err != nil {
			t.Fatalf("统计 done 事件失败: %v", err)
		}
		return doneCount == n
	})
	cancel1()
	cancel2()
	<-done1
	<-done2

	for _, id := range ids {
		total := h1.count(id) + h2.count(id)
		if total != 1 {
			t.Fatalf("事件 %s 被处理 %d 次，期望恰好 1 次", id, total)
		}
	}
	if h1.total()+h2.total() != n {
		t.Fatalf("总处理次数 = %d, 期望 %d", h1.total()+h2.total(), n)
	}
}

// TestRestartNoLoss 模拟 Worker 处理一半后停止，新 Consumer 接管后全部处理完，不丢事件。
func TestRestartNoLoss(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	const n = 10
	ids := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, insertEvent(t, d, testEventType))
	}

	cfg := testConfig()
	cfg.BatchSize = 1
	cfg.LeaseDuration = 500 * time.Millisecond // 崩溃遗留 claimed 可较快被接管

	h1 := newCountingHandler()
	c1 := projection.New(d.Pool, cfg)
	c1.Register(testEventType, h1)
	cancel1, done1 := runConsumer(c1)

	// 处理到一半（5 个）后停掉第一个 Consumer（模拟进程退出）。
	waitFor(t, 10*time.Second, "第一批处理 5 个事件", func() bool {
		return h1.total() >= 5
	})
	cancel1()
	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("第一个 Consumer 未在超时内退出")
	}

	// 新 Consumer 接管：剩余 pending 与（可能存在的）过期 claimed 都应被处理完。
	h2 := newCountingHandler()
	c2 := projection.New(d.Pool, cfg)
	c2.Register(testEventType, h2)
	cancel2, done2 := runConsumer(c2)
	defer func() { cancel2(); <-done2 }()

	waitFor(t, 15*time.Second, "全部 10 个事件 done", func() bool {
		var doneCount int
		if err := d.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM outbox_event WHERE status = 'done'`).Scan(&doneCount); err != nil {
			t.Fatalf("统计 done 事件失败: %v", err)
		}
		return doneCount == n
	})

	// 至少一次语义：每个事件至少被处理一次，全部到达 done。
	for _, id := range ids {
		if total := h1.count(id) + h2.count(id); total < 1 {
			t.Fatalf("事件 %s 未被处理（丢失）", id)
		}
	}
}

// TestRenewLease 长任务处理中周期续租：租约极短也不会被其他 Consumer 抢走。
func TestRenewLease(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	eventID := insertEvent(t, d, testEventType)

	cfg := testConfig()
	cfg.LeaseDuration = 300 * time.Millisecond // 极短租约：不续租必被抢

	c1 := projection.New(d.Pool, cfg)
	c2 := projection.New(d.Pool, cfg)

	started := make(chan struct{})
	var once sync.Once
	var renewErrs []error
	var mu sync.Mutex
	var c1Calls int
	c1.Register(testEventType, projection.HandlerFunc(func(ctx context.Context, e projection.Event) error {
		mu.Lock()
		c1Calls++
		mu.Unlock()
		once.Do(func() { close(started) })
		// 模拟约 900ms 的长任务，每 150ms 续租一次（远小于 300ms 租约）。
		for i := 0; i < 6; i++ {
			time.Sleep(150 * time.Millisecond)
			if err := c1.RenewLease(ctx, e.ID); err != nil {
				mu.Lock()
				renewErrs = append(renewErrs, err)
				mu.Unlock()
			}
		}
		return nil
	}))
	h2 := newCountingHandler()
	c2.Register(testEventType, h2)

	cancel1, done1 := runConsumer(c1)
	defer func() { cancel1(); <-done1 }()
	// 等 c1 领取并开始处理后再启动 c2，保证 c2 只能尝试「抢」。
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("c1 未及时开始处理事件")
	}
	cancel2, done2 := runConsumer(c2)
	defer func() { cancel2(); <-done2 }()

	waitFor(t, 10*time.Second, "长任务事件处理完成", func() bool {
		return readEvent(t, d, eventID).status == "done"
	})

	mu.Lock()
	defer mu.Unlock()
	if len(renewErrs) > 0 {
		t.Fatalf("续租失败: %v", renewErrs)
	}
	if c1Calls != 1 {
		t.Fatalf("c1 处理次数 = %d, 期望 1", c1Calls)
	}
	if h2.count(eventID) != 0 {
		t.Fatalf("事件被 c2 抢走（处理 %d 次），续租未生效", h2.count(eventID))
	}
}

// TestProcessedKeys 内存幂等去重器：首次返回 false，重复 key 返回 true。
func TestProcessedKeys(t *testing.T) {
	p := projection.NewProcessedKeys()
	if p.CheckAndMark("k1") {
		t.Fatal("首次 CheckAndMark 应返回 false")
	}
	if !p.CheckAndMark("k1") {
		t.Fatal("重复 key 应返回 true")
	}
	if p.CheckAndMark("k2") {
		t.Fatal("新 key 应返回 false")
	}
}
