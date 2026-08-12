package importer

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
)

const (
	ImportPlanFidelitySchemaURL = "https://anby.wiki/schemas/import-plan-fidelity/v1/audit.schema.json"
	ImportPlanFidelityPromptKey = "source-import-plan-fidelity-v3"

	planFidelityValidationAttempts = 3
)

//go:embed schema/import-plan-fidelity.schema.json
var importPlanFidelitySchemaJSON []byte

var compiledImportPlanFidelitySchema = mustCompileImportPlanFidelitySchema()

func ImportPlanFidelitySchemaJSON() json.RawMessage {
	return append(json.RawMessage(nil), importPlanFidelitySchemaJSON...)
}

type PlanFidelityMissingBlock struct {
	RouteIndex   int                 `json:"route_index"`
	AfterHeading string              `json:"after_heading"`
	Text         string              `json:"text"`
	Evidence     []CandidateEvidence `json:"evidence"`
}

type PlanFidelityAudit struct {
	SchemaVersion   int                        `json:"schema_version"`
	SourceVersionID uuid.UUID                  `json:"source_version_id"`
	Complete        bool                       `json:"complete"`
	CoverageBefore  float64                    `json:"coverage_before"`
	CoverageAfter   float64                    `json:"coverage_after"`
	MissingBlocks   []PlanFidelityMissingBlock `json:"missing_blocks"`
}

type planFidelityRouteView struct {
	RouteIndex int      `json:"route_index"`
	Action     string   `json:"action"`
	Title      string   `json:"title"`
	Blocks     []string `json:"blocks"`
}

type planFidelityOutcome struct {
	audit  *PlanFidelityAudit
	weight int
	err    error
}

// ensurePlanFidelity performs the source-vs-plan audit introduced after the
// normal planning/reduction pass. Each source window is checked against the
// complete draft, so material facts lost at a map boundary or by the reducer
// are returned as small, evidence-backed repair blocks rather than requiring
// another full-plan generation.
func (p *PagePlanner) ensurePlanFidelity(ctx context.Context, params PlanParams, plan *ImportPlan) (float64, error) {
	if plan == nil {
		return 0, ErrQualityGate
	}
	if !plan.Profile.Useful || actionablePageRouteCount(plan.Routes) == 0 {
		return 1, nil
	}
	if len(params.Chunks) == 0 {
		return 0, ErrEvidenceRequired
	}

	draftJSON, err := json.Marshal(planFidelityDraftView(plan))
	if err != nil {
		return 0, err
	}
	maxInputTokens := params.MaxInputTokens
	if maxInputTokens <= 0 {
		maxInputTokens = DefaultModelMaxInputTokens
	}
	availableForChunks := maxInputTokens - estimatePlanFidelityTokens(draftJSON) - extractionInputMinReserve
	if availableForChunks < extractionInputMinReserve {
		return 0, fmt.Errorf("%w: fidelity audit input budget", ErrQualityGate)
	}
	batches := batchSourceChunks(params.Chunks, availableForChunks)
	outcomes := make([]planFidelityOutcome, len(batches))
	auditCtx, cancel := context.WithCancel(ctx)
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
			case <-auditCtx.Done():
				outcomes[index].err = auditCtx.Err()
				return
			}
			outcomes[index].audit, outcomes[index].err = p.auditPlanFidelityBatch(
				auditCtx, params, draftJSON, plan, batches[index],
			)
			outcomes[index].weight = sourceChunkRuneCount(batches[index])
			if outcomes[index].err != nil {
				cancel()
			}
		}(index)
	}
	wait.Wait()
	if err := planFidelityOutcomeError(ctx, outcomes); err != nil {
		return 0, err
	}

	audits := make([]*PlanFidelityAudit, 0, len(outcomes))
	weightedCoverage, totalWeight := 0.0, 0
	for index := range outcomes {
		outcome := outcomes[index]
		if outcome.audit == nil {
			return 0, fmt.Errorf("%w: empty fidelity audit", ai.ErrInvalidOutput)
		}
		weight := max(1, outcome.weight)
		totalWeight += weight
		weightedCoverage += outcome.audit.CoverageAfter * float64(weight)
		audits = append(audits, outcome.audit)
	}
	if totalWeight == 0 {
		return 0, ErrQualityGate
	}
	coverage := weightedCoverage / float64(totalWeight)
	if coverage < DefaultQualityThreshold {
		return coverage, ErrQualityGate
	}
	applyPlanFidelityAudits(plan, audits)
	refineImportPlan(plan)
	return coverage, nil
}

// planFidelityOutcomeError prefers the batch error that triggered sibling
// cancellation. Batch order is deterministic, but completion order is not;
// returning the first array entry used to turn a useful validation error into
// a generic context.Canceled failure whenever an earlier sibling stopped first.
func planFidelityOutcomeError(ctx context.Context, outcomes []planFidelityOutcome) error {
	var cancelled error
	for index := range outcomes {
		err := outcomes[index].err
		if err == nil {
			continue
		}
		if errors.Is(err, context.Canceled) {
			if cancelled == nil {
				cancelled = err
			}
			continue
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return cancelled
}

func (p *PagePlanner) auditPlanFidelityBatch(ctx context.Context, params PlanParams, draftJSON []byte,
	plan *ImportPlan, chunks []evidence.SourceChunk) (*PlanFidelityAudit, error) {
	chunksJSON, err := json.Marshal(planChunkViews(chunks))
	if err != nil {
		return nil, err
	}
	validationFeedback := "No prior validation failure."
	var lastValidationErr error
	var lastOutput json.RawMessage
	for attempt := 0; attempt < planFidelityValidationAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := p.ai.Generate(ctx, ai.Request{
			Provider: params.Provider, Model: params.Model, PromptKey: ImportPlanFidelityPromptKey,
			Variables: map[string]any{
				"source_version_id":   params.SourceVersionID.String(),
				"source_label":        strings.TrimSpace(params.SourceLabel),
				"preferred_title":     strings.TrimSpace(params.PreferredTitle),
				"instructions":        strings.TrimSpace(params.Instructions),
				"route_mode":          params.RouteMode,
				"draft_plan_json":     string(draftJSON),
				"chunks_json":         string(chunksJSON),
				"validation_feedback": validationFeedback,
			},
			ImportJobID: params.ImportJobID, ImportRunID: params.ImportRunID,
		})
		if err == nil {
			lastOutput = append(lastOutput[:0], result.JSON...)
			var audit *PlanFidelityAudit
			audit, err = ValidatePlanFidelityAudit(result.JSON, params.SourceVersionID, chunks, plan)
			if err == nil {
				return audit, nil
			}
		}
		if !isPlanFidelityValidationError(err) {
			return nil, err
		}
		lastValidationErr = err
		validationFeedback = planFidelityValidationFeedback(err)
	}
	// Fidelity is a repair/quality pass over a plan whose existing blocks have
	// already passed strict evidence validation. After three failed correction
	// attempts, isolate only unverifiable repair blocks and revoke their claimed
	// coverage improvement. Never let an ungrounded repair into the plan, but do
	// not discard the independently grounded draft because one optional patch is
	// bad. The normal quality gate still rejects genuinely low coverage.
	if len(lastOutput) > 0 {
		audit, rejected, err := salvagePlanFidelityAudit(lastOutput, params.SourceVersionID, chunks, plan)
		if err == nil && rejected > 0 {
			return audit, nil
		}
	}
	return nil, lastValidationErr
}

func isPlanFidelityValidationError(err error) bool {
	var providerErr *ai.ProviderError
	if errors.As(err, &providerErr) && (providerErr.Code == "invalid_schema" || providerErr.Code == "output_truncated") {
		return false
	}
	return errors.Is(err, ai.ErrInvalidOutput) || errors.Is(err, ErrEvidenceRequired)
}

func planFidelityValidationFeedback(err error) string {
	if errors.Is(err, ErrEvidenceRequired) {
		return "The previous response cited unverifiable evidence. The requested article language applies only to missing_blocks.text, never to evidence.quotation. Keep each quotation in the source language and copy it verbatim, including punctuation, line breaks, and indentation, with its exact chunk_id and Unicode offsets."
	}
	return "The previous response failed server validation. Echo source_version_id exactly, use only authoritative create/update route_index values, keep coverage_after greater than or equal to coverage_before, and satisfy the JSON schema exactly."
}

func ValidatePlanFidelityAudit(raw []byte, sourceVersionID uuid.UUID, chunks []evidence.SourceChunk,
	plan *ImportPlan) (*PlanFidelityAudit, error) {
	audit, _, err := validatePlanFidelityAudit(raw, sourceVersionID, chunks, plan, false)
	return audit, err
}

// salvagePlanFidelityAudit is intentionally available only after strict
// validation and all correction attempts fail. It may remove invalid repair
// blocks, but it can never manufacture or broaden evidence.
func salvagePlanFidelityAudit(raw []byte, sourceVersionID uuid.UUID, chunks []evidence.SourceChunk,
	plan *ImportPlan) (*PlanFidelityAudit, int, error) {
	return validatePlanFidelityAudit(raw, sourceVersionID, chunks, plan, true)
}

func validatePlanFidelityAudit(raw []byte, sourceVersionID uuid.UUID, chunks []evidence.SourceChunk,
	plan *ImportPlan, discardInvalidRepairs bool) (*PlanFidelityAudit, int, error) {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil || compiledImportPlanFidelitySchema.Validate(instance) != nil {
		return nil, 0, fmt.Errorf("%w: import plan fidelity schema", ai.ErrInvalidOutput)
	}
	var audit PlanFidelityAudit
	if err := json.Unmarshal(raw, &audit); err != nil {
		return nil, 0, fmt.Errorf("%w: import plan fidelity document", ai.ErrInvalidOutput)
	}
	if plan == nil || audit.SourceVersionID != sourceVersionID || audit.CoverageAfter < audit.CoverageBefore {
		return nil, 0, fmt.Errorf("%w: import plan fidelity metadata", ai.ErrInvalidOutput)
	}
	catalog := newEvidenceCatalog(sourceVersionID, chunks)
	originalBlockCount := len(audit.MissingBlocks)
	retained := make([]PlanFidelityMissingBlock, 0, originalBlockCount)
	rejected := 0
	for index := range audit.MissingBlocks {
		block := audit.MissingBlocks[index]
		block.Text = strings.TrimSpace(block.Text)
		block.AfterHeading = strings.TrimSpace(block.AfterHeading)
		if block.RouteIndex < 0 || block.RouteIndex >= len(plan.Routes) || block.Text == "" {
			if discardInvalidRepairs {
				rejected++
				continue
			}
			return nil, 0, fmt.Errorf("%w: import plan fidelity route", ai.ErrInvalidOutput)
		}
		route := plan.Routes[block.RouteIndex]
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			if discardInvalidRepairs {
				rejected++
				continue
			}
			return nil, 0, fmt.Errorf("%w: import plan fidelity action", ai.ErrInvalidOutput)
		}
		block.Evidence = catalog.normalize(block.Evidence)
		if len(block.Evidence) == 0 {
			if discardInvalidRepairs {
				rejected++
				continue
			}
			return nil, 0, ErrEvidenceRequired
		}
		if block.AfterHeading != "" && !routeHasHeading(route, block.AfterHeading) {
			// A valid fact is more important than a guessed section label. Append
			// it to the route instead of allowing the model to invent structure.
			block.AfterHeading = ""
		}
		retained = append(retained, block)
	}
	audit.MissingBlocks = retained
	if rejected > 0 && originalBlockCount > 0 {
		// Only verified repairs may earn coverage improvement. Rejected repairs
		// contribute zero; accepted repairs receive their proportional share.
		improvement := max(0.0, audit.CoverageAfter-audit.CoverageBefore)
		retainedFraction := float64(len(retained)) / float64(originalBlockCount)
		audit.CoverageAfter = audit.CoverageBefore + improvement*retainedFraction
		audit.Complete = false
	}
	return &audit, rejected, nil
}

func planFidelityDraftView(plan *ImportPlan) []planFidelityRouteView {
	if plan == nil {
		return []planFidelityRouteView{}
	}
	result := make([]planFidelityRouteView, 0, len(plan.Routes))
	for routeIndex := range plan.Routes {
		route := plan.Routes[routeIndex]
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		blocks := make([]string, 0, len(route.Blocks))
		for _, block := range route.Blocks {
			switch block.Type {
			case string(ast.BlockHeading):
				blocks = append(blocks, strings.Repeat("#", max(1, block.Level))+" "+block.Text)
			case string(ast.BlockBulletList):
				for _, item := range block.Items {
					blocks = append(blocks, "- "+item)
				}
			default:
				blocks = append(blocks, block.Text)
			}
		}
		result = append(result, planFidelityRouteView{
			RouteIndex: routeIndex, Action: route.Action, Title: route.Title, Blocks: blocks,
		})
	}
	return result
}

func planChunkViews(chunks []evidence.SourceChunk) []map[string]any {
	result := make([]map[string]any, len(chunks))
	for index := range chunks {
		result[index] = map[string]any{
			"chunk_id": chunks[index].ID, "ordinal": chunks[index].Ordinal,
			"text": chunks[index].TextContent, "locator": json.RawMessage(chunks[index].LocatorJSON),
		}
	}
	return result
}

func estimatePlanFidelityTokens(value []byte) int {
	ascii, nonASCII := 0, 0
	for _, r := range string(value) {
		if r <= utf8.RuneSelf {
			ascii++
		} else {
			nonASCII++
		}
	}
	return 1024 + (ascii+3)/4 + nonASCII*2
}

func sourceChunkRuneCount(chunks []evidence.SourceChunk) int {
	total := 0
	for _, chunk := range chunks {
		total += utf8.RuneCountInString(chunk.TextContent)
	}
	return total
}

func routeHasHeading(route PageRoute, wanted string) bool {
	wanted = canonicalHeadingTopic(wanted)
	if wanted == "" {
		return false
	}
	for _, block := range route.Blocks {
		if block.Type == string(ast.BlockHeading) && canonicalHeadingTopic(block.Text) == wanted {
			return true
		}
	}
	return false
}

func applyPlanFidelityAudits(plan *ImportPlan, audits []*PlanFidelityAudit) int {
	if plan == nil {
		return 0
	}
	applied := 0
	for _, audit := range audits {
		if audit == nil {
			continue
		}
		for _, missing := range audit.MissingBlocks {
			if missing.RouteIndex < 0 || missing.RouteIndex >= len(plan.Routes) {
				continue
			}
			route := &plan.Routes[missing.RouteIndex]
			if routeContainsEquivalentProse(*route, missing.Text) {
				continue
			}
			block := PlannedBlock{
				Type: string(ast.BlockParagraph), Mode: BlockAppend, TargetBlockID: nil,
				Text: strings.TrimSpace(missing.Text), Evidence: missing.Evidence,
			}
			route.Blocks = insertPlanBlockAfterSection(route.Blocks, block, missing.AfterHeading)
			applied++
		}
	}
	return applied
}

func insertPlanBlockAfterSection(blocks []PlannedBlock, block PlannedBlock, afterHeading string) []PlannedBlock {
	if strings.TrimSpace(afterHeading) == "" {
		return append(blocks, block)
	}
	wanted := canonicalHeadingTopic(afterHeading)
	headingIndex, headingLevel := -1, 0
	for index := range blocks {
		if blocks[index].Type == string(ast.BlockHeading) && canonicalHeadingTopic(blocks[index].Text) == wanted {
			headingIndex, headingLevel = index, blocks[index].Level
			break
		}
	}
	if headingIndex < 0 {
		return append(blocks, block)
	}
	insertAt := len(blocks)
	for index := headingIndex + 1; index < len(blocks); index++ {
		if blocks[index].Type == string(ast.BlockHeading) && blocks[index].Level <= headingLevel {
			insertAt = index
			break
		}
	}
	blocks = append(blocks, PlannedBlock{})
	copy(blocks[insertAt+1:], blocks[insertAt:])
	blocks[insertAt] = block
	return blocks
}

func routeContainsEquivalentProse(route PageRoute, incoming string) bool {
	for _, block := range route.Blocks {
		if block.Type == string(ast.BlockHeading) {
			continue
		}
		text := block.Text
		if block.Type == string(ast.BlockBulletList) {
			text = strings.Join(block.Items, " ")
		}
		if proseEquivalent(text, incoming) {
			return true
		}
	}
	return false
}

func proseEquivalent(left, right string) bool {
	leftKey, rightKey := normalizedSentenceKey(left), normalizedSentenceKey(right)
	if leftKey == "" || rightKey == "" {
		return false
	}
	if leftKey == rightKey || (utf8.RuneCountInString(leftKey) >= 24 && strings.Contains(rightKey, leftKey)) ||
		(utf8.RuneCountInString(rightKey) >= 24 && strings.Contains(leftKey, rightKey)) {
		return true
	}
	leftTokens, rightTokens := planTokenSet(leftKey), planTokenSet(rightKey)
	if len(leftTokens) < 4 || len(rightTokens) < 4 {
		return false
	}
	intersection := 0
	for token := range leftTokens {
		if rightTokens[token] {
			intersection++
		}
	}
	union := len(leftTokens) + len(rightTokens) - intersection
	return union > 0 && float64(intersection)/float64(union) >= 0.82
}

func planTokenSet(value string) map[string]bool {
	result := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if utf8.RuneCountInString(token) >= 2 {
			result[token] = true
		}
	}
	return result
}

// assessImportPlanQuality deliberately ignores the model-authored score. The
// persisted score is derived from independently checked fidelity, exact
// evidence support, article structure, duplication, and route confidence.
func assessImportPlanQuality(plan *ImportPlan, chunks []evidence.SourceChunk, fidelity float64) float64 {
	if plan == nil {
		return 0
	}
	if !plan.Profile.Useful && actionablePageRouteCount(plan.Routes) == 0 {
		return 1
	}
	grounding := planGroundingScore(plan)
	structure := planStructureScore(plan, sourceChunkRuneCount(chunks))
	concision := planConcisionScore(plan)
	routing := planRoutingScore(plan)
	score := clampUnit(fidelity)*0.35 + grounding*0.25 + structure*0.20 + concision*0.10 + routing*0.10
	if fidelity < DefaultQualityThreshold || grounding < 0.65 || structure < 0.78 {
		score = min(score, DefaultQualityThreshold-0.01)
	}
	return math.Round(clampUnit(score)*1000) / 1000
}

func planGroundingScore(plan *ImportPlan) float64 {
	total, score := 0, 0.0
	for _, route := range plan.Routes {
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		for _, block := range route.Blocks {
			total++
			if len(block.Evidence) == 0 {
				continue
			}
			if block.Type == string(ast.BlockHeading) {
				score += 1
				continue
			}
			text := block.Text
			if block.Type == string(ast.BlockBulletList) {
				text = strings.Join(block.Items, " ")
			}
			proseRunes := max(1, utf8.RuneCountInString(text))
			quotedRunes := 0
			seen := map[string]bool{}
			for _, item := range block.Evidence {
				key := evidenceKey(item)
				if !seen[key] {
					seen[key] = true
					quotedRunes += utf8.RuneCountInString(item.Quotation)
				}
			}
			ratio := float64(quotedRunes) / float64(proseRunes)
			switch {
			case ratio >= 0.50:
				score += 1
			case ratio >= 0.25:
				score += 0.85
			case ratio >= 0.12:
				score += 0.70
			default:
				score += 0.45
			}
		}
	}
	if total == 0 {
		return 0
	}
	return clampUnit(score / float64(total))
}

func planStructureScore(plan *ImportPlan, sourceRunes int) float64 {
	total, score := 0, 0.0
	for _, route := range plan.Routes {
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		total++
		if len(route.Blocks) == 0 {
			continue
		}
		routeScore := 0.0
		if route.Action != RouteCreate || route.Blocks[0].Type == string(ast.BlockParagraph) {
			routeScore += 0.20
		}
		headings, contentBlocks, contentRunes := 0, 0, 0
		previousHeadingLevel := 0
		validHeadingFlow := true
		for _, block := range route.Blocks {
			if block.Type == string(ast.BlockHeading) {
				headings++
				if previousHeadingLevel > 0 && block.Level > previousHeadingLevel+1 {
					validHeadingFlow = false
				}
				previousHeadingLevel = block.Level
				continue
			}
			contentBlocks++
			if block.Type == string(ast.BlockBulletList) {
				contentRunes += utf8.RuneCountInString(strings.Join(block.Items, " "))
			} else {
				contentRunes += utf8.RuneCountInString(block.Text)
			}
		}
		headingDensity := 1.0
		if contentBlocks == 0 {
			headingDensity = 0
		} else if headings > 0 {
			ratio := float64(headings) / float64(contentBlocks)
			switch {
			case ratio <= 0.50:
				headingDensity = 1
			case ratio <= 0.75:
				headingDensity = 1 - (ratio-0.50)*2
			default:
				headingDensity = 0.20
			}
		}
		routeScore += 0.30 * headingDensity
		if validHeadingFlow {
			routeScore += 0.15
		}
		minimumRunes := 40
		if sourceRunes >= 1000 {
			minimumRunes = 120
		}
		if sourceRunes >= 4000 {
			minimumRunes = 240
		}
		routeScore += 0.35 * min(1, float64(contentRunes)/float64(minimumRunes))
		score += routeScore
	}
	if total == 0 {
		return 0
	}
	return clampUnit(score / float64(total))
}

func planConcisionScore(plan *ImportPlan) float64 {
	texts := make([]string, 0)
	for _, route := range plan.Routes {
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		for _, block := range route.Blocks {
			if block.Type == string(ast.BlockHeading) {
				if isDiscardedImportSection(block.Text) {
					return 0
				}
				continue
			}
			if block.Type == string(ast.BlockBulletList) {
				texts = append(texts, block.Items...)
			} else {
				texts = append(texts, splitPlanSentences(block.Text)...)
			}
		}
	}
	if len(texts) < 2 {
		return 1
	}
	duplicates := 0
	for index := range texts {
		for previous := 0; previous < index; previous++ {
			if proseEquivalent(texts[previous], texts[index]) {
				duplicates++
				break
			}
		}
	}
	return clampUnit(1 - float64(duplicates)/float64(len(texts)))
}

func planRoutingScore(plan *ImportPlan) float64 {
	count, confidence := 0, 0.0
	for _, route := range plan.Routes {
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		count++
		confidence += clampUnit(route.Confidence)
	}
	if count == 0 {
		return 0
	}
	return 0.50 + 0.50*confidence/float64(count)
}

func clampUnit(value float64) float64 {
	return max(0, min(1, value))
}

func mustCompileImportPlanFidelitySchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(importPlanFidelitySchemaJSON))
	if err != nil {
		panic(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(ImportPlanFidelitySchemaURL, doc); err != nil {
		panic(err)
	}
	schema, err := compiler.Compile(ImportPlanFidelitySchemaURL)
	if err != nil {
		panic(err)
	}
	return schema
}
