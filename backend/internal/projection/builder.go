// Projection 通用框架（M3-T02，设计 §15/§16）：Builder 契约、Builder 注册表、
// 版本防护装饰器与 page.revision_published 事件分发。
//
// 设计要点（设计 §15）：
//   - 每个投影必须记录来源 Revision（projection_state.source_revision_id，见 state.go）；
//   - 旧 Revision 触发的异步任务不得覆盖新投影——由 WithVersionGuard 统一落实：
//     处理前后断言事件的 revision_id 仍是 page.current_revision_id，否则跳过
//     （记 info 日志，事件视为处理成功进入 done，不重试循环）。
package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/platform/db"
)

// AggregateTypePage 页面聚合类型（与 page 领域写入 outbox_event 的 aggregate_type 一致）。
const AggregateTypePage = "page"

// errStaleEvent 版本防护哨兵：事件携带的 Revision 已非页面当前 Revision（或页面
// 已不存在/未发布）。分发器（NewRevisionPublishedHandler）将其视为处理成功——
// 事件进入 done 且不重试，但不写 projection_state（投影实际由更新的事件负责）。
var errStaleEvent = errors.New("projection: 事件的 Revision 已非页面当前版本")

// Builder 一类投影的构建器（M3-T03+ 注册具体实现：page_links、entity_mention 等）。
// 实现必须幂等：同一 (pageID, revisionID) 的 Rebuild 可重复执行，事件可能重复投递。
type Builder interface {
	// Type 投影类型标识（如 "page_links"），写入 projection_state.projection_type。
	Type() string
	// Rebuild 在 tx 内从该页权威 AST 重建本投影（revisionID 为页面当前 Revision，
	// AST 可用 RevisionAST 在同一 tx 内读取）。失败返回 error，整个重建事务回滚。
	Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error
	// HandleEvent 处理一条 Outbox 事件（通常重建该页本投影）。
	// 通用实现可直接委托 HandleRebuildEvent。
	HandleEvent(ctx context.Context, event Event) error
}

// Registry Builder 注册表：cmd/worker 启动时注册全部 Builder，
// 事件分发与重建命令都经注册表枚举 Builder。
type Registry struct {
	builders []Builder
	byType   map[string]Builder
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{byType: make(map[string]Builder)}
}

// Register 注册一个 Builder。传入 nil 或 Type 重复会 panic
// （属编程错误，启动期即应暴露，与 Consumer.Register 风格一致）。
func (r *Registry) Register(b Builder) {
	if b == nil {
		panic("projection: Register 传入 nil Builder")
	}
	if _, dup := r.byType[b.Type()]; dup {
		panic(fmt.Sprintf("projection: Builder 类型 %q 重复注册", b.Type()))
	}
	r.byType[b.Type()] = b
	r.builders = append(r.builders, b)
}

// Builders 按注册顺序返回全部 Builder 的副本。
func (r *Registry) Builders() []Builder {
	out := make([]Builder, len(r.builders))
	copy(out, r.builders)
	return out
}

// Len 已注册 Builder 数。
func (r *Registry) Len() int {
	return len(r.builders)
}

// revisionPublishedPayload page.revision_published 事件载荷（page 领域发布事务写入）。
type revisionPublishedPayload struct {
	RevisionID string `json:"revision_id"`
}

// eventRevisionID 从事件载荷解析 revision_id。非页面事件或载荷无 revision_id 时
// 返回 ok=false（调用方透传不做版本防护）；载荷含 revision_id 但不是合法 UUID 返回错误。
func eventRevisionID(event Event) (revID uuid.UUID, ok bool, err error) {
	if event.AggregateType != AggregateTypePage {
		return uuid.Nil, false, nil
	}
	var p revisionPublishedPayload
	if uerr := json.Unmarshal(event.Payload, &p); uerr != nil {
		return uuid.Nil, false, fmt.Errorf("projection: 解析事件载荷失败: %w", uerr)
	}
	if p.RevisionID == "" {
		return uuid.Nil, false, nil
	}
	revID, perr := uuid.Parse(p.RevisionID)
	if perr != nil {
		return uuid.Nil, false, fmt.Errorf("projection: 载荷 revision_id 非法 %q: %w", p.RevisionID, perr)
	}
	return revID, true, nil
}

// currentRevision 读取页面当前 Revision ID；页面不存在、已软删除或从未发布返回 ok=false。
func currentRevision(ctx context.Context, q db.Querier, pageID uuid.UUID) (revID uuid.UUID, ok bool, err error) {
	var current *uuid.UUID
	var deletedAt any
	err = q.QueryRow(ctx,
		`SELECT current_revision_id, deleted_at FROM page WHERE id = $1`, pageID).
		Scan(&current, &deletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("projection: 读取页面当前 Revision 失败: %w", err)
	}
	if deletedAt != nil || current == nil {
		return uuid.Nil, false, nil
	}
	return *current, true, nil
}

// RevisionAST 读取指定 Revision 的权威 AST（Builder.Rebuild 内使用，
// q 传 Rebuild 收到的 tx，保证与投影写入同事务）。Revision 不存在返回错误。
func RevisionAST(ctx context.Context, q db.Querier, revisionID uuid.UUID) (*ast.Document, error) {
	var raw []byte
	err := q.QueryRow(ctx, `
		SELECT cs.ast_json
		FROM revision r
		JOIN content_snapshot cs ON cs.id = r.content_snapshot_id
		WHERE r.id = $1`, revisionID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("projection: revision %s 不存在", revisionID)
	}
	if err != nil {
		return nil, fmt.Errorf("projection: 读取 revision %s 的 AST 失败: %w", revisionID, err)
	}
	doc, err := ast.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("projection: 解析 revision %s 的 AST 失败: %w", revisionID, err)
	}
	return doc, nil
}

// HandleRebuildEvent 是 Builder.HandleEvent 的通用实现：单事务内读取页面当前
// Revision 并调用 b.Rebuild 后提交。具体 Builder 可直接以它实现 HandleEvent；
// 页面未发布时返回错误（正常由 WithVersionGuard 先行拦截为 stale-skip）。
func HandleRebuildEvent(ctx context.Context, pool *pgxpool.Pool, b Builder, pageID uuid.UUID) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("projection: 开启重建事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	revID, ok, err := currentRevision(ctx, tx, pageID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("projection: 页面 %s 不存在或从未发布，无法重建", pageID)
	}
	if err := b.Rebuild(ctx, tx, pageID, revID); err != nil {
		return fmt.Errorf("projection: Builder %q 重建页面 %s 失败: %w", b.Type(), pageID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("projection: 提交重建事务失败: %w", err)
	}
	return nil
}

// guardedBuilder WithVersionGuard 的装饰结果。
type guardedBuilder struct {
	inner  Builder
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// WithVersionGuard 版本防护装饰器（设计 §15：旧 Revision 任务不得覆盖新投影）。
// 装饰后的 Builder 处理携带 revision_id 的页面事件（page.revision_published）时：
//   - 处理前：事件的 revision_id 已非 page.current_revision_id（或页面不存在/
//     已删除/未发布）→ 跳过，记 info 日志；
//   - 处理后再次断言：处理期间页面发布了更新的 Revision → 同样视为过期跳过
//     （更新的 Revision 自身的事件会重建投影，本次结果即使已落库也会被覆盖）。
//
// 跳过时不返回 error：配合 NewRevisionPublishedHandler 使用，事件标记 done，
// 旧任务不重试循环。不带 revision_id 的事件直接透传给内层 Builder。
func WithVersionGuard(b Builder, pool *pgxpool.Pool, logger *slog.Logger) Builder {
	if logger == nil {
		logger = slog.Default()
	}
	return &guardedBuilder{inner: b, pool: pool, logger: logger}
}

// Type 实现 Builder（透传内层）。
func (g *guardedBuilder) Type() string { return g.inner.Type() }

// Rebuild 实现 Builder（透传内层；版本防护只作用于事件路径，
// 显式重建总是以调用时读到的当前 Revision 为准）。
func (g *guardedBuilder) Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	return g.inner.Rebuild(ctx, tx, pageID, revisionID)
}

// HandleEvent 实现 Builder：处理前后做版本防护断言，过期返回 errStaleEvent。
func (g *guardedBuilder) HandleEvent(ctx context.Context, event Event) error {
	revID, ok, err := eventRevisionID(event)
	if err != nil {
		return err
	}
	if !ok {
		return g.inner.HandleEvent(ctx, event)
	}

	stale, err := g.isStale(ctx, event, revID, "处理前")
	if err != nil {
		return err
	}
	if stale {
		return errStaleEvent
	}
	if err := g.inner.HandleEvent(ctx, event); err != nil {
		return err
	}
	stale, err = g.isStale(ctx, event, revID, "处理后")
	if err != nil {
		return err
	}
	if stale {
		return errStaleEvent
	}
	return nil
}

// isStale 判断事件携带的 revID 是否已非页面当前 Revision；过期时记 info 日志。
func (g *guardedBuilder) isStale(ctx context.Context, event Event, revID uuid.UUID, phase string) (bool, error) {
	current, ok, err := currentRevision(ctx, g.pool, event.AggregateID)
	if err != nil {
		return false, err
	}
	if ok && current == revID {
		return false, nil
	}
	g.logger.Info("版本防护：跳过过期 Revision 的事件",
		slog.String("phase", phase),
		slog.String("builder", g.inner.Type()),
		slog.String("event_id", event.ID.String()),
		slog.String("page_id", event.AggregateID.String()),
		slog.String("event_revision_id", revID.String()),
		slog.String("current_revision_id", currentRevisionString(current, ok)),
	)
	return true, nil
}

func currentRevisionString(current uuid.UUID, ok bool) string {
	if !ok {
		return "(无已发布 Revision)"
	}
	return current.String()
}

// NewRevisionPublishedHandler 构造 Consumer 的 Handler：将事件分发给注册表中
// 全部 Builder（每个经 WithVersionGuard 包装），并按处理结果写 projection_state
// （成功 ok / 失败 error + last_error + source_revision_id）。
//
// 任一 Builder 失败即返回 error（事件进入退避重试；已成功的 Builder 依赖幂等
// 承受重放）。被版本防护跳过的 Builder 不写 projection_state。
// projection_state 落库失败只记 warn，不影响事件处理结果（簿记不阻塞主流程）。
func NewRevisionPublishedHandler(pool *pgxpool.Pool, reg *Registry, logger *slog.Logger) Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return HandlerFunc(func(ctx context.Context, event Event) error {
		revID, hasRev, err := eventRevisionID(event)
		if err != nil {
			return err
		}
		for _, b := range reg.Builders() {
			handleErr := WithVersionGuard(b, pool, logger).HandleEvent(ctx, event)
			switch {
			case errors.Is(handleErr, errStaleEvent):
				continue
			case handleErr != nil:
				if hasRev {
					upsertBestEffort(ctx, pool, logger,
						ErrorState(AggregateTypePage, event.AggregateID, b.Type(), revID, handleErr))
				}
				return handleErr
			default:
				if hasRev {
					upsertBestEffort(ctx, pool, logger,
						OKState(AggregateTypePage, event.AggregateID, b.Type(), revID))
				}
			}
		}
		return nil
	})
}

// upsertBestEffort 写 projection_state，失败只记 warn（簿记不阻塞主流程）。
func upsertBestEffort(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, st State) {
	if err := UpsertState(ctx, pool, st); err != nil {
		logger.Warn("projection_state 落库失败",
			slog.String("projection_type", st.ProjectionType),
			slog.String("aggregate_id", st.AggregateID.String()),
			slog.Any("error", err),
		)
	}
}
