package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/page"
)

type generatedPlanEvidence struct {
	ChunkID   uuid.UUID `json:"chunk_id"`
	Quotation string    `json:"quotation"`
}

type generatedPlannedBlock struct {
	Type           string                  `json:"type"`
	Text           string                  `json:"text,omitempty"`
	Level          int                     `json:"level,omitempty"`
	Items          []string                `json:"items,omitempty"`
	ReplaceBlockID *string                 `json:"replace_block_id,omitempty"`
	Evidence       []generatedPlanEvidence `json:"evidence"`
}

type generatedPageRoute struct {
	Action     string                  `json:"action"`
	Title      string                  `json:"title"`
	PageID     *uuid.UUID              `json:"page_id,omitempty"`
	Reason     string                  `json:"reason"`
	Confidence float64                 `json:"confidence"`
	RelatedTo  []string                `json:"related_to,omitempty"`
	Evidence   []generatedPlanEvidence `json:"evidence,omitempty"`
	Blocks     []generatedPlannedBlock `json:"blocks,omitempty"`
}

type generatedImportPlanDocument struct {
	Profile                 SourceProfile        `json:"profile"`
	Routes                  []generatedPageRoute `json:"routes"`
	PromptInjectionDetected bool                 `json:"prompt_injection_detected"`
}

// DecodeGeneratedImportPlan converts the compact model contract into the
// canonical persisted contract. Mechanical identity, collection defaults,
// evidence offsets, locator pages, block modes, and score are server-owned.
// Invalid individual routes/blocks are isolated; unsupported evidence is never
// broadened or invented.
func DecodeGeneratedImportPlan(raw []byte, sourceVersionID uuid.UUID, chunks []evidence.SourceChunk,
	candidates []PageCandidate, routeMode string) (*ImportPlan, json.RawMessage, error) {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: import plan JSON: %v", ai.ErrInvalidOutput, err)
	}
	if err := compiledImportPlanGenerationSchema.Validate(instance); err != nil {
		return nil, nil, fmt.Errorf("%w: import plan schema: %v", ai.ErrInvalidOutput, err)
	}
	var generated generatedImportPlanDocument
	if err := json.Unmarshal(raw, &generated); err != nil {
		return nil, nil, fmt.Errorf("%w: import plan document: %v", ai.ErrInvalidOutput, err)
	}

	plan := &ImportPlan{
		SchemaVersion:           1,
		SourceVersionID:         sourceVersionID,
		Profile:                 generated.Profile,
		Routes:                  []PageRoute{},
		QualityScore:            1,
		PromptInjectionDetected: generated.PromptInjectionDetected,
	}
	plan.Profile.Title = strings.TrimSpace(plan.Profile.Title)
	plan.Profile.Summary = strings.TrimSpace(plan.Profile.Summary)
	plan.Profile.Language = strings.TrimSpace(plan.Profile.Language)
	plan.Profile.Subjects = normalizedGeneratedSubjects(plan.Profile.Subjects)

	candidateByID := make(map[uuid.UUID]PageCandidate, len(candidates))
	blockOwner := map[string]uuid.UUID{}
	for _, candidate := range candidates {
		candidateByID[candidate.PageID] = candidate
		for _, block := range candidate.Blocks {
			blockOwner[block.ID] = candidate.PageID
		}
	}
	catalog := newEvidenceCatalog(sourceVersionID, chunks)
	originalBlocks, retainedBlocks := 0, 0
	for _, sourceRoute := range generated.Routes {
		route := PageRoute{
			Action: sourceRoute.Action, Title: strings.TrimSpace(sourceRoute.Title),
			PageID: sourceRoute.PageID, Reason: strings.TrimSpace(sourceRoute.Reason),
			Confidence: clampUnit(sourceRoute.Confidence), RelatedTo: []string{},
			Evidence: []CandidateEvidence{}, Blocks: []PlannedBlock{},
		}
		if route.Title == "" || route.Reason == "" {
			continue
		}
		if routeMode == RouteModeForceCreate && route.Action != RouteCreate && route.Action != RouteIgnore {
			continue
		}
		switch route.Action {
		case RouteCreate:
			route.PageID = nil
		case RouteUpdate, RouteLink:
			if route.PageID == nil {
				continue
			}
			candidate, ok := candidateByID[*route.PageID]
			if !ok {
				continue
			}
			route.Title = candidate.Title
		case RouteIgnore:
			route.PageID = nil
		default:
			continue
		}

		if route.Action == RouteLink {
			route.RelatedTo = normalizedRelatedTitles(sourceRoute.RelatedTo)
			route.Evidence = catalog.normalize(generatedEvidenceCandidates(sourceRoute.Evidence))
			if len(route.RelatedTo) == 0 || len(route.Evidence) == 0 {
				continue
			}
			plan.Routes = append(plan.Routes, route)
			continue
		}
		if route.Action == RouteIgnore {
			plan.Routes = append(plan.Routes, route)
			continue
		}

		for _, sourceBlock := range sourceRoute.Blocks {
			originalBlocks++
			block := PlannedBlock{
				Type: sourceBlock.Type, Mode: BlockAppend, TargetBlockID: nil,
				Text: strings.TrimSpace(sourceBlock.Text), Level: sourceBlock.Level,
				Items:    normalizedGeneratedItems(sourceBlock.Items),
				Evidence: catalog.normalize(generatedEvidenceCandidates(sourceBlock.Evidence)),
			}
			if len(block.Evidence) == 0 {
				continue
			}
			if route.Action == RouteUpdate && sourceBlock.ReplaceBlockID != nil &&
				blockOwner[*sourceBlock.ReplaceBlockID] == *route.PageID {
				block.Mode = BlockReplace
				block.TargetBlockID = sourceBlock.ReplaceBlockID
			}
			if !validPlannedBlockContent(block) {
				continue
			}
			retainedBlocks++
			route.Blocks = append(route.Blocks, block)
		}
		if len(route.Blocks) == 0 {
			continue
		}
		plan.Routes = append(plan.Routes, route)
	}

	if originalBlocks > 0 && retainedBlocks == 0 && plan.Profile.Useful {
		return nil, nil, ErrEvidenceRequired
	}
	if originalBlocks > 0 {
		plan.QualityScore = float64(retainedBlocks) / float64(originalBlocks)
	}
	normalizeImportPlanCollections(plan)
	canonical, err := json.Marshal(plan)
	if err != nil {
		return nil, nil, err
	}
	return plan, canonical, nil
}

func generatedEvidenceCandidates(items []generatedPlanEvidence) []CandidateEvidence {
	result := make([]CandidateEvidence, 0, len(items))
	for _, item := range items {
		result = append(result, CandidateEvidence{
			ChunkID: item.ChunkID, Quotation: item.Quotation, CharStart: -1, CharEnd: -1,
		})
	}
	return result
}

func normalizedGeneratedSubjects(items []SourceSubject) []SourceSubject {
	result := make([]SourceSubject, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item.Title = strings.TrimSpace(item.Title)
		item.Kind = strings.TrimSpace(item.Kind)
		item.Summary = strings.TrimSpace(item.Summary)
		key := strings.ToLower(item.Kind + "\x00" + item.Title)
		if item.Title == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func normalizedGeneratedItems(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func hasForcedCreateRoute(routes []PageRoute, preferredTitle string) bool {
	wanted, err := page.NormalizeTitle(preferredTitle)
	if err != nil {
		return false
	}
	for index := range routes {
		if routes[index].Action != RouteCreate {
			continue
		}
		got, err := page.NormalizeTitle(routes[index].Title)
		if err == nil && got == wanted {
			routes[index].Title = strings.TrimSpace(preferredTitle)
			return true
		}
	}
	return false
}
