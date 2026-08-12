package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/knowledge"
)

const (
	ClaimNew           = "new"
	ClaimSupport       = "support"
	ClaimContradiction = "contradiction"
	ClaimSupersede     = "supersede"
	RiskLow            = "low"
	RiskMedium         = "medium"
	RiskHigh           = "high"
)

type ClaimDecision struct {
	CandidateID     uuid.UUID  `json:"candidate_id"`
	SubjectEntityID uuid.UUID  `json:"subject_entity_id"`
	Outcome         string     `json:"outcome"`
	ExistingClaimID *uuid.UUID `json:"existing_claim_id,omitempty"`
	Risk            string     `json:"risk"`
	Reason          string     `json:"reason"`
	// ResolvedValue contains only authoritative or planned Entity IDs. It is a
	// compose-time value and is not part of the matching review DTO.
	ResolvedValue json.RawMessage `json:"-"`
}

type ClaimClassifier struct {
	knowledge ClaimKnowledge
}

type ClaimKnowledge interface {
	GetEntity(context.Context, uuid.UUID) (*knowledge.Entity, error)
	GetPropertyByKey(context.Context, string) (*knowledge.Property, error)
	ListClaims(context.Context, knowledge.ListClaimsParams) ([]knowledge.Claim, error)
}

func NewClaimClassifier(service ClaimKnowledge) *ClaimClassifier {
	return &ClaimClassifier{knowledge: service}
}

// Classify distinguishes exact support, a genuinely new fact, an overlapping
// multivalue contradiction, and replacement of a single-valued fact.
func (c *ClaimClassifier) Classify(ctx context.Context, candidates []ClaimCandidate, resolutions []EntityResolution) ([]ClaimDecision, error) {
	resolved := make(map[uuid.UUID]uuid.UUID)
	planned := make(map[uuid.UUID]bool)
	for _, resolution := range resolutions {
		if resolution.Outcome == EntityMatched && resolution.EntityID != nil {
			resolved[resolution.CandidateID] = *resolution.EntityID
		} else if resolution.Outcome == EntityNewReview && resolution.PlannedEntityID != nil {
			resolved[resolution.CandidateID] = *resolution.PlannedEntityID
			planned[*resolution.PlannedEntityID] = true
		}
	}
	decisions := make([]ClaimDecision, 0, len(candidates))
	knownExisting := make(map[uuid.UUID]bool)
	for _, rawCandidate := range candidates {
		candidate, ok := resolveClaimCandidateReferences(rawCandidate, resolved)
		if !ok {
			continue // ambiguous or missing local Entity dependency remains review-only
		}
		subjectID, ok := claimSubject(candidate.Subject, resolved)
		if !ok {
			continue // unresolved entity candidates remain visible in matching review
		}
		if !planned[subjectID] {
			exists, checked := knownExisting[subjectID]
			if !checked {
				_, err := c.knowledge.GetEntity(ctx, subjectID)
				if errors.Is(err, knowledge.ErrEntityNotFound) {
					knownExisting[subjectID] = false
					continue
				}
				if err != nil {
					return nil, err
				}
				knownExisting[subjectID] = true
				exists = true
			}
			if !exists {
				continue
			}
		}
		property, err := c.knowledge.GetPropertyByKey(ctx, candidate.PropertyKey)
		if errors.Is(err, knowledge.ErrPropertyNotFound) {
			continue // one unsupported model property must not poison the import
		}
		if err != nil {
			return nil, err
		}
		candidateValue, err := storedCandidateValue(property.ValueType, candidate.Value)
		if err != nil {
			continue // retain independently valid Entities and Claims from the batch
		}
		if property.ValueType == knowledge.ValueTypeEntity {
			targetID, ok := claimValueEntityID(candidate.Value)
			if !ok {
				continue
			}
			if !planned[targetID] {
				exists, checked := knownExisting[targetID]
				if !checked {
					_, err := c.knowledge.GetEntity(ctx, targetID)
					if errors.Is(err, knowledge.ErrEntityNotFound) {
						knownExisting[targetID] = false
						continue
					}
					if err != nil {
						return nil, err
					}
					knownExisting[targetID] = true
					exists = true
				}
				if !exists {
					continue
				}
			}
		}
		decision := ClaimDecision{CandidateID: candidate.CandidateID, SubjectEntityID: subjectID,
			Outcome: ClaimNew, Risk: RiskLow, Reason: "no active claim has this property",
			ResolvedValue: candidate.Value}
		if planned[subjectID] {
			decisions = append(decisions, decision)
			continue
		}
		claims, err := c.knowledge.ListClaims(ctx, knowledge.ListClaimsParams{
			SubjectEntityID: subjectID, PropertyKey: candidate.PropertyKey,
		})
		if err != nil {
			return nil, err
		}
		for i := range claims {
			existing := &claims[i]
			if !activeClaim(existing.Status) {
				continue
			}
			if canonicalJSONEqual(candidateValue, existing.ValueJSON) && sameTime(candidate.ValidFrom, existing.ValidFrom) && sameTime(candidate.ValidTo, existing.ValidTo) {
				claimID := existing.ID
				decision.Outcome = ClaimSupport
				decision.ExistingClaimID = &claimID
				decision.Reason = "same normalized value and validity interval already exists"
				break
			}
			if !timeRangesOverlap(candidate.ValidFrom, candidate.ValidTo, existing.ValidFrom, existing.ValidTo) {
				continue
			}
			claimID := existing.ID
			decision.ExistingClaimID = &claimID
			if property.IsMultivalued {
				decision.Outcome = ClaimContradiction
				decision.Risk = RiskMedium
				decision.Reason = "different value overlaps an active multivalued claim"
			} else {
				decision.Outcome = ClaimSupersede
				decision.Risk = RiskMedium
				decision.Reason = "single-valued property requires replacing the active claim"
			}
			if existing.VerificationStatus == knowledge.VerificationHumanVerified {
				decision.Risk = RiskHigh
				decision.Reason += "; existing claim is human-verified"
			}
			break
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func resolveClaimCandidateReferences(candidate ClaimCandidate, resolved map[uuid.UUID]uuid.UUID) (ClaimCandidate, bool) {
	if candidate.Subject.CandidateID != nil {
		entityID, ok := resolved[*candidate.Subject.CandidateID]
		if !ok {
			return ClaimCandidate{}, false
		}
		candidate.Subject = CandidateSubject{EntityID: &entityID}
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(candidate.Value, &value); err != nil {
		return ClaimCandidate{}, false
	}
	if rawID, ok := value["entity_candidate_id"]; ok {
		var candidateID uuid.UUID
		if err := json.Unmarshal(rawID, &candidateID); err != nil {
			return ClaimCandidate{}, false
		}
		entityID, ok := resolved[candidateID]
		if !ok {
			return ClaimCandidate{}, false
		}
		candidate.Value, _ = json.Marshal(map[string]uuid.UUID{"entity_id": entityID})
	}
	return candidate, true
}

func claimValueEntityID(raw json.RawMessage) (uuid.UUID, bool) {
	var value struct {
		EntityID uuid.UUID `json:"entity_id"`
	}
	if err := json.Unmarshal(raw, &value); err != nil || value.EntityID == uuid.Nil {
		return uuid.Nil, false
	}
	return value.EntityID, true
}

func claimSubject(subject CandidateSubject, resolved map[uuid.UUID]uuid.UUID) (uuid.UUID, bool) {
	if subject.EntityID != nil {
		return *subject.EntityID, true
	}
	if subject.CandidateID == nil {
		return uuid.Nil, false
	}
	id, ok := resolved[*subject.CandidateID]
	return id, ok
}

func storedCandidateValue(valueType string, raw json.RawMessage) (json.RawMessage, error) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	key := valueType
	if valueType == knowledge.ValueTypeEntity {
		key = "entity_id"
	}
	part, ok := value[key]
	if !ok {
		return nil, fmt.Errorf("missing %q member", key)
	}
	if valueType == knowledge.ValueTypeEntity {
		return json.Marshal(map[string]json.RawMessage{"entity_id": part})
	}
	return canonicalJSON(part)
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func canonicalJSONEqual(a, b json.RawMessage) bool {
	ca, errA := canonicalJSON(a)
	cb, errB := canonicalJSON(b)
	return errA == nil && errB == nil && bytes.Equal(ca, cb)
}

func activeClaim(status string) bool {
	return status == knowledge.ClaimStatusPublished || status == knowledge.ClaimStatusProposed
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func timeRangesOverlap(aFrom, aTo, bFrom, bTo *time.Time) bool {
	return (aTo == nil || bFrom == nil || aTo.After(*bFrom)) &&
		(bTo == nil || aFrom == nil || bTo.After(*aFrom))
}
