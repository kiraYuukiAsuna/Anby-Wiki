package dataset

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/anby/wiki/backend/internal/platform/db"
)

type Repository struct{ pool db.Querier }

func NewRepository(pool db.Querier) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(tx pgx.Tx) db.Querier {
	if tx != nil {
		return tx
	}
	return r.pool
}

const datasetColumns = `id,wiki_id,name,schema_json,created_by,created_at`

func scanDataset(row pgx.Row) (*Dataset, error) {
	var value Dataset
	if err := row.Scan(&value.ID, &value.WikiID, &value.Name, &value.Schema,
		&value.CreatedBy, &value.CreatedAt); err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *Repository) InsertDataset(ctx context.Context, tx pgx.Tx, value *Dataset) error {
	err := r.q(tx).QueryRow(ctx, `INSERT INTO dataset
		(id,wiki_id,name,schema_json,created_by) VALUES ($1,$2,$3,$4::jsonb,$5)
		RETURNING created_at`, value.ID, value.WikiID, value.Name, value.Schema,
		value.CreatedBy).Scan(&value.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateName
	}
	if err != nil {
		return fmt.Errorf("dataset: insert definition: %w", err)
	}
	return nil
}

func (r *Repository) GetDataset(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Dataset, error) {
	value, err := scanDataset(r.q(tx).QueryRow(ctx,
		`SELECT `+datasetColumns+` FROM dataset WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dataset: get definition: %w", err)
	}
	return value, nil
}

type datasetCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

func encodeDatasetCursor(value datasetCursor) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeDatasetCursor(raw string) (datasetCursor, error) {
	var value datasetCursor
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(data, &value)
	return value, err
}

func (r *Repository) ListDatasets(
	ctx context.Context, wikiID uuid.UUID, cursor string, limit int,
) (*DatasetPage, error) {
	var afterTime *time.Time
	afterID := uuid.Nil
	if cursor != "" {
		after, err := decodeDatasetCursor(cursor)
		if err != nil || after.CreatedAt.IsZero() || after.ID == uuid.Nil {
			return nil, ErrInvalidCursor
		}
		afterTime, afterID = &after.CreatedAt, after.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+datasetColumns+` FROM dataset
		WHERE wiki_id=$1
		  AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3))
		ORDER BY created_at DESC,id DESC LIMIT $4`,
		wikiID, afterTime, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("dataset: list definitions: %w", err)
	}
	defer rows.Close()
	result := &DatasetPage{Items: make([]Dataset, 0, limit)}
	for rows.Next() {
		value, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeDatasetCursor(datasetCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

const recordColumns = `id,dataset_id,entity_id,values_json,created_at,updated_at`

func scanRecord(row pgx.Row) (*Record, error) {
	var value Record
	if err := row.Scan(&value.ID, &value.DatasetID, &value.EntityID, &value.Values,
		&value.CreatedAt, &value.UpdatedAt); err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *Repository) InsertRecord(ctx context.Context, tx pgx.Tx, value *Record) error {
	return r.q(tx).QueryRow(ctx, `INSERT INTO dataset_record
		(id,dataset_id,entity_id,values_json) VALUES ($1,$2,$3,$4::jsonb)
		RETURNING created_at,updated_at`, value.ID, value.DatasetID, value.EntityID,
		value.Values).Scan(&value.CreatedAt, &value.UpdatedAt)
}

func (r *Repository) LockRecord(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Record, error) {
	value, err := scanRecord(r.q(tx).QueryRow(ctx,
		`SELECT `+recordColumns+` FROM dataset_record WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	return value, err
}

func (r *Repository) UpdateRecord(
	ctx context.Context, tx pgx.Tx, id uuid.UUID, entityID *uuid.UUID, values json.RawMessage,
) (*Record, error) {
	value, err := scanRecord(r.q(tx).QueryRow(ctx, `UPDATE dataset_record
		SET entity_id=$2,values_json=$3::jsonb,updated_at=now()
		WHERE id=$1 RETURNING `+recordColumns, id, entityID, values))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	return value, err
}

func (r *Repository) ValidateEntity(
	ctx context.Context, tx pgx.Tx, wikiID, entityID uuid.UUID,
) error {
	var exists bool
	if err := r.q(tx).QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM entity WHERE id=$1 AND wiki_id=$2 AND status='active'
	)`, entityID, wikiID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrInvalidRecord
	}
	return nil
}

const viewColumns = `id,dataset_id,view_type,name,config_json,created_by,created_at`

func scanView(row pgx.Row) (*View, error) {
	var value View
	if err := row.Scan(&value.ID, &value.DatasetID, &value.ViewType, &value.Name,
		&value.Config, &value.CreatedBy, &value.CreatedAt); err != nil {
		return nil, err
	}
	var config struct {
		ViewMode string `json:"view_mode"`
	}
	if err := json.Unmarshal(value.Config, &config); err != nil {
		return nil, ErrInvalidView
	}
	if config.ViewMode != "" {
		if !validViewType(config.ViewMode) ||
			databaseViewType(config.ViewMode) != value.ViewType {
			return nil, ErrInvalidView
		}
		value.ViewType = config.ViewMode
	}
	return &value, nil
}

func (r *Repository) InsertView(ctx context.Context, tx pgx.Tx, value *View) error {
	return r.q(tx).QueryRow(ctx, `INSERT INTO dataset_view
		(id,dataset_id,view_type,name,config_json,created_by)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6) RETURNING created_at`,
		value.ID, value.DatasetID, databaseViewType(value.ViewType), value.Name, value.Config,
		value.CreatedBy).Scan(&value.CreatedAt)
}

func (r *Repository) GetView(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*View, error) {
	value, err := scanView(r.q(tx).QueryRow(ctx,
		`SELECT `+viewColumns+` FROM dataset_view WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrViewNotFound
	}
	return value, err
}

func (r *Repository) ListViews(ctx context.Context, datasetID uuid.UUID) (*ViewPage, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+viewColumns+` FROM dataset_view
		WHERE dataset_id=$1 ORDER BY created_at DESC,id DESC`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &ViewPage{Items: []View{}}
	for rows.Next() {
		value, err := scanView(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *value)
	}
	return result, rows.Err()
}

func encodeOffsetCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodeOffsetCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, ErrInvalidCursor
	}
	offset, err := strconv.Atoi(string(raw))
	if err != nil || offset < 0 {
		return 0, ErrInvalidCursor
	}
	return offset, nil
}

func (r *Repository) QueryRecords(
	ctx context.Context, datasetID uuid.UUID, spec querySpec,
) (*RecordPage, error) {
	where, args, err := recordWhere(datasetID, spec.Filter)
	if err != nil {
		return nil, err
	}
	for _, group := range spec.Groups {
		if !group.HideEmpty {
			continue
		}
		args = append(args, group.Field)
		fieldArg := "$" + strconv.Itoa(len(args))
		where += " AND values_json ? " + fieldArg +
			" AND jsonb_typeof(values_json->" + fieldArg + ")<>'null'" +
			" AND COALESCE(values_json->>" + fieldArg + ",'')<>''"
	}
	groupArgs := append([]any(nil), args...)
	orderParts := make([]string, 0, len(spec.Sorts)+2)
	for _, sort := range spec.Sorts {
		args = append(args, sort.Field)
		fieldArg := "$" + strconv.Itoa(len(args))
		expression := "values_json->>" + fieldArg
		if sort.Numeric {
			expression = "CASE WHEN jsonb_typeof(values_json->" + fieldArg +
				")='number' THEN (values_json->>" + fieldArg + ")::numeric END"
		}
		direction := strings.ToUpper(sort.Direction)
		if direction != "ASC" && direction != "DESC" {
			return nil, ErrInvalidView
		}
		orderParts = append(orderParts, expression+" "+direction+" NULLS LAST")
	}
	orderParts = append(orderParts, "created_at DESC", "id DESC")
	order := strings.Join(orderParts, ",")
	args = append(args, spec.Limit+1, spec.Offset)
	limitArg, offsetArg := "$"+strconv.Itoa(len(args)-1), "$"+strconv.Itoa(len(args))
	rows, err := r.pool.Query(ctx, `SELECT `+recordColumns+`
		FROM dataset_record WHERE `+where+` ORDER BY `+order+
		` LIMIT `+limitArg+` OFFSET `+offsetArg, args...)
	if err != nil {
		return nil, fmt.Errorf("dataset: query records: %w", err)
	}
	defer rows.Close()
	result := &RecordPage{Items: make([]Record, 0, spec.Limit), Groups: []GroupCount{}}
	for rows.Next() {
		value, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > spec.Limit {
		next := encodeOffsetCursor(spec.Offset + spec.Limit)
		result.NextCursor = &next
		result.Items = result.Items[:spec.Limit]
	}
	if len(spec.Groups) > 0 {
		groups, err := r.groupCounts(ctx, where, groupArgs, spec.Groups)
		if err != nil {
			return nil, err
		}
		result.Groups = groups
	}
	return result, nil
}

func recordWhere(datasetID uuid.UUID, filter *ViewFilter) (string, []any, error) {
	where := "dataset_id=$1"
	args := []any{datasetID}
	if filter == nil {
		return where, args, nil
	}
	expression, err := recordFilterSQL(filter, &args)
	if err != nil {
		return "", nil, err
	}
	return where + " AND " + expression, args, nil
}

func recordFilterSQL(filter *ViewFilter, args *[]any) (string, error) {
	if filter.Logic != "" {
		if len(filter.Conditions) == 0 {
			return "", ErrInvalidView
		}
		parts := make([]string, 0, len(filter.Conditions))
		for index := range filter.Conditions {
			expression, err := recordFilterSQL(&filter.Conditions[index], args)
			if err != nil {
				return "", err
			}
			parts = append(parts, expression)
		}
		logic := strings.ToUpper(filter.Logic)
		if logic != "AND" && logic != "OR" {
			return "", ErrInvalidView
		}
		return "(" + strings.Join(parts, " "+logic+" ") + ")", nil
	}
	*args = append(*args, filter.Field)
	fieldArg := "$" + strconv.Itoa(len(*args))
	switch filter.Operator {
	case "exists":
		return "values_json ? " + fieldArg, nil
	case "not_exists":
		return "NOT values_json ? " + fieldArg, nil
	case "is_empty":
		return "(NOT values_json ? " + fieldArg +
			" OR jsonb_typeof(values_json->" + fieldArg + ")='null'" +
			" OR COALESCE(values_json->>" + fieldArg + ",'')='')", nil
	case "not_empty":
		return "(values_json ? " + fieldArg +
			" AND jsonb_typeof(values_json->" + fieldArg + ")<>'null'" +
			" AND COALESCE(values_json->>" + fieldArg + ",'')<>'')", nil
	case "eq":
		*args = append(*args, string(filter.Value))
		valueArg := "$" + strconv.Itoa(len(*args))
		return "values_json->" + fieldArg + " = " + valueArg + "::jsonb", nil
	case "neq":
		*args = append(*args, string(filter.Value))
		valueArg := "$" + strconv.Itoa(len(*args))
		return "values_json->" + fieldArg + " IS DISTINCT FROM " + valueArg + "::jsonb", nil
	case "contains", "not_contains":
		var text string
		if err := json.Unmarshal(filter.Value, &text); err != nil {
			return "", ErrInvalidView
		}
		*args = append(*args, text)
		valueArg := "$" + strconv.Itoa(len(*args))
		expression := "COALESCE(values_json->>" + fieldArg + ",'') ILIKE '%'||" + valueArg + "||'%'"
		if filter.Operator == "not_contains" {
			expression = "NOT (" + expression + ")"
		}
		return expression, nil
	case "gt", "gte", "lt", "lte":
		var number float64
		if err := json.Unmarshal(filter.Value, &number); err != nil {
			return "", ErrInvalidView
		}
		*args = append(*args, number)
		valueArg := "$" + strconv.Itoa(len(*args))
		operator := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[filter.Operator]
		return "CASE WHEN jsonb_typeof(values_json->" + fieldArg +
			")='number' THEN (values_json->>" + fieldArg + ")::numeric END " +
			operator + " " + valueArg, nil
	case "before", "after", "on_or_before", "on_or_after":
		var value string
		if err := json.Unmarshal(filter.Value, &value); err != nil {
			return "", ErrInvalidView
		}
		*args = append(*args, value)
		valueArg := "$" + strconv.Itoa(len(*args))
		operator := map[string]string{
			"before": "<", "after": ">",
			"on_or_before": "<=", "on_or_after": ">=",
		}[filter.Operator]
		return "values_json->>" + fieldArg + " " + operator + " " + valueArg, nil
	default:
		return "", ErrInvalidView
	}
}

func (r *Repository) groupCounts(
	ctx context.Context, where string, args []any, groupSpecs []ViewGroup,
) ([]GroupCount, error) {
	if len(groupSpecs) == 0 || len(groupSpecs) > 2 {
		return nil, ErrInvalidView
	}
	selectParts := make([]string, 0, len(groupSpecs))
	groupParts := make([]string, 0, len(groupSpecs))
	orderParts := make([]string, 0, len(groupSpecs))
	for index, group := range groupSpecs {
		args = append(args, group.Field)
		fieldArg := "$" + strconv.Itoa(len(args))
		selectParts = append(selectParts, "COALESCE(values_json->>"+fieldArg+",'(empty)')")
		position := strconv.Itoa(index + 1)
		groupParts = append(groupParts, position)
		direction := strings.ToUpper(group.SortDirection)
		if direction != "ASC" && direction != "DESC" {
			return nil, ErrInvalidView
		}
		orderParts = append(orderParts, position+" "+direction)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+strings.Join(selectParts, ",")+`,count(*)
		FROM dataset_record WHERE `+where+` GROUP BY `+strings.Join(groupParts, ",")+
		` ORDER BY `+strings.Join(orderParts, ",")+` LIMIT 500`, args...)
	if err != nil {
		return nil, fmt.Errorf("dataset: group records: %w", err)
	}
	defer rows.Close()
	result := []GroupCount{}
	for rows.Next() {
		var first, second string
		var count int
		scan := []any{&first}
		if len(groupSpecs) == 2 {
			scan = append(scan, &second)
		}
		scan = append(scan, &count)
		if err := rows.Scan(scan...); err != nil {
			return nil, err
		}
		path := []string{first}
		if len(groupSpecs) == 2 {
			path = append(path, second)
		}
		result = append(result, GroupCount{Value: first, Path: path, Count: count})
	}
	return result, rows.Err()
}
