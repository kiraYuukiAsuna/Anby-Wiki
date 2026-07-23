package governance_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

func TestEntityMergeRepairProposal_ReusesRetargetOperationsAtomically(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "merge-repair")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pages := page.NewService(pageRepo, txm, ids)
	knowledgeService := knowledge.NewService(
		knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids,
	)
	source, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "repair-source",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "旧实体", IsPrimary: true}},
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "repair-target",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "目标实体", IsPrimary: true}},
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := knowledgeService.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: source.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2026-07-24"), OriginType: knowledge.OriginHuman,
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	createdPage, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
		Title: "Entity merge repair", ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockID := "00000000-0000-7000-8000-000000009601"
	content, _ := json.Marshal([]*ast.InlineNode{
		{Type: ast.InlineEntityReference, EntityID: source.ID.String(), DisplayText: "旧实体"},
		{Type: ast.InlineClaimReference, ClaimID: claim.ID.String(), DisplayText: "发布日期"},
	})
	document := &ast.Document{
		Type: "document", SchemaVersion: 1,
		Children: []*ast.Block{{ID: blockID, Type: ast.BlockParagraph, Content: content}},
	}
	revision, err := pages.Publish(ctx, page.PublishParams{
		PageID: createdPage.ID, ActorID: human, AST: mustJSON(t, document),
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := tdb.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.NewEntityMentionsBuilder(tdb.Pool).
		Rebuild(ctx, tx, createdPage.ID, revision.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := projection.NewClaimUsageBuilder(tdb.Pool).
		Rebuild(ctx, tx, createdPage.ID, revision.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	merge, err := knowledgeService.MergeEntity(ctx, knowledge.MergeEntityParams{
		SourceEntityID: source.ID, TargetEntityID: target.ID,
		ActorID: human, Reason: "repair test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE actor SET status='disabled' WHERE id=$1`, human); err != nil {
		t.Fatal(err)
	}

	repo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(repo, txm, ids)
	result, err := proposals.CreateEntityMergeRepairProposals(ctx, merge.Merge.ID, human)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Proposals) != 1 || result.Operations != 2 {
		t.Fatalf("repair result=%+v", result)
	}
	proposal := result.Proposals[0]
	if proposal.Status != governance.ProposalDraft || proposal.RiskLevel != governance.RiskHigh ||
		proposal.BaseRevisionID == nil || *proposal.BaseRevisionID != revision.ID ||
		proposal.CreatedBy != testkit.SystemActorID {
		t.Fatalf("proposal=%+v", proposal)
	}
	var auditActor uuid.UUID
	var auditPayload map[string]any
	if err := tdb.Pool.QueryRow(ctx, `SELECT actor_id,payload_json FROM audit_event
		WHERE aggregate_id=$1 AND event_type='proposal.entity_merge_repair_created'`,
		proposal.ID).Scan(&auditActor, &auditPayload); err != nil {
		t.Fatal(err)
	}
	if auditActor != human || auditPayload["original_actor_id"] != human.String() ||
		auditPayload["system_actor_id"] != testkit.SystemActorID.String() {
		t.Fatalf("repair audit actor=%s payload=%v", auditActor, auditPayload)
	}
	operations, err := proposals.ListOperations(ctx, proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if operations[0].OperationType != governance.OpRetargetEntityReference ||
		operations[1].OperationType != governance.OpRetargetClaimReference {
		t.Fatalf("operation types=%s,%s", operations[0].OperationType, operations[1].OperationType)
	}
	if operations[0].ExpectedHash == nil || operations[1].ExpectedHash != nil {
		t.Fatalf("same-block hash guards first=%v second=%v",
			operations[0].ExpectedHash, operations[1].ExpectedHash)
	}
	again, err := proposals.CreateEntityMergeRepairProposals(ctx, merge.Merge.ID, human)
	if err != nil || !again.Idempotent || again.Proposals[0].ID != proposal.ID {
		t.Fatalf("idempotent repair=%+v err=%v", again, err)
	}
	var proposalCount, auditCount int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM proposal
		WHERE idempotency_key LIKE $1`, "entity-merge-repair:"+merge.Merge.ID.String()+":%").
		Scan(&proposalCount); err != nil || proposalCount != 1 {
		t.Fatalf("proposal count=%d err=%v", proposalCount, err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event
		WHERE aggregate_id=$1 AND event_type='proposal.entity_merge_repair_created'`,
		proposal.ID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("repair audit count=%d err=%v", auditCount, err)
	}
}

func TestEntityMergeRepairProposal_RolledBackBeforeConsumptionIsNoOp(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "merge-repair-rolled-back")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	knowledgeService := knowledge.NewService(
		knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids,
	)
	source, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "repair-rollback-source",
		Labels: []knowledge.LabelInput{
			{Language: "zh-Hans", Label: "回滚源实体", IsPrimary: true},
		},
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "repair-rollback-target",
		Labels: []knowledge.LabelInput{
			{Language: "zh-Hans", Label: "回滚目标实体", IsPrimary: true},
		},
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	merge, err := knowledgeService.MergeEntity(ctx, knowledge.MergeEntityParams{
		SourceEntityID: source.ID, TargetEntityID: target.ID, ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgeService.RollbackEntityMerge(ctx, merge.Merge.ID, human); err != nil {
		t.Fatal(err)
	}

	proposals := governance.NewService(governance.NewRepository(tdb.Pool), txm, ids)
	result, err := proposals.CreateEntityMergeRepairProposals(ctx, merge.Merge.ID, human)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Idempotent || len(result.Proposals) != 0 || result.Operations != 0 {
		t.Fatalf("rolled-back repair result=%+v", result)
	}
	var proposalCount, auditCount int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM proposal
		WHERE idempotency_key LIKE $1`, "entity-merge-repair:"+merge.Merge.ID.String()+":%").
		Scan(&proposalCount)
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event
		WHERE event_type='proposal.entity_merge_repair_created'
		AND payload_json->>'merge_id'=$1`, merge.Merge.ID.String()).Scan(&auditCount)
	if proposalCount != 0 || auditCount != 0 {
		t.Fatalf("rolled-back proposal count=%d audit count=%d", proposalCount, auditCount)
	}
}
