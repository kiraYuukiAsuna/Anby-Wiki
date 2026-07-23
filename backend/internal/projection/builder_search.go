// Search projection builder (M7-T01): builds adapter-neutral documents from
// current page metadata, aliases, primary Entity text, and Revision AST.
package projection

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	wikisearch "github.com/anby/wiki/backend/internal/search"
)

const ProjectionSearch = "search"

type SearchBuilder struct {
	pool   *pgxpool.Pool
	stage  *wikisearch.PostgresAdapter
	remote wikisearch.SearchAdapter
}

func NewSearchBuilder(pool *pgxpool.Pool, adapter wikisearch.SearchAdapter) *SearchBuilder {
	builder := &SearchBuilder{pool: pool, stage: wikisearch.NewPostgresAdapter(pool)}
	if _, postgresOnly := adapter.(*wikisearch.PostgresAdapter); !postgresOnly {
		builder.remote = adapter
	}
	return builder
}

func (b *SearchBuilder) Type() string { return ProjectionSearch }

func (b *SearchBuilder) Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	doc, err := RevisionAST(ctx, tx, revisionID)
	if err != nil {
		return err
	}
	body, err := searchBodyText(doc)
	if err != nil {
		return err
	}

	searchDoc, err := loadSearchDocument(ctx, tx, pageID, revisionID)
	if err != nil {
		return err
	}
	searchDoc.Body = body
	if err := b.stage.IndexTx(ctx, tx, searchDoc); err != nil {
		return fmt.Errorf("projection: stage search document for page %s: %w", pageID, err)
	}
	return nil
}

func (b *SearchBuilder) HandleEvent(ctx context.Context, event Event) error {
	revisionID, ok, err := eventRevisionID(event)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("projection: search revision event missing revision_id")
	}
	if err := b.rebuildStagingLocked(ctx, event.AggregateID, revisionID); err != nil {
		return err
	}
	return b.syncExpected(ctx, event.AggregateID, revisionID)
}

// HandlePageMetadataEvent refreshes title/alias/Entity metadata on lifecycle
// events. Unpublished or deleted pages must not leave searchable documents.
func (b *SearchBuilder) HandlePageMetadataEvent(ctx context.Context, event Event) error {
	revisionID, ok, err := b.rebuildCurrentStagingLocked(ctx, event.AggregateID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return b.syncExpected(ctx, event.AggregateID, revisionID)
}

// PrepareFullRebuild removes documents that no longer belong to a live page;
// Rebuilder then repopulates the adapter from every current Revision.
func (b *SearchBuilder) PrepareFullRebuild(ctx context.Context) error {
	if err := b.stage.Rebuild(ctx, nil); err != nil {
		return err
	}
	if b.remote != nil {
		return b.remote.Rebuild(ctx, nil)
	}
	return nil
}

// AfterRebuild synchronizes committed staging data to a remote backend.
func (b *SearchBuilder) AfterRebuild(ctx context.Context, pageID, revisionID uuid.UUID) error {
	return b.syncExpected(ctx, pageID, revisionID)
}

func (b *SearchBuilder) rebuildStagingLocked(ctx context.Context, pageID, expectedRevisionID uuid.UUID) error {
	revisionID, ok, err := b.rebuildCurrentStagingLocked(ctx, pageID)
	if err != nil {
		return err
	}
	if !ok || revisionID != expectedRevisionID {
		return nil
	}
	return nil
}

func (b *SearchBuilder) rebuildCurrentStagingLocked(ctx context.Context, pageID uuid.UUID) (uuid.UUID, bool, error) {
	tx, err := b.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("projection: begin search staging transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	revisionID, ok, err := lockedCurrentRevision(ctx, tx, pageID)
	if err != nil {
		return uuid.Nil, false, err
	}
	if !ok {
		if err := b.stage.DeleteTx(ctx, tx, pageID); err != nil {
			return uuid.Nil, false, err
		}
		if b.remote != nil {
			if err := b.remote.Delete(ctx, pageID); err != nil {
				return uuid.Nil, false, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return uuid.Nil, false, fmt.Errorf("projection: commit search deletion: %w", err)
		}
		return uuid.Nil, false, nil
	}
	if err := b.Rebuild(ctx, tx, pageID, revisionID); err != nil {
		return uuid.Nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, false, fmt.Errorf("projection: commit search staging: %w", err)
	}
	return revisionID, true, nil
}

// syncExpected holds the page row lock until the remote task completes. A
// publish transaction therefore cannot enqueue a newer revision between the
// final Current check and this remote write.
func (b *SearchBuilder) syncExpected(ctx context.Context, pageID, expectedRevisionID uuid.UUID) error {
	if b.remote == nil {
		return nil
	}
	tx, err := b.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("projection: begin remote search sync: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, ok, err := lockedCurrentRevision(ctx, tx, pageID)
	if err != nil {
		return err
	}
	if !ok || current != expectedRevisionID {
		return nil
	}
	doc, err := b.stage.StagedDocument(ctx, tx, pageID)
	if err != nil {
		return err
	}
	if doc.SourceRevisionID != expectedRevisionID {
		return fmt.Errorf("projection: staged search revision mismatch for page %s", pageID)
	}
	if err := b.remote.Index(ctx, doc); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("projection: commit remote search sync: %w", err)
	}
	return nil
}

func lockedCurrentRevision(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) (uuid.UUID, bool, error) {
	var current *uuid.UUID
	var deletedAt any
	err := tx.QueryRow(ctx,
		`SELECT current_revision_id, deleted_at FROM page WHERE id = $1 FOR UPDATE`, pageID).
		Scan(&current, &deletedAt)
	if err == pgx.ErrNoRows {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("projection: lock page for search sync: %w", err)
	}
	if current == nil || deletedAt != nil {
		return uuid.Nil, false, nil
	}
	return *current, true, nil
}

func loadSearchDocument(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) (wikisearch.SearchDocument, error) {
	var doc wikisearch.SearchDocument
	var entityType *string
	err := tx.QueryRow(ctx, `
		SELECT p.id, p.wiki_id, n.namespace_key, p.language, p.display_title,
			p.normalized_title,
			ARRAY(
				SELECT pa.normalized_title
				FROM page_alias pa
				WHERE pa.page_id = p.id
				ORDER BY pa.normalized_title
			),
			p.primary_entity_id,
			et.type_key,
			ARRAY(
				SELECT term
				FROM (
					SELECT el.label AS term
					FROM entity_label el
					WHERE el.entity_id = p.primary_entity_id
					UNION
					SELECT el.description
					FROM entity_label el
					WHERE el.entity_id = p.primary_entity_id AND el.description <> ''
					UNION
					SELECT ea.alias
					FROM entity_alias ea
					WHERE ea.entity_id = p.primary_entity_id
				) terms
				ORDER BY term
			)
		FROM page p
		JOIN namespace n ON n.id = p.namespace_id
		LEFT JOIN entity e ON e.id = p.primary_entity_id AND e.status = 'active'
		LEFT JOIN entity_type et ON et.id = e.entity_type_id
		WHERE p.id = $1 AND p.deleted_at IS NULL AND p.current_revision_id = $2`,
		pageID, revisionID).Scan(
		&doc.PageID, &doc.WikiID, &doc.Namespace, &doc.Language, &doc.DisplayTitle,
		&doc.NormalizedTitle, &doc.Aliases, &doc.EntityID, &entityType, &doc.EntityTerms)
	if err != nil {
		return wikisearch.SearchDocument{}, fmt.Errorf("projection: load search metadata for page %s: %w", pageID, err)
	}
	doc.SourceRevisionID = revisionID
	if entityType != nil {
		doc.EntityType = *entityType
	}
	return doc, nil
}

func searchBodyText(doc *ast.Document) (string, error) {
	var parts []string
	var collectErr error
	err := ast.Walk(doc, func(node ast.WalkNode) bool {
		if node.Block != nil && node.Block.Type == ast.BlockCode {
			text, err := node.Block.TextContent()
			if err != nil {
				collectErr = err
				return false
			}
			appendSearchText(&parts, text)
			return true
		}
		if node.Inline == nil {
			return true
		}
		switch node.Inline.Type {
		case ast.InlineText, ast.InlineCode:
			appendSearchText(&parts, node.Inline.Text)
		case ast.InlinePageReference:
			if node.Inline.ResolutionStatus == ast.ResolutionUnresolved {
				appendSearchText(&parts, node.Inline.NormalizedTitle)
			} else {
				appendSearchText(&parts, node.Inline.DisplayText)
			}
		case ast.InlineExternalLink, ast.InlineEntityReference, ast.InlineClaimReference,
			ast.InlineCitationReference:
			appendSearchText(&parts, node.Inline.DisplayText)
		}
		return true
	})
	if err != nil {
		return "", fmt.Errorf("projection: walk AST for search text: %w", err)
	}
	if collectErr != nil {
		return "", fmt.Errorf("projection: extract search text: %w", collectErr)
	}
	return strings.Join(parts, " "), nil
}

func appendSearchText(parts *[]string, text string) {
	if text = strings.TrimSpace(text); text != "" {
		*parts = append(*parts, text)
	}
}

var _ Builder = (*SearchBuilder)(nil)
var _ FullRebuildPreparer = (*SearchBuilder)(nil)
var _ PostCommitRebuilder = (*SearchBuilder)(nil)
