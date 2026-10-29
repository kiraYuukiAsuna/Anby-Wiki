package importer

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

func TestImportFailureLifecycleE2E(t *testing.T) {
	databaseURL := os.Getenv("IMPORT_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set IMPORT_E2E_DATABASE_URL for failure persistence and historical diagnostics")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schemaName := "import_failures_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaSQL); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schemaSQL+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema, err := os.ReadFile("../../migrations/000001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schema), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	wikiID, actorID := uuid.New(), uuid.New()
	txm := db.NewTxManager(pool)
	if err := txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO wiki_site
			(id,site_key,name,default_language,owner_actor_id) VALUES ($1,$2,'Failure test','zh',$3)`,
			wikiID, wikiID.String(), actorID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO actor (id,wiki_id,actor_type,display_name)
			VALUES ($1,$2,'human','Failure reviewer')`, actorID, wikiID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	jobs := NewService(repo, txm, id.NewGenerator())
	newJob := func() *Job {
		t.Helper()
		job, err := jobs.Create(ctx, actorID, wikiID, "test", uuid.NewString(), nil)
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	begin := func(jobID uuid.UUID, sourceID *uuid.UUID) (*Run, *StageRun) {
		t.Helper()
		run, err := jobs.BeginRun(ctx, jobID, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		var input *string
		if sourceID != nil {
			value := sourceID.String()
			input = &value
		}
		stage, err := jobs.StartStage(ctx, run.ID, StageExtract, input)
		if err != nil {
			t.Fatal(err)
		}
		return run, stage
	}
	decode := func(raw json.RawMessage) Failure {
		t.Helper()
		var failure Failure
		if err := json.Unmarshal(raw, &failure); err != nil {
			t.Fatal(err)
		}
		return failure
	}
	t.Run("atomic failure and late cancellation race", func(t *testing.T) {
		job := newJob()
		run, stage := begin(job.ID, nil)
		failure := newFailure(stage.Stage, ErrorExtractionTimeout, nil)
		if err := jobs.Fail(ctx, job.ID, run.ID, failure); err != nil {
			t.Fatal(err)
		}
		detail, err := jobs.Detail(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.Job.Status != JobFailed || detail.Runs[0].Status != JobFailed || detail.Stages[0].Status != StageFailed ||
			decode(detail.Job.Error).Code != ErrorExtractionTimeout ||
			!jsonEqual(detail.Job.Error, detail.Runs[0].Error) || !jsonEqual(detail.Job.Error, detail.Stages[0].Error) {
			t.Fatal("failure was not persisted consistently")
		}
		if err := jobs.Retry(ctx, job.ID); err != nil {
			t.Fatal(err)
		}
		current, _ := begin(job.ID, nil)
		if err := jobs.Fail(ctx, job.ID, run.ID, failure); err != nil {
			t.Fatal(err)
		}
		detail, err = jobs.Detail(ctx, job.ID)
		if err != nil || detail.Job.Status != JobRunning {
			t.Fatal("old run overwrote the active retry")
		}
		if err := jobs.Cancel(ctx, job.ID); err != nil {
			t.Fatal(err)
		}
		if err := jobs.Fail(ctx, job.ID, current.ID, failure); err != nil {
			t.Fatal(err)
		}
		detail, err = jobs.Detail(ctx, job.ID)
		if err != nil || detail.Job.Status != JobCancelled || decode(detail.Job.Error).Code != ErrorCancelled {
			t.Fatal("late failure overwrote cancellation")
		}
	})
	t.Run("legacy diagnostics belong to their own immutable extraction", func(t *testing.T) {
		job := newJob()
		sourceID := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO source (id,wiki_id,source_type,title)
			VALUES ($1,$2,'book','Private source')`, sourceID, wikiID); err != nil {
			t.Fatal(err)
		}
		for attempt := 1; attempt <= 2; attempt++ {
			versionID := uuid.New()
			if _, err := pool.Exec(ctx, `INSERT INTO source_version
				(id,source_id,version_hash,parse_track,parser_name,parser_version,parser_fingerprint,fetched_at)
				VALUES ($1,$2,$3,'text','test','1',$3,now())`, versionID, sourceID, HashBytes([]byte(versionID.String()))); err != nil {
				t.Fatal(err)
			}
			run, stage := begin(job.ID, &versionID)
			candidates := &Candidates{QualityScore: float64(attempt) / 3, PromptInjectionDetected: true}
			raw, _ := json.Marshal(candidates)
			record := &Extraction{ID: uuid.New(), SourceVersionID: versionID, SchemaVersion: 1,
				PromptKey: ExtractionPromptKey, PromptVersion: 1, Model: "test-model", CandidatesJSON: raw,
				QualityScore: candidates.QualityScore}
			if _, err := repo.InsertExtractionIfAbsent(ctx, record); err != nil {
				t.Fatal(err)
			}
			failure := extractionFailure(&ExtractResult{Extraction: record, Candidates: candidates}, 0.7)
			if err := jobs.Fail(ctx, job.ID, run.ID, failure); err != nil {
				t.Fatal(err)
			}
			legacy := `{"stage":"extract","code":"quality_gate"}`
			if err := txm.InTx(ctx, func(tx pgx.Tx) error {
				for _, query := range []string{
					`UPDATE import_stage_run SET error_json=$2::jsonb WHERE import_run_id=$1`,
					`UPDATE import_run SET error_json=$2::jsonb WHERE id=$1`,
				} {
					if _, err := tx.Exec(ctx, query, run.ID, legacy); err != nil {
						return err
					}
				}
				_, err := tx.Exec(ctx, `UPDATE import_job SET error_json=$2::jsonb WHERE id=$1`, job.ID, legacy)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if stage.InputHash == nil {
				t.Fatal("missing immutable checkpoint")
			}
			if attempt == 1 {
				if err := jobs.Retry(ctx, job.ID); err != nil {
					t.Fatal(err)
				}
			}
		}
		detail, err := jobs.Detail(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		for index, run := range detail.Runs {
			failure := decode(run.Error)
			if failure.Details == nil || *failure.Details.QualityScore != float64(index+1)/3 ||
				!*failure.Details.PromptInjectionDetected || failure.Details.QualityThreshold != nil {
				t.Fatalf("wrong historical diagnostics: %s", run.Error)
			}
		}
		if !jsonEqual(detail.Job.Error, detail.Runs[1].Error) {
			t.Fatal("job did not expose the latest run failure")
		}
		persisted, err := jobs.DetailJob(ctx, job.ID)
		if err != nil || !legacyFailure(persisted.Error) {
			t.Fatal("read enrichment mutated historical records")
		}
	})
	t.Run("pipeline finishes after context cancellation", func(t *testing.T) {
		job := newJob()
		pipeline := NewPipeline(PipelineServices{Jobs: jobs, Repository: repo, Evidence: &evidence.Service{}})
		request := PipelineRequest{JobID: job.ID, RunKey: uuid.NewString(), WikiID: wikiID,
			ActorID: actorID, SourceOnly: true}
		cancelled, cancel := context.WithCancel(ctx)
		_, err := pipeline.run(cancelled, request, func(context.Context) (*AcquiredSource, error) {
			cancel()
			return nil, context.DeadlineExceeded
		})
		if err == nil {
			t.Fatal("pipeline should fail")
		}
		detail, err := jobs.Detail(ctx, job.ID)
		if err != nil || detail.Job.Status != JobFailed || detail.Runs[0].Status != JobFailed ||
			detail.Stages[0].Status != StageFailed {
			t.Fatal("cancelled context left an unrecorded running job")
		}
	})
	t.Run("pipeline records stage startup failures", func(t *testing.T) {
		job := newJob()
		// Reproduce a persistence failure outside the business-stage fail
		// closure through the real repository and database.
		if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_stage_start() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'isolated stage start failure'; END $$;
			CREATE TRIGGER reject_stage_start BEFORE INSERT ON import_stage_run
			FOR EACH ROW EXECUTE FUNCTION reject_stage_start()`, pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := pool.Exec(ctx, `DROP TRIGGER reject_stage_start ON import_stage_run;
				DROP FUNCTION reject_stage_start()`, pgx.QueryExecModeSimpleProtocol); err != nil {
				t.Error(err)
			}
		}()
		pipeline := NewPipeline(PipelineServices{Jobs: jobs, Repository: repo, Evidence: &evidence.Service{}})
		_, err := pipeline.run(ctx, PipelineRequest{JobID: job.ID, RunKey: uuid.NewString(),
			WikiID: wikiID, ActorID: actorID, SourceOnly: true},
			func(context.Context) (*AcquiredSource, error) {
				t.Fatal("acquisition should not execute after stage startup fails")
				return nil, nil
			})
		if err == nil {
			t.Fatal("pipeline should fail")
		}
		detail, err := jobs.Detail(ctx, job.ID)
		if err != nil || detail.Job.Status != JobFailed || detail.Runs[0].Status != JobFailed ||
			decode(detail.Job.Error).Code != ErrorImportFailed || len(detail.Stages) != 0 {
			t.Fatal("stage startup error left a running job")
		}
	})
}
