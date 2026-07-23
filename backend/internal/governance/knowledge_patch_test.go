package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func newKnowledgePatchEngine(t *testing.T) (*governance.KnowledgePatchEngine, *knowledge.Service, *testkit.DB) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	svc := knowledge.NewService(
		knowledge.NewRepository(tdb.Pool), page.NewRepository(tdb.Pool),
		db.NewTxManager(tdb.Pool), id.NewGenerator(),
	).WithCitationChecker(evidence.NewRepository(tdb.Pool))
	return governance.NewKnowledgePatchEngine(svc), svc, tdb
}

func TestKnowledgePatchEngine_UsesDomainValidationAndStateMachine(t *testing.T) {
	engine, svc, tdb := newKnowledgePatchEngine(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "reviewer")
	batchID := tdb.NewID(t)
	proposalID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO proposal (id,target_type,status,risk_level,created_by,idempotency_key)
		VALUES ($1,'entity','approved','low',$2,'knowledge-patch-test')`, proposalID, human); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO change_batch (id,proposal_id,actor_id,status) VALUES ($1,$2,$3,'applying')`,
		batchID, proposalID, human); err != nil {
		t.Fatal(err)
	}

	createEntity := knowledgeOp(governance.OpCreateEntity, map[string]any{
		"type_key": "software", "canonical_key": "anby-wiki",
		"labels": []map[string]any{{"language": "zh-Hans", "label": "安比百科", "is_primary": true}},
	})
	createEntity.Target.WikiID = ptrUUID(testkit.DefaultWikiID)
	entityResult, err := engine.ApplyOne(ctx, createEntity, human, &batchID)
	if err != nil {
		t.Fatal(err)
	}
	if entityResult.EntityID == nil {
		t.Fatal("缺少 entity result")
	}

	createClaim := knowledgeOp(governance.OpCreateClaim, map[string]any{
		"property_key": "release_date", "value": map[string]any{"date": "2026-07-22"},
		"origin_type": "human",
	})
	createClaim.Target.EntityID = entityResult.EntityID
	claimResult, err := engine.ApplyOne(ctx, createClaim, human, &batchID)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := svc.GetClaim(ctx, *claimResult.ClaimID)
	if err != nil || claim.Status != knowledge.ClaimStatusPublished || claim.ChangeBatchID == nil || *claim.ChangeBatchID != batchID {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}

	supersede := knowledgeOp(governance.OpSupersedeClaim, map[string]any{
		"subject_entity_id": entityResult.EntityID, "property_key": "release_date",
		"value": map[string]any{"date": "2026-07-23"}, "origin_type": "human",
	})
	supersede.Target.ClaimID = claimResult.ClaimID
	newClaim, err := engine.ApplyOne(ctx, supersede, human, &batchID)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := svc.GetClaim(ctx, *claimResult.ClaimID)
	if old.Status != knowledge.ClaimStatusSuperseded || old.SupersededBy == nil || *old.SupersededBy != *newClaim.ClaimID {
		t.Fatalf("Supersede 链错误: old=%+v result=%+v", old, newClaim)
	}

	citationID := tdb.MakeCitation(t, human)
	addSource := knowledgeOp(governance.OpAddClaimSource, map[string]any{"support_type": "supports"})
	addSource.Target.ClaimID = newClaim.ClaimID
	addSource.Target.CitationID = &citationID
	if _, err := engine.ApplyOne(ctx, addSource, human, &batchID); err != nil {
		t.Fatal(err)
	}
	sources, _ := svc.ListClaimSources(ctx, *newClaim.ClaimID)
	if len(sources) != 1 || sources[0].CitationID != citationID {
		t.Fatalf("sources=%+v", sources)
	}
}

func TestKnowledgePatchEngine_RejectsInvalidValueAndAIActor(t *testing.T) {
	engine, _, tdb := newKnowledgePatchEngine(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "human")
	ai := tdb.MakeActor(t, "ai", "agent")

	create := knowledgeOp(governance.OpCreateEntity, map[string]any{
		"type_key": "software", "canonical_key": "target",
		"labels": []map[string]any{{"language": "zh-Hans", "label": "Target", "is_primary": true}},
	})
	create.Target.WikiID = ptrUUID(testkit.DefaultWikiID)
	result, err := engine.ApplyOne(ctx, create, human, nil)
	if err != nil {
		t.Fatal(err)
	}
	bad := knowledgeOp(governance.OpCreateClaim, map[string]any{
		"property_key": "release_date", "value": map[string]any{"date": "not-a-date"}, "origin_type": "human",
	})
	bad.Target.EntityID = result.EntityID
	if _, err := engine.ApplyOne(ctx, bad, human, nil); !errors.Is(err, knowledge.ErrInvalidClaimValue) {
		t.Fatalf("invalid value err=%v", err)
	}
	if _, err := engine.ApplyOne(ctx, create, ai, nil); !errors.Is(err, page.ErrActorNotAllowed) {
		t.Fatalf("AI direct write err=%v", err)
	}
}

func knowledgeOp(typ string, payload any) governance.OperationV1 {
	raw, _ := json.Marshal(payload)
	return governance.OperationV1{SchemaVersion: 1, OperationType: typ, Payload: raw}
}

func ptrUUID(v uuid.UUID) *uuid.UUID { return &v }
