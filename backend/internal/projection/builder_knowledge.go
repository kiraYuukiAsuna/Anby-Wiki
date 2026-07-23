// Entity/Claim/Citation 页面使用投影（M4-T07）。
// 每种引用独立注册为 Builder，均从 Current Revision AST 按页全删重插；
// 反向查询因此只读投影表，不扫描 ast_json，并始终携带来源 Revision。
package projection

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/component"
)

const (
	ProjectionEntityMentions = "entity_mentions"
	ProjectionClaimUsage     = "claim_usage"
	ProjectionCitationUsage  = "citation_usage"
)

type knowledgeUsageKind int

const (
	usageEntity knowledgeUsageKind = iota
	usageClaim
	usageCitation
)

// KnowledgeUsageBuilder 从知识引用 inline 节点构建一种使用投影。
type KnowledgeUsageBuilder struct {
	pool *pgxpool.Pool
	kind knowledgeUsageKind
}

func NewEntityMentionsBuilder(pool *pgxpool.Pool) *KnowledgeUsageBuilder {
	return &KnowledgeUsageBuilder{pool: pool, kind: usageEntity}
}

func NewClaimUsageBuilder(pool *pgxpool.Pool) *KnowledgeUsageBuilder {
	return &KnowledgeUsageBuilder{pool: pool, kind: usageClaim}
}

func NewCitationUsageBuilder(pool *pgxpool.Pool) *KnowledgeUsageBuilder {
	return &KnowledgeUsageBuilder{pool: pool, kind: usageCitation}
}

func (b *KnowledgeUsageBuilder) Type() string {
	switch b.kind {
	case usageEntity:
		return ProjectionEntityMentions
	case usageClaim:
		return ProjectionClaimUsage
	case usageCitation:
		return ProjectionCitationUsage
	default:
		panic("projection: 未知 KnowledgeUsageBuilder kind")
	}
}

type knowledgeUsageRow struct {
	blockID     uuid.UUID
	nodeID      string
	targetID    uuid.UUID
	displayText string
}

func (b *KnowledgeUsageBuilder) Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	doc, err := RevisionAST(ctx, tx, revisionID)
	if err != nil {
		return err
	}
	rows, err := b.collect(ctx, tx, doc)
	if err != nil {
		return err
	}

	table := map[knowledgeUsageKind]string{
		usageEntity:   "entity_mention_projection",
		usageClaim:    "claim_usage",
		usageCitation: "citation_usage",
	}[b.kind]
	if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE page_id = $1", table), pageID); err != nil {
		return fmt.Errorf("projection: 清空页面 %s 的 %s 失败: %w", pageID, b.Type(), err)
	}
	for _, row := range rows {
		switch b.kind {
		case usageEntity:
			_, err = tx.Exec(ctx, `
				INSERT INTO entity_mention_projection
					(page_id, revision_id, block_id, node_id, entity_id, mention_text)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				pageID, revisionID, row.blockID, row.nodeID, row.targetID, row.displayText)
		case usageClaim:
			_, err = tx.Exec(ctx, `
				INSERT INTO claim_usage (claim_id, page_id, revision_id, block_id, node_id)
				VALUES ($1, $2, $3, $4, $5)`,
				row.targetID, pageID, revisionID, row.blockID, row.nodeID)
		case usageCitation:
			_, err = tx.Exec(ctx, `
				INSERT INTO citation_usage
					(citation_id, page_id, revision_id, block_id, node_id, claim_id)
				VALUES ($1, $2, $3, $4, $5, NULL)`,
				row.targetID, pageID, revisionID, row.blockID, row.nodeID)
		}
		if err != nil {
			return fmt.Errorf("projection: 写入页面 %s 的 %s 失败: %w", pageID, b.Type(), err)
		}
	}
	return nil
}

func (b *KnowledgeUsageBuilder) HandleEvent(ctx context.Context, event Event) error {
	return HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}

func (b *KnowledgeUsageBuilder) collect(
	ctx context.Context, tx pgx.Tx, doc *ast.Document,
) ([]knowledgeUsageRow, error) {
	var rows []knowledgeUsageRow
	var collectErr error
	walkErr := ast.Walk(doc, func(node ast.WalkNode) bool {
		if b.kind == usageClaim && node.Block != nil && node.Block.Type == ast.BlockComponent {
			blockID, err := uuid.Parse(node.Block.ID)
			if err != nil {
				collectErr = fmt.Errorf("projection: ComponentBlock ID %q 非法: %w", node.Block.ID, err)
				return false
			}
			entityID, err := uuid.Parse(node.Block.EntityID)
			if err != nil {
				collectErr = fmt.Errorf("projection: ComponentBlock entity_id %q 非法: %w", node.Block.EntityID, err)
				return false
			}
			claimIDs, err := component.InfoboxClaimIDs(
				ctx, tx, entityID, node.Block.DisplayConfig,
			)
			if err != nil {
				collectErr = fmt.Errorf("projection: 解析 ComponentBlock %s Claim 失败: %w", node.Block.ID, err)
				return false
			}
			for _, claimID := range claimIDs {
				rows = append(rows, knowledgeUsageRow{
					blockID:  blockID,
					nodeID:   "component:" + claimID.String(),
					targetID: claimID,
				})
			}
			return true
		}
		if node.Inline == nil {
			return true
		}
		var rawID, displayText string
		switch {
		case b.kind == usageEntity && node.Inline.Type == ast.InlineEntityReference:
			rawID, displayText = node.Inline.EntityID, node.Inline.DisplayText
		case b.kind == usageClaim && node.Inline.Type == ast.InlineClaimReference:
			rawID = node.Inline.ClaimID
		case b.kind == usageCitation && node.Inline.Type == ast.InlineCitationReference:
			rawID = node.Inline.CitationID
		default:
			return true
		}
		blockID, err := uuid.Parse(node.Parent.ID)
		if err != nil {
			collectErr = fmt.Errorf("projection: 知识引用所属 Block ID %q 非法: %w", node.Parent.ID, err)
			return false
		}
		targetID, err := uuid.Parse(rawID)
		if err != nil {
			collectErr = fmt.Errorf("projection: %s 引用 ID %q 非法: %w", b.Type(), rawID, err)
			return false
		}
		rows = append(rows, knowledgeUsageRow{
			blockID: blockID, nodeID: strconv.Itoa(node.Index),
			targetID: targetID, displayText: displayText,
		})
		return true
	})
	if walkErr != nil {
		return nil, fmt.Errorf("projection: 遍历 AST 收集 %s 失败: %w", b.Type(), walkErr)
	}
	if collectErr != nil {
		return nil, collectErr
	}
	return rows, nil
}
