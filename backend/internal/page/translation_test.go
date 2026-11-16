package page_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/internal/translation"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTranslationLifecycle(t *testing.T) {
	url := os.Getenv("TRANSLATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TRANSLATION_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := "translation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = name + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	runMigration := func(file string) {
		t.Helper()
		raw, err := os.ReadFile("../../migrations/000001_initial_schema." + file + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
	}
	runMigration("up")
	runMigration("down")
	runMigration("up")
	actor := uuid.New()
	wiki, namespace := uuid.New(), uuid.New()
	err = db.NewTxManager(pool).InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO wiki_site(id,site_key,name,default_language,owner_actor_id)VALUES($1,$2,'test','en',$3)`, wiki, wiki.String(), actor); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO actor(id,wiki_id,actor_type,display_name)VALUES($1,$2,'human','owner')`, actor, wiki)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO namespace(id,wiki_id,namespace_key,canonical_name,display_name)VALUES($1,$2,'main','main','main')`, namespace, wiki); err != nil {
		t.Fatal(err)
	}
	service := page.NewService(page.NewRepository(pool), db.NewTxManager(pool), id.NewGenerator())
	source, err := service.CreatePage(ctx, page.CreatePageParams{WikiID: wiki, NamespaceID: namespace, Title: "Original", Language: "en", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.RequestTranslation(ctx, source.ID, actor); !errors.Is(err, page.ErrInvalidTranslation) {
		t.Fatalf("unpublished: %v", err)
	}
	document := json.RawMessage(`{"type":"document","schema_version":1,"children":[]}`)
	revision, err := service.Publish(ctx, page.PublishParams{PageID: source.ID, ActorID: actor, AST: document})
	if err != nil {
		t.Fatal(err)
	}
	if task, err := service.Translation(ctx, source.ID); err != nil || task != nil {
		t.Fatalf("manual publish auto queued: %v %v", task, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.RequestTranslation(ctx, source.ID, actor); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	task, err := service.Translation(ctx, source.ID)
	if err != nil || task == nil {
		t.Fatalf("task: %v", err)
	}
	if task.SourceRevisionID != revision.ID || task.TargetLanguage != "zh-Hans" {
		t.Fatal("wrong baseline")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE event_type=$1`, page.OutboxEventTranslationRequested).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate: %d %v", count, err)
	}
	old := *task
	if err = service.FailTranslation(ctx, source.ID, task.EventID); err != nil {
		t.Fatal(err)
	}
	task, err = service.RequestTranslation(ctx, source.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if task.EventID == old.EventID {
		t.Fatal("retry did not rotate event")
	}
	allow := func(context.Context, pgx.Tx, *page.Page, string) error { return nil }
	if err = service.CompleteTranslation(ctx, old, "Stale", document, allow); err != nil {
		t.Fatal(err)
	}
	forbidden := errors.New("forbidden")
	if err = service.CompleteTranslation(ctx, *task, "Translated", document, func(context.Context, pgx.Tx, *page.Page, string) error { return forbidden }); !errors.Is(err, forbidden) {
		t.Fatal(err)
	}
	if err = service.CompleteTranslation(ctx, *task, "Translated", document, allow); err != nil {
		t.Fatal(err)
	}
	if err = service.CompleteTranslation(ctx, *task, "Duplicate", document, allow); err != nil {
		t.Fatal(err)
	}
	done, err := service.Translation(ctx, source.ID)
	if err != nil || done.TargetPageID == nil || done.Status != "done" {
		t.Fatalf("not done: %+v %v", done, err)
	}
	reverse, err := service.Translation(ctx, *done.TargetPageID)
	if err != nil || reverse.SourcePageID != source.ID {
		t.Fatal("reverse association failed")
	}
	_, snapshot, err := service.CurrentContent(ctx, *done.TargetPageID)
	if err != nil || snapshot == nil {
		t.Fatal("target was not published")
	}
	// Imported publications queue the second language inside the same transaction.
	job, proposal, batch := uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO import_job(id,wiki_id,job_type,initiated_by,idempotency_key)VALUES($1,$2,'test',$3,$1::uuid::text)`, []any{job, wiki, actor}},
		{`INSERT INTO proposal(id,wiki_id,import_job_id,target_type,created_by,idempotency_key)VALUES($1,$2,$3,'wiki',$4,$1::uuid::text)`, []any{proposal, wiki, job, actor}},
		{`INSERT INTO change_batch(id,wiki_id,import_job_id,proposal_id,actor_id)VALUES($1,$2,$3,$4,$5)`, []any{batch, wiki, job, proposal, actor}},
	} {
		if _, err = pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	imported, err := service.CreatePage(ctx, page.CreatePageParams{WikiID: wiki, NamespaceID: namespace, Title: "中文原文", Language: "zh", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Publish(ctx, page.PublishParams{PageID: imported.ID, ActorID: actor, AST: document, ChangeBatchID: &batch}); err != nil {
		t.Fatal(err)
	}
	queued, err := service.Translation(ctx, imported.ID)
	if err != nil || queued == nil || queued.TargetLanguage != "en" {
		t.Fatalf("import not queued: %+v %v", queued, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO actor_role(actor_id,wiki_id,role_id) SELECT $1,$2,id FROM role WHERE role_key='editor'`, actor, wiki); err != nil {
		t.Fatal(err)
	}
	translator := &translation.Service{Pages: service, Auth: governance.NewAuthorizationService(pool), Gateway: echoTranslation{}}
	if err = translator.Run(ctx, imported.ID, queued.EventID); err != nil {
		t.Fatal(err)
	}
	translated, err := service.Translation(ctx, imported.ID)
	if err != nil || translated.TargetPageID == nil {
		t.Fatalf("worker failed: %v", err)
	}
	target, err := service.GetPage(ctx, *translated.TargetPageID)
	if err != nil || target.Language != "en" || !strings.Contains(target.DisplayTitle, " (English)") {
		t.Fatalf("collision fallback: %+v %v", target, err)
	}
	if err = translator.Run(ctx, imported.ID, queued.EventID); err != nil {
		t.Fatal(err)
	}

	t.Run("recreate either deleted language", func(t *testing.T) {
		survivor := imported.ID
		deleted := *translated.TargetPageID
		for attempt := 0; attempt < 2; attempt++ {
			if _, err := pool.Exec(ctx, `UPDATE page SET deleted_at=now() WHERE id=$1`, deleted); err != nil {
				t.Fatal(err)
			}
			visible, err := service.Translation(ctx, survivor)
			if err != nil || visible != nil {
				t.Fatalf("deleted partner remains visible: %+v %v", visible, err)
			}
			var workers sync.WaitGroup
			for i := 0; i < 4; i++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					if _, err := service.RequestTranslation(ctx, survivor, actor); err != nil {
						t.Error(err)
					}
				}()
			}
			workers.Wait()
			task, err := service.Translation(ctx, survivor)
			if err != nil || task == nil || task.Status != "pending" || task.TargetPageID != nil {
				t.Fatalf("not requeued: %+v %v", task, err)
			}
			if task.SourcePageID != survivor {
				t.Fatal("surviving version not used as source")
			}
			if err := translator.Run(ctx, survivor, task.EventID); err != nil {
				t.Fatal(err)
			}
			complete, err := service.Translation(ctx, survivor)
			if err != nil || complete.TargetPageID == nil {
				t.Fatalf("not completed: %+v %v", complete, err)
			}
			deleted = survivor
			survivor = *complete.TargetPageID
		}
	})
	t.Run("rollback invalidates queued and in-flight translations", func(t *testing.T) {
		for _, inFlight := range []bool{false, true} {
			p, err := service.CreatePage(ctx, page.CreatePageParams{WikiID: wiki, NamespaceID: namespace, Title: "Rollback " + uuid.NewString(), Language: "en", ActorID: actor})
			if err != nil {
				t.Fatal(err)
			}
			baseline, err := service.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: actor, AST: document})
			if err != nil {
				t.Fatal(err)
			}
			proposalID, batchID := uuid.New(), uuid.New()
			if _, err = pool.Exec(ctx, `INSERT INTO proposal(id,wiki_id,import_job_id,target_type,target_id,status,created_by,idempotency_key)VALUES($1,$2,$3,'wiki',$2,'applied',$4,$1::uuid::text)`, proposalID, wiki, job, actor); err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `INSERT INTO change_batch(id,wiki_id,import_job_id,proposal_id,actor_id,status)VALUES($1,$2,$3,$4,$5,'applied')`, batchID, wiki, job, proposalID, actor); err != nil {
				t.Fatal(err)
			}
			importedAST := json.RawMessage(`{"type":"document","schema_version":1,"children":[{"id":"00000000-0000-7000-8000-000000000099","type":"paragraph","content":[{"type":"text","text":"REMOVED_BY_ROLLBACK"}]}]}`)
			if _, err = service.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: actor, AST: importedAST, ExpectedRevisionID: &baseline.ID, ChangeBatchID: &batchID}); err != nil {
				t.Fatal(err)
			}
			task, err := service.Translation(ctx, p.ID)
			if err != nil || task == nil {
				t.Fatalf("task: %v", err)
			}
			rollback := func() {
				t.Helper()
				rollbackService := governance.NewRollbackService(governance.NewRepository(pool), service, nil, db.NewTxManager(pool), id.NewGenerator())
				if _, err := rollbackService.Rollback(ctx, batchID, actor); err != nil {
					t.Fatal(err)
				}
			}
			modelCalls := 0
			runner := translation.Service{Pages: service, Auth: governance.NewAuthorizationService(pool), Gateway: translationGeneratorFunc(func(ctx context.Context, r ai.Request) (*ai.Result, error) {
				modelCalls++
				rollback()
				return echoTranslation{}.Generate(ctx, r)
			})}
			if !inFlight {
				rollback()
			}
			if err := runner.Run(ctx, p.ID, task.EventID); err != nil {
				t.Fatal(err)
			}
			if (inFlight && modelCalls != 1) || (!inFlight && modelCalls != 0) {
				t.Fatalf("calls=%d", modelCalls)
			}
			if pending, err := service.Translation(ctx, p.ID); err != nil || pending != nil {
				t.Fatalf("rollback left an active request: %+v %v", pending, err)
			}
			_, restored, err := service.CurrentContent(ctx, p.ID)
			if err != nil || strings.Contains(string(restored.AST), "REMOVED_BY_ROLLBACK") {
				t.Fatalf("rollback content: %v", err)
			}
			// A late successful model response cannot create an orphan page after cancellation.
			lateTitle := "Late " + uuid.NewString()
			if err := service.CompleteTranslation(ctx, *task, lateTitle, importedAST, allow); err != nil {
				t.Fatal(err)
			}
			var exists bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM page WHERE display_title=$1)`, lateTitle).Scan(&exists); err != nil || exists {
				t.Fatalf("late publication: %v %v", exists, err)
			}
			// A deliberate request after rollback translates the restored current revision.
			requested, err := service.RequestTranslation(ctx, p.ID, actor)
			if err != nil {
				t.Fatal(err)
			}
			_, snap, err := service.GetRevision(ctx, p.ID, requested.SourceRevisionID)
			if err != nil || strings.Contains(string(snap.AST), "REMOVED_BY_ROLLBACK") {
				t.Fatalf("retry baseline: %v", err)
			}
		}
	})
	t.Run("translation consumer leaves projections available", func(t *testing.T) {
		consumer := projection.New(pool, projection.Config{RegisteredEventsOnly: true, BatchSize: 1, PollInterval: time.Millisecond})
		consumer.Register(page.OutboxEventTranslationRequested, projection.HandlerFunc(func(context.Context, projection.Event) error { return nil }))
		runCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		defer cancel()
		if err := consumer.Run(runCtx); err != nil {
			t.Fatal(err)
		}
		var handled, untouched int
		if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE event_type=$1 AND status='done'), count(*) FILTER(WHERE event_type<>$1 AND status='pending') FROM outbox_event`, page.OutboxEventTranslationRequested).Scan(&handled, &untouched); err != nil {
			t.Fatal(err)
		}
		if handled == 0 || untouched == 0 {
			t.Fatalf("handled=%d unrelated pending=%d", handled, untouched)
		}
	})
}

type echoTranslation struct{}

func (echoTranslation) Generate(_ context.Context, r ai.Request) (*ai.Result, error) {
	return &ai.Result{JSON: json.RawMessage(`{"texts":` + r.Variables["texts"].(string) + `}`)}, nil
}

type translationGeneratorFunc func(context.Context, ai.Request) (*ai.Result, error)

func (f translationGeneratorFunc) Generate(ctx context.Context, r ai.Request) (*ai.Result, error) {
	return f(ctx, r)
}
