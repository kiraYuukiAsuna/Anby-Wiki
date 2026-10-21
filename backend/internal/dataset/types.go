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
	ViewTable     = "table"
	ViewCards     = "cards"
	ViewChart     = "chart"
	ViewBoard     = "board"
	ViewTimeline  = "timeline"
	ViewCalendar  = "calendar"
	ViewList      = "list"
	ViewGallery   = "gallery"
	ViewForm      = "form"
	ViewDashboard = "dashboard"
)

func databaseViewType(viewType string) string {
	switch viewType {
	case ViewCards, ViewGallery:
		return ViewCards
	case ViewChart:
		return ViewChart
	default:
		return ViewTable
	}
}

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
	Field      string          `json:"field,omitempty"`
	Operator   string          `json:"operator,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Logic      string          `json:"logic,omitempty"`
	Conditions []ViewFilter    `json:"conditions,omitempty"`
}

type ViewSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type ViewGroup struct {
	Field         string            `json:"field"`
	Display       string            `json:"display,omitempty"`
	HideEmpty     bool              `json:"hide_empty,omitempty"`
	SortDirection string            `json:"sort_direction,omitempty"`
	Options       []json.RawMessage `json:"options,omitempty"`
}

type ViewConfig struct {
	Columns []string    `json:"columns,omitempty"`
	Filter  *ViewFilter `json:"filter,omitempty"`
	Sort    *ViewSort   `json:"sort,omitempty"`
	GroupBy string      `json:"group_by,omitempty"`
	// ViewMode preserves additive view types while the pre-release database
	// keeps its original table/cards/chart CHECK constraint.
	ViewMode string `json:"view_mode,omitempty"`

	Sorts  []ViewSort  `json:"sorts,omitempty"`
	Groups []ViewGroup `json:"groups,omitempty"`

	TitleField       string `json:"title_field,omitempty"`
	DescriptionField string `json:"description_field,omitempty"`
	CoverField       string `json:"cover_field,omitempty"`
	DateField        string `json:"date_field,omitempty"`
	EndDateField     string `json:"end_date_field,omitempty"`
	CardSize         string `json:"card_size,omitempty"`

	FormFields     []string `json:"form_fields,omitempty"`
	Public         bool     `json:"public,omitempty"`
	SubmitLabel    string   `json:"submit_label,omitempty"`
	SuccessMessage string   `json:"success_message,omitempty"`

	ViewIDs            []uuid.UUID `json:"view_ids,omitempty"`
	GlobalFilterFields []string    `json:"global_filter_fields,omitempty"`
	LayoutColumns      int         `json:"layout_columns,omitempty"`
}

type GroupCount struct {
	Value string   `json:"value"`
	Path  []string `json:"path,omitempty"`
	Count int      `json:"count"`
}

type PublicFormField struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Format   string `json:"format,omitempty"`
	Required bool   `json:"required"`
}

type PublicForm struct {
	ID             uuid.UUID         `json:"id"`
	DatasetID      uuid.UUID         `json:"dataset_id"`
	Name           string            `json:"name"`
	Fields         []PublicFormField `json:"fields"`
	SubmitLabel    string            `json:"submit_label"`
	SuccessMessage string            `json:"success_message"`
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
	Filter *ViewFilter
	Sorts  []querySort
	Groups []ViewGroup
	Offset int
	Limit  int
}

type querySort struct {
	ViewSort
	Numeric bool
}
