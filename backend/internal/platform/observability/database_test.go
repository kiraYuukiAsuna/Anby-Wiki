package observability

import (
	"context"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/testkit"
)

func TestCollectDatabaseEmptyState(t *testing.T) {
	database := testkit.Open(t)
	database.Reset(t)
	metrics := NewMetrics("test-worker")

	if err := metrics.CollectDatabase(context.Background(), database.Pool, "test-worker"); err != nil {
		t.Fatalf("空库采集失败: %v", err)
	}
	body := scrape(t, metrics)
	for _, metric := range []string{
		`wiki_outbox_events{service="test-worker",status="pending"} 0`,
		`wiki_projection_states{service="test-worker",state="error"} 0`,
		`wiki_importer_jobs{service="test-worker",status="queued"} 0`,
		`wiki_ai_requests{service="test-worker",status="succeeded"} 0`,
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("缺少零值指标 %q", metric)
		}
	}
}
