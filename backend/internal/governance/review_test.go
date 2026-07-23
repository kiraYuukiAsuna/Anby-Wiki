package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestRiskReview_HumanVerifiedClaimAlwaysManual(t *testing.T) {
	_, knowledgeService, tdb := newKnowledgePatchEngine(t)
	ctx := context.Background()
	human := tdb.MakeActor(t, "human", "reviewer")
	ai := tdb.MakeActor(t, "ai", "agent")
	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "software", CanonicalKey: "review-target",
		Labels:  []knowledge.LabelInput{{Language: "zh-Hans", Label: "审核目标", IsPrimary: true}},
		ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := knowledgeService.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: entity.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2026-07-22"), OriginType: knowledge.OriginHuman, ActorID: human,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := knowledgeService.UpdateVerificationStatus(ctx, knowledge.UpdateVerificationStatusParams{
		ClaimID: claim.ID, Status: knowledge.VerificationHumanVerified, ActorID: human,
	}); err != nil {
		t.Fatal(err)
	}

	repo := governance.NewRepository(tdb.Pool)
	txm := db.NewTxManager(tdb.Pool)
	ids := id.NewGenerator()
	proposals := governance.NewService(repo, txm, ids)
	reviews := governance.NewReviewService(repo, txm, ids, governance.NewRiskEvaluator(knowledgeService))
	p, err := proposals.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetClaim, TargetID: &claim.ID,
		BaseStateVersion: ptrInt(1), CreatedBy: ai, IdempotencyKey: "human-verified",
	})
	if err != nil {
		t.Fatal(err)
	}
	op := governance.OperationV1{
		SchemaVersion: 1, OperationType: governance.OpSupersedeClaim,
		Base:     governance.OperationBase{StateVersion: ptrInt(1)},
		Target:   governance.OperationTarget{ClaimID: &claim.ID},
		Evidence: []governance.OperationEvidence{{Note: "new source"}},
		Risk:     governance.OperationRisk{Level: governance.RiskLow, Reasons: []string{}},
		Payload: mustJSON(t, map[string]any{
			"subject_entity_id": entity.ID, "property_key": "release_date",
			"value": map[string]any{"date": "2026-07-23"}, "origin_type": "human",
		}),
	}
	addContractOperation(t, proposals, p.ID, op)

	result, err := reviews.Submit(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposal.Status != governance.ProposalInReview || result.Proposal.RiskLevel != governance.RiskHigh ||
		result.ReviewTask == nil || result.Decision.AutoApprove {
		t.Fatalf("result=%+v decision=%+v", result.Proposal, result.Decision)
	}
	if _, err := reviews.Decide(ctx, result.ReviewTask.ID, ai, true, ""); !errors.Is(err, governance.ErrActorNotAllowed) {
		t.Fatalf("AI review err=%v", err)
	}
	approved, err := reviews.Decide(ctx, result.ReviewTask.ID, human, true, "证据充分")
	if err != nil || approved.Status != governance.ProposalApproved {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}
}

func TestRiskReview_UnambiguousPageRetargetCanAutoApprove(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	ai := tdb.MakeActor(t, "ai", "resolver")
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "source", "Source", testkit.SystemActorID)
	targetID := tdb.MakePage(t, testkit.MainNamespaceID, "target", "Target", testkit.SystemActorID)
	repo := governance.NewRepository(tdb.Pool)
	txm := db.NewTxManager(tdb.Pool)
	ids := id.NewGenerator()
	proposals := governance.NewService(repo, txm, ids)
	reviews := governance.NewReviewService(repo, txm, ids, governance.NewRiskEvaluator(nil))
	p, err := proposals.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetPage, TargetID: &pageID,
		BaseStateVersion: ptrInt(0), CreatedBy: ai, IdempotencyKey: "resolver-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	blockID := "00000000-0000-7000-8000-000000000711"
	nodeID := "0"
	op := governance.OperationV1{
		SchemaVersion: 1, OperationType: governance.OpRetargetPageReference,
		Base:     governance.OperationBase{StateVersion: ptrInt(0)},
		Target:   governance.OperationTarget{PageID: &pageID, BlockID: &blockID, NodeID: &nodeID},
		Evidence: []governance.OperationEvidence{{Note: "唯一标题候选"}},
		Risk:     governance.OperationRisk{Level: governance.RiskLow, Reasons: []string{}},
		Payload:  mustJSON(t, map[string]any{"target_page_id": targetID, "display_text": "Target"}),
	}
	addContractOperation(t, proposals, p.ID, op)
	result, err := reviews.Submit(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposal.Status != governance.ProposalApproved || result.ReviewTask != nil || !result.Decision.AutoApprove {
		t.Fatalf("result=%+v decision=%+v", result.Proposal, result.Decision)
	}
}

func addContractOperation(t *testing.T, svc *governance.Service, proposalID uuid.UUID, op governance.OperationV1) {
	t.Helper()
	raw, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddOperationV1(context.Background(), proposalID, raw); err != nil {
		t.Fatal(err)
	}
}

func ptrInt(v int) *int { return &v }
