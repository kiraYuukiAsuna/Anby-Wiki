package importer

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/evidence"
	"golang.org/x/net/html"
)

var (
	ErrParseFailed               = errors.New("importer: 来源解析失败")
	ErrPDFExtractorUnavailable   = fmt.Errorf("%w: PDF 文本提取器不可用", ErrParseFailed)
	ErrPDFTextTooLarge           = fmt.Errorf("%w: PDF 解压后的文本过大", ErrParseFailed)
	ErrPDFRasterizerUnavailable  = fmt.Errorf("%w: PDF 页面渲染组件不可用", ErrParseFailed)
	ErrPDFPageLimitExceeded      = fmt.Errorf("%w: PDF 页数超过解析上限", ErrParseFailed)
	ErrOCRUnavailable            = fmt.Errorf("%w: OCR 组件不可用", ErrParseFailed)
	ErrOCRImageTooLarge          = fmt.Errorf("%w: OCR 图片像素超过上限", ErrParseFailed)
	ErrOCRTextTooLarge           = fmt.Errorf("%w: OCR 输出超过上限", ErrParseFailed)
	ErrOCRNoText                 = fmt.Errorf("%w: OCR 未识别到文本", ErrParseFailed)
	ErrOCRFailed                 = fmt.Errorf("%w: OCR 执行失败", ErrParseFailed)
	ErrAdvancedParserUnavailable = fmt.Errorf("%w: 高级文档解析器不可用", ErrParseFailed)
	ErrVisionPageAnalysisFailed  = fmt.Errorf("%w: 视觉页面分析失败", ErrParseFailed)
)

const (
	maxPDFTextBytes              = 32 << 20
	DefaultSourceChunkCharacters = 32000
)

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
	sectionPath := []string{}
	collectHTMLBlocks(root, &sectionPath, &blocks)
	if len(blocks) == 0 {
		if text := htmlNodeTextWithoutHeadings(root); text != "" {
			blocks = append(blocks, TextBlock{Kind: "text", Text: text})
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

func collectHTMLBlocks(node *html.Node, sectionPath *[]string, blocks *[]TextBlock) {
	if node.Type == html.ElementNode {
		if ignoredHTMLElement(node) {
			return
		}
		switch node.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			if value := htmlNodeText(node); value != "" {
				level := int(node.Data[1] - '0')
				parentCount := min(level-1, len(*sectionPath))
				*sectionPath = append(
					append([]string(nil), (*sectionPath)[:parentCount]...),
					value,
				)
				if level > 1 {
					appendHTMLHintBlock(blocks, *sectionPath, value, sourceBlockHint{
						BlockType: "heading", Text: value, Level: level,
					})
				}
			}
			return
		case "ul", "ol":
			items := htmlListItems(node)
			if len(items) > 0 {
				blockType := "bullet_list"
				if node.Data == "ol" {
					blockType = "ordered_list"
				}
				appendHTMLHintBlock(
					blocks, *sectionPath, strings.Join(items, "\n"),
					sourceBlockHint{BlockType: blockType, Items: items},
				)
			}
			return
		case "table":
			rows := htmlTableRows(node)
			if len(rows) > 0 {
				lines := make([]string, len(rows))
				for index := range rows {
					lines[index] = strings.Join(rows[index], "\t")
				}
				appendHTMLHintBlock(
					blocks, *sectionPath, strings.Join(lines, "\n"),
					sourceBlockHint{
						BlockType: "table", Rows: rows,
						Caption:         htmlNodeText(firstHTMLDescendant(node, "caption")),
						HasColumnHeader: firstHTMLDescendant(node, "th") != nil,
					},
				)
			}
			return
		case "details":
			summary := htmlNodeText(firstHTMLDescendant(node, "summary"))
			body := htmlDetailsBody(node)
			if summary != "" && body != "" {
				appendHTMLHintBlock(
					blocks, *sectionPath, summary+"\n"+body,
					sourceBlockHint{
						BlockType: "toggle", Text: summary,
						Children: []sourceBlockHint{{
							BlockType: "paragraph", Text: body,
						}},
					},
				)
			}
			return
		case "aside":
			if value := htmlNodeText(node); value != "" {
				kind, color := htmlCalloutKind(node)
				appendHTMLHintBlock(blocks, *sectionPath, value, sourceBlockHint{
					BlockType: "callout", Text: value, Kind: kind, Color: color,
				})
			}
			return
		case "pre":
			if value := strings.Trim(htmlNodeRawText(node), "\r\n"); value != "" {
				if code := firstHTMLDescendant(node, "code"); code != nil {
					appendHTMLHintBlock(blocks, *sectionPath, value, sourceBlockHint{
						BlockType: "code", Text: value, Language: htmlCodeLanguage(code),
					})
				} else {
					appendHTMLHintBlock(blocks, *sectionPath, value, sourceBlockHint{
						BlockType: "preformatted", Text: value,
						Kind: sourcePreformattedKind(value),
					})
				}
			}
			return
		case "blockquote":
			if value := htmlNodeText(node); value != "" {
				appendHTMLHintBlock(blocks, *sectionPath, value, sourceBlockHint{
					BlockType: "quote", Text: value,
				})
			}
			return
		case "p":
			if value := htmlNodeText(node); value != "" {
				if checkbox := htmlCheckbox(node); checkbox != nil {
					checked := htmlHasAttribute(checkbox, "checked")
					appendHTMLHintBlock(blocks, *sectionPath, value, sourceBlockHint{
						BlockType: "todo", Text: value, Checked: &checked,
					})
				} else if expression, ok := standaloneHTMLEquation(node, value); ok {
					appendHTMLHintBlock(blocks, *sectionPath, expression, sourceBlockHint{
						BlockType: "equation", Expression: expression,
					})
				} else {
					*blocks = append(*blocks, TextBlock{
						Kind: evidence.SourceChunkTypeText, Text: value,
						SectionPath: append([]string(nil), (*sectionPath)...),
					})
				}
			}
			return
		case "dt", "dd", "figcaption":
			if value := htmlNodeText(node); value != "" {
				*blocks = append(*blocks, TextBlock{
					Kind: evidence.SourceChunkTypeText, Text: value,
					SectionPath: append([]string(nil), (*sectionPath)...),
				})
			}
			return
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectHTMLBlocks(child, sectionPath, blocks)
	}
}

func ignoredHTMLElement(node *html.Node) bool {
	switch node.Data {
	case "script", "style", "noscript", "template", "svg", "nav", "header", "footer",
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
	if node == nil {
		return ""
	}
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

func appendHTMLHintBlock(
	blocks *[]TextBlock,
	sectionPath []string,
	text string,
	hint sourceBlockHint,
) {
	kind := evidence.SourceChunkTypeText
	if hint.BlockType == "table" {
		kind = evidence.SourceChunkTypeTable
	}
	*blocks = append(*blocks, TextBlock{
		Kind: kind, Text: text,
		SectionPath: append([]string(nil), sectionPath...),
		Content:     sourceBlockHintJSON(hint),
	})
}

func firstHTMLDescendant(node *html.Node, name string) *html.Node {
	if node == nil {
		return nil
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == name {
			return child
		}
		if found := firstHTMLDescendant(child, name); found != nil {
			return found
		}
	}
	return nil
}

func htmlNodeRawText(node *html.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.ElementNode && ignoredHTMLElement(current) {
			return
		}
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return builder.String()
}

func htmlListItems(node *html.Node) []string {
	result := []string{}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Data != "li" {
			continue
		}
		if value := htmlNodeText(child); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func htmlTableRows(node *html.Node) [][]string {
	rows := [][]string{}
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.ElementNode && current.Data == "tr" {
			row := []string{}
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode &&
					(child.Data == "th" || child.Data == "td") {
					row = append(row, htmlNodeText(child))
				}
			}
			if len(row) > 0 {
				rows = append(rows, row)
			}
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return rows
}

func htmlDetailsBody(node *html.Node) string {
	parts := []string{}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "summary" {
			continue
		}
		if value := htmlNodeText(child); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " ")
}

func htmlCalloutKind(node *html.Node) (string, string) {
	value := strings.ToLower(htmlAttribute(node, "class") + " " + htmlAttribute(node, "role"))
	switch {
	case strings.Contains(value, "danger"), strings.Contains(value, "error"):
		return "danger", "red"
	case strings.Contains(value, "warning"), strings.Contains(value, "alert"):
		return "warning", "yellow"
	default:
		return "info", "blue"
	}
}

func htmlCodeLanguage(node *html.Node) string {
	for _, token := range strings.Fields(htmlAttribute(node, "class")) {
		if strings.HasPrefix(token, "language-") {
			return strings.TrimPrefix(token, "language-")
		}
	}
	return ""
}

func htmlCheckbox(node *html.Node) *html.Node {
	var find func(*html.Node) *html.Node
	find = func(current *html.Node) *html.Node {
		if current.Type == html.ElementNode && current.Data == "input" &&
			strings.EqualFold(htmlAttribute(current, "type"), "checkbox") {
			return current
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if found := find(child); found != nil {
				return found
			}
		}
		return nil
	}
	return find(node)
}

func standaloneHTMLEquation(node *html.Node, text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "$$") && strings.HasSuffix(trimmed, "$$") &&
		len(trimmed) > 4 {
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "$$"), "$$")), true
	}
	if firstHTMLDescendant(node, "math") != nil {
		return trimmed, trimmed != ""
	}
	return "", false
}

func htmlAttribute(node *html.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return attribute.Val
		}
	}
	return ""
}

func htmlHasAttribute(node *html.Node, name string) bool {
	if node == nil {
		return false
	}
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return true
		}
	}
	return false
}
