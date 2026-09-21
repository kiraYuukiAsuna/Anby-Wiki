package importer

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/google/uuid"
)

type planBlockLimits struct {
	heading    int
	content    int
	hasHeading bool
	hasContent bool
}

var instructionIdentifierPattern = regexp.MustCompile(
	`(?i)\b[a-z_$][a-z0-9_$]*(?:\.[a-z_$][a-z0-9_$]*)*\(\)`,
)

// refineImportPlan deterministically merges model-authored windows and
// enforces the final article structure without another full-plan model call.
func refineImportPlan(plan *ImportPlan) {
	if plan == nil {
		return
	}
	plan.Profile.Subjects = refineSourceSubjects(plan.Profile.Subjects)
	routes := make([]PageRoute, 0, len(plan.Routes))
	for index := range plan.Routes {
		route := plan.Routes[index]
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			routes = append(routes, route)
			continue
		}
		route.Blocks = refinePlannedBlocks(route.Blocks)
		if len(route.Blocks) > 0 {
			routes = append(routes, route)
		}
	}
	plan.Routes = routes
	normalizeImportPlanCollections(plan)
}

func refineSourceSubjects(subjects []SourceSubject) []SourceSubject {
	result := make([]SourceSubject, 0, len(subjects))
	for _, subject := range subjects {
		subject.Title = strings.TrimSpace(subject.Title)
		subject.Kind = strings.TrimSpace(subject.Kind)
		subject.Summary = strings.TrimSpace(subject.Summary)
		if subject.Title == "" || subject.Kind == "" {
			continue
		}
		merged := false
		for index := range result {
			if !sourceSubjectsEquivalent(result[index], subject) {
				continue
			}
			if preferEntityLabel(subject.Kind, subject.Title, result[index].Title) {
				result[index].Title = subject.Title
			}
			if utf8.RuneCountInString(subject.Summary) > utf8.RuneCountInString(result[index].Summary) {
				result[index].Summary = subject.Summary
			}
			merged = true
			break
		}
		if !merged && len(result) < 20 {
			result = append(result, subject)
		}
	}
	return result
}

func sourceSubjectsEquivalent(left, right SourceSubject) bool {
	if normalizedPlanText(left.Kind) != normalizedPlanText(right.Kind) {
		return false
	}
	leftKeys, rightKeys := map[string]bool{}, map[string]bool{}
	addPlanIdentityKeys(leftKeys, left.Title)
	addPlanIdentityKeys(rightKeys, right.Title)
	for key := range leftKeys {
		if rightKeys[key] {
			return true
		}
	}
	return normalizedPlanText(left.Kind) == "person" && personNamesCompatible(left.Title, right.Title)
}

func refinePlannedBlocks(blocks []PlannedBlock) []PlannedBlock {
	result := make([]PlannedBlock, 0, len(blocks))
	seenSentences := map[string]bool{}
	seenHeadings := map[string]bool{}
	discardLevel := 0
	for _, raw := range blocks {
		block := raw
		block.Text = strings.TrimSpace(block.Text)
		if block.Type == string(ast.BlockBulletList) {
			block.Items = refineListItems(block.Items)
			if len(block.Items) == 0 {
				continue
			}
		} else if block.Text == "" {
			continue
		}

		if block.Type == string(ast.BlockHeading) {
			if block.Level < 1 || block.Level > 6 {
				block.Level = 2
			}
			if discardLevel > 0 {
				if block.Level > discardLevel {
					continue
				}
				discardLevel = 0
			}
			if isDiscardedImportSection(block.Text) {
				discardLevel = block.Level
				continue
			}
			if len(result) == 0 && isGenericLeadHeading(block.Text) {
				continue
			}
			topic := canonicalHeadingTopic(block.Text)
			if topic != "" && seenHeadings[topic] {
				// A second independently planned copy of the same section must not
				// leak its following prose under the previous, unrelated heading.
				discardLevel = block.Level
				continue
			}
			seenHeadings[topic] = true
			result = append(result, block)
			continue
		}
		if discardLevel > 0 {
			continue
		}
		if block.Type == string(ast.BlockParagraph) || block.Type == string(ast.BlockQuote) {
			block.Text = deduplicatePlanSentences(block.Text, seenSentences)
			if block.Text == "" {
				continue
			}
			if index := equivalentPlannedBlockIndex(result, block); index >= 0 {
				evidence := mergeCandidateEvidence(result[index].Evidence, block.Evidence)
				result[index].Evidence = evidence[:min(len(evidence), 8)]
				continue
			}
		}
		result = append(result, block)
	}
	return normalizeArticleHeadingLevels(removeOrphanHeadings(result))
}

func equivalentPlannedBlockIndex(blocks []PlannedBlock, incoming PlannedBlock) int {
	for index := range blocks {
		existing := blocks[index]
		if existing.Type != incoming.Type || existing.Mode != incoming.Mode ||
			!sameOptionalString(existing.TargetBlockID, incoming.TargetBlockID) {
			continue
		}
		if proseEquivalent(existing.Text, incoming.Text) {
			return index
		}
	}
	return -1
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func applyExplicitPlanBlockLimits(plan *ImportPlan, instructions string) int {
	if plan == nil {
		return 0
	}
	limits := explicitPlanBlockLimits(instructions)
	if !limits.hasHeading && !limits.hasContent {
		return 0
	}
	actionable := actionablePageRouteCount(plan.Routes)
	if actionable != 1 {
		return 0
	}
	for index := range plan.Routes {
		route := &plan.Routes[index]
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		before := len(route.Blocks)
		route.Blocks = selectPlannedBlocksWithinLimits(route.Blocks, limits, instructions)
		return before - len(route.Blocks)
	}
	return 0
}

func explicitPlanBlockLimits(instructions string) planBlockLimits {
	var result planBlockLimits
	for _, sentence := range splitPlanSentences(instructions) {
		lower := strings.ToLower(sentence)
		if !containsAnyPlanPhrase(lower, "at most", "no more than", "maximum", "最多", "不超过") {
			continue
		}
		words := strings.FieldsFunc(lower, func(character rune) bool {
			return !unicode.IsLetter(character) && !unicode.IsDigit(character)
		})
		for index, word := range words {
			switch word {
			case "heading", "headings":
				if value, ok := maximumPlanNumber(words[max(0, index-8):index]); ok {
					result.heading, result.hasHeading = value, true
				}
			case "block", "blocks", "paragraph", "paragraphs":
				if value, ok := maximumPlanNumber(words[max(0, index-8):index]); ok {
					result.content, result.hasContent = value, true
				}
			}
		}
		if value, ok := chinesePlanLimit(lower, []string{"个内容块", "个段落", "内容块", "段落"}); ok {
			result.content, result.hasContent = value, true
		}
		if value, ok := chinesePlanLimit(lower, []string{"个小标题", "个标题", "小标题", "标题"}); ok {
			result.heading, result.hasHeading = value, true
		}
	}
	return result
}

func containsAnyPlanPhrase(value string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

func maximumPlanNumber(words []string) (int, bool) {
	result, found := 0, false
	for _, word := range words {
		value, ok := planNumber(word)
		if ok && (!found || value > result) {
			result, found = value, true
		}
	}
	return result, found
}

func planNumber(value string) (int, bool) {
	if number, err := strconv.Atoi(value); err == nil && number >= 0 && number <= 10 {
		return number, true
	}
	switch value {
	case "zero":
		return 0, true
	case "one":
		return 1, true
	case "two":
		return 2, true
	case "three":
		return 3, true
	case "four":
		return 4, true
	case "five":
		return 5, true
	case "six":
		return 6, true
	case "seven":
		return 7, true
	case "eight":
		return 8, true
	case "nine":
		return 9, true
	case "ten":
		return 10, true
	default:
		return 0, false
	}
}

func chinesePlanLimit(value string, suffixes []string) (int, bool) {
	numerals := []struct {
		text  string
		value int
	}{
		{"十", 10}, {"九", 9}, {"八", 8}, {"七", 7}, {"六", 6}, {"五", 5},
		{"四", 4}, {"三", 3}, {"二", 2}, {"两", 2}, {"一", 1}, {"零", 0},
	}
	for _, suffix := range suffixes {
		position := strings.Index(value, suffix)
		if position < 0 {
			continue
		}
		prefix := strings.TrimSpace(value[:position])
		for _, numeral := range numerals {
			if strings.HasSuffix(prefix, numeral.text) {
				return numeral.value, true
			}
		}
		for number := 10; number >= 0; number-- {
			if strings.HasSuffix(prefix, strconv.Itoa(number)) {
				return number, true
			}
		}
	}
	return 0, false
}

func selectPlannedBlocksWithinLimits(
	blocks []PlannedBlock,
	limits planBlockLimits,
	instructions string,
) []PlannedBlock {
	contentLimit := len(blocks)
	if limits.hasContent {
		contentLimit = limits.content
	}
	contentIndexes := make([]int, 0, len(blocks))
	for index := range blocks {
		if blocks[index].Type != string(ast.BlockHeading) {
			contentIndexes = append(contentIndexes, index)
		}
	}
	selectedContent := map[int]bool{}
	if len(contentIndexes) <= contentLimit {
		for _, index := range contentIndexes {
			selectedContent[index] = true
		}
	} else {
		selectRelevantContentBlocks(blocks, contentIndexes, contentLimit, instructions, selectedContent)
	}

	selectedHeadings := map[int]bool{}
	headingLimit := len(blocks)
	if limits.hasHeading {
		headingLimit = limits.heading
	}
	for _, contentIndex := range contentIndexes {
		if !selectedContent[contentIndex] {
			continue
		}
		for headingIndex := contentIndex - 1; headingIndex >= 0; headingIndex-- {
			if blocks[headingIndex].Type != string(ast.BlockHeading) {
				continue
			}
			if len(selectedHeadings) < headingLimit {
				selectedHeadings[headingIndex] = true
			}
			break
		}
	}

	result := make([]PlannedBlock, 0, len(selectedContent)+len(selectedHeadings))
	for index, block := range blocks {
		if selectedContent[index] || selectedHeadings[index] {
			result = append(result, block)
		}
	}
	return normalizeArticleHeadingLevels(removeOrphanHeadings(result))
}

func selectRelevantContentBlocks(
	blocks []PlannedBlock,
	contentIndexes []int,
	limit int,
	instructions string,
	selected map[int]bool,
) {
	if limit <= 0 {
		return
	}
	instructionTokens, excludedTokens := instructionSelectionTokens(instructions)
	requiredIdentifiers := explicitInstructionIdentifiers(instructions)
	coveredInstruction := map[string]bool{}
	coveredContent := map[string]bool{}
	coveredIdentifiers := map[string]bool{}
	for len(selected) < limit {
		bestIndex, bestScore := -1, -1
		for _, index := range contentIndexes {
			if selected[index] {
				continue
			}
			score := plannedBlockSelectionScore(
				blocks[index], instructionTokens, excludedTokens, requiredIdentifiers,
				coveredInstruction, coveredContent, coveredIdentifiers,
			)
			if score > bestScore {
				bestIndex, bestScore = index, score
			}
		}
		if bestIndex < 0 {
			break
		}
		selected[bestIndex] = true
		for token := range planTokenSet(plannedBlockText(blocks[bestIndex])) {
			coveredContent[token] = true
			if instructionTokens[token] {
				coveredInstruction[token] = true
			}
		}
		text := strings.ToLower(plannedBlockText(blocks[bestIndex]))
		for _, identifier := range requiredIdentifiers {
			if strings.Contains(text, identifier) {
				coveredIdentifiers[identifier] = true
			}
		}
	}
}

func plannedBlockSelectionScore(
	block PlannedBlock,
	instructionTokens, excludedTokens map[string]bool,
	requiredIdentifiers []string,
	coveredInstruction, coveredContent, coveredIdentifiers map[string]bool,
) int {
	text := strings.ToLower(plannedBlockText(block))
	blockTokens := planTokenSet(text)
	newInstruction, instructionMatches, excludedMatches, novelContent := 0, 0, 0, 0
	for token := range blockTokens {
		if instructionTokens[token] {
			instructionMatches++
			if !coveredInstruction[token] {
				newInstruction++
			}
		}
		if excludedTokens[token] {
			excludedMatches++
		}
		if !coveredContent[token] {
			novelContent++
		}
	}
	tokenCount := max(1, len(blockTokens))
	relevance := instructionMatches * 100 / tokenCount
	grounding := int(plannedBlockGroundingScore(block) * 100)
	newIdentifiers := 0
	for _, identifier := range requiredIdentifiers {
		if strings.Contains(text, identifier) && !coveredIdentifiers[identifier] {
			newIdentifiers++
		}
	}
	score := newIdentifiers*10_000_000 + newInstruction*1_000_000 + instructionMatches*10_000 +
		grounding*100 + relevance + min(novelContent, 20)
	if grounding < int(minimumPlanBlockGrounding*100) {
		score -= 100_000_000
	}
	return score - excludedMatches*100_000_000
}

func instructionSelectionTokens(instructions string) (map[string]bool, map[string]bool) {
	var positive strings.Builder
	var excluded strings.Builder
	for _, sentence := range splitPlanSentences(instructions) {
		lower := strings.ToLower(sentence)
		if containsAnyPlanPhrase(lower, "do not", "don't", "avoid", "exclude", "不要", "避免") {
			if excluded.Len() > 0 {
				excluded.WriteByte(' ')
			}
			excluded.WriteString(sentence)
			continue
		}
		if containsAnyPlanPhrase(lower, "at most", "no more than", "maximum", "最多", "不超过") {
			continue
		}
		if positive.Len() > 0 {
			positive.WriteByte(' ')
		}
		positive.WriteString(sentence)
	}
	result := planTokenSet(positive.String())
	for _, token := range []string{
		"update", "only", "existing", "page", "add", "concise", "section", "common", "use",
		"focused", "content", "block", "blocks", "exact", "evidence",
	} {
		delete(result, token)
	}
	excludedTokens := planTokenSet(excluded.String())
	for _, token := range []string{
		"do", "not", "include", "repeat", "existing", "section", "sections", "avoid", "exclude",
		"type", "types", "detail", "details",
	} {
		delete(excludedTokens, token)
	}
	for token := range result {
		delete(excludedTokens, token)
	}
	return result, excludedTokens
}

func explicitInstructionIdentifiers(instructions string) []string {
	matches := instructionIdentifierPattern.FindAllString(strings.ToLower(instructions), -1)
	result := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		if !seen[match] {
			seen[match] = true
			result = append(result, match)
		}
	}
	return result
}

func constrainedPlanInstructionCoverage(plan *ImportPlan, instructions string) float64 {
	limits := explicitPlanBlockLimits(instructions)
	if plan == nil || actionablePageRouteCount(plan.Routes) != 1 ||
		!limits.hasHeading && !limits.hasContent {
		return 1
	}
	instructionTokens, _ := instructionSelectionTokens(instructions)
	if len(instructionTokens) == 0 {
		return 1
	}
	requiredIdentifiers := explicitInstructionIdentifiers(instructions)
	contentTokens := map[string]bool{}
	var content strings.Builder
	for _, route := range plan.Routes {
		if route.Action != RouteCreate && route.Action != RouteUpdate {
			continue
		}
		for _, block := range route.Blocks {
			text := plannedBlockText(block)
			for token := range planTokenSet(text) {
				contentTokens[token] = true
			}
			content.WriteByte(' ')
			content.WriteString(strings.ToLower(text))
		}
	}
	for _, identifier := range requiredIdentifiers {
		if !strings.Contains(content.String(), identifier) {
			return 0
		}
	}
	covered := 0
	for token := range instructionTokens {
		if contentTokens[token] {
			covered++
		}
	}
	return float64(covered) / float64(len(instructionTokens))
}

func importPlanExpansionWithinBudget(
	plan *ImportPlan,
	candidates []PageCandidate,
	instructions string,
) bool {
	if plan == nil {
		return false
	}
	candidateByID := make(map[uuid.UUID]PageCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.PageID] = candidate
	}
	limits := explicitPlanBlockLimits(instructions)
	singleActionable := actionablePageRouteCount(plan.Routes) == 1
	for _, route := range plan.Routes {
		if route.Action != RouteUpdate || route.PageID == nil {
			continue
		}
		if len(route.Blocks) > 100 {
			return false
		}
		currentContent, currentRunes := 0, 0
		if candidate, ok := candidateByID[*route.PageID]; ok {
			for _, block := range candidate.Blocks {
				if block.Type != string(ast.BlockHeading) {
					currentContent++
					currentRunes += utf8.RuneCountInString(block.Text)
				}
			}
		}
		contentLimit := max(12, min(32, currentContent/2+4))
		headingLimit := 8
		if singleActionable && limits.hasContent {
			contentLimit = limits.content
		}
		if singleActionable && limits.hasHeading {
			headingLimit = limits.heading
		}
		appendContent, appendHeadings, appendRunes := 0, 0, 0
		for _, block := range route.Blocks {
			if block.Mode != BlockAppend {
				continue
			}
			if block.Type == string(ast.BlockHeading) {
				appendHeadings++
				continue
			}
			appendContent++
			appendRunes += utf8.RuneCountInString(plannedBlockText(block))
		}
		runeLimit := max(6000, min(24000, currentRunes/2+4000))
		if appendContent > contentLimit || appendHeadings > headingLimit || appendRunes > runeLimit {
			return false
		}
	}
	return true
}

func normalizeArticleHeadingLevels(blocks []PlannedBlock) []PlannedBlock {
	firstLevel := 0
	previousLevel := 2
	for index := range blocks {
		if blocks[index].Type != string(ast.BlockHeading) {
			continue
		}
		if firstLevel == 0 {
			firstLevel = blocks[index].Level
		}
		level := blocks[index].Level - firstLevel + 2
		if level < 2 {
			level = 2
		}
		if level > 6 {
			level = 6
		}
		if level > previousLevel+1 {
			level = previousLevel + 1
		}
		blocks[index].Level = level
		previousLevel = level
	}
	return blocks
}

func refineListItems(items []string) []string {
	result := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, raw := range items {
		item := strings.TrimSpace(raw)
		key := normalizedSentenceKey(item)
		if item == "" || key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func deduplicatePlanSentences(text string, seen map[string]bool) string {
	parts := splitPlanSentences(text)
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		key := normalizedSentenceKey(part)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, strings.TrimSpace(part))
	}
	return joinPlanSentences(kept)
}

func splitPlanSentences(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	result := make([]string, 0, 4)
	start := 0
	for index, value := range runes {
		boundary := value == '。' || value == '！' || value == '？' ||
			((value == '.' || value == '!' || value == '?') &&
				(index+1 == len(runes) || unicode.IsSpace(runes[index+1])))
		if !boundary {
			continue
		}
		if part := strings.TrimSpace(string(runes[start : index+1])); part != "" {
			result = append(result, part)
		}
		start = index + 1
	}
	if start < len(runes) {
		if part := strings.TrimSpace(string(runes[start:])); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func normalizedSentenceKey(value string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(parts, " ")
}

func joinPlanSentences(values []string) string {
	var builder strings.Builder
	for _, value := range values {
		if builder.Len() > 0 && !endsWithCJKPunctuation(builder.String()) {
			builder.WriteByte(' ')
		}
		builder.WriteString(value)
	}
	return strings.TrimSpace(builder.String())
}

func endsWithCJKPunctuation(value string) bool {
	runes := []rune(value)
	if len(runes) == 0 {
		return false
	}
	switch runes[len(runes)-1] {
	case '。', '！', '？':
		return true
	default:
		return false
	}
}

func removeOrphanHeadings(blocks []PlannedBlock) []PlannedBlock {
	result := make([]PlannedBlock, 0, len(blocks))
	for index, block := range blocks {
		if block.Type != string(ast.BlockHeading) {
			result = append(result, block)
			continue
		}
		hasContent := false
		for next := index + 1; next < len(blocks); next++ {
			candidate := blocks[next]
			if candidate.Type == string(ast.BlockHeading) && candidate.Level <= block.Level {
				break
			}
			if candidate.Type != string(ast.BlockHeading) {
				hasContent = true
				break
			}
		}
		if hasContent {
			result = append(result, block)
		}
	}
	return result
}

func isGenericLeadHeading(value string) bool {
	switch canonicalHeadingTopic(value) {
	case "overview", "introduction", "summary", "概述", "简介", "摘要":
		return true
	default:
		return false
	}
}

func isDiscardedImportSection(value string) bool {
	switch normalizedSentenceKey(value) {
	case "references", "reference", "bibliography", "normative references", "informative references",
		"author addresses", "authors addresses", "see also", "related topics", "related articles",
		"further reading", "external links", "参考文献", "参考资料", "引用文献", "作者地址",
		"参见", "相关条目", "相关主题", "拓展阅读", "扩展阅读", "外部链接":
		return true
	default:
		return false
	}
}

func canonicalHeadingTopic(value string) string {
	key := normalizedSentenceKey(value)
	for _, token := range strings.Fields(key) {
		if token == "author" || token == "authors" || token == "authorship" {
			return "authors"
		}
	}
	if strings.Contains(key, "作者") {
		return "authors"
	}
	return key
}
