package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/render"
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
	pool       *pgxpool.Pool
}

func newComponentHTMLRenderer(pool *pgxpool.Pool) *componentHTMLRenderer {
	return &componentHTMLRenderer{
		components: newComponentService(pool),
		pool:       pool,
	}
}

// NewDynamicHTMLRenderer exposes the trusted resolver shared by projection
// builders and the synchronous read fallback. It is intentionally read-only.
func NewDynamicHTMLRenderer(pool *pgxpool.Pool) render.DynamicRenderer {
	return newComponentHTMLRenderer(pool)
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

// RenderClaim follows the immutable supersede chain and displays the newest
// value while retaining the originally referenced Claim ID for provenance.
func (r *componentHTMLRenderer) RenderClaim(
	ctx context.Context,
	rawClaimID string,
) (string, error) {
	claimID, err := uuid.Parse(rawClaimID)
	if err != nil {
		return "", fmt.Errorf("projection: invalid claim ID %q: %w", rawClaimID, err)
	}
	var (
		resolvedID         uuid.UUID
		valueType          string
		valueJSON          []byte
		targetID           *uuid.UUID
		targetLabel        *string
		status             string
		verificationStatus string
	)
	err = r.pool.QueryRow(ctx, `
		WITH RECURSIVE claim_chain AS (
			SELECT id,value_type,value_json,target_entity_id,status,
			       verification_status,superseded_by,0 AS depth,
			       ARRAY[id] AS visited
			FROM claim
			WHERE id=$1
			UNION ALL
			SELECT next.id,next.value_type,next.value_json,next.target_entity_id,
			       next.status,next.verification_status,next.superseded_by,
			       chain.depth+1,chain.visited || next.id
			FROM claim_chain chain
			JOIN claim next ON next.id=COALESCE(
				chain.superseded_by,
				(
					SELECT mapping.new_claim_id
					FROM entity_merge_claim_map mapping
					JOIN entity_merge merge ON merge.id=mapping.merge_id
					WHERE mapping.old_claim_id=chain.id
					  AND merge.status='applied'
					ORDER BY merge.created_at DESC,merge.id DESC
					LIMIT 1
				)
			)
			WHERE chain.depth < 31 AND NOT next.id=ANY(chain.visited)
		)
		SELECT chain.id,chain.value_type,chain.value_json,chain.target_entity_id,
		       chain.status,chain.verification_status,
		       COALESCE(
		         (SELECT label FROM entity_label
		          WHERE entity_id=chain.target_entity_id AND is_primary
		          ORDER BY (language IN ('zh-CN','zh-Hans','zh')) DESC,
		                   language,label LIMIT 1),
		         target.canonical_key)
		FROM claim_chain chain
		LEFT JOIN entity target ON target.id=chain.target_entity_id
		ORDER BY chain.depth DESC
		LIMIT 1`, claimID,
	).Scan(
		&resolvedID,
		&valueType,
		&valueJSON,
		&targetID,
		&status,
		&verificationStatus,
		&targetLabel,
	)
	if err != nil {
		return "", fmt.Errorf("projection: resolve claim %s: %w", claimID, err)
	}
	value, err := claimReferenceValueText(
		valueType,
		valueJSON,
		targetID,
		targetLabel,
	)
	if err != nil {
		return "", fmt.Errorf("projection: format claim %s: %w", resolvedID, err)
	}
	return fmt.Sprintf(
		`<a class="claim-reference" href="/claims/%s" data-claim-ref="%s" data-resolved-claim-id="%s" data-claim-status="%s" data-verification-status="%s">%s</a>`,
		html.EscapeString(resolvedID.String()),
		html.EscapeString(claimID.String()),
		html.EscapeString(resolvedID.String()),
		html.EscapeString(status),
		html.EscapeString(verificationStatus),
		html.EscapeString(value),
	), nil
}

func claimReferenceValueText(
	valueType string,
	raw json.RawMessage,
	targetID *uuid.UUID,
	targetLabel *string,
) (string, error) {
	if valueType == "entity" {
		if targetLabel != nil {
			return *targetLabel, nil
		}
		if targetID != nil {
			return targetID.String(), nil
		}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	switch typed := value.(type) {
	case string:
		return typed, nil
	case float64, bool:
		return fmt.Sprint(typed), nil
	case map[string]any:
		if valueType == "coordinate" {
			lat, latOK := typed["lat"]
			lon, lonOK := typed["lon"]
			if latOK && lonOK {
				return fmt.Sprintf("%v, %v", lat, lon), nil
			}
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(encoded)), nil
}
