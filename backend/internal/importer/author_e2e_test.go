package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ai"
	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/platform/storage"
)

// Only the model response is controlled. Runner, immutable artifacts, S3,
// composition, trust, review and publication execute their real domain paths.
type authorImportGenerator struct {
	plans        mergingPlanGenerator
	afterExtract func()
}

func (g *authorImportGenerator) Generate(ctx context.Context, request ai.Request) (*ai.Result, error) {
	if request.PromptKey != ExtractionPromptKey {
		result, err := g.plans.Generate(ctx, request)
		if err != nil || request.PromptKey != ImportPlanPromptKey {
			return result, err
		}
		var plan generatedImportPlanDocument
		if err := json.Unmarshal(result.JSON, &plan); err != nil {
			return nil, err
		}
		for index := range plan.Routes {
			plan.Routes[index].Title = request.Variables["preferred_title"].(string)
		}
		result.JSON, err = json.Marshal(plan)
		return result, err
	}
	versionID, err := uuid.Parse(request.Variables["source_version_id"].(string))
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(Candidates{
		SchemaVersion: 1, SourceVersionID: versionID,
		Entities: []EntityCandidate{}, Claims: []ClaimCandidate{}, QualityScore: 0.3,
	})
	if g.afterExtract != nil {
		g.afterExtract()
	}
	return &ai.Result{JSON: raw, PromptKey: request.PromptKey, PromptVersion: 1, Model: "test"}, err
}

func TestWebImportActorE2E(t *testing.T) {
	databaseURL, endpoint := os.Getenv("IMPORT_E2E_DATABASE_URL"), os.Getenv("IMPORT_E2E_S3_ENDPOINT")
	if databaseURL == "" || endpoint == "" {
		t.Skip("set IMPORT_E2E_DATABASE_URL and IMPORT_E2E_S3_ENDPOINT for isolated Web import identity tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schemaName := "import_authors_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	txm, ids := db.NewTxManager(pool), id.NewGenerator()
	auth := authdomain.NewService(pool, txm, ids, time.Hour)
	register := func(name string) authdomain.Session {
		t.Helper()
		session, err := auth.Register(ctx, authdomain.RegisterInput{
			Username: name, Email: name + "@example.invalid", Password: "import-author-test-password", DisplayName: name,
		})
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	owner, other := register("import_owner"), register("import_other")
	pageRepo := page.NewRepository(pool)
	pages := page.NewService(pageRepo, txm, ids)
	store := storage.NewS3Store(storage.S3Config{
		Endpoint: endpoint, Region: "us-east-1", Bucket: os.Getenv("IMPORT_E2E_S3_BUCKET"),
		AccessKey: os.Getenv("IMPORT_E2E_S3_ACCESS_KEY"), SecretKey: os.Getenv("IMPORT_E2E_S3_SECRET_KEY"),
	})
	evidenceRepo := evidence.NewRepository(pool)
	sources := evidence.NewService(evidenceRepo, pageRepo, store, "test", txm, ids)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(pool), pageRepo, txm, ids).WithCitationChecker(evidenceRepo)
	governanceRepo := governance.NewRepository(pool)
	proposals := governance.NewService(governanceRepo, txm, ids)
	authorization := governance.NewAuthorizationService(pool)
	trust := governance.NewAITrustService(governanceRepo, authorization, txm, ids)
	reviews := governance.NewReviewService(governanceRepo, txm, ids, governance.NewRiskEvaluator(knowledgeService)).
		WithAuthorization(authorization).WithAITrust(trust)
	knowledgePatch := governance.NewKnowledgePatchEngine(knowledgeService)
	apply := governance.NewApplyService(governanceRepo, pages, governance.NewPagePatchEngine(), knowledgePatch,
		governance.NewConflictService(governanceRepo, pages, knowledgeService, txm, ids), txm, ids).
		WithAuthorization(authorization)
	components := component.NewService(component.NewRepository(pool), pageRepo, txm, ids, component.NewKnowledgeRegistry(pool))
	repo := NewRepository(pool)
	jobs := NewService(repo, txm, ids)
	generator := &authorImportGenerator{}
	pipeline := NewPipeline(PipelineServices{
		Jobs: jobs, Repository: repo, Evidence: sources, Parser: NewParser(0, nil),
		Extraction: NewExtractionService(repo, evidenceRepo, generator, ids),
		Planner:    NewPagePlanner(repo, pages, generator, ids), Matcher: NewEntityMatcher(knowledgeService),
		Classifier: NewClaimClassifier(knowledgeService),
		Composer:   NewProposalComposer(sources, proposals, knowledgeService, pages, components, ids), Reviews: reviews,
	})
	autoConfirm := true
	runner := NewRunner(jobs, pipeline, RunnerConfig{
		UploadStore: store, Runtime: func(_ context.Context, wikiID uuid.UUID) (RunnerRuntime, error) {
			if wikiID != owner.WikiID {
				t.Error("model runtime left the human initiator's Wiki")
			}
			return RunnerRuntime{Available: true, Model: "test", AutoApprovePlans: autoConfirm}, nil
		},
	})
	raw := func(value any) json.RawMessage {
		t.Helper()
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	newConfig := func(authorID *uuid.UUID, title string) json.RawMessage {
		t.Helper()
		content := []byte("This document contains verified prose about " + title + ".")
		hash := HashBytes(content)
		key := "test/import-authors/" + schemaName + "/" + hash
		if err := store.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "text/plain"); err != nil {
			t.Fatal(err)
		}
		return raw(map[string]any{
			"source": map[string]any{"kind": "upload", "storage_key": key, "filename": title + ".txt",
				"mime_type": "text/plain", "content_hash": hash},
			"author_actor_id": authorID, "title": title, "route_mode": RouteModeForceCreate,
		})
	}
	detail := func(jobID uuid.UUID) *JobDetail {
		t.Helper()
		result, err := jobs.Detail(ctx, jobID)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	process := func(t *testing.T) {
		t.Helper()
		processed, err := runner.ProcessOne(ctx)
		if err != nil || !processed {
			failed, _ := jobs.ListOwned(ctx, owner.ActorID, JobFailed, "", 10)
			if failed != nil && len(failed.Items) > 0 {
				t.Fatalf("runner processed=%v error=%v diagnostic=%s", processed, err, failed.Items[0].Error)
			}
			t.Fatalf("runner processed=%v error=%v", processed, err)
		}
	}
	for index, test := range []struct {
		name, actorType, trust string
		percent                int
	}{
		{"default", authdomain.ActorTypeAI, governance.AITrustUntrusted, 100},
		{"trusted-zero", authdomain.ActorTypeAI, governance.AITrustTrusted, 0},
		{"trusted-full", authdomain.ActorTypeImport, governance.AITrustTrusted, 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			machine, err := auth.CreateMachineActor(ctx, authdomain.CreateMachineActorParams{
				WikiID: owner.WikiID, CreatedBy: owner.ActorID, ActorType: test.actorType,
				DisplayName: "Web " + test.name, IdempotencyKey: test.name,
			})
			if err != nil {
				t.Fatal(err)
			}
			config := newConfig(&machine.ActorID, "Import "+test.name)
			if _, err := jobs.Create(ctx, other.ActorID, other.WikiID, "source_import", uuid.NewString(), config); !errors.Is(err, ErrAuthorUnavailable) {
				t.Fatalf("cross-account binding: %v", err)
			}
			if err := jobs.ValidateAuthor(ctx, owner.ActorID, other.WikiID, &machine.ActorID); !errors.Is(err, ErrAuthorUnavailable) {
				t.Fatalf("cross-Wiki binding: %v", err)
			}
			if err := jobs.ValidateAuthor(ctx, owner.ActorID, owner.WikiID, &owner.ActorID); !errors.Is(err, ErrAuthorUnavailable) {
				t.Fatalf("human masquerading as machine: %v", err)
			}
			key := uuid.NewString()
			job, err := jobs.Create(ctx, owner.ActorID, owner.WikiID, "source_import", key, config)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := jobs.Create(ctx, owner.ActorID, owner.WikiID, "source_import", key, config)
			if err != nil || replayed.ID != job.ID || job.AuthorActorID != machine.ActorID || job.InitiatedBy != owner.ActorID {
				t.Fatalf("job lost author/initiator/idempotency: %v", err)
			}
			decoded, err := decodeSourceImportConfig(config)
			if err != nil {
				t.Fatal(err)
			}
			uploadIdentity := UploadIdentity{
				WikiID: owner.WikiID, Filename: decoded.Source.Filename, MIMEType: decoded.Source.MIMEType,
				ContentHash: decoded.Source.ContentHash, IngestionMode: decoded.IngestionMode,
				Title: decoded.Title, Instructions: decoded.Instructions, RouteMode: decoded.RouteMode,
				PageID: decoded.PageID, AuthorActorID: &machine.ActorID,
			}
			if existing, err := jobs.FindUploadByKey(ctx, owner.ActorID, key, uploadIdentity); err != nil || existing.ID != job.ID {
				t.Fatalf("upload replay lost machine author: %v", err)
			}
			uploadIdentity.WikiID = other.WikiID
			if _, err := jobs.FindUploadByKey(ctx, owner.ActorID, key, uploadIdentity); !errors.Is(err, ErrIdempotencyMismatch) {
				t.Fatalf("upload replay crossed Wiki: %v", err)
			}
			uploadIdentity.WikiID, uploadIdentity.AuthorActorID = owner.WikiID, nil
			if _, err := jobs.FindUploadByKey(ctx, owner.ActorID, key, uploadIdentity); !errors.Is(err, ErrIdempotencyMismatch) {
				t.Fatalf("upload replay changed author: %v", err)
			}
			if _, err := jobs.Create(ctx, owner.ActorID, owner.WikiID, "source_import", key, newConfig(nil, "Changed")); !errors.Is(err, ErrIdempotencyMismatch) {
				t.Fatalf("idempotency changed authorship: %v", err)
			}
			setStatus := func(status string) {
				t.Helper()
				if _, err := auth.SetMachineActorStatus(ctx, authdomain.SetMachineActorStatusParams{
					WikiID: owner.WikiID, ActorID: machine.ActorID, UpdatedBy: owner.ActorID, Status: status,
				}); err != nil {
					t.Fatal(err)
				}
			}
			setStatus(authdomain.ActorStatusDisabled)
			if processed, err := runner.ProcessOne(ctx); err == nil || !processed {
				t.Fatalf("disabled queued job ran: processed=%v error=%v", processed, err)
			}
			failed := detail(job.ID)
			var failure Failure
			if err := json.Unmarshal(failed.Job.Error, &failure); err != nil || failure.Code != ErrorAuthorUnavailable || failed.Job.Status != JobFailed {
				t.Fatalf("author failure was not persisted: %+v %v", failure, err)
			}
			if err := jobs.Retry(ctx, job.ID); !errors.Is(err, ErrAuthorUnavailable) {
				t.Fatalf("disabled retry accepted: %v", err)
			}
			setStatus(authdomain.ActorStatusActive)
			if test.trust != governance.AITrustUntrusted {
				if _, err := trust.Update(ctx, governance.UpdateAITrustProfileParams{
					WikiID: owner.WikiID, ActorID: machine.ActorID, UpdatedBy: owner.ActorID,
					TrustLevel: test.trust, RequiredSamplePercent: test.percent,
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := jobs.Retry(ctx, job.ID); err != nil {
				t.Fatal(err)
			}
			autoConfirm = false
			generator.afterExtract = func() { setStatus(authdomain.ActorStatusDisabled) }
			if _, err := runner.ProcessOne(ctx); err == nil {
				t.Fatal("Actor disabled during extraction continued to compose")
			}
			generator.afterExtract = nil
			interrupted := detail(job.ID)
			if err := json.Unmarshal(interrupted.Job.Error, &failure); err != nil ||
				failure.Code != ErrorAuthorUnavailable || interrupted.Job.SourceVersionID == nil ||
				interrupted.Job.ProposalID != nil {
				t.Fatalf("mid-run disable lost its checkpoint: %+v %v", failure, err)
			}
			setStatus(authdomain.ActorStatusActive)
			if err := jobs.Retry(ctx, job.ID); err != nil {
				t.Fatal(err)
			}
			process(t)
			waiting := detail(job.ID)
			if waiting.Job.Status != JobActionRequired || waiting.Job.CurrentPlanID == nil {
				t.Fatal("manual plan confirmation missing")
			}
			setStatus(authdomain.ActorStatusDisabled)
			if err := jobs.ConfirmPlan(ctx, job.ID, *waiting.Job.CurrentPlanID); !errors.Is(err, ErrAuthorUnavailable) {
				t.Fatalf("disabled confirmation accepted: %v", err)
			}
			setStatus(authdomain.ActorStatusActive)
			replanned, err := jobs.Replan(ctx, owner.ActorID, job.ID, uuid.NewString(), ReplanInput{
				Title: "Import " + test.name, Instructions: "Keep verified prose.", RouteMode: RouteModeForceCreate,
			})
			if err != nil || replanned.AuthorActorID != machine.ActorID {
				t.Fatalf("replan changed author: %v", err)
			}
			process(t)
			waiting = detail(job.ID)
			if err := jobs.ConfirmPlan(ctx, job.ID, *waiting.Job.CurrentPlanID); err != nil {
				t.Fatal(err)
			}
			process(t)
			done := detail(job.ID)
			if done.Job.Status != JobSucceeded || done.Job.ProposalID == nil || len(done.Plans) != 2 {
				t.Fatalf("import failed to resume with immutable plans: %+v", done.Job)
			}
			proposal, err := governanceRepo.GetProposal(ctx, nil, *done.Job.ProposalID)
			if err != nil || proposal.CreatedBy != machine.ActorID || proposal.ImportInitiatedBy == nil ||
				*proposal.ImportInitiatedBy != owner.ActorID || proposal.Status != governance.ProposalInReview {
				t.Fatalf("proposal identity/review lost: %+v %v", proposal, err)
			}
			if _, err := proposals.CreateProposal(ctx, governance.CreateProposalParams{
				WikiID: owner.WikiID, ImportJobID: &job.ID, TargetType: governance.TargetWiki,
				CreatedBy: owner.ActorID, IdempotencyKey: uuid.NewString(),
			}); !errors.Is(err, governance.ErrPermissionDenied) {
				t.Fatalf("human initiator impersonated the bound author: %v", err)
			}
			var decision governance.RiskDecision
			if err := json.Unmarshal(proposal.PolicyDecision, &decision); err != nil ||
				decision.AITrustLevel != test.trust || decision.RequiredSamplePercent != test.percent ||
				decision.SampledForReview != (test.percent == 100) || decision.AutoApprove {
				t.Fatalf("trust/sampling bypassed: %+v %v", decision, err)
			}
			owned, err := proposals.ListOwnedProposals(ctx, owner.WikiID, owner.ActorID, "", "", "", 100)
			if err != nil || len(owned.Items) != index+1 {
				t.Fatalf("human initiator lost proposal visibility: %v", err)
			}
			taskList, err := reviews.Pending(ctx, owner.WikiID, 100)
			if err != nil || len(taskList) != 1 {
				t.Fatalf("human review task missing: %v", err)
			}
			if _, err := reviews.Decide(ctx, owner.WikiID, taskList[0].ID, machine.ActorID, true, ""); err == nil {
				t.Fatal("machine reviewed its final proposal")
			}
			if _, err := apply.Apply(ctx, proposal.ID, machine.ActorID); err == nil {
				t.Fatal("machine published its final proposal")
			}
			setStatus(authdomain.ActorStatusDisabled)
			if _, err := reviews.Submit(ctx, proposal.ID); !errors.Is(err, governance.ErrImportAuthorUnavailable) {
				t.Fatalf("submission ignored the disabled import binding: %v", err)
			} else if failure := newFailure(StageReview, ErrorReviewSubmitFailed, err); failure.Code != ErrorAuthorUnavailable {
				t.Fatal("late submission lost the author-unavailable diagnosis")
			}
			if _, err := reviews.Decide(ctx, owner.WikiID, taskList[0].ID, owner.ActorID, true, "Reviewed source"); err != nil {
				t.Fatal(err)
			}
			if _, err := apply.Apply(ctx, proposal.ID, owner.ActorID); err != nil {
				t.Fatal(err)
			}
		})
	}
	autoConfirm = true
	human, err := jobs.Create(ctx, owner.ActorID, owner.WikiID, "source_import", uuid.NewString(), newConfig(nil, "Human import"))
	if err != nil {
		t.Fatal(err)
	}
	process(t)
	done := detail(human.ID)
	proposal, err := governanceRepo.GetProposal(ctx, nil, *done.Job.ProposalID)
	if err != nil || proposal.CreatedBy != owner.ActorID || done.Job.AuthorActorID != owner.ActorID {
		t.Fatalf("legacy human import changed identity: %v", err)
	}
}
