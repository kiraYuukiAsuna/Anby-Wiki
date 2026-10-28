package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/ai"
)

func TestParseStructuredSources(t *testing.T) {
	parser := NewParser(1200, nil)

	jsonResult, err := parser.Parse(
		context.Background(),
		ParseRequest{
			MIMEType: "application/json",
			Content:  []byte(`{"name":"安比","level":60}`),
		},
	)
	if err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	jsonChunks := jsonResult.Chunks
	if len(jsonChunks) != 1 || !strings.Contains(jsonChunks[0].TextContent, `"name": "安比"`) {
		t.Fatalf("unexpected JSON chunks: %#v", jsonChunks)
	}

	csvResult, err := parser.Parse(
		context.Background(),
		ParseRequest{
			MIMEType: "text/csv",
			Content:  []byte("name,faction\n安比,狡兔屋\n"),
		},
	)
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	csvChunks := csvResult.Chunks
	if len(csvChunks) != 1 || csvChunks[0].TextContent != "name=安比; faction=狡兔屋" {
		t.Fatalf("unexpected CSV chunks: %#v", csvChunks)
	}
	if csvChunks[0].ChunkType != "table" {
		t.Fatalf("csv chunk type=%q, want table", csvChunks[0].ChunkType)
	}
}

func TestDefaultParserUsesConfiguredProductDefault(t *testing.T) {
	parser := NewParser(0, nil)
	if parser.MaxChunkRunes != 32000 {
		t.Fatalf("default chunk characters=%d, want 32000", parser.MaxChunkRunes)
	}
}

func TestPlainTextParserRemovesNULPadding(t *testing.T) {
	content := []byte("RFC text\x00with padding")
	source, err := ValidateUpload(
		context.Background(), DefaultURLPolicy(), SignatureScanner{},
		"rfc.txt", "text/plain", content,
	)
	if err != nil {
		t.Fatal(err)
	}
	expectedHash := sha256.Sum256(content)
	if source.ContentHash != hex.EncodeToString(expectedHash[:]) {
		t.Fatal("raw content hash changed while validating NUL-padded text")
	}
	result, err := NewParser(0, nil).Parse(context.Background(), ParseRequest{
		Filename: source.Filename, MIMEType: source.MIMEType, Content: source.Content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].TextContent != "RFC textwith padding" {
		t.Fatalf("unexpected sanitized chunks: %#v", result.Chunks)
	}
	if len(result.Metadata.Warnings) != 1 ||
		result.Metadata.Warnings[0] != warningNULBytesRemoved {
		t.Fatalf("warnings=%v, want %q", result.Metadata.Warnings, warningNULBytesRemoved)
	}
}

func TestParserPrefersSemanticBoundaryWithinLimit(t *testing.T) {
	parser := NewParser(12, nil)
	result, err := parser.Parse(context.Background(), ParseRequest{
		MIMEType: "text/plain",
		Content:  []byte("Alpha one. Beta two. Gamma."),
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks := result.Chunks
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
	if len(blocks) != 3 {
		t.Fatalf("blocks=%d, want 3: %#v", len(blocks), blocks)
	}
	if blocks[0].Text != "JSON is a text-based data format." ||
		len(blocks[0].SectionPath) != 1 || blocks[0].SectionPath[0] != "JSON" {
		t.Fatalf("unexpected lead block: %#v", blocks[0])
	}
	if blocks[1].Text != "Converting objects and text" ||
		blocks[2].Text != "Use JSON.parse() and JSON.stringify()." ||
		len(blocks[2].SectionPath) != 2 ||
		blocks[2].SectionPath[1] != "Converting objects and text" {
		t.Fatalf("unexpected section blocks: %#v", blocks[1:])
	}
	combined := blocks[0].Text + blocks[1].Text + blocks[2].Text
	if strings.Contains(combined, "Documentation home") || strings.Contains(combined, "On this page") ||
		strings.Contains(combined, "Site legal links") {
		t.Fatalf("navigation leaked into article text: %q", combined)
	}
}

func TestParseHTMLPreservesTypedSourceBlocks(t *testing.T) {
	blocks, err := parseHTML([]byte(`<html><body><main>
<h1>Typed source</h1>
<h2>Checklist</h2>
<p><input type="checkbox" checked> Verify evidence</p>
<details><summary>Import policy</summary><p>Preserve source evidence.</p></details>
<aside class="warning">Review before apply.</aside>
<ol><li>Validate</li><li>Review</li><li>Apply</li></ol>
<table><caption>Stages</caption><tr><th>Stage</th><th>Result</th></tr><tr><td>Plan</td><td>Draft</td></tr></table>
<pre><code class="language-go">fmt.Println("ok")</code></pre>
<p>$$E = mc^2$$</p>
</main></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(blocks))
	for _, block := range blocks {
		var hint sourceBlockHint
		if json.Unmarshal(block.Content, &hint) == nil && hint.BlockType != "" {
			got = append(got, hint.BlockType)
		}
	}
	want := []string{
		"heading", "todo", "toggle", "callout",
		"ordered_list", "table", "code", "equation",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("typed blocks=%v, want %v", got, want)
	}
}

func TestParsePlainTextPreservesMarkdownBlocks(t *testing.T) {
	blocks, err := parsePlainText([]byte(strings.Join([]string{
		"## Release",
		"- safe",
		"- reviewable",
		"",
		"1. Validate",
		"2. Apply",
		"",
		"- [ ] Review generated page",
		"",
		"```go",
		`fmt.Println("ok")`,
		"```",
		"",
		"$$E = mc^2$$",
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(blocks))
	for _, block := range blocks {
		var hint sourceBlockHint
		if json.Unmarshal(block.Content, &hint) == nil && hint.BlockType != "" {
			got = append(got, hint.BlockType)
		}
	}
	want := []string{"heading", "bullet_list", "ordered_list", "todo", "code", "equation"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("typed blocks=%v, want %v", got, want)
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

func TestVisionPageAnalysisPreservesChartStructureAndRegion(t *testing.T) {
	structured := json.RawMessage(`{
		"series":[{"name":"Latency","points":[{"label":"P95","value":120,"unit":"ms"}]}]
	}`)
	blocks := pageAnalysisBlocks(pageAnalysis{
		Page: 3, RenderWidth: 2000, RenderHeight: 1000,
		SectionPath: []string{"Performance"},
		Elements: []pageElement{{
			Kind: "chart", Title: "Latency", Text: "P95 latency is 120 ms.",
			Summary: "Latency chart", StructuredData: structured,
			Confidence: 0.93,
			Region:     &normalizedRegion{X: 100, Y: 200, Width: 500, Height: 400},
		}},
	})
	if len(blocks) != 1 {
		t.Fatalf("blocks=%d, want 1", len(blocks))
	}
	block := blocks[0]
	if block.Kind != "chart" || block.Page == nil || *block.Page != 3 ||
		len(block.SectionPath) != 1 || block.SectionPath[0] != "Performance" {
		t.Fatalf("unexpected block: %#v", block)
	}
	if block.ImageRegion == nil ||
		block.ImageRegion.X != 200 || block.ImageRegion.Y != 200 ||
		block.ImageRegion.Width != 1000 || block.ImageRegion.Height != 400 ||
		block.ImageRegion.ImageWidth != 2000 || block.ImageRegion.ImageHeight != 1000 {
		t.Fatalf("unexpected chart region: %#v", block.ImageRegion)
	}
	chunks := NewParser(1200, nil).chunk(blocks)
	if len(chunks) != 1 || chunks[0].ChunkType != "chart" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	if err := chunks[0].Locator.Validate(); err != nil {
		t.Fatalf("invalid chart locator: %v", err)
	}
	var content map[string]any
	if err := json.Unmarshal(chunks[0].Content, &content); err != nil {
		t.Fatal(err)
	}
	if content["source"] != "vision_page" {
		t.Fatalf("content=%#v", content)
	}
}

func TestParserFingerprintIncludesFormatProviderAndPromptVersion(t *testing.T) {
	first := parserFingerprint(
		"vision", "1", "hybrid", "application/pdf",
		"openai-compatible", "model", "page", 1, 32000,
	)
	second := parserFingerprint(
		"vision", "1", "hybrid", "application/pdf",
		"openai-compatible", "model", "page", 2, 32000,
	)
	if first == second {
		t.Fatal("parser fingerprint did not change with prompt version")
	}
	otherMIME := parserFingerprint(
		"vision", "1", "hybrid", "text/plain",
		"openai-compatible", "model", "page", 1, 32000,
	)
	if first == otherMIME {
		t.Fatal("parser fingerprint did not change with MIME type")
	}
	otherProvider := parserFingerprint(
		"vision", "1", "hybrid", "application/pdf",
		"deepseek", "model", "page", 1, 32000,
	)
	if first == otherProvider {
		t.Fatal("parser fingerprint did not change with provider")
	}
}

type parserGeneratorFunc func(context.Context, ai.Request) (*ai.Result, error)

func (f parserGeneratorFunc) Generate(
	ctx context.Context,
	request ai.Request,
) (*ai.Result, error) {
	return f(ctx, request)
}

func TestVisionPDFAdapterUsesRenderedPageAndNativeText(t *testing.T) {
	requirePDFTestRuntime(t)
	generator := parserGeneratorFunc(func(
		_ context.Context,
		request ai.Request,
	) (*ai.Result, error) {
		if len(request.Images) != 1 ||
			request.Images[0].MediaType != "image/jpeg" ||
			len(request.Images[0].Data) == 0 {
			t.Fatalf("unexpected page request: %#v", request)
		}
		if request.PromptKey == DocumentPageLayoutPromptKey {
			if !strings.Contains(request.Variables["native_text"].(string), "Hello PDF") {
				t.Fatalf("native text=%q", request.Variables["native_text"])
			}
			return visionTestResult(request, `{
				"schema_version":1, "section_path":["Overview"],
				"elements":[{"kind":"chart","title":"Diagnostic chart","section_path":["Overview"],
					"region":{"x":100,"y":100,"width":300,"height":100}}], "warnings":[]
			}`), nil
		}
		if request.Variables["native_text"] != "" {
			t.Fatal("region recognition received unrelated page text")
		}
		return &ai.Result{
			PromptKey:     DocumentRegionAnalysisPromptKey,
			PromptVersion: 1,
			Provider:      "openai-compatible",
			Model:         "vision-model",
			JSON: json.RawMessage(`{
				"schema_version":1,
					"text":"The diagnostic chart contains the label Hello PDF.",
					"summary":"One labeled chart.",
					"structured_data":{"labels":["Hello PDF"]},
					"confidence":0.99,
				"warnings":[]
			}`),
		}, nil
	})

	result, err := NewParser(32000, generator).Parse(
		context.Background(),
		ParseRequest{
			Filename: "hello.pdf",
			MIMEType: "application/pdf",
			Content:  minimalPDF("Hello PDF"),
			Provider: "semantic-kernel",
			Model:    "vision-model",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Track != "hybrid" || result.ParserName != "anby-vision-pdf" ||
		result.Metadata.PageCount != 1 || result.Metadata.PromptVersion != 1 ||
		result.Metadata.Provider != "openai-compatible" ||
		len(result.Chunks) != 2 {
		t.Fatalf("unexpected parse result: %#v", result)
	}
	if result.Chunks[1].ChunkType != "page" ||
		!strings.Contains(result.Chunks[1].TextContent, "Hello PDF") ||
		result.Chunks[0].ChunkType != "chart" ||
		result.Chunks[0].Locator.ImageRegion == nil {
		t.Fatalf("unexpected parsed chunks: %#v", result.Chunks)
	}
}

func TestVisionPDFAdapterTranscribesScanOnlyPage(t *testing.T) {
	requirePDFTestRuntime(t)
	generator := parserGeneratorFunc(func(
		_ context.Context,
		request ai.Request,
	) (*ai.Result, error) {
		if request.Variables["native_text"] != "" {
			t.Fatalf("native text=%q, want empty scan text", request.Variables["native_text"])
		}
		if request.PromptKey == DocumentPageLayoutPromptKey {
			return &ai.Result{
				PromptKey: request.PromptKey, PromptVersion: 1, Provider: "test", Model: "vision-model",
				JSON: json.RawMessage(`{
					"schema_version":1,"section_path":["Scanned page"],
					"elements":[{"kind":"text","title":"","section_path":["Scanned page"],
						"region":{"x":100,"y":100,"width":500,"height":100}}],"warnings":[]
				}`),
			}, nil
		}
		return &ai.Result{
			PromptKey: DocumentRegionAnalysisPromptKey, PromptVersion: 1,
			Provider: "test", Model: "vision-model",
			JSON: json.RawMessage(`{
				"schema_version":1,
					"text":"Vision-only transcription.",
					"summary":"",
					"structured_data":{},
					"confidence":0.95,
				"warnings":[]
			}`),
		}, nil
	})

	result, err := NewParser(32000, generator).Parse(
		context.Background(),
		ParseRequest{
			Filename: "scan.pdf", MIMEType: "application/pdf",
			Content: minimalPDF(""), Provider: "test", Model: "vision-model",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Track != "vision" || len(result.Chunks) != 1 ||
		result.Chunks[0].ChunkType != "text" ||
		result.Chunks[0].TextContent != "Vision-only transcription." {
		t.Fatalf("unexpected scan-only result: %#v", result)
	}
}

func TestVisionPDFAdapterFailsWholeParseWhenPageAnalysisFails(t *testing.T) {
	requirePDFTestRuntime(t)
	providerFailure := errors.New("provider unavailable")
	generator := parserGeneratorFunc(func(
		context.Context,
		ai.Request,
	) (*ai.Result, error) {
		return nil, providerFailure
	})

	_, err := NewParser(32000, generator).Parse(
		context.Background(),
		ParseRequest{
			Filename: "failed.pdf", MIMEType: "application/pdf",
			Content: minimalPDF("Page"), Provider: "test", Model: "vision-model",
		},
	)
	if !errors.Is(err, ErrVisionPageAnalysisFailed) {
		t.Fatalf("error=%v, want ErrVisionPageAnalysisFailed", err)
	}
}

func requirePDFTestRuntime(t *testing.T) {
	t.Helper()
	for _, command := range []string{"pdfinfo", "pdftotext", "pdftoppm"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is not installed", command)
		}
	}
}

func minimalPDF(text string) []byte {
	var output strings.Builder
	output.WriteString("%PDF-1.4\n")
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
	objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n", len(objects)+1)
	output.WriteString("0000000000 65535 f \n")
	for index := 1; index < len(offsets); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(
		&output,
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1,
		xref,
	)
	return []byte(output.String())
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

func TestValidateUploadReaderStreamsAndRewinds(t *testing.T) {
	content := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("stream-data\n"), 120000)...)
	reader := bytes.NewReader(content)
	validated, err := ValidateUploadReader(
		context.Background(),
		DefaultURLPolicy(),
		SignatureScanner{},
		"large.pdf",
		"application/pdf",
		reader,
	)
	if err != nil {
		t.Fatalf("validate upload reader: %v", err)
	}
	sum := sha256.Sum256(content)
	if validated.SizeBytes != int64(len(content)) ||
		validated.ContentHash != hex.EncodeToString(sum[:]) ||
		validated.MIMEType != "application/pdf" {
		t.Fatalf("unexpected validation metadata: %#v", validated)
	}
	position, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if position != 0 {
		t.Fatalf("reader position=%d, want 0", position)
	}
}

func TestSignatureScannerDetectsCrossChunkPattern(t *testing.T) {
	signature := []byte("eicar-standard-antivirus-test-file")
	content := append(
		bytes.Repeat([]byte("x"), signatureScanChunkSize-len(signature)/2),
		signature...,
	)
	if err := (SignatureScanner{}).Scan(
		context.Background(),
		bytes.NewReader(content),
	); !errors.Is(err, ErrMalware) {
		t.Fatalf("scan error=%v, want ErrMalware", err)
	}
}

func TestValidateUploadReaderRuntime(t *testing.T) {
	path := strings.TrimSpace(os.Getenv("ANBY_LARGE_UPLOAD_TEST_FILE"))
	if path == "" {
		t.Skip("set ANBY_LARGE_UPLOAD_TEST_FILE to exercise streaming validation")
	}
	content, err := os.Open(path)
	if err != nil {
		t.Fatalf("open large upload fixture: %v", err)
	}
	defer content.Close()
	validated, err := ValidateUploadReader(
		context.Background(),
		DefaultURLPolicy(),
		SignatureScanner{},
		filepath.Base(path),
		"application/pdf",
		content,
	)
	if err != nil {
		t.Fatalf("validate large upload fixture: %v", err)
	}
	info, err := content.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if validated.SizeBytes != info.Size() {
		t.Fatalf("validated size=%d, want %d", validated.SizeBytes, info.Size())
	}
}
