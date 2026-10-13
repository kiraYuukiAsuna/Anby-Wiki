package importer

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/google/uuid"
)

const (
	preformattedMaxSignalGap     = 8
	preformattedMaxEvidenceRunes = 4000
	preformattedMinSignalLines   = 3
	preformattedMaxPlanBlocks    = 16
	preformattedMaxPlanRunes     = 32000
)

type preformattedLine struct {
	text       string
	start, end int
}

type preformattedSourceBlock struct {
	text     string
	evidence CandidateEvidence
}

// ensurePlanPreformattedBlocks preserves layout-bearing source material that a
// prose model cannot safely summarize. Mapping is intentionally limited to one
// create/update route so a diagram is never attached to a guessed page.
func ensurePlanPreformattedBlocks(plan *ImportPlan, chunks []evidence.SourceChunk, instructions string) int {
	if plan == nil || actionablePageRouteCount(plan.Routes) != 1 ||
		instructionsExcludePreformatted(instructions) {
		return 0
	}
	routeIndex := -1
	for index := range plan.Routes {
		if plan.Routes[index].Action == RouteCreate || plan.Routes[index].Action == RouteUpdate {
			routeIndex = index
			break
		}
	}
	if routeIndex < 0 {
		return 0
	}

	sources := sourcePreformattedBlocks(chunks)
	sourceKeys := make(map[string]bool, len(sources))
	for _, source := range sources {
		sourceKeys[preformattedKey(source.text)] = true
	}
	existing := map[string]bool{}
	blocks := make([]PlannedBlock, 0, len(plan.Routes[routeIndex].Blocks)+len(sources))
	onlyPreformatted := instructionsRequireOnlyPreformatted(instructions)
	for _, block := range plan.Routes[routeIndex].Blocks {
		if block.Type != string(ast.BlockCode) {
			if !onlyPreformatted {
				blocks = append(blocks, block)
			}
			continue
		}
		key := preformattedKey(block.Text)
		if sourceKeys[key] {
			existing[key] = true
			continue
		}
		if plannedBlockOverlapsPreformatted(block, sources) {
			continue
		}
		blocks = append(blocks, block)
	}
	added := 0
	appended := map[string]bool{}
	for _, source := range sources {
		key := preformattedKey(source.text)
		if key == "" || appended[key] {
			continue
		}
		blocks = append(blocks, PlannedBlock{
			Type: string(ast.BlockCode), Mode: BlockAppend,
			Text: source.text, Evidence: []CandidateEvidence{source.evidence},
		})
		appended[key] = true
		if !existing[key] {
			added++
		}
	}
	sortPlanCodeBlocksByEvidence(blocks, chunks)
	plan.Routes[routeIndex].Blocks = blocks
	return added
}

func plannedBlockOverlapsPreformatted(block PlannedBlock, sources []preformattedSourceBlock) bool {
	for _, item := range block.Evidence {
		for _, source := range sources {
			evidence := source.evidence
			if item.ChunkID == evidence.ChunkID &&
				item.CharStart < evidence.CharEnd && evidence.CharStart < item.CharEnd {
				return true
			}
		}
	}
	return false
}

func sortPlanCodeBlocksByEvidence(blocks []PlannedBlock, chunks []evidence.SourceChunk) {
	code := make([]PlannedBlock, 0)
	for _, block := range blocks {
		if block.Type == string(ast.BlockCode) {
			code = append(code, block)
		}
	}
	if len(code) < 2 {
		return
	}
	chunkOrder := make(map[uuid.UUID]int, len(chunks))
	for index := range chunks {
		chunkOrder[chunks[index].ID] = index
	}
	sort.SliceStable(code, func(left, right int) bool {
		leftChunk, leftStart, leftOK := plannedBlockEvidencePosition(code[left], chunkOrder)
		rightChunk, rightStart, rightOK := plannedBlockEvidencePosition(code[right], chunkOrder)
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return false
		}
		if leftChunk != rightChunk {
			return leftChunk < rightChunk
		}
		return leftStart < rightStart
	})
	codeIndex := 0
	for index := range blocks {
		if blocks[index].Type != string(ast.BlockCode) {
			continue
		}
		blocks[index] = code[codeIndex]
		codeIndex++
	}
}

func plannedBlockEvidencePosition(block PlannedBlock, chunkOrder map[uuid.UUID]int) (int, int, bool) {
	chunkIndex, charStart, found := 0, 0, false
	for _, item := range block.Evidence {
		index, ok := chunkOrder[item.ChunkID]
		if !ok || item.CharStart < 0 || item.CharEnd <= item.CharStart {
			continue
		}
		if !found || index < chunkIndex || index == chunkIndex && item.CharStart < charStart {
			chunkIndex, charStart, found = index, item.CharStart, true
		}
	}
	return chunkIndex, charStart, found
}

func preformattedPlanCoverage(plan *ImportPlan, chunks []evidence.SourceChunk, instructions string) float64 {
	if plan == nil || actionablePageRouteCount(plan.Routes) != 1 ||
		instructionsExcludePreformatted(instructions) {
		return 1
	}
	expected := sourcePreformattedBlocks(chunks)
	if len(expected) == 0 {
		return 1
	}
	actual := map[string]bool{}
	for _, route := range plan.Routes {
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		for _, block := range route.Blocks {
			if block.Type == string(ast.BlockCode) {
				actual[preformattedKey(block.Text)] = true
			}
		}
	}
	covered, total := 0, 0
	for _, source := range expected {
		weight := utf8.RuneCountInString(source.text)
		total += weight
		if actual[preformattedKey(source.text)] {
			covered += weight
		}
	}
	if total == 0 {
		return 1
	}
	return float64(covered) / float64(total)
}

func sourcePreformattedBlocks(chunks []evidence.SourceChunk) []preformattedSourceBlock {
	result := []preformattedSourceBlock{}
	for index := range chunks {
		chunk := &chunks[index]
		runes := []rune(chunk.TextContent)
		for _, span := range detectPreformattedSpans(runes) {
			for _, part := range splitPreformattedSpan(runes, span.start, span.end) {
				text := string(runes[part.start:part.end])
				result = append(result, preformattedSourceBlock{
					text: text,
					evidence: CandidateEvidence{
						ChunkID: chunk.ID, Quotation: text,
						CharStart: part.start, CharEnd: part.end,
						Page: chunkPage(chunk),
					},
				})
			}
		}
	}
	return result
}

func detectPreformattedSpans(runes []rune) []preformattedLine {
	lines := splitPreformattedLines(runes)
	result := []preformattedLine{}
	for pageStart := 0; pageStart < len(lines); {
		pageEnd := pageStart
		for pageEnd < len(lines) && !strings.ContainsRune(lines[pageEnd].text, '\f') {
			pageEnd++
		}
		signals := make([]int, 0)
		for index := pageStart; index < pageEnd; index++ {
			if isPreformattedSignal(lines[index].text) {
				signals = append(signals, index)
			}
		}
		for start := 0; start < len(signals); {
			end := start + 1
			for end < len(signals) &&
				signals[end]-signals[end-1] <= preformattedMaxSignalGap+1 {
				end++
			}
			if end-start >= preformattedMinSignalLines {
				first, last := signals[start], signals[end-1]
				first = includePreformattedTitle(lines, pageStart, first)
				last = includePreformattedTail(lines, pageEnd, last)
				result = append(result, preformattedLine{
					start: lines[first].start,
					end:   lines[last].end,
				})
			}
			start = end
		}
		pageStart = pageEnd + 1
	}
	return result
}

func splitPreformattedLines(runes []rune) []preformattedLine {
	lines := make([]preformattedLine, 0, strings.Count(string(runes), "\n")+1)
	start := 0
	for index, value := range runes {
		if value != '\n' {
			continue
		}
		lines = append(lines, preformattedLine{text: string(runes[start:index]), start: start, end: index})
		start = index + 1
	}
	if start <= len(runes) {
		lines = append(lines, preformattedLine{text: string(runes[start:]), start: start, end: len(runes)})
	}
	return lines
}

func includePreformattedTitle(lines []preformattedLine, pageStart, first int) int {
	start := first
	for index := first - 1; index >= pageStart && first-index <= 6; index-- {
		text := strings.TrimSpace(lines[index].text)
		if text == "" {
			continue
		}
		if isPageFurniture(text) {
			break
		}
		if isPreformattedContextLine(text) {
			start = index
			continue
		}
		if isPreformattedTitle(text) {
			start = index
		}
		break
	}
	return start
}

func includePreformattedTail(lines []preformattedLine, pageEnd, last int) int {
	for index := last + 1; index < pageEnd && index-last <= 3; index++ {
		text := strings.TrimSpace(lines[index].text)
		if text == "" || isPageFurniture(text) {
			break
		}
		last = index
	}
	return last
}

func isPreformattedSignal(line string) bool {
	if strings.TrimSpace(line) == "" || isPageFurniture(line) {
		return false
	}
	if strings.Count(line, "+") >= 2 && strings.Contains(line, "-") ||
		strings.Count(line, "|") >= 2 ||
		containsAnyPlanPhrase(line, "-->", "<--", "==>", "<==") {
		return true
	}
	structural := 0
	for _, value := range line {
		switch value {
		case '+', '|', '/', '\\', '<', '>', '_', '^', '=':
			structural++
		}
	}
	return structural >= 3 && strings.ContainsAny(line, "+|/\\")
}

func isPreformattedTitle(text string) bool {
	text = strings.TrimSpace(text)
	length := utf8.RuneCountInString(text)
	if length < 4 || length > 96 || strings.HasPrefix(text, "*") ||
		strings.HasPrefix(text, "-") || strings.Contains(text, "=") {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(text)
	if strings.ContainsRune(".?!。！？", last) {
		return false
	}
	for _, value := range text {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			return true
		}
	}
	return false
}

func isPreformattedContextLine(text string) bool {
	hasStructural := false
	for _, value := range text {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			return false
		}
		if strings.ContainsRune("+|/\\<>_^=-", value) {
			hasStructural = true
		}
	}
	return hasStructural
}

func isPageFurniture(text string) bool {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\f"))
	lower := strings.ToLower(text)
	return strings.HasPrefix(lower, "rfc ") || strings.Contains(lower, "[page ")
}

func splitPreformattedSpan(runes []rune, start, end int) []preformattedLine {
	result := []preformattedLine{}
	for start < end {
		partEnd := min(start+preformattedMaxEvidenceRunes, end)
		if partEnd < end {
			minimum := start + preformattedMaxEvidenceRunes/2
			for index := partEnd; index > minimum; index-- {
				if runes[index-1] == '\n' {
					partEnd = index - 1
					break
				}
			}
		}
		if partEnd <= start {
			partEnd = min(start+preformattedMaxEvidenceRunes, end)
		}
		result = append(result, preformattedLine{start: start, end: partEnd})
		start = partEnd
		for start < end && runes[start] == '\n' {
			start++
		}
	}
	return result
}

func preformattedKey(text string) string {
	return strings.Trim(text, "\r\n")
}

func normalizePlannedBlockText(blockType, text string) string {
	if blockType == string(ast.BlockCode) {
		return strings.Trim(text, "\r\n")
	}
	return strings.TrimSpace(text)
}

func instructionsExcludePreformatted(instructions string) bool {
	for _, sentence := range splitPlanSentences(instructions) {
		lower := strings.ToLower(sentence)
		excluded := containsAnyPlanPhrase(lower, "do not", "don't", "exclude", "omit", "不要", "排除", "省略")
		preformatted := containsAnyPlanPhrase(
			lower,
			"diagram", "figure", "ascii art", "flowchart", "flow chart",
			"ascii 图", "固定宽度图", "流程图",
		)
		if excluded && preformatted {
			return true
		}
	}
	return false
}

func instructionsRequireOnlyPreformatted(instructions string) bool {
	lower := strings.ToLower(instructions)
	required := containsAnyPlanPhrase(
		lower,
		"as exact code blocks", "only code blocks",
		"仅添加代码块", "只添加代码块",
	)
	if !required {
		return false
	}
	for _, sentence := range splitPlanSentences(lower) {
		excluded := containsAnyPlanPhrase(
			sentence,
			"do not add", "exclude", "omit",
			"不要添加", "不添加", "排除", "省略",
		)
		prose := containsAnyPlanPhrase(
			sentence,
			"prose", "paragraph", "heading",
			"正文", "段落", "标题",
		)
		if excluded && prose {
			return true
		}
	}
	return false
}
