package importer

import (
	"bytes"
	"errors"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/evidence"
)

var ErrParseFailed = errors.New("importer: 来源解析失败")

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
	pdfTextPattern  = regexp.MustCompile(`(?s)\(((?:\\.|[^\\)])*)\)\s*Tj`)
)

func (p *Parser) Parse(mimeType string, content []byte) ([]evidence.ChunkInput, error) {
	var blocks []TextBlock
	var err error
	switch mimeType {
	case "text/html":
		blocks, err = parseHTML(content)
	case "application/pdf":
		blocks, err = parsePDF(content)
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

func parsePDF(content []byte) ([]TextBlock, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(content), []byte("%PDF-")) ||
		bytes.Contains(content, []byte("/Encrypt")) {
		return nil, ErrParseFailed
	}
	pages := regexp.MustCompile(`(?m)^%%Page[^\n]*`).Split(string(content), -1)
	blocks := []TextBlock{}
	pageNumber := int32(0)
	for index, page := range pages {
		if index == 0 && len(pages) > 1 {
			continue
		}
		pageNumber++
		matches := pdfTextPattern.FindAllStringSubmatch(page, -1)
		for _, match := range matches {
			text := strings.NewReplacer(`\(`, `(`, `\)`, `)`, `\\`, `\`).Replace(match[1])
			text = strings.Join(strings.Fields(text), " ")
			if text != "" {
				page := pageNumber
				blocks = append(blocks, TextBlock{Text: text, Page: &page})
			}
		}
	}
	if len(blocks) == 0 {
		return nil, ErrParseFailed
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
