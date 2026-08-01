// Package dataset implements queryable structured records and saved views.
package dataset

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound          = errors.New("dataset: not found")
	ErrRecordNotFound    = errors.New("dataset: record not found")
	ErrViewNotFound      = errors.New("dataset: view not found")
	ErrDuplicateName     = errors.New("dataset: duplicate name")
	ErrInvalidDefinition = errors.New("dataset: invalid definition")
	ErrInvalidRecord     = errors.New("dataset: invalid record")
	ErrInvalidView       = errors.New("dataset: invalid view")
	ErrInvalidCursor     = errors.New("dataset: invalid cursor")
)

const (
	ViewTable = "table"
	ViewCards = "cards"
	ViewChart = "chart"
)

type Dataset struct {
	ID        uuid.UUID       `json:"id"`
	WikiID    uuid.UUID       `json:"wiki_id"`
	Name      string          `json:"name"`
	Schema    json.RawMessage `json:"schema"`
	CreatedBy uuid.UUID       `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
}

type Record struct {
	ID        uuid.UUID       `json:"id"`
	DatasetID uuid.UUID       `json:"dataset_id"`
	EntityID  *uuid.UUID      `json:"entity_id"`
	Values    json.RawMessage `json:"values"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type View struct {
	ID        uuid.UUID       `json:"id"`
	DatasetID uuid.UUID       `json:"dataset_id"`
	ViewType  string          `json:"view_type"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	CreatedBy uuid.UUID       `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
}

type ViewFilter struct {
	Field    string          `json:"field"`
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value,omitempty"`
}

type ViewSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type ViewConfig struct {
	Columns []string    `json:"columns,omitempty"`
	Filter  *ViewFilter `json:"filter,omitempty"`
	Sort    *ViewSort   `json:"sort,omitempty"`
	GroupBy string      `json:"group_by,omitempty"`
}

type GroupCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type DatasetPage struct {
	Items      []Dataset `json:"items"`
	NextCursor *string   `json:"next_cursor"`
}

type RecordPage struct {
	Items      []Record     `json:"items"`
	NextCursor *string      `json:"next_cursor"`
	Groups     []GroupCount `json:"groups"`
}

type ViewPage struct {
	Items []View `json:"items"`
}

type CreateDatasetParams struct {
	WikiID  uuid.UUID
	Name    string
	Schema  json.RawMessage
	ActorID uuid.UUID
}

type CreateRecordParams struct {
	WikiID    uuid.UUID
	DatasetID uuid.UUID
	EntityID  *uuid.UUID
	Values    json.RawMessage
	ActorID   uuid.UUID
}

type CreateViewParams struct {
	WikiID    uuid.UUID
	DatasetID uuid.UUID
	ViewType  string
	Name      string
	Config    json.RawMessage
	ActorID   uuid.UUID
}

type querySpec struct {
	Filter      *ViewFilter
	Sort        *ViewSort
	SortNumeric bool
	GroupBy     string
	Offset      int
	Limit       int
}
