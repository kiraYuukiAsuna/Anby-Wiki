package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type consolidatingPlanGenerator struct {
	mu               sync.Mutex
	mapCalls         int
	consolidateCalls int
	fidelityCalls    int
}

type retryingFidelityGenerator struct {
	mu         sync.Mutex
	calls      int
	validAfter int
	feedback   []string
}

type retryingPlanGenerator struct {
	mu         sync.Mutex
	calls      int
	validAfter int
	feedback   []string
}

func (g *retryingPlanGenerator) Generate(_ context.Context, request ai.Request) (*ai.Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	g.feedback = append(g.feedback, request.Variables["validation_feedback"].(string))
	var chunks []struct {
		ChunkID uuid.UUID `json:"chunk_id"`
		Text    string    `json:"text"`
	}
	if err := json.Unmarshal([]byte(request.Variables["chunks_json"].(string)), &chunks); err != nil {
		return nil, err
	}
	quotation := "invented quotation"
	if g.validAfter > 0 && g.calls >= g.validAfter {
		quotation = chunks[0].Text
	}
	sourceVersionID := uuid.MustParse(request.Variables["source_version_id"].(string))
	plan := &ImportPlan{
		SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Profile: SourceProfile{Title: "Subject", Summary: "Summary", Language: "en", Useful: true, Subjects: []SourceSubject{}},
		Routes: []PageRoute{{
			Action: RouteCreate, Title: "Subject", Reason: "documented", Confidence: 0.9,
			RelatedTo: []string{}, Evidence: []CandidateEvidence{}, Blocks: []PlannedBlock{{
				Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Verified prose.",
				Evidence: []CandidateEvidence{{
					ChunkID: chunks[0].ChunkID, Quotation: quotation,
					CharStart: 0, CharEnd: len([]rune(quotation)),
				}},
			}},
		}},
		QualityScore: 0.9,
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	return &ai.Result{JSON: raw, PromptKey: request.PromptKey, PromptVersion: 1, Model: "test"}, nil
}

func (g *retryingFidelityGenerator) Generate(_ context.Context, request ai.Request) (*ai.Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	g.feedback = append(g.feedback, request.Variables["validation_feedback"].(string))
	sourceVersionID, _ := uuid.Parse(request.Variables["source_version_id"].(string))
	audit := PlanFidelityAudit{
		SchemaVersion: 1, SourceVersionID: sourceVersionID, Complete: true,
		CoverageBefore: 0.9, CoverageAfter: 0.8, MissingBlocks: []PlanFidelityMissingBlock{},
	}
	if g.validAfter > 0 && g.calls >= g.validAfter {
		audit.CoverageAfter = audit.CoverageBefore
	}
	raw, err := json.Marshal(audit)
	if err != nil {
		return nil, err
	}
	return &ai.Result{JSON: raw, PromptKey: request.PromptKey, PromptVersion: 1, Model: "test"}, nil
}

func (g *consolidatingPlanGenerator) Generate(_ context.Context, request ai.Request) (*ai.Result, error) {
	sourceVersionID, _ := uuid.Parse(request.Variables["source_version_id"].(string))
	if request.PromptKey == ImportPlanFidelityPromptKey {
		g.mu.Lock()
		g.fidelityCalls++
		g.mu.Unlock()
		audit := PlanFidelityAudit{SchemaVersion: 1, SourceVersionID: sourceVersionID,
			Complete: true, CoverageBefore: 0.95, CoverageAfter: 0.95, MissingBlocks: []PlanFidelityMissingBlock{}}
		raw, err := json.Marshal(audit)
		if err != nil {
			return nil, err
		}
		return &ai.Result{JSON: raw, PromptKey: request.PromptKey, PromptVersion: 1, Model: "test"}, nil
	}
	plan := &ImportPlan{SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Profile:      SourceProfile{Title: "Source", Summary: "Summary", Language: "en", Useful: true, Subjects: []SourceSubject{}},
		QualityScore: 0.9, PromptInjectionDetected: false}
	if request.PromptKey == ImportPlanConsolidatePromptKey {
		g.mu.Lock()
		g.consolidateCalls++
		g.mu.Unlock()
		var draft ImportPlan
		if err := json.Unmarshal([]byte(request.Variables["draft_plan_json"].(string)), &draft); err != nil {
			return nil, err
		}
		evidenceItem := draft.Routes[0].Blocks[0].Evidence
		plan.Routes = []PageRoute{{Action: RouteCreate, Title: "Alpha", Reason: "consolidated", Confidence: 0.9,
			RelatedTo: []string{}, Evidence: []CandidateEvidence{}, Blocks: []PlannedBlock{{
				Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "A coherent article.", Evidence: evidenceItem,
			}}}}
	} else {
		g.mu.Lock()
		g.mapCalls++
		g.mu.Unlock()
		var chunks []struct {
			ChunkID uuid.UUID `json:"chunk_id"`
			Text    string    `json:"text"`
		}
		if err := json.Unmarshal([]byte(request.Variables["chunks_json"].(string)), &chunks); err != nil {
			return nil, err
		}
		item := CandidateEvidence{ChunkID: chunks[0].ChunkID, Quotation: chunks[0].Text,
			CharStart: 0, CharEnd: len([]rune(chunks[0].Text))}
		plan.Routes = []PageRoute{{Action: RouteCreate, Title: "Alpha", Reason: "window", Confidence: 0.8,
			RelatedTo: []string{}, Evidence: []CandidateEvidence{}, Blocks: []PlannedBlock{{
				Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: chunks[0].Text, Evidence: []CandidateEvidence{item},
			}}}}
	}
	normalizeImportPlanCollections(plan)
	raw, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	return &ai.Result{JSON: raw, PromptKey: request.PromptKey, PromptVersion: 1, Model: "test"}, nil
}

func TestValidateImportPlanRepairsEvidenceAndRejectsInventedTargets(t *testing.T) {
	sourceVersionID, chunkID := uuid.New(), uuid.New()
	pageID, inventedPageID := uuid.New(), uuid.New()
	blockID := uuid.New().String()
	chunks := []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID,
		TextContent: "Alpha is a documented topic.", LocatorJSON: json.RawMessage(`{"page":3}`),
	}}
	candidates := []PageCandidate{{PageID: pageID, Title: "Alpha", Blocks: []PageCandidateBlock{{
		ID: blockID, Type: string(ast.BlockParagraph), Text: "Old Alpha", Hash: HashBytes([]byte("old")),
	}}}}
	wrongChunkID := uuid.New()
	if start, ok := exactQuotationStart(chunks[0].TextContent, "Alpha is a documented topic.", 99); !ok || start != 0 {
		t.Fatalf("exact quotation lookup failed: start=%d ok=%v", start, ok)
	}
	repairedEvidence := newEvidenceCatalog(sourceVersionID, chunks).normalize([]CandidateEvidence{{
		ChunkID: wrongChunkID, Quotation: "Alpha is a documented topic.", CharStart: 99, CharEnd: 100,
	}})
	if len(repairedEvidence) != 1 {
		t.Fatal("evidence catalog did not repair a uniquely matching quotation")
	}
	plan := ImportPlan{
		SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Profile:      SourceProfile{Title: "Alpha source", Summary: "A source", Language: "en", Useful: true, Subjects: []SourceSubject{}},
		QualityScore: 0.9, Routes: []PageRoute{
			{Action: RouteUpdate, Title: "model title", PageID: &pageID, Reason: "new verified detail", Confidence: 0.9,
				RelatedTo: []string{}, Evidence: []CandidateEvidence{},
				Blocks: []PlannedBlock{{Type: string(ast.BlockParagraph), Mode: BlockReplace, TargetBlockID: &blockID,
					Text: "Alpha is a documented topic.", Evidence: []CandidateEvidence{{ChunkID: wrongChunkID, Quotation: "Alpha is a documented topic.", CharStart: 99, CharEnd: 100}}}}},
			{Action: RouteUpdate, Title: "invented", PageID: &inventedPageID, Reason: "invalid target", Confidence: 0.8,
				RelatedTo: []string{}, Evidence: []CandidateEvidence{},
				Blocks: []PlannedBlock{{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Alpha is a documented topic.",
					Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Alpha is a documented topic.", CharStart: 0, CharEnd: 28}}}}},
		},
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	validated, _, err := ValidateImportPlan(raw, sourceVersionID, chunks, candidates, RouteModeAuto, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(validated.Routes) != 1 || validated.Routes[0].PageID == nil || *validated.Routes[0].PageID != pageID {
		t.Fatalf("unexpected routes: %#v", validated.Routes)
	}
	if validated.Routes[0].Title != "Alpha" {
		t.Fatalf("title=%q, want authoritative candidate title", validated.Routes[0].Title)
	}
	item := validated.Routes[0].Blocks[0].Evidence[0]
	if item.ChunkID != chunkID || item.CharStart != 0 || item.CharEnd != 28 || item.Page == nil || *item.Page != 3 {
		t.Fatalf("evidence was not canonicalized: %#v", item)
	}
}

func TestGeneratePlanConsolidatesParallelSourceWindows(t *testing.T) {
	sourceVersionID := uuid.New()
	chunks := make([]evidence.SourceChunk, 7)
	for index := range chunks {
		text := "Evidence window " + string(rune('A'+index)) + "."
		chunks[index] = evidence.SourceChunk{ID: uuid.New(), SourceVersionID: sourceVersionID,
			Ordinal: index, TextContent: text, LocatorJSON: json.RawMessage(`{}`)}
	}
	generator := &consolidatingPlanGenerator{}
	planner := &PagePlanner{ai: generator}
	jobID, runID := uuid.New(), uuid.New()
	generated, err := planner.generatePlan(context.Background(), PlanParams{
		SourceVersionID: sourceVersionID, RouteMode: RouteModeAuto, Chunks: chunks,
		Provider: "test", Model: "test", MaxInputTokens: DefaultModelMaxInputTokens,
		ImportJobID: &jobID, ImportRunID: &runID,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	generator.mu.Lock()
	mapCalls, consolidateCalls, fidelityCalls := generator.mapCalls, generator.consolidateCalls, generator.fidelityCalls
	generator.mu.Unlock()
	if mapCalls != 2 || consolidateCalls != 1 || fidelityCalls != 0 {
		t.Fatalf("map calls=%d consolidate calls=%d fidelity calls=%d, want 2/1/0", mapCalls, consolidateCalls, fidelityCalls)
	}
	if generated.PromptKey != ImportPlanConsolidatePromptKey || len(generated.Plan.Routes) != 1 ||
		len(generated.Plan.Routes[0].Blocks) != 1 || generated.Plan.Routes[0].Blocks[0].Text != "A coherent article." {
		t.Fatalf("unexpected consolidated plan: %#v", generated)
	}
}

func TestGenerateValidatedPlanBatchRetriesSingleChunkEvidenceFailure(t *testing.T) {
	sourceVersionID := uuid.New()
	chunks := []evidence.SourceChunk{{
		ID: uuid.New(), SourceVersionID: sourceVersionID, Ordinal: 0,
		TextContent: "Clients MUST validate the assertion.", LocatorJSON: json.RawMessage(`{}`),
	}}
	generator := &retryingPlanGenerator{validAfter: planLeafValidationAttempts}
	planner := &PagePlanner{ai: generator}

	plan, _, err := planner.generateValidatedPlanBatch(context.Background(), PlanParams{
		SourceVersionID: sourceVersionID, Chunks: chunks, Provider: "test", Model: "test",
	}, nil, chunks)
	if err != nil {
		t.Fatal(err)
	}
	generator.mu.Lock()
	calls := generator.calls
	feedback := append([]string(nil), generator.feedback...)
	generator.mu.Unlock()
	if plan == nil || calls != planLeafValidationAttempts {
		t.Fatalf("plan=%#v calls=%d, want success on attempt %d", plan, calls, planLeafValidationAttempts)
	}
	if len(feedback) != planLeafValidationAttempts || feedback[0] != "No prior validation failure." ||
		!strings.Contains(feedback[1], "verifiable evidence") {
		t.Fatalf("unexpected validation feedback: %#v", feedback)
	}
}

func TestValidateImportPlanRestoresLineWrappedEvidenceVerbatim(t *testing.T) {
	sourceVersionID, chunkID := uuid.New(), uuid.New()
	sourceText := "Clients MUST validate\n      the assertion before use."
	modelQuotation := "Clients MUST validate the assertion before use."
	plan := ImportPlan{
		SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Profile: SourceProfile{Title: "Subject", Summary: "Summary", Language: "en", Useful: true, Subjects: []SourceSubject{}},
		Routes: []PageRoute{{
			Action: RouteCreate, Title: "Subject", Reason: "documented", Confidence: 0.9,
			RelatedTo: []string{}, Evidence: []CandidateEvidence{}, Blocks: []PlannedBlock{{
				Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "客户端必须在使用前验证断言。",
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: modelQuotation, CharStart: 0, CharEnd: len([]rune(modelQuotation))}},
			}},
		}},
		QualityScore: 0.9,
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}

	validated, _, err := ValidateImportPlan(raw, sourceVersionID, []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID, TextContent: sourceText, LocatorJSON: json.RawMessage(`{}`),
	}}, nil, RouteModeAuto, "")
	if err != nil {
		t.Fatal(err)
	}
	item := validated.Routes[0].Blocks[0].Evidence[0]
	if item.Quotation != sourceText || item.CharStart != 0 || item.CharEnd != len([]rune(sourceText)) {
		t.Fatalf("planning evidence was not restored verbatim: %#v", item)
	}
}

func TestValidatePlanFidelityAuditRepairsExactEvidenceAndRejectsInventedRoute(t *testing.T) {
	sourceVersionID, chunkID := uuid.New(), uuid.New()
	text := "Clients MUST reject an invalid JWT assertion."
	chunks := []evidence.SourceChunk{{ID: chunkID, SourceVersionID: sourceVersionID,
		TextContent: text, LocatorJSON: json.RawMessage(`{"page":7}`)}}
	plan := &ImportPlan{Routes: []PageRoute{{Action: RouteCreate, Title: "JWT profile",
		Blocks: []PlannedBlock{{Type: string(ast.BlockParagraph), Text: "The profile defines JWT assertions."}}}}}
	audit := PlanFidelityAudit{SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Complete: false, CoverageBefore: 0.6, CoverageAfter: 0.9,
		MissingBlocks: []PlanFidelityMissingBlock{{RouteIndex: 0, AfterHeading: "Invented section",
			Text: text, Evidence: []CandidateEvidence{{ChunkID: uuid.New(), Quotation: text, CharStart: 99, CharEnd: 100}}}}}
	raw, _ := json.Marshal(audit)
	validated, err := ValidatePlanFidelityAudit(raw, sourceVersionID, chunks, plan)
	if err != nil {
		t.Fatal(err)
	}
	if validated.MissingBlocks[0].AfterHeading != "" ||
		validated.MissingBlocks[0].Evidence[0].ChunkID != chunkID ||
		validated.MissingBlocks[0].Evidence[0].CharStart != 0 ||
		validated.MissingBlocks[0].Evidence[0].Page == nil || *validated.MissingBlocks[0].Evidence[0].Page != 7 {
		t.Fatalf("audit evidence was not canonicalized: %#v", validated.MissingBlocks[0])
	}
	audit.MissingBlocks[0].RouteIndex = 1
	raw, _ = json.Marshal(audit)
	if _, err := ValidatePlanFidelityAudit(raw, sourceVersionID, chunks, plan); !errors.Is(err, ai.ErrInvalidOutput) {
		t.Fatalf("error=%v, want ai.ErrInvalidOutput", err)
	}
}

func TestAuditPlanFidelityBatchRetriesSemanticValidationWithFeedback(t *testing.T) {
	sourceVersionID := uuid.New()
	chunks := []evidence.SourceChunk{{
		ID: uuid.New(), SourceVersionID: sourceVersionID, Ordinal: 0,
		TextContent: "A material fact.", LocatorJSON: json.RawMessage(`{}`),
	}}
	plan := &ImportPlan{Routes: []PageRoute{{
		Action: RouteCreate, Title: "Subject",
		Blocks: []PlannedBlock{{Type: string(ast.BlockParagraph), Text: "A material fact."}},
	}}}
	generator := &retryingFidelityGenerator{validAfter: 3}
	planner := &PagePlanner{ai: generator}

	audit, err := planner.auditPlanFidelityBatch(context.Background(), PlanParams{
		SourceVersionID: sourceVersionID, Chunks: chunks, Provider: "test", Model: "test",
	}, json.RawMessage(`[]`), plan, chunks)
	if err != nil {
		t.Fatal(err)
	}
	generator.mu.Lock()
	calls := generator.calls
	feedback := append([]string(nil), generator.feedback...)
	generator.mu.Unlock()
	if audit == nil || calls != planFidelityValidationAttempts {
		t.Fatalf("audit=%#v calls=%d, want success on attempt %d", audit, calls, planFidelityValidationAttempts)
	}
	if len(feedback) != planFidelityValidationAttempts || feedback[0] != "No prior validation failure." ||
		!strings.Contains(feedback[1], "coverage_after") {
		t.Fatalf("unexpected validation feedback: %#v", feedback)
	}
}

func TestAuditPlanFidelityBatchStopsAfterValidationAttemptLimit(t *testing.T) {
	sourceVersionID := uuid.New()
	chunks := []evidence.SourceChunk{{
		ID: uuid.New(), SourceVersionID: sourceVersionID,
		TextContent: "A material fact.", LocatorJSON: json.RawMessage(`{}`),
	}}
	generator := &retryingFidelityGenerator{}
	planner := &PagePlanner{ai: generator}
	plan := &ImportPlan{Routes: []PageRoute{{Action: RouteCreate, Title: "Subject"}}}

	_, err := planner.auditPlanFidelityBatch(context.Background(), PlanParams{
		SourceVersionID: sourceVersionID, Provider: "test", Model: "test",
	}, json.RawMessage(`[]`), plan, chunks)
	if !errors.Is(err, ai.ErrInvalidOutput) {
		t.Fatalf("error=%v, want ai.ErrInvalidOutput", err)
	}
	generator.mu.Lock()
	calls := generator.calls
	generator.mu.Unlock()
	if calls != planFidelityValidationAttempts {
		t.Fatalf("calls=%d, want %d", calls, planFidelityValidationAttempts)
	}
}

func TestPlanFidelityOutcomeErrorPrefersCausalValidationFailure(t *testing.T) {
	causalErr := fmt.Errorf("%w: fidelity route", ai.ErrInvalidOutput)
	err := planFidelityOutcomeError(context.Background(), []planFidelityOutcome{
		{err: context.Canceled},
		{err: causalErr},
		{err: context.Canceled},
	})
	if !errors.Is(err, ai.ErrInvalidOutput) {
		t.Fatalf("error=%v, want causal validation failure", err)
	}
	if code := planErrorCode(err); code != "page_plan_invalid_output" {
		t.Fatalf("code=%q, want page_plan_invalid_output", code)
	}
}

func TestApplyPlanFidelityAuditsRepairsSectionWithoutDuplicating(t *testing.T) {
	evidenceItem := []CandidateEvidence{{ChunkID: uuid.New(), Quotation: "MUST reject", CharStart: 0, CharEnd: 11}}
	plan := &ImportPlan{Routes: []PageRoute{{Action: RouteCreate, Title: "JWT profile", Blocks: []PlannedBlock{
		{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "The profile defines JWT assertions.", Evidence: evidenceItem},
		{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Processing", Level: 2, Evidence: evidenceItem},
		{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Servers validate the assertion.", Evidence: evidenceItem},
	}}}}
	audits := []*PlanFidelityAudit{{MissingBlocks: []PlanFidelityMissingBlock{
		{RouteIndex: 0, AfterHeading: "Processing", Text: "Clients MUST reject invalid assertions.", Evidence: evidenceItem},
		{RouteIndex: 0, AfterHeading: "Processing", Text: "Clients MUST reject invalid assertions.", Evidence: evidenceItem},
	}}}
	if applied := applyPlanFidelityAudits(plan, audits); applied != 1 {
		t.Fatalf("applied=%d, want 1", applied)
	}
	if len(plan.Routes[0].Blocks) != 4 || plan.Routes[0].Blocks[3].Text != "Clients MUST reject invalid assertions." {
		t.Fatalf("unexpected repaired blocks: %#v", plan.Routes[0].Blocks)
	}
}

func TestAssessImportPlanQualityUsesServerMetrics(t *testing.T) {
	evidenceItem := []CandidateEvidence{{ChunkID: uuid.New(), Quotation: "The profile defines JWT assertions and validation requirements.", CharStart: 0, CharEnd: 63}}
	plan := &ImportPlan{Profile: SourceProfile{Useful: true}, QualityScore: 0.99,
		Routes: []PageRoute{{Action: RouteCreate, Title: "JWT profile", Confidence: 0.9, Blocks: []PlannedBlock{
			{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "The profile defines JWT assertions and validation requirements.", Evidence: evidenceItem},
			{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Processing", Level: 2, Evidence: evidenceItem},
			{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Clients validate JWT assertions before accepting them.", Evidence: evidenceItem},
		}}}}
	chunks := []evidence.SourceChunk{{TextContent: strings.Repeat("source material ", 20)}}
	good := assessImportPlanQuality(plan, chunks, 0.9)
	if good < DefaultQualityThreshold {
		t.Fatalf("good score=%f, want >= threshold", good)
	}
	if bad := assessImportPlanQuality(plan, chunks, 0.5); bad >= DefaultQualityThreshold {
		t.Fatalf("bad score=%f, want below threshold", bad)
	}
	headingHeavy := *plan
	headingHeavy.Routes = append([]PageRoute(nil), plan.Routes...)
	headingHeavy.Routes[0].Blocks = append([]PlannedBlock(nil), plan.Routes[0].Blocks...)
	for index := 0; index < 8; index++ {
		headingHeavy.Routes[0].Blocks = append(headingHeavy.Routes[0].Blocks,
			PlannedBlock{Type: string(ast.BlockHeading), Text: "Thin section " + string(rune('A'+index)), Level: 2, Evidence: evidenceItem},
			PlannedBlock{Type: string(ast.BlockParagraph), Text: "One short sentence.", Evidence: evidenceItem})
	}
	if score := assessImportPlanQuality(&headingHeavy, chunks, 0.9); score >= DefaultQualityThreshold {
		t.Fatalf("heading-heavy score=%f, want below threshold", score)
	}
}

func TestValidateImportPlanForceCreateRequiresExactTitle(t *testing.T) {
	sourceVersionID, chunkID := uuid.New(), uuid.New()
	chunks := []evidence.SourceChunk{{ID: chunkID, SourceVersionID: sourceVersionID, TextContent: "Evidence text", LocatorJSON: json.RawMessage(`{}`)}}
	plan := ImportPlan{
		SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Profile:      SourceProfile{Title: "Source", Summary: "", Language: "en", Useful: true, Subjects: []SourceSubject{}},
		QualityScore: 0.9, Routes: []PageRoute{{Action: RouteCreate, Title: "Different", Reason: "new", Confidence: 0.9,
			RelatedTo: []string{}, Evidence: []CandidateEvidence{},
			Blocks: []PlannedBlock{{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Evidence text",
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Evidence text", CharStart: 0, CharEnd: 13}}}}}},
	}
	raw, _ := json.Marshal(plan)
	_, _, err := ValidateImportPlan(raw, sourceVersionID, chunks, nil, RouteModeForceCreate, "Wanted")
	if !errors.Is(err, ErrNoPagePlan) {
		t.Fatalf("error=%v, want ErrNoPagePlan", err)
	}
}

func TestMergePlannedBlocksKeepsOnlyOneReplacementPerTarget(t *testing.T) {
	blockID := uuid.New().String()
	merged := mergePlannedBlocks(nil, []PlannedBlock{
		{Type: string(ast.BlockParagraph), Mode: BlockReplace, TargetBlockID: &blockID, Text: "first"},
		{Type: string(ast.BlockParagraph), Mode: BlockReplace, TargetBlockID: &blockID, Text: "second"},
	})
	if len(merged) != 1 || merged[0].Text != "first" {
		t.Fatalf("unexpected replacement merge: %#v", merged)
	}
}

func TestCompilePlannedHeadingKeepsEvidenceOutOfVisibleTitle(t *testing.T) {
	composer := &ProposalComposer{ids: id.NewGenerator()}
	citationID := uuid.New()
	block, err := composer.compilePlannedBlock(PlannedBlock{
		Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Verified heading", Level: 2,
	}, []uuid.UUID{citationID})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := block.InlineContent()
	if err != nil {
		t.Fatal(err)
	}
	if block.Level != 2 || len(nodes) != 1 || nodes[0].Type != ast.InlineText || nodes[0].Text != "Verified heading" {
		t.Fatalf("unexpected heading block: %#v nodes=%#v", block, nodes)
	}
}

func TestRefineImportPlanRemovesBatchStitchingArtifacts(t *testing.T) {
	if canonicalHeadingTopic("Authorization Grant Processing") == "authors" {
		t.Fatal("authorization heading must not be classified as an author section")
	}
	evidenceItem := []CandidateEvidence{{ChunkID: uuid.New(), Quotation: "evidence", CharStart: 0, CharEnd: 8}}
	plan := &ImportPlan{Profile: SourceProfile{Subjects: []SourceSubject{
		{Title: "M. Jones", Kind: "person"},
		{Title: "Michael B. Jones", Kind: "person"},
	}}, Routes: []PageRoute{{Action: RouteCreate, Title: "RFC 7523", Blocks: []PlannedBlock{
		{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Overview", Level: 1, Evidence: evidenceItem},
		{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "RFC 7523 defines JWT use. It is an OAuth profile.", Evidence: evidenceItem},
		{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Empty section", Level: 2, Evidence: evidenceItem},
		{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Details", Level: 2, Evidence: evidenceItem},
		{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "RFC 7523 defines JWT use. It specifies token processing.", Evidence: evidenceItem},
		{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "References", Level: 2, Evidence: evidenceItem},
		{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "A bibliography dump.", Evidence: evidenceItem},
		{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Authors and Publication", Level: 2, Evidence: evidenceItem},
		{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Michael B. Jones authored the work.", Evidence: evidenceItem},
		{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Authors", Level: 2, Evidence: evidenceItem},
		{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Michael B. Jones authored the work.", Evidence: evidenceItem},
	}}}}

	refineImportPlan(plan)
	if len(plan.Profile.Subjects) != 1 || plan.Profile.Subjects[0].Title != "Michael B. Jones" {
		t.Fatalf("subjects were not consolidated: %#v", plan.Profile.Subjects)
	}
	blocks := plan.Routes[0].Blocks
	if len(blocks) != 5 {
		t.Fatalf("blocks=%d, want 5: %#v", len(blocks), blocks)
	}
	if blocks[0].Type != string(ast.BlockParagraph) || strings.Contains(blocks[0].Text, "Overview") {
		t.Fatalf("new page did not retain a lead paragraph: %#v", blocks[0])
	}
	combined := ""
	for _, block := range blocks {
		combined += "\n" + block.Text
	}
	for _, unwanted := range []string{"Empty section", "References", "bibliography dump", "\nAuthors\n"} {
		if strings.Contains(combined, unwanted) {
			t.Fatalf("refinement retained %q in %q", unwanted, combined)
		}
	}
	if strings.Count(combined, "RFC 7523 defines JWT use.") != 1 ||
		strings.Count(combined, "Michael B. Jones authored the work.") != 1 {
		t.Fatalf("duplicate sentences survived: %q", combined)
	}
}

func TestCompilePlannedBlockLinksSelectedEntities(t *testing.T) {
	composer := &ProposalComposer{ids: id.NewGenerator()}
	workID, personID := uuid.New(), uuid.New()
	block, err := composer.compilePlannedBlockWithReferences(PlannedBlock{
		Type: string(ast.BlockParagraph), Mode: BlockAppend,
		Text: "RFC 7523 was authored by Michael B. Jones.",
	}, nil, []entityReferenceTarget{
		{EntityID: workID, Label: "RFC 7523"},
		{EntityID: personID, Label: "Michael B. Jones"},
	})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := block.InlineContent()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 4 || nodes[0].Type != ast.InlineEntityReference || nodes[0].EntityID != workID.String() ||
		nodes[2].Type != ast.InlineEntityReference || nodes[2].EntityID != personID.String() {
		t.Fatalf("unexpected linked prose: %#v", nodes)
	}
}

func TestCompileRelatedPagesBlocksUseStandardSeeAlsoAndDeduplicate(t *testing.T) {
	composer := &ProposalComposer{ids: id.NewGenerator()}
	sourceID, existingID, targetID := uuid.New(), uuid.New(), uuid.New()
	blocks, err := composer.compileRelatedPagesBlocks("zh-Hans", sourceID, []relatedPageTarget{
		{PageID: sourceID, Title: "自己"},
		{PageID: existingID, Title: "已有"},
		{PageID: targetID, Title: "目标"},
		{PageID: targetID, Title: "重复目标"},
	}, map[uuid.UUID]bool{existingID: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Type != ast.BlockHeading || blocks[0].Level != 2 || blocks[1].Type != ast.BlockBulletList {
		t.Fatalf("unexpected See also blocks: %#v", blocks)
	}
	heading, err := blocks[0].InlineContent()
	if err != nil {
		t.Fatal(err)
	}
	if len(heading) != 1 || heading[0].Text != "参见" {
		t.Fatalf("unexpected See also heading: %#v", heading)
	}
	if len(blocks[1].Children) != 1 || len(blocks[1].Children[0].Children) != 1 {
		t.Fatalf("unexpected See also list: %#v", blocks[1])
	}
	nodes, err := blocks[1].Children[0].Children[0].InlineContent()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Type != ast.InlinePageReference || nodes[0].TargetPageID != targetID.String() {
		t.Fatalf("unexpected related-page nodes: %#v", nodes)
	}
}

func TestPrimaryEntityLabelScoreRejectsGenericTitleFragments(t *testing.T) {
	tests := []struct {
		name, title, label string
		want               int
	}{
		{name: "exact", title: "Semantic Kernel", label: "Semantic Kernel", want: 100},
		{name: "specific phrase", title: "Microsoft Semantic Kernel", label: "Semantic Kernel", want: 75},
		{name: "short acronym", title: "OpenAI API", label: "AI", want: 0},
		{name: "generic fragment", title: "Anby Wiki architecture", label: "Wiki", want: 0},
		{name: "CJK subject", title: "中华人民共和国历史", label: "中华人民共和国", want: 75},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := primaryEntityLabelScore(
				normalizedIdentityText(test.title), test.label,
			); got != test.want {
				t.Fatalf("score=%d, want %d", got, test.want)
			}
		})
	}
}

func TestInspectArticleEndMatterKeepsSeeAlsoAsSingleTerminalSection(t *testing.T) {
	heading := func(title string) *ast.Block {
		content, err := json.Marshal([]*ast.InlineNode{{Type: ast.InlineText, Text: title}})
		if err != nil {
			t.Fatal(err)
		}
		return &ast.Block{
			ID: uuid.NewString(), Type: ast.BlockHeading, Level: 2, Content: content,
		}
	}
	seeAlsoList := &ast.Block{
		ID: uuid.NewString(), Type: ast.BlockBulletList, Children: []*ast.Block{},
	}
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{
			{ID: uuid.NewString(), Type: ast.BlockParagraph, Content: json.RawMessage(`[{"type":"text","text":"body"}]`)},
			heading("参见"), seeAlsoList, heading("拓展阅读"),
		},
	}
	endMatter := inspectArticleEndMatter(document)
	if endMatter.FirstIndex != 1 || endMatter.SeeAlsoIndex != 1 || endMatter.SeeAlsoList != seeAlsoList {
		t.Fatalf("unexpected end-matter inspection: %#v", endMatter)
	}
}

func TestNormalizePlanLinksKeepsExplicitGroundedEdgesOnly(t *testing.T) {
	targetID := uuid.New()
	evidenceItem := CandidateEvidence{ChunkID: uuid.New(), Quotation: "related", CharStart: 0, CharEnd: 7}
	routes := normalizePlanLinks([]PageRoute{
		{Action: RouteCreate, Title: "Alpha", RelatedTo: []string{}, Evidence: []CandidateEvidence{}},
		{Action: RouteUpdate, Title: "Beta", RelatedTo: []string{}, Evidence: []CandidateEvidence{}},
		{Action: RouteLink, Title: "Gamma", PageID: &targetID,
			RelatedTo: []string{" alpha ", "invented", "ALPHA", "Beta"}, Evidence: []CandidateEvidence{evidenceItem}},
	})
	if len(routes) != 3 {
		t.Fatalf("unexpected routes: %#v", routes)
	}
	link := routes[2]
	if len(link.RelatedTo) != 2 || link.RelatedTo[0] != "Alpha" || link.RelatedTo[1] != "Beta" {
		t.Fatalf("unexpected explicit link mapping: %#v", link.RelatedTo)
	}
}

func TestValidateImportPlanAcceptsGroundedExplicitLink(t *testing.T) {
	sourceVersionID, chunkID, targetPageID := uuid.New(), uuid.New(), uuid.New()
	text := "Alpha is related to Gamma."
	chunks := []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID, TextContent: text, LocatorJSON: json.RawMessage(`{}`),
	}}
	plan := ImportPlan{
		SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Profile:      SourceProfile{Title: "Alpha", Summary: "", Language: "en", Useful: true, Subjects: []SourceSubject{}},
		QualityScore: 0.9,
		Routes: []PageRoute{
			{Action: RouteCreate, Title: "Alpha", Reason: "new subject", Confidence: 0.9,
				RelatedTo: []string{}, Evidence: []CandidateEvidence{},
				Blocks: []PlannedBlock{{Type: string(ast.BlockParagraph), Mode: BlockAppend,
					Text: text, Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: text, CharStart: 0, CharEnd: len([]rune(text))}}}}},
			{Action: RouteLink, Title: "Gamma", PageID: &targetPageID, Reason: "explicit relation", Confidence: 0.8,
				RelatedTo: []string{"Alpha"},
				Evidence:  []CandidateEvidence{{ChunkID: chunkID, Quotation: text, CharStart: 0, CharEnd: len([]rune(text))}},
				Blocks:    []PlannedBlock{}},
		},
	}
	raw, _ := json.Marshal(plan)
	validated, _, err := ValidateImportPlan(raw, sourceVersionID, chunks,
		[]PageCandidate{{PageID: targetPageID, Title: "Gamma", Blocks: []PageCandidateBlock{}}}, RouteModeAuto, "")
	if err != nil {
		t.Fatal(err)
	}
	validated.Routes = normalizePlanLinks(validated.Routes)
	if len(validated.Routes) != 2 || len(validated.Routes[1].RelatedTo) != 1 ||
		validated.Routes[1].RelatedTo[0] != "Alpha" || len(validated.Routes[1].Evidence) != 1 {
		t.Fatalf("unexpected grounded link: %#v", validated.Routes)
	}
}

func TestNormalizeImportPlanCollectionsProducesContractArrays(t *testing.T) {
	plan := &ImportPlan{
		SchemaVersion:   1,
		SourceVersionID: uuid.New(),
		Profile: SourceProfile{
			Title: "Source", Summary: "Summary", Language: "en", Useful: true,
		},
		Routes: []PageRoute{{
			Action: RouteCreate, Title: "Page", Reason: "new", Confidence: 0.9,
			Blocks: []PlannedBlock{{
				Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Text",
			}},
		}},
		QualityScore: 0.9,
	}

	normalizeImportPlanCollections(plan)
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	profile := document["profile"].(map[string]any)
	routes := document["routes"].([]any)
	route := routes[0].(map[string]any)
	blocks := route["blocks"].([]any)
	block := blocks[0].(map[string]any)
	for name, value := range map[string]any{
		"profile.subjects": profile["subjects"],
		"route.related_to": route["related_to"],
		"route.evidence":   route["evidence"],
		"route.blocks":     route["blocks"],
		"block.evidence":   block["evidence"],
	} {
		if _, ok := value.([]any); !ok {
			t.Fatalf("%s encoded as %T (%v), want JSON array", name, value, value)
		}
	}
}
