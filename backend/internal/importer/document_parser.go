package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/evidence"
)

const deterministicParserVersion = "2"

type ParseRequest struct {
	Filename    string
	MIMEType    string
	Content     []byte
	Provider    string
	Model       string
	ImportJobID *uuid.UUID
	ImportRunID *uuid.UUID
}

type ParseMetadata struct {
	Adapter        string   `json:"adapter"`
	AdapterVersion string   `json:"adapter_version"`
	Track          string   `json:"track"`
	MIMEType       string   `json:"mime_type"`
	PageCount      int      `json:"page_count,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	PromptKey      string   `json:"prompt_key,omitempty"`
	PromptVersion  int      `json:"prompt_version,omitempty"`
	Warnings       []string `json:"warnings"`
}

type ParseResult struct {
	Track             string
	ParserName        string
	ParserVersion     string
	ParserFingerprint string
	Metadata          ParseMetadata
	Chunks            []evidence.ChunkInput
}

type ParseArtifact struct {
	SchemaVersion int                   `json:"schema_version"`
	Metadata      ParseMetadata         `json:"metadata"`
	Chunks        []evidence.ChunkInput `json:"chunks"`
}

func (r *ParseResult) MetadataJSON() json.RawMessage {
	encoded, _ := json.Marshal(r.Metadata)
	return encoded
}

func (r *ParseResult) ArtifactJSON() ([]byte, error) {
	return json.Marshal(ParseArtifact{
		SchemaVersion: 1,
		Metadata:      r.Metadata,
		Chunks:        r.Chunks,
	})
}

type TextBlock struct {
	Kind        string
	Title       string
	Text        string
	Page        *int32
	PageEnd     *int32
	SectionPath []string
	ImageRegion *evidence.ImageRegion
	OCR         *evidence.OCRInfo
	Content     json.RawMessage
}

type adapterResult struct {
	Track    string
	Metadata ParseMetadata
	Blocks   []TextBlock
}

type DocumentParseAdapter interface {
	Name() string
	Version() string
	Supports(mimeType string) bool
	Parse(context.Context, ParseRequest) (*adapterResult, error)
}

type Parser struct {
	MaxChunkRunes int
	generator     StructuredGenerator
	adapters      []DocumentParseAdapter
}

func NewParser(maxChunkRunes int, generator StructuredGenerator) *Parser {
	if maxChunkRunes <= 0 {
		maxChunkRunes = DefaultSourceChunkCharacters
	}
	return &Parser{
		MaxChunkRunes: maxChunkRunes,
		generator:     generator,
		adapters: []DocumentParseAdapter{
			&visionPDFAdapter{generator: generator},
			deterministicDocumentAdapter{},
		},
	}
}

func (p *Parser) Parse(ctx context.Context, request ParseRequest) (*ParseResult, error) {
	for _, adapter := range p.adapters {
		if !adapter.Supports(request.MIMEType) {
			continue
		}
		parsed, err := adapter.Parse(ctx, request)
		if err != nil {
			return nil, err
		}
		if parsed == nil || !evidence.IsValidParseTrack(parsed.Track) {
			return nil, ErrParseFailed
		}
		chunks := p.chunk(parsed.Blocks)
		if len(chunks) == 0 {
			return nil, ErrParseFailed
		}
		fingerprint := parserFingerprint(
			adapter.Name(), adapter.Version(), parsed.Track,
			request.MIMEType, parsed.Metadata.Provider, parsed.Metadata.Model,
			parsed.Metadata.PromptKey,
			parsed.Metadata.PromptVersion, p.MaxChunkRunes,
		)
		parsed.Metadata.Adapter = adapter.Name()
		parsed.Metadata.AdapterVersion = adapter.Version()
		parsed.Metadata.Track = parsed.Track
		parsed.Metadata.MIMEType = request.MIMEType
		if parsed.Metadata.Warnings == nil {
			parsed.Metadata.Warnings = []string{}
		}
		return &ParseResult{
			Track:             parsed.Track,
			ParserName:        adapter.Name(),
			ParserVersion:     adapter.Version(),
			ParserFingerprint: fingerprint,
			Metadata:          parsed.Metadata,
			Chunks:            chunks,
		}, nil
	}
	return nil, ErrUnsupportedMIME
}

func parserFingerprint(
	name, version, track, mimeType, provider, model, promptKey string,
	promptVersion int,
	maxChunkRunes int,
) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		name,
		version,
		track,
		strings.TrimSpace(mimeType),
		strings.TrimSpace(provider),
		strings.TrimSpace(model),
		strings.TrimSpace(promptKey),
		fmt.Sprintf("%d", promptVersion),
		fmt.Sprintf("%d", maxChunkRunes),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (p *Parser) chunk(blocks []TextBlock) []evidence.ChunkInput {
	result := []evidence.ChunkInput{}
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		chunkType := block.Kind
		if !evidence.IsValidSourceChunkType(chunkType) {
			chunkType = evidence.SourceChunkTypeText
		}
		content := block.Content
		if len(content) == 0 {
			content, _ = json.Marshal(map[string]any{
				"title": block.Title,
			})
		}
		runes := []rune(text)
		for start := 0; start < len(runes); {
			end := semanticChunkEnd(runes, start, p.MaxChunkRunes)
			charStart, charEnd := int32(start), int32(end)
			locator := evidence.Locator{
				Page: block.Page, PageEnd: block.PageEnd,
				SectionPath: append([]string(nil), block.SectionPath...),
				CharStart:   &charStart, CharEnd: &charEnd,
				ImageRegion: block.ImageRegion, OCR: block.OCR,
			}
			result = append(result, evidence.ChunkInput{
				Ordinal: len(result), ChunkType: chunkType, Locator: locator,
				TextContent: string(runes[start:end]), Content: content,
			})
			start = end
		}
	}
	return result
}

// semanticChunkEnd keeps the configured size as a hard upper bound while
// preferring a nearby paragraph, line, sentence, or word boundary.
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

type deterministicDocumentAdapter struct{}

func (deterministicDocumentAdapter) Name() string    { return "anby-deterministic" }
func (deterministicDocumentAdapter) Version() string { return deterministicParserVersion }
func (deterministicDocumentAdapter) Supports(mimeType string) bool {
	return mimeType != "application/pdf"
}

func (deterministicDocumentAdapter) Parse(
	ctx context.Context,
	request ParseRequest,
) (*adapterResult, error) {
	var blocks []TextBlock
	var err error
	switch request.MIMEType {
	case "text/html":
		blocks, err = parseHTML(request.Content)
	case "text/plain":
		blocks = []TextBlock{{Kind: evidence.SourceChunkTypeText, Text: string(request.Content)}}
	case "application/json":
		blocks, err = parseJSON(request.Content)
	case "text/csv":
		blocks, err = parseCSV(request.Content)
	case "image/png", "image/jpeg":
		blocks, err = parseImageOCR(ctx, request.Content, nil)
	default:
		return nil, ErrUnsupportedMIME
	}
	if err != nil {
		return nil, err
	}
	return &adapterResult{
		Track: evidence.ParseTrackText,
		Metadata: ParseMetadata{
			Warnings: []string{},
		},
		Blocks: blocks,
	}, nil
}
