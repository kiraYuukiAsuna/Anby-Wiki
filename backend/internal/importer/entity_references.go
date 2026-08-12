package importer

import (
	"sort"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
)

type entityReferenceTarget struct {
	EntityID uuid.UUID
	Label    string
}

func entityReferencesForCompose(params ComposeParams) []entityReferenceTarget {
	resolved := make(map[uuid.UUID]uuid.UUID, len(params.Resolutions))
	for _, resolution := range params.Resolutions {
		switch resolution.Outcome {
		case EntityMatched:
			if resolution.EntityID != nil && *resolution.EntityID != uuid.Nil {
				resolved[resolution.CandidateID] = *resolution.EntityID
			}
		case EntityNewReview:
			if resolution.PlannedEntityID != nil && *resolution.PlannedEntityID != uuid.Nil {
				resolved[resolution.CandidateID] = *resolution.PlannedEntityID
			}
		}
	}
	byLabel := map[string]entityReferenceTarget{}
	ambiguous := map[string]bool{}
	if params.Candidates != nil {
		for _, candidate := range params.Candidates.Entities {
			entityID := resolved[candidate.CandidateID]
			if entityID == uuid.Nil {
				continue
			}
			for _, rawLabel := range append([]string{candidate.Label}, candidate.Aliases...) {
				label := strings.TrimSpace(rawLabel)
				key := normalizedIdentityText(label)
				if key == "" || len([]rune(label)) < 2 {
					continue
				}
				if existing, ok := byLabel[key]; ok {
					if existing.EntityID != entityID {
						ambiguous[key] = true
					}
					continue
				}
				byLabel[key] = entityReferenceTarget{EntityID: entityID, Label: label}
			}
		}
	}
	result := make([]entityReferenceTarget, 0, len(byLabel))
	for key, target := range byLabel {
		if !ambiguous[key] {
			result = append(result, target)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := len([]rune(result[i].Label)), len([]rune(result[j].Label))
		if left != right {
			return left > right
		}
		return result[i].Label < result[j].Label
	})
	return result
}

func inlineNodesWithEntityReferences(text string, targets []entityReferenceTarget) ([]*ast.InlineNode, error) {
	if len(targets) == 0 || strings.TrimSpace(text) == "" {
		return []*ast.InlineNode{{Type: ast.InlineText, Text: text}}, nil
	}
	runes := []rune(text)
	result := make([]*ast.InlineNode, 0, 4)
	used := map[uuid.UUID]bool{}
	start := 0
	for start < len(runes) {
		matchStart, matchEnd, target, found := nextEntityReference(runes, start, targets, used)
		if !found {
			result = appendTextNode(result, string(runes[start:]))
			break
		}
		result = appendTextNode(result, string(runes[start:matchStart]))
		displayText := string(runes[matchStart:matchEnd])
		node, err := ast.NewEntityRefNode(target.EntityID.String(), displayText)
		if err != nil {
			return nil, err
		}
		result = append(result, node)
		used[target.EntityID] = true
		start = matchEnd
	}
	if len(result) == 0 {
		return []*ast.InlineNode{{Type: ast.InlineText, Text: text}}, nil
	}
	return result, nil
}

func nextEntityReference(text []rune, start int, targets []entityReferenceTarget,
	used map[uuid.UUID]bool) (int, int, entityReferenceTarget, bool) {
	bestStart, bestEnd := len(text)+1, 0
	var best entityReferenceTarget
	for _, target := range targets {
		if used[target.EntityID] {
			continue
		}
		label := []rune(target.Label)
		if len(label) == 0 || start+len(label) > len(text) {
			continue
		}
		for index := start; index+len(label) <= len(text); index++ {
			if !runesEqualFold(text[index:index+len(label)], label) ||
				!entityReferenceBoundary(text, index, index+len(label)) {
				continue
			}
			end := index + len(label)
			if index < bestStart || (index == bestStart && end-index > bestEnd-bestStart) {
				bestStart, bestEnd, best = index, end, target
			}
			break
		}
	}
	return bestStart, bestEnd, best, bestStart <= len(text)
}

func runesEqualFold(left, right []rune) bool {
	return len(left) == len(right) && strings.EqualFold(string(left), string(right))
}

func entityReferenceBoundary(text []rune, start, end int) bool {
	if start > 0 && isWordRune(text[start-1]) && isWordRune(text[start]) {
		return false
	}
	if end < len(text) && isWordRune(text[end-1]) && isWordRune(text[end]) {
		return false
	}
	return true
}

func isWordRune(value rune) bool {
	return (unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_') &&
		!unicode.In(value, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func appendTextNode(nodes []*ast.InlineNode, value string) []*ast.InlineNode {
	if value == "" {
		return nodes
	}
	return append(nodes, &ast.InlineNode{Type: ast.InlineText, Text: value})
}
