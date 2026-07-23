package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func newService(t *testing.T) (*governance.Service, *testkit.DB) {
	t.Helper()
	tdb := testkit.Open(t)
	tdb.Reset(t)
	repo := governance.NewRepository(tdb.Pool)
	return governance.NewService(repo, db.NewTxManager(tdb.Pool), id.NewGenerator()), tdb
}

func TestProposalStateMachine_IdempotencySequenceAndImmutableOps(t *testing.T) {
	svc, tdb := newService(t)
	ctx := context.Background()
	ai := tdb.MakeActor(t, "ai", "proposal agent")

	p, err := svc.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetPage, CreatedBy: ai, IdempotencyKey: "import-42/page-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetPage, CreatedBy: ai, IdempotencyKey: "import-42/page-1",
	})
	if err != nil || again.ID != p.ID {
		t.Fatalf("幂等创建 = (%v,%v)，want same id %s", again, err, p.ID)
	}

	for _, typ := range []string{"insert_block", "replace_block"} {
		op, err := svc.AddOperation(ctx, governance.AddOperationParams{
			ProposalID: p.ID, OperationType: typ,
			Evidence: json.RawMessage(`[]`), Payload: json.RawMessage(`{"block":{"type":"paragraph"}}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := lenMust(t, svc, p.ID); op.Sequence != want {
			t.Fatalf("sequence=%d want=%d", op.Sequence, want)
		}
	}

	if _, err := svc.Transition(ctx, p.ID, governance.ProposalSubmitted); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddOperation(ctx, governance.AddOperationParams{
		ProposalID: p.ID, OperationType: "delete_block", Payload: json.RawMessage(`{}`),
	}); !errors.Is(err, governance.ErrProposalNotDraft) {
		t.Fatalf("submitted 后追加 err=%v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE proposal_operation SET expected_hash='tampered' WHERE proposal_id=$1`, p.ID); err == nil {
		t.Fatal("数据库应拒绝篡改已提交 Proposal 的 Operation")
	}

	for _, next := range []string{governance.ProposalInReview, governance.ProposalApproved, governance.ProposalApplying, governance.ProposalApplied} {
		if _, err := svc.Transition(ctx, p.ID, next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	if _, err := svc.Transition(ctx, p.ID, governance.ProposalApproved); !errors.Is(err, governance.ErrInvalidTransition) {
		t.Fatalf("非法回退 err=%v", err)
	}
}

func TestProposalRejectsEmptySubmitAnonymousAndIdempotencyMismatch(t *testing.T) {
	svc, tdb := newService(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "reviewer")
	anon := tdb.MakeActor(t, "anonymous", "guest")

	if _, err := svc.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetPage, CreatedBy: anon, IdempotencyKey: "x",
	}); !errors.Is(err, governance.ErrActorNotAllowed) {
		t.Fatalf("anonymous err=%v", err)
	}
	p, err := svc.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetPage, CreatedBy: human, IdempotencyKey: "same",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Transition(ctx, p.ID, governance.ProposalSubmitted); !errors.Is(err, governance.ErrProposalHasNoOps) {
		t.Fatalf("empty submit err=%v", err)
	}
	if _, err := svc.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetEntity, CreatedBy: human, IdempotencyKey: "same",
	}); !errors.Is(err, governance.ErrIdempotencyMismatch) {
		t.Fatalf("mismatch err=%v", err)
	}
}

func lenMust(t *testing.T, svc *governance.Service, proposalID uuid.UUID) int {
	t.Helper()
	ops, err := svc.ListOperations(context.Background(), proposalID)
	if err != nil {
		t.Fatal(err)
	}
	return len(ops)
}
