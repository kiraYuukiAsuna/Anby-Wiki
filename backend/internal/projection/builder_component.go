package projection

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

// ProjectionComponentDependency 记录页面 ComponentBlock 的稳定依赖。
const ProjectionComponentDependency = "component_dependency"

// ComponentDependencyBuilder 从 Current Revision 重建组件与 Entity 依赖。
type ComponentDependencyBuilder struct {
	pool       *pgxpool.Pool
	components *component.Service
}

func NewComponentDependencyBuilder(pool *pgxpool.Pool) *ComponentDependencyBuilder {
	return &ComponentDependencyBuilder{pool: pool, components: newComponentService(pool)}
}

func newComponentService(pool *pgxpool.Pool) *component.Service {
	return component.NewService(
		component.NewRepository(pool),
		page.NewRepository(pool),
		db.NewTxManager(pool),
		id.NewGenerator(),
		component.NewKnowledgeRegistry(pool),
	)
}

func (b *ComponentDependencyBuilder) Type() string {
	return ProjectionComponentDependency
}

func (b *ComponentDependencyBuilder) Rebuild(
	ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID,
) error {
	doc, err := RevisionAST(ctx, tx, revisionID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM component_dependency WHERE page_id = $1`, pageID); err != nil {
		return fmt.Errorf("projection: clear component dependencies for page %s: %w", pageID, err)
	}
	var collectErr error
	if err := ast.Walk(doc, func(node ast.WalkNode) bool {
		if node.Block == nil || node.Block.Type != ast.BlockComponent {
			return true
		}
		block := node.Block
		componentID, err := uuid.Parse(block.ComponentID)
		if err != nil {
			collectErr = fmt.Errorf("projection: invalid component_id %q: %w", block.ComponentID, err)
			return false
		}
		entityID, err := uuid.Parse(block.EntityID)
		if err != nil {
			collectErr = fmt.Errorf("projection: invalid entity_id %q: %w", block.EntityID, err)
			return false
		}
		blockID, err := uuid.Parse(block.ID)
		if err != nil {
			collectErr = fmt.Errorf("projection: invalid component block id %q: %w", block.ID, err)
			return false
		}
		if _, err := b.components.ValidateProps(
			ctx, componentID, block.ComponentVersion, block.DisplayConfig,
		); err != nil {
			collectErr = fmt.Errorf("projection: validate ComponentBlock %s: %w", block.ID, err)
			return false
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO component_dependency
				(page_id, revision_id, block_id, component_id, component_version, entity_id)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			pageID, revisionID, blockID, componentID, block.ComponentVersion, entityID,
		); err != nil {
			collectErr = fmt.Errorf("projection: insert ComponentBlock %s dependency: %w", block.ID, err)
			return false
		}
		return true
	}); err != nil {
		return err
	}
	return collectErr
}

func (b *ComponentDependencyBuilder) HandleEvent(ctx context.Context, event Event) error {
	return HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}

type componentHTMLRenderer struct {
	components *component.Service
}

func newComponentHTMLRenderer(pool *pgxpool.Pool) *componentHTMLRenderer {
	return &componentHTMLRenderer{components: newComponentService(pool)}
}

func (r *componentHTMLRenderer) RenderComponent(
	ctx context.Context, block *ast.Block,
) (string, error) {
	componentID, err := uuid.Parse(block.ComponentID)
	if err != nil {
		return "", err
	}
	entityID, err := uuid.Parse(block.EntityID)
	if err != nil {
		return "", err
	}
	return r.components.RenderEntity(
		ctx, componentID, block.ComponentVersion, entityID, block.DisplayConfig,
	)
}
