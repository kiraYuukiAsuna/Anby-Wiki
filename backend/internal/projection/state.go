// projection_state 仓储（M3-T02，设计 §15/§16）：每个投影记录来源 Revision 与
// ok/error 状态；投影失败必须记录状态并支持重试与按页重建。
package projection

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/platform/db"
)

// projection_state.status 取值（与迁移 000005 的 CHECK 约束一致）。
const (
	StatusOK    = "ok"
	StatusError = "error"
)

// State 一行 projection_state 的写入视图。
type State struct {
	// AggregateType 聚合类型（页面投影恒为 AggregateTypePage）。
	AggregateType string
	// AggregateID 聚合 ID（页面投影即 page_id）。
	AggregateID uuid.UUID
	// ProjectionType 投影类型（Builder.Type()）。
	ProjectionType string
	// SourceRevisionID 投影来源 Revision（设计 §15：每个投影必须记录来源 Revision）。
	SourceRevisionID uuid.UUID
	// Status StatusOK / StatusError。
	Status string
	// LastError 失败原因；StatusOK 时为 nil（落库为 NULL）。
	LastError *string
}

// OKState 构造成功状态。
func OKState(aggregateType string, aggregateID uuid.UUID, projectionType string, sourceRevisionID uuid.UUID) State {
	return State{
		AggregateType:    aggregateType,
		AggregateID:      aggregateID,
		ProjectionType:   projectionType,
		SourceRevisionID: sourceRevisionID,
		Status:           StatusOK,
	}
}

// ErrorState 构造失败状态（记录 cause 的错误文本到 last_error）。
func ErrorState(aggregateType string, aggregateID uuid.UUID, projectionType string, sourceRevisionID uuid.UUID, cause error) State {
	msg := cause.Error()
	return State{
		AggregateType:    aggregateType,
		AggregateID:      aggregateID,
		ProjectionType:   projectionType,
		SourceRevisionID: sourceRevisionID,
		Status:           StatusError,
		LastError:        &msg,
	}
}

// UpsertState upsert 一行 projection_state（pk 冲突时整行覆盖，projected_at 刷新）。
// q 可为连接池或事务：事件路径在 Builder 事务外簿记传 pool，
// RebuildPage 在重建事务内簿记传 tx（与投影写入同生共死）。
func UpsertState(ctx context.Context, q db.Querier, st State) error {
	if st.Status != StatusOK && st.Status != StatusError {
		return fmt.Errorf("projection: 非法 status %q", st.Status)
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO projection_state (
			aggregate_type, aggregate_id, projection_type,
			source_revision_id, status, projected_at, last_error
		) VALUES ($1, $2, $3, $4, $5, now(), $6)
		ON CONFLICT (aggregate_type, aggregate_id, projection_type) DO UPDATE SET
			source_revision_id = EXCLUDED.source_revision_id,
			status             = EXCLUDED.status,
			projected_at       = now(),
			last_error         = EXCLUDED.last_error`,
		st.AggregateType, st.AggregateID, st.ProjectionType,
		st.SourceRevisionID, st.Status, st.LastError); err != nil {
		return fmt.Errorf("projection: upsert projection_state 失败: %w", err)
	}
	return nil
}
