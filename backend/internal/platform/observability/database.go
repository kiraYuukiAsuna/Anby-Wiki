package observability

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	importJobStatuses   = []string{"queued", "running", "succeeded", "failed", "cancelled"}
	importStageNames    = []string{"fetch", "parse", "extract", "match", "compose", "review"}
	importStageStatuses = []string{"running", "succeeded", "failed", "skipped", "cancelled"}
	aiUsageStatuses     = []string{"succeeded", "failed", "timeout", "invalid_output"}
)

// CollectDatabase refreshes low-cardinality gauges from persisted state. It is
// deliberately read-only and intended for a low-frequency Worker ticker.
func (m *Metrics) CollectDatabase(ctx context.Context, pool *pgxpool.Pool, service string) error {
	if err := m.collectOutboxAndProjection(ctx, pool, service); err != nil {
		return err
	}
	if err := m.collectImporter(ctx, pool, service); err != nil {
		return err
	}
	if err := m.collectAIUsage(ctx, pool, service); err != nil {
		return err
	}
	return nil
}

func (m *Metrics) collectOutboxAndProjection(ctx context.Context, pool *pgxpool.Pool, service string) error {
	var pending, claimed, retrying, dead int64
	var oldestSeconds float64
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'pending'),
			count(*) FILTER (WHERE status = 'claimed'),
			count(*) FILTER (WHERE status = 'pending' AND attempt_count > 0),
			count(*) FILTER (WHERE status = 'dead'),
			COALESCE(EXTRACT(EPOCH FROM (
				now() - MIN(created_at) FILTER (WHERE status IN ('pending', 'claimed'))
			)), 0)::double precision
		FROM outbox_event`).Scan(&pending, &claimed, &retrying, &dead, &oldestSeconds); err != nil {
		return fmt.Errorf("observability: collect outbox metrics: %w", err)
	}
	m.outboxEvents.WithLabelValues(service, "pending").Set(float64(pending))
	m.outboxEvents.WithLabelValues(service, "claimed").Set(float64(claimed))
	m.outboxEvents.WithLabelValues(service, "retrying").Set(float64(retrying))
	m.outboxEvents.WithLabelValues(service, "dead").Set(float64(dead))
	m.outboxEvents.WithLabelValues(service, "backlog").Set(float64(pending + claimed))
	m.outboxOldestAge.Set(oldestSeconds)

	var errors, stale int64
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE ps.status = 'error'),
			count(*) FILTER (WHERE
				p.id IS NULL OR p.deleted_at IS NOT NULL OR
				p.current_revision_id IS DISTINCT FROM ps.source_revision_id)
		FROM projection_state ps
		LEFT JOIN page p
		  ON ps.aggregate_type = 'page' AND p.id = ps.aggregate_id
		WHERE ps.aggregate_type = 'page'`).Scan(&errors, &stale); err != nil {
		return fmt.Errorf("observability: collect projection metrics: %w", err)
	}
	m.projectionStates.WithLabelValues(service, "error").Set(float64(errors))
	m.projectionStates.WithLabelValues(service, "stale").Set(float64(stale))
	return nil
}

func (m *Metrics) collectImporter(ctx context.Context, pool *pgxpool.Pool, service string) error {
	for _, status := range importJobStatuses {
		m.importJobs.WithLabelValues(service, status).Set(0)
		m.importJobDurationSum.WithLabelValues(service, status).Set(0)
		m.importJobDurationCount.WithLabelValues(service, status).Set(0)
	}
	rows, err := pool.Query(ctx, `
		SELECT status, count(*),
			COALESCE(sum(EXTRACT(EPOCH FROM (finished_at - started_at)))
				FILTER (WHERE finished_at IS NOT NULL AND started_at IS NOT NULL), 0)::double precision,
			count(*) FILTER (WHERE finished_at IS NOT NULL AND started_at IS NOT NULL)
		FROM import_job
		GROUP BY status`)
	if err != nil {
		return fmt.Errorf("observability: collect import jobs: %w", err)
	}
	for rows.Next() {
		var status string
		var count, durationCount int64
		var durationSum float64
		if err := rows.Scan(&status, &count, &durationSum, &durationCount); err != nil {
			rows.Close()
			return fmt.Errorf("observability: scan import jobs: %w", err)
		}
		m.importJobs.WithLabelValues(service, status).Set(float64(count))
		m.importJobDurationSum.WithLabelValues(service, status).Set(durationSum)
		m.importJobDurationCount.WithLabelValues(service, status).Set(float64(durationCount))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("observability: iterate import jobs: %w", err)
	}

	for _, stage := range importStageNames {
		for _, status := range importStageStatuses {
			m.importStages.WithLabelValues(service, stage, status).Set(0)
			m.importStageDurationSum.WithLabelValues(service, stage, status).Set(0)
			m.importStageDurationCount.WithLabelValues(service, stage, status).Set(0)
		}
	}
	rows, err = pool.Query(ctx, `
		SELECT stage, status, count(*),
			COALESCE(sum(EXTRACT(EPOCH FROM (finished_at - started_at)))
				FILTER (WHERE finished_at IS NOT NULL), 0)::double precision,
			count(*) FILTER (WHERE finished_at IS NOT NULL)
		FROM import_stage_run
		GROUP BY stage, status`)
	if err != nil {
		return fmt.Errorf("observability: collect import stages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var stage, status string
		var count, durationCount int64
		var durationSum float64
		if err := rows.Scan(&stage, &status, &count, &durationSum, &durationCount); err != nil {
			return fmt.Errorf("observability: scan import stages: %w", err)
		}
		m.importStages.WithLabelValues(service, stage, status).Set(float64(count))
		m.importStageDurationSum.WithLabelValues(service, stage, status).Set(durationSum)
		m.importStageDurationCount.WithLabelValues(service, stage, status).Set(float64(durationCount))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("observability: iterate import stages: %w", err)
	}
	return nil
}

func (m *Metrics) collectAIUsage(ctx context.Context, pool *pgxpool.Pool, service string) error {
	for _, status := range aiUsageStatuses {
		m.aiRequests.WithLabelValues(service, status).Set(0)
		m.aiTokens.WithLabelValues(service, "input", status).Set(0)
		m.aiTokens.WithLabelValues(service, "output", status).Set(0)
		m.aiLatencySum.WithLabelValues(service, status).Set(0)
	}
	rows, err := pool.Query(ctx, `
		SELECT status, count(*), COALESCE(sum(input_tokens), 0),
			COALESCE(sum(output_tokens), 0),
			COALESCE(sum(latency_ms), 0)::double precision / 1000
		FROM ai_request_usage
		GROUP BY status`)
	if err != nil {
		return fmt.Errorf("observability: collect AI usage: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var requests, input, output int64
		var latency float64
		if err := rows.Scan(&status, &requests, &input, &output, &latency); err != nil {
			return fmt.Errorf("observability: scan AI usage: %w", err)
		}
		m.aiRequests.WithLabelValues(service, status).Set(float64(requests))
		m.aiTokens.WithLabelValues(service, "input", status).Set(float64(input))
		m.aiTokens.WithLabelValues(service, "output", status).Set(float64(output))
		m.aiLatencySum.WithLabelValues(service, status).Set(latency)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("observability: iterate AI usage: %w", err)
	}
	return nil
}

// MonitorDatabase refreshes database-backed metrics immediately and at interval.
func (m *Metrics) MonitorDatabase(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, service string, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	collect := func() {
		collectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := m.CollectDatabase(collectCtx, pool, service); err != nil && ctx.Err() == nil {
			logger.Warn("采集数据库指标失败", slog.Any("error", err))
		}
	}
	collect()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}
