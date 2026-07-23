package importer_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/importer"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/platform/storage"
	"github.com/anby/wiki/backend/testkit"
)

type pipelineGenerator struct {
	calls     int
	quality   float64
	injection bool
}

func (g *pipelineGenerator) Generate(_ context.Context, request ai.Request) (*ai.Result, error) {
	g.calls++
	sourceVersionID := uuid.MustParse(request.Variables["source_version_id"].(string))
	var chunks []struct {
		ID   uuid.UUID `json:"chunk_id"`
		Text string    `json:"text"`
	}
	_ = json.Unmarshal([]byte(request.Variables["chunks_json"].(string)), &chunks)
	chunk := chunks[len(chunks)-1]
	labelStart := strings.Index(chunk.Text, "Anby Wiki")
	dateStart := strings.Index(chunk.Text, "2026-07-22")
	entityCandidateID, claimCandidateID := uuid.New(), uuid.New()
	quality := g.quality
	if quality == 0 {
		quality = .96
	}
	output, _ := json.Marshal(map[string]any{
		"schema_version": 1, "source_version_id": sourceVersionID,
		"entities": []any{map[string]any{"candidate_id": entityCandidateID, "type_key": "software",
			"label": "Anby Wiki", "aliases": []any{}, "confidence": .99,
			"evidence": []any{map[string]any{"chunk_id": chunk.ID, "quotation": "Anby Wiki",
				"char_start": labelStart, "char_end": labelStart + len([]rune("Anby Wiki"))}}}},
		"claims": []any{map[string]any{"candidate_id": claimCandidateID,
			"subject": map[string]any{"candidate_id": entityCandidateID}, "property_key": "release_date",
			"value": map[string]any{"date": "2026-07-22"}, "confidence": .98,
			"evidence": []any{map[string]any{"chunk_id": chunk.ID, "quotation": "2026-07-22",
				"char_start": dateStart, "char_end": dateStart + len([]rune("2026-07-22"))}}}},
		"quality_score": quality, "prompt_injection_detected": g.injection,
	})
	return &ai.Result{JSON: output, PromptKey: "source-extraction-v1", PromptVersion: 1,
		Provider: "fake", Model: "golden-model"}, nil
}

type pipelineFixture struct {
	tdb        *testkit.DB
	actorID    uuid.UUID
	sourceID   uuid.UUID
	entityID   uuid.UUID
	jobs       *importer.Service
	pipeline   *importer.Pipeline
	governance *governance.Service
	reviews    *governance.ReviewService
	apply      *governance.ApplyService
	knowledge  *knowledge.Service
	generator  *pipelineGenerator
}

func newPipelineFixture(t *testing.T) *pipelineFixture {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "pipeline reviewer")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pageService := page.NewService(pageRepo, txm, ids)
	evidenceRepo := evidence.NewRepository(tdb.Pool)
	evidenceService := evidence.NewService(evidenceRepo, pageRepo, storage.NewFake(), "test", txm, ids)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids).WithCitationChecker(evidenceRepo)
	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "anby-wiki",
		Labels: []knowledge.LabelInput{{Language: "en", Label: "Anby Wiki", IsPrimary: true}}, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := evidenceService.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage, Title: "Golden release", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	importRepo := importer.NewRepository(tdb.Pool)
	jobs := importer.NewService(importRepo, txm, ids)
	governanceRepo := governance.NewRepository(tdb.Pool)
	governanceService := governance.NewService(governanceRepo, txm, ids)
	reviews := governance.NewReviewService(governanceRepo, txm, ids, governance.NewRiskEvaluator(knowledgeService))
	generator := &pipelineGenerator{}
	extraction := importer.NewExtractionService(importRepo, evidenceRepo, generator, ids)
	composer := importer.NewProposalComposer(evidenceService, governanceService, knowledgeService)
	pipeline := importer.NewPipeline(importer.PipelineServices{Jobs: jobs, Repository: importRepo,
		Evidence: evidenceService, Parser: importer.NewParser(1200), Extraction: extraction,
		Matcher: importer.NewEntityMatcher(knowledgeService), Classifier: importer.NewClaimClassifier(knowledgeService),
		Composer: composer, Reviews: reviews})
	conflicts := governance.NewConflictService(governanceRepo, pageService, knowledgeService, txm, ids)
	apply := governance.NewApplyService(governanceRepo, pageService, governance.NewPagePatchEngine(),
		governance.NewKnowledgePatchEngine(knowledgeService), conflicts, txm, ids)
	return &pipelineFixture{tdb: tdb, actorID: actorID, sourceID: source.ID, entityID: entity.ID,
		jobs: jobs, pipeline: pipeline, governance: governanceService, reviews: reviews,
		apply: apply, knowledge: knowledgeService, generator: generator}
}

func TestPipeline_GoldenImportReviewApplyAndRepeatedVersion(t *testing.T) {
	fixture := newPipelineFixture(t)
	ctx := context.Background()
	content, err := os.ReadFile("testdata/golden/release.html")
	if err != nil {
		t.Fatal(err)
	}
	job, err := fixture.jobs.Create(ctx, fixture.actorID, "source_import", "golden-1", json.RawMessage(`{"fixture":"release.html"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.pipeline.RunUpload(ctx, importer.UploadRequest{PipelineRequest: importer.PipelineRequest{
		JobID: job.ID, RunKey: "attempt-1", WikiID: testkit.DefaultWikiID, ActorID: fixture.actorID,
		SourceID: &fixture.sourceID, Provider: "fake", Model: "golden-model"},
		Filename: "release.html", MIMEType: "text/html", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.Status != importer.JobSucceeded || result.Job.Progress != 100 || result.Reused || len(result.ProposalIDs) != 1 {
		t.Fatalf("result=%+v", result)
	}
	tasks, err := fixture.reviews.Pending(ctx, 20)
	if err != nil || len(tasks) != 1 || tasks[0].ProposalID != result.ProposalIDs[0] {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	if _, err := fixture.reviews.Decide(ctx, tasks[0].ID, fixture.actorID, true, "golden evidence verified"); err != nil {
		t.Fatal(err)
	}
	applied, err := fixture.apply.Apply(ctx, result.ProposalIDs[0], fixture.actorID)
	if err != nil || len(applied.ClaimIDs) != 1 {
		t.Fatalf("applied=%+v err=%v", applied, err)
	}
	claim, err := fixture.knowledge.GetClaim(ctx, applied.ClaimIDs[0])
	if err != nil || claim.Status != knowledge.ClaimStatusPublished || string(claim.ValueJSON) != `"2026-07-22"` {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	sources, _ := fixture.knowledge.ListClaimSources(ctx, claim.ID)
	if len(sources) != 1 {
		t.Fatalf("claim sources=%+v", sources)
	}
	var claimEvents int
	if err := fixture.tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_type='claim' AND aggregate_id=$1`, claim.ID).Scan(&claimEvents); err != nil || claimEvents == 0 {
		t.Fatalf("claim projection events=%d err=%v", claimEvents, err)
	}

	secondJob, err := fixture.jobs.Create(ctx, fixture.actorID, "source_import", "golden-2", json.RawMessage(`{"fixture":"release.html"}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.pipeline.RunUpload(ctx, importer.UploadRequest{PipelineRequest: importer.PipelineRequest{
		JobID: secondJob.ID, RunKey: "attempt-1", WikiID: testkit.DefaultWikiID, ActorID: fixture.actorID,
		SourceID: &fixture.sourceID, Provider: "fake", Model: "golden-model"},
		Filename: "release.html", MIMEType: "text/html", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reused || second.SourceVersionID != result.SourceVersionID || len(second.ProposalIDs) != 1 || second.ProposalIDs[0] != result.ProposalIDs[0] || fixture.generator.calls != 1 {
		t.Fatalf("second=%+v generator calls=%d", second, fixture.generator.calls)
	}
	for table, expected := range map[string]int{"source_version": 1, "import_extraction": 1, "proposal": 1, "citation": 1, "claim": 1} {
		var count int
		if err := fixture.tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != expected {
			t.Fatalf("%s count=%d want=%d err=%v", table, count, expected, err)
		}
	}
}

func TestPipeline_ParseFailureRetainsRawAssetAndSource(t *testing.T) {
	fixture := newPipelineFixture(t)
	ctx := context.Background()
	job, err := fixture.jobs.Create(ctx, fixture.actorID, "source_import", "bad-pdf", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	badPDF := []byte("%PDF-1.4\nno extractable text\n%%EOF")
	_, err = fixture.pipeline.RunUpload(ctx, importer.UploadRequest{PipelineRequest: importer.PipelineRequest{
		JobID: job.ID, RunKey: "attempt-1", WikiID: testkit.DefaultWikiID, ActorID: fixture.actorID,
		Provider: "fake", Model: "golden-model", Title: "Bad PDF"},
		Filename: "bad.pdf", MIMEType: "application/pdf", Content: badPDF})
	if err == nil {
		t.Fatal("parse failure expected")
	}
	detail, err := fixture.jobs.Detail(ctx, job.ID)
	if err != nil || detail.Job.Status != importer.JobFailed || detail.Job.CurrentStage != importer.StageParse {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	var assets, sources, versions int
	_ = fixture.tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM asset_revision`).Scan(&assets)
	_ = fixture.tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM source`).Scan(&sources)
	_ = fixture.tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM source_version`).Scan(&versions)
	if assets != 1 || sources != 2 || versions != 0 { // fixture source + failed-import source
		t.Fatalf("assets=%d sources=%d versions=%d", assets, sources, versions)
	}
}

func TestPipeline_QualityAndPromptInjectionNeverReachProposal(t *testing.T) {
	for _, test := range []struct {
		name      string
		quality   float64
		injection bool
	}{
		{name: "low quality", quality: .25},
		{name: "prompt injection", quality: .95, injection: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPipelineFixture(t)
			fixture.generator.quality = test.quality
			fixture.generator.injection = test.injection
			content, err := os.ReadFile("testdata/golden/release.html")
			if err != nil {
				t.Fatal(err)
			}
			job, err := fixture.jobs.Create(context.Background(), fixture.actorID, "source_import", "quality-"+test.name, json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			_, err = fixture.pipeline.RunUpload(context.Background(), importer.UploadRequest{PipelineRequest: importer.PipelineRequest{
				JobID: job.ID, RunKey: "attempt-1", WikiID: testkit.DefaultWikiID, ActorID: fixture.actorID,
				SourceID: &fixture.sourceID, Provider: "fake", Model: "golden-model"},
				Filename: test.name + ".html", MIMEType: "text/html", Content: content})
			if !errors.Is(err, importer.ErrQualityGate) {
				t.Fatalf("err=%v", err)
			}
			detail, _ := fixture.jobs.Detail(context.Background(), job.ID)
			if detail.Job.Status != importer.JobFailed || detail.Job.CurrentStage != importer.StageExtract {
				t.Fatalf("job=%+v", detail.Job)
			}
			var proposals, citations int
			_ = fixture.tdb.Pool.QueryRow(context.Background(), `SELECT count(*) FROM proposal`).Scan(&proposals)
			_ = fixture.tdb.Pool.QueryRow(context.Background(), `SELECT count(*) FROM citation`).Scan(&citations)
			if proposals != 0 || citations != 0 {
				t.Fatalf("quality gate leaked writes: proposals=%d citations=%d", proposals, citations)
			}
		})
	}
}

func TestPipeline_AcquisitionFailureIsRecordedAtFetch(t *testing.T) {
	fixture := newPipelineFixture(t)
	job, err := fixture.jobs.Create(context.Background(), fixture.actorID, "source_import", "bad-upload",
		json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.pipeline.RunUpload(context.Background(), importer.UploadRequest{PipelineRequest: importer.PipelineRequest{
		JobID: job.ID, RunKey: "attempt-1", WikiID: testkit.DefaultWikiID, ActorID: fixture.actorID,
		Provider: "fake", Model: "golden-model"}, Filename: "payload.exe",
		MIMEType: "application/octet-stream", Content: []byte("not allowed")})
	if !errors.Is(err, importer.ErrUnsupportedMIME) {
		t.Fatalf("err=%v", err)
	}
	detail, err := fixture.jobs.Detail(context.Background(), job.ID)
	if err != nil || detail.Job.Status != importer.JobFailed || detail.Job.CurrentStage != importer.StageFetch || len(detail.Stages) != 1 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}

func TestRunner_ConsumesStoredUploadEndToEnd(t *testing.T) {
	fixture := newPipelineFixture(t)
	content, err := os.ReadFile("testdata/golden/release.html")
	if err != nil {
		t.Fatal(err)
	}
	staging := storage.NewFake()
	hash := importer.HashBytes(content)
	key := "test/asset/" + hash[:2] + "/" + hash
	if err := staging.Put(context.Background(), key, strings.NewReader(string(content)), int64(len(content)), "text/html"); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]any{"source": map[string]any{"kind": "upload", "storage_key": key,
		"filename": "release.html", "mime_type": "text/html", "content_hash": hash}, "source_id": fixture.sourceID})
	job, err := fixture.jobs.Create(context.Background(), fixture.actorID, "source_import", "stored-upload", config)
	if err != nil {
		t.Fatal(err)
	}
	runner := importer.NewRunner(fixture.jobs, fixture.pipeline, importer.RunnerConfig{WikiID: testkit.DefaultWikiID,
		Provider: "fake", Model: "golden-model", UploadStore: staging})
	processed, err := runner.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	detail, err := fixture.jobs.Detail(context.Background(), job.ID)
	if err != nil || detail.Job.Status != importer.JobSucceeded || detail.Job.Progress != 100 || fixture.generator.calls != 1 {
		t.Fatalf("detail=%+v calls=%d err=%v", detail, fixture.generator.calls, err)
	}
}
