package importer

import (
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const maxMetadataProbeBytes = 256 * 1024

var (
	metaTagPattern          = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	htmlTitlePattern        = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
	htmlHeadingPattern      = regexp.MustCompile(`(?is)<h1\b[^>]*>(.*?)</h1>`)
	htmlMarkupPattern       = regexp.MustCompile(`(?is)<[^>]+>`)
	htmlAttributePattern    = regexp.MustCompile("(?i)([a-z_:][a-z0-9_:.-]*)\\s*=\\s*(?:\"([^\"]*)\"|'([^']*)'|([^\\s\"'=<>`]+))")
	markdownTitlePattern    = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	frontMatterFieldPattern = regexp.MustCompile(`(?mi)^(title|author|publisher|date)\s*:\s*["']?(.+?)["']?\s*$`)
	rfcNumberPattern        = regexp.MustCompile(`(?mi)^Request for Comments:\s*([0-9]+)\s*`)
	rfcAbstractPattern      = regexp.MustCompile(`(?i)^abstract$`)
	monthYearPattern        = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+[12][0-9]{3}\b`)
)

type inferredSourceMetadata struct {
	Title       string
	Author      string
	Publisher   string
	PublishedAt *time.Time
	Metadata    json.RawMessage
}

// inferSourceMetadata extracts only bounded bibliographic metadata. It never
// stores source prose in metadata and therefore keeps the evidence content in
// immutable SourceVersion/Chunk records instead of leaking it into logs or
// list responses.
func inferSourceMetadata(source *AcquiredSource) inferredSourceMetadata {
	if source == nil {
		return inferredSourceMetadata{Metadata: json.RawMessage(`{}`)}
	}
	probe := source.Content
	if len(probe) > maxMetadataProbeBytes {
		probe = probe[:maxMetadataProbeBytes]
	}
	result := inferredSourceMetadata{}
	details := map[string]any{
		"filename":  source.Filename,
		"mime_type": source.MIMEType,
	}
	if parsed, err := url.Parse(source.URL); err == nil && parsed.Hostname() != "" {
		details["host"] = strings.ToLower(parsed.Hostname())
	}

	switch source.MIMEType {
	case "text/html":
		inferHTMLMetadata(string(probe), &result, details)
	case "application/json":
		inferJSONMetadata(probe, &result, details)
	case "text/plain":
		inferTextMetadata(string(probe), &result, details)
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		encoded = []byte(`{}`)
	}
	result.Metadata = encoded
	return result
}

func inferHTMLMetadata(
	content string,
	result *inferredSourceMetadata,
	details map[string]any,
) {
	values := map[string]string{}
	for _, rawTag := range metaTagPattern.FindAllString(content, -1) {
		attributes := htmlAttributes(rawTag)
		key := strings.ToLower(strings.TrimSpace(firstNonEmpty(
			attributes["property"], attributes["name"], attributes["itemprop"],
		)))
		value := cleanMetadataText(attributes["content"])
		if key != "" && value != "" && values[key] == "" {
			values[key] = value
		}
	}
	result.Title = firstNonEmptyValue(values,
		"citation_title", "og:title", "twitter:title", "dc.title")
	if result.Title == "" {
		result.Title = firstHTMLText(content, htmlTitlePattern)
	}
	if result.Title == "" {
		result.Title = firstHTMLText(content, htmlHeadingPattern)
	}
	result.Author = firstNonEmptyValue(values,
		"citation_author", "author", "article:author", "dc.creator")
	result.Publisher = firstNonEmptyValue(values,
		"citation_publisher", "publisher", "og:site_name", "dc.publisher")
	dateValue := firstNonEmptyValue(values,
		"citation_publication_date", "article:published_time", "date", "dc.date")
	result.PublishedAt = parseBibliographicDate(dateValue)
	if doi := firstNonEmptyValue(values, "citation_doi", "dc.identifier"); doi != "" {
		details["identifier"] = doi
	}
	if language := firstNonEmptyValue(values, "citation_language", "og:locale"); language != "" {
		details["language"] = language
	}
}

func inferJSONMetadata(
	content []byte,
	result *inferredSourceMetadata,
	details map[string]any,
) {
	var object map[string]any
	if json.Unmarshal(content, &object) != nil {
		return
	}
	stringValue := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := object[key].(string); ok {
				if cleaned := cleanMetadataText(value); cleaned != "" {
					return cleaned
				}
			}
		}
		return ""
	}
	result.Title = stringValue("title", "name", "headline")
	result.Author = stringValue("author", "creator")
	result.Publisher = stringValue("publisher", "provider")
	result.PublishedAt = parseBibliographicDate(
		stringValue("datePublished", "published_at", "publishedAt", "date"),
	)
	if identifier := stringValue("doi", "identifier"); identifier != "" {
		details["identifier"] = identifier
	}
}

func inferTextMetadata(
	content string,
	result *inferredSourceMetadata,
	details map[string]any,
) {
	for _, match := range frontMatterFieldPattern.FindAllStringSubmatch(content, 12) {
		if len(match) != 3 {
			continue
		}
		value := cleanMetadataText(match[2])
		switch strings.ToLower(match[1]) {
		case "title":
			if result.Title == "" {
				result.Title = value
			}
		case "author":
			result.Author = value
		case "publisher":
			result.Publisher = value
		case "date":
			result.PublishedAt = parseBibliographicDate(value)
		}
	}
	if result.Title == "" {
		inferRFCTextMetadata(content, result, details)
	}
	if result.Title == "" {
		if match := markdownTitlePattern.FindStringSubmatch(content); len(match) == 2 {
			result.Title = cleanMetadataText(match[1])
		}
	}
}

func inferRFCTextMetadata(
	content string,
	result *inferredSourceMetadata,
	details map[string]any,
) {
	numberMatch := rfcNumberPattern.FindStringSubmatch(content)
	if len(numberMatch) != 2 {
		return
	}
	result.Publisher = "RFC Editor"
	details["identifier"] = "RFC " + numberMatch[1]
	if date := monthYearPattern.FindString(content); date != "" {
		result.PublishedAt = parseBibliographicDate(date)
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	abstractIndex := -1
	for index, line := range lines {
		if rfcAbstractPattern.MatchString(strings.TrimSpace(line)) {
			abstractIndex = index
			break
		}
	}
	if abstractIndex < 0 {
		return
	}
	end := abstractIndex - 1
	for end >= 0 && strings.TrimSpace(strings.Trim(lines[end], "\f")) == "" {
		end--
	}
	start := end
	for start >= 0 && strings.TrimSpace(strings.Trim(lines[start], "\f")) != "" {
		start--
	}
	parts := make([]string, 0, end-start)
	for index := start + 1; index <= end; index++ {
		line := cleanMetadataText(strings.Trim(lines[index], "\f"))
		if line != "" {
			parts = append(parts, line)
		}
	}
	title := strings.Join(parts, " ")
	if title != "" && len([]rune(title)) <= 500 {
		result.Title = title
	}
}

func htmlAttributes(tag string) map[string]string {
	result := map[string]string{}
	for _, match := range htmlAttributePattern.FindAllStringSubmatch(tag, -1) {
		if len(match) != 5 {
			continue
		}
		value := firstNonEmpty(match[2], match[3], match[4])
		result[strings.ToLower(match[1])] = html.UnescapeString(value)
	}
	return result
}

func firstHTMLText(content string, pattern *regexp.Regexp) string {
	match := pattern.FindStringSubmatch(content)
	if len(match) != 2 {
		return ""
	}
	return cleanMetadataText(htmlMarkupPattern.ReplaceAllString(match[1], " "))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if values[key] != "" {
			return values[key]
		}
	}
	return ""
}

func cleanMetadataText(value string) string {
	value = html.UnescapeString(strings.TrimSpace(value))
	value = strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
	return boundedRunes(value, 500)
}

func parseBibliographicDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339, time.RFC3339Nano, time.RFC1123, time.RFC1123Z,
		"2006-01-02", "2006/01/02", "January 2, 2006", "2 January 2006", "January 2006",
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}
