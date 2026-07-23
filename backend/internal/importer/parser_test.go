package importer_test

import (
	"errors"
	"testing"

	"github.com/anby/wiki/backend/internal/importer"
)

func TestParser_HTMLSectionsCharactersAndScriptRemoval(t *testing.T) {
	parser := importer.NewParser(8)
	chunks, err := parser.Parse("text/html", []byte(`<!doctype html><html><body>
		<h2>History</h2><p>Alpha &amp; beta gamma</p>
		<script>ignore me</script><p>Delta</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 3 || chunks[0].Locator.Section == nil || *chunks[0].Locator.Section != "History" ||
		chunks[0].Locator.CharStart == nil || *chunks[0].Locator.CharStart != 0 {
		t.Fatalf("chunks=%+v", chunks)
	}
	for _, chunk := range chunks {
		if chunk.TextContent == "ignore me" {
			t.Fatal("script 内容不应进入分块")
		}
	}
}

func TestParser_PDFPageLocatorAndFailureRetainsCallerArtifact(t *testing.T) {
	parser := importer.NewParser(100)
	content := []byte("%PDF-1.4\n%%Page: 1\n(First page) Tj\n%%Page: 2\n(Second page) Tj")
	chunks, err := parser.Parse("application/pdf", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].Locator.Page == nil || *chunks[0].Locator.Page != 1 ||
		chunks[1].Locator.Page == nil || *chunks[1].Locator.Page != 2 {
		t.Fatalf("chunks=%+v", chunks)
	}
	if _, err := parser.Parse("application/pdf", []byte("%PDF-1.4\n/Encrypt true")); !errors.Is(err, importer.ErrParseFailed) {
		t.Fatalf("encrypted err=%v", err)
	}
}
