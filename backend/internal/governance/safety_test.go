package governance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

// INV-05 / M5-T11：只伪造 proposal.status=approved 而没有策略自动批准或已批准
// ReviewTask，Apply 必须拒绝且不产生 ChangeBatch/Revision。
func TestGovernanceAdversarial_ForgedApprovalRejected(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "attacker")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pages := page.NewService(pageRepo, txm, ids)
	repo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(repo, txm, ids)
	p, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
		Title: "Forged", ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockID := "00000000-0000-7000-8000-000000000741"
	baseDoc := documentWithParagraphs(t, paragraph(t, blockID, "Base"))
	baseRev, err := pages.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: human, AST: mustJSON(t, baseDoc)})
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := governance.BlockHash(baseDoc.Children[0])
	forged := makeApprovedPageProposal(t, proposals, human, p.ID, baseRev.ID,
		blockID, hash, "forged", "Tampered")
	conflicts := governance.NewConflictService(repo, pages, nil, txm, ids)
	apply := governance.NewApplyService(repo, pages, governance.NewPagePatchEngine(), nil, conflicts, txm, ids)
	if _, err := apply.Apply(ctx, forged.ID, human); !errors.Is(err, governance.ErrApprovalRequired) {
		t.Fatalf("err=%v", err)
	}
	var batches, revisions int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM change_batch WHERE proposal_id=$1`, forged.ID).Scan(&batches)
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE page_id=$1`, p.ID).Scan(&revisions)
	if batches != 0 || revisions != 1 {
		t.Fatalf("batches=%d revisions=%d", batches, revisions)
	}
}
