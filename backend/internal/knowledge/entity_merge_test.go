package knowledge_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anby/wiki/backend/internal/knowledge"
)

func TestEntityMerge_MigratesLabelsClaimsResolvesAndCompensates(t *testing.T) {
	tdb, svc := setup(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "entity-merger")
	source := createEntity(t, svc, "software", "merge-source", "旧实体", human)
	target := createEntity(t, svc, "software", "merge-target", "目标实体", human)
	owner := createEntity(t, svc, "organization", "merge-owner", "所有者", human)
	if _, err := svc.AddLabel(ctx, source.ID, knowledge.LabelInput{
		Language: "en", Label: "Old Entity", IsPrimary: true,
	}); err != nil {
		t.Fatal(err)
	}
	subjectClaim, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: source.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2026-07-24"), OriginType: knowledge.OriginHuman,
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	targetClaim, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: owner.ID, PropertyKey: "developer",
		Value: knowledge.EntityValue(source.ID), OriginType: knowledge.OriginHuman,
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}

	merged, err := svc.MergeEntity(ctx, knowledge.MergeEntityParams{
		SourceEntityID: source.ID, TargetEntityID: target.ID,
		ActorID: human, Reason: "人工确认重复实体",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.LabelMappings) != 2 || len(merged.ClaimMappings) != 2 {
		t.Fatalf("merge mappings labels=%d claims=%d", len(merged.LabelMappings), len(merged.ClaimMappings))
	}
	resolved, err := svc.ResolveEntity(ctx, source.ID)
	if err != nil || resolved.ID != target.ID {
		t.Fatalf("ResolveEntity=%v err=%v", resolved, err)
	}
	again, err := svc.MergeEntity(ctx, knowledge.MergeEntityParams{
		SourceEntityID: source.ID, TargetEntityID: target.ID,
		ActorID: human, Reason: "重复请求",
	})
	if err != nil || !again.Idempotent || again.Merge.ID != merged.Merge.ID {
		t.Fatalf("idempotent merge=%+v err=%v", again, err)
	}
	for _, mapping := range merged.ClaimMappings {
		oldClaim, err := svc.GetClaim(ctx, mapping.OldClaimID)
		if err != nil || oldClaim.Status != knowledge.ClaimStatusDeprecated {
			t.Fatalf("old claim=%+v err=%v", oldClaim, err)
		}
		newClaim, err := svc.GetClaim(ctx, mapping.NewClaimID)
		if err != nil || newClaim.Status != knowledge.ClaimStatusPublished {
			t.Fatalf("new claim=%+v err=%v", newClaim, err)
		}
		resolvedClaim, err := svc.ResolveMergedClaim(ctx, mapping.OldClaimID)
		if err != nil || resolvedClaim.ID != mapping.NewClaimID {
			t.Fatalf("resolved claim=%+v err=%v", resolvedClaim, err)
		}
		switch mapping.OldClaimID {
		case subjectClaim.ID:
			if newClaim.SubjectEntityID != target.ID {
				t.Fatalf("subject claim target=%s", newClaim.SubjectEntityID)
			}
		case targetClaim.ID:
			if newClaim.TargetEntityID == nil || *newClaim.TargetEntityID != target.ID {
				t.Fatalf("target claim target=%v", newClaim.TargetEntityID)
			}
		}
	}
	var auditCount int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event
		WHERE aggregate_type='entity_merge' AND aggregate_id=$1`, merged.Merge.ID).
		Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("merge audit=%d err=%v", auditCount, err)
	}
	var outboxCount int
	var outboxPayload []byte
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*),max(payload_json::text)
		FROM outbox_event WHERE aggregate_type='entity_merge' AND aggregate_id=$1
			AND event_type=$2`, merged.Merge.ID, knowledge.OutboxEventEntityMerged).
		Scan(&outboxCount, &outboxPayload); err != nil || outboxCount != 1 {
		t.Fatalf("merge outbox=%d err=%v", outboxCount, err)
	}
	var event struct {
		MergeID        string `json:"merge_id"`
		ActorID        string `json:"actor_id"`
		SourceEntityID string `json:"source_entity_id"`
		TargetEntityID string `json:"target_entity_id"`
	}
	if err := json.Unmarshal(outboxPayload, &event); err != nil {
		t.Fatal(err)
	}
	if event.MergeID != merged.Merge.ID.String() || event.ActorID != human.String() ||
		event.SourceEntityID != source.ID.String() || event.TargetEntityID != target.ID.String() {
		t.Fatalf("entity.merged payload=%+v", event)
	}

	rolledBack, err := svc.RollbackEntityMerge(ctx, merged.Merge.ID, human)
	if err != nil {
		t.Fatal(err)
	}
	if len(rolledBack.CompensatedClaimIDs) != 2 || rolledBack.RemovedTargetLabels != 2 {
		t.Fatalf("rollback=%+v", rolledBack)
	}
	restored, err := svc.ResolveEntity(ctx, source.ID)
	if err != nil || restored.ID != source.ID {
		t.Fatalf("restored=%v err=%v", restored, err)
	}
	for _, mapping := range merged.ClaimMappings {
		oldClaim, _ := svc.GetClaim(ctx, mapping.OldClaimID)
		newClaim, _ := svc.GetClaim(ctx, mapping.NewClaimID)
		if oldClaim.Status != knowledge.ClaimStatusPublished ||
			newClaim.Status != knowledge.ClaimStatusDeprecated {
			t.Fatalf("compensation old=%s new=%s", oldClaim.Status, newClaim.Status)
		}
		resolvedClaim, err := svc.ResolveMergedClaim(ctx, mapping.OldClaimID)
		if err != nil || resolvedClaim.ID != mapping.OldClaimID {
			t.Fatalf("rollback resolved claim=%+v err=%v", resolvedClaim, err)
		}
	}
	againRollback, err := svc.RollbackEntityMerge(ctx, merged.Merge.ID, human)
	if err != nil || !againRollback.Idempotent {
		t.Fatalf("idempotent rollback=%+v err=%v", againRollback, err)
	}
}

func TestEntityMerge_RollbackRejectsChangedClaimAtomically(t *testing.T) {
	tdb, svc := setup(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "entity-merger-stale")
	source := createEntity(t, svc, "software", "stale-source", "旧实体", human)
	target := createEntity(t, svc, "software", "stale-target", "目标实体", human)
	if _, err := svc.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: source.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2026-07-24"), OriginType: knowledge.OriginHuman,
		ActorID: human,
	}); err != nil {
		t.Fatal(err)
	}
	merged, err := svc.MergeEntity(ctx, knowledge.MergeEntityParams{
		SourceEntityID: source.ID, TargetEntityID: target.ID, ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	newClaimID := merged.ClaimMappings[0].NewClaimID
	if _, err := tdb.Pool.Exec(ctx, `UPDATE claim SET status='deprecated' WHERE id=$1`, newClaimID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RollbackEntityMerge(ctx, merged.Merge.ID, human); !errors.Is(err, knowledge.ErrEntityMergeStale) {
		t.Fatalf("rollback err=%v", err)
	}
	entity, err := svc.GetEntity(ctx, source.ID)
	if err != nil || entity.Status != knowledge.StatusMerged {
		t.Fatalf("rollback must be atomic entity=%+v err=%v", entity, err)
	}
}
