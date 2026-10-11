package importer

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

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
			}
			return
		case "p", "pre", "blockquote", "li", "dt", "dd", "figcaption", "td", "th":
			if value := htmlNodeText(node); value != "" {
				*blocks = append(*blocks, TextBlock{
					Kind: "text", Text: value,
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
