package governance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/collaboration"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestMergeWorkingDocumentCASAppliesProposalWithoutRevision(t *testing.T) {
	tdb, merge, document, proposal, actor, currentAST, mergedAST := setupWorkingMerge(t)
	ctx := context.Background()
	result, update, err := merge.Merge(ctx, governance.MergeWorkingDocumentParams{
		ProposalID: proposal.ID, DocumentID: document.ID, ActorID: actor,
		ClientID: uuid.New(), ClientUpdateID: uuid.New(), ExpectedSequence: 0,
		CurrentAST: currentAST, MergedAST: mergedAST, Update: []byte("yjs-delta"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Sequence != 1 || update.Sequence != 1 || result.ChangeBatchID == uuid.Nil {
		t.Fatalf("result=%+v update=%+v", result, update)
	}
	var status string
	var latest int64
	if err := tdb.Pool.QueryRow(ctx, `SELECT status FROM proposal WHERE id=$1`, proposal.ID).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := tdb.Pool.QueryRow(ctx, `SELECT latest_sequence FROM working_document WHERE id=$1`, document.ID).
		Scan(&latest); err != nil {
		t.Fatal(err)
	}
	var revisions int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM revision WHERE page_id=$1`, document.PageID).
		Scan(&revisions)
	if status != governance.ProposalApplied || latest != 1 || revisions != 1 {
		t.Fatalf("status=%s latest=%d revisions=%d", status, latest, revisions)
	}
	replayed, replayedUpdate, err := merge.Merge(ctx, governance.MergeWorkingDocumentParams{
		ProposalID: proposal.ID, DocumentID: document.ID, ActorID: actor,
		ClientID: uuid.New(), ClientUpdateID: uuid.New(), ExpectedSequence: 0,
		CurrentAST: currentAST, MergedAST: mergedAST, Update: []byte("yjs-delta"),
	})
	if err != nil || !replayed.Idempotent || replayedUpdate != nil ||
		replayed.ChangeBatchID != result.ChangeBatchID {
		t.Fatalf("replayed=%+v update=%+v err=%v", replayed, replayedUpdate, err)
	}
}

func TestMergeWorkingDocumentRejectsSequenceAndASTMismatch(t *testing.T) {
	tdb, merge, document, proposal, actor, currentAST, mergedAST := setupWorkingMerge(t)
	ctx := context.Background()
	if _, _, err := merge.Merge(ctx, governance.MergeWorkingDocumentParams{
		ProposalID: proposal.ID, DocumentID: document.ID, ActorID: actor,
		ClientID: uuid.New(), ClientUpdateID: uuid.New(), ExpectedSequence: 1,
		CurrentAST: currentAST, MergedAST: mergedAST, Update: []byte("stale"),
	}); !errors.Is(err, collaboration.ErrSequenceMismatch) {
		t.Fatalf("sequence err=%v", err)
	}
	if _, _, err := merge.Merge(ctx, governance.MergeWorkingDocumentParams{
		ProposalID: proposal.ID, DocumentID: document.ID, ActorID: actor,
		ClientID: uuid.New(), ClientUpdateID: uuid.New(), ExpectedSequence: 0,
		CurrentAST: currentAST, MergedAST: currentAST, Update: []byte("wrong-ast"),
	}); !errors.Is(err, governance.ErrMergedASTMismatch) {
		t.Fatalf("AST err=%v", err)
	}
	var updates, batches int
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM working_document_update WHERE document_id=$1`, document.ID).
		Scan(&updates)
	_ = tdb.Pool.QueryRow(ctx, `SELECT count(*) FROM change_batch WHERE proposal_id=$1`, proposal.ID).
		Scan(&batches)
	if updates != 0 || batches != 0 {
		t.Fatalf("updates=%d batches=%d", updates, batches)
	}
}

func setupWorkingMerge(t *testing.T) (
	*testkit.DB,
	*governance.MergeWorkingDocumentService,
	collaboration.Document,
	*governance.Proposal,
	uuid.UUID,
	[]byte,
	[]byte,
) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actor := tdb.MakeActor(t, "human", "working-merge")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pages := page.NewService(page.NewRepository(tdb.Pool), txm, ids)
	created, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
		Title: "Working merge", ActorID: actor,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockID := "00000000-0000-7000-8000-000000000751"
	current := documentWithParagraphs(t, paragraph(t, blockID, "Current"))
	revision, err := pages.Publish(ctx, page.PublishParams{
		PageID: created.ID, ActorID: actor, AST: mustJSON(t, current),
	})
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := governance.BlockHash(current.Children[0])
	repo := governance.NewRepository(tdb.Pool)
	proposals := governance.NewService(repo, txm, ids)
	proposal := makeApprovedPageProposal(
		t, proposals, actor, created.ID, revision.ID, blockID, hash,
		"working-merge", "AI merged",
	)
	recordApprovalEvidence(t, tdb, proposal.ID, actor)
	collaborationService := collaboration.NewService(tdb.Pool, txm, ids)
	document, err := collaborationService.Open(ctx, created.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	conflicts := governance.NewConflictService(repo, pages, nil, txm, ids)
	merge := governance.NewMergeWorkingDocumentService(
		repo, governance.NewPagePatchEngine(), conflicts,
		collaborationService, txm, ids,
	)
	merged := documentWithParagraphs(t, paragraph(t, blockID, "AI merged"))
	return tdb, merge, document, proposal, actor, mustJSON(t, current), mustJSON(t, merged)
}
