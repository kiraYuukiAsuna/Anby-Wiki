package importer

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type splittingExtractionGenerator struct {
	calls       int
	failureCode string
}

type plannedClaimKnowledge struct{}

type rejectingClaimKnowledge struct {
	property *knowledge.Property
	err      error
}

func (f rejectingClaimKnowledge) GetEntity(context.Context, uuid.UUID) (*knowledge.Entity, error) {
	return nil, errors.New("planned IDs must not be read before Apply")
}

func (f rejectingClaimKnowledge) GetPropertyByKey(context.Context, string) (*knowledge.Property, error) {
	return f.property, f.err
}

func (rejectingClaimKnowledge) ListClaims(context.Context, knowledge.ListClaimsParams) ([]knowledge.Claim, error) {
	return nil, errors.New("rejected claims must not query existing claims")
}

func (plannedClaimKnowledge) GetEntity(context.Context, uuid.UUID) (*knowledge.Entity, error) {
	return nil, errors.New("planned IDs must not be read before Apply")
}

func (plannedClaimKnowledge) GetPropertyByKey(_ context.Context, key string) (*knowledge.Property, error) {
	return &knowledge.Property{ID: uuid.New(), PropertyKey: key, ValueType: knowledge.ValueTypeEntity, IsMultivalued: true}, nil
}

func (plannedClaimKnowledge) ListClaims(context.Context, knowledge.ListClaimsParams) ([]knowledge.Claim, error) {
	return nil, errors.New("planned subjects cannot have existing claims")
}

func (g *splittingExtractionGenerator) Generate(_ context.Context, request ai.Request) (*ai.Result, error) {
	g.calls++
	var chunks []struct {
		ChunkID uuid.UUID `json:"chunk_id"`
		Text    string    `json:"text"`
	}
	if err := json.Unmarshal([]byte(request.Variables["chunks_json"].(string)), &chunks); err != nil {
		return nil, err
	}
	if len(chunks) > 1 {
		code := g.failureCode
		if code == "" {
			code = "output_truncated"
		}
		return nil, &ai.ProviderError{Code: code, Err: ai.ErrInvalidOutput}
	}
	sourceVersionID := uuid.MustParse(request.Variables["source_version_id"].(string))
	candidates := Candidates{
		SchemaVersion: 1, SourceVersionID: sourceVersionID, QualityScore: 0.9,
		Entities: []EntityCandidate{{
			CandidateID: uuid.New(), TypeKey: "concept", Label: chunks[0].Text,
			Aliases: []string{}, Confidence: 0.9, Evidence: []CandidateEvidence{{
				ChunkID: chunks[0].ChunkID, Quotation: chunks[0].Text,
				CharStart: 0, CharEnd: len([]rune(chunks[0].Text)),
			}},
		}},
		Claims: []ClaimCandidate{},
	}
	raw, err := json.Marshal(candidates)
	if err != nil {
		return nil, err
	}
	return &ai.Result{
		JSON: raw, PromptKey: ExtractionPromptKey, PromptVersion: 1, Model: "test-model",
	}, nil
}

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

func TestValidateCandidatesFromChunksRepairsSourceAndDropsOnlyAmbiguousClaims(t *testing.T) {
	sourceVersionID := uuid.New()
	wrongSourceVersionID := uuid.New()
	chunkID := uuid.New()
	duplicateID := uuid.New()
	uniqueID := uuid.New()
	from := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	candidates := Candidates{
		SchemaVersion: 1, SourceVersionID: wrongSourceVersionID, QualityScore: 0.9,
		Entities: []EntityCandidate{
			{CandidateID: duplicateID, TypeKey: "concept", Label: "Alpha", Aliases: []string{}, Confidence: 0.9,
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Alpha", CharStart: 0, CharEnd: 5}}},
			{CandidateID: duplicateID, TypeKey: "concept", Label: "Beta", Aliases: []string{}, Confidence: 0.9,
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Beta", CharStart: 6, CharEnd: 10}}},
			{CandidateID: uniqueID, TypeKey: "concept", Label: "Gamma", Aliases: []string{}, Confidence: 0.9,
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Gamma", CharStart: 11, CharEnd: 16}}},
		},
		Claims: []ClaimCandidate{
			{CandidateID: duplicateID, Subject: CandidateSubject{CandidateID: &duplicateID},
				PropertyKey: "release_date", Value: json.RawMessage(`{"date":"2026-08-12"}`), Confidence: 0.8,
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Alpha", CharStart: 0, CharEnd: 5}}},
			{CandidateID: uuid.New(), Subject: CandidateSubject{CandidateID: &uniqueID},
				PropertyKey: "release_date", Value: json.RawMessage(`{"date":"2026-08-12"}`),
				ValidFrom: &from, ValidTo: &to, Confidence: 0.8,
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Gamma", CharStart: 11, CharEnd: 16}}},
		},
	}
	raw, err := json.Marshal(candidates)
	if err != nil {
		t.Fatal(err)
	}

	validated, _, err := ValidateCandidatesFromChunks(raw, sourceVersionID, []evidence.SourceChunk{{
		ID: chunkID, SourceVersionID: sourceVersionID, TextContent: "Alpha Beta Gamma", LocatorJSON: []byte(`{}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if validated.SourceVersionID != sourceVersionID {
		t.Fatalf("source version=%s, want authoritative %s", validated.SourceVersionID, sourceVersionID)
	}
	if len(validated.Entities) != 3 || len(validated.Claims) != 0 {
		t.Fatalf("entities=%d claims=%d, want all evidenced entities and no invalid claims",
			len(validated.Entities), len(validated.Claims))
	}
	if validated.QualityScore >= candidates.QualityScore {
		t.Fatalf("quality=%f, repaired output must be penalized", validated.QualityScore)
	}
}

func TestBatchSourceChunksBoundsOutputAndPreservesOrder(t *testing.T) {
	sourceVersionID := uuid.New()
	chunks := make([]evidence.SourceChunk, 78)
	for index := range chunks {
		chunks[index] = evidence.SourceChunk{
			ID: uuid.New(), SourceVersionID: sourceVersionID, Ordinal: index,
			TextContent: "small chunk", LocatorJSON: []byte(`{}`),
		}
	}
	batches := batchSourceChunks(chunks, DefaultModelMaxInputTokens)
	wantBatches := (len(chunks) + extractionBatchMaxChunks - 1) / extractionBatchMaxChunks
	if len(batches) != wantBatches {
		t.Fatalf("batch count=%d, want %d", len(batches), wantBatches)
	}
	ordinal := 0
	for _, batch := range batches {
		if len(batch) > extractionBatchMaxChunks {
			t.Fatalf("batch has %d chunks, max=%d", len(batch), extractionBatchMaxChunks)
		}
		for _, chunk := range batch {
			if chunk.Ordinal != ordinal {
				t.Fatalf("ordinal=%d, want %d", chunk.Ordinal, ordinal)
			}
			ordinal++
		}
	}
}

func TestBatchSourceChunksUsesConfiguredInputBudget(t *testing.T) {
	sourceVersionID := uuid.New()
	chunks := make([]evidence.SourceChunk, 6)
	for index := range chunks {
		chunks[index] = evidence.SourceChunk{
			ID: uuid.New(), SourceVersionID: sourceVersionID, Ordinal: index,
			TextContent: strings.Repeat("a", 1200), LocatorJSON: []byte(`{}`),
		}
	}
	batches := batchSourceChunks(chunks, 4096)
	if len(batches) < 2 {
		t.Fatalf("batch count=%d, configured input budget was not applied", len(batches))
	}
}

func TestGenerateCandidatesSplitsInvalidStructuredBatchesAndMergesResults(t *testing.T) {
	for _, failureCode := range []string{"output_truncated", "invalid_structured_output"} {
		t.Run(failureCode, func(t *testing.T) {
			sourceVersionID := uuid.New()
			chunks := []evidence.SourceChunk{
				{ID: uuid.New(), SourceVersionID: sourceVersionID, Ordinal: 0, TextContent: "Alpha", LocatorJSON: []byte(`{}`)},
				{ID: uuid.New(), SourceVersionID: sourceVersionID, Ordinal: 1, TextContent: "Beta", LocatorJSON: []byte(`{}`)},
			}
			gateway := &splittingExtractionGenerator{failureCode: failureCode}
			service := NewExtractionService(nil, nil, gateway, id.NewGenerator())

			result, err := service.generateCandidates(context.Background(), ExtractParams{
				SourceVersionID: sourceVersionID, SourceLabel: "test", Chunks: chunks,
				Provider: "test", Model: "test", MaxInputTokens: DefaultModelMaxInputTokens,
			})
			if err != nil {
				t.Fatal(err)
			}
			if gateway.calls != 3 {
				t.Fatalf("calls=%d, want initial plus two halves", gateway.calls)
			}
			if len(result.Candidates.Entities) != 2 || result.Candidates.Entities[0].Label != "Alpha" ||
				result.Candidates.Entities[1].Label != "Beta" {
				t.Fatalf("unexpected merged entities: %#v", result.Candidates.Entities)
			}
			if result.Candidates.Entities[0].CandidateID == result.Candidates.Entities[1].CandidateID {
				t.Fatal("merged candidates must receive unique server IDs")
			}
		})
	}
}

func TestMergeCandidateBatchesDeduplicatesAndRemapsClaimSubject(t *testing.T) {
	sourceVersionID := uuid.New()
	firstEntityID, secondEntityID := uuid.New(), uuid.New()
	firstChunkID, secondChunkID := uuid.New(), uuid.New()
	part := func(entityID, chunkID uuid.UUID) extractionBatchResult {
		return extractionBatchResult{Candidates: &Candidates{
			SchemaVersion: 1, SourceVersionID: sourceVersionID, QualityScore: 0.8,
			Entities: []EntityCandidate{{
				CandidateID: entityID, TypeKey: "work", Label: "Alpha", Aliases: []string{},
				Confidence: 0.8, Evidence: []CandidateEvidence{{
					ChunkID: chunkID, Quotation: "Alpha", CharStart: 0, CharEnd: 5,
				}},
			}},
			Claims: []ClaimCandidate{{
				CandidateID: uuid.New(), Subject: CandidateSubject{CandidateID: &entityID},
				PropertyKey: "release_date", Value: json.RawMessage(`{"date":"2026-08-11"}`),
				Confidence: 0.8, Evidence: []CandidateEvidence{{
					ChunkID: chunkID, Quotation: "Alpha", CharStart: 0, CharEnd: 5,
				}},
			}},
		}}
	}
	service := NewExtractionService(nil, nil, nil, id.NewGenerator())
	merged, err := service.mergeCandidateBatches(sourceVersionID, []extractionBatchResult{
		part(firstEntityID, firstChunkID), part(secondEntityID, secondChunkID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Entities) != 1 || len(merged.Claims) != 1 {
		t.Fatalf("entities=%d claims=%d, want one deduplicated candidate each", len(merged.Entities), len(merged.Claims))
	}
	if len(merged.Entities[0].Evidence) != 2 || len(merged.Claims[0].Evidence) != 2 {
		t.Fatal("deduplicated candidates must retain evidence from both batches")
	}
	if merged.Claims[0].Subject.CandidateID == nil ||
		*merged.Claims[0].Subject.CandidateID != merged.Entities[0].CandidateID {
		t.Fatal("claim subject was not remapped to the server-generated entity candidate ID")
	}
}

func TestMergeCandidateBatchesRemapsEntityValuedClaimCandidate(t *testing.T) {
	sourceVersionID := uuid.New()
	subjectID, valueID := uuid.New(), uuid.New()
	chunkID := uuid.New()
	part := extractionBatchResult{Candidates: &Candidates{
		SchemaVersion: 1, SourceVersionID: sourceVersionID, QualityScore: 0.9,
		Entities: []EntityCandidate{
			{CandidateID: subjectID, TypeKey: "work", Label: "Story", Aliases: []string{}, Confidence: 0.9,
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Story", CharStart: 0, CharEnd: 5}}},
			{CandidateID: valueID, TypeKey: "person", Label: "Author", Aliases: []string{}, Confidence: 0.9,
				Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Author", CharStart: 6, CharEnd: 12}}},
		},
		Claims: []ClaimCandidate{{CandidateID: uuid.New(), Subject: CandidateSubject{CandidateID: &subjectID},
			PropertyKey: "author", Value: json.RawMessage(`{"entity_candidate_id":"` + valueID.String() + `"}`),
			Confidence: 0.9, Evidence: []CandidateEvidence{{ChunkID: chunkID, Quotation: "Author", CharStart: 6, CharEnd: 12}}}},
	}}
	service := NewExtractionService(nil, nil, nil, id.NewGenerator())
	merged, err := service.mergeCandidateBatches(sourceVersionID, []extractionBatchResult{part})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Entities) != 2 || len(merged.Claims) != 1 {
		t.Fatalf("entities=%d claims=%d", len(merged.Entities), len(merged.Claims))
	}
	valueCandidate, ok, valid := claimValueCandidateReference(merged.Claims[0].Value)
	if !valid || !ok || valueCandidate != merged.Entities[1].CandidateID {
		t.Fatalf("entity value was not remapped: value=%s ok=%v valid=%v", valueCandidate, ok, valid)
	}
}

func TestResolveClaimCandidateReferencesUsesPlannedStableIDs(t *testing.T) {
	subjectCandidateID, valueCandidateID := uuid.New(), uuid.New()
	subjectEntityID, valueEntityID := uuid.New(), uuid.New()
	claim := ClaimCandidate{
		CandidateID: uuid.New(), Subject: CandidateSubject{CandidateID: &subjectCandidateID},
		PropertyKey: "author",
		Value:       json.RawMessage(`{"entity_candidate_id":"` + valueCandidateID.String() + `"}`),
	}
	resolved, ok := resolveClaimCandidateReferences(claim, map[uuid.UUID]uuid.UUID{
		subjectCandidateID: subjectEntityID,
		valueCandidateID:   valueEntityID,
	})
	if !ok || resolved.Subject.EntityID == nil || *resolved.Subject.EntityID != subjectEntityID {
		t.Fatalf("subject was not resolved: %#v", resolved.Subject)
	}
	if got, ok := claimValueEntityID(resolved.Value); !ok || got != valueEntityID {
		t.Fatalf("value entity=%s ok=%v, want %s", got, ok, valueEntityID)
	}
}

func TestClassifyCreatesClaimBetweenTwoPlannedEntities(t *testing.T) {
	subjectCandidateID, valueCandidateID := uuid.New(), uuid.New()
	classifier := NewClaimClassifier(plannedClaimKnowledge{})
	decisions, err := classifier.Classify(context.Background(), []ClaimCandidate{{
		CandidateID: uuid.New(), Subject: CandidateSubject{CandidateID: &subjectCandidateID},
		PropertyKey: "author",
		Value:       json.RawMessage(`{"entity_candidate_id":"` + valueCandidateID.String() + `"}`),
	}}, []EntityResolution{
		{CandidateID: subjectCandidateID, Outcome: EntityNewReview, PlannedEntityID: &subjectCandidateID},
		{CandidateID: valueCandidateID, Outcome: EntityNewReview, PlannedEntityID: &valueCandidateID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Outcome != ClaimNew || decisions[0].SubjectEntityID != subjectCandidateID {
		t.Fatalf("unexpected decisions: %#v", decisions)
	}
	if got, ok := claimValueEntityID(decisions[0].ResolvedValue); !ok || got != valueCandidateID {
		t.Fatalf("resolved target=%s ok=%v, want %s", got, ok, valueCandidateID)
	}
}

func TestClassifySkipsOneUnsupportedOrMalformedClaim(t *testing.T) {
	subjectCandidateID := uuid.New()
	resolutions := []EntityResolution{{
		CandidateID: subjectCandidateID, Outcome: EntityNewReview, PlannedEntityID: &subjectCandidateID,
	}}
	claim := ClaimCandidate{
		CandidateID: uuid.New(), Subject: CandidateSubject{CandidateID: &subjectCandidateID},
		PropertyKey: "unsupported", Value: json.RawMessage(`{"string":"value"}`),
	}
	classifier := NewClaimClassifier(rejectingClaimKnowledge{err: knowledge.ErrPropertyNotFound})
	decisions, err := classifier.Classify(context.Background(), []ClaimCandidate{claim}, resolutions)
	if err != nil || len(decisions) != 0 {
		t.Fatalf("unsupported property decisions=%#v error=%v", decisions, err)
	}

	claim.PropertyKey = "release_date"
	classifier = NewClaimClassifier(rejectingClaimKnowledge{property: &knowledge.Property{
		ID: uuid.New(), PropertyKey: "release_date", ValueType: knowledge.ValueTypeDate,
	}})
	decisions, err = classifier.Classify(context.Background(), []ClaimCandidate{claim}, resolutions)
	if err != nil || len(decisions) != 0 {
		t.Fatalf("malformed value decisions=%#v error=%v", decisions, err)
	}
}

func TestCompositeOperationsUseStableEntityIDs(t *testing.T) {
	wikiID, subjectID, valueID := uuid.New(), uuid.New(), uuid.New()
	params := ComposeParams{WikiID: wikiID}
	entityOp, err := newEntityOperation(params, EntityCandidate{
		CandidateID: subjectID, TypeKey: "work", Label: "Story", Aliases: []string{},
	}, subjectID)
	if err != nil {
		t.Fatal(err)
	}
	if entityOp.Target.EntityID == nil || *entityOp.Target.EntityID != subjectID {
		t.Fatalf("create target=%v, want %s", entityOp.Target.EntityID, subjectID)
	}
	var entityPayload struct {
		CanonicalKey string `json:"canonical_key"`
	}
	if err := json.Unmarshal(entityOp.Payload, &entityPayload); err != nil || entityPayload.CanonicalKey != "work:Story" {
		t.Fatalf("canonical key=%q error=%v, want work:Story", entityPayload.CanonicalKey, err)
	}
	claimOp, err := claimOperation(wikiID, 0, ClaimCandidate{PropertyKey: "author"}, ClaimDecision{
		SubjectEntityID: subjectID, Outcome: ClaimNew, Risk: RiskLow, Reason: "new",
		ResolvedValue: json.RawMessage(`{"entity_id":"` + valueID.String() + `"}`),
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if claimOp.Target.EntityID == nil || *claimOp.Target.EntityID != subjectID {
		t.Fatalf("claim subject=%v, want %s", claimOp.Target.EntityID, subjectID)
	}
	var payload struct {
		Value struct {
			EntityID uuid.UUID `json:"entity_id"`
		} `json:"value"`
	}
	if err := json.Unmarshal(claimOp.Payload, &payload); err != nil || payload.Value.EntityID != valueID {
		t.Fatalf("claim value=%s error=%v, want %s", payload.Value.EntityID, err, valueID)
	}
}

func TestNewEntityOperationBoundsLongCanonicalKey(t *testing.T) {
	wikiID, entityID := uuid.New(), uuid.New()
	op, err := newEntityOperation(ComposeParams{WikiID: wikiID}, EntityCandidate{
		CandidateID: entityID, TypeKey: "concept", Label: strings.Repeat("界", 255), Aliases: []string{},
	}, entityID)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		CanonicalKey string `json:"canonical_key"`
	}
	if err := json.Unmarshal(op.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len([]rune(payload.CanonicalKey)) > 255 || !strings.HasPrefix(payload.CanonicalKey, "concept:") {
		t.Fatalf("unexpected bounded canonical key %q", payload.CanonicalKey)
	}
}

func TestNormalizeCandidatesDeduplicatesNamesAndRejectsWrongRelationDirection(t *testing.T) {
	shortPersonID, fullPersonID := uuid.New(), uuid.New()
	workID, organizationID := uuid.New(), uuid.New()
	correctClaimID, reversedClaimID, affiliationClaimID := uuid.New(), uuid.New(), uuid.New()
	value := func(id uuid.UUID) json.RawMessage {
		raw, _ := json.Marshal(map[string]uuid.UUID{"entity_candidate_id": id})
		return raw
	}
	candidates := &Candidates{SchemaVersion: 1, SourceVersionID: uuid.New(), QualityScore: 0.9,
		Entities: []EntityCandidate{
			{CandidateID: shortPersonID, TypeKey: "person", Label: "M. Jones", Aliases: []string{}},
			{CandidateID: fullPersonID, TypeKey: "person", Label: "Michael B. Jones", Aliases: []string{}},
			{CandidateID: workID, TypeKey: "work", Label: "RFC 7523", Aliases: []string{}},
			{CandidateID: organizationID, TypeKey: "organization", Label: "Microsoft", Aliases: []string{}},
		},
		Claims: []ClaimCandidate{
			{CandidateID: correctClaimID, Subject: CandidateSubject{CandidateID: &workID}, PropertyKey: "author", Value: value(fullPersonID)},
			{CandidateID: reversedClaimID, Subject: CandidateSubject{CandidateID: &fullPersonID}, PropertyKey: "author", Value: value(workID)},
			{CandidateID: affiliationClaimID, Subject: CandidateSubject{CandidateID: &fullPersonID}, PropertyKey: "located_in", Value: value(organizationID)},
		}}

	normalized := normalizeCandidatesForUse(candidates)
	if len(normalized.Entities) != 3 || len(normalized.Claims) != 1 {
		t.Fatalf("unexpected normalized candidates: entities=%#v claims=%#v", normalized.Entities, normalized.Claims)
	}
	person := normalized.Entities[0]
	if person.CandidateID != shortPersonID || person.Label != "Michael B. Jones" || len(person.Aliases) != 1 || person.Aliases[0] != "M. Jones" {
		t.Fatalf("person identity was not consolidated: %#v", person)
	}
	claim := normalized.Claims[0]
	valueID, referenced, valid := claimValueCandidateReference(claim.Value)
	if claim.Subject.CandidateID == nil || *claim.Subject.CandidateID != workID || !referenced || !valid || valueID != shortPersonID {
		t.Fatalf("claim dependency was not remapped: %#v value=%s", claim, valueID)
	}

	plan := &ImportPlan{Profile: SourceProfile{Subjects: []SourceSubject{{Title: "RFC 7523", Kind: "work"}}},
		Routes: []PageRoute{{Action: RouteCreate, Title: "RFC 7523"}}}
	selected := selectCandidatesForPlan(normalized, plan)
	if len(selected.Entities) != 2 || len(selected.Claims) != 1 {
		t.Fatalf("plan relevance filter retained unrelated graph data: %#v", selected)
	}
	plan.Profile.Subjects = append(plan.Profile.Subjects, SourceSubject{Title: "Microsoft", Kind: "organization"})
	selected = selectCandidatesForPlan(normalized, plan)
	if len(selected.Entities) != 2 || len(selected.Claims) != 1 {
		t.Fatalf("profile-only subject became an authoritative graph write: %#v", selected)
	}
}

func TestPassesExtractionQualityGateAcceptsOnlyStrongVerifiedSalvage(t *testing.T) {
	evidenceItem := []CandidateEvidence{{ChunkID: uuid.New(), Quotation: "verified", CharStart: 0, CharEnd: 8}}
	candidates := &Candidates{QualityScore: 0.52, Entities: []EntityCandidate{
		{Confidence: 0.9, Evidence: evidenceItem},
		{Confidence: 0.8, Evidence: evidenceItem},
	}}
	if !passesExtractionQualityGate(candidates, 0.7) {
		t.Fatal("strong evidence-verified salvage should pass")
	}
	candidates.QualityScore = 0.34
	if passesExtractionQualityGate(candidates, 0.7) {
		t.Fatal("aggregate quality below half the threshold must fail")
	}
	candidates.QualityScore = 0.52
	candidates.Entities[1].Confidence = 0.4
	if passesExtractionQualityGate(candidates, 0.7) {
		t.Fatal("low retained average confidence must fail")
	}
	candidates.Entities[1].Confidence = 0.8
	candidates.PromptInjectionDetected = true
	if passesExtractionQualityGate(candidates, 0.7) {
		t.Fatal("prompt injection must always fail")
	}
}
