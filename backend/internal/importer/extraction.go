package importer

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/id"
)

const (
	ExtractionSchemaURL = "https://anby.wiki/schemas/extraction/v1/candidates.schema.json"
	ExtractionPromptKey = "source-extraction-v7"
)

//go:embed schema/candidates.schema.json
var extractionSchemaJSON []byte

var compiledExtractionSchema = mustCompileExtractionSchema()

// ExtractionSchemaJSON returns an isolated copy of the authoritative schema.
// Runtime prompt bootstrapping uses the same embedded bytes as domain
// validation, preventing provider/output contract drift.
func ExtractionSchemaJSON() json.RawMessage {
	return append(json.RawMessage(nil), extractionSchemaJSON...)
}

type CandidateEvidence struct {
	ChunkID   uuid.UUID `json:"chunk_id"`
	Quotation string    `json:"quotation"`
	CharStart int       `json:"char_start"`
	CharEnd   int       `json:"char_end"`
	Page      *int      `json:"page,omitempty"`
}

type EntityCandidate struct {
	CandidateID uuid.UUID           `json:"candidate_id"`
	TypeKey     string              `json:"type_key"`
	Label       string              `json:"label"`
	Aliases     []string            `json:"aliases"`
	Confidence  float64             `json:"confidence"`
	Evidence    []CandidateEvidence `json:"evidence"`
}

type CandidateSubject struct {
	CandidateID *uuid.UUID `json:"candidate_id,omitempty"`
	EntityID    *uuid.UUID `json:"entity_id,omitempty"`
}

type ClaimCandidate struct {
	CandidateID uuid.UUID           `json:"candidate_id"`
	Subject     CandidateSubject    `json:"subject"`
	PropertyKey string              `json:"property_key"`
	Value       json.RawMessage     `json:"value"`
	ValidFrom   *time.Time          `json:"valid_from,omitempty"`
	ValidTo     *time.Time          `json:"valid_to,omitempty"`
	Confidence  float64             `json:"confidence"`
	Evidence    []CandidateEvidence `json:"evidence"`
}

type Candidates struct {
	SchemaVersion           int               `json:"schema_version"`
	SourceVersionID         uuid.UUID         `json:"source_version_id"`
	Entities                []EntityCandidate `json:"entities"`
	Claims                  []ClaimCandidate  `json:"claims"`
	QualityScore            float64           `json:"quality_score"`
	PromptInjectionDetected bool              `json:"prompt_injection_detected"`
}

type Extraction struct {
	ID              uuid.UUID
	SourceVersionID uuid.UUID
	SchemaVersion   int
	PromptKey       string
	PromptVersion   int
	Model           string
	CandidatesJSON  json.RawMessage
	QualityScore    float64
	CreatedAt       time.Time
}

type ChunkLookup interface {
	GetSourceChunkByID(context.Context, pgx.Tx, uuid.UUID) (*evidence.SourceChunk, error)
}

type StructuredGenerator interface {
	Generate(context.Context, ai.Request) (*ai.Result, error)
}

type ExtractionService struct {
	repo   *Repository
	chunks ChunkLookup
	ai     StructuredGenerator
	ids    *id.Generator
}

func NewExtractionService(repo *Repository, chunks ChunkLookup, gateway StructuredGenerator, ids *id.Generator) *ExtractionService {
	return &ExtractionService{repo: repo, chunks: chunks, ai: gateway, ids: ids}
}

type ExtractParams struct {
	SourceVersionID uuid.UUID
	SourceLabel     string
	Chunks          []evidence.SourceChunk
	Provider        string
	Model           string
	MaxInputTokens  int
	ImportJobID     *uuid.UUID
	ImportRunID     *uuid.UUID
}

type ExtractResult struct {
	Extraction *Extraction
	Candidates *Candidates
	Reused     bool
}

func (s *ExtractionService) Extract(ctx context.Context, params ExtractParams) (*ExtractResult, error) {
	if existing, err := s.repo.GetExtraction(ctx, params.SourceVersionID); err == nil {
		var candidates Candidates
		if err := json.Unmarshal(existing.CandidatesJSON, &candidates); err != nil {
			return nil, err
		}
		// Prompt improvements cannot rewrite an immutable historical extraction.
		// Re-run deterministic identity and relation guards on every read so a
		// new ImportJob does not inherit duplicate or directionally invalid graph
		// candidates from an older prompt version.
		return &ExtractResult{Extraction: existing, Candidates: normalizeCandidatesForUse(&candidates), Reused: true}, nil
	} else if !errors.Is(err, ErrExtractionNotFound) {
		return nil, err
	}
	texts := make([]string, len(params.Chunks))
	for i := range params.Chunks {
		texts[i] = params.Chunks[i].TextContent
	}
	generated, err := s.generateCandidates(ctx, params)
	if err != nil {
		return nil, err
	}
	candidates := normalizeCandidatesForUse(generated.Candidates)
	if DetectPromptInjection(texts) {
		candidates.PromptInjectionDetected = true
	}
	canonical, _ := json.Marshal(candidates)
	extractionID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	extraction := &Extraction{ID: extractionID, SourceVersionID: params.SourceVersionID,
		SchemaVersion: 1, PromptKey: generated.PromptKey, PromptVersion: generated.PromptVersion,
		Model: generated.Model, CandidatesJSON: canonical, QualityScore: candidates.QualityScore}
	inserted, err := s.repo.InsertExtractionIfAbsent(ctx, extraction)
	if err != nil {
		return nil, err
	}
	if !inserted {
		extraction, err = s.repo.GetExtraction(ctx, params.SourceVersionID)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(extraction.CandidatesJSON, candidates)
	}
	return &ExtractResult{Extraction: extraction, Candidates: candidates, Reused: !inserted}, nil
}

// ValidateCandidatesFromChunks validates structural output, then keeps only
// evidence that can be proven against the immutable chunks supplied to the
// model. A model occasionally copies the right exact quotation but the wrong
// chunk UUID; when that quotation occurs in exactly one chunk, this function
// safely repairs the reference. Unsupported evidence and candidates are
// discarded instead of poisoning an otherwise reviewable extraction.
func ValidateCandidatesFromChunks(raw []byte, sourceVersionID uuid.UUID, chunks []evidence.SourceChunk) (*Candidates, json.RawMessage, error) {
	candidates, sourceIDRepaired, err := decodeCandidates(raw, sourceVersionID)
	if err != nil {
		return nil, nil, err
	}

	// Candidate IDs are temporary model-local references. Duplicate claim IDs
	// do not affect authoritative writes because mergeCandidateBatches replaces
	// every retained ID. Duplicate entity IDs are ambiguous only for claims
	// that try to reference them, so retain the independently evidenced
	// entities and discard just those ambiguous claims.
	entityIDCounts := map[uuid.UUID]int{}
	for i := range candidates.Entities {
		entityIDCounts[candidates.Entities[i].CandidateID]++
	}

	catalog := newEvidenceCatalog(sourceVersionID, chunks)
	originalCandidates := len(candidates.Entities) + len(candidates.Claims)
	originalEvidence := 0
	retainedEvidence := 0
	retainedEntityIDs := map[uuid.UUID]bool{}
	entities := make([]EntityCandidate, 0, len(candidates.Entities))
	for i := range candidates.Entities {
		candidate := candidates.Entities[i]
		originalEvidence += len(candidate.Evidence)
		candidate.Evidence = catalog.normalize(candidate.Evidence)
		retainedEvidence += len(candidate.Evidence)
		if len(candidate.Evidence) == 0 {
			continue
		}
		retainedEntityIDs[candidate.CandidateID] = true
		entities = append(entities, candidate)
	}
	claims := make([]ClaimCandidate, 0, len(candidates.Claims))
	for i := range candidates.Claims {
		candidate := candidates.Claims[i]
		originalEvidence += len(candidate.Evidence)
		if candidate.Subject.CandidateID != nil && entityIDCounts[*candidate.Subject.CandidateID] != 1 {
			continue
		}
		valueCandidateID, hasValueCandidate, validValue := claimValueCandidateReference(candidate.Value)
		if !validValue || (hasValueCandidate && entityIDCounts[valueCandidateID] != 1) {
			continue
		}
		if claimCandidateSelfReferential(candidate) {
			continue
		}
		if candidate.ValidFrom != nil && candidate.ValidTo != nil && !candidate.ValidTo.After(*candidate.ValidFrom) {
			continue
		}
		candidate.Evidence = catalog.normalize(candidate.Evidence)
		if len(candidate.Evidence) == 0 ||
			(candidate.Subject.CandidateID != nil && !retainedEntityIDs[*candidate.Subject.CandidateID]) ||
			(hasValueCandidate && !retainedEntityIDs[valueCandidateID]) {
			continue
		}
		retainedEvidence += len(candidate.Evidence)
		claims = append(claims, candidate)
	}
	candidates.Entities = entities
	candidates.Claims = claims
	retainedCandidates := len(entities) + len(claims)
	if originalCandidates > 0 && retainedCandidates == 0 {
		return nil, nil, ErrEvidenceRequired
	}
	if originalCandidates > 0 && originalEvidence > 0 {
		candidateRatio := float64(retainedCandidates) / float64(originalCandidates)
		evidenceRatio := float64(retainedEvidence) / float64(originalEvidence)
		candidates.QualityScore *= min(candidateRatio, evidenceRatio)
	}
	if sourceIDRepaired {
		candidates.QualityScore *= 0.9
	}
	canonical, _ := json.Marshal(candidates)
	return candidates, canonical, nil
}

func claimValueCandidateReference(raw json.RawMessage) (uuid.UUID, bool, bool) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || len(value) != 1 {
		return uuid.Nil, false, false
	}
	rawID, ok := value["entity_candidate_id"]
	if !ok {
		return uuid.Nil, false, true
	}
	var candidateID uuid.UUID
	if err := json.Unmarshal(rawID, &candidateID); err != nil || candidateID == uuid.Nil {
		return uuid.Nil, true, false
	}
	return candidateID, true, true
}

func decodeCandidates(raw []byte, sourceVersionID uuid.UUID) (*Candidates, bool, error) {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil || compiledExtractionSchema.Validate(instance) != nil {
		return nil, false, fmt.Errorf("%w: extraction schema", ai.ErrInvalidOutput)
	}
	var candidates Candidates
	if err := json.Unmarshal(raw, &candidates); err != nil {
		return nil, false, fmt.Errorf("%w: extraction document", ai.ErrInvalidOutput)
	}
	repaired := candidates.SourceVersionID != sourceVersionID
	// The requested immutable source version is authoritative. The model is not
	// allowed to select or redirect it, so an otherwise valid echoed UUID can be
	// canonicalized without trusting model output.
	candidates.SourceVersionID = sourceVersionID
	return &candidates, repaired, nil
}

type evidenceCatalog struct {
	byID   map[uuid.UUID]*evidence.SourceChunk
	chunks []*evidence.SourceChunk
}

func newEvidenceCatalog(sourceVersionID uuid.UUID, chunks []evidence.SourceChunk) evidenceCatalog {
	catalog := evidenceCatalog{byID: make(map[uuid.UUID]*evidence.SourceChunk, len(chunks))}
	for index := range chunks {
		chunk := &chunks[index]
		if chunk.SourceVersionID != sourceVersionID {
			continue
		}
		catalog.byID[chunk.ID] = chunk
		catalog.chunks = append(catalog.chunks, chunk)
	}
	return catalog
}

func (c evidenceCatalog) normalize(items []CandidateEvidence) []CandidateEvidence {
	result := make([]CandidateEvidence, 0, len(items))
	seen := map[string]bool{}
	for index := range items {
		item := items[index]
		if chunk := c.byID[item.ChunkID]; chunk != nil {
			if start, end, ok := canonicalQuotationSpan(chunk.TextContent, item.Quotation, item.CharStart); ok {
				canonicalizeEvidence(&item, chunk, start, end)
				key := evidenceKey(item)
				if !seen[key] {
					seen[key] = true
					result = append(result, item)
				}
				continue
			}
		}

		// Wrong chunk UUIDs are repairable only when the exact quotation has a
		// unique destination. Page metadata narrows otherwise repeated text.
		matches := make([]struct {
			chunk *evidence.SourceChunk
			start int
			end   int
		}, 0, 2)
		for _, chunk := range c.chunks {
			start, end, ok := canonicalQuotationSpan(chunk.TextContent, item.Quotation, item.CharStart)
			if ok {
				matches = append(matches, struct {
					chunk *evidence.SourceChunk
					start int
					end   int
				}{chunk: chunk, start: start, end: end})
			}
		}
		if item.Page != nil && len(matches) > 1 {
			pageMatches := matches[:0]
			for _, match := range matches {
				if page := chunkPage(match.chunk); page != nil && *page == *item.Page {
					pageMatches = append(pageMatches, match)
				}
			}
			if len(pageMatches) > 0 {
				matches = pageMatches
			}
		}
		if len(matches) != 1 {
			continue
		}
		canonicalizeEvidence(&item, matches[0].chunk, matches[0].start, matches[0].end)
		key := evidenceKey(item)
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}
	return result
}

func exactQuotationStart(text, quotation string, hintedStart int) (int, bool) {
	runes, quoted := []rune(text), []rune(quotation)
	if len(quoted) == 0 || len(quoted) > len(runes) {
		return 0, false
	}
	if hintedStart >= 0 && hintedStart+len(quoted) <= len(runes) &&
		equalRunesAt(runes, quoted, hintedStart) {
		return hintedStart, true
	}
	matchStart := -1
	// hintedStart is model-controlled and may be far outside the chunk. Use the
	// largest int as the initial distance so an otherwise unique exact quotation
	// remains repairable instead of being discarded because its hint was huge.
	matchDistance := int(^uint(0) >> 1)
	tied := false
	for start := 0; start+len(quoted) <= len(runes); start++ {
		if !equalRunesAt(runes, quoted, start) {
			continue
		}
		distance := start - hintedStart
		if distance < 0 {
			distance = -distance
		}
		if distance < matchDistance {
			matchStart, matchDistance, tied = start, distance, false
		} else if distance == matchDistance {
			tied = true
		}
	}
	return matchStart, matchStart >= 0 && !tied
}

// canonicalQuotationSpan accepts an exact quotation first, then a
// whitespace-equivalent quotation. Plain-text specifications commonly contain
// hard line wraps and indentation that models collapse to spaces. The fallback
// changes no letters, punctuation, or case, and its result is replaced with
// the exact immutable source substring before it can become Citation data.
func canonicalQuotationSpan(text, quotation string, hintedStart int) (int, int, bool) {
	if start, ok := exactQuotationStart(text, quotation, hintedStart); ok {
		return start, start + len([]rune(quotation)), true
	}
	return whitespaceEquivalentQuotationSpan(text, quotation, hintedStart)
}

type normalizedRuneSpan struct {
	start int
	end   int
}

func whitespaceEquivalentQuotationSpan(text, quotation string, hintedStart int) (int, int, bool) {
	normalizedText, spans := collapseWhitespaceWithSpans([]rune(text))
	normalizedQuotation, _ := collapseWhitespaceWithSpans([]rune(strings.TrimSpace(quotation)))
	if len(normalizedQuotation) == 0 || len(normalizedQuotation) > len(normalizedText) {
		return 0, 0, false
	}
	matchStart, matchEnd := -1, -1
	matchDistance := int(^uint(0) >> 1)
	tied := false
	for start := 0; start+len(normalizedQuotation) <= len(normalizedText); start++ {
		if !equalRunesAt(normalizedText, normalizedQuotation, start) {
			continue
		}
		rawStart := spans[start].start
		rawEnd := spans[start+len(normalizedQuotation)-1].end
		distance := rawStart - hintedStart
		if distance < 0 {
			distance = -distance
		}
		if distance < matchDistance {
			matchStart, matchEnd, matchDistance, tied = rawStart, rawEnd, distance, false
		} else if distance == matchDistance {
			tied = true
		}
	}
	return matchStart, matchEnd, matchStart >= 0 && !tied
}

func collapseWhitespaceWithSpans(value []rune) ([]rune, []normalizedRuneSpan) {
	normalized := make([]rune, 0, len(value))
	spans := make([]normalizedRuneSpan, 0, len(value))
	for index := 0; index < len(value); {
		if !unicode.IsSpace(value[index]) {
			normalized = append(normalized, value[index])
			spans = append(spans, normalizedRuneSpan{start: index, end: index + 1})
			index++
			continue
		}
		end := index + 1
		for end < len(value) && unicode.IsSpace(value[end]) {
			end++
		}
		normalized = append(normalized, ' ')
		spans = append(spans, normalizedRuneSpan{start: index, end: end})
		index = end
	}
	return normalized, spans
}

func equalRunesAt(text, quotation []rune, start int) bool {
	for index := range quotation {
		if text[start+index] != quotation[index] {
			return false
		}
	}
	return true
}

func canonicalizeEvidence(item *CandidateEvidence, chunk *evidence.SourceChunk, start, end int) {
	runes := []rune(chunk.TextContent)
	item.ChunkID = chunk.ID
	item.CharStart = start
	item.CharEnd = end
	item.Quotation = string(runes[start:end])
	item.Page = chunkPage(chunk)
}

func chunkPage(chunk *evidence.SourceChunk) *int {
	var locator struct {
		Page *int `json:"page"`
	}
	if json.Unmarshal(chunk.LocatorJSON, &locator) != nil {
		return nil
	}
	return locator.Page
}

func evidenceKey(item CandidateEvidence) string {
	return fmt.Sprintf("%s\x00%d\x00%d\x00%s", item.ChunkID, item.CharStart, item.CharEnd, item.Quotation)
}

func mustCompileExtractionSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(extractionSchemaJSON))
	if err != nil {
		panic(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(ExtractionSchemaURL, doc); err != nil {
		panic(err)
	}
	schema, err := compiler.Compile(ExtractionSchemaURL)
	if err != nil {
		panic(err)
	}
	return schema
}

func ValidateCandidates(ctx context.Context, raw []byte, sourceVersionID uuid.UUID, chunks ChunkLookup) (*Candidates, json.RawMessage, error) {
	candidates, _, err := decodeCandidates(raw, sourceVersionID)
	if err != nil {
		return nil, nil, err
	}
	ids := map[uuid.UUID]bool{}
	entityIDs := map[uuid.UUID]bool{}
	for i := range candidates.Entities {
		candidate := &candidates.Entities[i]
		if ids[candidate.CandidateID] {
			return nil, nil, fmt.Errorf("%w: duplicate candidate", ai.ErrInvalidOutput)
		}
		ids[candidate.CandidateID], entityIDs[candidate.CandidateID] = true, true
		if err := normalizeEvidence(ctx, sourceVersionID, candidate.Evidence, chunks); err != nil {
			return nil, nil, err
		}
	}
	for i := range candidates.Claims {
		candidate := &candidates.Claims[i]
		if ids[candidate.CandidateID] {
			return nil, nil, fmt.Errorf("%w: duplicate candidate", ai.ErrInvalidOutput)
		}
		ids[candidate.CandidateID] = true
		if candidate.Subject.CandidateID != nil && !entityIDs[*candidate.Subject.CandidateID] {
			return nil, nil, fmt.Errorf("%w: subject candidate", ai.ErrInvalidOutput)
		}
		if candidate.ValidFrom != nil && candidate.ValidTo != nil && !candidate.ValidTo.After(*candidate.ValidFrom) {
			return nil, nil, fmt.Errorf("%w: valid time", ai.ErrInvalidOutput)
		}
		if err := normalizeEvidence(ctx, sourceVersionID, candidate.Evidence, chunks); err != nil {
			return nil, nil, err
		}
	}
	canonical, _ := json.Marshal(candidates)
	return candidates, canonical, nil
}

// normalizeEvidence verifies the immutable quotation and derives rune offsets
// when a model counted bytes or otherwise supplied an inaccurate range. For a
// repeated quotation, the exact occurrence nearest the supplied start is used;
// an equal-distance tie remains ambiguous and is rejected.
func normalizeEvidence(ctx context.Context, sourceVersionID uuid.UUID, items []CandidateEvidence, lookup ChunkLookup) error {
	if len(items) == 0 {
		return ErrEvidenceRequired
	}
	for index := range items {
		item := &items[index]
		chunk, err := lookup.GetSourceChunkByID(ctx, nil, item.ChunkID)
		if err != nil || chunk.SourceVersionID != sourceVersionID {
			return fmt.Errorf("%w: chunk", ErrEvidenceRequired)
		}
		matchStart, matchEnd, ok := canonicalQuotationSpan(chunk.TextContent, item.Quotation, item.CharStart)
		if !ok {
			return fmt.Errorf("%w: quotation/range", ErrEvidenceRequired)
		}
		canonicalizeEvidence(item, chunk, matchStart, matchEnd)
	}
	return nil
}

func DetectPromptInjection(texts []string) bool {
	patterns := []string{"ignore previous instructions", "ignore all previous", "system prompt",
		"developer message", "reveal your prompt", "忽略之前的指令", "忽略以上指令", "系统提示词"}
	for _, text := range texts {
		lower := strings.ToLower(text)
		for _, pattern := range patterns {
			if strings.Contains(lower, pattern) {
				return true
			}
		}
	}
	return false
}
