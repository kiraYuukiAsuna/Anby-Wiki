// Package render 把 Typed Block AST v1 渲染为安全 HTML 片段（M1-T06）。
//
// 输出是不含 <html>/<head> 的片段，供阅读 API 随 PageWithContent 返回；
// 本包是唯一的渲染实现：M3-T05 的 RenderedPage 投影直接复用本包生成缓存 HTML，
// 并按 RendererVersion 记录版本（阅读路径对版本不匹配的投影行实时渲染兜底）。
//
// 安全规则（详见 README.md）：
//   - 所有文本节点与属性值一律经 html.EscapeString 转义；
//   - external_link 仅允许 http/https URL，其余 scheme（javascript:/data: 等）
//     降级为纯文本渲染，不产生 <a>；
//   - 渲染结果不含任何脚本可执行面（无 <script>、无事件属性、无危险 URL）。
package render

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/anby/wiki/backend/internal/ast"
)

// RendererVersion 渲染器版本。渲染规则发生任何影响输出的变更时必须升版
// （M3 的 RenderedPage 投影按此版本判断缓存是否需要重建）。变更规则见 README。
const RendererVersion = "v4"

type renderContext struct {
	citationNumbers map[string]int
	ctx             context.Context
	components      ComponentRenderer
}

// ComponentRenderer 由装配层实现，负责校验冻结组件版本并解析 Entity/Claim。
type ComponentRenderer interface {
	RenderComponent(context.Context, *ast.Block) (string, error)
}

func newRenderContext(ctx context.Context, components ComponentRenderer) *renderContext {
	return &renderContext{
		citationNumbers: make(map[string]int),
		ctx:             ctx,
		components:      components,
	}
}

// citationNumber 按 citation_id 首次出现顺序编号；同一 Citation 重复引用复用序号。
func (c *renderContext) citationNumber(citationID string) int {
	if n, ok := c.citationNumbers[citationID]; ok {
		return n
	}
	n := len(c.citationNumbers) + 1
	c.citationNumbers[citationID] = n
	return n
}

// RenderHTML 把 AST v1 文档渲染为安全 HTML 片段。
// doc 应已通过 ast.Validate（阅读路径上来自 ContentSnapshot，入库时已校验）；
// 本函数仍对未知 Block/Inline 类型与无法解码的 content 返回错误，不做静默降级。
func RenderHTML(doc *ast.Document) (string, error) {
	return RenderHTMLWithComponents(context.Background(), doc, nil)
}

// RenderHTMLWithComponents 渲染动态 ComponentBlock；resolver 缺失时输出安全占位，
// 保证阅读路径的受控降级不执行不可信模板或返回陈旧 Claim 值。
func RenderHTMLWithComponents(
	ctx context.Context, doc *ast.Document, components ComponentRenderer,
) (string, error) {
	if doc == nil {
		return "", fmt.Errorf("render: doc 为 nil")
	}
	var b strings.Builder
	if err := renderBlocks(&b, doc.Children, newRenderContext(ctx, components)); err != nil {
		return "", err
	}
	return b.String(), nil
}

func renderBlocks(b *strings.Builder, blocks []*ast.Block, ctx *renderContext) error {
	for _, blk := range blocks {
		if err := renderBlock(b, blk, ctx); err != nil {
			return err
		}
	}
	return nil
}

func renderBlock(b *strings.Builder, blk *ast.Block, ctx *renderContext) error {
	switch blk.Type {
	case ast.BlockHeading:
		level := blk.Level
		if level < 1 || level > 6 {
			return fmt.Errorf("render: heading Block %s level=%d 越界", blk.ID, level)
		}
		tag := fmt.Sprintf("h%d", level)
		fmt.Fprintf(b, `<%s id="%s">`, tag, html.EscapeString(blk.ID))
		if err := renderInlines(b, blk, ctx); err != nil {
			return err
		}
		fmt.Fprintf(b, `</%s>`, tag)
		return nil

	case ast.BlockParagraph:
		b.WriteString("<p>")
		if err := renderInlines(b, blk, ctx); err != nil {
			return err
		}
		b.WriteString("</p>")
		return nil

	case ast.BlockBulletList:
		return renderContainer(b, blk, "ul", ctx)
	case ast.BlockOrderedList:
		return renderContainer(b, blk, "ol", ctx)
	case ast.BlockListItem:
		return renderContainer(b, blk, "li", ctx)
	case ast.BlockTableRow:
		return renderContainer(b, blk, "tr", ctx)
	case ast.BlockTableCell:
		return renderContainer(b, blk, "td", ctx)
	case ast.BlockQuote:
		return renderContainer(b, blk, "blockquote", ctx)

	case ast.BlockTable:
		b.WriteString("<table><tbody>")
		if err := renderBlocks(b, blk.Children, ctx); err != nil {
			return err
		}
		b.WriteString("</tbody></table>")
		return nil

	case ast.BlockCode:
		code, err := blk.TextContent()
		if err != nil {
			return err
		}
		b.WriteString("<pre><code")
		if blk.Language != "" {
			fmt.Fprintf(b, ` class="language-%s"`, html.EscapeString(blk.Language))
		}
		b.WriteString(">")
		b.WriteString(html.EscapeString(code))
		b.WriteString("</code></pre>")
		return nil

	case ast.BlockCallout:
		kind := string(blk.Kind)
		fmt.Fprintf(b, `<div class="callout" data-callout="%s">`, html.EscapeString(kind))
		fmt.Fprintf(b, `<div class="callout-title">%s</div>`, html.EscapeString(kind))
		if err := renderBlocks(b, blk.Children, ctx); err != nil {
			return err
		}
		b.WriteString("</div>")
		return nil

	case ast.BlockComponent:
		if ctx.components == nil {
			fmt.Fprintf(b,
				`<aside class="component-unavailable" data-component-id="%s" data-entity-id="%s"></aside>`,
				html.EscapeString(blk.ComponentID), html.EscapeString(blk.EntityID))
			return nil
		}
		rendered, err := ctx.components.RenderComponent(ctx.ctx, blk)
		if err != nil {
			return fmt.Errorf("render: ComponentBlock %s 渲染失败: %w", blk.ID, err)
		}
		b.WriteString(rendered)
		return nil

	case ast.BlockDivider:
		b.WriteString("<hr>")
		return nil

	default:
		return fmt.Errorf("render: 未知 Block 类型 %q（Block %s）", blk.Type, blk.ID)
	}
}

// renderContainer 渲染「开标签 + children Blocks + 闭标签」的简单容器。
func renderContainer(b *strings.Builder, blk *ast.Block, tag string, ctx *renderContext) error {
	fmt.Fprintf(b, "<%s>", tag)
	if err := renderBlocks(b, blk.Children, ctx); err != nil {
		return err
	}
	fmt.Fprintf(b, "</%s>", tag)
	return nil
}

func renderInlines(b *strings.Builder, blk *ast.Block, ctx *renderContext) error {
	nodes, err := blk.InlineContent()
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if err := renderInline(b, n, ctx); err != nil {
			return err
		}
	}
	return nil
}

func renderInline(b *strings.Builder, n *ast.InlineNode, ctx *renderContext) error {
	switch n.Type {
	case ast.InlineText:
		text := html.EscapeString(n.Text)
		// marks 按数组顺序由外向内叠加包裹（可叠加）。
		for i := len(n.Marks) - 1; i >= 0; i-- {
			tag, ok := markTags[n.Marks[i]]
			if !ok {
				return fmt.Errorf("render: 未知 mark %q", n.Marks[i])
			}
			text = "<" + tag + ">" + text + "</" + tag + ">"
		}
		b.WriteString(text)
		return nil

	case ast.InlineCode:
		b.WriteString("<code>")
		b.WriteString(html.EscapeString(n.Text))
		b.WriteString("</code>")
		return nil

	case ast.InlinePageReference:
		if n.ResolutionStatus == ast.ResolutionUnresolved {
			// 未解析引用不可跳转：无 href，以 data-* 保留解析线索供前端样式化。
			fmt.Fprintf(b, `<a class="unresolved" data-unresolved-ref data-target-namespace="%s" data-target-title="%s">%s</a>`,
				html.EscapeString(n.TargetNamespace),
				html.EscapeString(n.NormalizedTitle),
				html.EscapeString(n.NormalizedTitle),
			)
			return nil
		}
		href := "/pages/" + n.TargetPageID
		if n.TargetHeadingBlockID != "" {
			href += "#" + n.TargetHeadingBlockID
		}
		fmt.Fprintf(b, `<a href="%s" data-page-ref>%s</a>`,
			html.EscapeString(href), html.EscapeString(n.DisplayText))
		return nil

	case ast.InlineExternalLink:
		if !isSafeExternalURL(n.URL) {
			// 非 http/https（javascript:/data: 等）：降级为纯文本，不产生 <a>。
			b.WriteString(html.EscapeString(n.DisplayText))
			return nil
		}
		fmt.Fprintf(b, `<a href="%s" rel="noopener noreferrer nofollow" target="_blank">%s</a>`,
			html.EscapeString(n.URL), html.EscapeString(n.DisplayText))
		return nil

	case ast.InlineEntityReference:
		fmt.Fprintf(b, `<a href="/entities/%s" data-entity-ref>%s</a>`,
			html.EscapeString(n.EntityID), html.EscapeString(n.DisplayText))
		return nil

	case ast.InlineClaimReference:
		fmt.Fprintf(b, `<a href="/claims/%s" data-claim-ref="%s">%s</a>`,
			html.EscapeString(n.ClaimID), html.EscapeString(n.ClaimID), html.EscapeString(n.DisplayText))
		return nil

	case ast.InlineCitationReference:
		title := n.DisplayText
		if title == "" {
			title = n.CitationID
		}
		fmt.Fprintf(b, `<sup data-citation-ref="%s" title="%s"><a href="/citations/%s">[%d]</a></sup>`,
			html.EscapeString(n.CitationID), html.EscapeString(title), html.EscapeString(n.CitationID), ctx.citationNumber(n.CitationID))
		return nil

	default:
		return fmt.Errorf("render: 未知 Inline 类型 %q", n.Type)
	}
}

// markTags mark → HTML 标签映射。
var markTags = map[ast.Mark]string{
	ast.MarkBold:          "strong",
	ast.MarkItalic:        "em",
	ast.MarkStrikethrough: "del",
	ast.MarkCode:          "code",
}

// isSafeExternalURL 判定外链 URL 是否允许渲染为超链接：
// 仅 http/https（scheme 大小写不敏感）；空 scheme、相对地址与其他 scheme 一律拒绝。
func isSafeExternalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}
