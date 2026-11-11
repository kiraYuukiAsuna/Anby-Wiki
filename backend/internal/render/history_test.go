package render

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/ast"
)

type historyResolver struct{ fail bool }

func (r historyResolver) RenderComponent(context.Context, *ast.Block) (string, error) {
	if r.fail {
		return "", errors.New("unavailable")
	}
	return `<aside class="entity-infobox"><h2>Current entity</h2></aside>`, nil
}
func (r historyResolver) RenderClaim(context.Context, string) (string, error) {
	return `<span class="claim-reference">Current value</span>`, nil
}

func TestHistoryHTMLPreservesReadingLayoutAndEveryBlockAnchor(t *testing.T) {
	doc := &ast.Document{Type: "document", SchemaVersion: 1, Children: []*ast.Block{
		{ID: "heading", Type: ast.BlockHeading, Level: 2, IsToggleable: true, Content: []byte(`[{"type":"text","text":"Title <unsafe>"}]`), Children: []*ast.Block{{ID: "p", Type: ast.BlockParagraph, Content: []byte(`[{"type":"text","text":"Body"}]`)}}},
		{ID: "table", Type: ast.BlockTable, HasColumnHeader: true, Children: []*ast.Block{{ID: "row", Type: ast.BlockTableRow, Children: []*ast.Block{{ID: "cell", Type: ast.BlockTableCell, Children: []*ast.Block{{ID: "cell-p", Type: ast.BlockParagraph, Content: []byte(`[{"type":"text","text":"Value"}]`)}}}}}}},
		{ID: "component", Type: ast.BlockComponent},
		{ID: `divider"`, Type: ast.BlockDivider},
	}}
	normal, err := RenderHTMLWithResolvers(context.Background(), doc, historyResolver{}, historyResolver{})
	if err != nil {
		t.Fatal(err)
	}
	history, err := RenderHistoryHTML(context.Background(), doc, historyResolver{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"heading", "p", "table", "row", "cell", "cell-p", "component", "divider&#34;"} {
		if strings.Count(history, `data-history-block-id="`+id+`"`) != 1 {
			t.Fatalf("missing/duplicate anchor %s: %s", id, history)
		}
	}
	stripped := regexp.MustCompile(` data-history-block-id="[^"]*"`).ReplaceAllString(history, "")
	if stripped != normal {
		t.Fatalf("history annotations changed reading layout\n%s\n%s", stripped, normal)
	}
	if !strings.Contains(history, "Title &lt;unsafe&gt;") {
		t.Fatal("text was not escaped")
	}
	if _, err := RenderHistoryHTML(context.Background(), doc, historyResolver{fail: true}); err == nil {
		t.Fatal("resolver failure swallowed")
	}
	if _, err := RenderHistoryHTML(context.Background(), nil, nil); err == nil {
		t.Fatal("nil document accepted")
	}
	empty, err := RenderHistoryHTML(context.Background(), &ast.Document{}, nil)
	if err != nil || empty != "" {
		t.Fatalf("empty document: %q %v", empty, err)
	}
}
