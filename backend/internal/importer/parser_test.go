package importer

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestParseStructuredSources(t *testing.T) {
	parser := NewParser(1200)

	jsonChunks, err := parser.Parse(
		context.Background(),
		"application/json",
		[]byte(`{"name":"安比","level":60}`),
	)
	if err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if len(jsonChunks) != 1 || !strings.Contains(jsonChunks[0].TextContent, `"name": "安比"`) {
		t.Fatalf("unexpected JSON chunks: %#v", jsonChunks)
	}

	csvChunks, err := parser.Parse(
		context.Background(),
		"text/csv",
		[]byte("name,faction\n安比,狡兔屋\n"),
	)
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(csvChunks) != 1 || csvChunks[0].TextContent != "name=安比; faction=狡兔屋" {
		t.Fatalf("unexpected CSV chunks: %#v", csvChunks)
	}
}

func TestDefaultParserUsesConfiguredProductDefault(t *testing.T) {
	parser := NewParser(0)
	if parser.MaxChunkRunes != 32000 {
		t.Fatalf("default chunk characters=%d, want 32000", parser.MaxChunkRunes)
	}
}

func TestParserPrefersSemanticBoundaryWithinLimit(t *testing.T) {
	parser := NewParser(12)
	chunks, err := parser.Parse(context.Background(), "text/plain", []byte("Alpha one. Beta two. Gamma."))
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 || chunks[0].TextContent != "Alpha one." {
		t.Fatalf("unexpected semantic chunks: %#v", chunks)
	}
	for _, chunk := range chunks {
		if len([]rune(chunk.TextContent)) > 12 {
			t.Fatalf("chunk exceeded limit: %q", chunk.TextContent)
		}
	}
}

func TestParseHTMLUsesMainContentAndSkipsNavigation(t *testing.T) {
	blocks, err := parseHTML([]byte(`<!doctype html>
<html>
  <body>
    <header><nav><a href="/">Documentation home</a></nav></header>
    <main>
      <aside class="reference-toc"><a href="#syntax">On this page</a></aside>
      <h1>JSON</h1>
      <p>JSON is a text-based data format.</p>
      <section>
        <h2>Converting objects and text</h2>
        <p>Use <code>JSON.parse()</code> and <code>JSON.stringify()</code>.</p>
      </section>
    </main>
    <footer>Site legal links</footer>
  </body>
</html>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks=%d, want 2: %#v", len(blocks), blocks)
	}
	if blocks[0].Text != "JSON is a text-based data format." ||
		blocks[0].Section == nil || *blocks[0].Section != "JSON" {
		t.Fatalf("unexpected lead block: %#v", blocks[0])
	}
	if blocks[1].Text != "Use JSON.parse() and JSON.stringify()." ||
		blocks[1].Section == nil || *blocks[1].Section != "Converting objects and text" {
		t.Fatalf("unexpected section block: %#v", blocks[1])
	}
	combined := blocks[0].Text + blocks[1].Text
	if strings.Contains(combined, "Documentation home") || strings.Contains(combined, "On this page") ||
		strings.Contains(combined, "Site legal links") {
		t.Fatalf("navigation leaked into article text: %q", combined)
	}
}

func TestParseHTMLFallsBackToBodyContent(t *testing.T) {
	blocks, err := parseHTML([]byte(`<html><body><h1>Topic</h1><div>Loose body text.</div></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Text != "Loose body text." {
		t.Fatalf("unexpected fallback blocks: %#v", blocks)
	}
}

func TestParseTesseractTSVPreservesImageLocator(t *testing.T) {
	content := strings.Join([]string{
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext",
		"5\t1\t1\t1\t1\t1\t10\t20\t30\t12\t95.0\t安比",
		"5\t1\t1\t1\t1\t2\t42\t20\t45\t12\t91.0\t狡兔屋",
	}, "\n")
	page := int32(3)
	blocks, err := parseTesseractTSV([]byte(content), &page, 200, 100)
	if err != nil {
		t.Fatalf("parse TSV: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Text != "安比狡兔屋" {
		t.Fatalf("unexpected OCR blocks: %#v", blocks)
	}
	if blocks[0].ImageRegion == nil ||
		blocks[0].ImageRegion.X != 10 ||
		blocks[0].ImageRegion.Y != 20 ||
		blocks[0].ImageRegion.Width != 77 ||
		blocks[0].ImageRegion.Height != 12 {
		t.Fatalf("unexpected image region: %#v", blocks[0].ImageRegion)
	}
	if blocks[0].OCR == nil || blocks[0].OCR.Confidence == nil ||
		*blocks[0].OCR.Confidence < 0.92 ||
		*blocks[0].OCR.Confidence > 0.94 {
		t.Fatalf("unexpected OCR metadata: %#v", blocks[0].OCR)
	}
}

func TestValidateUploadDetectsStructuredMIME(t *testing.T) {
	source, err := ValidateUpload(
		context.Background(),
		DefaultURLPolicy(),
		SignatureScanner{},
		"records.json",
		"application/octet-stream",
		[]byte(`{"entity":"安比"}`),
	)
	if err != nil {
		t.Fatalf("validate JSON upload: %v", err)
	}
	if source.MIMEType != "application/json" {
		t.Fatalf("mime=%q, want application/json", source.MIMEType)
	}
}

func TestValidateUploadAcceptsHTMLFragment(t *testing.T) {
	source, err := ValidateUpload(
		context.Background(),
		DefaultURLPolicy(),
		SignatureScanner{},
		"rfc6901.html",
		"text/html",
		[]byte("<pre>Internet Engineering Task Force (IETF)</pre>"),
	)
	if err != nil {
		t.Fatalf("validate HTML fragment: %v", err)
	}
	if source.MIMEType != "text/html" {
		t.Fatalf("mime=%q, want text/html", source.MIMEType)
	}
	blocks, err := parseHTML(source.Content)
	if err != nil {
		t.Fatalf("parse HTML fragment: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Text != "Internet Engineering Task Force (IETF)" {
		t.Fatalf("unexpected HTML fragment blocks: %#v", blocks)
	}
}

func TestParseImageOCRRuntime(t *testing.T) {
	imagePath := strings.TrimSpace(os.Getenv("ANBY_OCR_TEST_IMAGE"))
	if imagePath == "" {
		t.Skip("set ANBY_OCR_TEST_IMAGE to exercise the installed OCR runtime")
	}
	content, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read OCR fixture: %v", err)
	}
	chunks, err := NewParser(1200).Parse(
		context.Background(),
		"image/png",
		content,
	)
	if err != nil {
		t.Fatalf("parse OCR image: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("OCR returned no chunks")
	}
	for _, chunk := range chunks {
		if chunk.Locator.ImageRegion == nil || chunk.Locator.OCR == nil ||
			chunk.Locator.OCR.Confidence == nil {
			t.Fatalf("OCR chunk lost locator metadata: %#v", chunk)
		}
		if err := chunk.Locator.Validate(); err != nil {
			t.Fatalf("invalid OCR locator: %v", err)
		}
	}
}

func TestParseScannedPDFOCRRuntime(t *testing.T) {
	pdfPath := strings.TrimSpace(os.Getenv("ANBY_OCR_TEST_PDF"))
	if pdfPath == "" {
		t.Skip("set ANBY_OCR_TEST_PDF to exercise scanned-PDF OCR")
	}
	content, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read scanned PDF fixture: %v", err)
	}
	chunks, err := NewParser(1200).Parse(
		context.Background(),
		"application/pdf",
		content,
	)
	if err != nil {
		t.Fatalf("parse scanned PDF: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("scanned-PDF OCR returned no chunks")
	}
	for _, chunk := range chunks {
		if chunk.Locator.Page == nil || *chunk.Locator.Page != 1 ||
			chunk.Locator.ImageRegion == nil || chunk.Locator.OCR == nil {
			t.Fatalf("scanned-PDF chunk lost page/OCR locator: %#v", chunk)
		}
		if err := chunk.Locator.Validate(); err != nil {
			t.Fatalf("invalid scanned-PDF locator: %v", err)
		}
	}
}
