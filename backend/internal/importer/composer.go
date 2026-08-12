package importer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type ProposalComposer struct {
	evidence   *evidence.Service
	governance *governance.Service
	knowledge  *knowledge.Service
	pages      *page.Service
	ids        *id.Generator
}

func NewProposalComposer(evidenceService *evidence.Service, governanceService *governance.Service,
	knowledgeService *knowledge.Service, pageService *page.Service, ids *id.Generator) *ProposalComposer {
	return &ProposalComposer{evidence: evidenceService, governance: governanceService,
		knowledge: knowledgeService, pages: pageService, ids: ids}
}

type ComposeParams struct {
	ImportJobID     uuid.UUID
	WikiID          uuid.UUID
	SourceVersionID uuid.UUID
	ActorID         uuid.UUID
	Candidates      *Candidates
	Plan            *ImportPlan
	Resolutions     []EntityResolution
	Decisions       []ClaimDecision
}

type ComposeResult struct {
	Proposals  []governance.Proposal `json:"proposals"`
	Unresolved []EntityResolution    `json:"unresolved"`
}

// Compose creates one reviewable, dependency-ordered Proposal for the complete
// import. New Entity candidates use their immutable server-issued candidate ID
// as the final Entity ID; all CreateEntity operations precede Claim operations,
// allowing the entire import to apply as one atomic ChangeBatch.
func (c *ProposalComposer) Compose(ctx context.Context, params ComposeParams) (*ComposeResult, error) {
	if c == nil || c.evidence == nil || c.governance == nil || c.knowledge == nil || c.pages == nil || c.ids == nil ||
		params.Candidates == nil || params.Plan == nil || params.ImportJobID == uuid.Nil ||
		params.SourceVersionID == uuid.Nil || params.WikiID == uuid.Nil || params.ActorID == uuid.Nil {
		return nil, ErrInvalidJob
	}
	result := &ComposeResult{Proposals: []governance.Proposal{}, Unresolved: []EntityResolution{}}
	entities := make(map[uuid.UUID]EntityCandidate, len(params.Candidates.Entities))
	for _, candidate := range params.Candidates.Entities {
		entities[candidate.CandidateID] = candidate
	}
	newEntities := make([]EntityResolution, 0)
	for _, resolution := range params.Resolutions {
		if resolution.Outcome == EntityAmbiguous {
			result.Unresolved = append(result.Unresolved, resolution)
		}
		if resolution.Outcome == EntityNewReview {
			if _, ok := entities[resolution.CandidateID]; !ok || resolution.PlannedEntityID == nil {
				return nil, fmt.Errorf("importer: entity resolution references unknown candidate %s", resolution.CandidateID)
			}
			newEntities = append(newEntities, resolution)
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
	pageRouteCount := actionablePageRouteCount(params.Plan.Routes)
	pageWriteCount := pageRouteCount
	if pageRouteCount > 0 && linkedPageRouteCount(params.Plan.Routes) > 0 {
		pageWriteCount++
	}
	if len(newEntities) == 0 && len(params.Decisions) == 0 && pageWriteCount == 0 {
		return result, nil
	}

	baseVersion := 0
	risk := RiskLow
	if len(newEntities) > 0 {
		risk = RiskMedium
	}
	if pageRouteCount > 0 {
		risk = maxImportRisk(risk, RiskMedium)
	}
	for _, decision := range params.Decisions {
		risk = maxImportRisk(risk, decision.Risk)
	}
	proposal, err := c.governance.CreateProposal(ctx, governance.CreateProposalParams{
		ImportJobID: &params.ImportJobID, TargetType: governance.TargetWiki, TargetID: &params.WikiID,
		BaseStateVersion: &baseVersion, RiskLevel: risk, CreatedBy: params.ActorID,
		IdempotencyKey: fmt.Sprintf("import:%s:composite:source-version:%s", params.ImportJobID, params.SourceVersionID),
	})
	if err != nil {
		return nil, err
	}
	existing, err := c.governance.ListOperations(ctx, proposal.ID)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		result.Proposals = append(result.Proposals, *proposal)
		return result, nil
	}

	sort.Slice(newEntities, func(i, j int) bool {
		return newEntities[i].CandidateID.String() < newEntities[j].CandidateID.String()
	})
	operations := make([]governance.OperationV1, 0, len(newEntities)+len(params.Decisions))
	for _, resolution := range newEntities {
		candidate := entities[resolution.CandidateID]
		op, err := newEntityOperation(params, candidate, *resolution.PlannedEntityID)
		if err != nil {
			return nil, err
		}
		operations = append(operations, op)
	}
	subjects := make([]uuid.UUID, 0, len(bySubject))
	for subjectID := range bySubject {
		subjects = append(subjects, subjectID)
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].String() < subjects[j].String() })
	for _, subjectID := range subjects {
		claimOps, err := c.buildClaimOperations(ctx, params, bySubject[subjectID], claims)
		if err != nil {
			return nil, err
		}
		operations = append(operations, claimOps...)
	}
	pageOperations, err := c.buildPageOperations(ctx, params)
	if err != nil {
		return nil, err
	}
	operations = append(operations, pageOperations...)
	rawOperations := make([][]byte, len(operations))
	for index := range operations {
		rawOperations[index], err = json.Marshal(operations[index])
		if err != nil {
			return nil, err
		}
	}
	if _, err := c.governance.AddOperationsV1(ctx, proposal.ID, rawOperations); err != nil {
		if !errors.Is(err, governance.ErrOperationSequenceRace) {
			return nil, err
		}
		// A concurrent retry may have frozen the same idempotent Proposal first.
		// Only accept the race when a complete non-empty set is now visible.
		frozen, listErr := c.governance.ListOperations(ctx, proposal.ID)
		if listErr != nil || len(frozen) == 0 {
			return nil, err
		}
	}
	result.Proposals = append(result.Proposals, *proposal)
	return result, nil
}

func newEntityOperation(params ComposeParams, candidate EntityCandidate, entityID uuid.UUID) (governance.OperationV1, error) {
	if entityID == uuid.Nil {
		return governance.OperationV1{}, ErrInvalidJob
	}
	baseVersion := 0
	labels := []map[string]any{{"language": "und", "label": candidate.Label, "is_primary": true}}
	for _, alias := range candidate.Aliases {
		if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(candidate.Label)) {
			continue
		}
		labels = append(labels, map[string]any{"language": "und", "label": alias, "is_primary": false})
	}
	evidenceItems := candidateEvidence(candidate.Evidence, nil)
	// canonical_key is unique across an entire Wiki while labels are allowed to
	// be shared by different Entity types. Prefixing the normalized type keeps
	// two independently reviewed same-label candidates from rolling back the
	// complete ChangeBatch at Apply time.
	canonicalKey := strings.TrimSpace(candidate.TypeKey) + ":" + strings.TrimSpace(candidate.Label)
	if utf8.RuneCountInString(canonicalKey) > 255 {
		sum := sha256.Sum256([]byte(canonicalKey))
		canonicalKey = strings.TrimSpace(candidate.TypeKey) + ":" + fmt.Sprintf("%x", sum[:])
	}
	payload, _ := json.Marshal(map[string]any{"type_key": candidate.TypeKey, "canonical_key": canonicalKey, "labels": labels})
	op := governance.OperationV1{SchemaVersion: 1, OperationType: governance.OpCreateEntity,
		Base:     governance.OperationBase{StateVersion: &baseVersion},
		Target:   governance.OperationTarget{WikiID: &params.WikiID, EntityID: &entityID},
		Evidence: evidenceItems, Risk: governance.OperationRisk{Level: governance.RiskMedium,
			Reasons: []string{"import candidate requires human entity review"}}, Payload: payload}
	return op, nil
}

func (c *ProposalComposer) buildClaimOperations(ctx context.Context, params ComposeParams,
	decisions []ClaimDecision, candidates map[uuid.UUID]ClaimCandidate) ([]governance.OperationV1, error) {
	baseVersion := 0
	operations := make([]governance.OperationV1, 0, len(decisions))
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
		op, err := claimOperation(params.WikiID, baseVersion, candidate, decision, citationIDs, opEvidence, expectedHash)
		if err != nil {
			return nil, err
		}
		operations = append(operations, op)
		if decision.Outcome == ClaimSupport {
			for i := 1; i < len(citationIDs); i++ {
				extra := op
				extra.Target.CitationID = &citationIDs[i]
				extra.Evidence = []governance.OperationEvidence{opEvidence[i]}
				operations = append(operations, extra)
			}
		}
	}
	return operations, nil
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

func claimOperation(wikiID uuid.UUID, baseVersion int, candidate ClaimCandidate, decision ClaimDecision, citationIDs []uuid.UUID,
	evidenceItems []governance.OperationEvidence, expectedHash *string) (governance.OperationV1, error) {
	riskReasons := []string{decision.Reason}
	value := decision.ResolvedValue
	if len(value) == 0 {
		return governance.OperationV1{}, ErrInvalidJob
	}
	payload := map[string]any{"property_key": candidate.PropertyKey, "value": json.RawMessage(value),
		"origin_type": knowledge.OriginImport, "citation_ids": citationIDs}
	if candidate.ValidFrom != nil {
		payload["valid_from"] = candidate.ValidFrom
	}
	if candidate.ValidTo != nil {
		payload["valid_to"] = candidate.ValidTo
	}
	target := governance.OperationTarget{WikiID: &wikiID, EntityID: &decision.SubjectEntityID}
	opType := governance.OpCreateClaim
	switch decision.Outcome {
	case ClaimNew, ClaimContradiction:
	case ClaimSupersede:
		if decision.ExistingClaimID == nil {
			return governance.OperationV1{}, ErrInvalidJob
		}
		opType = governance.OpSupersedeClaim
		target = governance.OperationTarget{WikiID: &wikiID, ClaimID: decision.ExistingClaimID}
		payload["subject_entity_id"] = decision.SubjectEntityID
	case ClaimSupport:
		if decision.ExistingClaimID == nil || len(citationIDs) == 0 {
			return governance.OperationV1{}, ErrEvidenceRequired
		}
		// Existing Claim sources are independent operations, one per immutable citation.
		// The caller currently invokes this helper once, so multiple evidence items are
		// represented by the first operation and composed below by cloneSupportOps.
		opType = governance.OpAddClaimSource
		target = governance.OperationTarget{WikiID: &wikiID, ClaimID: decision.ExistingClaimID, CitationID: &citationIDs[0]}
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
