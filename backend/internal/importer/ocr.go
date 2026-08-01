package importer

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/evidence"
)

const (
	maxOCRPixels            = int64(40_000_000)
	maxOCRTextBytes         = int64(16 << 20)
	maxPDFInfoBytes         = int64(1 << 20)
	maxPDFOCRPages          = 20
	maxRasterizedImageBytes = int64(24 << 20)
	maxRasterizedPageEdge   = 2400
	tesseractOCRLanguages   = "chi_sim+eng"
	tesseractOCREngine      = "tesseract"
)

var pdfPagesPattern = regexp.MustCompile(`(?m)^Pages:\s+([0-9]+)\s*$`)

func parseImageOCR(
	ctx context.Context,
	content []byte,
	page *int32,
) ([]TextBlock, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return nil, ErrParseFailed
	}
	if int64(config.Width) > maxOCRPixels/int64(config.Height) {
		return nil, ErrOCRImageTooLarge
	}
	tesseractPath, err := exec.LookPath("tesseract")
	if err != nil {
		return nil, ErrOCRUnavailable
	}
	command := exec.CommandContext(
		ctx,
		tesseractPath,
		"stdin",
		"stdout",
		"-l",
		tesseractOCRLanguages,
		"--psm",
		"3",
		"tsv",
	)
	// OCR never needs provider or storage credentials.
	command.Env = []string{}
	command.Stdin = bytes.NewReader(content)
	command.Stderr = io.Discard
	output, err := readBoundedCommandOutput(
		ctx, command, maxOCRTextBytes, ErrOCRTextTooLarge,
	)
	if err != nil {
		if errors.Is(err, ErrOCRTextTooLarge) || errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, ErrOCRFailed
	}
	return parseTesseractTSV(output, page, int32(config.Width), int32(config.Height))
}

func parsePDFOCR(ctx context.Context, content []byte) ([]TextBlock, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return nil, ErrOCRUnavailable
	}
	pdfInfoPath, err := exec.LookPath("pdfinfo")
	if err != nil {
		return nil, ErrPDFRasterizerUnavailable
	}
	rasterizerPath, err := exec.LookPath("pdftoppm")
	if err != nil {
		return nil, ErrPDFRasterizerUnavailable
	}
	pageCount, err := pdfPageCount(ctx, pdfInfoPath, content)
	if err != nil {
		return nil, err
	}
	if pageCount > maxPDFOCRPages {
		return nil, ErrPDFPageLimitExceeded
	}

	blocks := make([]TextBlock, 0, pageCount)
	for pageNumber := 1; pageNumber <= pageCount; pageNumber++ {
		rendered, renderErr := renderPDFPage(
			ctx, rasterizerPath, content, pageNumber,
		)
		if renderErr != nil {
			return nil, renderErr
		}
		page := int32(pageNumber)
		pageBlocks, ocrErr := parseImageOCR(ctx, rendered, &page)
		if errors.Is(ocrErr, ErrOCRNoText) {
			continue
		}
		if ocrErr != nil {
			return nil, ocrErr
		}
		blocks = append(blocks, pageBlocks...)
	}
	if len(blocks) == 0 {
		return nil, ErrOCRNoText
	}
	return blocks, nil
}

func pdfPageCount(
	ctx context.Context,
	pdfInfoPath string,
	content []byte,
) (int, error) {
	command := exec.CommandContext(ctx, pdfInfoPath, "-")
	command.Env = []string{}
	command.Stdin = bytes.NewReader(content)
	command.Stderr = io.Discard
	output, err := readBoundedCommandOutput(
		ctx, command, maxPDFInfoBytes, ErrParseFailed,
	)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, ErrParseFailed
	}
	match := pdfPagesPattern.FindSubmatch(output)
	if len(match) != 2 {
		return 0, ErrParseFailed
	}
	count, err := strconv.Atoi(string(match[1]))
	if err != nil || count <= 0 {
		return 0, ErrParseFailed
	}
	return count, nil
}

func renderPDFPage(
	ctx context.Context,
	rasterizerPath string,
	content []byte,
	pageNumber int,
) ([]byte, error) {
	tempDirectory, err := os.MkdirTemp("", "anby-wiki-pdf-ocr-*")
	if err != nil {
		return nil, ErrParseFailed
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
		"-scale-to", strconv.Itoa(maxRasterizedPageEdge),
		"-png",
		"-",
		outputRoot,
	)
	command.Env = []string{}
	command.Stdin = bytes.NewReader(content)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrOCRFailed
	}
	outputPath := outputRoot + ".png"
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() <= 0 || info.Size() > maxRasterizedImageBytes {
		return nil, ErrOCRImageTooLarge
	}
	rendered, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, ErrParseFailed
	}
	return rendered, nil
}

func readBoundedCommandOutput(
	ctx context.Context,
	command *exec.Cmd,
	maxBytes int64,
	tooLarge error,
) ([]byte, error) {
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxBytes+1))
	if int64(len(output)) > maxBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, tooLarge
	}
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, readErr
	}
	if err := command.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return output, nil
}

type ocrLine struct {
	words           []string
	x               int32
	y               int32
	right           int32
	bottom          int32
	confidenceTotal float64
	confidenceWords int
}

func parseTesseractTSV(
	content []byte,
	page *int32,
	imageWidth, imageHeight int32,
) ([]TextBlock, error) {
	if !utf8.Valid(content) || imageWidth <= 0 || imageHeight <= 0 {
		return nil, ErrOCRFailed
	}
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	order := make([]string, 0)
	lines := make(map[string]*ocrLine)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) < 12 || record[0] != "5" {
			continue
		}
		text := strings.TrimSpace(record[11])
		if text == "" {
			continue
		}
		x, xErr := strconv.Atoi(record[6])
		y, yErr := strconv.Atoi(record[7])
		width, widthErr := strconv.Atoi(record[8])
		height, heightErr := strconv.Atoi(record[9])
		confidence, confidenceErr := strconv.ParseFloat(record[10], 64)
		if xErr != nil || yErr != nil || widthErr != nil || heightErr != nil ||
			confidenceErr != nil || x < 0 || y < 0 || width <= 0 || height <= 0 {
			continue
		}
		left := min(int32(x), imageWidth-1)
		top := min(int32(y), imageHeight-1)
		right := min(int32(x+width), imageWidth)
		bottom := min(int32(y+height), imageHeight)
		if right <= left || bottom <= top {
			continue
		}
		key := strings.Join(record[1:5], ":")
		line := lines[key]
		if line == nil {
			line = &ocrLine{
				x: left, y: top, right: right, bottom: bottom,
			}
			lines[key] = line
			order = append(order, key)
		} else {
			line.x = min(line.x, left)
			line.y = min(line.y, top)
			line.right = max(line.right, right)
			line.bottom = max(line.bottom, bottom)
		}
		line.words = append(line.words, text)
		if confidence >= 0 {
			line.confidenceTotal += min(confidence, 100)
			line.confidenceWords++
		}
	}

	blocks := make([]TextBlock, 0, len(order))
	for _, key := range order {
		line := lines[key]
		text := joinOCRWords(line.words)
		if text == "" {
			continue
		}
		confidence := 0.0
		if line.confidenceWords > 0 {
			confidence = line.confidenceTotal / float64(line.confidenceWords) / 100
		}
		blocks = append(blocks, TextBlock{
			Text: text,
			Page: page,
			ImageRegion: &evidence.ImageRegion{
				X: line.x, Y: line.y,
				Width: line.right - line.x, Height: line.bottom - line.y,
				ImageWidth: imageWidth, ImageHeight: imageHeight,
				Unit: evidence.ImageRegionUnitPixel,
			},
			OCR: &evidence.OCRInfo{
				Engine:     tesseractOCREngine,
				Languages:  strings.Split(tesseractOCRLanguages, "+"),
				Confidence: &confidence,
			},
		})
	}
	if len(blocks) == 0 {
		return nil, ErrOCRNoText
	}
	return blocks, nil
}

func joinOCRWords(words []string) string {
	var builder strings.Builder
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		if builder.Len() > 0 && needsOCRSpace(builder.String(), word) {
			builder.WriteByte(' ')
		}
		builder.WriteString(word)
	}
	return builder.String()
}

func needsOCRSpace(existing, next string) bool {
	previousRune, _ := utf8.DecodeLastRuneInString(existing)
	nextRune, _ := utf8.DecodeRuneInString(next)
	if unicode.Is(unicode.Han, previousRune) || unicode.Is(unicode.Han, nextRune) {
		return false
	}
	if unicode.IsPunct(previousRune) || unicode.IsPunct(nextRune) {
		return false
	}
	return true
}
