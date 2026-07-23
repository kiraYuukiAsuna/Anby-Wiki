// 外链使用投影 Builder（M3-T06，设计 §8、§18.9）。
//
// ExternalLinksBuilder Walk 当前 Revision AST 中的 external_link，交给 Evidence
// 系统级领域服务规范化并 upsert external_resource，再按页全删重插
// external_link_usage。资源行独立于页面生命周期：usage 消失时不删除资源。
package projection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/evidence"
	"github.com/anby/wiki/backend/internal/platform/id"
)

// ExternalLinksBuilder 外链资源与使用关系投影 Builder。
type ExternalLinksBuilder struct {
	pool      *pgxpool.Pool
	resources *evidence.ExternalResourceService
	logger    *slog.Logger
}

// NewExternalLinksBuilder 装配外链投影 Builder。
func NewExternalLinksBuilder(pool *pgxpool.Pool, logger *slog.Logger) *ExternalLinksBuilder {
	if logger == nil {
		logger = slog.Default()
	}
	return &ExternalLinksBuilder{
		pool: pool,
		resources: evidence.NewExternalResourceService(
			evidence.NewRepository(pool), id.NewGenerator(),
		),
		logger: logger,
	}
}

// Type 实现 Builder。
func (b *ExternalLinksBuilder) Type() string { return "external_links" }

type externalLinkCandidate struct {
	blockID uuid.UUID
	nodeID  string
	url     string
}

// Rebuild 以当前 AST 为唯一来源重建一页的外链 usage。
func (b *ExternalLinksBuilder) Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	doc, err := RevisionAST(ctx, tx, revisionID)
	if err != nil {
		return err
	}
	candidates, err := collectExternalLinks(doc)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM external_link_usage WHERE page_id = $1`, pageID); err != nil {
		return fmt.Errorf("projection: 清空页面 %s 的外链投影失败: %w", pageID, err)
	}
	for _, candidate := range candidates {
		resource, err := b.resources.UpsertInTx(ctx, tx, candidate.url)
		if errors.Is(err, evidence.ErrInvalidURL) {
			// 历史/外来 AST 可能含 NormalizeURL 不接受的 URI。投影是派生数据，
			// 单个坏链接降级跳过，不能阻断同页其余合法外链重建。
			b.logger.Warn("跳过无法规范化的外链",
				slog.String("page_id", pageID.String()),
				slog.String("revision_id", revisionID.String()),
				slog.String("block_id", candidate.blockID.String()),
				slog.String("node_id", candidate.nodeID),
				slog.Any("error", err))
			continue
		}
		if err != nil {
			return fmt.Errorf("projection: upsert 页面 %s 外链资源失败: %w", pageID, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO external_link_usage (
				external_resource_id, page_id, revision_id, block_id, node_id, link_role)
			VALUES ($1, $2, $3, $4, $5, 'inline')`,
			resource.ID, pageID, revisionID, candidate.blockID, candidate.nodeID); err != nil {
			return fmt.Errorf("projection: 写入页面 %s 的外链投影失败: %w", pageID, err)
		}
	}
	return nil
}

// HandleEvent 实现 Builder：框架默认（读当前 Revision 重建）。
func (b *ExternalLinksBuilder) HandleEvent(ctx context.Context, event Event) error {
	return HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}

func collectExternalLinks(doc *ast.Document) ([]externalLinkCandidate, error) {
	var candidates []externalLinkCandidate
	var candidateErr error
	walkErr := ast.Walk(doc, func(node ast.WalkNode) bool {
		if node.Inline == nil || node.Inline.Type != ast.InlineExternalLink {
			return true
		}
		blockID, err := uuid.Parse(node.Parent.ID)
		if err != nil {
			candidateErr = fmt.Errorf("projection: external_link 所属 Block ID %q 非法: %w", node.Parent.ID, err)
			return false
		}
		candidates = append(candidates, externalLinkCandidate{
			blockID: blockID,
			nodeID:  strconv.Itoa(node.Index),
			url:     node.Inline.URL,
		})
		return true
	})
	if walkErr != nil {
		return nil, fmt.Errorf("projection: 遍历 AST 收集外链失败: %w", walkErr)
	}
	if candidateErr != nil {
		return nil, candidateErr
	}
	return candidates, nil
}
