package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/evidence"
)

const (
	// DefaultModelMaxInputTokens is used only when a legacy caller has not
	// supplied the administrator-managed model input limit.
	DefaultModelMaxInputTokens = 128000

	// A model may accept a large input while still having a much smaller output
	// allowance. Keeping extraction batches bounded prevents a valid but large
	// structured response from exhausting that independent output limit.
	extractionBatchMaxChunks   = 6
	extractionBatchConcurrency = 3
	extractionBatchTarget      = 12000
	extractionInputMinReserve  = 2048
	extractionMaxEntities      = 200
	extractionMaxClaims        = 500
)

type generatedCandidates struct {
	Candidates    *Candidates
	PromptKey     string
	PromptVersion int
	Model         string
}

type extractionBatchResult struct {
	Candidates *Candidates
	Result     *ai.Result
}

func (s *ExtractionService) generateCandidates(ctx context.Context, params ExtractParams) (*generatedCandidates, error) {
	batches := batchModelSourceChunks(initialModelSourceChunks(params.Chunks), params.MaxInputTokens)
	type batchOutcome struct {
		parts    []extractionBatchResult
		rejected int
		err      error
	}
	outcomes := make([]batchOutcome, len(batches))
	limit := make(chan struct{}, extractionBatchConcurrency)
	var wait sync.WaitGroup
	for index := range batches {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				outcomes[index].err = ctx.Err()
				return
			}
			outcomes[index].parts, outcomes[index].rejected, outcomes[index].err =
				s.generateBatch(ctx, params, batches[index])
		}(index)
	}
	wait.Wait()

	parts := make([]extractionBatchResult, 0, len(batches))
	rejectedBatches := 0
	var firstError error
	for index := range outcomes {
		outcome := outcomes[index]
		if outcome.err != nil {
			if firstError == nil {
				firstError = outcome.err
			}
			continue
		}
		rejectedBatches += outcome.rejected
		parts = append(parts, outcome.parts...)
	}
	if firstError != nil {
		return nil, firstError
	}
	if len(parts) == 0 {
		if rejectedBatches > 0 {
			return nil, ErrEvidenceRequired
		}
		return nil, fmt.Errorf("%w: no extraction batch result", ai.ErrInvalidOutput)
	}

	first := parts[0].Result
	for _, part := range parts[1:] {
		if part.Result.PromptKey != first.PromptKey || part.Result.PromptVersion != first.PromptVersion ||
			part.Result.Model != first.Model {
			return nil, fmt.Errorf("%w: inconsistent extraction batch metadata", ai.ErrInvalidOutput)
		}
	}
	merged, err := s.mergeCandidateBatches(params.SourceVersionID, parts)
	if err != nil {
		return nil, err
	}
	if len(merged.Entities)+len(merged.Claims) == 0 && rejectedBatches > 0 {
		return nil, ErrEvidenceRequired
	}
	if rejectedBatches > 0 {
		merged.QualityScore *= float64(len(parts)) / float64(len(parts)+rejectedBatches)
	}
	return &generatedCandidates{
		Candidates: merged, PromptKey: first.PromptKey,
		PromptVersion: first.PromptVersion, Model: first.Model,
	}, nil
}

// generateBatch recursively halves a batch when structured generation is too
// large or too complex for the model. Transport, authentication, quota and
// timeout errors retain their original semantics.
func (s *ExtractionService) generateBatch(ctx context.Context, params ExtractParams,
	chunks []modelSourceChunk) ([]extractionBatchResult, int, error) {
	result, err := s.generateBatchOnce(ctx, params, chunks)
	if err != nil {
		if isAdaptiveBatchError(err) {
			return s.splitBatch(ctx, params, chunks)
		}
		return nil, 0, err
	}

	var candidates *Candidates
	if len(chunks) > 0 {
		candidates, _, err = ValidateCandidatesFromChunks(result.JSON, params.SourceVersionID, modelSourceChunkViews(chunks))
		if err == nil {
			remapCandidatesEvidence(candidates, chunks)
		}
	} else {
		candidates, _, err = ValidateCandidates(ctx, result.JSON, params.SourceVersionID, s.chunks)
	}
	if errors.Is(err, ErrEvidenceRequired) {
		return s.splitBatch(ctx, params, chunks)
	}
	if err != nil {
		if errors.Is(err, ai.ErrInvalidOutput) {
			return s.splitBatch(ctx, params, chunks)
		}
		return nil, 0, err
	}
	return []extractionBatchResult{{Candidates: candidates, Result: result}}, 0, nil
}

func (s *ExtractionService) splitBatch(ctx context.Context, params ExtractParams,
	chunks []modelSourceChunk) ([]extractionBatchResult, int, error) {
	leftChunks, rightChunks, ok := splitModelSourceChunks(chunks)
	if !ok {
		return nil, 1, nil
	}
	left, leftRejected, err := s.generateBatch(ctx, params, leftChunks)
	if err != nil {
		return nil, 0, err
	}
	right, rightRejected, err := s.generateBatch(ctx, params, rightChunks)
	if err != nil {
		return nil, 0, err
	}
	return append(left, right...), leftRejected + rightRejected, nil
}

func (s *ExtractionService) generateBatchOnce(ctx context.Context, params ExtractParams,
	chunks []modelSourceChunk) (*ai.Result, error) {
	views := modelSourceChunkViews(chunks)
	chunkViews := make([]map[string]any, len(chunks))
	for index := range chunks {
		chunkViews[index] = map[string]any{
			"chunk_id": views[index].ID,
			"ordinal":  views[index].Ordinal,
			"text":     views[index].TextContent,
			"locator":  json.RawMessage(views[index].LocatorJSON),
		}
	}
	chunksJSON, err := json.Marshal(chunkViews)
	if err != nil {
		return nil, err
	}
	return s.ai.Generate(ctx, ai.Request{
		Provider: params.Provider, Model: params.Model, PromptKey: ExtractionPromptKey,
		Variables: map[string]any{
			"source_version_id": params.SourceVersionID.String(),
			"source_label":      strings.TrimSpace(params.SourceLabel),
			"chunks_json":       string(chunksJSON),
		},
		ImportJobID: params.ImportJobID, ImportRunID: params.ImportRunID,
	})
}

func isAdaptiveBatchError(err error) bool {
	var providerErr *ai.ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Code == "output_truncated" || providerErr.Code == "invalid_structured_output"
	}
	return errors.Is(err, ai.ErrInvalidOutput)
}

func estimateChunkInputTokens(chunk evidence.SourceChunk) int {
	ascii, nonASCII := 0, 0
	for _, value := range chunk.TextContent {
		if value <= utf8.RuneSelf {
			ascii++
		} else {
			nonASCII++
		}
	}
	// JSON field names, UUID, ordinal, locator, and escaping account for the
	// fixed overhead. Non-ASCII is intentionally conservative for multilingual
	// tokenizers; ASCII prose/code averages roughly four characters per token.
	return 96 + (ascii+3)/4 + nonASCII*2 + (len(chunk.LocatorJSON)+3)/4
}

func (s *ExtractionService) mergeCandidateBatches(sourceVersionID uuid.UUID,
	parts []extractionBatchResult) (*Candidates, error) {
	merged := &Candidates{SchemaVersion: 1, SourceVersionID: sourceVersionID,
		Entities: []EntityCandidate{}, Claims: []ClaimCandidate{}}
	entityByKey := map[string]int{}
	claimByKey := map[string]int{}
	qualityWeight, qualityTotal := 0, 0.0
	dropped := 0

	for _, part := range parts {
		candidates := part.Candidates
		count := len(candidates.Entities) + len(candidates.Claims)
		if count > 0 {
			qualityWeight += count
			qualityTotal += candidates.QualityScore * float64(count)
		}
		merged.PromptInjectionDetected = merged.PromptInjectionDetected || candidates.PromptInjectionDetected
		entityIDs := make(map[uuid.UUID]uuid.UUID, len(candidates.Entities))

		for _, candidate := range candidates.Entities {
			originalID := candidate.CandidateID
			key := normalizedEntityKey(candidate)
			if index, ok := entityByKey[key]; ok {
				existing := &merged.Entities[index]
				entityIDs[originalID] = existing.CandidateID
				existing.Aliases = mergeAliases(existing.Aliases, candidate.Aliases)
				existing.Evidence = mergeCandidateEvidence(existing.Evidence, candidate.Evidence)
				existing.Confidence = max(existing.Confidence, candidate.Confidence)
				continue
			}
			if len(merged.Entities) >= extractionMaxEntities {
				dropped++
				continue
			}
			candidateID, err := s.ids.New()
			if err != nil {
				return nil, err
			}
			candidate.CandidateID = candidateID
			candidate.Aliases = mergeAliases(nil, candidate.Aliases)
			candidate.Evidence = mergeCandidateEvidence(nil, candidate.Evidence)
			entityIDs[originalID] = candidateID
			entityByKey[key] = len(merged.Entities)
			merged.Entities = append(merged.Entities, candidate)
		}

		for _, candidate := range candidates.Claims {
			if candidate.Subject.CandidateID != nil {
				mapped, ok := entityIDs[*candidate.Subject.CandidateID]
				if !ok {
					dropped++
					continue
				}
				candidate.Subject.CandidateID = &mapped
			}
			if valueCandidateID, ok, valid := claimValueCandidateReference(candidate.Value); !valid {
				dropped++
				continue
			} else if ok {
				mapped, found := entityIDs[valueCandidateID]
				if !found {
					dropped++
					continue
				}
				candidate.Value, _ = json.Marshal(map[string]uuid.UUID{"entity_candidate_id": mapped})
			}
			key, err := normalizedClaimKey(candidate)
			if err != nil {
				return nil, fmt.Errorf("%w: claim value", ai.ErrInvalidOutput)
			}
			if index, ok := claimByKey[key]; ok {
				existing := &merged.Claims[index]
				existing.Evidence = mergeCandidateEvidence(existing.Evidence, candidate.Evidence)
				existing.Confidence = max(existing.Confidence, candidate.Confidence)
				continue
			}
			if len(merged.Claims) >= extractionMaxClaims {
				dropped++
				continue
			}
			candidateID, err := s.ids.New()
			if err != nil {
				return nil, err
			}
			candidate.CandidateID = candidateID
			candidate.Evidence = mergeCandidateEvidence(nil, candidate.Evidence)
			claimByKey[key] = len(merged.Claims)
			merged.Claims = append(merged.Claims, candidate)
		}
	}
	if qualityWeight > 0 {
		merged.QualityScore = qualityTotal / float64(qualityWeight)
		if dropped > 0 {
			merged.QualityScore *= float64(qualityWeight-dropped) / float64(qualityWeight)
		}
	}
	return normalizeCandidatesForUse(merged), nil
}

func normalizedEntityKey(candidate EntityCandidate) string {
	return strings.ToLower(strings.Join(strings.Fields(candidate.TypeKey), " ")) + "\x00" +
		strings.ToLower(strings.Join(strings.Fields(candidate.Label), " "))
}

func mergeAliases(existing, incoming []string) []string {
	result := append([]string(nil), existing...)
	seen := make(map[string]bool, len(result)+len(incoming))
	for _, alias := range result {
		seen[strings.ToLower(strings.TrimSpace(alias))] = true
	}
	for _, alias := range incoming {
		key := strings.ToLower(strings.TrimSpace(alias))
		if key == "" || seen[key] || len(result) >= 50 {
			continue
		}
		seen[key] = true
		result = append(result, alias)
	}
	return result
}

func mergeCandidateEvidence(existing, incoming []CandidateEvidence) []CandidateEvidence {
	result := append([]CandidateEvidence(nil), existing...)
	seen := make(map[string]bool, len(result)+len(incoming))
	for _, item := range result {
		seen[evidenceKey(item)] = true
	}
	for _, item := range incoming {
		key := evidenceKey(item)
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}
	return result
}

func normalizedClaimKey(candidate ClaimCandidate) (string, error) {
	subject := ""
	if candidate.Subject.CandidateID != nil {
		subject = "candidate:" + candidate.Subject.CandidateID.String()
	} else if candidate.Subject.EntityID != nil {
		subject = "entity:" + candidate.Subject.EntityID.String()
	}
	value, err := canonicalJSON(candidate.Value)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{subject, strings.ToLower(strings.TrimSpace(candidate.PropertyKey)), string(value),
		optionalTimeKey(candidate.ValidFrom), optionalTimeKey(candidate.ValidTo)}, "\x00"), nil
}

func optionalTimeKey(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
