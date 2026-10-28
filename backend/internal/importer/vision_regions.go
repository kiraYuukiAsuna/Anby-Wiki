package importer

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"strings"
	"unicode"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/evidence"
)

const (
	DocumentPageLayoutPromptKey     = "document-page-layout-v1"
	DocumentRegionAnalysisPromptKey = "document-region-analysis-v1"
	DocumentVisionPromptVersion     = 1
	visionLayoutEdge                = 1600
	visionRegionEdge                = 2000
	maxVisionImagePixels            = int64(40_000_000)
	visionSourceNativeRegion        = "native_text_region"
	visionSourceNativeRemainder     = "native_text_remainder"
	warningNativeRegionUnmatched    = "native_region_unmatched"
)

//go:embed schema/document-page-layout.schema.json
var documentPageLayoutSchemaJSON []byte

//go:embed schema/document-region-analysis.schema.json
var documentRegionAnalysisSchemaJSON []byte

var (
	compiledPageLayoutSchema = compileVisionSchema(documentPageLayoutSchemaJSON)
	compiledRegionSchema     = compileVisionSchema(documentRegionAnalysisSchemaJSON)
)

func DocumentPageLayoutSchemaJSON() json.RawMessage {
	return append(json.RawMessage(nil), documentPageLayoutSchemaJSON...)
}

func DocumentRegionAnalysisSchemaJSON() json.RawMessage {
	return append(json.RawMessage(nil), documentRegionAnalysisSchemaJSON...)
}

func compileVisionSchema(data []byte) *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	compiler := jsonschema.NewCompiler()
	const schemaURL = "https://anby.wiki/prompts/document-vision/output.schema.json"
	if err := compiler.AddResource(schemaURL, doc); err != nil {
		panic(err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		panic(err)
	}
	return schema
}

type pageLayout struct {
	SchemaVersion int           `json:"schema_version"`
	SectionPath   []string      `json:"section_path"`
	Elements      []pageElement `json:"elements"`
	Warnings      []string      `json:"warnings"`
}

type regionAnalysis struct {
	SchemaVersion  int             `json:"schema_version"`
	Text           string          `json:"text"`
	Summary        string          `json:"summary"`
	StructuredData json.RawMessage `json:"structured_data"`
	Confidence     float64         `json:"confidence"`
	Warnings       []string        `json:"warnings"`
}

// Each page owns its images and recognizes regions serially. The PDF adapter's
// existing page worker limit therefore also bounds all model and image work.
func analyzeVisionPage(
	ctx context.Context,
	generator StructuredGenerator,
	request ParseRequest,
	rendered renderedVisionPage,
	nativeText string,
	page, pageCount int,
) (pageAnalysis, error) {
	if generator == nil {
		return pageAnalysis{}, ErrAdvancedParserUnavailable
	}
	img, err := decodeVisionImage(rendered.Data)
	if err != nil {
		return pageAnalysis{}, err
	}
	if img.Bounds().Dx() != int(rendered.Width) || img.Bounds().Dy() != int(rendered.Height) {
		return pageAnalysis{}, ErrParseFailed
	}
	layoutImage, err := encodeVisionImage(img, img.Bounds(), visionLayoutEdge)
	if err != nil {
		return pageAnalysis{}, err
	}
	variables := map[string]any{
		"filename": request.Filename, "page": page, "page_count": pageCount,
		"native_text": trimRunes(nativeText, maxVisionNativeTextRunes),
	}
	modelRequest := ai.Request{
		Provider: request.Provider, Model: request.Model,
		PromptKey: DocumentPageLayoutPromptKey, Variables: variables,
		Images:      []ai.ImageInput{{MediaType: "image/jpeg", Data: layoutImage}},
		ImportJobID: request.ImportJobID, ImportRunID: request.ImportRunID,
	}
	generated, err := generator.Generate(ctx, modelRequest)
	if err != nil {
		return pageAnalysis{}, err
	}
	if !validVisionResult(generated, compiledPageLayoutSchema) ||
		generated.PromptKey != DocumentPageLayoutPromptKey {
		return pageAnalysis{}, ErrVisionPageAnalysisFailed
	}
	var layout pageLayout
	if json.Unmarshal(generated.JSON, &layout) != nil {
		return pageAnalysis{}, ErrVisionPageAnalysisFailed
	}
	// Validate all geometry before spending any requests on recognition.
	for _, element := range layout.Elements {
		r := element.Region
		if r == nil || r.X+r.Width > 1000 || r.Y+r.Height > 1000 {
			return pageAnalysis{}, ErrVisionPageAnalysisFailed
		}
	}
	analysis := pageAnalysis{
		SchemaVersion: 1, Page: page, SectionPath: layout.SectionPath,
		Elements: []pageElement{}, Warnings: layout.Warnings, QualityScore: 1,
		RenderWidth: rendered.Width, RenderHeight: rendered.Height,
		LayoutPromptVersion: generated.PromptVersion,
		Provider:            generated.Provider, Model: generated.Model,
	}
	// Recognition sees only its crop. Supplying the whole native page here
	// could cause text from another column to be copied into this region.
	// Native character correction is performed after recognition by exact
	// matching in groundedPageBlocks instead.
	variables["native_text"] = ""
	for index, element := range layout.Elements {
		if err := ctx.Err(); err != nil {
			return pageAnalysis{}, err
		}
		pixelRegion := normalizedToPixelRegion(element.Region, rendered.Width, rendered.Height)
		crop := image.Rect(
			int(pixelRegion.X), int(pixelRegion.Y),
			int(pixelRegion.X+pixelRegion.Width), int(pixelRegion.Y+pixelRegion.Height),
		)
		data, err := encodeVisionImage(img, crop, visionRegionEdge)
		if err != nil {
			return pageAnalysis{}, err
		}
		variables["kind"], variables["title"], variables["region_index"] = element.Kind, element.Title, index
		modelRequest.PromptKey = DocumentRegionAnalysisPromptKey
		modelRequest.Images = []ai.ImageInput{{MediaType: "image/jpeg", Data: data}}
		generated, err := generator.Generate(ctx, modelRequest)
		if err != nil {
			return pageAnalysis{}, err
		}
		if !validVisionResult(generated, compiledRegionSchema) ||
			generated.PromptKey != DocumentRegionAnalysisPromptKey ||
			generated.Provider != analysis.Provider || generated.Model != analysis.Model ||
			(analysis.PromptVersion > 0 && analysis.PromptVersion != generated.PromptVersion) {
			return pageAnalysis{}, ErrVisionPageAnalysisFailed
		}
		var recognized regionAnalysis
		if json.Unmarshal(generated.JSON, &recognized) != nil || strings.TrimSpace(recognized.Text) == "" {
			return pageAnalysis{}, ErrVisionPageAnalysisFailed
		}
		element.Text = strings.TrimSpace(recognized.Text)
		element.Summary = strings.TrimSpace(recognized.Summary)
		element.StructuredData = recognized.StructuredData
		element.Confidence = recognized.Confidence
		analysis.Elements = append(analysis.Elements, element)
		analysis.PromptVersion = generated.PromptVersion
		analysis.QualityScore = min(analysis.QualityScore, recognized.Confidence)
		analysis.Warnings = append(analysis.Warnings, recognized.Warnings...)
	}
	return analysis, nil
}

func validVisionResult(result *ai.Result, schema *jsonschema.Schema) bool {
	if result == nil || result.PromptVersion <= 0 ||
		strings.TrimSpace(result.Provider) == "" || strings.TrimSpace(result.Model) == "" {
		return false
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(result.JSON))
	return err == nil && schema.Validate(doc) == nil
}

func decodeVisionImage(data []byte) (image.Image, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return nil, ErrParseFailed
	}
	if int64(config.Width) > maxVisionImagePixels/int64(config.Height) {
		return nil, ErrOCRImageTooLarge
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrParseFailed
	}
	return img, nil
}

// Box sampling downsizes only the request image; persisted locators always use
// the original dimensions. Alpha is composited on white for document images.
func encodeVisionImage(img image.Image, crop image.Rectangle, edge int) ([]byte, error) {
	if crop.Empty() || !crop.In(img.Bounds()) || edge <= 0 {
		return nil, ErrParseFailed
	}
	width, height := crop.Dx(), crop.Dy()
	if longest := max(width, height); longest > edge {
		width, height = max(1, width*edge/longest), max(1, height*edge/longest)
	}
	output := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		top, bottom := crop.Min.Y+y*crop.Dy()/height, crop.Min.Y+(y+1)*crop.Dy()/height
		for x := range width {
			left, right := crop.Min.X+x*crop.Dx()/width, crop.Min.X+(x+1)*crop.Dx()/width
			var red, green, blue, count uint64
			for sy := top; sy < bottom; sy++ {
				for sx := left; sx < right; sx++ {
					r, g, b, a := img.At(sx, sy).RGBA()
					red += uint64(r + 65535 - a)
					green += uint64(g + 65535 - a)
					blue += uint64(b + 65535 - a)
					count++
				}
			}
			output.SetRGBA(x, y, color.RGBA{
				R: uint8(red / count / 257), G: uint8(green / count / 257),
				B: uint8(blue / count / 257), A: 255,
			})
		}
	}
	var encoded bytes.Buffer
	if jpeg.Encode(&encoded, output, &jpeg.Options{Quality: 90}) != nil {
		return nil, ErrParseFailed
	}
	if encoded.Len() > maxVisionPageImageBytes {
		return nil, ErrOCRImageTooLarge
	}
	return encoded.Bytes(), nil
}

type visionImageAdapter struct {
	generator StructuredGenerator
}

func (*visionImageAdapter) Name() string    { return "anby-vision-image" }
func (*visionImageAdapter) Version() string { return "1" }
func (*visionImageAdapter) Supports(mimeType string) bool {
	return mimeType == "image/png" || mimeType == "image/jpeg"
}

func (a *visionImageAdapter) Parse(ctx context.Context, request ParseRequest) (*adapterResult, error) {
	if a.generator == nil {
		return nil, ErrAdvancedParserUnavailable
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(request.Content))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return nil, ErrParseFailed
	}
	if int64(config.Width) > maxVisionImagePixels/int64(config.Height) {
		return nil, ErrOCRImageTooLarge
	}
	analysis, err := analyzeVisionPage(ctx, a.generator, request, renderedVisionPage{
		Data: request.Content, Width: int32(config.Width), Height: int32(config.Height),
	}, "", 1, 1)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrVisionPageAnalysisFailed
	}
	return &adapterResult{
		Track: evidence.ParseTrackVision,
		Metadata: ParseMetadata{
			PageCount: 1, Provider: analysis.Provider, Model: analysis.Model,
			PromptKey: DocumentPageLayoutPromptKey, PromptVersion: analysis.LayoutPromptVersion,
			Prompts: []ParsePrompt{
				{Key: DocumentPageLayoutPromptKey, Version: analysis.LayoutPromptVersion},
				{Key: DocumentRegionAnalysisPromptKey, Version: analysis.PromptVersion},
			},
			Warnings: analysis.Warnings,
		},
		Blocks: pageAnalysisBlocks(analysis),
	}, nil
}

// Native prose is never replaced by an unverified visual transcription.
// Unique matching regions supply reading order; all remaining native text is
// retained separately, including text missed by layout detection.
func groundedPageBlocks(analysis *pageAnalysis, nativeText string) []TextBlock {
	visual := pageAnalysisBlocks(*analysis)
	if nativeText == "" {
		return visual
	}
	nativeRunes := []rune(nativeText)
	normalized, spans := collapseWhitespaceWithSpans(nativeRunes)
	covered := make([]bool, len(nativeRunes))
	blocks := make([]TextBlock, 0, len(visual)+1)
	for index, block := range visual {
		if block.Kind != evidence.SourceChunkTypeText {
			blocks = append(blocks, block)
			continue
		}
		quoted, _ := collapseWhitespaceWithSpans([]rune(block.Text))
		match := -1
		for start := 0; start+len(quoted) <= len(normalized); start++ {
			if equalRunesAt(normalized, quoted, start) {
				if match >= 0 {
					match = -1
					break
				}
				match = start
			}
		}
		if match < 0 || len(quoted) == 0 {
			analysis.Warnings = append(analysis.Warnings, warningNativeRegionUnmatched)
			continue
		}
		start, end := spans[match].start, spans[match+len(quoted)-1].end
		overlap := false
		for _, used := range covered[start:end] {
			overlap = overlap || used
		}
		if overlap {
			analysis.Warnings = append(analysis.Warnings, warningNativeRegionUnmatched)
			continue
		}
		for offset := start; offset < end; offset++ {
			covered[offset] = true
		}
		block.Text = string(nativeRunes[start:end])
		block.Content, _ = json.Marshal(map[string]any{
			"source": visionSourceNativeRegion, "reading_order": index, "title": block.Title,
			"native_char_start": start, "native_char_end": end,
		})
		blocks = append(blocks, block)
	}
	page := int32(analysis.Page)
	for start := 0; start < len(nativeRunes); {
		if covered[start] {
			start++
			continue
		}
		end := start + 1
		for end < len(nativeRunes) && !covered[end] {
			end++
		}
		textStart, textEnd := start, end
		for textStart < textEnd && unicode.IsSpace(nativeRunes[textStart]) {
			textStart++
		}
		for textEnd > textStart && unicode.IsSpace(nativeRunes[textEnd-1]) {
			textEnd--
		}
		if textStart < textEnd {
			content, _ := json.Marshal(map[string]any{
				"source":            visionSourceNativeRemainder,
				"native_char_start": textStart, "native_char_end": textEnd,
			})
			blocks = append(blocks, TextBlock{
				Kind: evidence.SourceChunkTypePage, Text: string(nativeRunes[textStart:textEnd]),
				Page: &page, PageEnd: &page, SectionPath: analysis.SectionPath, Content: content,
			})
		}
		start = end
	}
	return blocks
}
