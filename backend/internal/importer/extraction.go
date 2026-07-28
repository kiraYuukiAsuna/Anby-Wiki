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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/id"
)

const (
	ExtractionSchemaURL = "https://anby.wiki/schemas/extraction/v1/candidates.schema.json"
	ExtractionPromptKey = "source-extraction-v2"
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
		return &ExtractResult{Extraction: existing, Candidates: &candidates, Reused: true}, nil
	} else if !errors.Is(err, ErrExtractionNotFound) {
		return nil, err
	}
	chunkViews := make([]map[string]any, len(params.Chunks))
	texts := make([]string, len(params.Chunks))
	for i := range params.Chunks {
		chunkViews[i] = map[string]any{"chunk_id": params.Chunks[i].ID, "ordinal": params.Chunks[i].Ordinal,
			"text": params.Chunks[i].TextContent, "locator": json.RawMessage(params.Chunks[i].LocatorJSON)}
		texts[i] = params.Chunks[i].TextContent
	}
	chunksJSON, _ := json.Marshal(chunkViews)
	result, err := s.ai.Generate(ctx, ai.Request{Provider: params.Provider, Model: params.Model,
		PromptKey: ExtractionPromptKey, Variables: map[string]any{
			"source_version_id": params.SourceVersionID.String(), "source_label": strings.TrimSpace(params.SourceLabel),
			"chunks_json": string(chunksJSON),
		}, ImportJobID: params.ImportJobID, ImportRunID: params.ImportRunID})
	if err != nil {
		return nil, err
	}
	candidates, canonical, err := ValidateCandidates(ctx, result.JSON, params.SourceVersionID, s.chunks)
	if err != nil {
		return nil, err
	}
	if DetectPromptInjection(texts) {
		candidates.PromptInjectionDetected = true
		canonical, _ = json.Marshal(candidates)
	}
	extractionID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	extraction := &Extraction{ID: extractionID, SourceVersionID: params.SourceVersionID,
		SchemaVersion: 1, PromptKey: result.PromptKey, PromptVersion: result.PromptVersion,
		Model: result.Model, CandidatesJSON: canonical, QualityScore: candidates.QualityScore}
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
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil || compiledExtractionSchema.Validate(instance) != nil {
		return nil, nil, fmt.Errorf("%w: extraction schema", ai.ErrInvalidOutput)
	}
	var candidates Candidates
	if err := json.Unmarshal(raw, &candidates); err != nil || candidates.SourceVersionID != sourceVersionID {
		return nil, nil, fmt.Errorf("%w: source_version_id", ai.ErrInvalidOutput)
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
	return &candidates, canonical, nil
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
		runes := []rune(chunk.TextContent)
		if item.CharStart >= 0 && item.CharEnd > item.CharStart && item.CharEnd <= len(runes) &&
			string(runes[item.CharStart:item.CharEnd]) == item.Quotation {
			continue
		}
		quotation := []rune(item.Quotation)
		matchStart := -1
		matchDistance := len(runes) + len(quotation) + 1
		tied := false
		for start := 0; start+len(quotation) <= len(runes); start++ {
			if string(runes[start:start+len(quotation)]) != item.Quotation {
				continue
			}
			distance := start - item.CharStart
			if distance < 0 {
				distance = -distance
			}
			if distance < matchDistance {
				matchStart, matchDistance, tied = start, distance, false
			} else if distance == matchDistance {
				tied = true
			}
		}
		if matchStart < 0 {
			return fmt.Errorf("%w: quotation/range", ErrEvidenceRequired)
		}
		if tied {
			return fmt.Errorf("%w: ambiguous quotation", ErrEvidenceRequired)
		}
		item.CharStart = matchStart
		item.CharEnd = matchStart + len(quotation)
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
