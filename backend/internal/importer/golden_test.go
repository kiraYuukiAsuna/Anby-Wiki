package importer_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/importer"
)

func TestGoldenSet_HTMLAndPDFKeepAnnotatedTextAndLocators(t *testing.T) {
	var expected struct {
		HTML struct {
			Contains  []string `json:"contains"`
			Forbidden []string `json:"forbidden"`
		} `json:"html"`
		PDF struct {
			Contains []string `json:"contains"`
			Page     int32    `json:"page"`
		} `json:"pdf"`
		Annotation struct {
			Entity map[string]any `json:"entity"`
			Claim  map[string]any `json:"claim"`
		} `json:"annotation"`
	}
	raw, err := os.ReadFile("testdata/golden/expected.json")
	if err != nil || json.Unmarshal(raw, &expected) != nil {
		t.Fatalf("golden annotation: %v", err)
	}
	parser := importer.NewParser(1200)
	for _, fixture := range []struct {
		name, mime string
		contains   []string
	}{
		{"release.html", "text/html", expected.HTML.Contains},
		{"release.pdf", "application/pdf", expected.PDF.Contains},
	} {
		content, err := os.ReadFile("testdata/golden/" + fixture.name)
		if err != nil {
			t.Fatal(err)
		}
		chunks, err := parser.Parse(fixture.mime, content)
		if err != nil {
			t.Fatalf("%s: %v", fixture.name, err)
		}
		var joined []string
		for _, chunk := range chunks {
			joined = append(joined, chunk.TextContent)
		}
		text := strings.Join(joined, "\n")
		for _, wanted := range fixture.contains {
			if !strings.Contains(text, wanted) {
				t.Fatalf("%s missing %q in %q", fixture.name, wanted, text)
			}
		}
		if fixture.mime == "application/pdf" && (chunks[0].Locator.Page == nil || *chunks[0].Locator.Page != expected.PDF.Page) {
			t.Fatalf("pdf locator=%+v", chunks[0].Locator)
		}
	}
	html, _ := os.ReadFile("testdata/golden/release.html")
	chunks, _ := parser.Parse("text/html", html)
	for _, forbidden := range expected.HTML.Forbidden {
		for _, chunk := range chunks {
			if strings.Contains(chunk.TextContent, forbidden) {
				t.Fatalf("HTML retained forbidden active content %q", forbidden)
			}
		}
	}
	if expected.Annotation.Entity["label"] != "Anby Wiki" || expected.Annotation.Claim["property_key"] != "release_date" {
		t.Fatal("golden annotations are incomplete")
	}
}
