package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
)

type ProposalComposer struct {
	evidence   *evidence.Service
	governance *governance.Service
	knowledge  *knowledge.Service
}

func NewProposalComposer(evidenceService *evidence.Service, governanceService *governance.Service,
	knowledgeService *knowledge.Service) *ProposalComposer {
	return &ProposalComposer{evidence: evidenceService, governance: governanceService, knowledge: knowledgeService}
}

type ComposeParams struct {
	ImportJobID     uuid.UUID
	WikiID          uuid.UUID
	SourceVersionID uuid.UUID
	ActorID         uuid.UUID
	Candidates      *Candidates
	Resolutions     []EntityResolution
	Decisions       []ClaimDecision
}

type ComposeResult struct {
	Proposals  []governance.Proposal `json:"proposals"`
	Unresolved []EntityResolution    `json:"unresolved"`
}

// Compose creates reviewable governance drafts only. Every Claim mutation is
// backed by immutable Citations; unresolved ambiguous entities produce no write
// operation, and genuinely new entities get a separate review proposal.
func (c *ProposalComposer) Compose(ctx context.Context, params ComposeParams) (*ComposeResult, error) {
	if params.Candidates == nil || params.ImportJobID == uuid.Nil || params.SourceVersionID == uuid.Nil || params.ActorID == uuid.Nil {
		return nil, ErrInvalidJob
	}
	result := &ComposeResult{Proposals: []governance.Proposal{}, Unresolved: []EntityResolution{}}
	entities := make(map[uuid.UUID]EntityCandidate, len(params.Candidates.Entities))
	for _, candidate := range params.Candidates.Entities {
		entities[candidate.CandidateID] = candidate
	}
	resolutionByCandidate := make(map[uuid.UUID]EntityResolution, len(params.Resolutions))
	for _, resolution := range params.Resolutions {
		resolutionByCandidate[resolution.CandidateID] = resolution
		if resolution.Outcome == EntityAmbiguous {
			result.Unresolved = append(result.Unresolved, resolution)
		}
		if resolution.Outcome == EntityNewReview {
			candidate, ok := entities[resolution.CandidateID]
			if !ok {
				return nil, fmt.Errorf("importer: entity resolution references unknown candidate %s", resolution.CandidateID)
			}
			proposal, err := c.composeNewEntity(ctx, params, candidate)
			if err != nil {
				return nil, err
			}
			result.Proposals = append(result.Proposals, *proposal)
			result.Unresolved = append(result.Unresolved, resolution)
		}
	}

	claims := make(map[uuid.UUID]ClaimCandidate, len(params.Candidates.Claims))
	for _, candidate := range params.Candidates.Claims {
		claims[candidate.CandidateID] = candidate
	}
	bySubject := make(map[uuid.UUID][]ClaimDecision)
	for _, decision := range params.Decisions {
		bySubject[decision.SubjectEntityID] = append(bySubject[decision.SubjectEntityID], decision)
	}
	subjects := make([]uuid.UUID, 0, len(bySubject))
	for subjectID := range bySubject {
		subjects = append(subjects, subjectID)
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].String() < subjects[j].String() })
	for _, subjectID := range subjects {
		proposal, err := c.composeClaims(ctx, params, subjectID, bySubject[subjectID], claims)
		if err != nil {
			return nil, err
		}
		result.Proposals = append(result.Proposals, *proposal)
	}
	return result, nil
}

func (c *ProposalComposer) composeNewEntity(ctx context.Context, params ComposeParams, candidate EntityCandidate) (*governance.Proposal, error) {
	baseVersion := 0
	proposal, err := c.governance.CreateProposal(ctx, governance.CreateProposalParams{
		ImportJobID: &params.ImportJobID, TargetType: governance.TargetEntity,
		BaseStateVersion: &baseVersion, RiskLevel: governance.RiskMedium, CreatedBy: params.ActorID,
		IdempotencyKey: fmt.Sprintf("import:%s:entity-candidate:%s", params.ImportJobID, candidate.CandidateID),
	})
	if err != nil {
		return nil, err
	}
	existing, err := c.governance.ListOperations(ctx, proposal.ID)
	if err != nil || len(existing) > 0 {
		return proposal, err
	}
	labels := []map[string]any{{"language": "und", "label": candidate.Label, "is_primary": true}}
	for _, alias := range candidate.Aliases {
		if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(candidate.Label)) {
			continue
		}
		labels = append(labels, map[string]any{"language": "und", "label": alias, "is_primary": false})
	}
	evidenceItems := candidateEvidence(candidate.Evidence, nil)
	payload, _ := json.Marshal(map[string]any{"type_key": candidate.TypeKey, "canonical_key": candidate.Label, "labels": labels})
	op := governance.OperationV1{SchemaVersion: 1, OperationType: governance.OpCreateEntity,
		Base: governance.OperationBase{StateVersion: &baseVersion}, Target: governance.OperationTarget{WikiID: &params.WikiID},
		Evidence: evidenceItems, Risk: governance.OperationRisk{Level: governance.RiskMedium,
			Reasons: []string{"import candidate requires human entity review"}}, Payload: payload}
	return proposal, c.addOperation(ctx, proposal.ID, op)
}

func (c *ProposalComposer) composeClaims(ctx context.Context, params ComposeParams, subjectID uuid.UUID,
	decisions []ClaimDecision, candidates map[uuid.UUID]ClaimCandidate) (*governance.Proposal, error) {
	baseVersion := 0
	risk := RiskLow
	for _, decision := range decisions {
		risk = maxImportRisk(risk, decision.Risk)
	}
	proposal, err := c.governance.CreateProposal(ctx, governance.CreateProposalParams{
		ImportJobID: &params.ImportJobID, TargetType: governance.TargetEntity, TargetID: &subjectID,
		BaseStateVersion: &baseVersion, RiskLevel: risk, CreatedBy: params.ActorID,
		IdempotencyKey: fmt.Sprintf("import:%s:entity:%s:source-version:%s", params.ImportJobID, subjectID, params.SourceVersionID),
	})
	if err != nil {
		return nil, err
	}
	existing, err := c.governance.ListOperations(ctx, proposal.ID)
	if err != nil || len(existing) > 0 {
		return proposal, err
	}
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].CandidateID.String() < decisions[j].CandidateID.String() })
	for _, decision := range decisions {
		candidate, ok := candidates[decision.CandidateID]
		if !ok {
			return nil, fmt.Errorf("importer: claim decision references unknown candidate %s", decision.CandidateID)
		}
		citationIDs, opEvidence, err := c.createCitations(ctx, params, candidate.Evidence)
		if err != nil {
			return nil, err
		}
		var expectedHash *string
		if decision.ExistingClaimID != nil {
			existingClaim, err := c.knowledge.GetClaim(ctx, *decision.ExistingClaimID)
			if err != nil {
				return nil, err
			}
			hash, err := governance.ClaimHash(existingClaim)
			if err != nil {
				return nil, err
			}
			expectedHash = &hash
		}
		op, err := claimOperation(baseVersion, candidate, decision, citationIDs, opEvidence, expectedHash)
		if err != nil {
			return nil, err
		}
		if err := c.addOperation(ctx, proposal.ID, op); err != nil {
			return nil, err
		}
		if decision.Outcome == ClaimSupport {
			for i := 1; i < len(citationIDs); i++ {
				extra := op
				extra.Target.CitationID = &citationIDs[i]
				extra.Evidence = []governance.OperationEvidence{opEvidence[i]}
				if err := c.addOperation(ctx, proposal.ID, extra); err != nil {
					return nil, err
				}
			}
		}
	}
	return proposal, nil
}

func (c *ProposalComposer) createCitations(ctx context.Context, params ComposeParams, items []CandidateEvidence) ([]uuid.UUID, []governance.OperationEvidence, error) {
	ids := make([]uuid.UUID, 0, len(items))
	out := make([]governance.OperationEvidence, 0, len(items))
	for _, item := range items {
		start, end := int32(item.CharStart), int32(item.CharEnd)
		locator := &evidence.Locator{CharStart: &start, CharEnd: &end}
		if item.Page != nil {
			page := int32(*item.Page)
			locator.Page = &page
		}
		citation, err := c.evidence.CreateCitation(ctx, evidence.CreateCitationParams{
			SourceVersionID: params.SourceVersionID, SourceChunkID: &item.ChunkID,
			Locator: locator, Quotation: item.Quotation, ActorID: params.ActorID,
		})
		if err != nil {
			return nil, nil, err
		}
		ids = append(ids, citation.ID)
		out = append(out, governance.OperationEvidence{CitationID: &citation.ID, SourceChunkID: &item.ChunkID})
	}
	return ids, out, nil
}

func claimOperation(baseVersion int, candidate ClaimCandidate, decision ClaimDecision, citationIDs []uuid.UUID,
	evidenceItems []governance.OperationEvidence, expectedHash *string) (governance.OperationV1, error) {
	riskReasons := []string{decision.Reason}
	payload := map[string]any{"property_key": candidate.PropertyKey, "value": json.RawMessage(candidate.Value),
		"origin_type": knowledge.OriginImport, "citation_ids": citationIDs}
	if candidate.ValidFrom != nil {
		payload["valid_from"] = candidate.ValidFrom
	}
	if candidate.ValidTo != nil {
		payload["valid_to"] = candidate.ValidTo
	}
	target := governance.OperationTarget{EntityID: &decision.SubjectEntityID}
	opType := governance.OpCreateClaim
	switch decision.Outcome {
	case ClaimNew, ClaimContradiction:
	case ClaimSupersede:
		if decision.ExistingClaimID == nil {
			return governance.OperationV1{}, ErrInvalidJob
		}
		opType = governance.OpSupersedeClaim
		target = governance.OperationTarget{ClaimID: decision.ExistingClaimID}
		payload["subject_entity_id"] = decision.SubjectEntityID
	case ClaimSupport:
		if decision.ExistingClaimID == nil || len(citationIDs) == 0 {
			return governance.OperationV1{}, ErrEvidenceRequired
		}
		// Existing Claim sources are independent operations, one per immutable citation.
		// The caller currently invokes this helper once, so multiple evidence items are
		// represented by the first operation and composed below by cloneSupportOps.
		opType = governance.OpAddClaimSource
		target = governance.OperationTarget{ClaimID: decision.ExistingClaimID, CitationID: &citationIDs[0]}
		payload = map[string]any{"support_type": knowledge.SupportTypeSupports}
	default:
		return governance.OperationV1{}, ErrInvalidJob
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return governance.OperationV1{}, err
	}
	return governance.OperationV1{SchemaVersion: 1, OperationType: opType,
		Base: governance.OperationBase{StateVersion: &baseVersion}, Target: target, ExpectedHash: expectedHash, Evidence: evidenceItems,
		Risk: governance.OperationRisk{Level: decision.Risk, Reasons: riskReasons}, Payload: payloadJSON}, nil
}

func (c *ProposalComposer) addOperation(ctx context.Context, proposalID uuid.UUID, op governance.OperationV1) error {
	raw, err := json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = c.governance.AddOperationV1(ctx, proposalID, raw)
	return err
}

func candidateEvidence(items []CandidateEvidence, citationIDs []uuid.UUID) []governance.OperationEvidence {
	out := make([]governance.OperationEvidence, 0, len(items))
	for i, item := range items {
		e := governance.OperationEvidence{SourceChunkID: &item.ChunkID}
		if i < len(citationIDs) {
			e.CitationID = &citationIDs[i]
		}
		out = append(out, e)
	}
	return out
}

func maxImportRisk(a, b string) string {
	rank := map[string]int{RiskLow: 0, RiskMedium: 1, RiskHigh: 2, governance.RiskCritical: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
