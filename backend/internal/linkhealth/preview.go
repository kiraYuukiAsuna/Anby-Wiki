package linkhealth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/anby/wiki/backend/internal/evidence"
	nethtml "golang.org/x/net/html"
)

var (
	ErrPreviewUnavailable = errors.New("linkhealth: preview unavailable")
	ErrPreviewTooLarge    = errors.New("linkhealth: preview too large")
)

const maxPreviewBytes = 512 << 10

type PreviewResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
}

func (p *HTTPProber) ResolvePreview(
	ctx context.Context,
	rawURL string,
) (PreviewResult, error) {
	target, err := url.Parse(rawURL)
	if err != nil || p.validateURL(ctx, target) != nil {
		return PreviewResult{}, ErrUnsafeURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return PreviewResult{}, ErrUnsafeURL
	}
	request.Header.Set("User-Agent", "AnbyWiki-LinkPreview/1.0")
	request.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")
	response, err := p.client.Do(request)
	if err != nil {
		if errors.Is(err, ErrUnsafeURL) {
			return PreviewResult{}, ErrUnsafeURL
		}
		return PreviewResult{}, fmt.Errorf("%w: %v", ErrPreviewUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return PreviewResult{}, fmt.Errorf(
			"%w: status=%d", ErrPreviewUnavailable, response.StatusCode,
		)
	}
	finalURL, err := evidence.NormalizeURL(response.Request.URL.String())
	if err != nil {
		return PreviewResult{}, ErrUnsafeURL
	}
	parsedFinal, _ := url.Parse(finalURL)
	result := PreviewResult{
		URL:      finalURL,
		SiteName: parsedFinal.Hostname(),
		Title:    previewFallbackTitle(parsedFinal),
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return result, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPreviewBytes+1))
	if err != nil {
		return PreviewResult{}, fmt.Errorf("%w: read body: %v", ErrPreviewUnavailable, err)
	}
	if len(body) > maxPreviewBytes {
		return PreviewResult{}, ErrPreviewTooLarge
	}
	metadata := parsePreviewHTML(body)
	if metadata.Title != "" {
		result.Title = metadata.Title
	}
	if metadata.Description != "" {
		result.Description = metadata.Description
	}
	if metadata.SiteName != "" {
		result.SiteName = metadata.SiteName
	}
	return result, nil
}

func parsePreviewHTML(body []byte) PreviewResult {
	root, err := nethtml.Parse(strings.NewReader(string(body)))
	if err != nil {
		return PreviewResult{}
	}
	values := map[string]string{}
	var title string
	var visit func(*nethtml.Node)
	visit = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode {
			tag := strings.ToLower(node.Data)
			if tag == "meta" {
				var key, content string
				for _, attribute := range node.Attr {
					switch strings.ToLower(attribute.Key) {
					case "property", "name":
						key = strings.ToLower(strings.TrimSpace(attribute.Val))
					case "content":
						content = attribute.Val
					}
				}
				if key != "" && content != "" && values[key] == "" {
					values[key] = previewText(content, 1000)
				}
			}
			if tag == "title" && title == "" && node.FirstChild != nil {
				title = previewText(node.FirstChild.Data, 500)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	result := PreviewResult{
		Title: firstPreviewValue(values,
			"og:title", "twitter:title", "title"),
		Description: firstPreviewValue(values,
			"og:description", "twitter:description", "description"),
		SiteName: firstPreviewValue(values, "og:site_name", "application-name"),
	}
	if result.Title == "" {
		result.Title = title
	}
	return result
}

func firstPreviewValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if values[key] != "" {
			return values[key]
		}
	}
	return ""
}

func previewFallbackTitle(target *url.URL) string {
	if target == nil {
		return "External link"
	}
	parts := strings.Split(strings.Trim(target.Path, "/"), "/")
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		return previewText(parts[len(parts)-1], 500)
	}
	return target.Hostname()
}

func previewText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
