package importer

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/id"
)

const (
	ImportPlanSchemaURL = "https://anby.wiki/schemas/import-plan/v1/plan.schema.json"
	ImportPlanPromptKey = "source-import-plan-v2"
	// ImportPlanConsolidatePromptKey reduces independently grounded source
	// windows into one coherent article plan. It uses the same output contract
	// but receives only validated draft material, never raw source instructions.
	ImportPlanConsolidatePromptKey = "source-import-plan-consolidate-v1"
	planBatchConcurrency           = 3

	RouteCreate = "create"
	RouteUpdate = "update"
	RouteLink   = "link"
	RouteIgnore = "ignore"

	BlockAppend  = "append"
	BlockReplace = "replace"

	RouteModeAuto        = "auto"
	RouteModeForceCreate = "force_create"
)

var (
	ErrImportPlanNotFound = errors.New("importer: ImportPlan 不存在")
	ErrNoPagePlan         = errors.New("importer: 没有形成可审核页面计划")
	ErrPlanTargetConflict = errors.New("importer: 页面规划目标冲突")
)

//go:embed schema/import-plan.schema.json
var importPlanSchemaJSON []byte

var compiledImportPlanSchema = mustCompileImportPlanSchema()

func ImportPlanSchemaJSON() json.RawMessage {
	return append(json.RawMessage(nil), importPlanSchemaJSON...)
}

type SourceProfile struct {
	Title    string          `json:"title"`
	Summary  string          `json:"summary"`
	Language string          `json:"language"`
	Useful   bool            `json:"useful"`
	Subjects []SourceSubject `json:"subjects"`
}

type SourceSubject struct {
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

// PlannedBlock is a model-authored content intent. It deliberately carries
// prose and evidence rather than authoritative AST IDs. The composer assigns
// IDs and compiles it into Typed Block AST or block-level Proposal operations.
type PlannedBlock struct {
	Type          string              `json:"type"`
	Mode          string              `json:"mode"`
	TargetBlockID *string             `json:"target_block_id"`
	Text          string              `json:"text,omitempty"`
	Level         int                 `json:"level,omitempty"`
	Items         []string            `json:"items,omitempty"`
	Evidence      []CandidateEvidence `json:"evidence"`
}

type PageRoute struct {
	Action     string              `json:"action"`
	Title      string              `json:"title"`
	PageID     *uuid.UUID          `json:"page_id"`
	Reason     string              `json:"reason"`
	Confidence float64             `json:"confidence"`
	RelatedTo  []string            `json:"related_to"`
	Evidence   []CandidateEvidence `json:"evidence"`
	Blocks     []PlannedBlock      `json:"blocks"`
}

type ImportPlan struct {
	SchemaVersion           int           `json:"schema_version"`
	SourceVersionID         uuid.UUID     `json:"source_version_id"`
	Profile                 SourceProfile `json:"profile"`
	Routes                  []PageRoute   `json:"routes"`
	QualityScore            float64       `json:"quality_score"`
	PromptInjectionDetected bool          `json:"prompt_injection_detected"`
}

type ImportPlanRecord struct {
	ID              uuid.UUID
	ImportJobID     uuid.UUID
	SourceVersionID uuid.UUID
	InputHash       string
	SchemaVersion   int
	PromptKey       string
	PromptVersion   int
	Model           string
	PlanJSON        json.RawMessage
	QualityScore    float64
	CreatedAt       time.Time
}

type PageCandidateBlock struct {
	ID   string `json:"block_id"`
	Type string `json:"type"`
	Text string `json:"text"`
	Hash string `json:"hash"`
}

type PageCandidate struct {
	PageID            uuid.UUID            `json:"page_id"`
	Title             string               `json:"title"`
	MatchedOn         string               `json:"matched_on"`
	CurrentRevisionID *uuid.UUID           `json:"current_revision_id"`
	Blocks            []PageCandidateBlock `json:"blocks"`
}

type PagePlanningCatalog interface {
	SearchPages(context.Context, uuid.UUID, string, string, int) ([]page.PageSearchHit, error)
	ResolveTitle(context.Context, uuid.UUID, string, string) (*page.ResolvedPage, error)
	GetPage(context.Context, uuid.UUID) (*page.Page, error)
	CurrentContent(context.Context, uuid.UUID) (*page.Revision, *page.ContentSnapshot, error)
}

type PagePlanner struct {
	repo  *Repository
	pages PagePlanningCatalog
	ai    StructuredGenerator
	ids   *id.Generator
}

func NewPagePlanner(repo *Repository, pages PagePlanningCatalog, gateway StructuredGenerator, ids *id.Generator) *PagePlanner {
	return &PagePlanner{repo: repo, pages: pages, ai: gateway, ids: ids}
}

type PlanParams struct {
	SourceVersionID uuid.UUID
	SourceLabel     string
	PreferredTitle  string
	Instructions    string
	RouteMode       string
	TargetPageID    *uuid.UUID
	WikiID          uuid.UUID
	Chunks          []evidence.SourceChunk
	Candidates      *Candidates
	Provider        string
	Model           string
	MaxInputTokens  int
	InputHash       string
	ImportJobID     *uuid.UUID
	ImportRunID     *uuid.UUID
}

type PlanResult struct {
	Record *ImportPlanRecord
	Plan   *ImportPlan
	Reused bool
}

func (p *PagePlanner) Plan(ctx context.Context, params PlanParams) (*PlanResult, error) {
	if p == nil || p.repo == nil || p.pages == nil || p.ai == nil || p.ids == nil ||
		params.SourceVersionID == uuid.Nil || params.WikiID == uuid.Nil || params.ImportJobID == nil ||
		*params.ImportJobID == uuid.Nil || !validSHA256(params.InputHash) {
		return nil, ErrInvalidJob
	}
	if params.RouteMode == "" {
		params.RouteMode = RouteModeAuto
	}
	if params.RouteMode != RouteModeAuto && params.RouteMode != RouteModeForceCreate {
		return nil, ErrInvalidJob
	}
	if params.RouteMode == RouteModeForceCreate && strings.TrimSpace(params.PreferredTitle) == "" {
		return nil, ErrInvalidJob
	}
	if existing, err := p.repo.GetImportPlan(ctx, *params.ImportJobID, params.InputHash); err == nil {
		var plan ImportPlan
		if err := json.Unmarshal(existing.PlanJSON, &plan); err != nil {
			return nil, err
		}
		normalizeImportPlanCollections(&plan)
		return &PlanResult{Record: existing, Plan: &plan, Reused: true}, nil
	} else if !errors.Is(err, ErrImportPlanNotFound) {
		return nil, err
	}

	candidatePages, err := p.recallPages(ctx, params)
	if err != nil {
		return nil, err
	}
	generated, err := p.generatePlan(ctx, params, candidatePages)
	if err != nil {
		return nil, err
	}
	plan := generated.Plan
	if DetectPromptInjection(chunkTexts(params.Chunks)) {
		plan.PromptInjectionDetected = true
	}
	if plan.PromptInjectionDetected {
		return nil, ErrQualityGate
	}
	if err := p.resolveCreateCollisions(ctx, params, plan); err != nil {
		return nil, err
	}
	p.removeNoOpUpdates(plan, candidatePages)
	plan.Routes = mergePageRoutes(plan.Routes)
	plan.Routes = normalizePlanLinks(plan.Routes)
	refineImportPlan(plan)
	if plan.Profile.Useful && actionablePageRouteCount(plan.Routes) == 0 {
		return nil, ErrNoPagePlan
	}
	normalizeImportPlanCollections(plan)
	canonical, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	recordID, err := p.ids.New()
	if err != nil {
		return nil, err
	}
	record := &ImportPlanRecord{
		ID: recordID, ImportJobID: *params.ImportJobID, SourceVersionID: params.SourceVersionID, InputHash: params.InputHash,
		SchemaVersion: 1, PromptKey: generated.PromptKey, PromptVersion: generated.PromptVersion,
		Model: generated.Model, PlanJSON: canonical, QualityScore: plan.QualityScore,
	}
	inserted, err := p.repo.InsertImportPlanIfAbsent(ctx, record)
	if err != nil {
		return nil, err
	}
	if !inserted {
		record, err = p.repo.GetImportPlan(ctx, *params.ImportJobID, params.InputHash)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(record.PlanJSON, plan); err != nil {
			return nil, err
		}
		normalizeImportPlanCollections(plan)
	}
	return &PlanResult{Record: record, Plan: plan, Reused: !inserted}, nil
}

type generatedImportPlan struct {
	Plan          *ImportPlan
	PromptKey     string
	PromptVersion int
	Model         string
}

func (p *PagePlanner) generatePlan(ctx context.Context, params PlanParams, candidates []PageCandidate) (*generatedImportPlan, error) {
	batches := batchSourceChunks(params.Chunks, params.MaxInputTokens)
	type planBatchOutcome struct {
		plans  []*ImportPlan
		result *ai.Result
		err    error
	}
	outcomes := make([]planBatchOutcome, len(batches))
	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	limit := make(chan struct{}, planBatchConcurrency)
	var wait sync.WaitGroup
	for index := range batches {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-batchCtx.Done():
				outcomes[index].err = batchCtx.Err()
				return
			}
			outcomes[index].plans, outcomes[index].result, outcomes[index].err =
				p.generatePlanBatch(batchCtx, params, candidates, batches[index])
			if outcomes[index].err != nil {
				cancel()
			}
		}(index)
	}
	wait.Wait()

	parts := make([]*ImportPlan, 0, len(batches))
	var first *ai.Result
	var firstError error
	for index := range outcomes {
		outcome := outcomes[index]
		if outcome.err != nil {
			if firstError == nil && !errors.Is(outcome.err, context.Canceled) {
				firstError = outcome.err
			}
			continue
		}
		if outcome.result == nil {
			if firstError == nil {
				firstError = fmt.Errorf("%w: empty import plan batch", ai.ErrInvalidOutput)
			}
			continue
		}
		if first == nil {
			first = outcome.result
		} else if outcome.result.PromptKey != first.PromptKey || outcome.result.PromptVersion != first.PromptVersion ||
			outcome.result.Model != first.Model {
			return nil, fmt.Errorf("%w: inconsistent import plan batch metadata", ai.ErrInvalidOutput)
		}
		parts = append(parts, outcome.plans...)
	}
	if firstError != nil {
		return nil, firstError
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if first == nil || len(parts) == 0 {
		return nil, fmt.Errorf("%w: no import plan result", ai.ErrInvalidOutput)
	}
	merged := mergeImportPlanParts(params.SourceVersionID, parts)
	refineImportPlan(merged)
	if len(parts) > 1 {
		consolidated, result, err := p.consolidatePlan(ctx, params, candidates, merged)
		if err == nil && consolidated != nil && result != nil {
			merged = consolidated
			first = result
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// A reducer timeout or invalid structure must not throw away already
		// validated map results. The deterministic refinement above remains a
		// safe, evidence-grounded fallback.
	}
	refineImportPlan(merged)
	return &generatedImportPlan{Plan: merged, PromptKey: first.PromptKey,
		PromptVersion: first.PromptVersion, Model: first.Model}, nil
}

func (p *PagePlanner) consolidatePlan(ctx context.Context, params PlanParams, candidates []PageCandidate,
	draft *ImportPlan) (*ImportPlan, *ai.Result, error) {
	draftJSON, err := json.Marshal(draft)
	if err != nil {
		return nil, nil, err
	}
	maxInputTokens := params.MaxInputTokens
	if maxInputTokens <= 0 {
		maxInputTokens = DefaultModelMaxInputTokens
	}
	// The reducer has prompt/schema overhead and must still leave room for a
	// complete rewritten plan. A conservative byte/token estimate makes the
	// optional pass self-disabling for unusually large plans.
	budget := maxInputTokens - max(extractionInputMinReserve, maxInputTokens/5)
	if (len(draftJSON)+2)/3+4096 > budget {
		return nil, nil, nil
	}
	candidatesJSON, err := json.Marshal(candidates)
	if err != nil {
		return nil, nil, err
	}
	result, err := p.ai.Generate(ctx, ai.Request{
		Provider: params.Provider, Model: params.Model, PromptKey: ImportPlanConsolidatePromptKey,
		Variables: map[string]any{
			"source_version_id": params.SourceVersionID.String(),
			"source_label":      strings.TrimSpace(params.SourceLabel),
			"preferred_title":   strings.TrimSpace(params.PreferredTitle),
			"instructions":      strings.TrimSpace(params.Instructions),
			"route_mode":        params.RouteMode,
			"candidate_pages":   string(candidatesJSON),
			"draft_plan_json":   string(draftJSON),
		},
		ImportJobID: params.ImportJobID, ImportRunID: params.ImportRunID,
	})
	if err != nil {
		return nil, nil, err
	}
	plan, _, err := ValidateImportPlan(result.JSON, params.SourceVersionID, params.Chunks,
		candidates, params.RouteMode, params.PreferredTitle)
	if err != nil {
		return nil, nil, err
	}
	plan.PromptInjectionDetected = plan.PromptInjectionDetected || draft.PromptInjectionDetected
	return plan, result, nil
}

func (p *PagePlanner) generatePlanBatch(ctx context.Context, params PlanParams, candidates []PageCandidate,
	chunks []evidence.SourceChunk) ([]*ImportPlan, *ai.Result, error) {
	result, err := p.generatePlanOnce(ctx, params, candidates, chunks)
	if err != nil {
		if isAdaptiveBatchError(err) && len(chunks) > 1 {
			middle := len(chunks) / 2
			left, leftResult, leftErr := p.generatePlanBatch(ctx, params, candidates, chunks[:middle])
			if leftErr != nil {
				return nil, nil, leftErr
			}
			right, rightResult, rightErr := p.generatePlanBatch(ctx, params, candidates, chunks[middle:])
			if rightErr != nil {
				return nil, nil, rightErr
			}
			if leftResult.PromptKey != rightResult.PromptKey || leftResult.PromptVersion != rightResult.PromptVersion || leftResult.Model != rightResult.Model {
				return nil, nil, fmt.Errorf("%w: inconsistent split plan metadata", ai.ErrInvalidOutput)
			}
			return append(left, right...), leftResult, nil
		}
		return nil, nil, err
	}
	plan, _, err := ValidateImportPlan(result.JSON, params.SourceVersionID, chunks, candidates, params.RouteMode, params.PreferredTitle)
	if err != nil {
		if (errors.Is(err, ai.ErrInvalidOutput) || errors.Is(err, ErrEvidenceRequired)) && len(chunks) > 1 {
			middle := len(chunks) / 2
			left, leftResult, leftErr := p.generatePlanBatch(ctx, params, candidates, chunks[:middle])
			if leftErr != nil {
				return nil, nil, leftErr
			}
			right, _, rightErr := p.generatePlanBatch(ctx, params, candidates, chunks[middle:])
			if rightErr != nil {
				return nil, nil, rightErr
			}
			return append(left, right...), leftResult, nil
		}
		return nil, nil, err
	}
	return []*ImportPlan{plan}, result, nil
}

func (p *PagePlanner) generatePlanOnce(ctx context.Context, params PlanParams, candidates []PageCandidate,
	chunks []evidence.SourceChunk) (*ai.Result, error) {
	chunkViews := make([]map[string]any, len(chunks))
	for index := range chunks {
		chunkViews[index] = map[string]any{
			"chunk_id": chunks[index].ID, "ordinal": chunks[index].Ordinal,
			"text": chunks[index].TextContent, "locator": json.RawMessage(chunks[index].LocatorJSON),
		}
	}
	chunksJSON, err := json.Marshal(chunkViews)
	if err != nil {
		return nil, err
	}
	candidatesJSON, err := json.Marshal(candidates)
	if err != nil {
		return nil, err
	}
	entities := []map[string]any{}
	for _, candidate := range rankedEntityCandidates(params.Candidates, 24) {
		entities = append(entities, map[string]any{
			"type": candidate.TypeKey, "label": candidate.Label, "aliases": candidate.Aliases,
		})
	}
	entitiesJSON, err := json.Marshal(entities)
	if err != nil {
		return nil, err
	}
	return p.ai.Generate(ctx, ai.Request{
		Provider: params.Provider, Model: params.Model, PromptKey: ImportPlanPromptKey,
		Variables: map[string]any{
			"source_version_id": params.SourceVersionID.String(),
			"source_label":      strings.TrimSpace(params.SourceLabel),
			"preferred_title":   strings.TrimSpace(params.PreferredTitle),
			"instructions":      strings.TrimSpace(params.Instructions),
			"route_mode":        params.RouteMode,
			"entities_json":     string(entitiesJSON),
			"candidate_pages":   string(candidatesJSON),
			"chunks_json":       string(chunksJSON),
		},
		ImportJobID: params.ImportJobID, ImportRunID: params.ImportRunID,
	})
}

func ValidateImportPlan(raw []byte, sourceVersionID uuid.UUID, chunks []evidence.SourceChunk,
	candidates []PageCandidate, routeMode, preferredTitle string) (*ImportPlan, json.RawMessage, error) {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil || compiledImportPlanSchema.Validate(instance) != nil {
		return nil, nil, fmt.Errorf("%w: import plan schema", ai.ErrInvalidOutput)
	}
	var plan ImportPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, nil, fmt.Errorf("%w: import plan document", ai.ErrInvalidOutput)
	}
	if plan.SourceVersionID != sourceVersionID {
		plan.SourceVersionID = sourceVersionID
		plan.QualityScore *= 0.9
	}
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
	routes := make([]PageRoute, 0, len(plan.Routes))
	for _, route := range plan.Routes {
		route.Title = strings.TrimSpace(route.Title)
		route.Reason = strings.TrimSpace(route.Reason)
		if route.Title == "" || route.Reason == "" {
			continue
		}
		if route.Action == RouteUpdate || route.Action == RouteLink {
			if route.PageID == nil {
				continue
			}
			candidate, ok := candidateByID[*route.PageID]
			if !ok {
				continue
			}
			route.Title = candidate.Title
		}
		if route.Action == RouteLink {
			route.Evidence = catalog.normalize(route.Evidence)
			if len(route.Evidence) == 0 {
				continue
			}
			route.RelatedTo = normalizedRelatedTitles(route.RelatedTo)
			if len(route.RelatedTo) == 0 {
				continue
			}
		}
		if routeMode == RouteModeForceCreate && route.Action != RouteCreate && route.Action != RouteIgnore {
			continue
		}
		blocks := make([]PlannedBlock, 0, len(route.Blocks))
		for _, block := range route.Blocks {
			originalBlocks++
			block.Evidence = catalog.normalize(block.Evidence)
			if len(block.Evidence) == 0 {
				continue
			}
			if route.Action == RouteCreate && (block.Mode != BlockAppend || block.TargetBlockID != nil) {
				continue
			}
			if route.Action == RouteUpdate && block.Mode == BlockReplace {
				if block.TargetBlockID == nil || blockOwner[*block.TargetBlockID] != *route.PageID {
					continue
				}
			}
			if route.Action == RouteUpdate && block.Mode == BlockAppend && block.TargetBlockID != nil {
				continue
			}
			if !validPlannedBlockContent(block) {
				continue
			}
			retainedBlocks++
			blocks = append(blocks, block)
		}
		route.Blocks = blocks
		if (route.Action == RouteCreate || route.Action == RouteUpdate) && len(route.Blocks) == 0 {
			continue
		}
		routes = append(routes, route)
	}
	plan.Routes = routes
	if routeMode == RouteModeForceCreate {
		wanted, err := page.NormalizeTitle(preferredTitle)
		if err != nil {
			return nil, nil, err
		}
		found := false
		for index := range plan.Routes {
			if plan.Routes[index].Action != RouteCreate {
				continue
			}
			title, titleErr := page.NormalizeTitle(plan.Routes[index].Title)
			if titleErr == nil && title == wanted {
				plan.Routes[index].Title = strings.TrimSpace(preferredTitle)
				found = true
			}
		}
		if !found {
			return nil, nil, ErrNoPagePlan
		}
	}
	if originalBlocks > 0 && retainedBlocks == 0 && plan.Profile.Useful {
		return nil, nil, ErrEvidenceRequired
	}
	if originalBlocks > 0 && retainedBlocks < originalBlocks {
		plan.QualityScore *= float64(retainedBlocks) / float64(originalBlocks)
	}
	canonical, _ := json.Marshal(&plan)
	return &plan, canonical, nil
}

func validPlannedBlockContent(block PlannedBlock) bool {
	switch block.Type {
	case string(ast.BlockHeading):
		return strings.TrimSpace(block.Text) != "" && block.Level >= 1 && block.Level <= 6 && len(block.Items) == 0
	case string(ast.BlockParagraph), string(ast.BlockQuote):
		return strings.TrimSpace(block.Text) != "" && len(block.Items) == 0
	case string(ast.BlockBulletList):
		if strings.TrimSpace(block.Text) != "" || len(block.Items) == 0 {
			return false
		}
		for _, item := range block.Items {
			if strings.TrimSpace(item) == "" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// normalizeImportPlanCollections keeps the Go representation aligned with the
// JSON Schema/OpenAPI contract. A nil slice is semantically empty in Go, but
// encoding/json emits it as null; generated clients expect required collection
// fields to always be arrays and deserialize them with Array.map.
//
// Keep this normalization on both persistence and read paths so plans written
// by an older worker remain readable without mutating the immutable plan row.
func normalizeImportPlanCollections(plan *ImportPlan) {
	if plan == nil {
		return
	}
	if plan.Profile.Subjects == nil {
		plan.Profile.Subjects = []SourceSubject{}
	}
	if plan.Routes == nil {
		plan.Routes = []PageRoute{}
	}
	for routeIndex := range plan.Routes {
		route := &plan.Routes[routeIndex]
		if route.RelatedTo == nil {
			route.RelatedTo = []string{}
		}
		if route.Evidence == nil {
			route.Evidence = []CandidateEvidence{}
		}
		if route.Blocks == nil {
			route.Blocks = []PlannedBlock{}
		}
		for blockIndex := range route.Blocks {
			if route.Blocks[blockIndex].Evidence == nil {
				route.Blocks[blockIndex].Evidence = []CandidateEvidence{}
			}
		}
	}
}

func (p *PagePlanner) recallPages(ctx context.Context, params PlanParams) ([]PageCandidate, error) {
	terms := []string{params.PreferredTitle, params.SourceLabel}
	for _, candidate := range rankedEntityCandidates(params.Candidates, 12) {
		terms = append(terms, candidate.Label)
		terms = append(terms, candidate.Aliases...)
	}
	seenTerm := map[string]bool{}
	hits := map[uuid.UUID]page.PageSearchHit{}
	if params.TargetPageID != nil && *params.TargetPageID != uuid.Nil {
		target, err := p.pages.GetPage(ctx, *params.TargetPageID)
		if err != nil || target.WikiID != params.WikiID || target.DeletedAt != nil {
			return nil, ErrPlanTargetConflict
		}
		hits[target.ID] = page.PageSearchHit{ID: target.ID, DisplayTitle: target.DisplayTitle,
			NamespaceKey: "main", MatchedOn: "explicit"}
	}
	for _, rawTerm := range terms {
		term := strings.TrimSpace(rawTerm)
		key := strings.ToLower(term)
		if term == "" || seenTerm[key] || len(seenTerm) >= 16 || len(hits) >= 20 {
			continue
		}
		seenTerm[key] = true
		found, err := p.pages.SearchPages(ctx, params.WikiID, "main", term, 5)
		if err != nil {
			return nil, err
		}
		for _, hit := range found {
			if _, exists := hits[hit.ID]; !exists {
				hits[hit.ID] = hit
			}
		}
	}
	ordered := make([]page.PageSearchHit, 0, len(hits))
	for _, hit := range hits {
		ordered = append(ordered, hit)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].DisplayTitle == ordered[j].DisplayTitle {
			return ordered[i].ID.String() < ordered[j].ID.String()
		}
		return strings.ToLower(ordered[i].DisplayTitle) < strings.ToLower(ordered[j].DisplayTitle)
	})
	if len(ordered) > 12 {
		ordered = ordered[:12]
	}
	result := make([]PageCandidate, 0, len(ordered))
	for _, hit := range ordered {
		candidate, err := p.loadPageCandidate(ctx, hit)
		if err != nil {
			return nil, err
		}
		result = append(result, candidate)
	}
	return result, nil
}

func (p *PagePlanner) loadPageCandidate(ctx context.Context, hit page.PageSearchHit) (PageCandidate, error) {
	candidate := PageCandidate{PageID: hit.ID, Title: hit.DisplayTitle, MatchedOn: hit.MatchedOn,
		Blocks: []PageCandidateBlock{}}
	revision, snapshot, err := p.pages.CurrentContent(ctx, hit.ID)
	if err != nil {
		return PageCandidate{}, err
	}
	if revision == nil || snapshot == nil {
		return candidate, nil
	}
	candidate.CurrentRevisionID = &revision.ID
	doc, err := ast.Parse(snapshot.AST)
	if err != nil {
		return PageCandidate{}, err
	}
	characters := 0
	for _, block := range ast.Blocks(doc) {
		text := plannedBlockTextFromAST(block)
		if text == "" {
			continue
		}
		if characters+len([]rune(text)) > 6000 || len(candidate.Blocks) >= 40 {
			break
		}
		hash, err := governanceBlockHash(block)
		if err != nil {
			return PageCandidate{}, err
		}
		candidate.Blocks = append(candidate.Blocks, PageCandidateBlock{
			ID: block.ID, Type: string(block.Type), Text: text, Hash: hash,
		})
		characters += len([]rune(text))
	}
	return candidate, nil
}

// governanceBlockHash is kept local to avoid making importer depend on the
// governance package merely for a pure AST hash helper.
func governanceBlockHash(block *ast.Block) (string, error) {
	raw, err := json.Marshal(block)
	if err != nil {
		return "", err
	}
	canonical, err := ast.CanonicalizeJSON(raw)
	if err != nil {
		return "", err
	}
	return HashBytes(canonical), nil
}

func plannedBlockTextFromAST(block *ast.Block) string {
	if block == nil {
		return ""
	}
	if block.Type == ast.BlockParagraph || block.Type == ast.BlockHeading {
		nodes, err := block.InlineContent()
		if err != nil {
			return ""
		}
		parts := make([]string, 0, len(nodes))
		for _, node := range nodes {
			if node == nil {
				continue
			}
			if node.Text != "" {
				parts = append(parts, node.Text)
			} else if node.DisplayText != "" {
				parts = append(parts, node.DisplayText)
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	}
	return ""
}

func (p *PagePlanner) resolveCreateCollisions(ctx context.Context, params PlanParams, plan *ImportPlan) error {
	for index := range plan.Routes {
		route := &plan.Routes[index]
		if route.Action != RouteCreate {
			continue
		}
		resolved, err := p.pages.ResolveTitle(ctx, params.WikiID, "main", route.Title)
		if errors.Is(err, page.ErrPageNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if params.RouteMode == RouteModeForceCreate {
			return fmt.Errorf("%w: title=%q", ErrPlanTargetConflict, route.Title)
		}
		// Exact title/alias resolution is stronger than a model create decision.
		// Reuse the authoritative Page and retain only append intents; the model
		// never saw this page's Block catalog, so replacing would be unsafe.
		previousTitle := route.Title
		route.Action = RouteUpdate
		route.PageID = &resolved.Page.ID
		route.Title = resolved.Page.DisplayTitle
		rewriteRelatedTitle(plan.Routes, previousTitle, route.Title)
		for blockIndex := range route.Blocks {
			route.Blocks[blockIndex].Mode = BlockAppend
			route.Blocks[blockIndex].TargetBlockID = nil
		}
	}
	return nil
}

func (p *PagePlanner) removeNoOpUpdates(plan *ImportPlan, candidates []PageCandidate) {
	byID := make(map[uuid.UUID]PageCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.PageID] = candidate
	}
	for routeIndex := range plan.Routes {
		route := &plan.Routes[routeIndex]
		if route.Action != RouteUpdate || route.PageID == nil {
			continue
		}
		candidate, ok := byID[*route.PageID]
		if !ok {
			continue
		}
		existingText := map[string]bool{}
		blockText := map[string]string{}
		for _, block := range candidate.Blocks {
			normalized := normalizedPlanText(block.Text)
			existingText[normalized] = true
			blockText[block.ID] = normalized
		}
		filtered := make([]PlannedBlock, 0, len(route.Blocks))
		for _, block := range route.Blocks {
			text := normalizedPlanText(plannedBlockText(block))
			if block.Mode == BlockReplace && block.TargetBlockID != nil && blockText[*block.TargetBlockID] == text {
				continue
			}
			if block.Mode == BlockAppend && existingText[text] {
				continue
			}
			filtered = append(filtered, block)
		}
		route.Blocks = filtered
		if len(filtered) == 0 {
			route.Action = RouteLink
		}
	}
}

func mergeImportPlanParts(sourceVersionID uuid.UUID, parts []*ImportPlan) *ImportPlan {
	merged := &ImportPlan{SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Routes: []PageRoute{}, Profile: SourceProfile{Subjects: []SourceSubject{}}}
	qualityWeight := 0
	for _, part := range parts {
		if part == nil {
			continue
		}
		if merged.Profile.Title == "" || len([]rune(part.Profile.Summary)) > len([]rune(merged.Profile.Summary)) {
			merged.Profile.Title = part.Profile.Title
			merged.Profile.Summary = part.Profile.Summary
			merged.Profile.Language = part.Profile.Language
		}
		merged.Profile.Useful = merged.Profile.Useful || part.Profile.Useful
		merged.Profile.Subjects = mergeSourceSubjects(merged.Profile.Subjects, part.Profile.Subjects)
		merged.Routes = append(merged.Routes, part.Routes...)
		merged.PromptInjectionDetected = merged.PromptInjectionDetected || part.PromptInjectionDetected
		count := max(1, len(part.Routes))
		qualityWeight += count
		merged.QualityScore += part.QualityScore * float64(count)
	}
	if qualityWeight > 0 {
		merged.QualityScore /= float64(qualityWeight)
	}
	merged.Routes = mergePageRoutes(merged.Routes)
	return merged
}

func mergeSourceSubjects(existing, incoming []SourceSubject) []SourceSubject {
	result := append([]SourceSubject(nil), existing...)
	seen := map[string]bool{}
	for _, subject := range result {
		seen[strings.ToLower(strings.TrimSpace(subject.Title))] = true
	}
	for _, subject := range incoming {
		key := strings.ToLower(strings.TrimSpace(subject.Title))
		if key == "" || seen[key] || len(result) >= 20 {
			continue
		}
		seen[key] = true
		result = append(result, subject)
	}
	return result
}

func mergePageRoutes(routes []PageRoute) []PageRoute {
	result := make([]PageRoute, 0, len(routes))
	indexByKey := map[string]int{}
	for _, route := range routes {
		key := route.Action + "\x00" + strings.ToLower(strings.TrimSpace(route.Title))
		if route.PageID != nil {
			key = route.Action + "\x00" + route.PageID.String()
		}
		if index, ok := indexByKey[key]; ok {
			existing := &result[index]
			existing.Confidence = max(existing.Confidence, route.Confidence)
			existing.Blocks = mergePlannedBlocks(existing.Blocks, route.Blocks)
			existing.RelatedTo = mergeRelatedTitles(existing.RelatedTo, route.RelatedTo)
			existing.Evidence = mergeCandidateEvidence(existing.Evidence, route.Evidence)
			continue
		}
		indexByKey[key] = len(result)
		route.Blocks = mergePlannedBlocks(nil, route.Blocks)
		result = append(result, route)
	}
	sort.SliceStable(result, func(i, j int) bool {
		rank := map[string]int{RouteCreate: 0, RouteUpdate: 1, RouteLink: 2, RouteIgnore: 3}
		if rank[result[i].Action] != rank[result[j].Action] {
			return rank[result[i].Action] < rank[result[j].Action]
		}
		return strings.ToLower(result[i].Title) < strings.ToLower(result[j].Title)
	})
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

func normalizePlanLinks(routes []PageRoute) []PageRoute {
	actionableTitles := map[string]string{}
	for _, route := range routes {
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		actionableTitles[normalizedPlanText(route.Title)] = route.Title
	}
	result := make([]PageRoute, 0, len(routes))
	for _, route := range routes {
		if route.Action != RouteLink {
			result = append(result, route)
			continue
		}
		resolved := make([]string, 0, len(route.RelatedTo))
		seen := map[string]bool{}
		for _, rawTitle := range route.RelatedTo {
			key := normalizedPlanText(rawTitle)
			title, ok := actionableTitles[key]
			if !ok || seen[key] {
				continue
			}
			seen[key] = true
			resolved = append(resolved, title)
		}
		if len(resolved) == 0 || len(route.Evidence) == 0 {
			continue
		}
		route.RelatedTo = resolved
		result = append(result, route)
	}
	return result
}

func normalizedRelatedTitles(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		title := strings.TrimSpace(raw)
		key := normalizedPlanText(title)
		if key == "" || seen[key] || len(result) >= 20 {
			continue
		}
		seen[key] = true
		result = append(result, title)
	}
	return result
}

func mergeRelatedTitles(existing, incoming []string) []string {
	return normalizedRelatedTitles(append(append([]string{}, existing...), incoming...))
}

func rewriteRelatedTitle(routes []PageRoute, previous, replacement string) {
	previousKey := normalizedPlanText(previous)
	if previousKey == "" {
		return
	}
	for index := range routes {
		for relatedIndex := range routes[index].RelatedTo {
			if normalizedPlanText(routes[index].RelatedTo[relatedIndex]) == previousKey {
				routes[index].RelatedTo[relatedIndex] = replacement
			}
		}
	}
}

func mergePlannedBlocks(existing, incoming []PlannedBlock) []PlannedBlock {
	result := append([]PlannedBlock(nil), existing...)
	seen := map[string]bool{}
	replaceTargets := map[string]bool{}
	for _, block := range result {
		seen[plannedBlockKey(block)] = true
		if block.Mode == BlockReplace && block.TargetBlockID != nil {
			replaceTargets[*block.TargetBlockID] = true
		}
	}
	for _, block := range incoming {
		key := plannedBlockKey(block)
		if seen[key] || len(result) >= 100 ||
			(block.Mode == BlockReplace && block.TargetBlockID != nil && replaceTargets[*block.TargetBlockID]) {
			continue
		}
		seen[key] = true
		if block.Mode == BlockReplace && block.TargetBlockID != nil {
			replaceTargets[*block.TargetBlockID] = true
		}
		result = append(result, block)
	}
	return result
}

func plannedBlockKey(block PlannedBlock) string {
	target := ""
	if block.TargetBlockID != nil {
		target = *block.TargetBlockID
	}
	return strings.Join([]string{block.Mode, target, block.Type, normalizedPlanText(plannedBlockText(block))}, "\x00")
}

func plannedBlockText(block PlannedBlock) string {
	if block.Type == string(ast.BlockBulletList) {
		return strings.Join(block.Items, "\n")
	}
	return block.Text
}

func normalizedPlanText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func actionablePageRouteCount(routes []PageRoute) int {
	count := 0
	for _, route := range routes {
		if route.Action == RouteCreate || route.Action == RouteUpdate {
			count++
		}
	}
	return count
}

func linkedPageRouteCount(routes []PageRoute) int {
	count := 0
	for _, route := range routes {
		if route.Action == RouteLink {
			count++
		}
	}
	return count
}

func chunkTexts(chunks []evidence.SourceChunk) []string {
	result := make([]string, len(chunks))
	for index := range chunks {
		result[index] = chunks[index].TextContent
	}
	return result
}

func mustCompileImportPlanSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(importPlanSchemaJSON))
	if err != nil {
		panic(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(ImportPlanSchemaURL, doc); err != nil {
		panic(err)
	}
	schema, err := compiler.Compile(ImportPlanSchemaURL)
	if err != nil {
		panic(err)
	}
	return schema
}
