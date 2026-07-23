package collection

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	TypeManual = "manual"
	TypeRule   = "rule"

	MemberPage   = "page"
	MemberEntity = "entity"

	RuleVersion = 1
)

var (
	ErrNotFound          = errors.New("collection: not found")
	ErrInvalidDefinition = errors.New("collection: invalid definition")
	ErrInvalidRule       = errors.New("collection: invalid rule")
	ErrInvalidMember     = errors.New("collection: invalid member")
	ErrInvalidCursor     = errors.New("collection: invalid cursor")
)

type Collection struct {
	ID                uuid.UUID
	WikiID            uuid.UUID
	CollectionType    string
	Title             string
	DescriptionPageID *uuid.UUID
	QueryJSON         json.RawMessage
	CreatedBy         uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Rule struct {
	Version    int    `json:"version"`
	Kind       string `json:"kind"`
	EntityType string `json:"entity_type,omitempty"`
	Property   string `json:"property,omitempty"`
}

type CreateParams struct {
	WikiID            uuid.UUID
	CollectionType    string
	Title             string
	DescriptionPageID *uuid.UUID
	Rule              json.RawMessage
	ActorID           uuid.UUID
}

type MemberInput struct {
	MemberType       string
	PageID           *uuid.UUID
	EntityID         *uuid.UUID
	SortKey          string
	SourceRevisionID uuid.UUID
}

type Membership struct {
	CollectionID     uuid.UUID
	PageID           *uuid.UUID
	EntityID         *uuid.UUID
	MemberType       string
	SourceType       string
	SortKey          string
	SourceRevisionID uuid.UUID
	DisplayTitle     string
	CreatedAt        time.Time
}

type MembershipPage struct {
	Items      []Membership
	NextCursor *string
}

type CollectionPage struct {
	Items      []Collection
	NextCursor *string
}
