package component

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	StatusDraft      = "draft"
	StatusPublished  = "published"
	StatusDeprecated = "deprecated"
)

var (
	ErrNotFound           = errors.New("component: not found")
	ErrVersionNotFound    = errors.New("component: version not found")
	ErrDuplicateKey       = errors.New("component: duplicate key")
	ErrInvalidDefinition  = errors.New("component: invalid definition")
	ErrInvalidProps       = errors.New("component: invalid props")
	ErrRendererNotFound   = errors.New("component: renderer not found")
	ErrRendererRegistered = errors.New("component: renderer already registered")
	ErrVersionFrozen      = errors.New("component: version is frozen")
	ErrInvalidTransition  = errors.New("component: invalid version transition")
)

type Component struct {
	ID           uuid.UUID
	ComponentKey string
	Name         string
	CreatedBy    uuid.UUID
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Version struct {
	ComponentID uuid.UUID
	Version     int
	PropsSchema json.RawMessage
	RendererRef string
	Status      string
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	PublishedAt *time.Time
}

type CreateParams struct {
	ComponentKey string
	Name         string
	ActorID      uuid.UUID
}

type CreateVersionParams struct {
	ComponentID uuid.UUID
	PropsSchema json.RawMessage
	RendererRef string
	ActorID     uuid.UUID
}
