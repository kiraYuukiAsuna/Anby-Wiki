package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	if len(blocks) != 2 {
		t.Fatalf("blocks=%d, want 2: %#v", len(blocks), blocks)
	}
	if blocks[0].Text != "JSON is a text-based data format." ||
		len(blocks[0].SectionPath) != 1 || blocks[0].SectionPath[0] != "JSON" {
		t.Fatalf("unexpected lead block: %#v", blocks[0])
	}
	if blocks[1].Text != "Use JSON.parse() and JSON.stringify()." ||
		len(blocks[1].SectionPath) != 2 ||
		blocks[1].SectionPath[1] != "Converting objects and text" {
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

func TestVisionPageAnalysisSanitizesOutOfBoundsRegions(t *testing.T) {
	generator := parserGeneratorFunc(func(
		context.Context,
		ai.Request,
	) (*ai.Result, error) {
		return &ai.Result{
			PromptKey: DocumentPageAnalysisPromptKey, PromptVersion: 1,
			Provider: "test", Model: "vision-model",
			JSON: json.RawMessage(`{
				"schema_version":1,
				"page":1,
				"section_path":[],
				"elements":[
					{
						"kind":"text",
						"title":"",
						"text":"Outside page.",
						"summary":"",
						"section_path":[],
						"region":{"x":100,"y":1050,"width":200,"height":100},
						"structured_data":{},
						"confidence":0.8
					},
					{
						"kind":"figure",
						"title":"Clipped",
						"text":"Clipped figure.",
						"summary":"Clipped.",
						"section_path":[],
						"region":{"x":900,"y":100,"width":200,"height":200},
						"structured_data":{},
						"confidence":0.8
					}
				],
				"warnings":[],
				"quality_score":0.8
			}`),
		}, nil
	})
	adapter := visionPDFAdapter{generator: generator}
	analysis, err := adapter.analyzePage(
		context.Background(),
		ParseRequest{Filename: "regions.pdf", Provider: "test", Model: "vision-model"},
		renderedVisionPage{Data: []byte("jpeg"), Width: 1000, Height: 1000},
		"",
		1,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Elements[0].Region != nil ||
		analysis.Elements[1].Region == nil ||
		analysis.Elements[1].Region.Width != 100 ||
		len(analysis.Warnings) != 2 {
		t.Fatalf("unexpected sanitized analysis: %#v", analysis)
	}
	blocks := pageAnalysisBlocks(analysis)
	if blocks[0].ImageRegion != nil ||
		blocks[1].ImageRegion == nil ||
		blocks[1].ImageRegion.Width != 100 {
		t.Fatalf("unexpected sanitized blocks: %#v", blocks)
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
		if request.PromptKey != DocumentPageAnalysisPromptKey ||
			len(request.Images) != 1 ||
			request.Images[0].MediaType != "image/jpeg" ||
			len(request.Images[0].Data) == 0 {
			t.Fatalf("unexpected page request: %#v", request)
		}
		if !strings.Contains(request.Variables["native_text"].(string), "Hello PDF") {
			t.Fatalf("native text=%q", request.Variables["native_text"])
		}
		return &ai.Result{
			PromptKey:     DocumentPageAnalysisPromptKey,
			PromptVersion: 1,
			Provider:      "openai-compatible",
			Model:         "vision-model",
			JSON: json.RawMessage(`{
				"schema_version":1,
				"page":1,
				"section_path":["Overview"],
				"elements":[{
					"kind":"chart",
					"title":"Diagnostic chart",
					"text":"The diagnostic chart contains the label Hello PDF.",
					"summary":"One labeled chart.",
					"section_path":["Overview"],
					"region":{"x":100,"y":100,"width":300,"height":100},
					"structured_data":{"labels":["Hello PDF"]},
					"confidence":0.99
				}],
				"warnings":[],
				"quality_score":0.99
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
	if result.Chunks[0].ChunkType != "page" ||
		!strings.Contains(result.Chunks[0].TextContent, "Hello PDF") ||
		result.Chunks[1].ChunkType != "chart" ||
		result.Chunks[1].Locator.ImageRegion == nil {
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
		return &ai.Result{
			PromptKey: DocumentPageAnalysisPromptKey, PromptVersion: 1,
			Provider: "test", Model: "vision-model",
			JSON: json.RawMessage(`{
				"schema_version":1,
				"page":1,
				"section_path":["Scanned page"],
				"elements":[{
					"kind":"text",
					"title":"",
					"text":"Vision-only transcription.",
					"summary":"",
					"section_path":["Scanned page"],
					"region":{"x":100,"y":100,"width":500,"height":100},
					"structured_data":{},
					"confidence":0.95
				}],
				"warnings":[],
				"quality_score":0.95
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

func TestParseImageOCRRuntime(t *testing.T) {
	imagePath := strings.TrimSpace(os.Getenv("ANBY_OCR_TEST_IMAGE"))
	if imagePath == "" {
		t.Skip("set ANBY_OCR_TEST_IMAGE to exercise the installed OCR runtime")
	}
	content, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read OCR fixture: %v", err)
	}
	result, err := NewParser(1200, nil).Parse(context.Background(), ParseRequest{
		MIMEType: "image/png",
		Content:  content,
	})
	if err != nil {
		t.Fatalf("parse OCR image: %v", err)
	}
	chunks := result.Chunks
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
