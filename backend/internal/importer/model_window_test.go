package importer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
)

func TestSplitModelWindowRemapsEvidenceToImmutableChunk(t *testing.T) {
	sourceVersionID, chunkID := uuid.New(), uuid.New()
	first := strings.Repeat("Alpha sentence. ", 300)
	second := "Beta material fact. " + strings.Repeat("Gamma sentence. ", 300)
	text := first + second
	original := evidence.SourceChunk{
		ID: chunkID, SourceVersionID: sourceVersionID, Ordinal: 4,
		TextContent: text, TextHash: HashBytes([]byte(text)), LocatorJSON: json.RawMessage(`{"page":7,"char_start":100}`),
	}
	left, right, ok := splitModelSourceChunks(initialModelSourceChunks([]evidence.SourceChunk{original}))
	if !ok || len(left) != 1 || len(right) != 1 || right[0].View.ID == chunkID {
		t.Fatalf("window was not split: left=%#v right=%#v", left, right)
	}
	quotation := "Beta material fact."
	localStart := strings.Index(right[0].View.TextContent, quotation)
	if localStart < 0 {
		t.Fatal("expected quotation in right window")
	}
	mapped := remapCandidateEvidence([]CandidateEvidence{{
		ChunkID: right[0].View.ID, Quotation: quotation,
		CharStart: len([]rune(right[0].View.TextContent[:localStart])),
		CharEnd:   len([]rune(right[0].View.TextContent[:localStart+len(quotation)])),
	}}, right)
	if len(mapped) != 1 || mapped[0].ChunkID != chunkID || mapped[0].Page == nil || *mapped[0].Page != 7 {
		t.Fatalf("evidence was not mapped to immutable chunk: %#v", mapped)
	}
	wantStart := len([]rune(text[:strings.Index(text, quotation)]))
	if mapped[0].CharStart != wantStart || mapped[0].CharEnd != wantStart+len([]rune(quotation)) {
		t.Fatalf("mapped range=%d:%d, want %d:%d", mapped[0].CharStart, mapped[0].CharEnd,
			wantStart, wantStart+len([]rune(quotation)))
	}
}

func TestDecodeGeneratedImportPlanDerivesMechanicalFields(t *testing.T) {
	sourceVersionID, chunkID := uuid.New(), uuid.New()
	text := "Clients MUST validate the assertion before use."
	raw := []byte(`{
      "profile":{"title":"JWT profile","summary":"A profile.","language":"en","useful":true},
      "routes":[{"action":"create","title":"JWT profile","reason":"documented","confidence":0.9,
        "blocks":[{"type":"paragraph","text":"Clients must validate assertions.",
          "evidence":[{"chunk_id":"` + chunkID.String() + `","quotation":"` + text + `"}]}]}],
      "prompt_injection_detected":false
    }`)
	plan, canonical, err := DecodeGeneratedImportPlan(raw, sourceVersionID, []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID, TextContent: text, LocatorJSON: json.RawMessage(`{"page":2}`),
	}}, nil, RouteModeAuto)
	if err != nil {
		t.Fatal(err)
	}
	item := plan.Routes[0].Blocks[0].Evidence[0]
	if plan.SchemaVersion != 1 || plan.SourceVersionID != sourceVersionID ||
		plan.Routes[0].Blocks[0].Mode != BlockAppend || item.CharStart != 0 ||
		item.CharEnd != len([]rune(text)) || item.Page == nil || *item.Page != 2 {
		t.Fatalf("server fields were not derived: plan=%#v evidence=%#v", plan, item)
	}
	if !json.Valid(canonical) {
		t.Fatal("canonical plan is not JSON")
	}
}

func TestBatchModelSourceChunksFitsOversizedPersistentChunkToBudget(t *testing.T) {
	sourceVersionID := uuid.New()
	text := strings.Repeat("密集内容。", 5000)
	batches := batchModelSourceChunks(initialModelSourceChunks([]evidence.SourceChunk{{
		ID: uuid.New(), SourceVersionID: sourceVersionID, TextContent: text,
		TextHash: HashBytes([]byte(text)), LocatorJSON: json.RawMessage(`{}`),
	}}), 4096)
	if len(batches) < 2 {
		t.Fatalf("oversized persistent chunk was not split into model windows: %d batch", len(batches))
	}
	for _, batch := range batches {
		for _, chunk := range batch {
			if estimateChunkInputTokens(chunk.View) > 4096-extractionInputMinReserve {
				t.Fatalf("model window remains over budget: %d tokens", estimateChunkInputTokens(chunk.View))
			}
		}
	}
}

func TestBatchModelSourceChunksFitsMinimumConfiguredCJKChunkToBudget(t *testing.T) {
	sourceVersionID := uuid.New()
	text := strings.Repeat("密", 1000)
	budget := 4096 - extractionInputMinReserve
	batches := batchModelSourceChunks(initialModelSourceChunks([]evidence.SourceChunk{{
		ID: uuid.New(), SourceVersionID: sourceVersionID, TextContent: text,
		TextHash: HashBytes([]byte(text)), LocatorJSON: json.RawMessage(`{}`),
	}}), 4096)
	if len(batches) < 2 {
		t.Fatalf("minimum configured CJK chunk was not split to fit: %d batch", len(batches))
	}
	for _, batch := range batches {
		for _, chunk := range batch {
			if estimateChunkInputTokens(chunk.View) > budget {
				t.Fatalf("CJK model window remains over budget: %d tokens", estimateChunkInputTokens(chunk.View))
			}
		}
	}
}
