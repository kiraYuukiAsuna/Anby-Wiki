package projection

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/render"
)

// RenderedSectionsBuilder materializes non-overlapping top-level document
// sections. The projection is disposable: each rebuild replaces every row for
// the Page and records the source Revision on both section and block locator.
type RenderedSectionsBuilder struct {
	pool    *pgxpool.Pool
	dynamic render.DynamicRenderer
}

func NewRenderedSectionsBuilder(pool *pgxpool.Pool) *RenderedSectionsBuilder {
	return &RenderedSectionsBuilder{
		pool: pool, dynamic: newComponentHTMLRenderer(pool),
	}
}

func (b *RenderedSectionsBuilder) Type() string { return "rendered_sections" }

type renderedSectionBuild struct {
	key            string
	headingBlockID *uuid.UUID
	level          *int
	title          string
	blocks         []*ast.Block
}

func (b *RenderedSectionsBuilder) Rebuild(
	ctx context.Context,
	tx pgx.Tx,
	pageID, revisionID uuid.UUID,
) error {
	doc, err := RevisionAST(ctx, tx, revisionID)
	if err != nil {
		return err
	}
	sections, err := splitRenderedSections(doc)
	if err != nil {
		return err
	}
	citationOrder, err := sectionCitationOrder(doc)
	if err != nil {
		return err
	}
	citationJSON, err := json.Marshal(citationOrder)
	if err != nil {
		return fmt.Errorf("projection: encode section citation order: %w", err)
	}

	if _, err := tx.Exec(
		ctx, `DELETE FROM rendered_section WHERE page_id=$1`, pageID,
	); err != nil {
		return fmt.Errorf("projection: clear sections for page %s: %w", pageID, err)
	}
	for position, section := range sections {
		fragment := &ast.Document{
			Type: "document", SchemaVersion: ast.SchemaVersion,
			Children: section.blocks,
		}
		raw, err := json.Marshal(fragment)
		if err != nil {
			return fmt.Errorf(
				"projection: encode page %s section %q: %w",
				pageID, section.key, err,
			)
		}
		html, err := render.RenderHTMLWithResolversAndCitationOrder(
			ctx,
			fragment,
			b.dynamic,
			b.dynamic,
			citationOrder,
		)
		if err != nil {
			return fmt.Errorf(
				"projection: render page %s section %q: %w",
				pageID, section.key, err,
			)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rendered_section (
				page_id,revision_id,section_key,position,heading_block_id,
				level,title,ast_json,html_content,renderer_version,size_bytes,
				citation_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			pageID, revisionID, section.key, position, section.headingBlockID,
			section.level, section.title, raw, html, render.RendererVersion,
			len(raw), citationJSON,
		); err != nil {
			return fmt.Errorf(
				"projection: write page %s section %q: %w",
				pageID, section.key, err,
			)
		}
		if err := insertSectionBlockLocators(
			ctx, tx, pageID, revisionID, section.key, fragment,
		); err != nil {
			return err
		}
	}
	return nil
}

func (b *RenderedSectionsBuilder) HandleEvent(
	ctx context.Context,
	event Event,
) error {
	return HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}

func splitRenderedSections(
	doc *ast.Document,
) ([]renderedSectionBuild, error) {
	sections := make([]renderedSectionBuild, 0)
	current := renderedSectionBuild{key: "lead", title: "导言"}
	flush := func(force bool) {
		if !force && len(current.blocks) == 0 {
			return
		}
		sections = append(sections, current)
	}
	for _, block := range doc.Children {
		if block.Type == ast.BlockHeading {
			flush(false)
			headingID, err := uuid.Parse(block.ID)
			if err != nil {
				return nil, fmt.Errorf(
					"projection: section heading ID %q: %w", block.ID, err,
				)
			}
			title, err := headingPlainText(block)
			if err != nil {
				return nil, err
			}
			level := block.Level
			current = renderedSectionBuild{
				key: headingID.String(), headingBlockID: &headingID,
				level: &level, title: title,
			}
		}
		current.blocks = append(current.blocks, block)
	}
	flush(len(sections) == 0)
	return sections, nil
}

func insertSectionBlockLocators(
	ctx context.Context,
	tx pgx.Tx,
	pageID, revisionID uuid.UUID,
	sectionKey string,
	doc *ast.Document,
) error {
	var locatorErr error
	if err := ast.Walk(doc, func(node ast.WalkNode) bool {
		if node.Block == nil {
			return true
		}
		blockID, err := uuid.Parse(node.Block.ID)
		if err != nil {
			locatorErr = fmt.Errorf(
				"projection: section block ID %q: %w", node.Block.ID, err,
			)
			return false
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO rendered_section_block
				(page_id,revision_id,section_key,block_id)
			VALUES ($1,$2,$3,$4)`,
			pageID, revisionID, sectionKey, blockID,
		); err != nil {
			locatorErr = fmt.Errorf(
				"projection: write section locator for block %s: %w",
				blockID, err,
			)
			return false
		}
		return true
	}); err != nil {
		return fmt.Errorf("projection: walk section blocks: %w", err)
	}
	return locatorErr
}

func sectionCitationOrder(doc *ast.Document) ([]string, error) {
	order := make([]string, 0)
	seen := make(map[string]struct{})
	if err := ast.Walk(doc, func(node ast.WalkNode) bool {
		if node.Inline == nil ||
			node.Inline.Type != ast.InlineCitationReference {
			return true
		}
		if _, exists := seen[node.Inline.CitationID]; exists {
			return true
		}
		seen[node.Inline.CitationID] = struct{}{}
		order = append(order, node.Inline.CitationID)
		return true
	}); err != nil {
		return nil, fmt.Errorf("projection: collect section citations: %w", err)
	}
	return order, nil
}
