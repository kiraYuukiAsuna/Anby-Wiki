package projection

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// M9OperationalMetrics summarizes bounded background work introduced by M9.
// Counts come from the existing authoritative queues and projection tables.
type M9OperationalMetrics struct {
	PendingClaimChanges      int64
	ComponentDependencies    int64
	RuleCollections          int64
	CollectionMemberships    int64
	DueExternalLinks         int64
	OldestExternalLinkDueAge time.Duration
	AppliedEntityMerges      int64
}

// CollectM9OperationalMetrics exposes one stable backlog/capacity snapshot for
// worker logs, acceptance tests, and performance reports.
func CollectM9OperationalMetrics(ctx context.Context, pool *pgxpool.Pool) (M9OperationalMetrics, error) {
	var result M9OperationalMetrics
	var oldestDueSeconds float64
	err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM outbox_event
			 WHERE event_type = 'claim.changed' AND status IN ('pending', 'claimed')),
			(SELECT count(*) FROM component_dependency),
			(SELECT count(*) FROM collection WHERE collection_type = 'rule'),
			(SELECT count(*) FROM collection_membership),
			(SELECT count(*) FROM external_resource WHERE next_check_at <= now()),
			COALESCE((SELECT EXTRACT(EPOCH FROM (now() - min(next_check_at)))
			 FROM external_resource WHERE next_check_at <= now()), 0)::double precision,
			(SELECT count(*) FROM entity_merge WHERE status = 'applied')`).
		Scan(
			&result.PendingClaimChanges,
			&result.ComponentDependencies,
			&result.RuleCollections,
			&result.CollectionMemberships,
			&result.DueExternalLinks,
			&oldestDueSeconds,
			&result.AppliedEntityMerges,
		)
	if err != nil {
		return M9OperationalMetrics{}, fmt.Errorf("projection: collect M9 operational metrics: %w", err)
	}
	if oldestDueSeconds > 0 {
		result.OldestExternalLinkDueAge = time.Duration(oldestDueSeconds * float64(time.Second))
	}
	return result, nil
}
