package importer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/id"
)

func TestEnsurePlanPreformattedBlocksPreservesASCIIFlowDiagram(t *testing.T) {
	sourceVersionID, chunkID, pageID := uuid.New(), uuid.New(), uuid.New()
	diagram := "IMP to HOST Message\n" +
		"       +-------+\n" +
		"       | Start |\n" +
		"       +---+---+\n" +
		"           |\n" +
		"           V\n" +
		"       +-------+\n" +
		"       |  End  |\n" +
		"       +-------+"
	source := "Introductory prose.\n\n\f\nRFC 12 header\n\n" + diagram + "\n\nClosing prose."
	chunks := []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID,
		TextContent: source, LocatorJSON: json.RawMessage(`{"page":2}`),
	}}
	plan := &ImportPlan{Routes: []PageRoute{{
		Action: RouteUpdate, PageID: &pageID, Title: "RFC 12",
		Blocks: []PlannedBlock{{
			Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Introductory prose.",
			Evidence: []CandidateEvidence{{
				ChunkID: chunkID, Quotation: "Introductory prose.", CharStart: 0, CharEnd: 19,
			}},
		}},
	}}}

	if added := ensurePlanPreformattedBlocks(plan, chunks, "Preserve all diagrams."); added != 1 {
		t.Fatalf("added=%d, want 1", added)
	}
	code := plan.Routes[0].Blocks[1]
	if code.Type != string(ast.BlockCode) || code.Text != diagram || len(code.Evidence) != 1 {
		t.Fatalf("unexpected code block: %#v", code)
	}
	item := code.Evidence[0]
	runes := []rune(source)
	if item.Page == nil || *item.Page != 2 ||
		item.Quotation != diagram ||
		string(runes[item.CharStart:item.CharEnd]) != diagram {
		t.Fatalf("unexpected code evidence: %#v", item)
	}
	if coverage := preformattedPlanCoverage(plan, chunks, ""); coverage != 1 {
		t.Fatalf("coverage=%f, want 1", coverage)
	}

	missing := &ImportPlan{Routes: []PageRoute{{
		Action: RouteUpdate, PageID: &pageID, Title: "RFC 12",
		Blocks: []PlannedBlock{{Type: string(ast.BlockParagraph), Text: "Summary"}},
	}}}
	if coverage := preformattedPlanCoverage(missing, chunks, ""); coverage != 0 {
		t.Fatalf("missing coverage=%f, want 0", coverage)
	}
}

func TestEnsurePlanPreformattedBlocksReplacesOverlappingFragmentsInSourceOrder(t *testing.T) {
	sourceVersionID, chunkID, pageID := uuid.New(), uuid.New(), uuid.New()
	formula := "Formula alpha\nFormula beta"
	source := "First diagram\n" +
		"+-------+\n" +
		"| Start |\n" +
		"+---+---+\n" +
		"    |\n" +
		"    V\n" +
		"+-------+\n" +
		"|  End  |\n" +
		"+-------+\n\n" +
		formula + "\n" +
		"\fRFC header\n\n" +
		"Second diagram\n" +
		"+-------+\n" +
		"| Input |\n" +
		"+---+---+\n" +
		"    |\n" +
		"    V\n" +
		"+-------+\n" +
		"|Output |\n" +
		"+-------+"
	chunks := []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID, TextContent: source,
	}}
	detected := sourcePreformattedBlocks(chunks)
	if len(detected) != 2 {
		t.Fatalf("detected=%d, want 2", len(detected))
	}
	first := detected[0]
	firstRunes := []rune(first.text)
	split := len(firstRunes) / 2
	firstHalf := string(firstRunes[:split])
	secondHalf := string(firstRunes[split:])
	formulaStart := strings.Index(source, formula)
	plan := &ImportPlan{Routes: []PageRoute{{
		Action: RouteUpdate, PageID: &pageID, Title: "Diagrams",
		Blocks: []PlannedBlock{
			{Type: string(ast.BlockHeading), Mode: BlockAppend, Text: "Generated heading"},
			{Type: string(ast.BlockParagraph), Mode: BlockAppend, Text: "Generated prose."},
			{
				Type: string(ast.BlockCode), Mode: BlockAppend, Text: detected[1].text,
				Evidence: []CandidateEvidence{detected[1].evidence},
			},
			{
				Type: string(ast.BlockCode), Mode: BlockAppend, Text: firstHalf,
				Evidence: []CandidateEvidence{{
					ChunkID: chunkID, Quotation: firstHalf,
					CharStart: first.evidence.CharStart,
					CharEnd:   first.evidence.CharStart + split,
				}},
			},
			{
				Type: string(ast.BlockCode), Mode: BlockAppend, Text: secondHalf,
				Evidence: []CandidateEvidence{{
					ChunkID: chunkID, Quotation: secondHalf,
					CharStart: first.evidence.CharStart + split,
					CharEnd:   first.evidence.CharEnd,
				}},
			},
			{
				Type: string(ast.BlockCode), Mode: BlockAppend, Text: formula,
				Evidence: []CandidateEvidence{{
					ChunkID: chunkID, Quotation: formula,
					CharStart: formulaStart, CharEnd: formulaStart + len([]rune(formula)),
				}},
			},
		},
	}}}

	instructions := "Add every diagram as exact code blocks. Do not add, rewrite, or duplicate prose."
	if added := ensurePlanPreformattedBlocks(plan, chunks, instructions); added != 1 {
		t.Fatalf("added=%d, want 1", added)
	}
	blocks := plan.Routes[0].Blocks
	if len(blocks) != 3 ||
		blocks[0].Text != detected[0].text ||
		blocks[1].Text != formula ||
		blocks[2].Text != detected[1].text {
		t.Fatalf("unexpected reconciled blocks: %#v", blocks)
	}
	for _, block := range blocks {
		if block.Type != string(ast.BlockCode) {
			t.Fatalf("unexpected non-code block: %#v", block)
		}
	}
}

func TestCompilePlannedCodePreservesWhitespace(t *testing.T) {
	composer := &ProposalComposer{ids: id.NewGenerator()}
	text := "  +---+\n  | A |\n  +---+"
	block, err := composer.compilePlannedBlock(PlannedBlock{
		Type: string(ast.BlockCode), Mode: BlockAppend, Text: text,
	}, []uuid.UUID{uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	content, err := block.TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if block.Type != ast.BlockCode || content != text {
		t.Fatalf("unexpected code block: %#v content=%q", block, content)
	}
}

func TestDecodeGeneratedImportPlanAcceptsExactCodeEvidence(t *testing.T) {
	sourceVersionID, chunkID := uuid.New(), uuid.New()
	text := "  +---+\n  | A |\n  +---+"
	raw, err := json.Marshal(generatedImportPlanDocument{
		Profile: SourceProfile{
			Title: "Diagram", Summary: "A fixed-width diagram.", Language: "en", Useful: true,
			Subjects: []SourceSubject{},
		},
		Routes: []generatedPageRoute{{
			Action: RouteCreate, Title: "Diagram", Reason: "The source contains a diagram.", Confidence: 0.9,
			Blocks: []generatedPlannedBlock{{
				Type: string(ast.BlockCode), Text: text,
				Evidence: []generatedPlanEvidence{{ChunkID: chunkID, Quotation: text}},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, _, err := DecodeGeneratedImportPlan(raw, sourceVersionID, []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID, TextContent: text,
	}}, nil, RouteModeAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Routes) != 1 || len(plan.Routes[0].Blocks) != 1 ||
		plan.Routes[0].Blocks[0].Type != string(ast.BlockCode) ||
		plan.Routes[0].Blocks[0].Text != text {
		t.Fatalf("unexpected decoded plan: %#v", plan)
	}
}

func TestPlanConcisionDoesNotMergeDistinctDiagrams(t *testing.T) {
	plan := &ImportPlan{Routes: []PageRoute{{
		Action: RouteCreate, Blocks: []PlannedBlock{
			{Type: string(ast.BlockCode), Text: "+---+\n| A |\n+---+"},
			{Type: string(ast.BlockCode), Text: "+---+\n| B |\n+---+"},
		},
	}}}
	if score := planConcisionScore(plan); score != 1 {
		t.Fatalf("distinct diagram concision=%f, want 1", score)
	}
	plan.Routes[0].Blocks = append(plan.Routes[0].Blocks, plan.Routes[0].Blocks[0])
	if score := planConcisionScore(plan); score >= 1 {
		t.Fatalf("duplicate diagram concision=%f, want below 1", score)
	}
}

func TestImportPlanExpansionBudgetSeparatesPreformattedContent(t *testing.T) {
	pageID := uuid.New()
	blocks := make([]PlannedBlock, 4)
	for index := range blocks {
		blocks[index] = PlannedBlock{
			Type: string(ast.BlockCode), Mode: BlockAppend,
			Text: strings.Repeat(string(rune('A'+index)), 2000),
		}
	}
	plan := &ImportPlan{Routes: []PageRoute{{
		Action: RouteUpdate, PageID: &pageID, Blocks: blocks,
	}}}
	candidates := []PageCandidate{{
		PageID: pageID, Blocks: []PageCandidateBlock{{
			Type: string(ast.BlockParagraph), Text: "Existing article.",
		}},
	}}
	if !importPlanExpansionWithinBudget(plan, candidates, "") {
		t.Fatal("bounded preformatted content should not consume the prose expansion budget")
	}

	plan.Routes[0].Blocks = make([]PlannedBlock, preformattedMaxPlanBlocks+1)
	for index := range plan.Routes[0].Blocks {
		plan.Routes[0].Blocks[index] = PlannedBlock{
			Type: string(ast.BlockCode), Mode: BlockAppend, Text: "diagram",
		}
	}
	if importPlanExpansionWithinBudget(plan, candidates, "") {
		t.Fatal("unbounded preformatted block count should fail the expansion budget")
	}
}
