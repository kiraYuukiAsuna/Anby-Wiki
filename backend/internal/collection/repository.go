package collection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
)

type Repository struct {
	pool db.Querier
}

func NewRepository(pool db.Querier) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) q(tx pgx.Tx) db.Querier {
	if tx != nil {
		return tx
	}
	return r.pool
}

func scanCollection(row pgx.Row) (*Collection, error) {
	var value Collection
	if err := row.Scan(
		&value.ID, &value.WikiID, &value.CollectionType, &value.Title,
		&value.DescriptionPageID, &value.QueryJSON, &value.CreatedBy,
		&value.CreatedAt, &value.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *Repository) Insert(ctx context.Context, tx pgx.Tx, value *Collection) error {
	return tx.QueryRow(ctx, `INSERT INTO collection
		(id,wiki_id,collection_type,title,description_page_id,query_json,created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING created_at,updated_at`,
		value.ID, value.WikiID, value.CollectionType, value.Title,
		value.DescriptionPageID, nullableJSON(value.QueryJSON), value.CreatedBy,
	).Scan(&value.CreatedAt, &value.UpdatedAt)
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func (r *Repository) Get(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Collection, error) {
	value, err := scanCollection(r.q(tx).QueryRow(ctx, `SELECT
		id,wiki_id,collection_type,title,description_page_id,query_json,
		created_by,created_at,updated_at
		FROM collection WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrNotFound, id)
	}
	return value, err
}

func (r *Repository) Lock(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Collection, error) {
	value, err := scanCollection(tx.QueryRow(ctx, `SELECT
		id,wiki_id,collection_type,title,description_page_id,query_json,
		created_by,created_at,updated_at
		FROM collection WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrNotFound, id)
	}
	return value, err
}

type listCursor struct {
	Title string    `json:"t"`
	ID    uuid.UUID `json:"i"`
}

func (r *Repository) List(
	ctx context.Context, wikiID uuid.UUID, cursor string, limit int,
) (*CollectionPage, error) {
	var after listCursor
	var err error
	if cursor != "" {
		after, err = decodeCursor[listCursor](cursor)
		if err != nil || after.Title == "" || after.ID == uuid.Nil {
			return nil, ErrInvalidCursor
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT
		id,wiki_id,collection_type,title,description_page_id,query_json,
		created_by,created_at,updated_at
		FROM collection
		WHERE wiki_id=$1 AND ($2='' OR (title,id) > ($2,$3))
		ORDER BY title,id LIMIT $4`,
		wikiID, after.Title, after.ID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &CollectionPage{Items: make([]Collection, 0, limit)}
	for rows.Next() {
		value, err := scanCollection(rows)
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
		next := encodeCursor(listCursor{Title: last.Title, ID: last.ID})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (r *Repository) ValidateRevision(
	ctx context.Context, tx pgx.Tx, wikiID, revisionID uuid.UUID,
) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM revision r JOIN page p ON p.id=r.page_id
		WHERE r.id=$1 AND p.wiki_id=$2 AND p.current_revision_id IS NOT NULL
	)`, revisionID, wikiID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrInvalidMember
	}
	return nil
}

func (r *Repository) ValidatePage(
	ctx context.Context, tx pgx.Tx, wikiID, pageID uuid.UUID,
) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM page WHERE id=$1 AND wiki_id=$2 AND deleted_at IS NULL
	)`, pageID, wikiID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrInvalidMember
	}
	return nil
}

func (r *Repository) ValidateEntity(
	ctx context.Context, tx pgx.Tx, wikiID, entityID uuid.UUID,
) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM entity WHERE id=$1 AND wiki_id=$2 AND status='active'
	)`, entityID, wikiID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrInvalidMember
	}
	return nil
}

func (r *Repository) ReplaceMembers(
	ctx context.Context, tx pgx.Tx, collectionID uuid.UUID, members []Membership,
) error {
	if _, err := tx.Exec(ctx, `DELETE FROM collection_membership WHERE collection_id=$1`, collectionID); err != nil {
		return err
	}
	for _, member := range members {
		if _, err := tx.Exec(ctx, `INSERT INTO collection_membership
			(collection_id,page_id,entity_id,member_type,source_type,sort_key,source_revision_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			collectionID, member.PageID, member.EntityID, member.MemberType,
			member.SourceType, member.SortKey, member.SourceRevisionID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ResolveRule(
	ctx context.Context, tx pgx.Tx, wikiID uuid.UUID, rule Rule,
) ([]uuid.UUID, error) {
	var rows pgx.Rows
	var err error
	switch rule.Kind {
	case "entity_type":
		rows, err = tx.Query(ctx, `SELECT e.id FROM entity e
			JOIN entity_type t ON t.id=e.entity_type_id
			WHERE e.wiki_id=$1 AND e.status='active' AND t.type_key=$2
			ORDER BY e.canonical_key,e.id`, wikiID, rule.EntityType)
	case "claim_exists":
		rows, err = tx.Query(ctx, `SELECT DISTINCT e.id FROM entity e
			JOIN claim c ON c.subject_entity_id=e.id
			JOIN property p ON p.id=c.property_id
			WHERE e.wiki_id=$1 AND e.status='active'
			  AND c.status='published' AND p.property_key=$2
			ORDER BY e.id`, wikiID, rule.Property)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) ValidateRuleReference(
	ctx context.Context, tx pgx.Tx, rule Rule,
) error {
	var exists bool
	var err error
	if rule.Kind == "entity_type" {
		err = tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM entity_type WHERE type_key=$1
		)`, rule.EntityType).Scan(&exists)
	} else {
		err = tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM property WHERE property_key=$1
		)`, rule.Property).Scan(&exists)
	}
	if err != nil {
		return err
	}
	if !exists {
		return ErrInvalidRule
	}
	return nil
}

type memberCursor struct {
	SortKey    string    `json:"s"`
	MemberType string    `json:"m"`
	TargetID   uuid.UUID `json:"i"`
}

func (r *Repository) ListMembers(
	ctx context.Context, collectionID uuid.UUID, cursor string, limit int,
) (*MembershipPage, error) {
	var after memberCursor
	var err error
	if cursor != "" {
		after, err = decodeCursor[memberCursor](cursor)
		if err != nil || after.SortKey == "" || after.TargetID == uuid.Nil ||
			(after.MemberType != MemberPage && after.MemberType != MemberEntity) {
			return nil, ErrInvalidCursor
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT
		m.collection_id,m.page_id,m.entity_id,m.member_type,m.source_type,
		m.sort_key,m.source_revision_id,
		CASE WHEN m.member_type='page' THEN p.display_title
		     ELSE COALESCE(el.label,e.canonical_key) END,
		m.created_at
		FROM collection_membership m
		LEFT JOIN page p ON p.id=m.page_id
		LEFT JOIN entity e ON e.id=m.entity_id
		LEFT JOIN LATERAL (
			SELECT label FROM entity_label
			WHERE entity_id=e.id ORDER BY is_primary DESC,language,label LIMIT 1
		) el ON true
		WHERE m.collection_id=$1
		  AND ($2='' OR (m.sort_key,m.member_type,COALESCE(m.page_id,m.entity_id)) > ($2,$3,$4))
		ORDER BY m.sort_key,m.member_type,COALESCE(m.page_id,m.entity_id)
		LIMIT $5`, collectionID, after.SortKey, after.MemberType, after.TargetID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &MembershipPage{Items: make([]Membership, 0, limit)}
	for rows.Next() {
		var value Membership
		if err := rows.Scan(
			&value.CollectionID, &value.PageID, &value.EntityID, &value.MemberType,
			&value.SourceType, &value.SortKey, &value.SourceRevisionID,
			&value.DisplayTitle, &value.CreatedAt,
		); err != nil {
			return nil, err
		}
		result.Items = append(result.Items, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		targetID := *last.EntityID
		if last.PageID != nil {
			targetID = *last.PageID
		}
		next := encodeCursor(memberCursor{
			SortKey: last.SortKey, MemberType: last.MemberType, TargetID: targetID,
		})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func encodeCursor(value any) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor[T any](cursor string) (T, error) {
	var value T
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	return value, nil
}
