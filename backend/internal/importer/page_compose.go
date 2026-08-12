package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/governance"
)

func (c *ProposalComposer) buildPageOperations(
	ctx context.Context,
	params ComposeParams,
) ([]governance.OperationV1, error) {
	if params.Plan == nil {
		return nil, ErrInvalidJob
	}
	namespaceID, err := c.pages.NamespaceID(ctx, params.WikiID, "main")
	if err != nil {
		return nil, err
	}
	operations := make([]governance.OperationV1, 0)
	for _, route := range params.Plan.Routes {
		switch route.Action {
		case RouteCreate:
			op, err := c.buildCreatePageOperation(
				ctx, params, namespaceID, route, relatedPageTargets(params.Plan, route.Title),
			)
			if err != nil {
				return nil, err
			}
			operations = append(operations, op)
		case RouteUpdate:
			pageOps, err := c.buildUpdatePageOperations(
				ctx, params, route, relatedPageTargets(params.Plan, route.Title),
			)
			if err != nil {
				return nil, err
			}
			operations = append(operations, pageOps...)
		case RouteLink, RouteIgnore:
			// Link intents are compiled into their related create/update route;
			// ignore remains immutable planning metadata only.
		default:
			return nil, ErrInvalidJob
		}
	}
	return operations, nil
}

func (c *ProposalComposer) buildCreatePageOperation(
	ctx context.Context,
	params ComposeParams,
	namespaceID uuid.UUID,
	route PageRoute,
	linkTargets []relatedPageTarget,
) (governance.OperationV1, error) {
	pageID, err := c.ids.New()
	if err != nil {
		return governance.OperationV1{}, err
	}
	linkTargets = filterRelatedPageTargets(pageID, linkTargets, nil)
	document := ast.NewDocument()
	evidenceItems := make([]governance.OperationEvidence, 0)
	for _, planned := range route.Blocks {
		citationIDs, blockEvidence, err := c.createCitations(ctx, params, planned.Evidence)
		if err != nil {
			return governance.OperationV1{}, err
		}
		block, err := c.compilePlannedBlock(planned, citationIDs)
		if err != nil {
			return governance.OperationV1{}, err
		}
		document.Children = append(document.Children, block)
		evidenceItems = appendOperationEvidence(evidenceItems, blockEvidence)
	}
	linkCitationIDs, linkEvidence, err := c.createCitations(
		ctx, params, relatedPageEvidence(linkTargets),
	)
	if err != nil {
		return governance.OperationV1{}, err
	}
	related, err := c.compileRelatedPagesBlock(
		params.Plan.Profile.Language, pageID, linkTargets, nil, linkCitationIDs,
	)
	if err != nil {
		return governance.OperationV1{}, err
	}
	if related != nil {
		document.Children = append(document.Children, related)
		evidenceItems = appendOperationEvidence(evidenceItems, linkEvidence)
	}
	initialAST, err := ast.CanonicalJSON(document)
	if err != nil {
		return governance.OperationV1{}, err
	}
	baseVersion := 0
	payload, err := json.Marshal(map[string]any{
		"title":         route.Title,
		"language":      importPlanLanguage(params.Plan.Profile.Language),
		"content_model": "block-v1",
		"initial_ast":   json.RawMessage(initialAST),
		"summary":       boundedRunes("AI 导入："+route.Reason, 1000),
	})
	if err != nil {
		return governance.OperationV1{}, err
	}
	return governance.OperationV1{
		SchemaVersion: 1,
		OperationType: governance.OpCreatePage,
		Base:          governance.OperationBase{StateVersion: &baseVersion},
		Target: governance.OperationTarget{
			WikiID: &params.WikiID, NamespaceID: &namespaceID, PageID: &pageID,
		},
		Evidence: evidenceItems,
		Risk: governance.OperationRisk{
			Level: governance.RiskMedium,
			Reasons: []string{
				"AI import creates a new encyclopedia page",
				route.Reason,
			},
		},
		Payload: payload,
	}, nil
}

func (c *ProposalComposer) buildUpdatePageOperations(
	ctx context.Context,
	params ComposeParams,
	route PageRoute,
	linkTargets []relatedPageTarget,
) ([]governance.OperationV1, error) {
	if route.PageID == nil || *route.PageID == uuid.Nil {
		return nil, ErrInvalidJob
	}
	revision, snapshot, err := c.pages.CurrentContent(ctx, *route.PageID)
	if err != nil {
		return nil, err
	}
	document := ast.NewDocument()
	if snapshot != nil {
		document, err = ast.Parse(snapshot.AST)
		if err != nil {
			return nil, err
		}
	}
	base := governance.OperationBase{}
	if revision != nil {
		base.RevisionID = &revision.ID
	} else {
		version := 0
		base.StateVersion = &version
	}
	appendIndex := len(document.Children)
	operations := make([]governance.OperationV1, 0, len(route.Blocks))
	for _, planned := range route.Blocks {
		citationIDs, blockEvidence, err := c.createCitations(ctx, params, planned.Evidence)
		if err != nil {
			return nil, err
		}
		block, err := c.compilePlannedBlock(planned, citationIDs)
		if err != nil {
			return nil, err
		}
		operation := governance.OperationV1{
			SchemaVersion: 1,
			Base:          base,
			Target: governance.OperationTarget{
				WikiID: &params.WikiID, PageID: route.PageID,
			},
			Evidence: blockEvidence,
			Risk: governance.OperationRisk{
				Level: governance.RiskMedium,
				Reasons: []string{
					"AI import updates an existing encyclopedia page",
					route.Reason,
				},
			},
		}
		if planned.Mode == BlockReplace {
			if planned.TargetBlockID == nil {
				return nil, ErrInvalidJob
			}
			location, ok := ast.FindBlock(document, *planned.TargetBlockID)
			if !ok {
				return nil, fmt.Errorf("%w: block=%s", ErrPlanTargetConflict, *planned.TargetBlockID)
			}
			hash, err := governance.BlockHash(location.Block)
			if err != nil {
				return nil, err
			}
			operation.OperationType = governance.OpReplaceBlock
			operation.Target.BlockID = planned.TargetBlockID
			operation.ExpectedHash = &hash
			// A replacement preserves the stable block identity so anchors and
			// references do not break merely because AI rewrote its contents.
			block.ID = *planned.TargetBlockID
			operation.Payload, err = json.Marshal(map[string]any{"block": block})
			if err != nil {
				return nil, err
			}
		} else {
			operation.OperationType = governance.OpInsertBlock
			operation.Payload, err = json.Marshal(map[string]any{
				"parent_block_id": nil,
				"index":           appendIndex,
				"block":           block,
			})
			if err != nil {
				return nil, err
			}
			appendIndex++
		}
		operations = append(operations, operation)
	}
	existingReferences, err := resolvedPageReferences(document)
	if err != nil {
		return nil, err
	}
	linkTargets = filterRelatedPageTargets(*route.PageID, linkTargets, existingReferences)
	linkCitationIDs, linkEvidence, err := c.createCitations(
		ctx, params, relatedPageEvidence(linkTargets),
	)
	if err != nil {
		return nil, err
	}
	related, err := c.compileRelatedPagesBlock(
		params.Plan.Profile.Language, *route.PageID, linkTargets, existingReferences, linkCitationIDs,
	)
	if err != nil {
		return nil, err
	}
	if related != nil {
		payload, err := json.Marshal(map[string]any{
			"parent_block_id": nil,
			"index":           appendIndex,
			"block":           related,
		})
		if err != nil {
			return nil, err
		}
		operations = append(operations, governance.OperationV1{
			SchemaVersion: 1,
			OperationType: governance.OpInsertBlock,
			Base:          base,
			Target: governance.OperationTarget{
				WikiID: &params.WikiID, PageID: route.PageID,
			},
			Evidence: linkEvidence,
			Risk: governance.OperationRisk{
				Level:   governance.RiskLow,
				Reasons: []string{"import plan adds reviewed related-page references"},
			},
			Payload: payload,
		})
	}
	return operations, nil
}

type relatedPageTarget struct {
	PageID   uuid.UUID
	Title    string
	Evidence []CandidateEvidence
}

func relatedPageTargets(plan *ImportPlan, sourceTitle string) []relatedPageTarget {
	if plan == nil {
		return nil
	}
	sourceKey := normalizedPlanText(sourceTitle)
	result := make([]relatedPageTarget, 0)
	seen := map[uuid.UUID]bool{}
	for _, route := range plan.Routes {
		if route.Action != RouteLink || route.PageID == nil || *route.PageID == uuid.Nil ||
			seen[*route.PageID] || !relatedTitleContains(route.RelatedTo, sourceKey) {
			continue
		}
		seen[*route.PageID] = true
		result = append(result, relatedPageTarget{
			PageID: *route.PageID, Title: route.Title, Evidence: route.Evidence,
		})
	}
	return result
}

func relatedTitleContains(titles []string, wanted string) bool {
	for _, title := range titles {
		if normalizedPlanText(title) == wanted {
			return true
		}
	}
	return false
}

func relatedPageEvidence(targets []relatedPageTarget) []CandidateEvidence {
	result := make([]CandidateEvidence, 0)
	for _, target := range targets {
		result = mergeCandidateEvidence(result, target.Evidence)
		if len(result) >= 8 {
			return result[:8]
		}
	}
	return result
}

func filterRelatedPageTargets(
	sourcePageID uuid.UUID,
	targets []relatedPageTarget,
	existing map[uuid.UUID]bool,
) []relatedPageTarget {
	filtered := make([]relatedPageTarget, 0, len(targets))
	seen := map[uuid.UUID]bool{}
	for _, target := range targets {
		if target.PageID == uuid.Nil || target.PageID == sourcePageID || existing[target.PageID] ||
			seen[target.PageID] || strings.TrimSpace(target.Title) == "" {
			continue
		}
		seen[target.PageID] = true
		filtered = append(filtered, target)
	}
	return filtered
}

func resolvedPageReferences(document *ast.Document) (map[uuid.UUID]bool, error) {
	result := map[uuid.UUID]bool{}
	err := ast.Walk(document, func(node ast.WalkNode) bool {
		if node.Inline == nil || node.Inline.Type != ast.InlinePageReference || node.Inline.TargetPageID == "" {
			return true
		}
		if target, err := uuid.Parse(node.Inline.TargetPageID); err == nil {
			result[target] = true
		}
		return true
	})
	return result, err
}

func (c *ProposalComposer) compileRelatedPagesBlock(
	language string,
	sourcePageID uuid.UUID,
	targets []relatedPageTarget,
	existing map[uuid.UUID]bool,
	citationIDs []uuid.UUID,
) (*ast.Block, error) {
	filtered := filterRelatedPageTargets(sourcePageID, targets, existing)
	if len(filtered) == 0 {
		return nil, nil
	}
	blockID, err := c.ids.NewString()
	if err != nil {
		return nil, err
	}
	prefix := "相关页面："
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		prefix = "Related pages: "
	}
	nodes := []*ast.InlineNode{{Type: ast.InlineText, Text: prefix}}
	for index, target := range filtered {
		if index > 0 {
			separator := "、"
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
				separator = ", "
			}
			nodes = append(nodes, &ast.InlineNode{Type: ast.InlineText, Text: separator})
		}
		nodes = append(nodes, &ast.InlineNode{
			Type: ast.InlinePageReference, TargetPageID: target.PageID.String(), DisplayText: target.Title,
		})
	}
	for _, citationID := range citationIDs {
		node, err := ast.NewCitationRefNode(citationID.String(), "")
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	content, err := json.Marshal(nodes)
	if err != nil {
		return nil, err
	}
	block := &ast.Block{ID: blockID, Type: ast.BlockParagraph, Content: content}
	if err := ast.Validate(&ast.Document{Type: "document", SchemaVersion: 1, Children: []*ast.Block{block}}); err != nil {
		return nil, err
	}
	return block, nil
}

func (c *ProposalComposer) compilePlannedBlock(planned PlannedBlock, citationIDs []uuid.UUID) (*ast.Block, error) {
	blockID, err := c.ids.NewString()
	if err != nil {
		return nil, err
	}
	switch planned.Type {
	case string(ast.BlockHeading):
		block, err := c.textBlock(blockID, ast.BlockHeading, strings.TrimSpace(planned.Text), citationIDs)
		if err != nil {
			return nil, err
		}
		block.Level = planned.Level
		if err := ast.Validate(&ast.Document{Type: "document", SchemaVersion: 1, Children: []*ast.Block{block}}); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockParagraph):
		return c.textBlock(blockID, ast.BlockParagraph, strings.TrimSpace(planned.Text), citationIDs)

	case string(ast.BlockQuote):
		paragraphID, err := c.ids.NewString()
		if err != nil {
			return nil, err
		}
		paragraph, err := c.textBlock(paragraphID, ast.BlockParagraph, strings.TrimSpace(planned.Text), citationIDs)
		if err != nil {
			return nil, err
		}
		block := &ast.Block{ID: blockID, Type: ast.BlockQuote, Children: []*ast.Block{paragraph}}
		if err := ast.Validate(&ast.Document{Type: "document", SchemaVersion: 1, Children: []*ast.Block{block}}); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockBulletList):
		list := &ast.Block{ID: blockID, Type: ast.BlockBulletList, Children: []*ast.Block{}}
		for index, rawItem := range planned.Items {
			itemID, err := c.ids.NewString()
			if err != nil {
				return nil, err
			}
			paragraphID, err := c.ids.NewString()
			if err != nil {
				return nil, err
			}
			itemCitations := []uuid.UUID{}
			if index == len(planned.Items)-1 {
				itemCitations = citationIDs
			}
			paragraph, err := c.textBlock(paragraphID, ast.BlockParagraph, strings.TrimSpace(rawItem), itemCitations)
			if err != nil {
				return nil, err
			}
			list.Children = append(list.Children, &ast.Block{
				ID: itemID, Type: ast.BlockListItem, Children: []*ast.Block{paragraph},
			})
		}
		if err := ast.Validate(&ast.Document{Type: "document", SchemaVersion: 1, Children: []*ast.Block{list}}); err != nil {
			return nil, err
		}
		return list, nil
	default:
		return nil, ErrInvalidJob
	}
}

func (c *ProposalComposer) textBlock(
	blockID string,
	blockType ast.BlockType,
	text string,
	citationIDs []uuid.UUID,
) (*ast.Block, error) {
	nodes := []*ast.InlineNode{{Type: ast.InlineText, Text: text}}
	for _, citationID := range citationIDs {
		node, err := ast.NewCitationRefNode(citationID.String(), "")
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	content, err := json.Marshal(nodes)
	if err != nil {
		return nil, err
	}
	return &ast.Block{ID: blockID, Type: blockType, Content: content}, nil
}

func appendOperationEvidence(
	existing []governance.OperationEvidence,
	incoming []governance.OperationEvidence,
) []governance.OperationEvidence {
	seen := map[string]bool{}
	for _, item := range existing {
		seen[operationEvidenceKey(item)] = true
	}
	for _, item := range incoming {
		key := operationEvidenceKey(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		existing = append(existing, item)
	}
	return existing
}

func operationEvidenceKey(item governance.OperationEvidence) string {
	citation, chunk := "", ""
	if item.CitationID != nil {
		citation = item.CitationID.String()
	}
	if item.SourceChunkID != nil {
		chunk = item.SourceChunkID.String()
	}
	return citation + "\x00" + chunk + "\x00" + item.Note
}

func importPlanLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" || len([]rune(language)) > 32 {
		return "zh-Hans"
	}
	return language
}

func boundedRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
