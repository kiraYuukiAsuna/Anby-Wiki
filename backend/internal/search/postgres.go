package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/platform/db"
)

// PostgresAdapter implements weighted full-text search over search_document.
type PostgresAdapter struct {
	pool *pgxpool.Pool
}

func NewPostgresAdapter(pool *pgxpool.Pool) *PostgresAdapter {
	return &PostgresAdapter{pool: pool}
}

func (a *PostgresAdapter) Index(ctx context.Context, doc SearchDocument) error {
	return a.index(ctx, a.pool, doc)
}

// IndexTx lets projection builders keep index writes in their framework
// transaction, avoiding a second connection waiting on the page row lock.
func (a *PostgresAdapter) IndexTx(ctx context.Context, tx pgx.Tx, doc SearchDocument) error {
	return a.index(ctx, tx, doc)
}

func (a *PostgresAdapter) index(ctx context.Context, q db.Querier, doc SearchDocument) error {
	if err := validateDocument(doc); err != nil {
		return err
	}
	if err := upsertDocument(ctx, q, doc); err != nil {
		return fmt.Errorf("search: index page %s: %w", doc.PageID, err)
	}
	return nil
}

func (a *PostgresAdapter) Delete(ctx context.Context, pageID uuid.UUID) error {
	return a.DeleteTx(ctx, a.pool, pageID)
}

// DeleteTx removes a staged document using either a transaction or the pool.
func (a *PostgresAdapter) DeleteTx(ctx context.Context, q db.Querier, pageID uuid.UUID) error {
	if _, err := q.Exec(ctx, `DELETE FROM search_document WHERE page_id = $1`, pageID); err != nil {
		return fmt.Errorf("search: delete page %s: %w", pageID, err)
	}
	return nil
}

// StagedDocument reads the adapter-neutral document committed by SearchBuilder.
func (a *PostgresAdapter) StagedDocument(ctx context.Context, q db.Querier, pageID uuid.UUID) (SearchDocument, error) {
	var doc SearchDocument
	var entityType *string
	err := q.QueryRow(ctx, `
		SELECT page_id, wiki_id, namespace_key, language, source_revision_id,
			display_title, normalized_title, aliases, body_text, entity_id,
			entity_type, entity_terms
		FROM search_document
		WHERE page_id = $1`, pageID).Scan(
		&doc.PageID, &doc.WikiID, &doc.Namespace, &doc.Language, &doc.SourceRevisionID,
		&doc.DisplayTitle, &doc.NormalizedTitle, &doc.Aliases, &doc.Body, &doc.EntityID,
		&entityType, &doc.EntityTerms,
	)
	if err != nil {
		return SearchDocument{}, fmt.Errorf("search: load staged page %s: %w", pageID, err)
	}
	if entityType != nil {
		doc.EntityType = *entityType
	}
	return doc, nil
}

// AllStagedDocuments returns the rebuild source for an external index.
func (a *PostgresAdapter) AllStagedDocuments(ctx context.Context) ([]SearchDocument, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT page_id, wiki_id, namespace_key, language, source_revision_id,
			display_title, normalized_title, aliases, body_text, entity_id,
			entity_type, entity_terms
		FROM search_document
		ORDER BY page_id`)
	if err != nil {
		return nil, fmt.Errorf("search: list staged documents: %w", err)
	}
	defer rows.Close()
	documents := make([]SearchDocument, 0)
	for rows.Next() {
		var doc SearchDocument
		var entityType *string
		if err := rows.Scan(
			&doc.PageID, &doc.WikiID, &doc.Namespace, &doc.Language, &doc.SourceRevisionID,
			&doc.DisplayTitle, &doc.NormalizedTitle, &doc.Aliases, &doc.Body, &doc.EntityID,
			&entityType, &doc.EntityTerms,
		); err != nil {
			return nil, fmt.Errorf("search: scan staged document: %w", err)
		}
		if entityType != nil {
			doc.EntityType = *entityType
		}
		documents = append(documents, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: iterate staged documents: %w", err)
	}
	return documents, nil
}

func (a *PostgresAdapter) Rebuild(ctx context.Context, documents []SearchDocument) error {
	for _, doc := range documents {
		if err := validateDocument(doc); err != nil {
			return err
		}
	}
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("search: begin rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM search_document`); err != nil {
		return fmt.Errorf("search: clear index: %w", err)
	}
	for _, doc := range documents {
		if err := upsertDocument(ctx, tx, doc); err != nil {
			return fmt.Errorf("search: rebuild page %s: %w", doc.PageID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("search: commit rebuild: %w", err)
	}
	return nil
}

func upsertDocument(ctx context.Context, q db.Querier, doc SearchDocument) error {
	if doc.Aliases == nil {
		doc.Aliases = []string{}
	}
	if doc.EntityTerms == nil {
		doc.EntityTerms = []string{}
	}
	_, err := q.Exec(ctx, `
		INSERT INTO search_document (
			page_id, wiki_id, namespace_key, language, source_revision_id,
			display_title, normalized_title, aliases, body_text,
			entity_id, entity_type, entity_terms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12)
		ON CONFLICT (page_id) DO UPDATE SET
			wiki_id = EXCLUDED.wiki_id,
			namespace_key = EXCLUDED.namespace_key,
			language = EXCLUDED.language,
			source_revision_id = EXCLUDED.source_revision_id,
			display_title = EXCLUDED.display_title,
			normalized_title = EXCLUDED.normalized_title,
			aliases = EXCLUDED.aliases,
			body_text = EXCLUDED.body_text,
			entity_id = EXCLUDED.entity_id,
			entity_type = EXCLUDED.entity_type,
			entity_terms = EXCLUDED.entity_terms`,
		doc.PageID, doc.WikiID, doc.Namespace, doc.Language, doc.SourceRevisionID,
		doc.DisplayTitle, doc.NormalizedTitle, doc.Aliases, doc.Body,
		doc.EntityID, doc.EntityType, doc.EntityTerms)
	return err
}

func (a *PostgresAdapter) Search(ctx context.Context, query Query) ([]Hit, int, error) {
	query = normalizeQuery(query)
	if query.Namespace != "" {
		var exists bool
		if err := a.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM namespace
				WHERE wiki_id = $1 AND namespace_key = $2
			)`, query.WikiID, query.Namespace).Scan(&exists); err != nil {
			return nil, 0, fmt.Errorf("search: validate namespace: %w", err)
		}
		if !exists {
			return nil, 0, ErrNamespaceNotFound
		}
	}
	if query.Text == "" {
		return []Hit{}, 0, nil
	}
	title, alias, body, entity := selectedFields(query.Fields)
	pattern := "%" + escapeLike(query.Text) + "%"
	prefix := escapeLike(query.Text) + "%"
	args := []any{
		query.Text, query.WikiID, query.Namespace, query.Language, query.EntityType,
		title, alias, body, entity, pattern, prefix,
	}

	var total int
	if err := a.pool.QueryRow(ctx, searchCountSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("search: count results: %w", err)
	}
	rows, err := a.pool.Query(ctx, searchHitsSQL, append(args, query.Limit, query.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("search: query: %w", err)
	}
	defer rows.Close()

	hits := make([]Hit, 0, query.Limit)
	for rows.Next() {
		var hit Hit
		if err := rows.Scan(&hit.PageID, &hit.DisplayTitle, &hit.Namespace,
			&hit.MatchedOn, &hit.Highlight, &hit.Score); err != nil {
			return nil, 0, fmt.Errorf("search: scan hit: %w", err)
		}
		if !strings.Contains(hit.Highlight, "[[") {
			hit.Highlight = markLiteral(hit.Highlight, query.Text)
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search: iterate hits: %w", err)
	}
	return hits, total, nil
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func markLiteral(value, query string) string {
	index := strings.Index(strings.ToLower(value), strings.ToLower(query))
	if index < 0 {
		return value
	}
	end := index + len(query)
	return value[:index] + "[[" + value[index:end] + "]]" + value[end:]
}

const searchBaseSQL = `
	WITH input AS (
		SELECT websearch_to_tsquery('simple', $1) AS tsq, lower($1) AS needle
	),
	matched AS (
		SELECT d.*, i.tsq, i.needle,
			CASE
				WHEN $6 AND (lower(d.normalized_title) = i.needle OR lower(d.display_title) = i.needle) THEN 0
				WHEN $6 AND (d.normalized_title ILIKE $11 ESCAPE '\' OR d.display_title ILIKE $11 ESCAPE '\') THEN 1
				WHEN $6 AND (d.normalized_title ILIKE $10 ESCAPE '\' OR d.display_title ILIKE $10 ESCAPE '\') THEN 2
				WHEN $7 AND EXISTS (SELECT 1 FROM unnest(d.aliases) a WHERE a ILIKE $10 ESCAPE '\') THEN 3
				WHEN $9 AND (
					to_tsvector('simple', array_to_string(d.entity_terms, ' ')) @@ i.tsq OR
					EXISTS (SELECT 1 FROM unnest(d.entity_terms) e WHERE e ILIKE $10 ESCAPE '\')
				) THEN 4
				ELSE 5
			END AS match_rank
		FROM search_document d
		CROSS JOIN input i
		WHERE ($2::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR d.wiki_id = $2)
		  AND ($3 = '' OR d.namespace_key = $3)
		  AND ($4 = '' OR d.language = $4)
		  AND ($5 = '' OR d.entity_type = $5)
		  AND (
			($6 AND (
				to_tsvector('simple', d.display_title || ' ' || d.normalized_title) @@ i.tsq OR
				d.normalized_title ILIKE $10 ESCAPE '\' OR d.display_title ILIKE $10 ESCAPE '\'))
			OR ($7 AND (
				to_tsvector('simple', array_to_string(d.aliases, ' ')) @@ i.tsq OR
				EXISTS (SELECT 1 FROM unnest(d.aliases) a WHERE a ILIKE $10 ESCAPE '\')))
			OR ($8 AND (
				to_tsvector('simple', d.body_text) @@ i.tsq OR
				d.body_text ILIKE $10 ESCAPE '\'))
			OR ($9 AND (
				to_tsvector('simple', array_to_string(d.entity_terms, ' ')) @@ i.tsq OR
				EXISTS (SELECT 1 FROM unnest(d.entity_terms) e WHERE e ILIKE $10 ESCAPE '\')))
		  )
	)`

const searchCountSQL = searchBaseSQL + `
	SELECT count(*) FROM matched`

const searchHitsSQL = searchBaseSQL + `
	SELECT page_id, display_title, namespace_key,
		CASE match_rank
			WHEN 0 THEN 'title'
			WHEN 1 THEN 'title'
			WHEN 2 THEN 'title'
			WHEN 3 THEN 'alias'
			WHEN 4 THEN 'entity'
			ELSE 'body'
		END,
		ts_headline(
			'simple',
			CASE match_rank
				WHEN 0 THEN display_title
				WHEN 1 THEN display_title
				WHEN 2 THEN display_title
				WHEN 3 THEN array_to_string(aliases, ' ')
				WHEN 4 THEN array_to_string(entity_terms, ' ')
				ELSE body_text
			END,
			tsq,
			'StartSel=[[, StopSel=]], MaxWords=24, MinWords=8'
		),
		(ts_rank_cd(search_vector, tsq) + (5 - match_rank)::real) AS score
	FROM matched
	ORDER BY match_rank, score DESC, display_title, page_id
	LIMIT $12 OFFSET $13`

var _ SearchAdapter = (*PostgresAdapter)(nil)
