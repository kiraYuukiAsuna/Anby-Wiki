package governance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestApply_PageAtomicBatchAuditOutboxAndIdempotency(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "applier")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pages := page.NewService(pageRepo, txm, ids)
	govRepo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(govRepo, txm, ids)

	p, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
		Title: "Apply", ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockID := "00000000-0000-7000-8000-000000000731"
	baseDoc := documentWithParagraphs(t, paragraph(t, blockID, "Base"))
	baseRev, err := pages.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: human, AST: mustJSON(t, baseDoc)})
	if err != nil {
		t.Fatal(err)
	}
	blockHash, _ := governance.BlockHash(baseDoc.Children[0])
	proposal := makeApprovedPageProposal(t, proposals, human, p.ID, baseRev.ID,
		blockID, blockHash, "apply-page", "Applied")
	recordApprovalEvidence(t, tdb, proposal.ID, human)
	conflictService := governance.NewConflictService(govRepo, pages, nil, txm, ids)
	apply := governance.NewApplyService(govRepo, pages, governance.NewPagePatchEngine(), nil,
		conflictService, txm, ids)

	result, err := apply.Apply(ctx, proposal.ID, human)
	if err != nil {
		t.Fatal(err)
	}
	if result.RevisionID == nil || result.ChangeBatchID == [16]byte{} {
		t.Fatalf("result=%+v", result)
	}
	rev, snap, err := pages.GetRevision(ctx, p.ID, *result.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.ChangeBatchID == nil || *rev.ChangeBatchID != result.ChangeBatchID {
		t.Fatalf("revision batch=%v want=%s", rev.ChangeBatchID, result.ChangeBatchID)
	}
	doc, _ := ast.Parse(snap.AST)
	nodes, _ := doc.Children[0].InlineContent()
	if nodes[0].Text != "Applied" {
		t.Fatalf("text=%s", nodes[0].Text)
	}
	var batchAudit, batchRevision, batchOutbox int
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE change_batch_id=$1`, result.ChangeBatchID).Scan(&batchAudit); err != nil {
		t.Fatal(err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE change_batch_id=$1`, result.ChangeBatchID).Scan(&batchRevision); err != nil {
		t.Fatal(err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='page.revision_published'`, p.ID).Scan(&batchOutbox); err != nil {
		t.Fatal(err)
	}
	if batchAudit < 2 || batchRevision != 1 || batchOutbox < 2 {
		t.Fatalf("audit=%d revisions=%d outbox=%d", batchAudit, batchRevision, batchOutbox)
	}

	again, err := apply.Apply(ctx, proposal.ID, human)
	if err != nil || !again.Idempotent || again.ChangeBatchID != result.ChangeBatchID {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	var revisionCount int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE page_id=$1`, p.ID).Scan(&revisionCount)
	if revisionCount != 2 {
		t.Fatalf("重复 Apply 创建 Revision: %d", revisionCount)
	}

	rollback := governance.NewRollbackService(govRepo, pages, nil, txm, ids)
	rolled, err := rollback.Rollback(ctx, result.ChangeBatchID, human)
	if err != nil || len(rolled.RevisionIDs) != 1 {
		t.Fatalf("rollback=%+v err=%v", rolled, err)
	}
	_, rolledSnap, err := pages.GetRevision(ctx, p.ID, rolled.RevisionIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	rolledDoc, _ := ast.Parse(rolledSnap.AST)
	rolledNodes, _ := rolledDoc.Children[0].InlineContent()
	if rolledNodes[0].Text != "Base" {
		t.Fatalf("rollback text=%s", rolledNodes[0].Text)
	}
	batch, _ := govRepo.GetChangeBatchByProposal(ctx, nil, proposal.ID)
	rolledProposal, _ := govRepo.GetProposal(ctx, nil, proposal.ID)
	if batch.Status != governance.BatchRolledBack || rolledProposal.Status != governance.ProposalRolledBack {
		t.Fatalf("batch=%s proposal=%s", batch.Status, rolledProposal.Status)
	}
	againRollback, err := rollback.Rollback(ctx, result.ChangeBatchID, human)
	if err != nil || !againRollback.Idempotent {
		t.Fatalf("again rollback=%+v err=%v", againRollback, err)
	}
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE page_id=$1`, p.ID).Scan(&revisionCount)
	if revisionCount != 3 {
		t.Fatalf("重复 Rollback 创建 Revision: %d", revisionCount)
	}
}

func TestApply_KnowledgePartialFailureRollsBackEverything(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "applier")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pages := page.NewService(pageRepo, txm, ids)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids).
		WithCitationChecker(evidence.NewRepository(tdb.Pool))
	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "atomic-target",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "原子目标", IsPrimary: true}},
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	govRepo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(govRepo, txm, ids)
	p, err := proposals.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetEntity, TargetID: &entity.ID,
		BaseStateVersion: ptrInt(0), CreatedBy: human, IdempotencyKey: "atomic-failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, date := range []string{"2026-07-22", "not-a-date"} {
		op := governance.OperationV1{
			SchemaVersion: 1, OperationType: governance.OpCreateClaim,
			Base:     governance.OperationBase{StateVersion: ptrInt(0)},
			Target:   governance.OperationTarget{EntityID: &entity.ID},
			Evidence: []governance.OperationEvidence{{Note: "test"}},
			Risk:     governance.OperationRisk{Level: governance.RiskMedium, Reasons: []string{"知识变更"}},
			Payload: mustJSON(t, map[string]any{
				"property_key": "release_date", "value": map[string]any{"date": date}, "origin_type": "human",
			}),
		}
		_ = i
		addContractOperation(t, proposals, p.ID, op)
	}
	if _, err := proposals.Transition(ctx, p.ID, governance.ProposalSubmitted); err != nil {
		t.Fatal(err)
	}
	if _, err := proposals.Transition(ctx, p.ID, governance.ProposalApproved); err != nil {
		t.Fatal(err)
	}
	recordApprovalEvidence(t, tdb, p.ID, human)
	knowledgePatch := governance.NewKnowledgePatchEngine(knowledgeService)
	conflictService := governance.NewConflictService(govRepo, pages, knowledgeService, txm, ids)
	apply := governance.NewApplyService(govRepo, pages, governance.NewPagePatchEngine(), knowledgePatch,
		conflictService, txm, ids)
	if _, err := apply.Apply(ctx, p.ID, human); err == nil {
		t.Fatal("第二个非法操作应让整个 Apply 失败")
	}
	var claims, batches, audits int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM claim WHERE subject_entity_id=$1`, entity.ID).Scan(&claims)
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM change_batch WHERE proposal_id=$1`, p.ID).Scan(&batches)
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE aggregate_id=$1`, p.ID).Scan(&audits)
	got, _ := govRepo.GetProposal(ctx, nil, p.ID)
	if claims != 0 || batches != 0 || audits != 0 || got.Status != governance.ProposalApproved {
		t.Fatalf("claims=%d batches=%d audits=%d status=%s", claims, batches, audits, got.Status)
	}
}

func TestRollback_StalePageBatchIsRejectedWithoutCompensation(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "applier")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pages := page.NewService(page.NewRepository(tdb.Pool), txm, ids)
	repo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(repo, txm, ids)
	p, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
		Title: "Stale Rollback", ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockID := "00000000-0000-7000-8000-000000000751"
	baseDoc := documentWithParagraphs(t, paragraph(t, blockID, "Base"))
	baseRev, err := pages.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: human, AST: mustJSON(t, baseDoc)})
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := governance.BlockHash(baseDoc.Children[0])
	proposal := makeApprovedPageProposal(t, proposals, human, p.ID, baseRev.ID,
		blockID, hash, "stale-rollback", "Applied")
	recordApprovalEvidence(t, tdb, proposal.ID, human)
	conflicts := governance.NewConflictService(repo, pages, nil, txm, ids)
	apply := governance.NewApplyService(repo, pages, governance.NewPagePatchEngine(), nil, conflicts, txm, ids)
	applied, err := apply.Apply(ctx, proposal.ID, human)
	if err != nil || applied.RevisionID == nil {
		t.Fatalf("apply=%+v err=%v", applied, err)
	}
	laterDoc := documentWithParagraphs(t, paragraph(t, blockID, "Later human edit"))
	if _, err := pages.Publish(ctx, page.PublishParams{
		PageID: p.ID, ActorID: human, ExpectedRevisionID: applied.RevisionID, AST: mustJSON(t, laterDoc),
	}); err != nil {
		t.Fatal(err)
	}

	rollback := governance.NewRollbackService(repo, pages, nil, txm, ids)
	if _, err := rollback.Rollback(ctx, applied.ChangeBatchID, human); !errors.Is(err, governance.ErrRollbackStale) {
		t.Fatalf("rollback err=%v", err)
	}
	var revisionCount int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE page_id=$1`, p.ID).Scan(&revisionCount)
	batch, _ := repo.GetChangeBatchByProposal(ctx, nil, proposal.ID)
	gotProposal, _ := repo.GetProposal(ctx, nil, proposal.ID)
	if revisionCount != 3 || batch.Status != governance.BatchApplied || gotProposal.Status != governance.ProposalApplied {
		t.Fatalf("revisions=%d batch=%s proposal=%s", revisionCount, batch.Status, gotProposal.Status)
	}
}

func TestRollback_CreatedClaimUsesCompensatingStateTransition(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "applier")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pages := page.NewService(pageRepo, txm, ids)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(tdb.Pool), pageRepo, txm, ids).
		WithCitationChecker(evidence.NewRepository(tdb.Pool))
	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "rollback-claim",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "回滚 Claim", IsPrimary: true}},
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(repo, txm, ids)
	p, err := proposals.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetEntity, TargetID: &entity.ID,
		BaseStateVersion: ptrInt(0), CreatedBy: human, IdempotencyKey: "rollback-created-claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	addContractOperation(t, proposals, p.ID, governance.OperationV1{
		SchemaVersion: 1, OperationType: governance.OpCreateClaim,
		Base:     governance.OperationBase{StateVersion: ptrInt(0)},
		Target:   governance.OperationTarget{EntityID: &entity.ID},
		Evidence: []governance.OperationEvidence{{Note: "reviewed evidence"}},
		Risk:     governance.OperationRisk{Level: governance.RiskMedium, Reasons: []string{"知识变更"}},
		Payload: mustJSON(t, map[string]any{
			"property_key": "release_date", "value": map[string]any{"date": "2026-07-22"},
			"origin_type": "human",
		}),
	})
	if _, err := proposals.Transition(ctx, p.ID, governance.ProposalSubmitted); err != nil {
		t.Fatal(err)
	}
	if _, err := proposals.Transition(ctx, p.ID, governance.ProposalApproved); err != nil {
		t.Fatal(err)
	}
	recordApprovalEvidence(t, tdb, p.ID, human)
	patch := governance.NewKnowledgePatchEngine(knowledgeService)
	conflicts := governance.NewConflictService(repo, pages, knowledgeService, txm, ids)
	apply := governance.NewApplyService(repo, pages, governance.NewPagePatchEngine(), patch, conflicts, txm, ids)
	applied, err := apply.Apply(ctx, p.ID, human)
	if err != nil || len(applied.ClaimIDs) != 1 {
		t.Fatalf("apply=%+v err=%v", applied, err)
	}
	createdID := applied.ClaimIDs[0]
	created, err := knowledgeService.GetClaim(ctx, createdID)
	if err != nil || created.Status != knowledge.ClaimStatusPublished {
		t.Fatalf("created=%+v err=%v", created, err)
	}

	rollback := governance.NewRollbackService(repo, pages, patch, txm, ids)
	rolled, err := rollback.Rollback(ctx, applied.ChangeBatchID, human)
	if err != nil || len(rolled.CompensationClaimIDs) != 1 || rolled.CompensationClaimIDs[0] != createdID {
		t.Fatalf("rollback=%+v err=%v", rolled, err)
	}
	deprecated, err := knowledgeService.GetClaim(ctx, createdID)
	if err != nil || deprecated.Status != knowledge.ClaimStatusDeprecated {
		t.Fatalf("deprecated=%+v err=%v", deprecated, err)
	}
}

func recordApprovalEvidence(t *testing.T, tdb *testkit.DB, proposalID, reviewerID uuid.UUID) {
	t.Helper()
	if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO review_task
		(id,proposal_id,status,reviewer_id,decision_reason,reviewed_at)
		VALUES ($1,$2,'approved',$3,'test approval',now())`, tdb.NewID(t), proposalID, reviewerID); err != nil {
		t.Fatal(err)
	}
}
