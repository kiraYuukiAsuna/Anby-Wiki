package importer_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

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

func TestProposalComposer_CitationSchemaReviewApplyAndIdempotency(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "import reviewer")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pageService := page.NewService(pageRepo, txm, ids)
	evidenceRepo := evidence.NewRepository(tdb.Pool)
	evidenceService := evidence.NewService(evidenceRepo, pageRepo, storage.NewFake(), "test", txm, ids)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids).WithCitationChecker(evidenceRepo)
	governanceRepo := governance.NewRepository(tdb.Pool)
	governanceService := governance.NewService(governanceRepo, txm, ids)
	importService := importer.NewService(importer.NewRepository(tdb.Pool), txm, ids)

	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "anby-wiki",
		Labels: []knowledge.LabelInput{{Language: "en", Label: "Anby Wiki", IsPrimary: true}}, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := evidenceService.CreateSource(ctx, evidence.CreateSourceParams{
		SourceType: evidence.SourceTypeWebpage, Title: "Release", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := "Anby Wiki was released on 2026-07-22."
	end := int32(len([]rune(text)))
	version, err := evidenceService.AddSourceVersion(ctx, evidence.AddSourceVersionParams{
		SourceID: source.ID, VersionHash: importer.HashBytes([]byte(text)), FetchedAt: time.Now(),
		Chunks: []evidence.ChunkInput{{Ordinal: 0, TextContent: text,
			Locator: evidence.Locator{CharStart: ptrInt32(0), CharEnd: &end}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := importService.Create(ctx, actorID, "source_import", "composer-job", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	entityCandidateID, claimCandidateID := uuid.New(), uuid.New()
	start := len([]rune("Anby Wiki was released on "))
	candidates := &importer.Candidates{SchemaVersion: 1, SourceVersionID: version.Version.ID,
		Entities: []importer.EntityCandidate{{CandidateID: entityCandidateID, TypeKey: "software", Label: "Anby Wiki", Confidence: .99}},
		Claims: []importer.ClaimCandidate{{CandidateID: claimCandidateID,
			Subject: importer.CandidateSubject{CandidateID: &entityCandidateID}, PropertyKey: "release_date",
			Value: json.RawMessage(`{"date":"2026-07-22"}`), Confidence: .98,
			Evidence: []importer.CandidateEvidence{{ChunkID: version.Chunks[0].ID, Quotation: "2026-07-22",
				CharStart: start, CharEnd: start + len([]rune("2026-07-22"))}}}}, QualityScore: .96}
	resolutions := []importer.EntityResolution{{CandidateID: entityCandidateID, Outcome: importer.EntityMatched, EntityID: &entity.ID, Score: .88}}
	decisions := []importer.ClaimDecision{{CandidateID: claimCandidateID, SubjectEntityID: entity.ID,
		Outcome: importer.ClaimNew, Risk: importer.RiskLow, Reason: "new cited fact"}}
	composer := importer.NewProposalComposer(evidenceService, governanceService, knowledgeService)
	composed, err := composer.Compose(ctx, importer.ComposeParams{ImportJobID: job.ID, WikiID: testkit.DefaultWikiID,
		SourceVersionID: version.Version.ID, ActorID: actorID, Candidates: candidates,
		Resolutions: resolutions, Decisions: decisions})
	if err != nil {
		t.Fatal(err)
	}
	if len(composed.Proposals) != 1 || composed.Proposals[0].Status != governance.ProposalDraft {
		t.Fatalf("composed=%+v", composed)
	}
	operations, err := governanceService.ListOperations(ctx, composed.Proposals[0].ID)
	if err != nil || len(operations) != 1 {
		t.Fatalf("operations=%+v err=%v", operations, err)
	}
	parsed, err := governance.OperationFromRecord(&operations[0])
	if err != nil || parsed.OperationType != governance.OpCreateClaim || len(parsed.Evidence) != 1 || parsed.Evidence[0].CitationID == nil {
		t.Fatalf("parsed=%+v err=%v", parsed, err)
	}

	reviews := governance.NewReviewService(governanceRepo, txm, ids, governance.NewRiskEvaluator(knowledgeService))
	submitted, err := reviews.Submit(ctx, composed.Proposals[0].ID)
	if err != nil || submitted.ReviewTask == nil || submitted.Proposal.Status != governance.ProposalInReview {
		t.Fatalf("submitted=%+v err=%v", submitted, err)
	}
	if _, err := reviews.Decide(ctx, submitted.ReviewTask.ID, actorID, true, "citation verified"); err != nil {
		t.Fatal(err)
	}
	knowledgePatch := governance.NewKnowledgePatchEngine(knowledgeService)
	conflicts := governance.NewConflictService(governanceRepo, pageService, knowledgeService, txm, ids)
	apply := governance.NewApplyService(governanceRepo, pageService, governance.NewPagePatchEngine(), knowledgePatch, conflicts, txm, ids)
	applyResult, err := apply.Apply(ctx, composed.Proposals[0].ID, actorID)
	if err != nil || len(applyResult.ClaimIDs) != 1 {
		t.Fatalf("apply=%+v err=%v", applyResult, err)
	}
	claim, err := knowledgeService.GetClaim(ctx, applyResult.ClaimIDs[0])
	if err != nil || claim.Status != knowledge.ClaimStatusPublished {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	sources, err := knowledgeService.ListClaimSources(ctx, claim.ID)
	if err != nil || len(sources) != 1 || sources[0].CitationID != *parsed.Evidence[0].CitationID {
		t.Fatalf("sources=%+v err=%v", sources, err)
	}

	again, err := composer.Compose(ctx, importer.ComposeParams{ImportJobID: job.ID, WikiID: testkit.DefaultWikiID,
		SourceVersionID: version.Version.ID, ActorID: actorID, Candidates: candidates,
		Resolutions: resolutions, Decisions: decisions})
	if err != nil || again.Proposals[0].ID != composed.Proposals[0].ID {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	var citationCount int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM citation`).Scan(&citationCount); err != nil || citationCount != 1 {
		t.Fatalf("citation count=%d err=%v", citationCount, err)
	}
}
