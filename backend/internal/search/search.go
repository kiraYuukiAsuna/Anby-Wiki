// Package search defines the replaceable search boundary used by API and projections.
package search

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/google/uuid"
)

const (
	DefaultLimit = 10
	MaxLimit     = 50
)

// Mode controls how the independent engine ranks a query.
type Mode string

const (
	ModeKeyword  Mode = "keyword"
	ModeHybrid   Mode = "hybrid"
	ModeSemantic Mode = "semantic"
)

const DefaultSemanticRatio = 0.5

// Field identifies a searchable document field.
type Field string

const (
	FieldTitle  Field = "title"
	FieldAlias  Field = "alias"
	FieldBody   Field = "body"
	FieldEntity Field = "entity"
)

// SearchDocument is the versioned projection payload indexed by an adapter.
type SearchDocument struct {
	PageID           uuid.UUID
	WikiID           uuid.UUID
	Namespace        string
	Language         string
	SourceRevisionID uuid.UUID
	DisplayTitle     string
	NormalizedTitle  string
	Aliases          []string
	Body             string
	EntityID         *uuid.UUID
	EntityType       string
	EntityTerms      []string
}

// Query combines free text with projection metadata and field filters.
type Query struct {
	Text       string
	WikiID     uuid.UUID
	Namespace  string
	Language   string
	EntityType string
	Fields     []Field
	Mode       Mode
	// SemanticRatio is used only in hybrid mode. Zero means the caller did not
	// provide a value and is normalized to DefaultSemanticRatio.
	SemanticRatio float64
	Limit         int
	Offset        int
}

// Hit is an adapter-neutral search result.
type Hit struct {
	PageID       uuid.UUID
	DisplayTitle string
	Namespace    string
	Language     string
	EntityID     *uuid.UUID
	EntityType   string
	MatchedOn    Field
	Highlight    string
	Score        float32
}

// FacetValue is an adapter-neutral aggregation bucket.
type FacetValue struct {
	Value string
	Count int
}

// Facets contains the stable aggregation dimensions exposed by the API.
type Facets struct {
	Namespaces  []FacetValue
	Languages   []FacetValue
	EntityTypes []FacetValue
}

// Result contains one result page plus aggregations for the current query.
type Result struct {
	Hits   []Hit
	Total  int
	Facets Facets
	Mode   Mode
}

// Capabilities lets clients discover whether semantic modes are actually
// configured. This prevents a keyword fallback from masquerading as semantic
// search.
type Capabilities struct {
	Backend         string
	Modes           []Mode
	DefaultMode     Mode
	DefaultRatio    float64
	AggregationKeys []string
}

// SearchAdapter is the replaceable search engine boundary.
type SearchAdapter interface {
	Index(context.Context, SearchDocument) error
	Delete(context.Context, uuid.UUID) error
	Search(context.Context, Query) (Result, error)
	Rebuild(context.Context, []SearchDocument) error
	Capabilities() Capabilities
}

var (
	ErrInvalidDocument     = errors.New("search: invalid document")
	ErrNamespaceNotFound   = errors.New("search: namespace not found")
	ErrSemanticUnavailable = errors.New("search: semantic search is not configured")
	ErrInvalidMode         = errors.New("search: invalid mode")
)

func validateDocument(doc SearchDocument) error {
	if doc.PageID == uuid.Nil || doc.WikiID == uuid.Nil || doc.SourceRevisionID == uuid.Nil ||
		strings.TrimSpace(doc.Namespace) == "" || strings.TrimSpace(doc.Language) == "" ||
		strings.TrimSpace(doc.DisplayTitle) == "" || strings.TrimSpace(doc.NormalizedTitle) == "" {
		return ErrInvalidDocument
	}
	return nil
}

func normalizeQuery(q Query) Query {
	q.Text = strings.TrimSpace(q.Text)
	q.Namespace = strings.TrimSpace(q.Namespace)
	q.Language = strings.TrimSpace(q.Language)
	q.EntityType = strings.TrimSpace(q.EntityType)
	if q.Mode == "" {
		q.Mode = ModeKeyword
	}
	switch q.Mode {
	case ModeKeyword:
		q.SemanticRatio = 0
	case ModeSemantic:
		q.SemanticRatio = 1
	case ModeHybrid:
		if q.SemanticRatio == 0 {
			q.SemanticRatio = DefaultSemanticRatio
		}
	}
	if q.Limit <= 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit > MaxLimit {
		q.Limit = MaxLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return q
}

func validateQuery(q Query) error {
	switch q.Mode {
	case ModeKeyword, ModeHybrid, ModeSemantic:
	default:
		return ErrInvalidMode
	}
	if math.IsNaN(q.SemanticRatio) || math.IsInf(q.SemanticRatio, 0) ||
		q.SemanticRatio < 0 || q.SemanticRatio > 1 {
		return ErrInvalidMode
	}
	return nil
}

func keywordCapabilities(backend string) Capabilities {
	return Capabilities{
		Backend:         backend,
		Modes:           []Mode{ModeKeyword},
		DefaultMode:     ModeKeyword,
		DefaultRatio:    DefaultSemanticRatio,
		AggregationKeys: []string{"namespace", "language", "entity_type"},
	}
}

func emptyResult(mode Mode) Result {
	return Result{
		Hits: []Hit{},
		Facets: Facets{
			Namespaces:  []FacetValue{},
			Languages:   []FacetValue{},
			EntityTypes: []FacetValue{},
		},
		Mode: mode,
	}
}

func selectedFields(fields []Field) (title, alias, body, entity bool) {
	if len(fields) == 0 {
		return true, true, true, true
	}
	for _, field := range fields {
		switch field {
		case FieldTitle:
			title = true
		case FieldAlias:
			alias = true
		case FieldBody:
			body = true
		case FieldEntity:
			entity = true
		}
	}
	return
}
