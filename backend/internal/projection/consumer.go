// Package projection 实现 Outbox 消费框架（M3-T01，ADR-0003、设计 §15/§16）。
//
// Worker 通过 SELECT ... FOR UPDATE SKIP LOCKED 从 outbox_event 表领取事件，
// 按 event_type 路由到注册的 Handler 处理：成功标记 done，失败指数退避重试，
// 超过 MaxAttempts 进入死信（status='dead'）。
//
// # 投递语义与幂等契约
//
// 框架保证「至少一次」投递：事件在 Handler 返回成功前可能因进程崩溃、
// 租约过期被重新领取而重复投递。Event.IdempotencyKey 等于事件 ID 的字符串形式，
// 事件本身唯一，Handler 必须以它为去重键实现幂等（可用 ProcessedKeys 或
// 带唯一约束的投影表 UPSERT）。Handler 还需做版本防护：旧 Revision 触发的
// 事件不得覆盖新投影（设计 §15，M3-T02+ 的 Projection Builder 负责落实）。
//
// # 崩溃恢复与租约
//
// 领取在一个独立事务中提交（status='claimed', claimed_at=now()），随后 Handler
// 在该事务外执行。若 Worker 在 Handler 完成前崩溃，事件保持 claimed；
// 当 claimed_at 早于 now()-LeaseDuration 后，事件可被任意 Consumer 重新领取。
// 长任务应在处理过程中周期调用 RenewLease 刷新 claimed_at，防止被他人抢走。
package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// 默认配置（Config 零值字段回落到这些值）。
const (
	defaultBatchSize       = 20
	defaultPollInterval    = 500 * time.Millisecond
	defaultLeaseDuration   = 30 * time.Second
	defaultMaxAttempts     = 10
	defaultBackoffBase     = 1 * time.Second
	defaultShutdownTimeout = 10 * time.Second

	// backoffCap 单次退避上限。
	backoffCap = 5 * time.Minute
	// backoffJitter 退避抖动幅度（±10%），防止多 Worker 同步重试。
	backoffJitter = 0.1
)

// ErrLeaseLost 续租/完成时发现事件已不处于 claimed 状态
// （租约过期被他人领取，或已被其他 Consumer 处理）。
var ErrLeaseLost = errors.New("projection: 事件租约已丢失")

// Event 投递给 Handler 的 Outbox 事件。
// IdempotencyKey 等于事件 ID 的字符串形式（事件本身唯一），Handler 用它做幂等去重。
type Event struct {
	ID             uuid.UUID
	IdempotencyKey string
	AggregateType  string
	AggregateID    uuid.UUID
	EventType      string
	Payload        json.RawMessage
	AttemptCount   int
	CreatedAt      time.Time
}

// Handler 处理一类 Outbox 事件。实现必须幂等（见包文档投递语义）：
// 同一 Event 可能被投递多次。返回 nil 表示处理成功（事件标记 done）；
// 返回 error 触发退避重试，达到 MaxAttempts 后进入死信。
type Handler interface {
	Handle(ctx context.Context, event Event) error
}

// HandlerFunc 将普通函数适配为 Handler。
type HandlerFunc func(ctx context.Context, event Event) error

// Handle 实现 Handler。
func (f HandlerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Config Consumer 配置；零值字段使用默认值。
type Config struct {
	// BatchSize 单次领取的事件数，默认 20。
	BatchSize int
	// PollInterval 无事件时的轮询间隔，默认 500ms。
	PollInterval time.Duration
	// LeaseDuration 领取租约时长，默认 30s。claimed 且 claimed_at 早于
	// now()-LeaseDuration 的事件视为持有者已崩溃，可被重新领取。
	LeaseDuration time.Duration
	// MaxAttempts 最大尝试次数（含首次），达到后事件进入死信，默认 10。
	MaxAttempts int
	// BackoffBase 指数退避基数，默认 1s；第 n 次失败退避 base*2^(n-1)，
	// 封顶 5min，加 ±10% jitter。
	BackoffBase time.Duration
	// ShutdownTimeout 优雅关闭时等待在途事件处理完成的时长，默认 10s；
	// 超时后强制取消 Handler 的 ctx。
	ShutdownTimeout time.Duration
	// Logger 日志；nil 时使用 slog.Default()。
	Logger *slog.Logger
}

func (c *Config) withDefaults() {
	if c.BatchSize <= 0 {
		c.BatchSize = defaultBatchSize
	}
	if c.PollInterval <= 0 {
		c.PollInterval = defaultPollInterval
	}
	if c.LeaseDuration <= 0 {
		c.LeaseDuration = defaultLeaseDuration
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = defaultMaxAttempts
	}
	if c.BackoffBase <= 0 {
		c.BackoffBase = defaultBackoffBase
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = defaultShutdownTimeout
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Consumer Outbox 事件消费者：轮询领取、按 event_type 路由处理、维护退避与死信。
type Consumer struct {
	pool     *pgxpool.Pool
	cfg      Config
	handlers map[string]Handler
}

// New 创建 Consumer。pool 生命周期由调用方管理。
func New(pool *pgxpool.Pool, cfg Config) *Consumer {
	cfg.withDefaults()
	return &Consumer{
		pool:     pool,
		cfg:      cfg,
		handlers: make(map[string]Handler),
	}
}

// Register 注册 eventType 的 Handler。重复注册同一 eventType 或传入 nil 会 panic
// （属编程错误，启动期即应暴露，与 http.ServeMux 风格一致）。
func (c *Consumer) Register(eventType string, h Handler) {
	if h == nil {
		panic("projection: Register 传入 nil Handler")
	}
	if _, dup := c.handlers[eventType]; dup {
		panic(fmt.Sprintf("projection: event_type %q 重复注册", eventType))
	}
	c.handlers[eventType] = h
}

// Run 启动消费循环并阻塞，直到 ctx 取消：停止领取新批次，等待在途事件处理完
// （最长 ShutdownTimeout，超时强制取消 Handler 的 ctx）后返回 nil。
// 领取/落库错误只记日志并继续轮询，不终止循环。
func (c *Consumer) Run(ctx context.Context) error {
	// workCtx 是 Handler 的 ctx：不随 ctx 取消而立即取消，
	// 仅在超过 ShutdownTimeout 后被强制取消，保证在途事件有机会处理完。
	workCtx, stopWork := context.WithCancel(context.Background())
	defer stopWork()
	go func() {
		select {
		case <-ctx.Done():
			timer := time.NewTimer(c.cfg.ShutdownTimeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				stopWork()
			case <-workCtx.Done():
			}
		case <-workCtx.Done():
		}
	}()

	for {
		if ctx.Err() != nil {
			return nil
		}
		events, err := c.claimBatch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.cfg.Logger.Warn("outbox 领取批次失败", slog.Any("error", err))
		} else if len(events) > 0 {
			c.processBatch(workCtx, events)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(c.cfg.PollInterval):
		}
	}
}

// claimBatch 在单事务内领取一批事件：SKIP LOCKED 选中可领取行并立即标记
// claimed（attempt_count+1）后提交。claimed 且租约过期的行可被重新领取（崩溃恢复）。
func (c *Consumer) claimBatch(ctx context.Context) ([]Event, error) {
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("projection: 开启领取事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload_json, attempt_count, created_at
		FROM outbox_event
		WHERE status IN ('pending', 'claimed')
		  AND next_attempt_at <= now()
		  AND (status = 'pending' OR claimed_at < now() - $2::interval)
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`,
		c.cfg.BatchSize, c.cfg.LeaseDuration.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("projection: 领取查询失败: %w", err)
	}
	var events []Event
	var ids []uuid.UUID
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType,
			&e.Payload, &e.AttemptCount, &e.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("projection: 扫描事件失败: %w", err)
		}
		events = append(events, e)
		ids = append(ids, e.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: 领取查询迭代失败: %w", err)
	}
	if len(events) == 0 {
		return nil, tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE outbox_event
		SET status = 'claimed', claimed_at = now(), attempt_count = attempt_count + 1
		WHERE id = ANY($1)`, ids); err != nil {
		return nil, fmt.Errorf("projection: 标记 claimed 失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("projection: 提交领取事务失败: %w", err)
	}
	// 领取已提交，attempt_count 已 +1；同步到返回值供退避计算使用。
	for i := range events {
		events[i].AttemptCount++
		events[i].IdempotencyKey = events[i].ID.String()
	}
	return events, nil
}

// processBatch 顺序处理一批已领取事件；workCtx 取消时放弃剩余事件
// （保持 claimed，租约过期后会被重新领取）。
func (c *Consumer) processBatch(ctx context.Context, events []Event) {
	for _, e := range events {
		if ctx.Err() != nil {
			return
		}
		c.processOne(ctx, e)
	}
}

// processOne 路由并处理单个事件，随后按结果落库（done / 退避重试 / 死信）。
func (c *Consumer) processOne(ctx context.Context, e Event) {
	ctx, span := otel.Tracer("github.com/anby/wiki/backend/projection").Start(ctx, "outbox.process")
	defer span.End()
	span.SetAttributes(attribute.String("outbox.event_type", e.EventType))
	h, ok := c.handlers[e.EventType]
	if !ok {
		err := fmt.Errorf("event_type %q 未注册 Handler", e.EventType)
		span.RecordError(err)
		span.SetStatus(codes.Error, "handler_not_registered")
		c.fail(ctx, e, err)
		return
	}
	if err := h.Handle(ctx, e); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "handler_failed")
		c.fail(ctx, e, err)
		return
	}
	tag, err := c.pool.Exec(ctx, `
		UPDATE outbox_event
		SET status = 'done', processed_at = now(), last_error = NULL
		WHERE id = $1 AND status = 'claimed'`, e.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "mark_done_failed")
		c.cfg.Logger.Warn("outbox 标记 done 失败", slog.String("event_id", e.ID.String()), slog.Any("error", err))
		return
	}
	if tag.RowsAffected() == 0 {
		c.cfg.Logger.Warn("outbox 事件处理成功但租约已丢失，跳过标记 done",
			slog.String("event_id", e.ID.String()), slog.Any("error", ErrLeaseLost))
	}
}

// fail 处理失败：未达 MaxAttempts 置回 pending 并按指数退避安排下次尝试；
// 达到 MaxAttempts 进入死信（status='dead'）。
func (c *Consumer) fail(ctx context.Context, e Event, handleErr error) {
	status := "pending"
	nextAttemptAt := time.Now().Add(backoff(c.cfg.BackoffBase, e.AttemptCount))
	if e.AttemptCount >= c.cfg.MaxAttempts {
		status = "dead"
	}
	tag, err := c.pool.Exec(ctx, `
		UPDATE outbox_event
		SET status = $2, last_error = $3, next_attempt_at = $4, claimed_at = NULL
		WHERE id = $1 AND status = 'claimed'`,
		e.ID, status, handleErr.Error(), nextAttemptAt)
	if err != nil {
		c.cfg.Logger.Warn("outbox 记录失败状态失败", slog.String("event_id", e.ID.String()), slog.Any("error", err))
		return
	}
	if tag.RowsAffected() == 0 {
		c.cfg.Logger.Warn("outbox 事件处理失败但租约已丢失，跳过更新",
			slog.String("event_id", e.ID.String()), slog.Any("error", ErrLeaseLost))
		return
	}
	logFn := c.cfg.Logger.Warn
	if status == "dead" {
		logFn = c.cfg.Logger.Error
	}
	logFn("outbox 事件处理失败",
		slog.String("event_id", e.ID.String()),
		slog.String("event_type", e.EventType),
		slog.Int("attempt_count", e.AttemptCount),
		slog.String("next_status", status),
		slog.Any("error", handleErr),
	)
}

// RenewLease 刷新事件的 claimed_at，延长租约。长任务 Handler 应在处理过程中
// 周期调用（间隔明显小于 LeaseDuration），防止事件因租约过期被他人重新领取。
// 事件已不处于 claimed 状态时返回 ErrLeaseLost。
func (c *Consumer) RenewLease(ctx context.Context, eventID uuid.UUID) error {
	tag, err := c.pool.Exec(ctx, `
		UPDATE outbox_event SET claimed_at = now()
		WHERE id = $1 AND status = 'claimed'`, eventID)
	if err != nil {
		return fmt.Errorf("projection: 续租失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", ErrLeaseLost, eventID)
	}
	return nil
}

// backoff 计算第 attempt 次失败后的退避时长：base * 2^(attempt-1)，
// 封顶 backoffCap，再加 ±10% jitter。
func backoff(base time.Duration, attempt int) time.Duration {
	d := base
	for i := 1; i < attempt && d < backoffCap; i++ {
		d *= 2
	}
	if d > backoffCap {
		d = backoffCap
	}
	jitter := 1 + (rand.Float64()*2-1)*backoffJitter
	return time.Duration(float64(d) * jitter)
}

// ProcessedKeys 基于内存的幂等去重器，供 Handler 以 Event.IdempotencyKey
// 去重（主要用于测试与单进程场景；跨进程幂等应依赖投影表唯一约束/UPSERT）。
// 注意：进程重启后记录丢失，不能替代存储层幂等。
type ProcessedKeys struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

// NewProcessedKeys 创建空的 ProcessedKeys。
func NewProcessedKeys() *ProcessedKeys {
	return &ProcessedKeys{keys: make(map[string]struct{})}
}

// CheckAndMark 若 key 已处理过返回 true（调用方应跳过）；否则记录并返回 false。
func (p *ProcessedKeys) CheckAndMark(key string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, seen := p.keys[key]; seen {
		return true
	}
	p.keys[key] = struct{}{}
	return false
}
