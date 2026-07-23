package governance_test

import (
	"context"
	"testing"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestResolveConflictChooseProposedRevalidatesAndApplies(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "resolver")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pages := page.NewService(page.NewRepository(tdb.Pool), txm, ids)
	repo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(repo, txm, ids)
	conflictService := governance.NewConflictService(repo, pages, nil, txm, ids)

	created, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
		Title: "Resolve conflict", ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockID := "00000000-0000-7000-8000-000000000761"
	base := documentWithParagraphs(t, paragraph(t, blockID, "Base"))
	baseRevision, err := pages.Publish(ctx, page.PublishParams{
		PageID: created.ID, ActorID: human, AST: mustJSON(t, base),
	})
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := governance.BlockHash(base.Children[0])
	proposal := makeApprovedPageProposal(
		t, proposals, human, created.ID, baseRevision.ID, blockID, hash,
		"resolve-proposed", "Proposed",
	)
	recordApprovalEvidence(t, tdb, proposal.ID, human)
	current := documentWithParagraphs(t, paragraph(t, blockID, "Current"))
	if _, err := pages.Publish(ctx, page.PublishParams{
		PageID: created.ID, ActorID: human, ExpectedRevisionID: &baseRevision.ID,
		AST: mustJSON(t, current),
	}); err != nil {
		t.Fatal(err)
	}
	conflicts, err := conflictService.DetectAndRecord(ctx, proposal.ID)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts=%+v err=%v", conflicts, err)
	}
	resolution := governance.NewConflictResolutionService(repo, txm, ids)
	resolved, err := resolution.Resolve(ctx, governance.ResolveConflictParams{
		ProposalID: proposal.ID, ConflictID: conflicts[0].ID, ActorID: human,
		Choice: governance.ResolutionChooseProposed, Reason: "人工确认 AI 值",
	})
	if err != nil || resolved.Status != governance.ProposalApproved {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	apply := governance.NewApplyService(
		repo, pages, governance.NewPagePatchEngine(), nil,
		conflictService, txm, ids,
	)
	result, err := apply.Apply(ctx, proposal.ID, human)
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, err := pages.GetRevision(ctx, created.ID, *result.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	document, _ := ast.Parse(snapshot.AST)
	nodes, _ := document.Children[0].InlineContent()
	if nodes[0].Text != "Proposed" {
		t.Fatalf("text=%s", nodes[0].Text)
	}
	persisted, _ := repo.ListMergeConflicts(ctx, proposal.ID)
	if len(persisted) != 1 || persisted[0].Status != governance.ConflictResolved ||
		persisted[0].ResolvedBy == nil || len(persisted[0].Resolution) == 0 {
		t.Fatalf("persisted=%+v", persisted)
	}
	var audits int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_event
		WHERE aggregate_id=$1 AND event_type='proposal.conflict_resolved'`, proposal.ID).
		Scan(&audits)
	if audits != 1 {
		t.Fatalf("audits=%d", audits)
	}
}

func TestResolveConflictRejectsNonHumanAndDuplicate(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "resolver-human")
	ai := tdb.MakeActor(t, "ai", "resolver-ai")
	proposalID := tdb.NewID(t)
	conflictID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO proposal
		(id,target_type,status,risk_level,created_by,idempotency_key)
		VALUES ($1,'page','conflicted','low',$2,'resolution-negative')`,
		proposalID, human); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO merge_conflict
		(id,proposal_id,conflict_type,status) VALUES ($1,$2,'semantic','open')`,
		conflictID, proposalID); err != nil {
		t.Fatal(err)
	}
	service := governance.NewConflictResolutionService(
		governance.NewRepository(tdb.Pool), db.NewTxManager(tdb.Pool), id.NewGenerator(),
	)
	if _, err := service.Resolve(ctx, governance.ResolveConflictParams{
		ProposalID: proposalID, ConflictID: conflictID, ActorID: ai,
		Choice: governance.ResolutionChooseCurrent,
	}); err == nil {
		t.Fatal("AI resolution should be rejected")
	}
	if _, err := service.Resolve(ctx, governance.ResolveConflictParams{
		ProposalID: proposalID, ConflictID: conflictID, ActorID: human,
		Choice: governance.ResolutionChooseCurrent,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(ctx, governance.ResolveConflictParams{
		ProposalID: proposalID, ConflictID: conflictID, ActorID: human,
		Choice: governance.ResolutionChooseCurrent,
	}); err == nil {
		t.Fatal("duplicate resolution should be rejected")
	}
}
