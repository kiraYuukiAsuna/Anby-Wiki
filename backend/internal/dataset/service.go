package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

type fieldDefinition struct {
	Type     string
	Format   string
	Title    string
	Required bool
}

func compileRecordSchema(raw json.RawMessage) (*jsonschema.Schema, map[string]fieldDefinition, json.RawMessage, error) {
	var definition struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
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
	required := make(map[string]bool, len(definition.Required))
	for _, name := range definition.Required {
		required[name] = true
	}
	fields := make(map[string]fieldDefinition, len(definition.Properties))
	for name, property := range definition.Properties {
		var value struct {
			Type   string `json:"type"`
			Format string `json:"format"`
			Title  string `json:"title"`
		}
		if json.Unmarshal(property, &value) == nil {
			fields[name] = fieldDefinition{
				Type: value.Type, Format: value.Format, Title: value.Title,
				Required: required[name],
			}
		}
	}
	canonical, err := json.Marshal(json.RawMessage(raw))
	if err != nil {
		return nil, nil, nil, ErrInvalidDefinition
	}
	return schema, fields, canonical, nil
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

func validateViewFilter(
	filter *ViewFilter, fields map[string]fieldDefinition, depth int,
) error {
	if filter == nil {
		return nil
	}
	if depth > 3 {
		return ErrInvalidView
	}
	if filter.Logic != "" {
		filter.Logic = strings.ToLower(strings.TrimSpace(filter.Logic))
		if (filter.Logic != "and" && filter.Logic != "or") ||
			filter.Field != "" || filter.Operator != "" || len(filter.Value) > 0 ||
			len(filter.Conditions) == 0 || len(filter.Conditions) > 20 {
			return ErrInvalidView
		}
		for index := range filter.Conditions {
			if err := validateViewFilter(&filter.Conditions[index], fields, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if len(filter.Conditions) > 0 {
		return ErrInvalidView
	}
	field, ok := fields[filter.Field]
	if !ok || filter.Field == "" {
		return ErrInvalidView
	}
	filter.Operator = strings.ToLower(strings.TrimSpace(filter.Operator))
	switch filter.Operator {
	case "exists", "not_exists", "is_empty", "not_empty":
		if len(filter.Value) > 0 {
			return ErrInvalidView
		}
	case "eq", "neq":
		if len(filter.Value) == 0 || !json.Valid(filter.Value) {
			return ErrInvalidView
		}
	case "contains", "not_contains":
		var value string
		if field.Type != "string" || json.Unmarshal(filter.Value, &value) != nil {
			return ErrInvalidView
		}
	case "gt", "gte", "lt", "lte":
		var value float64
		if (field.Type != "number" && field.Type != "integer") ||
			json.Unmarshal(filter.Value, &value) != nil {
			return ErrInvalidView
		}
	case "before", "after", "on_or_before", "on_or_after":
		var value string
		if field.Type != "string" ||
			(field.Format != "date" && field.Format != "date-time") ||
			json.Unmarshal(filter.Value, &value) != nil || value == "" {
			return ErrInvalidView
		}
		layout := "2006-01-02"
		if field.Format == "date-time" {
			layout = time.RFC3339
		}
		if _, err := time.Parse(layout, value); err != nil {
			return ErrInvalidView
		}
	default:
		return ErrInvalidView
	}
	return nil
}

func effectiveViewSorts(config ViewConfig) []ViewSort {
	if len(config.Sorts) > 0 {
		return config.Sorts
	}
	if config.Sort != nil {
		return []ViewSort{*config.Sort}
	}
	return nil
}

func effectiveViewGroups(config ViewConfig) []ViewGroup {
	if len(config.Groups) > 0 {
		return config.Groups
	}
	if config.GroupBy != "" {
		return []ViewGroup{{Field: config.GroupBy, Display: "expanded", SortDirection: "asc"}}
	}
	return nil
}

func validViewType(value string) bool {
	switch value {
	case ViewTable, ViewCards, ViewChart, ViewBoard, ViewTimeline, ViewCalendar,
		ViewList, ViewGallery, ViewForm, ViewDashboard:
		return true
	default:
		return false
	}
}

func parseViewConfig(
	raw json.RawMessage, fields map[string]fieldDefinition, viewType string,
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
	if databaseViewType(viewType) == viewType {
		if config.ViewMode != "" {
			return config, nil, ErrInvalidView
		}
	} else {
		if config.ViewMode != "" && config.ViewMode != viewType {
			return config, nil, ErrInvalidView
		}
		config.ViewMode = viewType
	}
	validField := func(field string) bool {
		_, ok := fields[field]
		return field != "" && ok
	}
	seen := map[string]bool{}
	for _, field := range config.Columns {
		if !validField(field) || seen[field] {
			return config, nil, ErrInvalidView
		}
		seen[field] = true
	}
	if err := validateViewFilter(config.Filter, fields, 0); err != nil {
		return config, nil, err
	}
	if config.Sort != nil && len(config.Sorts) > 0 {
		return config, nil, ErrInvalidView
	}
	sorts := effectiveViewSorts(config)
	if len(sorts) > 5 {
		return config, nil, ErrInvalidView
	}
	seen = map[string]bool{}
	for index := range sorts {
		sort := &sorts[index]
		sort.Direction = strings.ToLower(strings.TrimSpace(sort.Direction))
		if !validField(sort.Field) || seen[sort.Field] ||
			(sort.Direction != "asc" && sort.Direction != "desc") {
			return config, nil, ErrInvalidView
		}
		seen[sort.Field] = true
	}
	if config.Sort != nil {
		config.Sort = &sorts[0]
	} else {
		config.Sorts = sorts
	}
	if config.GroupBy != "" && len(config.Groups) > 0 {
		return config, nil, ErrInvalidView
	}
	groups := effectiveViewGroups(config)
	if len(groups) > 2 {
		return config, nil, ErrInvalidView
	}
	seen = map[string]bool{}
	for index := range groups {
		group := &groups[index]
		if !validField(group.Field) || seen[group.Field] {
			return config, nil, ErrInvalidView
		}
		seen[group.Field] = true
		if group.Display == "" {
			group.Display = "expanded"
		}
		if group.Display != "expanded" && group.Display != "collapsed" {
			return config, nil, ErrInvalidView
		}
		if group.SortDirection == "" {
			group.SortDirection = "asc"
		}
		group.SortDirection = strings.ToLower(strings.TrimSpace(group.SortDirection))
		if group.SortDirection != "asc" && group.SortDirection != "desc" {
			return config, nil, ErrInvalidView
		}
		for _, option := range group.Options {
			if len(option) == 0 || !json.Valid(option) {
				return config, nil, ErrInvalidView
			}
		}
	}
	if config.GroupBy != "" {
		config.GroupBy = groups[0].Field
	} else {
		config.Groups = groups
	}
	for _, field := range []string{
		config.TitleField, config.DescriptionField, config.CoverField,
		config.DateField, config.EndDateField,
	} {
		if field != "" && !validField(field) {
			return config, nil, ErrInvalidView
		}
	}
	if config.DateField != "" {
		field := fields[config.DateField]
		if field.Type != "string" || (field.Format != "date" && field.Format != "date-time") {
			return config, nil, ErrInvalidView
		}
	}
	if config.EndDateField != "" {
		field := fields[config.EndDateField]
		if field.Type != "string" || (field.Format != "date" && field.Format != "date-time") {
			return config, nil, ErrInvalidView
		}
	}
	if viewType == ViewGallery {
		if config.CardSize == "" {
			config.CardSize = "medium"
		}
		if config.CardSize != "small" && config.CardSize != "medium" && config.CardSize != "large" {
			return config, nil, ErrInvalidView
		}
	} else if config.CardSize != "" {
		return config, nil, ErrInvalidView
	}
	seen = map[string]bool{}
	for _, field := range config.FormFields {
		if !validField(field) || seen[field] {
			return config, nil, ErrInvalidView
		}
		seen[field] = true
	}
	seen = map[string]bool{}
	for _, field := range config.GlobalFilterFields {
		if !validField(field) || seen[field] {
			return config, nil, ErrInvalidView
		}
		seen[field] = true
	}
	switch viewType {
	case ViewBoard:
		if len(groups) == 0 {
			return config, nil, ErrInvalidView
		}
	case ViewChart:
		if len(groups) == 0 {
			return config, nil, ErrInvalidView
		}
	case ViewTimeline, ViewCalendar:
		if config.DateField == "" {
			return config, nil, ErrInvalidView
		}
	case ViewForm:
		if len(config.FormFields) == 0 {
			config.FormFields = append([]string(nil), config.Columns...)
		}
		if len(config.FormFields) == 0 {
			return config, nil, ErrInvalidView
		}
		formFields := make(map[string]bool, len(config.FormFields))
		for _, field := range config.FormFields {
			formFields[field] = true
		}
		for field, definition := range fields {
			if definition.Required && !formFields[field] {
				return config, nil, ErrInvalidView
			}
		}
		if strings.TrimSpace(config.SubmitLabel) == "" {
			config.SubmitLabel = "提交"
		}
		if strings.TrimSpace(config.SuccessMessage) == "" {
			config.SuccessMessage = "提交成功"
		}
	case ViewDashboard:
		if len(config.ViewIDs) == 0 || len(config.ViewIDs) > 12 {
			return config, nil, ErrInvalidView
		}
		if config.LayoutColumns == 0 {
			config.LayoutColumns = 2
		}
		if config.LayoutColumns < 1 || config.LayoutColumns > 3 {
			return config, nil, ErrInvalidView
		}
		viewIDs := make(map[uuid.UUID]bool, len(config.ViewIDs))
		for _, viewID := range config.ViewIDs {
			if viewID == uuid.Nil || viewIDs[viewID] {
				return config, nil, ErrInvalidView
			}
			viewIDs[viewID] = true
		}
	default:
		if config.LayoutColumns != 0 {
			return config, nil, ErrInvalidView
		}
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
	if name == "" || !validViewType(params.ViewType) {
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
		_, fields, _, err := compileRecordSchema(definition.Schema)
		if err != nil {
			return err
		}
		configValue, config, err := parseViewConfig(params.Config, fields, params.ViewType)
		if err != nil {
			return err
		}
		if params.ViewType == ViewDashboard {
			for _, childID := range configValue.ViewIDs {
				child, err := s.repo.GetView(ctx, tx, childID)
				if err != nil || child.DatasetID != params.DatasetID ||
					child.ViewType == ViewDashboard {
					return ErrInvalidView
				}
			}
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
	view, err := s.repo.GetView(ctx, nil, viewID)
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
	runtimeFilter *ViewFilter,
) (*RecordPage, error) {
	view, err := s.GetView(ctx, wikiID, viewID)
	if err != nil {
		return nil, err
	}
	definition, err := s.repo.GetDataset(ctx, nil, view.DatasetID)
	if err != nil {
		return nil, err
	}
	_, fields, _, err := compileRecordSchema(definition.Schema)
	if err != nil {
		return nil, err
	}
	config, _, err := parseViewConfig(view.Config, fields, view.ViewType)
	if err != nil {
		return nil, err
	}
	if err := validateViewFilter(runtimeFilter, fields, 0); err != nil {
		return nil, err
	}
	filter := config.Filter
	if runtimeFilter != nil {
		if filter == nil {
			filter = runtimeFilter
		} else {
			filter = &ViewFilter{
				Logic: "and", Conditions: []ViewFilter{*filter, *runtimeFilter},
			}
		}
	}
	sorts := effectiveViewSorts(config)
	querySorts := make([]querySort, 0, len(sorts))
	for _, sort := range sorts {
		field := fields[sort.Field]
		querySorts = append(querySorts, querySort{
			ViewSort: sort,
			Numeric:  field.Type == "number" || field.Type == "integer",
		})
	}
	offset, err := decodeOffsetCursor(cursor)
	if err != nil {
		return nil, err
	}
	return s.repo.QueryRecords(ctx, view.DatasetID, querySpec{
		Filter: filter, Sorts: querySorts, Groups: effectiveViewGroups(config),
		Offset: offset, Limit: normalizeLimit(limit),
	})
}

func (s *Service) GetPublicForm(
	ctx context.Context, wikiID, viewID uuid.UUID,
) (*PublicForm, error) {
	view, err := s.GetView(ctx, wikiID, viewID)
	if err != nil {
		return nil, err
	}
	if view.ViewType != ViewForm {
		return nil, ErrViewNotFound
	}
	definition, err := s.repo.GetDataset(ctx, nil, view.DatasetID)
	if err != nil {
		return nil, err
	}
	_, fields, _, err := compileRecordSchema(definition.Schema)
	if err != nil {
		return nil, err
	}
	config, _, err := parseViewConfig(view.Config, fields, view.ViewType)
	if err != nil || !config.Public {
		return nil, ErrViewNotFound
	}
	result := &PublicForm{
		ID: view.ID, DatasetID: view.DatasetID, Name: view.Name,
		SubmitLabel: config.SubmitLabel, SuccessMessage: config.SuccessMessage,
		Fields: make([]PublicFormField, 0, len(config.FormFields)),
	}
	for _, key := range config.FormFields {
		field := fields[key]
		title := strings.TrimSpace(field.Title)
		if title == "" {
			title = key
		}
		result.Fields = append(result.Fields, PublicFormField{
			Key: key, Title: title, Type: field.Type, Format: field.Format,
			Required: field.Required,
		})
	}
	return result, nil
}

func (s *Service) SubmitPublicForm(
	ctx context.Context, wikiID, viewID uuid.UUID, raw json.RawMessage,
) (*Record, error) {
	var result *Record
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		view, err := s.repo.GetView(ctx, tx, viewID)
		if err != nil || view.ViewType != ViewForm {
			return ErrViewNotFound
		}
		definition, err := s.repo.GetDataset(ctx, tx, view.DatasetID)
		if err != nil || definition.WikiID != wikiID {
			return ErrViewNotFound
		}
		schema, fields, _, err := compileRecordSchema(definition.Schema)
		if err != nil {
			return err
		}
		config, _, err := parseViewConfig(view.Config, fields, view.ViewType)
		if err != nil || !config.Public {
			return ErrViewNotFound
		}
		var submitted map[string]json.RawMessage
		if json.Unmarshal(raw, &submitted) != nil || submitted == nil {
			return ErrInvalidRecord
		}
		allowed := make(map[string]bool, len(config.FormFields))
		for _, field := range config.FormFields {
			allowed[field] = true
		}
		for field := range submitted {
			if !allowed[field] {
				return ErrInvalidRecord
			}
		}
		values, err := validateRecord(schema, raw)
		if err != nil {
			return err
		}
		recordID, err := s.ids.New()
		if err != nil {
			return err
		}
		result = &Record{ID: recordID, DatasetID: view.DatasetID, Values: values}
		return s.repo.InsertRecord(ctx, tx, result)
	})
	return result, err
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
