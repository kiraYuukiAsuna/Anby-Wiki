// 按页重建与全量重放（M3-T02，设计 §15「自动重建」、§16「支持按 Page 重建」）。
package projection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/platform/db"
)

// rebuildAllBatchSize RebuildAll 分页扫描的批大小。
const rebuildAllBatchSize = 500

// ErrPageNotFound 重建目标页面不存在或已软删除。
var ErrPageNotFound = errors.New("projection: 页面不存在或已删除")

// Rebuilder 投影重建器：从权威数据（page 当前 Revision 的 AST）重建投影，
// 是投影损坏/漂移后的修复入口（投影可丢弃、可重建，设计 §15/§16）。
type Rebuilder struct {
	pool   *pgxpool.Pool
	txm    *db.TxManager
	reg    *Registry
	logger *slog.Logger
}

// NewRebuilder 装配重建器。logger 为 nil 时使用 slog.Default()。
func NewRebuilder(pool *pgxpool.Pool, reg *Registry, logger *slog.Logger) *Rebuilder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Rebuilder{
		pool:   pool,
		txm:    db.NewTxManager(pool),
		reg:    reg,
		logger: logger,
	}
}

// RebuildPage 重建单页全部已注册投影：单事务内读取页面当前 Revision 的 AST，
// 依次调用各 Builder.Rebuild，并在同一事务内把各 Builder 的 projection_state
// 更新为 ok（投影与簿记同生共死）。
//
// 页面从未发布时跳过：记 info 日志，返回 (false, nil)。
// 页面不存在/已软删除返回 ErrPageNotFound。
// 某 Builder 失败时整个事务回滚，该 Builder 的 error 状态在事务外补记
// （best-effort），返回包裹后的错误；返回 rebuilt=false。
func (r *Rebuilder) RebuildPage(ctx context.Context, pageID uuid.UUID) (rebuilt bool, err error) {
	var skipped bool
	var failedBuilder Builder
	var revisionID uuid.UUID

	err = r.txm.InTx(ctx, func(tx pgx.Tx) error {
		var current *uuid.UUID
		var deletedAt any
		qerr := tx.QueryRow(ctx,
			`SELECT current_revision_id, deleted_at FROM page WHERE id = $1 FOR UPDATE`, pageID).
			Scan(&current, &deletedAt)
		if errors.Is(qerr, pgx.ErrNoRows) {
			return fmt.Errorf("%w: id=%s", ErrPageNotFound, pageID)
		}
		if qerr != nil {
			return fmt.Errorf("projection: 锁定页面失败: %w", qerr)
		}
		if deletedAt != nil {
			return fmt.Errorf("%w: id=%s 已软删除", ErrPageNotFound, pageID)
		}
		if current == nil {
			// 从未发布：没有权威 Revision 可投影，跳过（空事务提交，无写入）。
			skipped = true
			return nil
		}
		revisionID = *current

		// 先校验权威 AST 可解析，再让 Builder 逐个重建。
		if _, aerr := RevisionAST(ctx, tx, revisionID); aerr != nil {
			return aerr
		}
		for _, b := range r.reg.Builders() {
			if berr := b.Rebuild(ctx, tx, pageID, revisionID); berr != nil {
				failedBuilder = b
				return fmt.Errorf("projection: Builder %q 重建页面 %s 失败: %w", b.Type(), pageID, berr)
			}
		}
		for _, b := range r.reg.Builders() {
			if serr := UpsertState(ctx, tx, OKState(AggregateTypePage, pageID, b.Type(), revisionID)); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		if failedBuilder != nil {
			upsertBestEffort(ctx, r.pool, r.logger,
				ErrorState(AggregateTypePage, pageID, failedBuilder.Type(), revisionID, err))
		}
		return false, err
	}
	if skipped {
		r.logger.Info("页面从未发布，跳过投影重建", slog.String("page_id", pageID.String()))
		return false, nil
	}
	for _, builder := range r.reg.Builders() {
		postCommit, ok := builder.(PostCommitRebuilder)
		if !ok {
			continue
		}
		if syncErr := postCommit.AfterRebuild(ctx, pageID, revisionID); syncErr != nil {
			upsertBestEffort(ctx, r.pool, r.logger,
				ErrorState(AggregateTypePage, pageID, builder.Type(), revisionID, syncErr))
			return false, fmt.Errorf("projection: Builder %q post-commit sync page %s: %w",
				builder.Type(), pageID, syncErr)
		}
	}
	return true, nil
}

// PageFailure RebuildAll 中单页重建失败的记录。
type PageFailure struct {
	PageID uuid.UUID
	Err    error
}

// RebuildReport RebuildAll 的聚合报告。
type RebuildReport struct {
	// Total 扫描到的活页面总数。
	Total int
	// Rebuilt 成功重建的页面数。
	Rebuilt int
	// Skipped 因从未发布而跳过的页面数。
	Skipped int
	// Failed 重建失败的页面数（= len(Failures)）。
	Failed int
	// Failures 各失败页面的错误明细。
	Failures []PageFailure
}

// FullRebuildPreparer lets projections clear global/stale state before the
// authoritative live-page scan. Per-page builders need not implement it.
type FullRebuildPreparer interface {
	PrepareFullRebuild(context.Context) error
}

// PostCommitRebuilder lets non-transactional projections synchronize only
// after their database staging transaction has committed.
type PostCommitRebuilder interface {
	AfterRebuild(context.Context, uuid.UUID, uuid.UUID) error
}

// RebuildAll 全量重放：分页扫描全部活页面（软删除页除外），逐页 RebuildPage；
// 单页失败不中断，记入聚合报告继续后续页面。返回报告与扫描本身的错误
// （逐页失败只入报告，不作为返回 error；ctx 取消时返回已完成部分的报告与 ctx 错误）。
func (r *Rebuilder) RebuildAll(ctx context.Context) (*RebuildReport, error) {
	report := &RebuildReport{}
	for _, builder := range r.reg.Builders() {
		preparer, ok := builder.(FullRebuildPreparer)
		if !ok {
			continue
		}
		if err := preparer.PrepareFullRebuild(ctx); err != nil {
			return report, fmt.Errorf("projection: prepare full rebuild for %q: %w", builder.Type(), err)
		}
	}
	lastID := uuid.Nil
	for {
		rows, err := r.pool.Query(ctx, `
			SELECT id FROM page
			WHERE deleted_at IS NULL AND id > $1
			ORDER BY id
			LIMIT $2`, lastID, rebuildAllBatchSize)
		if err != nil {
			return report, fmt.Errorf("projection: 扫描页面失败: %w", err)
		}
		var pageIDs []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return report, fmt.Errorf("projection: 扫描页面行失败: %w", err)
			}
			pageIDs = append(pageIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return report, fmt.Errorf("projection: 扫描页面迭代失败: %w", err)
		}
		if len(pageIDs) == 0 {
			return report, nil
		}

		for _, pageID := range pageIDs {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			report.Total++
			rebuilt, err := r.RebuildPage(ctx, pageID)
			switch {
			case err != nil:
				report.Failed++
				report.Failures = append(report.Failures, PageFailure{PageID: pageID, Err: err})
				r.logger.Warn("页面投影重建失败",
					slog.String("page_id", pageID.String()), slog.Any("error", err))
			case rebuilt:
				report.Rebuilt++
			default:
				report.Skipped++
			}
		}
		lastID = pageIDs[len(pageIDs)-1]
		if len(pageIDs) < rebuildAllBatchSize {
			return report, nil
		}
	}
}
