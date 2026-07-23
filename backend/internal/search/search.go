// Package search defines the replaceable search boundary used by API and projections.
package search

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const (
	DefaultLimit = 10
	MaxLimit     = 50
)

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
	Limit      int
	Offset     int
}

// Hit is an adapter-neutral search result.
type Hit struct {
	PageID       uuid.UUID
	DisplayTitle string
	Namespace    string
	MatchedOn    Field
	Highlight    string
	Score        float32
}

// SearchAdapter is the replaceable search engine boundary.
type SearchAdapter interface {
	Index(context.Context, SearchDocument) error
	Delete(context.Context, uuid.UUID) error
	Search(context.Context, Query) ([]Hit, int, error)
	Rebuild(context.Context, []SearchDocument) error
}

var (
	ErrInvalidDocument   = errors.New("search: invalid document")
	ErrNamespaceNotFound = errors.New("search: namespace not found")
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
