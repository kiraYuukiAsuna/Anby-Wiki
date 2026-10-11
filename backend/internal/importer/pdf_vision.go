package importer

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/evidence"
)

const (
	DocumentPageAnalysisPromptKey = "document-page-analysis-v1"
	visionPDFParserVersion        = "1"
	maxAdvancedPDFPages           = 500
	maxVisionPageImageBytes       = 16 << 20
	maxVisionNativeTextRunes      = 24000
	visionRenderEdge              = 2000
	visionPageWorkers             = 3
)

//go:embed schema/document-page-analysis.schema.json
var documentPageAnalysisSchemaJSON []byte

func DocumentPageAnalysisSchemaJSON() json.RawMessage {
	return append(json.RawMessage(nil), documentPageAnalysisSchemaJSON...)
}

type visionPDFAdapter struct {
	generator StructuredGenerator
}

func (*visionPDFAdapter) Name() string    { return "anby-vision-pdf" }
func (*visionPDFAdapter) Version() string { return visionPDFParserVersion }
func (*visionPDFAdapter) Supports(mimeType string) bool {
	return mimeType == "application/pdf"
}

type pageAnalysis struct {
	SchemaVersion int           `json:"schema_version"`
	Page          int           `json:"page"`
	SectionPath   []string      `json:"section_path"`
	Elements      []pageElement `json:"elements"`
	Warnings      []string      `json:"warnings"`
	QualityScore  float64       `json:"quality_score"`
	RenderWidth   int32         `json:"-"`
	RenderHeight  int32         `json:"-"`
	PromptVersion int           `json:"-"`
	Provider      string        `json:"-"`
	Model         string        `json:"-"`
}

type pageElement struct {
	Kind           string            `json:"kind"`
	Title          string            `json:"title"`
	Text           string            `json:"text"`
	Summary        string            `json:"summary"`
	SectionPath    []string          `json:"section_path"`
	Region         *normalizedRegion `json:"region"`
	StructuredData json.RawMessage   `json:"structured_data"`
	Confidence     float64           `json:"confidence"`
}

type normalizedRegion struct {
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
	Width  int32 `json:"width"`
	Height int32 `json:"height"`
}

type renderedVisionPage struct {
	Data   []byte
	Width  int32
	Height int32
}

func (a *visionPDFAdapter) Parse(
	ctx context.Context,
	request ParseRequest,
) (*adapterResult, error) {
	if a.generator == nil {
		return nil, ErrAdvancedParserUnavailable
	}
	if !bytes.HasPrefix(bytes.TrimSpace(request.Content), []byte("%PDF-")) {
		return nil, ErrParseFailed
	}
	pdfInfoPath, err := exec.LookPath("pdfinfo")
	if err != nil {
		return nil, ErrPDFRasterizerUnavailable
	}
	rasterizerPath, err := exec.LookPath("pdftoppm")
	if err != nil {
		return nil, ErrPDFRasterizerUnavailable
	}
	textPath, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, ErrPDFExtractorUnavailable
	}
	pageCount, err := pdfPageCount(ctx, pdfInfoPath, request.Content)
	if err != nil {
		return nil, err
	}
	if pageCount > maxAdvancedPDFPages {
		return nil, ErrPDFPageLimitExceeded
	}
	nativePages, err := extractNativePDFPages(ctx, textPath, request.Content, pageCount)
	if err != nil {
		return nil, err
	}
	analyses, err := a.analyzePages(
		ctx, request, rasterizerPath, nativePages, pageCount,
	)
	if err != nil {
		return nil, err
	}

	blocks := make([]TextBlock, 0, pageCount*2)
	warnings := make([]string, 0)
	nativeTextPresent := false
	promptVersion := 0
	effectiveProvider := ""
	effectiveModel := ""
	for pageIndex := range analyses {
		pageNumber := pageIndex + 1
		nativeText := strings.TrimSpace(nativePages[pageIndex])
		nativeTextPresent = nativeTextPresent || nativeText != ""
		analysis := analyses[pageIndex]
		if pageIndex == 0 {
			promptVersion = analysis.PromptVersion
			effectiveProvider = analysis.Provider
			effectiveModel = analysis.Model
		} else if promptVersion != analysis.PromptVersion ||
			effectiveProvider != analysis.Provider ||
			effectiveModel != analysis.Model {
			return nil, ErrVisionPageAnalysisFailed
		}
		for _, warning := range analysis.Warnings {
			warnings = append(warnings, fmt.Sprintf("page %d: %s", pageNumber, warning))
		}
		if analysis.QualityScore < 0.6 {
			warnings = append(
				warnings,
				fmt.Sprintf("page %d: visual analysis quality %.2f", pageNumber, analysis.QualityScore),
			)
		}
		pageBlocks := pageAnalysisBlocks(analysis)
		if nativeText != "" {
			page := int32(pageNumber)
			exactTextBlock := TextBlock{
				Kind: evidence.SourceChunkTypePage,
				Text: nativeText,
				Page: &page, PageEnd: &page,
				SectionPath: append([]string(nil), analysis.SectionPath...),
				Content:     json.RawMessage(`{"source":"native_text"}`),
			}
			visualBlocks := make([]TextBlock, 0, len(pageBlocks))
			for index := range pageBlocks {
				if pageBlocks[index].Kind != evidence.SourceChunkTypeText {
					visualBlocks = append(visualBlocks, pageBlocks[index])
				}
			}
			pageBlocks = append([]TextBlock{exactTextBlock}, visualBlocks...)
		}
		blocks = append(blocks, pageBlocks...)
	}
	if len(blocks) == 0 {
		return nil, ErrVisionPageAnalysisFailed
	}
	track := evidence.ParseTrackVision
	if nativeTextPresent {
		track = evidence.ParseTrackHybrid
	}
	return &adapterResult{
		Track: track,
		Metadata: ParseMetadata{
			PageCount:     pageCount,
			Provider:      effectiveProvider,
			Model:         effectiveModel,
			PromptKey:     DocumentPageAnalysisPromptKey,
			PromptVersion: promptVersion,
			Warnings:      warnings,
		},
		Blocks: blocks,
	}, nil
}

func (a *visionPDFAdapter) analyzePages(
	ctx context.Context,
	request ParseRequest,
	rasterizerPath string,
	nativePages []string,
	pageCount int,
) ([]pageAnalysis, error) {
	workerCount := min(visionPageWorkers, pageCount)
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	results := make([]pageAnalysis, pageCount)
	var firstErr error
	var errorOnce sync.Once
	var workers sync.WaitGroup

	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for pageIndex := range jobs {
				if workCtx.Err() != nil {
					return
				}
				rendered, err := renderPDFPageForVision(
					workCtx, rasterizerPath, request.Content, pageIndex+1,
				)
				if err == nil {
					results[pageIndex], err = a.analyzePage(
						workCtx, request, rendered, nativePages[pageIndex],
						pageIndex+1, pageCount,
					)
				}
				if err != nil {
					errorOnce.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
			}
		}()
	}
sendLoop:
	for pageIndex := range pageCount {
		select {
		case jobs <- pageIndex:
		case <-workCtx.Done():
			break sendLoop
		}
	}
	close(jobs)
	workers.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (a *visionPDFAdapter) analyzePage(
	ctx context.Context,
	request ParseRequest,
	rendered renderedVisionPage,
	nativeText string,
	page, pageCount int,
) (pageAnalysis, error) {
	nativeText = trimRunes(nativeText, maxVisionNativeTextRunes)
	generated, err := a.generator.Generate(ctx, ai.Request{
		Provider:  request.Provider,
		Model:     request.Model,
		PromptKey: DocumentPageAnalysisPromptKey,
		Variables: map[string]any{
			"filename":    request.Filename,
			"page":        page,
			"page_count":  pageCount,
			"native_text": nativeText,
		},
		Images: []ai.ImageInput{{
			MediaType: "image/jpeg",
			Data:      rendered.Data,
		}},
		ImportJobID: request.ImportJobID,
		ImportRunID: request.ImportRunID,
	})
	if err != nil {
		if ctx.Err() != nil {
			return pageAnalysis{}, ctx.Err()
		}
		return pageAnalysis{}, fmt.Errorf("%w: page %d: %v", ErrVisionPageAnalysisFailed, page, err)
	}
	var analysis pageAnalysis
	if err := json.Unmarshal(generated.JSON, &analysis); err != nil ||
		analysis.SchemaVersion != 1 || analysis.Page != page ||
		generated.PromptVersion <= 0 ||
		strings.TrimSpace(generated.Provider) == "" ||
		strings.TrimSpace(generated.Model) == "" {
		return pageAnalysis{}, fmt.Errorf("%w: page %d output", ErrVisionPageAnalysisFailed, page)
	}
	for index := range analysis.Elements {
		element := &analysis.Elements[index]
		element.Text = strings.TrimSpace(element.Text)
		element.Title = strings.TrimSpace(element.Title)
		element.Summary = strings.TrimSpace(element.Summary)
		if element.Text == "" || !evidence.IsValidSourceChunkType(element.Kind) ||
			element.Kind == evidence.SourceChunkTypePage {
			return pageAnalysis{}, fmt.Errorf(
				"%w: page %d element %d",
				ErrVisionPageAnalysisFailed, page, index,
			)
		}
		if len(element.StructuredData) == 0 {
			element.StructuredData = json.RawMessage(`{}`)
		}
	}
	analysis.RenderWidth = rendered.Width
	analysis.RenderHeight = rendered.Height
	analysis.PromptVersion = generated.PromptVersion
	analysis.Provider = generated.Provider
	analysis.Model = generated.Model
	return analysis, nil
}

func pageAnalysisBlocks(analysis pageAnalysis) []TextBlock {
	page := int32(analysis.Page)
	blocks := make([]TextBlock, 0, len(analysis.Elements))
	for index := range analysis.Elements {
		element := analysis.Elements[index]
		sectionPath := element.SectionPath
		if len(sectionPath) == 0 {
			sectionPath = analysis.SectionPath
		}
		content, _ := json.Marshal(map[string]any{
			"title":           element.Title,
			"summary":         element.Summary,
			"structured_data": json.RawMessage(element.StructuredData),
			"confidence":      element.Confidence,
			"source":          "vision_page",
		})
		blocks = append(blocks, TextBlock{
			Kind: element.Kind, Title: element.Title, Text: element.Text,
			Page: &page, PageEnd: &page,
			SectionPath: append([]string(nil), sectionPath...),
			ImageRegion: normalizedToPixelRegion(
				element.Region, analysis.RenderWidth, analysis.RenderHeight,
			),
			Content: content,
		})
	}
	return blocks
}

func normalizedToPixelRegion(
	region *normalizedRegion,
	imageWidth, imageHeight int32,
) *evidence.ImageRegion {
	if region == nil || imageWidth <= 0 || imageHeight <= 0 {
		return nil
	}
	x := min(region.X, 999)
	y := min(region.Y, 999)
	width := min(region.Width, 1000-x)
	height := min(region.Height, 1000-y)
	if width <= 0 || height <= 0 {
		return nil
	}
	return &evidence.ImageRegion{
		X:          x * imageWidth / 1000,
		Y:          y * imageHeight / 1000,
		Width:      max(1, width*imageWidth/1000),
		Height:     max(1, height*imageHeight/1000),
		ImageWidth: imageWidth, ImageHeight: imageHeight,
		Unit: evidence.ImageRegionUnitPixel,
	}
}

func extractNativePDFPages(
	ctx context.Context,
	textPath string,
	content []byte,
	pageCount int,
) ([]string, error) {
	command := exec.CommandContext(ctx, textPath, "-enc", "UTF-8", "-eol", "unix", "-", "-")
	command.Env = []string{}
	command.Stdin = bytes.NewReader(content)
	command.Stderr = io.Discard
	output, err := readBoundedCommandOutput(
		ctx, command, maxPDFTextBytes, ErrPDFTextTooLarge,
	)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == ErrPDFTextTooLarge {
			return nil, err
		}
		return nil, ErrParseFailed
	}
	if !utf8.Valid(output) {
		return nil, ErrParseFailed
	}
	rawPages := strings.Split(string(output), "\f")
	pages := make([]string, pageCount)
	for index := 0; index < pageCount && index < len(rawPages); index++ {
		pages[index] = strings.TrimSpace(rawPages[index])
	}
	return pages, nil
}

func renderPDFPageForVision(
	ctx context.Context,
	rasterizerPath string,
	content []byte,
	pageNumber int,
) (renderedVisionPage, error) {
	tempDirectory, err := os.MkdirTemp("", "anby-wiki-pdf-vision-*")
	if err != nil {
		return renderedVisionPage{}, ErrParseFailed
	}
	defer os.RemoveAll(tempDirectory)

	outputRoot := filepath.Join(tempDirectory, "page")
	page := strconv.Itoa(pageNumber)
	command := exec.CommandContext(
		ctx,
		rasterizerPath,
		"-f", page,
		"-l", page,
		"-singlefile",
		"-scale-to", strconv.Itoa(visionRenderEdge),
		"-jpeg",
		"-jpegopt", "quality=85,optimize=y",
		"-",
		outputRoot,
	)
	command.Env = []string{}
	command.Stdin = bytes.NewReader(content)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return renderedVisionPage{}, ctx.Err()
		}
		return renderedVisionPage{}, ErrPDFRasterizerUnavailable
	}
	outputPath := outputRoot + ".jpg"
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() <= 0 || info.Size() > maxVisionPageImageBytes {
		return renderedVisionPage{}, ErrOCRImageTooLarge
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return renderedVisionPage{}, ErrParseFailed
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return renderedVisionPage{}, ErrParseFailed
	}
	return renderedVisionPage{
		Data: data, Width: int32(config.Width), Height: int32(config.Height),
	}, nil
}

func trimRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
