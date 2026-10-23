// RenderedPage 投影 Builder（M3-T05，设计 §15、§17.1 阅读路径）。
//
// RenderedPageBuilder（type "rendered_page"）：取当前 Revision 的权威 AST 经
// render.RenderHTML 渲染为安全 HTML 片段，UPSERT 到 rendered_page（每页一行，
// Worker 覆盖写）。投影记录 revision_id / renderer_version / content_hash，
// 阅读路径（cmd/api/reading.go）在三者匹配时优先返回投影 HTML，否则实时渲染兜底。
package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/render"
)

// RenderedPageBuilder RenderedPage 投影 Builder（rendered_page，设计 §17.1）。
type RenderedPageBuilder struct {
	pool    *pgxpool.Pool
	dynamic render.DynamicRenderer
}

// NewRenderedPageBuilder 装配 RenderedPage 投影 Builder。
func NewRenderedPageBuilder(pool *pgxpool.Pool) *RenderedPageBuilder {
	return &RenderedPageBuilder{pool: pool, dynamic: newComponentHTMLRenderer(pool)}
}

// Type 实现 Builder。
func (b *RenderedPageBuilder) Type() string { return "rendered_page" }

// Rebuild 实现 Builder：tx 内读权威 AST → RenderHTML → UPSERT rendered_page。
// 幂等：同一 (pageID, revisionID) 重复执行产出同一行。
//
// 覆盖语义：Rebuild 总是以调用方给定的当前 Revision 为准直接覆盖——事件侧的
// 「旧 Revision 不得覆盖新投影」由 M3-T02 版本防护（WithVersionGuard）在处理
// 前后断言保证；RebuildPage 直调时 revisionID 即锁内读到的页面当前 Revision。
func (b *RenderedPageBuilder) Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	doc, err := RevisionAST(ctx, tx, revisionID)
	if err != nil {
		return err
	}
	if err := applyResolvedPageReferences(ctx, tx, pageID, revisionID, doc); err != nil {
		return err
	}
	html, err := render.RenderHTMLWithResolvers(
		ctx,
		doc,
		b.dynamic,
		b.dynamic,
	)
	if err != nil {
		return fmt.Errorf("projection: 渲染页面 %s（revision %s）失败: %w", pageID, revisionID, err)
	}
	hash, err := ast.ContentHash(doc)
	if err != nil {
		return fmt.Errorf("projection: 计算页面 %s（revision %s）content hash 失败: %w", pageID, revisionID, err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO rendered_page (page_id, revision_id, renderer_version, html_content, content_hash)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (page_id) DO UPDATE SET
			revision_id      = EXCLUDED.revision_id,
			renderer_version = EXCLUDED.renderer_version,
			html_content     = EXCLUDED.html_content,
			content_hash     = EXCLUDED.content_hash,
			created_at       = now()`,
		pageID, revisionID, render.RendererVersion, html, hash); err != nil {
		return fmt.Errorf("projection: 写入页面 %s 的渲染投影失败: %w", pageID, err)
	}
	return nil
}

type renderedPageReference struct {
	targetPageID        uuid.UUID
	targetAnchorBlockID *uuid.UUID
	displayText         string
}

func applyResolvedPageReferences(
	ctx context.Context,
	tx pgx.Tx,
	pageID uuid.UUID,
	revisionID uuid.UUID,
	doc *ast.Document,
) error {
	rows, err := tx.Query(ctx, `
		SELECT source_block_id,source_node_id,target_page_id,
		       target_anchor_block_id,display_text
		FROM page_link_projection
		WHERE source_page_id=$1 AND source_revision_id=$2
		  AND resolution_status='resolved' AND target_page_id IS NOT NULL`,
		pageID, revisionID)
	if err != nil {
		return fmt.Errorf("projection: query resolved page references: %w", err)
	}
	defer rows.Close()
	resolved := map[string]renderedPageReference{}
	for rows.Next() {
		var blockID uuid.UUID
		var nodeID string
		var ref renderedPageReference
		if err := rows.Scan(
			&blockID,
			&nodeID,
			&ref.targetPageID,
			&ref.targetAnchorBlockID,
			&ref.displayText,
		); err != nil {
			return fmt.Errorf("projection: scan resolved page reference: %w", err)
		}
		resolved[blockID.String()+"\x00"+nodeID] = ref
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("projection: iterate resolved page references: %w", err)
	}
	if len(resolved) == 0 {
		return nil
	}
	return applyResolvedPageReferencesToBlocks(doc.Children, resolved)
}

func applyResolvedPageReferencesToBlocks(
	blocks []*ast.Block,
	resolved map[string]renderedPageReference,
) error {
	for _, block := range blocks {
		if len(block.Content) > 0 && block.Content[0] == '[' {
			nodes, err := block.InlineContent()
			if err != nil {
				return err
			}
			changed := false
			for index, node := range nodes {
				if node.Type != ast.InlinePageReference ||
					node.ResolutionStatus != ast.ResolutionUnresolved {
					continue
				}
				ref, ok := resolved[block.ID+"\x00"+strconv.Itoa(index)]
				if !ok {
					continue
				}
				if ref.displayText == "" {
					ref.displayText = node.NormalizedTitle
				}
				node.TargetPageID = ref.targetPageID.String()
				node.TargetHeadingBlockID = ""
				if ref.targetAnchorBlockID != nil {
					node.TargetHeadingBlockID = ref.targetAnchorBlockID.String()
				}
				node.DisplayText = ref.displayText
				node.ResolutionStatus = ""
				node.TargetNamespace = ""
				node.NormalizedTitle = ""
				node.ExpectedEntityType = ""
				changed = true
			}
			if changed {
				raw, err := json.Marshal(nodes)
				if err != nil {
					return fmt.Errorf("projection: encode resolved inline references: %w", err)
				}
				block.Content = raw
			}
		}
		if err := applyResolvedPageReferencesToBlocks(block.Children, resolved); err != nil {
			return err
		}
	}
	return nil
}

// HandleEvent 实现 Builder：框架默认（读当前 Revision 重建）。
func (b *RenderedPageBuilder) HandleEvent(ctx context.Context, event Event) error {
	return HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}
