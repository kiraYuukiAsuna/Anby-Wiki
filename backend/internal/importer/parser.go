package importer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/evidence"
)

var (
	ErrParseFailed             = errors.New("importer: 来源解析失败")
	ErrPDFExtractorUnavailable = fmt.Errorf("%w: PDF 文本提取器不可用", ErrParseFailed)
	ErrPDFTextTooLarge         = fmt.Errorf("%w: PDF 解压后的文本过大", ErrParseFailed)
	ErrPDFNoExtractableText    = fmt.Errorf("%w: PDF 没有可提取的文本层，可能需要 OCR", ErrParseFailed)
)

const maxPDFTextBytes = 32 << 20

type TextBlock struct {
	Text    string
	Page    *int32
	Section *string
}

type Parser struct{ MaxChunkRunes int }

func NewParser(maxChunkRunes int) *Parser {
	if maxChunkRunes <= 0 {
		maxChunkRunes = 1200
	}
	return &Parser{MaxChunkRunes: maxChunkRunes}
}

var (
	scriptPattern   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	stylePattern    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptPattern = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	commentPattern  = regexp.MustCompile(`(?s)<!--.*?-->`)
	headingPattern  = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	breakPattern    = regexp.MustCompile(`(?is)</?(p|div|li|tr|br|section|article)[^>]*>`)
	tagPattern      = regexp.MustCompile(`(?s)<[^>]+>`)
)

func (p *Parser) Parse(ctx context.Context, mimeType string, content []byte) ([]evidence.ChunkInput, error) {
	var blocks []TextBlock
	var err error
	switch mimeType {
	case "text/html":
		blocks, err = parseHTML(content)
	case "application/pdf":
		blocks, err = parsePDF(ctx, content)
	case "text/plain":
		blocks = []TextBlock{{Text: string(content)}}
	default:
		return nil, ErrUnsupportedMIME
	}
	if err != nil {
		return nil, err
	}
	return p.chunk(blocks), nil
}

func parseHTML(content []byte) ([]TextBlock, error) {
	if !utf8.Valid(content) {
		return nil, ErrParseFailed
	}
	source := scriptPattern.ReplaceAllString(string(content), " ")
	source = stylePattern.ReplaceAllString(source, " ")
	source = noscriptPattern.ReplaceAllString(source, " ")
	source = commentPattern.ReplaceAllString(source, " ")
	source = headingPattern.ReplaceAllStringFunc(source, func(match string) string {
		inner := headingPattern.FindStringSubmatch(match)
		if len(inner) != 2 {
			return "\n"
		}
		return "\n§§" + strings.TrimSpace(tagPattern.ReplaceAllString(inner[1], " ")) + "\n"
	})
	source = breakPattern.ReplaceAllString(source, "\n")
	source = html.UnescapeString(tagPattern.ReplaceAllString(source, " "))
	lines := strings.Split(source, "\n")
	blocks := []TextBlock{}
	var section *string
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if strings.HasPrefix(line, "§§") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "§§"))
			if value != "" {
				section = &value
			}
			continue
		}
		if line != "" {
			blocks = append(blocks, TextBlock{Text: line, Section: section})
		}
	}
	if len(blocks) == 0 {
		return nil, ErrParseFailed
	}
	return blocks, nil
}

func parsePDF(ctx context.Context, content []byte) ([]TextBlock, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(content), []byte("%PDF-")) {
		return nil, ErrParseFailed
	}
	path, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, ErrPDFExtractorUnavailable
	}
	command := exec.CommandContext(ctx, path, "-enc", "UTF-8", "-eol", "unix", "-", "-")
	// The child only needs stdin/stdout. Do not expose the Worker's provider or
	// storage credentials through inherited environment variables.
	command.Env = []string{}
	command.Stdin = bytes.NewReader(content)
	command.Stderr = io.Discard
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: output pipe", ErrPDFExtractorUnavailable)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("%w: start", ErrPDFExtractorUnavailable)
	}
	extracted, readErr := io.ReadAll(io.LimitReader(stdout, maxPDFTextBytes+1))
	if int64(len(extracted)) > maxPDFTextBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, ErrPDFTextTooLarge
	}
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("%w: read output", ErrParseFailed)
	}
	if err := command.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: pdftotext failed", ErrParseFailed)
	}
	if !utf8.Valid(extracted) {
		return nil, ErrParseFailed
	}

	pages := strings.Split(string(extracted), "\f")
	blocks := make([]TextBlock, 0, len(pages))
	for index, pageText := range pages {
		text := strings.Join(strings.Fields(pageText), " ")
		if text == "" {
			continue
		}
		pageNumber := int32(index + 1)
		blocks = append(blocks, TextBlock{Text: text, Page: &pageNumber})
	}
	if len(blocks) == 0 {
		return nil, ErrPDFNoExtractableText
	}
	return blocks, nil
}

func (p *Parser) chunk(blocks []TextBlock) []evidence.ChunkInput {
	result := []evidence.ChunkInput{}
	for _, block := range blocks {
		runes := []rune(block.Text)
		for start := 0; start < len(runes); start += p.MaxChunkRunes {
			end := min(start+p.MaxChunkRunes, len(runes))
			charStart, charEnd := int32(start), int32(end)
			locator := evidence.Locator{Page: block.Page, Section: block.Section,
				CharStart: &charStart, CharEnd: &charEnd}
			result = append(result, evidence.ChunkInput{Ordinal: len(result), Locator: locator,
				TextContent: string(runes[start:end])})
		}
	}
	return result
}
