package importer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/evidence"
	"golang.org/x/net/html"
)

var (
	ErrParseFailed              = errors.New("importer: 来源解析失败")
	ErrPDFExtractorUnavailable  = fmt.Errorf("%w: PDF 文本提取器不可用", ErrParseFailed)
	ErrPDFTextTooLarge          = fmt.Errorf("%w: PDF 解压后的文本过大", ErrParseFailed)
	ErrPDFNoExtractableText     = fmt.Errorf("%w: PDF 没有可提取的文本层，可能需要 OCR", ErrParseFailed)
	ErrPDFRasterizerUnavailable = fmt.Errorf("%w: PDF OCR 栅格化组件不可用", ErrParseFailed)
	ErrPDFPageLimitExceeded     = fmt.Errorf("%w: PDF OCR 页数超过上限", ErrParseFailed)
	ErrOCRUnavailable           = fmt.Errorf("%w: OCR 组件不可用", ErrParseFailed)
	ErrOCRImageTooLarge         = fmt.Errorf("%w: OCR 图片像素超过上限", ErrParseFailed)
	ErrOCRTextTooLarge          = fmt.Errorf("%w: OCR 输出超过上限", ErrParseFailed)
	ErrOCRNoText                = fmt.Errorf("%w: OCR 未识别到文本", ErrParseFailed)
	ErrOCRFailed                = fmt.Errorf("%w: OCR 执行失败", ErrParseFailed)
)

const (
	maxPDFTextBytes              = 32 << 20
	DefaultSourceChunkCharacters = 32000
)

type TextBlock struct {
	Text        string
	Page        *int32
	Section     *string
	ImageRegion *evidence.ImageRegion
	OCR         *evidence.OCRInfo
}

type Parser struct{ MaxChunkRunes int }

func NewParser(maxChunkRunes int) *Parser {
	if maxChunkRunes <= 0 {
		maxChunkRunes = DefaultSourceChunkCharacters
	}
	return &Parser{MaxChunkRunes: maxChunkRunes}
}

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
	case "application/json":
		blocks, err = parseJSON(content)
	case "text/csv":
		blocks, err = parseCSV(content)
	case "image/png", "image/jpeg":
		blocks, err = parseImageOCR(ctx, content, nil)
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
	document, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, ErrParseFailed
	}
	root := firstHTMLElement(document, "main")
	if root == nil {
		root = firstHTMLElement(document, "body")
	}
	if root == nil {
		return nil, ErrParseFailed
	}
	blocks := make([]TextBlock, 0)
	var section *string
	collectHTMLBlocks(root, &section, &blocks)
	if len(blocks) == 0 {
		if text := htmlNodeTextWithoutHeadings(root); text != "" {
			blocks = append(blocks, TextBlock{Text: text})
		}
	}
	if len(blocks) == 0 {
		return nil, ErrParseFailed
	}
	return blocks, nil
}

func firstHTMLElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := firstHTMLElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func collectHTMLBlocks(node *html.Node, section **string, blocks *[]TextBlock) {
	if node.Type == html.ElementNode {
		if ignoredHTMLElement(node) {
			return
		}
		switch node.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			if value := htmlNodeText(node); value != "" {
				*section = &value
			}
			return
		case "p", "pre", "blockquote", "li", "dt", "dd", "figcaption", "td", "th":
			if value := htmlNodeText(node); value != "" {
				*blocks = append(*blocks, TextBlock{Text: value, Section: *section})
			}
			return
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectHTMLBlocks(child, section, blocks)
	}
}

func ignoredHTMLElement(node *html.Node) bool {
	switch node.Data {
	case "script", "style", "noscript", "template", "svg", "nav", "aside", "header", "footer",
		"form", "button":
		return true
	}
	for _, attribute := range node.Attr {
		key := strings.ToLower(attribute.Key)
		value := strings.ToLower(strings.TrimSpace(attribute.Val))
		if key == "hidden" || key == "aria-hidden" && value == "true" ||
			key == "role" && (value == "navigation" || value == "contentinfo") {
			return true
		}
		if key == "class" {
			for _, token := range strings.Fields(value) {
				if strings.Contains(token, "sidebar") || strings.Contains(token, "breadcrumb") ||
					token == "toc" || strings.HasSuffix(token, "-toc") || strings.Contains(token, "pagination") {
					return true
				}
			}
		}
	}
	return false
}

func htmlNodeText(node *html.Node) string {
	return collectHTMLNodeText(node, false)
}

func htmlNodeTextWithoutHeadings(node *html.Node) string {
	return collectHTMLNodeText(node, true)
}

func collectHTMLNodeText(node *html.Node, skipHeadings bool) string {
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.ElementNode && ignoredHTMLElement(current) {
			return
		}
		if skipHeadings && current.Type == html.ElementNode && len(current.Data) == 2 &&
			current.Data[0] == 'h' && current.Data[1] >= '1' && current.Data[1] <= '6' {
			return
		}
		if current.Type == html.TextNode {
			if text := strings.TrimSpace(current.Data); text != "" {
				if builder.Len() > 0 && !startsWithClosingPunctuation(text) {
					builder.WriteByte(' ')
				}
				builder.WriteString(text)
			}
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func startsWithClosingPunctuation(value string) bool {
	for _, character := range value {
		switch character {
		case '.', ',', ';', ':', '!', '?', ')', ']', '}', '。', '，', '；', '：', '！', '？', '）', '】', '》':
			return true
		default:
			return false
		}
	}
	return false
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
		return parsePDFOCR(ctx, content)
	}
	return blocks, nil
}

func (p *Parser) chunk(blocks []TextBlock) []evidence.ChunkInput {
	result := []evidence.ChunkInput{}
	for _, block := range blocks {
		runes := []rune(block.Text)
		for start := 0; start < len(runes); {
			end := semanticChunkEnd(runes, start, p.MaxChunkRunes)
			charStart, charEnd := int32(start), int32(end)
			locator := evidence.Locator{Page: block.Page, Section: block.Section,
				CharStart: &charStart, CharEnd: &charEnd,
				ImageRegion: block.ImageRegion, OCR: block.OCR}
			result = append(result, evidence.ChunkInput{Ordinal: len(result), Locator: locator,
				TextContent: string(runes[start:end])})
			start = end
		}
	}
	return result
}

// semanticChunkEnd keeps the configured size as a hard upper bound while
// preferring a nearby paragraph, line, sentence, or word boundary. This keeps
// persisted evidence stable and avoids cutting a quotation in the middle of a
// sentence merely because its last rune happened to land on the size limit.
func semanticChunkEnd(runes []rune, start, limit int) int {
	end := min(start+limit, len(runes))
	if end == len(runes) || limit <= 0 {
		return end
	}
	minimum := start + limit*2/3
	for index := end - 1; index >= minimum; index-- {
		if index > start && runes[index-1] == '\n' && runes[index] == '\n' {
			return index + 1
		}
	}
	for index := end - 1; index >= minimum; index-- {
		if runes[index] == '\n' {
			return index + 1
		}
	}
	for index := end - 1; index >= minimum; index-- {
		switch runes[index] {
		case '.', '!', '?', '\u3002', '\uff01', '\uff1f', ';', '\uff1b':
			return index + 1
		}
	}
	for index := end - 1; index >= minimum; index-- {
		if runes[index] == ' ' || runes[index] == '\t' {
			return index + 1
		}
	}
	return end
}
