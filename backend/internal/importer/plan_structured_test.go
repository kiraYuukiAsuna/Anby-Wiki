package importer

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/google/uuid"
)

func TestEnsurePlanStructuredBlocksReplacesModelFallbacks(t *testing.T) {
	sourceVersionID := uuid.New()
	checked := false
	hints := []struct {
		text string
		hint sourceBlockHint
	}{
		{
			text: "Validate\nReview\nApply",
			hint: sourceBlockHint{
				BlockType: string(ast.BlockOrderedList),
				Items:     []string{"Validate", "Review", "Apply"},
			},
		},
		{
			text: `fmt.Println("ok")`,
			hint: sourceBlockHint{
				BlockType: string(ast.BlockCode), Text: `fmt.Println("ok")`,
				Language: "go",
			},
		},
		{
			text: "E = mc^2",
			hint: sourceBlockHint{
				BlockType: string(ast.BlockEquation), Expression: "E = mc^2",
			},
		},
		{
			text: "Review generated page",
			hint: sourceBlockHint{
				BlockType: string(ast.BlockTodo), Text: "Review generated page",
				Checked: &checked,
			},
		},
		{
			text: "Import policy\nPreserve source evidence.",
			hint: sourceBlockHint{
				BlockType: string(ast.BlockToggle), Text: "Import policy",
				Children: []sourceBlockHint{{
					BlockType: string(ast.BlockParagraph),
					Text:      "Preserve source evidence.",
				}},
			},
		},
		{
			text: "Review before apply.",
			hint: sourceBlockHint{
				BlockType: string(ast.BlockCallout), Text: "Review before apply.",
				Kind: string(ast.CalloutWarning), Color: string(ast.CalloutColorYellow),
			},
		},
		{
			text: "Stage\tResult\nPlan\tDraft\nReview\tHuman",
			hint: sourceBlockHint{
				BlockType: string(ast.BlockTable),
				Rows: [][]string{
					{"Stage", "Result"}, {"Plan", "Draft"}, {"Review", "Human"},
				},
				HasColumnHeader: true,
			},
		},
	}

	chunks := make([]evidence.SourceChunk, len(hints))
	fallbacks := make([]PlannedBlock, len(hints))
	for index, item := range hints {
		chunkID := uuid.New()
		content, err := json.Marshal(item.hint)
		if err != nil {
			t.Fatal(err)
		}
		chunks[index] = evidence.SourceChunk{
			ID: chunkID, SourceVersionID: sourceVersionID, Ordinal: index,
			ChunkType:   evidence.SourceChunkTypeText,
			TextContent: item.text, ContentJSON: content,
		}
		fallbacks[index] = PlannedBlock{
			Type: string(ast.BlockParagraph), Mode: BlockAppend,
			Text: "Model fallback " + fmt.Sprint(index),
			Evidence: []CandidateEvidence{{
				ChunkID: chunkID, Quotation: item.text,
				CharStart: 0, CharEnd: len([]rune(item.text)),
			}},
		}
	}
	plan := &ImportPlan{
		Profile: SourceProfile{Useful: true},
		Routes: []PageRoute{{
			Action: RouteCreate, Title: "Typed source",
			Confidence: 0.95, Blocks: fallbacks,
		}},
	}

	if added := ensurePlanStructuredBlocks(plan, chunks); added != len(hints) {
		t.Fatalf("added=%d, want %d", added, len(hints))
	}
	got := make([]string, len(plan.Routes[0].Blocks))
	for index, block := range plan.Routes[0].Blocks {
		got[index] = block.Type
		if plannedBlockGroundingScore(block) < minimumPlanBlockGrounding {
			t.Fatalf("block %s is not exactly grounded: %#v", block.Type, block)
		}
	}
	want := []string{
		string(ast.BlockOrderedList), string(ast.BlockCode),
		string(ast.BlockEquation), string(ast.BlockTodo),
		string(ast.BlockToggle), string(ast.BlockCallout),
		string(ast.BlockTable),
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("types=%v, want %v", got, want)
	}
	quality := assessImportPlanQuality(
		plan, chunks, 1, true, DefaultQualityThreshold,
	)
	if !quality.Passed {
		t.Fatalf("quality=%#v, want passed", quality)
	}
}
