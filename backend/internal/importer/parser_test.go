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
