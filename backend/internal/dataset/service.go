package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

const (
	DefaultPageSize = 24
	MaxPageSize     = 100
)

type Service struct {
	repo   *Repository
	actors *page.Repository
	txm    *db.TxManager
	ids    *id.Generator
}

func NewService(
	repo *Repository, actors *page.Repository, txm *db.TxManager, ids *id.Generator,
) *Service {
	return &Service{repo: repo, actors: actors, txm: txm, ids: ids}
}

func compileRecordSchema(raw json.RawMessage) (*jsonschema.Schema, map[string]string, json.RawMessage, error) {
	var definition struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &definition); err != nil ||
		definition.Type != "object" || len(definition.Properties) == 0 {
		return nil, nil, nil, ErrInvalidDefinition
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, nil, ErrInvalidDefinition
	}
	compiler := jsonschema.NewCompiler()
	const schemaURL = "https://anby.wiki/datasets/record.schema.json"
	if err := compiler.AddResource(schemaURL, document); err != nil {
		return nil, nil, nil, ErrInvalidDefinition
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %v", ErrInvalidDefinition, err)
	}
	fieldTypes := make(map[string]string, len(definition.Properties))
	for name, property := range definition.Properties {
		var value struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(property, &value) == nil {
			fieldTypes[name] = value.Type
		}
	}
	canonical, err := json.Marshal(json.RawMessage(raw))
	if err != nil {
		return nil, nil, nil, ErrInvalidDefinition
	}
	return schema, fieldTypes, canonical, nil
}

func (s *Service) CreateDataset(
	ctx context.Context, params CreateDatasetParams,
) (*Dataset, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" || params.WikiID == uuid.Nil {
		return nil, ErrInvalidDefinition
	}
	_, _, schema, err := compileRecordSchema(params.Schema)
	if err != nil {
		return nil, err
	}
	var result *Dataset
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		datasetID, err := s.ids.New()
		if err != nil {
			return err
		}
		result = &Dataset{
			ID: datasetID, WikiID: params.WikiID, Name: name, Schema: schema,
			CreatedBy: params.ActorID,
		}
		return s.repo.InsertDataset(ctx, tx, result)
	})
	return result, err
}

func (s *Service) GetDataset(
	ctx context.Context, wikiID, datasetID uuid.UUID,
) (*Dataset, error) {
	value, err := s.repo.GetDataset(ctx, nil, datasetID)
	if err != nil {
		return nil, err
	}
	if value.WikiID != wikiID {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *Service) ListDatasets(
	ctx context.Context, wikiID uuid.UUID, cursor string, limit int,
) (*DatasetPage, error) {
	return s.repo.ListDatasets(ctx, wikiID, cursor, normalizeLimit(limit))
}

func validateRecord(schema *jsonschema.Schema, raw json.RawMessage) (json.RawMessage, error) {
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, ErrInvalidRecord
	}
	if err := schema.Validate(object); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRecord, err)
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, ErrInvalidRecord
	}
	return canonical, nil
}

func (s *Service) CreateRecord(
	ctx context.Context, params CreateRecordParams,
) (*Record, error) {
	var result *Record
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		definition, err := s.repo.GetDataset(ctx, tx, params.DatasetID)
		if err != nil {
			return err
		}
		if definition.WikiID != params.WikiID {
			return ErrNotFound
		}
		schema, _, _, err := compileRecordSchema(definition.Schema)
		if err != nil {
			return err
		}
		values, err := validateRecord(schema, params.Values)
		if err != nil {
			return err
		}
		if params.EntityID != nil {
			if err := s.repo.ValidateEntity(ctx, tx, definition.WikiID, *params.EntityID); err != nil {
				return err
			}
		}
		recordID, err := s.ids.New()
		if err != nil {
			return err
		}
		result = &Record{
			ID: recordID, DatasetID: params.DatasetID, EntityID: params.EntityID,
			Values: values,
		}
		return s.repo.InsertRecord(ctx, tx, result)
	})
	return result, err
}

func (s *Service) ListRecords(
	ctx context.Context, wikiID, datasetID uuid.UUID, cursor string, limit int,
) (*RecordPage, error) {
	if _, err := s.GetDataset(ctx, wikiID, datasetID); err != nil {
		return nil, err
	}
	offset, err := decodeOffsetCursor(cursor)
	if err != nil {
		return nil, err
	}
	return s.repo.QueryRecords(ctx, datasetID, querySpec{
		Offset: offset, Limit: normalizeLimit(limit),
	})
}

func (s *Service) UpdateRecord(
	ctx context.Context, wikiID, recordID, actorID uuid.UUID,
	entityID *uuid.UUID, raw json.RawMessage,
) (*Record, error) {
	var result *Record
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		current, err := s.repo.LockRecord(ctx, tx, recordID)
		if err != nil {
			return err
		}
		definition, err := s.repo.GetDataset(ctx, tx, current.DatasetID)
		if err != nil {
			return err
		}
		if definition.WikiID != wikiID {
			return ErrRecordNotFound
		}
		schema, _, _, err := compileRecordSchema(definition.Schema)
		if err != nil {
			return err
		}
		values, err := validateRecord(schema, raw)
		if err != nil {
			return err
		}
		if entityID != nil {
			if err := s.repo.ValidateEntity(ctx, tx, definition.WikiID, *entityID); err != nil {
				return err
			}
		}
		result, err = s.repo.UpdateRecord(ctx, tx, recordID, entityID, values)
		return err
	})
	return result, err
}

func parseViewConfig(
	raw json.RawMessage, fieldTypes map[string]string,
) (ViewConfig, json.RawMessage, error) {
	var config ViewConfig
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, nil, ErrInvalidView
	}
	validField := func(field string) bool {
		_, ok := fieldTypes[field]
		return field != "" && ok
	}
	seen := map[string]bool{}
	for _, field := range config.Columns {
		if !validField(field) || seen[field] {
			return config, nil, ErrInvalidView
		}
		seen[field] = true
	}
	if config.Filter != nil {
		if !validField(config.Filter.Field) {
			return config, nil, ErrInvalidView
		}
		switch config.Filter.Operator {
		case "exists":
		case "eq", "contains", "gt", "gte", "lt", "lte":
			if len(config.Filter.Value) == 0 {
				return config, nil, ErrInvalidView
			}
		default:
			return config, nil, ErrInvalidView
		}
		if (config.Filter.Operator == "gt" || config.Filter.Operator == "gte" ||
			config.Filter.Operator == "lt" || config.Filter.Operator == "lte") &&
			fieldTypes[config.Filter.Field] != "number" &&
			fieldTypes[config.Filter.Field] != "integer" {
			return config, nil, ErrInvalidView
		}
	}
	if config.Sort != nil {
		config.Sort.Direction = strings.ToLower(strings.TrimSpace(config.Sort.Direction))
		if !validField(config.Sort.Field) ||
			(config.Sort.Direction != "asc" && config.Sort.Direction != "desc") {
			return config, nil, ErrInvalidView
		}
	}
	if config.GroupBy != "" && !validField(config.GroupBy) {
		return config, nil, ErrInvalidView
	}
	canonical, err := json.Marshal(config)
	if err != nil {
		return config, nil, ErrInvalidView
	}
	return config, canonical, nil
}

func (s *Service) CreateView(
	ctx context.Context, params CreateViewParams,
) (*View, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" ||
		(params.ViewType != ViewTable && params.ViewType != ViewCards &&
			params.ViewType != ViewChart) {
		return nil, ErrInvalidView
	}
	var result *View
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.actors.CheckWriteActor(ctx, tx, params.ActorID); err != nil {
			return err
		}
		definition, err := s.repo.GetDataset(ctx, tx, params.DatasetID)
		if err != nil {
			return err
		}
		if definition.WikiID != params.WikiID {
			return ErrNotFound
		}
		_, fieldTypes, _, err := compileRecordSchema(definition.Schema)
		if err != nil {
			return err
		}
		_, config, err := parseViewConfig(params.Config, fieldTypes)
		if err != nil {
			return err
		}
		viewID, err := s.ids.New()
		if err != nil {
			return err
		}
		result = &View{
			ID: viewID, DatasetID: params.DatasetID, ViewType: params.ViewType,
			Name: name, Config: config, CreatedBy: params.ActorID,
		}
		return s.repo.InsertView(ctx, tx, result)
	})
	return result, err
}

func (s *Service) ListViews(
	ctx context.Context, wikiID, datasetID uuid.UUID,
) (*ViewPage, error) {
	if _, err := s.GetDataset(ctx, wikiID, datasetID); err != nil {
		return nil, err
	}
	return s.repo.ListViews(ctx, datasetID)
}

func (s *Service) GetView(
	ctx context.Context, wikiID, viewID uuid.UUID,
) (*View, error) {
	view, err := s.repo.GetView(ctx, viewID)
	if err != nil {
		return nil, err
	}
	definition, err := s.repo.GetDataset(ctx, nil, view.DatasetID)
	if err != nil || definition.WikiID != wikiID {
		return nil, ErrViewNotFound
	}
	return view, nil
}

func (s *Service) QueryView(
	ctx context.Context, wikiID, viewID uuid.UUID, cursor string, limit int,
) (*RecordPage, error) {
	view, err := s.GetView(ctx, wikiID, viewID)
	if err != nil {
		return nil, err
	}
	definition, err := s.repo.GetDataset(ctx, nil, view.DatasetID)
	if err != nil {
		return nil, err
	}
	_, fieldTypes, _, err := compileRecordSchema(definition.Schema)
	if err != nil {
		return nil, err
	}
	config, _, err := parseViewConfig(view.Config, fieldTypes)
	if err != nil {
		return nil, err
	}
	offset, err := decodeOffsetCursor(cursor)
	if err != nil {
		return nil, err
	}
	return s.repo.QueryRecords(ctx, view.DatasetID, querySpec{
		Filter: config.Filter, Sort: config.Sort,
		SortNumeric: config.Sort != nil &&
			(fieldTypes[config.Sort.Field] == "number" ||
				fieldTypes[config.Sort.Field] == "integer"),
		GroupBy: config.GroupBy, Offset: offset, Limit: normalizeLimit(limit),
	})
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultPageSize
	}
	if limit > MaxPageSize {
		return MaxPageSize
	}
	return limit
}
