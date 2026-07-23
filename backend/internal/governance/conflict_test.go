package governance_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestConflictDetection_ThreeWayAllowsUnchangedBlockAndRecordsChangedBlock(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "reviewer")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pages := page.NewService(pageRepo, txm, ids)
	repo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(repo, txm, ids)
	conflicts := governance.NewConflictService(repo, pages, nil, txm, ids)

	p, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
		Title: "Conflict", ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	aID := "00000000-0000-7000-8000-000000000721"
	bID := "00000000-0000-7000-8000-000000000722"
	baseDoc := documentWithParagraphs(t, paragraph(t, aID, "A"), paragraph(t, bID, "B"))
	baseRev, err := pages.Publish(ctx, page.PublishParams{
		PageID: p.ID, ActorID: human, AST: mustJSON(t, baseDoc),
	})
	if err != nil {
		t.Fatal(err)
	}
	aHash, _ := governance.BlockHash(baseDoc.Children[0])
	bHash, _ := governance.BlockHash(baseDoc.Children[1])

	safe := makeApprovedPageProposal(t, proposals, human, p.ID, baseRev.ID, aID, aHash, "safe", "A proposed")
	conflicting := makeApprovedPageProposal(t, proposals, human, p.ID, baseRev.ID, bID, bHash, "conflict", "B proposed")

	currentDoc := documentWithParagraphs(t, paragraph(t, aID, "A"), paragraph(t, bID, "B current"))
	if _, err := pages.Publish(ctx, page.PublishParams{
		PageID: p.ID, ActorID: human, ExpectedRevisionID: &baseRev.ID, AST: mustJSON(t, currentDoc),
	}); err != nil {
		t.Fatal(err)
	}

	safeResult, err := conflicts.DetectAndRecord(ctx, safe.ID)
	if err != nil || len(safeResult) != 0 {
		t.Fatalf("unrelated block should merge: conflicts=%+v err=%v", safeResult, err)
	}
	result, err := conflicts.DetectAndRecord(ctx, conflicting.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].ConflictType != governance.ConflictBlockHash {
		t.Fatalf("conflicts=%+v", result)
	}
	got, _ := repo.GetProposal(ctx, nil, conflicting.ID)
	if got.Status != governance.ProposalConflicted {
		t.Fatalf("status=%s", got.Status)
	}
	persisted, err := repo.ListMergeConflicts(ctx, conflicting.ID)
	if err != nil || len(persisted) != 1 || len(persisted[0].CurrentValue) == 0 {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
}

func makeApprovedPageProposal(t *testing.T, svc *governance.Service, actor, pageID, baseRevisionID uuid.UUID,
	blockID, blockHash, key, newText string) *governance.Proposal {
	t.Helper()
	p, err := svc.CreateProposal(context.Background(), governance.CreateProposalParams{
		TargetType: governance.TargetPage, TargetID: &pageID, BaseRevisionID: &baseRevisionID,
		CreatedBy: actor, IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	op := governance.OperationV1{
		SchemaVersion: 1, OperationType: governance.OpReplaceBlock,
		Base:         governance.OperationBase{RevisionID: &baseRevisionID},
		Target:       governance.OperationTarget{PageID: &pageID, BlockID: &blockID},
		ExpectedHash: &blockHash, Evidence: []governance.OperationEvidence{{Note: "test"}},
		Risk:    governance.OperationRisk{Level: governance.RiskLow, Reasons: []string{}},
		Payload: mustJSON(t, map[string]any{"block": paragraph(t, blockID, newText)}),
	}
	addContractOperation(t, svc, p.ID, op)
	if _, err := svc.Transition(context.Background(), p.ID, governance.ProposalSubmitted); err != nil {
		t.Fatal(err)
	}
	p, err = svc.Transition(context.Background(), p.ID, governance.ProposalApproved)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
