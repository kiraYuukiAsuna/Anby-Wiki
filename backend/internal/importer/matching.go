package importer

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/knowledge"
)

const (
	EntityMatched   = "matched"
	EntityAmbiguous = "ambiguous"
	EntityNewReview = "new_review"
)

type EntityAlternative struct {
	EntityID  uuid.UUID `json:"entity_id"`
	Score     float64   `json:"score"`
	MatchedOn string    `json:"matched_on"`
}

// EntityResolution is an explainable match result. Ambiguous and new entities
// deliberately have no EntityID: downstream code must route them to review.
type EntityResolution struct {
	CandidateID  uuid.UUID           `json:"candidate_id"`
	Outcome      string              `json:"outcome"`
	EntityID     *uuid.UUID          `json:"entity_id,omitempty"`
	Score        float64             `json:"score"`
	Reason       string              `json:"reason"`
	Alternatives []EntityAlternative `json:"alternatives"`
}

type EntityMatcher struct {
	knowledge *knowledge.Service
}

func NewEntityMatcher(service *knowledge.Service) *EntityMatcher {
	return &EntityMatcher{knowledge: service}
}

// Match uses type-filtered labels/aliases and an optional page binding signal.
// A close second candidate is always reported as ambiguous; this method never
// creates or merges an Entity.
func (m *EntityMatcher) Match(ctx context.Context, wikiID uuid.UUID, pageID *uuid.UUID, candidates []EntityCandidate) ([]EntityResolution, error) {
	results := make([]EntityResolution, 0, len(candidates))
	for _, candidate := range candidates {
		scores := make(map[uuid.UUID]EntityAlternative)
		queries := append([]string{candidate.Label}, candidate.Aliases...)
		for queryIndex, query := range queries {
			found, err := m.knowledge.SearchEntities(ctx, knowledge.SearchParams{
				WikiID: wikiID, Query: query, TypeKey: candidate.TypeKey, Limit: 20,
			})
			if err != nil {
				return nil, fmt.Errorf("importer: 匹配实体候选 %s: %w", candidate.CandidateID, err)
			}
			for _, hit := range found {
				score := entityMatchScore(hit, queryIndex == 0)
				if pageID != nil {
					bound, err := m.knowledge.HasPageBinding(ctx, *pageID, hit.Entity.ID)
					if err != nil {
						return nil, err
					}
					if bound {
						score += 0.12
					}
				}
				if score > 1 {
					score = 1
				}
				old, ok := scores[hit.Entity.ID]
				if !ok || score > old.Score {
					scores[hit.Entity.ID] = EntityAlternative{EntityID: hit.Entity.ID, Score: score, MatchedOn: hit.MatchedOn}
				}
			}
		}
		alternatives := make([]EntityAlternative, 0, len(scores))
		for _, alternative := range scores {
			alternatives = append(alternatives, alternative)
		}
		sort.Slice(alternatives, func(i, j int) bool {
			if alternatives[i].Score == alternatives[j].Score {
				return alternatives[i].EntityID.String() < alternatives[j].EntityID.String()
			}
			return alternatives[i].Score > alternatives[j].Score
		})
		resolution := EntityResolution{CandidateID: candidate.CandidateID, Outcome: EntityNewReview,
			Reason: "no sufficiently strong typed label or alias match", Alternatives: alternatives}
		if len(alternatives) > 0 {
			resolution.Score = alternatives[0].Score
		}
		if len(alternatives) > 1 && alternatives[0].Score >= 0.78 && alternatives[0].Score-alternatives[1].Score < 0.08 {
			resolution.Outcome = EntityAmbiguous
			resolution.Reason = "top candidates are too close; human resolution required"
		} else if len(alternatives) > 0 && alternatives[0].Score >= 0.78 {
			entityID := alternatives[0].EntityID
			resolution.Outcome = EntityMatched
			resolution.EntityID = &entityID
			resolution.Reason = "typed label/alias match passed confidence and ambiguity thresholds"
		}
		results = append(results, resolution)
	}
	return results, nil
}

func entityMatchScore(hit knowledge.SearchResult, primaryQuery bool) float64 {
	if hit.Exact {
		switch hit.MatchedOn {
		case knowledge.MatchedOnCanonical, knowledge.MatchedOnLabel:
			if primaryQuery {
				return 0.88
			}
			return 0.80
		case knowledge.MatchedOnAlias:
			if primaryQuery {
				return 0.84
			}
			return 0.80
		}
	}
	if primaryQuery {
		return 0.58
	}
	return 0.50
}
