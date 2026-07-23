package importer_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anby/wiki/backend/internal/importer"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func newImportService(t *testing.T) (*importer.Service, *importer.Repository, *testkit.DB) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	repo := importer.NewRepository(tdb.Pool)
	return importer.NewService(repo, db.NewTxManager(tdb.Pool), id.NewGenerator()), repo, tdb
}

func TestImportJob_IdempotentRunStagesCancelAndRetry(t *testing.T) {
	svc, _, tdb := newImportService(t)
	ctx := context.Background()
	job, err := svc.Create(ctx, testkit.SystemActorID, "source_import", "job-1",
		json.RawMessage(`{"url":"https://example.com/a"}`))
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Create(ctx, testkit.SystemActorID, "source_import", "job-1",
		json.RawMessage(`{"url":"https://example.com/a"}`))
	if err != nil || again.ID != job.ID {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	if _, err := svc.Create(ctx, testkit.SystemActorID, "source_import", "job-1",
		json.RawMessage(`{"url":"https://example.com/b"}`)); !errors.Is(err, importer.ErrIdempotencyMismatch) {
		t.Fatalf("mismatch err=%v", err)
	}

	run, err := svc.BeginRun(ctx, job.ID, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	runAgain, err := svc.BeginRun(ctx, job.ID, "run-1")
	if err != nil || runAgain.ID != run.ID {
		t.Fatalf("run again=%+v err=%v", runAgain, err)
	}
	inputHash := importer.HashBytes([]byte("input"))
	stage, err := svc.StartStage(ctx, run.ID, importer.StageFetch, &inputHash)
	if err != nil {
		t.Fatal(err)
	}
	outputHash := importer.HashBytes([]byte("output"))
	if err := svc.CompleteStage(ctx, job.ID, stage, &outputHash); err != nil {
		t.Fatal(err)
	}
	if err := svc.Cancel(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.Detail(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Job.Status != importer.JobCancelled || len(detail.Runs) != 1 ||
		len(detail.Stages) != 1 || detail.Stages[0].Status != importer.StageSucceeded {
		t.Fatalf("detail=%+v", detail)
	}
	if err := svc.Retry(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	retry, err := svc.BeginRun(ctx, job.ID, "run-2")
	if err != nil || retry.Attempt != 2 {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}

	_ = tdb
}

func TestImportJob_StageFailureIsSanitizedAndRetryable(t *testing.T) {
	svc, _, _ := newImportService(t)
	ctx := context.Background()
	job, _ := svc.Create(ctx, testkit.SystemActorID, "source_import", "failure", json.RawMessage(`{}`))
	run, _ := svc.BeginRun(ctx, job.ID, "attempt-1")
	stage, _ := svc.StartStage(ctx, run.ID, importer.StageExtract, nil)
	if err := svc.Fail(ctx, job.ID, run.ID, stage, "model_timeout"); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.Detail(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Job.Status != importer.JobFailed || string(detail.Job.Error) != `{"code": "model_timeout", "stage": "extract"}` {
		// jsonb 输出可能带空格，但绝不能包含输入正文/Prompt。
		var got map[string]string
		if err := json.Unmarshal(detail.Job.Error, &got); err != nil || got["code"] != "model_timeout" || got["stage"] != "extract" {
			t.Fatalf("error=%s", detail.Job.Error)
		}
	}
	if err := svc.Retry(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
}

func TestImportJob_CancelClosesRunningStage(t *testing.T) {
	svc, _, _ := newImportService(t)
	ctx := context.Background()
	job, _ := svc.Create(ctx, testkit.SystemActorID, "source_import", "cancel-running", json.RawMessage(`{}`))
	run, _ := svc.BeginRun(ctx, job.ID, "attempt-1")
	if _, err := svc.StartStage(ctx, run.ID, importer.StageFetch, nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.Cancel(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.Detail(ctx, job.ID)
	if err != nil || len(detail.Stages) != 1 || detail.Stages[0].Status != importer.StageCancelled ||
		detail.Runs[0].Status != importer.JobCancelled {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}

func TestRunner_InvalidQueuedConfigFailsSafely(t *testing.T) {
	svc, _, _ := newImportService(t)
	ctx := context.Background()
	job, err := svc.Create(ctx, testkit.SystemActorID, "source_import", "invalid-runner-config",
		json.RawMessage(`{"source":{"kind":"file","url":"file:///etc/passwd"}}`))
	if err != nil {
		t.Fatal(err)
	}
	runner := importer.NewRunner(svc, nil, importer.RunnerConfig{WikiID: testkit.DefaultWikiID,
		Provider: "fake", Model: "fake"})
	processed, err := runner.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	detail, err := svc.Detail(ctx, job.ID)
	if err != nil || detail.Job.Status != importer.JobFailed || detail.Job.CurrentStage != importer.StageFetch {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	var safe map[string]string
	if err := json.Unmarshal(detail.Job.Error, &safe); err != nil || safe["code"] != "invalid_config" {
		t.Fatalf("error=%s", detail.Job.Error)
	}
}
