package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/anby/wiki/backend/internal/doctor"
	"github.com/anby/wiki/backend/testkit"
)

func TestRunRejectsInvalidArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-format", "xml"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("退出码=%d，期望 %d；stderr=%s", code, exitUsage, stderr.String())
	}
}

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), nil, &stdout, &stderr); code != exitRuntime {
		t.Fatalf("退出码=%d，期望 %d；stderr=%s", code, exitRuntime, stderr.String())
	}
}

func TestRunWritesJSONAndUsesIssueExitCode(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过 PostgreSQL 集成测试")
	}
	t.Setenv("DATABASE_URL", databaseURL)
	d := testkit.Open(t)
	d.Reset(t)
	if _, err := d.Pool.Exec(context.Background(), `
		INSERT INTO outbox_event(id,aggregate_type,aggregate_id,event_type,status)
		VALUES($1,'page',$2,'fixture.dead','dead')`,
		d.NewID(t), d.NewID(t)); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-format", "json"}, &stdout, &stderr); code != exitIssues {
		t.Fatalf("退出码=%d，期望 %d；stderr=%s", code, exitIssues, stderr.String())
	}
	var report doctor.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("JSON 报告不可解析: %v\n%s", err, stdout.String())
	}
	if report.Version != "m9-t08-v1" || report.Summary.Error == 0 {
		t.Fatalf("报告字段异常: %+v", report)
	}
}
