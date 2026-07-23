package importer_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/importer"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func setupMatching(t *testing.T) (*testkit.DB, *knowledge.Service, uuid.UUID) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	actorID := tdb.MakeActor(t, "human", "matcher reviewer")
	service := knowledge.NewService(knowledge.NewRepository(tdb.Pool), page.NewRepository(tdb.Pool),
		db.NewTxManager(tdb.Pool), id.NewGenerator()).WithCitationChecker(evidence.NewRepository(tdb.Pool))
	return tdb, service, actorID
}

func createMatchingEntity(t *testing.T, service *knowledge.Service, actorID uuid.UUID, canonical string) *knowledge.Entity {
	t.Helper()
	entity, err := service.CreateEntity(context.Background(), knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: canonical,
		Labels: []knowledge.LabelInput{{Language: "en", Label: "Anby Wiki", IsPrimary: true}}, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return entity
}

func TestEntityMatcher_AmbiguityAndPageBindingWithoutAutoCreate(t *testing.T) {
	tdb, service, actorID := setupMatching(t)
	first := createMatchingEntity(t, service, actorID, "anby-wiki-one")
	second := createMatchingEntity(t, service, actorID, "anby-wiki-two")
	candidateID := uuid.New()
	candidates := []importer.EntityCandidate{{CandidateID: candidateID, TypeKey: "software", Label: "Anby Wiki", Confidence: .98}}

	matcher := importer.NewEntityMatcher(service)
	results, err := matcher.Match(context.Background(), testkit.DefaultWikiID, nil, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Outcome != importer.EntityAmbiguous || results[0].EntityID != nil {
		t.Fatalf("ambiguous result=%+v", results)
	}

	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "anby-wiki", "Anby Wiki", actorID)
	if _, err := service.BindPage(context.Background(), knowledge.BindPageParams{
		PageID: pageID, EntityID: second.ID, Role: knowledge.BindingRolePrimary, Language: "en",
	}); err != nil {
		t.Fatal(err)
	}
	results, err = matcher.Match(context.Background(), testkit.DefaultWikiID, &pageID, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Outcome != importer.EntityMatched || results[0].EntityID == nil || *results[0].EntityID != second.ID {
		t.Fatalf("bound result=%+v first=%s second=%s", results[0], first.ID, second.ID)
	}
	var count int
	if err := tdb.Pool.QueryRow(context.Background(), `SELECT count(*) FROM entity`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("matcher must not create entities: count=%d err=%v", count, err)
	}
}

func TestClaimClassifier_SupportSupersedeContradictionAndRisk(t *testing.T) {
	tdb, service, actorID := setupMatching(t)
	entity := createMatchingEntity(t, service, actorID, "anby-wiki")
	release, err := service.CreateClaim(context.Background(), knowledge.CreateClaimParams{
		SubjectEntityID: entity.ID, PropertyKey: "release_date", Value: knowledge.DateValue("2026-07-22"),
		OriginType: knowledge.OriginHuman, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateVerificationStatus(context.Background(), knowledge.UpdateVerificationStatusParams{
		ClaimID: release.ID, Status: knowledge.VerificationHumanVerified, ActorID: actorID,
	}); err != nil {
		t.Fatal(err)
	}
	tdb.MakeProperty(t, "supported_platform", knowledge.ValueTypeString, true, nil, nil, "")
	platformClaim, err := service.CreateClaim(context.Background(), knowledge.CreateClaimParams{
		SubjectEntityID: entity.ID, PropertyKey: "supported_platform", Value: knowledge.StringValue("macOS"),
		OriginType: knowledge.OriginHuman, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}

	entityCandidateID := uuid.New()
	resolutions := []importer.EntityResolution{{CandidateID: entityCandidateID, Outcome: importer.EntityMatched, EntityID: &entity.ID}}
	makeClaim := func(property string, value any) importer.ClaimCandidate {
		raw, _ := json.Marshal(value)
		return importer.ClaimCandidate{CandidateID: uuid.New(), Subject: importer.CandidateSubject{CandidateID: &entityCandidateID},
			PropertyKey: property, Value: raw, Confidence: .95}
	}
	candidates := []importer.ClaimCandidate{
		makeClaim("release_date", map[string]any{"date": "2026-07-22"}),
		makeClaim("release_date", map[string]any{"date": "2026-08-01"}),
		makeClaim("supported_platform", map[string]any{"string": "Linux"}),
	}
	decisions, err := importer.NewClaimClassifier(service).Classify(context.Background(), candidates, resolutions)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 3 || decisions[0].Outcome != importer.ClaimSupport ||
		decisions[0].ExistingClaimID == nil || *decisions[0].ExistingClaimID != release.ID {
		t.Fatalf("support=%+v", decisions)
	}
	if decisions[1].Outcome != importer.ClaimSupersede || decisions[1].Risk != importer.RiskHigh {
		t.Fatalf("supersede=%+v", decisions[1])
	}
	if decisions[2].Outcome != importer.ClaimContradiction || decisions[2].Risk != importer.RiskMedium ||
		decisions[2].ExistingClaimID == nil || *decisions[2].ExistingClaimID != platformClaim.ID {
		t.Fatalf("contradiction=%+v", decisions[2])
	}
}
