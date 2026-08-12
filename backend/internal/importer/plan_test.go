package importer

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/id"
)

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

func TestCompilePlannedHeadingCarriesCitation(t *testing.T) {
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
	if block.Level != 2 || len(nodes) != 2 || nodes[1].Type != ast.InlineCitationReference || nodes[1].CitationID != citationID.String() {
		t.Fatalf("unexpected heading block: %#v nodes=%#v", block, nodes)
	}
}

func TestCompileRelatedPagesBlockUsesStableReferencesAndDeduplicates(t *testing.T) {
	composer := &ProposalComposer{ids: id.NewGenerator()}
	sourceID, existingID, targetID := uuid.New(), uuid.New(), uuid.New()
	citationID := uuid.New()
	block, err := composer.compileRelatedPagesBlock("zh-Hans", sourceID, []relatedPageTarget{
		{PageID: sourceID, Title: "自己"},
		{PageID: existingID, Title: "已有"},
		{PageID: targetID, Title: "目标"},
		{PageID: targetID, Title: "重复目标"},
	}, map[uuid.UUID]bool{existingID: true}, []uuid.UUID{citationID})
	if err != nil {
		t.Fatal(err)
	}
	if block == nil {
		t.Fatal("expected a related-pages block")
	}
	nodes, err := block.InlineContent()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 || nodes[0].Text != "相关页面：" ||
		nodes[1].Type != ast.InlinePageReference || nodes[1].TargetPageID != targetID.String() ||
		nodes[2].Type != ast.InlineCitationReference || nodes[2].CitationID != citationID.String() {
		t.Fatalf("unexpected related-page nodes: %#v", nodes)
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
