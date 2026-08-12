package render

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
)

func TestCitationOrderAndBidirectionalAnchors(t *testing.T) {
	firstCitation := uuid.New().String()
	secondCitation := uuid.New().String()
	firstBlock := uuid.New().String()
	secondBlock := uuid.New().String()
	content := func(nodes ...*ast.InlineNode) json.RawMessage {
		raw, err := json.Marshal(nodes)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{
			{ID: firstBlock, Type: ast.BlockParagraph, Content: content(
				&ast.InlineNode{Type: ast.InlineText, Text: "Second first"},
				&ast.InlineNode{Type: ast.InlineCitationReference, CitationID: secondCitation},
			)},
			{ID: secondBlock, Type: ast.BlockParagraph, Content: content(
				&ast.InlineNode{Type: ast.InlineCitationReference, CitationID: firstCitation},
				&ast.InlineNode{Type: ast.InlineText, Text: " repeated "},
				&ast.InlineNode{Type: ast.InlineCitationReference, CitationID: secondCitation},
			)},
		},
	}

	html, err := RenderHTMLWithResolversAndCitationOrder(
		context.Background(), document, nil, nil,
		[]string{firstCitation, secondCitation},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`id="cite-ref-` + firstBlock + `-1"`,
		`href="#cite-note-2">[2]</a>`,
		`id="cite-ref-` + secondBlock + `-0"`,
		`href="#cite-note-1">[1]</a>`,
		`id="cite-ref-` + secondBlock + `-2"`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rendered citation HTML missing %q: %s", expected, html)
		}
	}
}
