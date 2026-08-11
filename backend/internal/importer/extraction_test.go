package importer

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
)

func TestValidateCandidatesFromChunksRepairsUniqueWrongChunkAndDropsBadCandidate(t *testing.T) {
	sourceVersionID := uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d197")
	actualChunkID := uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d198")
	wrongChunkID := uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d199")
	chunks := []evidence.SourceChunk{{
		ID: actualChunkID, SourceVersionID: sourceVersionID, Ordinal: 0,
		TextContent: "prefix Unique fact suffix", LocatorJSON: []byte(`{"page":2}`),
	}}
	candidates := Candidates{
		SchemaVersion: 1, SourceVersionID: sourceVersionID, QualityScore: 0.9,
		Entities: []EntityCandidate{
			{CandidateID: uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d200"), TypeKey: "concept",
				Label: "kept", Aliases: []string{}, Confidence: 0.9,
				Evidence: []CandidateEvidence{{ChunkID: wrongChunkID, Quotation: "Unique fact", CharStart: 0, CharEnd: 1}}},
			{CandidateID: uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d201"), TypeKey: "concept",
				Label: "dropped", Aliases: []string{}, Confidence: 0.9,
				Evidence: []CandidateEvidence{{ChunkID: actualChunkID, Quotation: "not present", CharStart: 0, CharEnd: 1}}},
		}, Claims: []ClaimCandidate{},
	}
	raw, err := json.Marshal(candidates)
	if err != nil {
		t.Fatal(err)
	}

	validated, _, err := ValidateCandidatesFromChunks(raw, sourceVersionID, chunks)
	if err != nil {
		t.Fatal(err)
	}
	if len(validated.Entities) != 1 || validated.Entities[0].Label != "kept" {
		t.Fatalf("unexpected retained entities: %#v", validated.Entities)
	}
	item := validated.Entities[0].Evidence[0]
	if item.ChunkID != actualChunkID || item.CharStart != 7 || item.CharEnd != 18 {
		t.Fatalf("evidence was not canonicalized: %#v", item)
	}
	if item.Page == nil || *item.Page != 2 {
		t.Fatalf("page was not derived from chunk locator: %#v", item.Page)
	}
	if math.Abs(validated.QualityScore-0.45) > 0.000001 {
		t.Fatalf("quality score=%f, want 0.45", validated.QualityScore)
	}
}

func TestValidateCandidatesFromChunksUsesPageToResolveRepeatedQuotation(t *testing.T) {
	sourceVersionID := uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d210")
	firstChunkID := uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d211")
	secondChunkID := uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d212")
	page := 2
	chunks := []evidence.SourceChunk{
		{ID: firstChunkID, SourceVersionID: sourceVersionID, TextContent: "same quote", LocatorJSON: []byte(`{"page":1}`)},
		{ID: secondChunkID, SourceVersionID: sourceVersionID, TextContent: "same quote", LocatorJSON: []byte(`{"page":2}`)},
	}
	candidates := Candidates{
		SchemaVersion: 1, SourceVersionID: sourceVersionID, QualityScore: 0.8,
		Entities: []EntityCandidate{{
			CandidateID: uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d213"), TypeKey: "concept",
			Label: "kept", Aliases: []string{}, Confidence: 0.8,
			Evidence: []CandidateEvidence{{ChunkID: uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d214"),
				Quotation: "same quote", CharStart: 0, CharEnd: 10, Page: &page}},
		}}, Claims: []ClaimCandidate{},
	}
	raw, err := json.Marshal(candidates)
	if err != nil {
		t.Fatal(err)
	}

	validated, _, err := ValidateCandidatesFromChunks(raw, sourceVersionID, chunks)
	if err != nil {
		t.Fatal(err)
	}
	if got := validated.Entities[0].Evidence[0].ChunkID; got != secondChunkID {
		t.Fatalf("chunk_id=%s, want page-matched %s", got, secondChunkID)
	}
}

func TestValidateCandidatesFromChunksRejectsWhenNoEvidenceSurvives(t *testing.T) {
	sourceVersionID := uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d220")
	chunkID := uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d221")
	candidates := Candidates{
		SchemaVersion: 1, SourceVersionID: sourceVersionID, QualityScore: 0.9,
		Entities: []EntityCandidate{{
			CandidateID: uuid.MustParse("019ff0e9-0c3a-79cf-be14-d3e76bc8d222"), TypeKey: "concept",
			Label: "invalid", Aliases: []string{}, Confidence: 0.9,
			Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "hallucinated", CharStart: 0, CharEnd: 12}},
		}}, Claims: []ClaimCandidate{},
	}
	raw, err := json.Marshal(candidates)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ValidateCandidatesFromChunks(raw, sourceVersionID, []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID, TextContent: "actual source text", LocatorJSON: []byte(`{}`),
	}})
	if !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("error=%v, want ErrEvidenceRequired", err)
	}
}
