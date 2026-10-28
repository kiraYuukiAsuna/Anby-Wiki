package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
)

func visionTestResult(request ai.Request, output string) *ai.Result {
	return &ai.Result{
		PromptKey: request.PromptKey, PromptVersion: 1,
		Provider: "openai-compatible", Model: "vision-model", JSON: json.RawMessage(output),
	}
}

func visionTestImage(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			pixel := color.RGBA{R: 255, A: 255}
			if x >= width/2 {
				pixel = color.RGBA{B: 255, A: 255}
			}
			img.SetRGBA(x, y, pixel)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestVisionImageUsesOrderedOriginalResolutionCrops(t *testing.T) {
	jobID, runID := uuid.New(), uuid.New()
	calls := 0
	generator := parserGeneratorFunc(func(_ context.Context, request ai.Request) (*ai.Result, error) {
		calls++
		if request.ImportJobID != &jobID || request.ImportRunID != &runID {
			t.Fatal("model call lost import audit context")
		}
		if len(request.Images) != 1 || request.Images[0].MediaType != "image/jpeg" {
			t.Fatal("expected one JPEG")
		}
		img, _, err := image.Decode(bytes.NewReader(request.Images[0].Data))
		if err != nil {
			t.Fatal(err)
		}
		if request.PromptKey == DocumentPageLayoutPromptKey {
			if img.Bounds().Dx() != 1600 || img.Bounds().Dy() != 800 {
				t.Fatalf("layout bounds=%v", img.Bounds())
			}
			return visionTestResult(request, `{
				"schema_version":1,"section_path":["Data"],"warnings":[],
				"elements":[
					{"kind":"table","title":"Right","section_path":[],"region":{"x":500,"y":0,"width":500,"height":1000}},
					{"kind":"text","title":"Left","section_path":[],"region":{"x":0,"y":0,"width":500,"height":1000}}
				]
			}`), nil
		}
		if request.PromptKey != DocumentRegionAnalysisPromptKey ||
			img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 1200 {
			t.Fatalf("region prompt=%s, bounds=%v", request.PromptKey, img.Bounds())
		}
		r, _, b, _ := img.At(600, 600).RGBA()
		switch request.Variables["region_index"] {
		case 0:
			if b <= r || request.Variables["kind"] != "table" {
				t.Fatal("first region was not cropped from the right table")
			}
			return visionTestResult(request, `{
				"schema_version":1,"text":"| 名称 | 数值 |\n|---|---|\n| 延迟 | 120 ms |",
				"summary":"延迟表","structured_data":{"headers":["名称","数值"],"rows":[["延迟","120 ms"]]},
				"confidence":0.9,"warnings":[]
			}`), nil
		case 1:
			if r <= b || request.Variables["kind"] != "text" {
				t.Fatal("second region was not cropped from the left prose")
			}
			return visionTestResult(request, `{
				"schema_version":1,"text":"左栏正文。","summary":"","structured_data":{},
				"confidence":0.95,"warnings":[]
			}`), nil
		default:
			t.Fatal("unexpected region")
			return nil, nil
		}
	})
	result, err := NewParser(32000, generator).Parse(context.Background(), ParseRequest{
		Filename: "table.png", MIMEType: "image/png", Content: visionTestImage(t, 2400, 1200),
		Provider: "semantic-kernel", Model: "vision-model", ImportJobID: &jobID, ImportRunID: &runID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || result.Track != "vision" || len(result.Chunks) != 2 ||
		result.Chunks[0].ChunkType != "table" || result.Chunks[1].TextContent != "左栏正文。" {
		t.Fatalf("unexpected result: calls=%d result=%#v", calls, result)
	}
	region := result.Chunks[0].Locator.ImageRegion
	if region == nil || region.X != 1200 || region.Width != 1200 ||
		region.ImageWidth != 2400 || region.ImageHeight != 1200 {
		t.Fatalf("locator lost original geometry: %#v", region)
	}
	for index, chunk := range result.Chunks {
		if chunk.Ordinal != index || chunk.Locator.OCR != nil {
			t.Fatalf("unexpected LLM chunk: %#v", chunk)
		}
		if err := chunk.Locator.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if len(result.Metadata.Prompts) != 2 ||
		result.Metadata.Prompts[0].Key != DocumentPageLayoutPromptKey ||
		result.Metadata.Prompts[1].Key != DocumentRegionAnalysisPromptKey {
		t.Fatalf("missing stage identities: %#v", result.Metadata)
	}
	var content struct {
		ReadingOrder int             `json:"reading_order"`
		Data         json.RawMessage `json:"structured_data"`
	}
	if json.Unmarshal(result.Chunks[0].Content, &content) != nil ||
		content.ReadingOrder != 0 || !bytes.Contains(content.Data, []byte("120 ms")) {
		t.Fatalf("structured content=%s", result.Chunks[0].Content)
	}
}

func TestNativeRegionsPreserveExactTextOrderAndUnmatchedContent(t *testing.T) {
	native := "左栏\n原文。\n\n右栏 原文。\n\n被漏掉的注释。\n\n重复。\n重复。"
	analysis := pageAnalysis{
		Page: 2, RenderWidth: 1000, RenderHeight: 1000,
		Elements: []pageElement{
			{Kind: "text", Text: "右栏 原文。"},
			{Kind: "text", Text: "左栏 原文。"},
			{Kind: "text", Text: "重复。"},
			{Kind: "text", Text: "模型改写的原文。"},
		},
	}
	blocks := groundedPageBlocks(&analysis, native)
	if len(blocks) != 3 || blocks[0].Text != "右栏 原文。" ||
		blocks[1].Text != "左栏\n原文。" ||
		!strings.Contains(blocks[2].Text, "被漏掉的注释。") ||
		strings.Count(blocks[2].Text, "重复。") != 2 ||
		len(analysis.Warnings) != 2 {
		t.Fatalf("native text was lost or rewritten: %#v warnings=%v", blocks, analysis.Warnings)
	}
	for _, block := range blocks {
		if !strings.Contains(native, block.Text) {
			t.Fatalf("non-native text became evidence: %q", block.Text)
		}
		var span struct {
			Start int `json:"native_char_start"`
			End   int `json:"native_char_end"`
		}
		if json.Unmarshal(block.Content, &span) != nil ||
			string([]rune(native)[span.Start:span.End]) != block.Text {
			t.Fatalf("incorrect native span: %s", block.Content)
		}
	}
}

func TestVisionRejectsInvalidLayoutBeforeRecognition(t *testing.T) {
	content := visionTestImage(t, 100, 100)
	for _, region := range []string{
		`null`,
		`{"x":-1,"y":0,"width":100,"height":100}`,
		`{"x":900,"y":0,"width":200,"height":100}`,
		`{"x":0,"y":900,"width":100,"height":200}`,
		`{"x":0,"y":0,"width":0,"height":100}`,
	} {
		t.Run(region, func(t *testing.T) {
			calls := 0
			generator := parserGeneratorFunc(func(_ context.Context, request ai.Request) (*ai.Result, error) {
				calls++
				return visionTestResult(request, `{
					"schema_version":1,"section_path":[],"warnings":[],
					"elements":[{"kind":"text","title":"","section_path":[],"region":`+region+`}]
				}`), nil
			})
			result, err := NewParser(0, generator).Parse(context.Background(), ParseRequest{
				MIMEType: "image/png", Content: content,
			})
			if result != nil || !errors.Is(err, ErrVisionPageAnalysisFailed) || calls != 1 {
				t.Fatalf("result=%#v err=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestVisionRegionFailureRejectsWholeParse(t *testing.T) {
	content := visionTestImage(t, 100, 100)
	for _, failure := range []string{"provider", "schema", "model_changed", "prompt_changed"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			generator := parserGeneratorFunc(func(_ context.Context, request ai.Request) (*ai.Result, error) {
				calls++
				if request.PromptKey == DocumentPageLayoutPromptKey {
					return visionTestResult(request, `{
						"schema_version":1,"section_path":[],"warnings":[],
						"elements":[
							{"kind":"text","title":"","section_path":[],"region":{"x":0,"y":0,"width":500,"height":1000}},
							{"kind":"text","title":"","section_path":[],"region":{"x":500,"y":0,"width":500,"height":1000}}
						]
					}`), nil
				}
				result := visionTestResult(request, `{
					"schema_version":1,"text":"Recognized.","summary":"","structured_data":{},
					"confidence":0.9,"warnings":[]
				}`)
				if calls == 3 {
					switch failure {
					case "provider":
						return nil, ai.ErrProvider
					case "schema":
						result.JSON = json.RawMessage(`{"schema_version":1,"text":"Incomplete."}`)
					case "model_changed":
						result.Model = "other-model"
					case "prompt_changed":
						result.PromptVersion = 2
					}
				}
				return result, nil
			})
			result, err := NewParser(0, generator).Parse(context.Background(), ParseRequest{
				MIMEType: "image/png", Content: content,
			})
			if result != nil || !errors.Is(err, ErrVisionPageAnalysisFailed) || calls != 3 {
				t.Fatalf("partial parse escaped: result=%#v err=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestPDFLayoutWithoutRegionsPreservesNativeText(t *testing.T) {
	requirePDFTestRuntime(t)
	generator := parserGeneratorFunc(func(_ context.Context, request ai.Request) (*ai.Result, error) {
		if request.PromptKey != DocumentPageLayoutPromptKey {
			t.Fatal("empty layout must not call region recognition")
		}
		return visionTestResult(request, `{"schema_version":1,"section_path":[],"elements":[],"warnings":[]}`), nil
	})
	result, err := NewParser(0, generator).Parse(context.Background(), ParseRequest{
		MIMEType: "application/pdf", Content: minimalPDF("Native text omitted by layout"),
	})
	if err != nil || len(result.Chunks) != 1 ||
		result.Chunks[0].TextContent != "Native text omitted by layout" ||
		len(result.Metadata.Prompts) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestVisionParserFingerprintIncludesBothStageVersions(t *testing.T) {
	content := visionTestImage(t, 100, 100)
	parse := func(regionVersion int) *ParseResult {
		generator := parserGeneratorFunc(func(_ context.Context, request ai.Request) (*ai.Result, error) {
			if request.PromptKey == DocumentPageLayoutPromptKey {
				return visionTestResult(request, `{
					"schema_version":1,"section_path":[],"warnings":[],
					"elements":[{"kind":"text","title":"","section_path":[],"region":{"x":0,"y":0,"width":1000,"height":1000}}]
				}`), nil
			}
			result := visionTestResult(request, `{
				"schema_version":1,"text":"Text.","summary":"","structured_data":{},"confidence":0.9,"warnings":[]
			}`)
			result.PromptVersion = regionVersion
			return result, nil
		})
		result, err := NewParser(0, generator).Parse(context.Background(), ParseRequest{
			MIMEType: "image/png", Content: content,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if parse(1).ParserFingerprint == parse(2).ParserFingerprint {
		t.Fatal("recognition prompt version did not change parser fingerprint")
	}
}
