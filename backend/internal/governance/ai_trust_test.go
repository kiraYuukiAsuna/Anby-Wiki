package governance

import (
	"encoding/binary"
	"testing"

	"github.com/google/uuid"
)

func TestSampledProposalUsesWholeUUIDv7(t *testing.T) {
	id := uuid.MustParse("019a25c0-0000-7000-8000-000000000000")
	sampled := 0
	for i := range uint16(1000) {
		binary.BigEndian.PutUint16(id[14:], i)
		first := sampledProposal(id, 25)
		if first != sampledProposal(id, 25) {
			t.Fatal("retry changed sample membership")
		}
		if sampledProposal(id, 0) || !sampledProposal(id, 100) {
			t.Fatal("sample percentage boundary violated")
		}
		if first {
			sampled++
		}
	}
	if sampled < 180 || sampled > 320 {
		t.Fatalf("same-timestamp UUIDs yielded %d/1000 samples at 25%%", sampled)
	}
}

func TestFullAccessAutoApprovesOnlyImportProposals(t *testing.T) {
	importJobID := uuid.New()
	proposal := &Proposal{ID: uuid.New(), ImportJobID: &importJobID}
	decision := &RiskDecision{Level: RiskMedium, AutoApprove: false}

	applyAITrustDecision(proposal, decision, AITrustFullAccess, 100)
	if !decision.AutoApprove || decision.SampledForReview ||
		decision.AITrustLevel != AITrustFullAccess ||
		decision.RequiredSamplePercent != 0 {
		t.Fatalf("full access import decision=%+v", decision)
	}

	proposal.ImportJobID = nil
	decision = &RiskDecision{Level: RiskLow, AutoApprove: true}
	applyAITrustDecision(proposal, decision, AITrustFullAccess, 0)
	if decision.AutoApprove || decision.SampledForReview {
		t.Fatalf("full access non-import bypassed review: %+v", decision)
	}
}
