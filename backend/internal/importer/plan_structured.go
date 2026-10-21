package importer

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/google/uuid"
)

type sourceBlockHint struct {
	BlockType       string            `json:"block_type"`
	Text            string            `json:"text,omitempty"`
	Level           int               `json:"level,omitempty"`
	IsToggleable    bool              `json:"is_toggleable,omitempty"`
	Checked         *bool             `json:"checked,omitempty"`
	Kind            string            `json:"kind,omitempty"`
	Icon            string            `json:"icon,omitempty"`
	Color           string            `json:"color,omitempty"`
	Language        string            `json:"language,omitempty"`
	Caption         string            `json:"caption,omitempty"`
	Expression      string            `json:"expression,omitempty"`
	Items           []string          `json:"items,omitempty"`
	Rows            [][]string        `json:"rows,omitempty"`
	HasColumnHeader bool              `json:"has_column_header,omitempty"`
	HasRowHeader    bool              `json:"has_row_header,omitempty"`
	Children        []sourceBlockHint `json:"children,omitempty"`
}

func sourceBlockHintJSON(hint sourceBlockHint) json.RawMessage {
	raw, _ := json.Marshal(hint)
	return raw
}

func ensurePlanStructuredBlocks(plan *ImportPlan, chunks []evidence.SourceChunk) int {
	if plan == nil || actionablePageRouteCount(plan.Routes) != 1 {
		return 0
	}
	routeIndex := -1
	for index := range plan.Routes {
		if plan.Routes[index].Action == RouteCreate ||
			plan.Routes[index].Action == RouteUpdate {
			routeIndex = index
			break
		}
	}
	if routeIndex < 0 {
		return 0
	}

	chunkOrder := make(map[uuid.UUID]int, len(chunks))
	hintedChunks := make(map[uuid.UUID]bool)
	structured := make([]PlannedBlock, 0)
	for index := range chunks {
		chunk := &chunks[index]
		chunkOrder[chunk.ID] = index
		var hint sourceBlockHint
		if len(chunk.ContentJSON) == 0 ||
			json.Unmarshal(chunk.ContentJSON, &hint) != nil ||
			strings.TrimSpace(hint.BlockType) == "" {
			continue
		}
		evidenceItem := CandidateEvidence{
			ChunkID: chunk.ID, Quotation: chunk.TextContent,
			CharStart: 0, CharEnd: utf8.RuneCountInString(chunk.TextContent),
			Page: chunkPage(chunk),
		}
		block, ok := plannedBlockFromSourceHint(hint, evidenceItem, 0)
		if !ok {
			continue
		}
		hintedChunks[chunk.ID] = true
		structured = append(structured, block)
	}
	if len(structured) == 0 {
		return 0
	}

	blocks := make([]PlannedBlock, 0, len(plan.Routes[routeIndex].Blocks)+len(structured))
	for _, block := range plan.Routes[routeIndex].Blocks {
		overlaps := false
		for _, item := range block.Evidence {
			if hintedChunks[item.ChunkID] {
				overlaps = true
				break
			}
		}
		if !overlaps {
			blocks = append(blocks, block)
		}
	}
	blocks = append(blocks, structured...)
	sort.SliceStable(blocks, func(left, right int) bool {
		leftChunk, leftStart, leftOK := plannedBlockEvidencePosition(
			blocks[left], chunkOrder,
		)
		rightChunk, rightStart, rightOK := plannedBlockEvidencePosition(
			blocks[right], chunkOrder,
		)
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
	plan.Routes[routeIndex].Blocks = refinePlannedBlocks(blocks)
	return len(structured)
}

func plannedBlockFromSourceHint(
	hint sourceBlockHint,
	evidenceItem CandidateEvidence,
	depth int,
) (PlannedBlock, bool) {
	if depth > 6 {
		return PlannedBlock{}, false
	}
	block := PlannedBlock{
		Type: strings.TrimSpace(hint.BlockType), Mode: BlockAppend,
		Text:  normalizePlannedBlockText(hint.BlockType, hint.Text),
		Level: hint.Level, IsToggleable: hint.IsToggleable,
		Checked: hint.Checked, Kind: strings.TrimSpace(hint.Kind),
		Icon: strings.TrimSpace(hint.Icon), Color: strings.TrimSpace(hint.Color),
		Language: strings.TrimSpace(hint.Language), Caption: strings.TrimSpace(hint.Caption),
		Expression: strings.TrimSpace(hint.Expression),
		Items:      append([]string(nil), hint.Items...), Rows: cloneRows(hint.Rows),
		HasColumnHeader: hint.HasColumnHeader, HasRowHeader: hint.HasRowHeader,
		Evidence: []CandidateEvidence{evidenceItem},
		Children: []PlannedBlock{},
	}
	for _, childHint := range hint.Children {
		child, ok := plannedBlockFromSourceHint(childHint, evidenceItem, depth+1)
		if ok {
			block.Children = append(block.Children, child)
		}
	}
	if !validPlannedBlockContent(block) {
		return PlannedBlock{}, false
	}
	return block, true
}

func cloneRows(rows [][]string) [][]string {
	result := make([][]string, len(rows))
	for index := range rows {
		result[index] = append([]string(nil), rows[index]...)
	}
	return result
}
