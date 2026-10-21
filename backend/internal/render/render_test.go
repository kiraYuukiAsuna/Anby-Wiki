package render

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
)

func TestCitationOrderAndBidirectionalAnchors(t *testing.T) {
	firstCitation := uuid.New().String()
	secondCitation := uuid.New().String()
	firstBlock := uuid.New().String()
	secondBlock := uuid.New().String()
	content := func(nodes ...*ast.InlineNode) json.RawMessage {
		raw, err := json.Marshal(nodes)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{
			{ID: firstBlock, Type: ast.BlockParagraph, Content: content(
				&ast.InlineNode{Type: ast.InlineText, Text: "Second first"},
				&ast.InlineNode{Type: ast.InlineCitationReference, CitationID: secondCitation},
			)},
			{ID: secondBlock, Type: ast.BlockParagraph, Content: content(
				&ast.InlineNode{Type: ast.InlineCitationReference, CitationID: firstCitation},
				&ast.InlineNode{Type: ast.InlineText, Text: " repeated "},
				&ast.InlineNode{Type: ast.InlineCitationReference, CitationID: secondCitation},
			)},
		},
	}

	html, err := RenderHTMLWithResolversAndCitationOrder(
		context.Background(), document, nil, nil,
		[]string{firstCitation, secondCitation},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`id="cite-ref-` + firstBlock + `-1"`,
		`href="#cite-note-2">[2]</a>`,
		`id="cite-ref-` + secondBlock + `-0"`,
		`href="#cite-note-1">[1]</a>`,
		`id="cite-ref-` + secondBlock + `-2"`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rendered citation HTML missing %q: %s", expected, html)
		}
	}
}

func TestInlineRichTextAndMentions(t *testing.T) {
	actorID := uuid.NewString()
	datasetID := uuid.NewString()
	raw, err := json.Marshal([]*ast.InlineNode{
		{
			Type: ast.InlineText, Text: "<styled>",
			Marks:     []ast.Mark{ast.MarkBold, ast.MarkUnderline},
			TextColor: ast.InlineColorRed, BackgroundColor: ast.InlineColorYellow,
		},
		{Type: ast.InlineMention, ActorID: actorID, DisplayText: "<Alice>"},
		{
			Type: ast.InlineDateMention, Date: "2026-10-20", EndDate: "2026-10-22",
			DisplayText: "Oct <20-22>",
		},
		{Type: ast.InlineDatabaseMention, DatasetID: datasetID, DisplayText: "<Sources>"},
	})
	if err != nil {
		t.Fatal(err)
	}
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{{
			ID: uuid.NewString(), Type: ast.BlockParagraph, Content: raw,
		}},
	}
	if err := ast.Validate(document); err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderHTML(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`<span data-text-color="red" data-background-color="yellow"><strong><u>&lt;styled&gt;</u></strong></span>`,
		`class="mention" data-actor-id="` + actorID + `">@&lt;Alice&gt;</span>`,
		`<time datetime="2026-10-20" data-date-mention data-end-date="2026-10-22">Oct &lt;20-22&gt;</time>`,
		`<a href="/datasets/` + datasetID + `" data-database-mention>&lt;Sources&gt;</a>`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered rich inline HTML missing %q: %s", expected, rendered)
		}
	}

	document.Children[0].Content, err = json.Marshal([]*ast.InlineNode{{
		Type: ast.InlineText, Text: "unsafe", TextColor: ast.InlineColor("#fff"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ast.Validate(document); err == nil {
		t.Fatal("AST accepted arbitrary CSS text color")
	}
}

func TestExtendedContentBlocksRenderSafely(t *testing.T) {
	content := func(text string) json.RawMessage {
		raw, err := json.Marshal([]*ast.InlineNode{{
			Type: ast.InlineText, Text: text,
		}})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	checked := true
	paragraph := func(text string) *ast.Block {
		return &ast.Block{
			ID: uuid.NewString(), Type: ast.BlockParagraph,
			Content: content(text),
		}
	}
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{
			{
				ID: uuid.NewString(), Type: ast.BlockHeading, Level: 2,
				IsToggleable: true, Content: content("Folded"),
				Children: []*ast.Block{paragraph("Heading body")},
			},
			{
				ID: uuid.NewString(), Type: ast.BlockTodo, Checked: &checked,
				Content: content("Done"),
			},
			{
				ID: uuid.NewString(), Type: ast.BlockToggle,
				Content: content("Details"), Children: []*ast.Block{paragraph("Toggle body")},
			},
			{
				ID: uuid.NewString(), Type: ast.BlockCallout,
				Kind: string(ast.CalloutWarning), Icon: "<i>",
				Color:    ast.CalloutColorPurple,
				Children: []*ast.Block{paragraph("Warning body")},
			},
			{
				ID: uuid.NewString(), Type: ast.BlockTable,
				Caption: "Matrix", HasColumnHeader: true, HasRowHeader: true,
				Children: []*ast.Block{
					{
						ID: uuid.NewString(), Type: ast.BlockTableRow,
						Children: []*ast.Block{
							{ID: uuid.NewString(), Type: ast.BlockTableCell, Children: []*ast.Block{paragraph("Name")}},
							{ID: uuid.NewString(), Type: ast.BlockTableCell, Children: []*ast.Block{paragraph("Value")}},
						},
					},
					{
						ID: uuid.NewString(), Type: ast.BlockTableRow,
						Children: []*ast.Block{
							{ID: uuid.NewString(), Type: ast.BlockTableCell, Children: []*ast.Block{paragraph("Alpha")}},
							{ID: uuid.NewString(), Type: ast.BlockTableCell, Children: []*ast.Block{paragraph("1")}},
						},
					},
				},
			},
			{
				ID: uuid.NewString(), Type: ast.BlockCode,
				Language: "go", Caption: "Example",
				Content: json.RawMessage(`"fmt.Println(\"<x>\")"`),
			},
			{
				ID: uuid.NewString(), Type: ast.BlockPreformatted,
				Kind: string(ast.PreformattedASCIIDiagram), Caption: "Flow",
				Content: json.RawMessage(`"+---+\n|<x>|\n+---+"`),
			},
			{
				ID: uuid.NewString(), Type: ast.BlockEquation,
				Expression: `x < y`, Caption: "Constraint",
			},
		},
	}
	if err := ast.Validate(document); err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderHTML(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`<details class="toggleable-heading">`,
		`class="todo" data-checked="true"`,
		`<details class="toggle">`,
		`data-callout-color="purple"`,
		`&lt;i&gt;`,
		`<caption>Matrix</caption>`,
		`<th scope="col"`,
		`<th scope="row"`,
		`<span class="code-caption">Example</span>`,
		`fmt.Println(&#34;&lt;x&gt;&#34;)`,
		`data-preformatted-kind="ascii_diagram"`,
		`|&lt;x&gt;|`,
		`data-math-expression="x &lt; y"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered extended HTML missing %q: %s", expected, rendered)
		}
	}
}

func TestLayoutActionAndIsolatedHTMLBlocks(t *testing.T) {
	content := func(text string) json.RawMessage {
		raw, err := json.Marshal([]*ast.InlineNode{{Type: ast.InlineText, Text: text}})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	paragraph := func(text string) *ast.Block {
		return &ast.Block{
			ID: uuid.NewString(), Type: ast.BlockParagraph, Content: content(text),
		}
	}
	targetPageID := uuid.NewString()
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{
			{
				ID: uuid.NewString(), Type: ast.BlockBreadcrumb,
				Items: []ast.BreadcrumbItem{
					{TargetPageID: targetPageID, DisplayText: "Parent"},
					{DisplayText: "Current"},
				},
			},
			{
				ID: uuid.NewString(), Type: ast.BlockColumnList,
				Children: []*ast.Block{
					{
						ID: uuid.NewString(), Type: ast.BlockColumn,
						WidthRatio: 1, Children: []*ast.Block{paragraph("Left")},
					},
					{
						ID: uuid.NewString(), Type: ast.BlockColumn,
						WidthRatio: 2, Children: []*ast.Block{paragraph("Right")},
					},
				},
			},
			{
				ID: uuid.NewString(), Type: ast.BlockButton, Label: `Run <safe>`,
				Confirmation: `Continue "now"?`,
				Actions: []ast.ButtonAction{
					{
						Type:    ast.ButtonActionNotification,
						Message: "Complete", Tone: "success",
					},
					{
						Type:    ast.ButtonActionWebhook,
						URL:     "https://example.com/hook",
						Payload: json.RawMessage(`{"event":"done"}`),
					},
				},
			},
			{
				ID: uuid.NewString(), Type: ast.BlockHTML,
				HTML:   `<h1>Preview</h1><script>alert(document.cookie)</script>`,
				Height: 240, Caption: `Upload "preview"`,
			},
		},
	}
	if err := ast.Validate(document); err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderHTML(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`class="wiki-breadcrumb"`,
		`href="/pages/` + targetPageID + `"`,
		`aria-current="page">Current</span>`,
		`class="column-list"`,
		`style="--column-ratio:2"`,
		`data-page-button`,
		`data-confirmation="Continue &#34;now&#34;?"`,
		`Run &lt;safe&gt;`,
		`<iframe sandbox=""`,
		`referrerpolicy="no-referrer"`,
		`height="240"`,
		`Content-Security-Policy`,
		`default-src &#39;none&#39;`,
		`&lt;script&gt;alert(document.cookie)&lt;/script&gt;`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered layout/action HTML missing %q: %s", expected, rendered)
		}
	}
	if strings.Contains(rendered, `<script>`) {
		t.Fatalf("rendered HTML exposed executable script: %s", rendered)
	}
}

func TestMediaAndExternalContentBlocksRenderSafely(t *testing.T) {
	assetID := uuid.NewString()
	duration := 95.4
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{
			{
				ID: uuid.NewString(), Type: ast.BlockAudio,
				AssetRevisionID: assetID, Title: "Episode <1>",
				Artist: "Anby", Album: "Protocol", Caption: "Audio caption",
				DurationSeconds: &duration,
			},
			{
				ID: uuid.NewString(), Type: ast.BlockFile,
				AssetRevisionID: assetID, Name: `guide "final".zip`,
				Caption: "Download package",
			},
			{
				ID: uuid.NewString(), Type: ast.BlockPDF,
				AssetRevisionID: assetID, Title: "RFC", StartPage: 3,
				Height: 640, Caption: "PDF caption",
			},
			{
				ID: uuid.NewString(), Type: ast.BlockBookmark,
				URL: "https://example.com/docs", Title: "Docs <safe>",
				Description: "Description", SiteName: "Example", Caption: "Bookmark caption",
			},
			{
				ID: uuid.NewString(), Type: ast.BlockLinkPreview,
				URL: "https://example.com/news", Title: "Preview",
				Description: "Third-party metadata", SiteName: "Example",
			},
			{
				ID: uuid.NewString(), Type: ast.BlockEmbed,
				URL:   "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
				Title: "Video", Provider: "youtube", Caption: "Interactive",
			},
		},
	}
	if err := ast.Validate(document); err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderHTML(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`class="wiki-audio"`,
		`Episode &lt;1&gt;`,
		`<audio controls preload="metadata"`,
		`data-duration-seconds="95.4">1:35</span>`,
		`class="wiki-file"`,
		`download="guide &#34;final&#34;.zip"`,
		`class="wiki-pdf"`,
		`#page=3&amp;view=FitH`,
		`height="640"`,
		`class="wiki-bookmark"`,
		`Docs &lt;safe&gt;`,
		`class="wiki-link-preview"`,
		`data-embed-provider="youtube"`,
		`src="https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"`,
		`sandbox="allow-scripts allow-same-origin allow-presentation"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered media HTML missing %q: %s", expected, rendered)
		}
	}
}

func TestInteractiveEmbedRejectsProviderURLMismatch(t *testing.T) {
	document := &ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{{
			ID: uuid.NewString(), Type: ast.BlockEmbed,
			URL:   "https://example.com/watch?v=dQw4w9WgXcQ",
			Title: "Untrusted", Provider: "youtube",
		}},
	}
	rendered, err := RenderHTML(document)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "<iframe") ||
		!strings.Contains(rendered, `class="external-embed"`) {
		t.Fatalf("provider mismatch did not degrade to link: %s", rendered)
	}
}
