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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/anby/wiki/backend/internal/ast"
)

// RendererVersion 渲染器版本。渲染规则发生任何影响输出的变更时必须升版
// （M3 的 RenderedPage 投影按此版本判断缓存是否需要重建）。变更规则见 README。
const RendererVersion = "v13"

type renderContext struct {
	citationNumbers map[string]int
	ctx             context.Context
	components      ComponentRenderer
	claims          ClaimRenderer
}

// ComponentRenderer 由装配层实现，负责校验冻结组件版本并解析 Entity/Claim。
type ComponentRenderer interface {
	RenderComponent(context.Context, *ast.Block) (string, error)
}

// ClaimRenderer resolves a stable Claim reference to its current display value.
// Implementations must return escaped, trusted HTML and may follow a supersede
// chain. The interface lives in render so both projection builds and the
// authoritative read fallback share the exact same rendering contract.
type ClaimRenderer interface {
	RenderClaim(context.Context, string) (string, error)
}

// DynamicRenderer is the complete resolver required by a production reading
// path. A single implementation keeps Component and Claim reads consistent.
type DynamicRenderer interface {
	ComponentRenderer
	ClaimRenderer
}

func newRenderContext(
	ctx context.Context,
	components ComponentRenderer,
	claims ClaimRenderer,
	citationOrder []string,
) *renderContext {
	result := &renderContext{
		citationNumbers: make(map[string]int),
		ctx:             ctx,
		components:      components,
		claims:          claims,
	}
	for _, citationID := range citationOrder {
		if citationID == "" {
			continue
		}
		if _, exists := result.citationNumbers[citationID]; exists {
			continue
		}
		result.citationNumbers[citationID] = len(result.citationNumbers) + 1
	}
	return result
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
	return RenderHTMLWithResolvers(context.Background(), doc, nil, nil)
}

// RenderHTMLWithComponents 渲染动态 ComponentBlock；resolver 缺失时输出安全占位，
// 保证阅读路径的受控降级不执行不可信模板或返回陈旧 Claim 值。
func RenderHTMLWithComponents(
	ctx context.Context, doc *ast.Document, components ComponentRenderer,
) (string, error) {
	return RenderHTMLWithResolvers(ctx, doc, components, nil)
}

// RenderHTMLWithResolvers resolves both ComponentBlock and ClaimReference
// nodes. Resolvers are trusted server-side implementations; every other text
// and attribute continues to be escaped by this package.
func RenderHTMLWithResolvers(
	ctx context.Context,
	doc *ast.Document,
	components ComponentRenderer,
	claims ClaimRenderer,
) (string, error) {
	return RenderHTMLWithResolversAndCitationOrder(
		ctx, doc, components, claims, nil,
	)
}

// RenderHTMLWithResolversAndCitationOrder keeps Citation numbers stable when
// only an AST fragment is rendered. citationOrder must be the first-appearance
// order collected from the complete immutable Revision.
func RenderHTMLWithResolversAndCitationOrder(
	ctx context.Context,
	doc *ast.Document,
	components ComponentRenderer,
	claims ClaimRenderer,
	citationOrder []string,
) (string, error) {
	if doc == nil {
		return "", fmt.Errorf("render: doc 为 nil")
	}
	var b strings.Builder
	if err := renderBlocks(
		&b,
		doc.Children,
		newRenderContext(ctx, components, claims, citationOrder),
	); err != nil {
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
		if blk.IsToggleable {
			fmt.Fprintf(b, `<details class="toggleable-heading"><summary id="%s"><span role="heading" aria-level="%d">`,
				html.EscapeString(blk.ID), level)
			if err := renderInlines(b, blk, ctx); err != nil {
				return err
			}
			b.WriteString("</span></summary>")
			if err := renderBlocks(b, blk.Children, ctx); err != nil {
				return err
			}
			b.WriteString("</details>")
			return nil
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

	case ast.BlockTodo:
		checked := blk.Checked != nil && *blk.Checked
		fmt.Fprintf(b, `<div class="todo" data-checked="%t"><input type="checkbox" disabled`, checked)
		if checked {
			b.WriteString(" checked")
		}
		b.WriteString(` aria-hidden="true"><span class="todo-content">`)
		if err := renderInlines(b, blk, ctx); err != nil {
			return err
		}
		b.WriteString("</span>")
		if err := renderBlocks(b, blk.Children, ctx); err != nil {
			return err
		}
		b.WriteString("</div>")
		return nil

	case ast.BlockToggle:
		b.WriteString(`<details class="toggle"><summary>`)
		if err := renderInlines(b, blk, ctx); err != nil {
			return err
		}
		b.WriteString("</summary>")
		if err := renderBlocks(b, blk.Children, ctx); err != nil {
			return err
		}
		b.WriteString("</details>")
		return nil

	case ast.BlockBreadcrumb:
		b.WriteString(`<nav class="wiki-breadcrumb" aria-label="Breadcrumb"><ol>`)
		for index, item := range blk.Items {
			b.WriteString("<li>")
			if item.TargetPageID != "" {
				fmt.Fprintf(b, `<a href="/pages/%s">%s</a>`,
					html.EscapeString(item.TargetPageID), html.EscapeString(item.DisplayText))
			} else {
				fmt.Fprintf(b, `<span aria-current="page">%s</span>`, html.EscapeString(item.DisplayText))
			}
			b.WriteString("</li>")
			if index < len(blk.Items)-1 {
				b.WriteString(`<li class="breadcrumb-separator" aria-hidden="true">/</li>`)
			}
		}
		b.WriteString("</ol></nav>")
		return nil

	case ast.BlockColumnList:
		b.WriteString(`<div class="column-list">`)
		if err := renderBlocks(b, blk.Children, ctx); err != nil {
			return err
		}
		b.WriteString("</div>")
		return nil

	case ast.BlockColumn:
		fmt.Fprintf(b, `<section class="column" style="--column-ratio:%g">`, blk.WidthRatio)
		if err := renderBlocks(b, blk.Children, ctx); err != nil {
			return err
		}
		b.WriteString("</section>")
		return nil

	case ast.BlockButton:
		actions, err := json.Marshal(blk.Actions)
		if err != nil {
			return fmt.Errorf("render: ButtonBlock %s 动作序列化失败: %w", blk.ID, err)
		}
		fmt.Fprintf(b,
			`<button class="wiki-button" type="button" data-page-button data-block-id="%s" data-actions="%s"`,
			html.EscapeString(blk.ID),
			html.EscapeString(base64.RawURLEncoding.EncodeToString(actions)),
		)
		if blk.Confirmation != "" {
			fmt.Fprintf(b, ` data-confirmation="%s"`, html.EscapeString(blk.Confirmation))
		}
		fmt.Fprintf(b, ">%s</button>", html.EscapeString(blk.Label))
		return nil

	case ast.BlockHTML:
		title := blk.Caption
		if title == "" {
			title = "HTML content"
		}
		srcdoc := isolatedHTMLDocument(blk.HTML)
		fmt.Fprintf(b,
			`<figure class="html-block"><iframe sandbox="" loading="lazy" referrerpolicy="no-referrer" title="%s" height="%d" srcdoc="%s"></iframe>`,
			html.EscapeString(title), blk.Height, html.EscapeString(srcdoc))
		if blk.Caption != "" {
			fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
		}
		b.WriteString("</figure>")
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
		return renderTable(b, blk, ctx)

	case ast.BlockCode:
		code, err := blk.TextContent()
		if err != nil {
			return err
		}
		b.WriteString(`<figure class="wiki-code">`)
		if blk.Caption != "" || blk.Language != "" {
			b.WriteString("<figcaption>")
			if blk.Caption != "" {
				fmt.Fprintf(b, `<span class="code-caption">%s</span>`, html.EscapeString(blk.Caption))
			}
			if blk.Language != "" {
				fmt.Fprintf(b, `<span class="code-language">%s</span>`, html.EscapeString(blk.Language))
			}
			b.WriteString("</figcaption>")
		}
		b.WriteString("<pre><code")
		if blk.Language != "" {
			fmt.Fprintf(b, ` class="language-%s"`, html.EscapeString(blk.Language))
		}
		b.WriteString(">")
		b.WriteString(html.EscapeString(code))
		b.WriteString("</code></pre></figure>")
		return nil

	case ast.BlockPreformatted:
		text, err := blk.TextContent()
		if err != nil {
			return err
		}
		fmt.Fprintf(b, `<figure class="preformatted" data-preformatted-kind="%s">`,
			html.EscapeString(blk.Kind))
		if blk.Caption != "" {
			fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
		}
		b.WriteString("<pre><code>")
		b.WriteString(html.EscapeString(text))
		b.WriteString("</code></pre></figure>")
		return nil

	case ast.BlockEquation:
		fmt.Fprintf(b, `<figure class="equation-block" data-math-expression="%s"><div class="math">%s</div>`,
			html.EscapeString(blk.Expression), html.EscapeString(blk.Expression))
		if blk.Caption != "" {
			fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
		}
		b.WriteString("</figure>")
		return nil

	case ast.BlockCallout:
		kind := string(blk.Kind)
		fmt.Fprintf(b, `<div class="callout" data-callout="%s"`, html.EscapeString(kind))
		if blk.Color != "" {
			fmt.Fprintf(b, ` data-callout-color="%s"`, html.EscapeString(string(blk.Color)))
		}
		b.WriteString(">")
		b.WriteString(`<div class="callout-title">`)
		if blk.Icon != "" {
			fmt.Fprintf(b, `<span class="callout-icon" aria-hidden="true">%s</span>`, html.EscapeString(blk.Icon))
		}
		b.WriteString(html.EscapeString(kind))
		b.WriteString("</div>")
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

	case ast.BlockImage:
		fmt.Fprintf(b,
			`<figure class="wiki-image"><img src="/api/v1/assets/revisions/%s/content" alt="%s" loading="lazy" decoding="async">`,
			html.EscapeString(blk.AssetRevisionID), html.EscapeString(blk.AltText))
		if blk.Caption != "" {
			fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
		}
		b.WriteString("</figure>")
		return nil

	case ast.BlockVideo:
		fmt.Fprintf(b, `<figure class="wiki-video"><video controls preload="metadata" src="/api/v1/assets/revisions/%s/content"`,
			html.EscapeString(blk.AssetRevisionID))
		if blk.PosterAssetRevisionID != "" {
			fmt.Fprintf(b, ` poster="/api/v1/assets/revisions/%s/content"`,
				html.EscapeString(blk.PosterAssetRevisionID))
		}
		if blk.Title != "" {
			fmt.Fprintf(b, ` title="%s"`, html.EscapeString(blk.Title))
		}
		b.WriteString("></video>")
		if blk.Caption != "" {
			fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
		}
		b.WriteString("</figure>")
		return nil

	case ast.BlockAudio:
		fmt.Fprintf(b,
			`<figure class="wiki-audio"><div class="audio-metadata"><strong>%s</strong>`,
			html.EscapeString(blk.Title))
		if blk.Artist != "" {
			fmt.Fprintf(b, `<span>%s</span>`, html.EscapeString(blk.Artist))
		}
		if blk.Album != "" {
			fmt.Fprintf(b, `<span>%s</span>`, html.EscapeString(blk.Album))
		}
		if blk.DurationSeconds != nil {
			fmt.Fprintf(b, `<span class="audio-duration" data-duration-seconds="%g">%s</span>`,
				*blk.DurationSeconds, html.EscapeString(formatDurationSeconds(*blk.DurationSeconds)))
		}
		b.WriteString(`</div><audio controls preload="metadata" src="/api/v1/assets/revisions/`)
		b.WriteString(html.EscapeString(blk.AssetRevisionID))
		b.WriteString(`/content"></audio>`)
		if blk.Caption != "" {
			fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
		}
		b.WriteString("</figure>")
		return nil

	case ast.BlockFile:
		fmt.Fprintf(b,
			`<aside class="wiki-file"><a href="/api/v1/assets/revisions/%s/content" download="%s"><span class="file-name">%s</span><span class="file-action">Download</span></a>`,
			html.EscapeString(blk.AssetRevisionID), html.EscapeString(blk.Name),
			html.EscapeString(blk.Name))
		if blk.Caption != "" {
			fmt.Fprintf(b, `<p>%s</p>`, html.EscapeString(blk.Caption))
		}
		b.WriteString("</aside>")
		return nil

	case ast.BlockPDF:
		title := blk.Title
		if title == "" {
			title = "PDF document"
		}
		startPage := blk.StartPage
		if startPage < 1 {
			startPage = 1
		}
		source := fmt.Sprintf(
			"/api/v1/assets/revisions/%s/content#page=%d&view=FitH",
			blk.AssetRevisionID, startPage,
		)
		fmt.Fprintf(b,
			`<figure class="wiki-pdf"><iframe src="%s" title="%s" height="%d" loading="lazy"></iframe>`,
			html.EscapeString(source), html.EscapeString(title), blk.Height)
		if blk.Caption != "" {
			fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
		}
		b.WriteString("</figure>")
		return nil

	case ast.BlockDatasetView:
		fmt.Fprintf(b,
			`<div class="dataset-view" data-dataset-view-id="%s"><a href="/datasets/views/%s">打开数据视图</a></div>`,
			html.EscapeString(blk.DatasetViewID), html.EscapeString(blk.DatasetViewID))
		return nil

	case ast.BlockBookmark:
		return renderExternalCard(b, blk, "wiki-bookmark")

	case ast.BlockLinkPreview:
		return renderExternalCard(b, blk, "wiki-link-preview")

	case ast.BlockEmbed:
		title := blk.Title
		if title == "" {
			title = blk.URL
		}
		if embedURL, ok := interactiveEmbedURL(blk.Provider, blk.URL); ok {
			fmt.Fprintf(b,
				`<figure class="interactive-embed" data-embed-provider="%s"><iframe src="%s" title="%s" loading="lazy" referrerpolicy="strict-origin-when-cross-origin" sandbox="allow-scripts allow-same-origin allow-presentation" allow="accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture; fullscreen" allowfullscreen></iframe>`,
				html.EscapeString(blk.Provider), html.EscapeString(embedURL),
				html.EscapeString(title))
			if blk.Caption != "" {
				fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
			}
			b.WriteString("</figure>")
			return nil
		}
		if !isSafeExternalURL(blk.URL) {
			b.WriteString(html.EscapeString(title))
			return nil
		}
		fmt.Fprintf(b,
			`<aside class="external-embed"><a href="%s" rel="noopener noreferrer nofollow" target="_blank">%s</a></aside>`,
			html.EscapeString(blk.URL), html.EscapeString(title))
		return nil

	case ast.BlockDivider:
		b.WriteString("<hr>")
		return nil

	default:
		return fmt.Errorf("render: 未知 Block 类型 %q（Block %s）", blk.Type, blk.ID)
	}
}

func renderExternalCard(b *strings.Builder, blk *ast.Block, className string) error {
	if !isSafeExternalURL(blk.URL) {
		b.WriteString(html.EscapeString(blk.Title))
		return nil
	}
	siteName := blk.SiteName
	if siteName == "" {
		if parsed, err := url.Parse(blk.URL); err == nil {
			siteName = parsed.Hostname()
		}
	}
	fmt.Fprintf(b,
		`<figure class="%s"><a href="%s" rel="noopener noreferrer nofollow" target="_blank">`,
		className, html.EscapeString(blk.URL))
	if siteName != "" {
		fmt.Fprintf(b, `<span class="preview-site">%s</span>`, html.EscapeString(siteName))
	}
	fmt.Fprintf(b, `<strong>%s</strong>`, html.EscapeString(blk.Title))
	if blk.Description != "" {
		fmt.Fprintf(b, `<span class="preview-description">%s</span>`,
			html.EscapeString(blk.Description))
	}
	b.WriteString("</a>")
	if blk.Caption != "" {
		fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(blk.Caption))
	}
	b.WriteString("</figure>")
	return nil
}

func interactiveEmbedURL(provider, rawURL string) (string, bool) {
	if provider == "" || !isSafeExternalURL(rawURL) {
		return "", false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	switch provider {
	case "youtube":
		var videoID string
		switch host {
		case "youtu.be":
			videoID = strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")[0]
		case "youtube.com", "m.youtube.com":
			videoID = parsed.Query().Get("v")
			if videoID == "" && strings.HasPrefix(parsed.Path, "/embed/") {
				videoID = strings.TrimPrefix(parsed.Path, "/embed/")
			}
		}
		if safeEmbedToken(videoID) {
			return "https://www.youtube-nocookie.com/embed/" + videoID, true
		}
	case "vimeo":
		if host == "vimeo.com" || host == "player.vimeo.com" {
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			videoID := parts[len(parts)-1]
			if videoID != "" && strings.IndexFunc(videoID, func(r rune) bool {
				return r < '0' || r > '9'
			}) == -1 {
				return "https://player.vimeo.com/video/" + videoID, true
			}
		}
	case "figma":
		if host == "figma.com" &&
			(strings.HasPrefix(parsed.Path, "/design/") ||
				strings.HasPrefix(parsed.Path, "/file/") ||
				strings.HasPrefix(parsed.Path, "/proto/")) {
			return "https://www.figma.com/embed?embed_host=anby&url=" +
				url.QueryEscape(parsed.String()), true
		}
	}
	return "", false
}

func safeEmbedToken(value string) bool {
	value = strings.Trim(value, "/")
	if len(value) < 6 || len(value) > 32 {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-')
	}) == -1
}

func formatDurationSeconds(value float64) string {
	total := int(value + 0.5)
	if total < 0 {
		total = 0
	}
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func isolatedHTMLDocument(content string) string {
	const policy = "default-src 'none'; img-src data:; font-src data:; style-src 'unsafe-inline'; form-action 'none'; base-uri 'none'"
	return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="` +
		policy +
		`"><meta name="viewport" content="width=device-width,initial-scale=1"><style>html{color-scheme:light}body{margin:0;padding:12px;font:14px/1.5 system-ui,sans-serif;overflow-wrap:anywhere}img,video,canvas,svg{max-width:100%;height:auto}table{max-width:100%;border-collapse:collapse}pre{max-width:100%;overflow:auto}</style></head><body>` +
		content + `</body></html>`
}

func renderTable(b *strings.Builder, table *ast.Block, ctx *renderContext) error {
	b.WriteString(`<figure class="wiki-table"><table>`)
	if table.Caption != "" {
		fmt.Fprintf(b, "<caption>%s</caption>", html.EscapeString(table.Caption))
	}
	bodyStart := 0
	if table.HasColumnHeader && len(table.Children) > 0 {
		b.WriteString("<thead>")
		if err := renderTableRow(b, table.Children[0], 0, true, table.HasRowHeader, ctx); err != nil {
			return err
		}
		b.WriteString("</thead>")
		bodyStart = 1
	}
	b.WriteString("<tbody>")
	for rowIndex := bodyStart; rowIndex < len(table.Children); rowIndex++ {
		if err := renderTableRow(
			b, table.Children[rowIndex], rowIndex, false, table.HasRowHeader, ctx,
		); err != nil {
			return err
		}
	}
	b.WriteString("</tbody></table></figure>")
	return nil
}

func renderTableRow(
	b *strings.Builder,
	row *ast.Block,
	rowIndex int,
	columnHeader bool,
	rowHeader bool,
	ctx *renderContext,
) error {
	b.WriteString("<tr>")
	for cellIndex, cell := range row.Children {
		tag, scope := "td", ""
		if columnHeader {
			tag, scope = "th", "col"
		} else if rowHeader && cellIndex == 0 {
			tag, scope = "th", "row"
		}
		fmt.Fprintf(b, "<%s", tag)
		if scope != "" {
			fmt.Fprintf(b, ` scope="%s"`, scope)
		}
		fmt.Fprintf(b, ` data-row="%d" data-column="%d">`, rowIndex, cellIndex)
		if err := renderBlocks(b, cell.Children, ctx); err != nil {
			return err
		}
		fmt.Fprintf(b, "</%s>", tag)
	}
	b.WriteString("</tr>")
	return nil
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
	for index, n := range nodes {
		if err := renderInline(b, n, ctx, blk.ID, index); err != nil {
			return err
		}
	}
	return nil
}

func renderInline(
	b *strings.Builder,
	n *ast.InlineNode,
	ctx *renderContext,
	blockID string,
	nodeIndex int,
) error {
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
		if n.TextColor != "" || n.BackgroundColor != "" {
			var attrs strings.Builder
			if n.TextColor != "" {
				fmt.Fprintf(&attrs, ` data-text-color="%s"`, html.EscapeString(string(n.TextColor)))
			}
			if n.BackgroundColor != "" {
				fmt.Fprintf(&attrs, ` data-background-color="%s"`, html.EscapeString(string(n.BackgroundColor)))
			}
			text = "<span" + attrs.String() + ">" + text + "</span>"
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

	case ast.InlinePageAnchorReference:
		href := "/pages/" + n.TargetPageID + "#" + n.TargetHeadingBlockID
		fmt.Fprintf(b, `<a href="%s" data-page-anchor-ref>%s</a>`,
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
		fmt.Fprintf(b, `<a href="/concepts/%s" data-entity-ref="%s">%s</a>`,
			html.EscapeString(n.EntityID), html.EscapeString(n.EntityID), html.EscapeString(n.DisplayText))
		return nil

	case ast.InlineClaimReference:
		if ctx.claims != nil {
			rendered, err := ctx.claims.RenderClaim(ctx.ctx, n.ClaimID)
			if err != nil {
				return fmt.Errorf(
					"render: ClaimReference %s 渲染失败: %w",
					n.ClaimID,
					err,
				)
			}
			b.WriteString(rendered)
			return nil
		}
		fmt.Fprintf(b, `<a href="/claims/%s" data-claim-ref="%s">%s</a>`,
			html.EscapeString(n.ClaimID), html.EscapeString(n.ClaimID), html.EscapeString(n.DisplayText))
		return nil

	case ast.InlineCitationReference:
		title := n.DisplayText
		if title == "" {
			title = n.CitationID
		}
		number := ctx.citationNumber(n.CitationID)
		fmt.Fprintf(b, `<sup id="cite-ref-%s-%d" data-citation-ref="%s" title="%s"><a href="#cite-note-%d">[%d]</a></sup>`,
			html.EscapeString(blockID), nodeIndex,
			html.EscapeString(n.CitationID), html.EscapeString(title), number, number)
		return nil

	case ast.InlineMath:
		fmt.Fprintf(b, `<span class="math" data-math-expression="%s">%s</span>`,
			html.EscapeString(n.Expression), html.EscapeString(n.Expression))
		return nil

	case ast.InlineMention:
		fmt.Fprintf(b, `<span class="mention" data-actor-id="%s">@%s</span>`,
			html.EscapeString(n.ActorID), html.EscapeString(n.DisplayText))
		return nil

	case ast.InlineDateMention:
		if n.EndDate != "" {
			fmt.Fprintf(b, `<time datetime="%s" data-date-mention data-end-date="%s">%s</time>`,
				html.EscapeString(n.Date), html.EscapeString(n.EndDate), html.EscapeString(n.DisplayText))
			return nil
		}
		fmt.Fprintf(b, `<time datetime="%s" data-date-mention>%s</time>`,
			html.EscapeString(n.Date), html.EscapeString(n.DisplayText))
		return nil

	case ast.InlineDatabaseMention:
		fmt.Fprintf(b, `<a href="/datasets/%s" data-database-mention>%s</a>`,
			html.EscapeString(n.DatasetID), html.EscapeString(n.DisplayText))
		return nil

	default:
		return fmt.Errorf("render: 未知 Inline 类型 %q", n.Type)
	}
}

// markTags mark → HTML 标签映射。
var markTags = map[ast.Mark]string{
	ast.MarkBold:          "strong",
	ast.MarkItalic:        "em",
	ast.MarkUnderline:     "u",
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
