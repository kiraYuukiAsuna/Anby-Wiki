package component

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var componentKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

const (
	DefaultPageSize = 24
	MaxPageSize     = 100
)

type Service struct {
	repo     *Repository
	actors   *page.Repository
	txm      *db.TxManager
	ids      *id.Generator
	registry *Registry
}

func NewService(
	repo *Repository,
	actors *page.Repository,
	txm *db.TxManager,
	ids *id.Generator,
	registry *Registry,
) *Service {
	return &Service{
		repo: repo, actors: actors, txm: txm, ids: ids, registry: registry,
	}
}

func (s *Service) Create(ctx context.Context, params CreateParams) (*Component, error) {
	key := strings.ToLower(strings.TrimSpace(params.ComponentKey))
	name := strings.TrimSpace(params.Name)
	if !componentKeyPattern.MatchString(key) || name == "" {
		return nil, ErrInvalidDefinition
	}
	var value *Component
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		componentID, err := s.ids.New()
		if err != nil {
			return err
		}
		value = &Component{
			ID: componentID, ComponentKey: key, Name: name, CreatedBy: params.ActorID,
		}
		return s.repo.Insert(ctx, tx, value)
	})
	return value, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Component, error) {
	return s.repo.Get(ctx, nil, id)
}

func (s *Service) List(
	ctx context.Context, cursor string, limit int,
) (*ComponentPage, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	return s.repo.List(ctx, cursor, limit)
}

func (s *Service) ListVersions(
	ctx context.Context, componentID uuid.UUID,
) (*VersionList, error) {
	return s.repo.ListVersions(ctx, componentID)
}

func (s *Service) CreateVersion(
	ctx context.Context, params CreateVersionParams,
) (*Version, error) {
	schema, err := compilePropsSchema(params.PropsSchema)
	if err != nil {
		return nil, err
	}
	_ = schema
	rendererRef := strings.TrimSpace(params.RendererRef)
	if s.registry == nil || !s.registry.Has(rendererRef) {
		return nil, fmt.Errorf("%w: %s", ErrRendererNotFound, rendererRef)
	}

	var value *Version
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		if _, err := s.repo.Lock(ctx, tx, params.ComponentID); err != nil {
			return err
		}
		version, err := s.repo.NextVersion(ctx, tx, params.ComponentID)
		if err != nil {
			return err
		}
		value = &Version{
			ComponentID: params.ComponentID, Version: version,
			PropsSchema: append(json.RawMessage(nil), params.PropsSchema...),
			RendererRef: rendererRef, Status: StatusDraft, CreatedBy: params.ActorID,
		}
		return s.repo.InsertVersion(ctx, tx, value)
	})
	return value, err
}

func (s *Service) UpdateDraft(
	ctx context.Context,
	componentID uuid.UUID,
	version int,
	propsSchema json.RawMessage,
	rendererRef string,
	actorID uuid.UUID,
) (*Version, error) {
	if _, err := compilePropsSchema(propsSchema); err != nil {
		return nil, err
	}
	rendererRef = strings.TrimSpace(rendererRef)
	if s.registry == nil || !s.registry.Has(rendererRef) {
		return nil, fmt.Errorf("%w: %s", ErrRendererNotFound, rendererRef)
	}
	var value *Version
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		current, err := s.repo.LockVersion(ctx, tx, componentID, version)
		if err != nil {
			return err
		}
		if current.Status != StatusDraft {
			return ErrVersionFrozen
		}
		if err := s.repo.UpdateDraft(
			ctx, tx, componentID, version, propsSchema, rendererRef,
		); err != nil {
			return err
		}
		value, err = s.repo.GetVersion(ctx, tx, componentID, version)
		return err
	})
	return value, err
}

func (s *Service) Publish(
	ctx context.Context, componentID uuid.UUID, version int, actorID uuid.UUID,
) (*Version, error) {
	return s.transition(ctx, componentID, version, actorID, StatusPublished)
}

func (s *Service) Deprecate(
	ctx context.Context, componentID uuid.UUID, version int, actorID uuid.UUID,
) (*Version, error) {
	return s.transition(ctx, componentID, version, actorID, StatusDeprecated)
}

func (s *Service) transition(
	ctx context.Context,
	componentID uuid.UUID,
	version int,
	actorID uuid.UUID,
	target string,
) (*Version, error) {
	var value *Version
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		current, err := s.repo.LockVersion(ctx, tx, componentID, version)
		if err != nil {
			return err
		}
		if target == StatusPublished {
			if current.Status != StatusDraft {
				return ErrInvalidTransition
			}
			if _, err := compilePropsSchema(current.PropsSchema); err != nil {
				return err
			}
			if s.registry == nil || !s.registry.Has(current.RendererRef) {
				return fmt.Errorf("%w: %s", ErrRendererNotFound, current.RendererRef)
			}
		} else if target != StatusDeprecated || current.Status != StatusPublished {
			return ErrInvalidTransition
		}
		if err := s.repo.SetStatus(ctx, tx, componentID, version, target); err != nil {
			return err
		}
		value, err = s.repo.GetVersion(ctx, tx, componentID, version)
		return err
	})
	return value, err
}

func (s *Service) ValidateProps(
	ctx context.Context,
	componentID uuid.UUID,
	version int,
	props json.RawMessage,
) (*Version, error) {
	value, err := s.repo.GetVersion(ctx, nil, componentID, version)
	if err != nil {
		return nil, err
	}
	if value.Status != StatusPublished && value.Status != StatusDeprecated {
		return nil, ErrVersionFrozen
	}
	if err := validatePropsInstance(value.PropsSchema, props); err != nil {
		return nil, err
	}
	return value, nil
}

func validatePropsInstance(schemaJSON, props json.RawMessage) error {
	schema, err := compilePropsSchema(schemaJSON)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(props))
	if err != nil {
		return fmt.Errorf("%w: invalid JSON", ErrInvalidProps)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProps, err)
	}
	return nil
}

// Preview validates a draft or frozen version without changing its lifecycle.
// Renderer refs still resolve only through the trusted in-process registry.
func (s *Service) Preview(
	ctx context.Context,
	componentID uuid.UUID,
	version int,
	entityID *uuid.UUID,
	props json.RawMessage,
) (string, error) {
	value, err := s.repo.GetVersion(ctx, nil, componentID, version)
	if err != nil {
		return "", err
	}
	if err := validatePropsInstance(value.PropsSchema, props); err != nil {
		return "", err
	}
	if entityID != nil {
		return s.registry.RenderEntity(ctx, value.RendererRef, *entityID, props)
	}
	return s.registry.Render(ctx, value.RendererRef, props)
}

func (s *Service) Render(
	ctx context.Context,
	componentID uuid.UUID,
	version int,
	props json.RawMessage,
) (string, error) {
	value, err := s.ValidateProps(ctx, componentID, version, props)
	if err != nil {
		return "", err
	}
	return s.registry.Render(ctx, value.RendererRef, props)
}

// RenderEntity 校验 ComponentBlock.display_config 后，以稳定 Entity ID 调用可信渲染器。
func (s *Service) RenderEntity(
	ctx context.Context,
	componentID uuid.UUID,
	version int,
	entityID uuid.UUID,
	displayConfig json.RawMessage,
) (string, error) {
	value, err := s.ValidateProps(ctx, componentID, version, displayConfig)
	if err != nil {
		return "", err
	}
	return s.registry.RenderEntity(ctx, value.RendererRef, entityID, displayConfig)
}

func compilePropsSchema(raw json.RawMessage) (*jsonschema.Schema, error) {
	var definition map[string]any
	if err := json.Unmarshal(raw, &definition); err != nil || definition["type"] != "object" {
		return nil, fmt.Errorf("%w: props schema root type must be object", ErrInvalidDefinition)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}
	compiler := jsonschema.NewCompiler()
	const schemaURL = "https://anby.wiki/components/props.schema.json"
	if err := compiler.AddResource(schemaURL, document); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}
	return schema, nil
}
