// 页面链接与目录投影 Builder（M3-T03，设计 §18.9、§9/§18.1、§17.3）。
//
//   - PageLinksBuilder（type "page_links"）：Walk AST 收集全部 page_reference
//     行内节点写入 page_link_projection（已解析/未解析两态）；
//     external_link 不在本 Builder（属 M3-T06）。
//   - OutlineBuilder（type "document_outline"）：heading 层级树写入
//     document_outline_projection，同事务写 page_anchor（current_slug）。
//
// 两个 Builder 都按页全删重插（幂等），投影可丢弃、可经 RebuildPage 重建。
package projection

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
)

// PageLinksBuilder 页面引用投影 Builder（page_link_projection，设计 §18.9）。
type PageLinksBuilder struct {
	pool *pgxpool.Pool
}

// NewPageLinksBuilder 装配页面引用投影 Builder。
func NewPageLinksBuilder(pool *pgxpool.Pool) *PageLinksBuilder {
	return &PageLinksBuilder{pool: pool}
}

// Type 实现 Builder。
func (b *PageLinksBuilder) Type() string { return "page_links" }

// linkRow 一条待写入的 page_link_projection 行。
type linkRow struct {
	sourceBlockID       uuid.UUID
	sourceNodeID        string
	targetPageID        *uuid.UUID
	targetNamespaceKey  string // 未解析引用的 namespace key（落库时解析为 id，解析不了为 NULL）
	targetTitle         *string
	targetAnchorBlockID *uuid.UUID
	resolutionStatus    string
	displayText         string
}

// Rebuild 实现 Builder：Walk AST 收集全部 page_reference，tx 内按 source_page_id
// 全删重插（幂等）。
func (b *PageLinksBuilder) Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	doc, err := RevisionAST(ctx, tx, revisionID)
	if err != nil {
		return err
	}

	var rows []linkRow
	var rowErr error
	werr := ast.Walk(doc, func(n ast.WalkNode) bool {
		if n.Inline == nil || n.Inline.Type != ast.InlinePageReference {
			return true
		}
		row, err := pageReferenceRow(n.Parent, n.Inline, n.Index)
		if err != nil {
			rowErr = err
			return false
		}
		rows = append(rows, row)
		return true
	})
	if werr != nil {
		return fmt.Errorf("projection: 遍历 AST 收集页面引用失败: %w", werr)
	}
	if rowErr != nil {
		return rowErr
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM page_link_projection WHERE source_page_id = $1`, pageID); err != nil {
		return fmt.Errorf("projection: 清空页面 %s 的链接投影失败: %w", pageID, err)
	}
	for _, r := range rows {
		// 未解析引用：target_namespace key 在同一 INSERT 内经子查询解析为 id
		// （页面所属 wiki 内的 namespace；解析不了落 NULL）。已解析行 key 为空串，
		// 子查询不命中同样落 NULL。
		if _, err := tx.Exec(ctx, `
			INSERT INTO page_link_projection (
				source_page_id, source_revision_id, source_block_id, source_node_id,
				target_page_id, target_namespace_id, target_title, target_anchor_block_id,
				resolution_status, display_text)
			VALUES ($1, $2, $3, $4, $5,
				(SELECT n.id FROM namespace n JOIN page p ON p.wiki_id = n.wiki_id
				 WHERE p.id = $1 AND n.namespace_key = $6),
				$7, $8, $9, $10)`,
			pageID, revisionID, r.sourceBlockID, r.sourceNodeID,
			r.targetPageID, r.targetNamespaceKey, r.targetTitle, r.targetAnchorBlockID,
			r.resolutionStatus, r.displayText); err != nil {
			return fmt.Errorf("projection: 写入页面 %s 的链接投影失败: %w", pageID, err)
		}
	}
	return nil
}

// HandleEvent 实现 Builder：框架默认（读当前 Revision 重建）。
func (b *PageLinksBuilder) HandleEvent(ctx context.Context, event Event) error {
	return HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}

// pageReferenceRow 把一个 page_reference 行内节点转成投影行。
// block 是节点所属 Block；index 是节点在 Block content 内的下标（v1 行内层扁平，
// source_node_id 即其十进制形式）。
func pageReferenceRow(block *ast.Block, node *ast.InlineNode, index int) (linkRow, error) {
	blockID, err := uuid.Parse(block.ID)
	if err != nil {
		return linkRow{}, fmt.Errorf("projection: Block ID %q 非法: %w", block.ID, err)
	}
	row := linkRow{
		sourceBlockID: blockID,
		sourceNodeID:  strconv.Itoa(index),
	}
	if node.ResolutionStatus == ast.ResolutionUnresolved {
		// 未解析引用（设计 §5.2）：namespace key 留待落库时解析为 id；
		// 无 display_text 字段，展示文本取 normalized_title。
		row.resolutionStatus = "unresolved"
		row.targetNamespaceKey = node.TargetNamespace
		title := node.NormalizedTitle
		row.targetTitle = &title
		row.displayText = node.NormalizedTitle
		return row, nil
	}
	// 已解析引用（设计 §5.1）。
	targetID, err := uuid.Parse(node.TargetPageID)
	if err != nil {
		return linkRow{}, fmt.Errorf("projection: 已解析引用 target_page_id %q 非法: %w", node.TargetPageID, err)
	}
	row.resolutionStatus = "resolved"
	row.targetPageID = &targetID
	if node.TargetHeadingBlockID != "" {
		anchorID, err := uuid.Parse(node.TargetHeadingBlockID)
		if err != nil {
			return linkRow{}, fmt.Errorf("projection: 引用锚点 %q 非法: %w", node.TargetHeadingBlockID, err)
		}
		row.targetAnchorBlockID = &anchorID
	}
	row.displayText = node.DisplayText
	return row, nil
}

// OutlineBuilder 文档大纲投影 Builder（document_outline_projection + page_anchor，
// 设计 §18.9、§9/§18.1）。
type OutlineBuilder struct {
	pool *pgxpool.Pool
}

// NewOutlineBuilder 装配文档大纲投影 Builder。
func NewOutlineBuilder(pool *pgxpool.Pool) *OutlineBuilder {
	return &OutlineBuilder{pool: pool}
}

// Type 实现 Builder。
func (b *OutlineBuilder) Type() string { return "document_outline" }

// outlineRow 一条待写入的大纲/锚点行（两表同构，page_anchor 多 current_slug）。
type outlineRow struct {
	headingBlockID       uuid.UUID
	parentHeadingBlockID *uuid.UUID
	level                int
	title                string
	slug                 string
	positionKey          string
}

// Rebuild 实现 Builder：按文档顺序遍历 heading Block，parent 按 level 栈推导、
// position_key 为同级序号路径（'1.2.3'），slug 由标题确定性生成；
// tx 内按 page_id 全删重插 document_outline_projection 与 page_anchor（幂等）。
func (b *OutlineBuilder) Rebuild(ctx context.Context, tx pgx.Tx, pageID, revisionID uuid.UUID) error {
	doc, err := RevisionAST(ctx, tx, revisionID)
	if err != nil {
		return err
	}
	rows, err := buildOutlineRows(doc)
	if err != nil {
		return err
	}

	// Preserve historical slugs before replacing the current projection.
	if _, err := tx.Exec(ctx, `INSERT INTO page_anchor_alias
		(page_id,alias_slug,heading_block_id,source_revision_id)
		SELECT page_id,current_slug,heading_block_id,revision_id
		FROM page_anchor WHERE page_id=$1
		ON CONFLICT (page_id,alias_slug) DO NOTHING`, pageID); err != nil {
		return fmt.Errorf("projection: 保存页面 %s 的历史锚点失败: %w", pageID, err)
	}
	for _, table := range []string{"document_outline_projection", "page_anchor"} {
		if _, err := tx.Exec(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE page_id = $1`, table), pageID); err != nil {
			return fmt.Errorf("projection: 清空页面 %s 的 %s 失败: %w", pageID, table, err)
		}
	}
	for _, r := range rows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_outline_projection (
				page_id, revision_id, heading_block_id, parent_heading_block_id,
				level, title, position_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			pageID, revisionID, r.headingBlockID, r.parentHeadingBlockID,
			r.level, r.title, r.positionKey); err != nil {
			return fmt.Errorf("projection: 写入页面 %s 的大纲投影失败: %w", pageID, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO page_anchor (
				page_id, revision_id, heading_block_id, parent_heading_block_id,
				level, title, current_slug, position_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			pageID, revisionID, r.headingBlockID, r.parentHeadingBlockID,
			r.level, r.title, r.slug, r.positionKey); err != nil {
			return fmt.Errorf("projection: 写入页面 %s 的锚点失败: %w", pageID, err)
		}
	}
	return nil
}

// HandleEvent 实现 Builder：框架默认（读当前 Revision 重建）。
func (b *OutlineBuilder) HandleEvent(ctx context.Context, event Event) error {
	return HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}

// headingFrame level 栈中的一层：最近的未闭合 heading。
type headingFrame struct {
	blockID uuid.UUID
	level   int
}

// buildOutlineRows 从 AST 推导大纲行（纯计算，可单测）：
//   - parent：弹栈至栈顶 level < 当前 level 后的栈顶（跳级如 H1→H3 时父为 H1）；
//   - position_key：同级 1 起编号的序号路径（'1.2.3'），栈深即路径深度；
//   - slug：slugAssigner 按文档顺序分配（重复标题加 -2/-3 后缀）。
func buildOutlineRows(doc *ast.Document) ([]outlineRow, error) {
	var rows []outlineRow
	var stack []headingFrame
	var counters []int
	slugs := newSlugAssigner()

	var rowErr error
	werr := ast.Walk(doc, func(n ast.WalkNode) bool {
		if n.Block == nil || n.Block.Type != ast.BlockHeading {
			return true
		}
		blk := n.Block
		blockID, err := uuid.Parse(blk.ID)
		if err != nil {
			rowErr = fmt.Errorf("projection: heading Block ID %q 非法: %w", blk.ID, err)
			return false
		}
		title, err := headingPlainText(blk)
		if err != nil {
			rowErr = err
			return false
		}

		for len(stack) > 0 && stack[len(stack)-1].level >= blk.Level {
			stack = stack[:len(stack)-1]
		}
		var parentID *uuid.UUID
		if len(stack) > 0 {
			id := stack[len(stack)-1].blockID
			parentID = &id
		}

		depth := len(stack)
		// counters[i] 为深度 i 处的同级序号；同级再次出现时在原计数上累加，
		// 更深层级的计数在回到浅层时截断重置。
		if len(counters) > depth+1 {
			counters = counters[:depth+1]
		}
		for len(counters) < depth+1 {
			counters = append(counters, 0)
		}
		counters[depth]++
		parts := make([]string, len(counters))
		for i, c := range counters {
			parts[i] = strconv.Itoa(c)
		}

		rows = append(rows, outlineRow{
			headingBlockID:       blockID,
			parentHeadingBlockID: parentID,
			level:                blk.Level,
			title:                title,
			slug:                 slugs.assign(title),
			positionKey:          strings.Join(parts, "."),
		})
		stack = append(stack, headingFrame{blockID: blockID, level: blk.Level})
		return true
	})
	if werr != nil {
		return nil, fmt.Errorf("projection: 遍历 AST 推导大纲失败: %w", werr)
	}
	if rowErr != nil {
		return nil, rowErr
	}
	return rows, nil
}

// headingPlainText 提取 heading 的纯文本标题（行内节点拼接）：
// text/inline_code 取 text；page_reference 取 display_text（未解析取 normalized_title）；
// external_link 取 display_text。
func headingPlainText(blk *ast.Block) (string, error) {
	nodes, err := blk.InlineContent()
	if err != nil {
		return "", fmt.Errorf("projection: 解码 heading %s 的行内内容失败: %w", blk.ID, err)
	}
	var b strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case ast.InlineText, ast.InlineCode:
			b.WriteString(n.Text)
		case ast.InlinePageReference:
			if n.ResolutionStatus == ast.ResolutionUnresolved {
				b.WriteString(n.NormalizedTitle)
			} else {
				b.WriteString(n.DisplayText)
			}
		case ast.InlineExternalLink, ast.InlineEntityReference, ast.InlineClaimReference,
			ast.InlineCitationReference:
			b.WriteString(n.DisplayText)
		}
	}
	return b.String(), nil
}
