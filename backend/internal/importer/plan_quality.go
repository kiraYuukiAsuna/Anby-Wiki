package importer

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/ast"
)

// refineImportPlan is a deterministic safety net around model-authored and
// cross-batch plans. The model consolidator handles semantic organization;
// this pass enforces structural invariants even when consolidation is skipped
// or safely falls back after a provider error.
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
		}
		result = append(result, block)
	}
	return removeOrphanHeadings(result)
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
		"author addresses", "authors addresses", "参考文献", "参考资料", "引用文献", "作者地址":
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
