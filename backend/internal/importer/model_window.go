package importer

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
)

const minAdaptiveModelWindowCharacters = 512

// modelSourceChunk is a transient view of an immutable SourceChunk. Full
// chunks keep their real ID. Adaptive child windows get a deterministic ID so
// model evidence can be verified against the smaller text and then mapped back
// to the original chunk and absolute rune offsets before persistence.
type modelSourceChunk struct {
	View     evidence.SourceChunk
	Original evidence.SourceChunk
	Start    int
	End      int
}

func initialModelSourceChunks(chunks []evidence.SourceChunk) []modelSourceChunk {
	result := make([]modelSourceChunk, len(chunks))
	for index := range chunks {
		result[index] = modelSourceChunk{
			View: chunks[index], Original: chunks[index],
			Start: 0, End: utf8.RuneCountInString(chunks[index].TextContent),
		}
	}
	return result
}

func modelSourceChunkViews(chunks []modelSourceChunk) []evidence.SourceChunk {
	result := make([]evidence.SourceChunk, len(chunks))
	for index := range chunks {
		result[index] = chunks[index].View
	}
	return result
}

func batchModelSourceChunks(chunks []modelSourceChunk, maxInputTokens int) [][]modelSourceChunk {
	if maxInputTokens <= 0 {
		maxInputTokens = DefaultModelMaxInputTokens
	}
	reserve := max(extractionInputMinReserve, maxInputTokens/5)
	budget := min(max(1, maxInputTokens-reserve), extractionBatchTarget)
	return batchModelSourceChunksWithinBudget(chunks, budget)
}

func batchModelSourceChunksWithinBudget(chunks []modelSourceChunk, budget int) [][]modelSourceChunk {
	if len(chunks) == 0 {
		return [][]modelSourceChunk{{}}
	}
	budget = max(1, budget)
	chunks = fitModelSourceChunksToBudget(chunks, budget)
	batches := make([][]modelSourceChunk, 0, (len(chunks)+extractionBatchMaxChunks-1)/extractionBatchMaxChunks)
	start, tokens := 0, 0
	for index := range chunks {
		chunkTokens := estimateChunkInputTokens(chunks[index].View)
		if index > start && (index-start >= extractionBatchMaxChunks || tokens+chunkTokens > budget) {
			batches = append(batches, chunks[start:index])
			start, tokens = index, 0
		}
		tokens += chunkTokens
	}
	batches = append(batches, chunks[start:])
	return batches
}

func fitModelSourceChunksToBudget(chunks []modelSourceChunk, budget int) []modelSourceChunk {
	result := make([]modelSourceChunk, 0, len(chunks))
	var appendChunk func(modelSourceChunk)
	appendChunk = func(chunk modelSourceChunk) {
		if estimateChunkInputTokens(chunk.View) <= budget {
			result = append(result, chunk)
			return
		}
		left, right, ok := splitModelSourceChunks([]modelSourceChunk{chunk})
		if !ok {
			result = append(result, chunk)
			return
		}
		appendChunk(left[0])
		appendChunk(right[0])
	}
	for _, chunk := range chunks {
		appendChunk(chunk)
	}
	return result
}

func splitModelSourceChunks(chunks []modelSourceChunk) ([]modelSourceChunk, []modelSourceChunk, bool) {
	if len(chunks) > 1 {
		middle := len(chunks) / 2
		return chunks[:middle], chunks[middle:], true
	}
	if len(chunks) != 1 {
		return nil, nil, false
	}
	parent := chunks[0]
	runes := []rune(parent.View.TextContent)
	if len(runes) <= minAdaptiveModelWindowCharacters {
		return nil, nil, false
	}
	localEnd := semanticChunkEnd(runes, 0, (len(runes)+1)/2)
	if localEnd <= 0 || localEnd >= len(runes) {
		return nil, nil, false
	}
	left := newChildModelSourceChunk(parent, 0, localEnd, runes[:localEnd])
	right := newChildModelSourceChunk(parent, localEnd, len(runes), runes[localEnd:])
	return []modelSourceChunk{left}, []modelSourceChunk{right}, true
}

func newChildModelSourceChunk(parent modelSourceChunk, localStart, localEnd int, text []rune) modelSourceChunk {
	start, end := parent.Start+localStart, parent.Start+localEnd
	view := parent.View
	view.ID = uuid.NewSHA1(parent.Original.ID, []byte(fmt.Sprintf("model-window:%d:%d", start, end)))
	view.TextContent = string(text)
	view.TextHash = HashBytes([]byte(view.TextContent))
	view.LocatorJSON = modelWindowLocator(parent.Original.LocatorJSON, start, end)
	return modelSourceChunk{View: view, Original: parent.Original, Start: start, End: end}
}

func modelWindowLocator(raw []byte, start, end int) []byte {
	var locator map[string]any
	if json.Unmarshal(raw, &locator) != nil || locator == nil {
		locator = map[string]any{}
	}
	base := 0
	if value, ok := locator["char_start"].(float64); ok {
		base = int(value)
	}
	locator["char_start"] = base + start
	locator["char_end"] = base + end
	canonical, _ := json.Marshal(locator)
	return canonical
}

func modelSourceChunkRuneCount(chunks []modelSourceChunk) int {
	total := 0
	for _, chunk := range chunks {
		total += utf8.RuneCountInString(chunk.View.TextContent)
	}
	return total
}

func remapCandidateEvidence(items []CandidateEvidence, chunks []modelSourceChunk) []CandidateEvidence {
	byViewID := make(map[uuid.UUID]modelSourceChunk, len(chunks))
	for _, chunk := range chunks {
		byViewID[chunk.View.ID] = chunk
	}
	result := make([]CandidateEvidence, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		window, ok := byViewID[item.ChunkID]
		if !ok || item.CharStart < 0 || item.CharEnd <= item.CharStart {
			continue
		}
		start, end := window.Start+item.CharStart, window.Start+item.CharEnd
		originalRunes := []rune(window.Original.TextContent)
		if start < 0 || end > len(originalRunes) || end <= start {
			continue
		}
		canonicalizeEvidence(&item, &window.Original, start, end)
		key := evidenceKey(item)
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}
	return result
}

func remapImportPlanEvidence(plan *ImportPlan, chunks []modelSourceChunk) {
	if plan == nil {
		return
	}
	for routeIndex := range plan.Routes {
		route := &plan.Routes[routeIndex]
		route.Evidence = remapCandidateEvidence(route.Evidence, chunks)
		for blockIndex := range route.Blocks {
			block := &route.Blocks[blockIndex]
			block.Evidence = remapCandidateEvidence(block.Evidence, chunks)
		}
	}
}

func remapCandidatesEvidence(candidates *Candidates, chunks []modelSourceChunk) {
	if candidates == nil {
		return
	}
	for index := range candidates.Entities {
		candidates.Entities[index].Evidence = remapCandidateEvidence(candidates.Entities[index].Evidence, chunks)
	}
	for index := range candidates.Claims {
		candidates.Claims[index].Evidence = remapCandidateEvidence(candidates.Claims[index].Evidence, chunks)
	}
}

func planModelChunkViews(chunks []modelSourceChunk) []map[string]any {
	views := modelSourceChunkViews(chunks)
	result := make([]map[string]any, len(views))
	for index := range views {
		result[index] = map[string]any{
			"chunk_id": views[index].ID, "ordinal": views[index].Ordinal,
			"text": views[index].TextContent, "locator": json.RawMessage(views[index].LocatorJSON),
		}
	}
	return result
}
