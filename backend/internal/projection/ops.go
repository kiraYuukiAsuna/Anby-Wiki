// Projection 运维与一致性工具（M3-T07）。
//
// OperationalMetrics 从现有 outbox_event / projection_state 汇总结构化指标，
// 不引入第二套状态；CheckConsistency 对已发布页面抽样，检查每个已注册 Builder
// 的状态是否存在、成功且来源 Revision 与 Current 一致；ReplayDead 将死信安全地
// 重置为 pending，仍由至少一次消费框架和 Builder 幂等契约承接重放。
package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OperationalMetrics 是一次 Outbox/Projection 健康快照。
type OperationalMetrics struct {
	Pending               int64
	Claimed               int64
	Retrying              int64
	Dead                  int64
	Backlog               int64
	OldestBacklogAge      time.Duration
	ProjectionErrors      int64
	StaleProjectionStates int64
}

// CollectOperationalMetrics 汇总积压、最老积压延迟、重试/死信与投影状态失败。
func CollectOperationalMetrics(ctx context.Context, pool *pgxpool.Pool) (OperationalMetrics, error) {
	var m OperationalMetrics
	var oldestSeconds float64
	err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'pending'),
			count(*) FILTER (WHERE status = 'claimed'),
			count(*) FILTER (WHERE status = 'pending' AND attempt_count > 0),
			count(*) FILTER (WHERE status = 'dead'),
			COALESCE(EXTRACT(EPOCH FROM (
				now() - MIN(created_at) FILTER (WHERE status IN ('pending', 'claimed'))
			)), 0)::double precision
		FROM outbox_event`).Scan(
		&m.Pending, &m.Claimed, &m.Retrying, &m.Dead, &oldestSeconds,
	)
	if err != nil {
		return OperationalMetrics{}, fmt.Errorf("projection: 汇总 outbox 指标失败: %w", err)
	}
	m.Backlog = m.Pending + m.Claimed
	if oldestSeconds > 0 {
		m.OldestBacklogAge = time.Duration(oldestSeconds * float64(time.Second))
	}

	err = pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE ps.status = 'error'),
			count(*) FILTER (WHERE
				p.id IS NULL OR p.deleted_at IS NOT NULL OR
				p.current_revision_id IS DISTINCT FROM ps.source_revision_id)
		FROM projection_state ps
		LEFT JOIN page p
		  ON ps.aggregate_type = 'page' AND p.id = ps.aggregate_id
		WHERE ps.aggregate_type = 'page'`).Scan(
		&m.ProjectionErrors, &m.StaleProjectionStates,
	)
	if err != nil {
		return OperationalMetrics{}, fmt.Errorf("projection: 汇总 projection_state 指标失败: %w", err)
	}
	return m, nil
}

// ConsistencyIssueKind 一致性抽检问题类型。
type ConsistencyIssueKind string

const (
	ConsistencyMissingState ConsistencyIssueKind = "missing_state"
	ConsistencyErrorState   ConsistencyIssueKind = "error_state"
	ConsistencyStaleSource  ConsistencyIssueKind = "stale_source_revision"
)

// ConsistencyIssue 是一个页面/投影类型的不一致状态。
type ConsistencyIssue struct {
	PageID            uuid.UUID
	ProjectionType    string
	Kind              ConsistencyIssueKind
	CurrentRevisionID uuid.UUID
	SourceRevisionID  *uuid.UUID
}

// ConsistencyReport 是一次抽检报告。
type ConsistencyReport struct {
	SampledPages     int
	ExpectedStates   int
	ConsistentStates int
	Issues           []ConsistencyIssue
}

// Healthy 报告是否无不一致。
func (r ConsistencyReport) Healthy() bool { return len(r.Issues) == 0 }

// CheckConsistency 对最多 sampleSize 个活跃已发布页面做控制面一致性抽检。
// 抽样按 Page ID 稳定排序，方便同一数据集重复定位；内容投影等价重建由各 Builder
// 的 INV-03 测试与 Rebuilder 负责，这里专注发现缺状态、失败状态和陈旧来源。
func CheckConsistency(ctx context.Context, pool *pgxpool.Pool, reg *Registry, sampleSize int) (ConsistencyReport, error) {
	if reg == nil {
		return ConsistencyReport{}, fmt.Errorf("projection: 一致性抽检缺少 Builder Registry")
	}
	if sampleSize <= 0 || sampleSize > 10_000 {
		return ConsistencyReport{}, fmt.Errorf("projection: sample_size 必须在 1..10000，实际 %d", sampleSize)
	}

	rows, err := pool.Query(ctx, `
		SELECT id, current_revision_id
		FROM page
		WHERE deleted_at IS NULL AND current_revision_id IS NOT NULL
		ORDER BY id
		LIMIT $1`, sampleSize)
	if err != nil {
		return ConsistencyReport{}, fmt.Errorf("projection: 抽样页面失败: %w", err)
	}
	defer rows.Close()

	type sampledPage struct {
		id      uuid.UUID
		current uuid.UUID
	}
	var pages []sampledPage
	for rows.Next() {
		var p sampledPage
		if err := rows.Scan(&p.id, &p.current); err != nil {
			return ConsistencyReport{}, fmt.Errorf("projection: 扫描抽样页面失败: %w", err)
		}
		pages = append(pages, p)
	}
	if err := rows.Err(); err != nil {
		return ConsistencyReport{}, fmt.Errorf("projection: 迭代抽样页面失败: %w", err)
	}

	builders := reg.Builders()
	report := ConsistencyReport{
		SampledPages:   len(pages),
		ExpectedStates: len(pages) * len(builders),
	}
	for _, p := range pages {
		for _, b := range builders {
			var status string
			var source uuid.UUID
			err := pool.QueryRow(ctx, `
				SELECT status, source_revision_id
				FROM projection_state
				WHERE aggregate_type = 'page' AND aggregate_id = $1 AND projection_type = $2`,
				p.id, b.Type()).Scan(&status, &source)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					report.Issues = append(report.Issues, ConsistencyIssue{
						PageID: p.id, ProjectionType: b.Type(), Kind: ConsistencyMissingState,
						CurrentRevisionID: p.current,
					})
					continue
				}
				return report, fmt.Errorf("projection: 读取抽检状态失败: %w", err)
			}
			sourceCopy := source
			switch {
			case status != StatusOK:
				report.Issues = append(report.Issues, ConsistencyIssue{
					PageID: p.id, ProjectionType: b.Type(), Kind: ConsistencyErrorState,
					CurrentRevisionID: p.current, SourceRevisionID: &sourceCopy,
				})
			case source != p.current:
				report.Issues = append(report.Issues, ConsistencyIssue{
					PageID: p.id, ProjectionType: b.Type(), Kind: ConsistencyStaleSource,
					CurrentRevisionID: p.current, SourceRevisionID: &sourceCopy,
				})
			default:
				report.ConsistentStates++
			}
		}
	}
	return report, nil
}

// ReplayDead 把全部死信重置为可立即领取的 pending；返回重放数量。
func ReplayDead(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE outbox_event
		SET status = 'pending', attempt_count = 0, next_attempt_at = now(),
			claimed_at = NULL, processed_at = NULL, last_error = NULL
		WHERE status = 'dead'`)
	if err != nil {
		return 0, fmt.Errorf("projection: 重放死信失败: %w", err)
	}
	return tag.RowsAffected(), nil
}
