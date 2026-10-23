package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/component"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
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
	entityReferences := entityReferencesForCompose(params)
	operations := make([]governance.OperationV1, 0)
	createPageIDs := map[string]uuid.UUID{}
	for _, route := range params.Plan.Routes {
		if route.Action != RouteCreate {
			continue
		}
		pageID, err := c.ids.New()
		if err != nil {
			return nil, err
		}
		createPageIDs[normalizedPlanText(route.Title)] = pageID
	}
	for _, route := range params.Plan.Routes {
		switch route.Action {
		case RouteCreate:
			pageID, ok := createPageIDs[normalizedPlanText(route.Title)]
			if !ok || pageID == uuid.Nil {
				return nil, ErrInvalidJob
			}
			pageOps, err := c.buildCreatePageOperations(
				ctx, params, namespaceID, pageID, route,
				relatedPageTargets(params.Plan, route.Title, createPageIDs), entityReferences,
			)
			if err != nil {
				return nil, err
			}
			operations = append(operations, pageOps...)
		case RouteUpdate:
			pageOps, err := c.buildUpdatePageOperations(
				ctx, params, route,
				relatedPageTargets(params.Plan, route.Title, createPageIDs), entityReferences,
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

func (c *ProposalComposer) buildCreatePageOperations(
	ctx context.Context,
	params ComposeParams,
	namespaceID uuid.UUID,
	pageID uuid.UUID,
	route PageRoute,
	linkTargets []relatedPageTarget,
	entityReferences []entityReferenceTarget,
) ([]governance.OperationV1, error) {
	linkTargets = filterRelatedPageTargets(pageID, linkTargets, nil)
	document := ast.NewDocument()
	evidenceItems := make([]governance.OperationEvidence, 0)
	primaryEntity := primaryEntityForRoute(params, route)
	if primaryEntity != nil {
		componentID, componentVersion, err := c.articleInfoboxDefinition(
			ctx, params.WikiID,
		)
		if err != nil {
			return nil, err
		}
		infobox, err := c.newArticleInfoboxBlock(
			componentID, componentVersion, primaryEntity.EntityID,
			params.Plan.Profile.Language,
		)
		if err != nil {
			return nil, err
		}
		document.Children = append(document.Children, infobox)
	}
	for _, planned := range route.Blocks {
		citationIDs, blockEvidence, err := c.createCitations(
			ctx, params, plannedBlockEvidence(planned),
		)
		if err != nil {
			return nil, err
		}
		block, err := c.compilePlannedBlockWithReferences(planned, citationIDs, entityReferences)
		if err != nil {
			return nil, err
		}
		document.Children = append(document.Children, block)
		evidenceItems = appendOperationEvidence(evidenceItems, blockEvidence)
	}
	linkEvidence := operationEvidenceFromCandidates(relatedPageEvidence(linkTargets))
	related, err := c.compileRelatedPagesBlocks(
		params.Plan.Profile.Language, pageID, linkTargets, nil,
	)
	if err != nil {
		return nil, err
	}
	if len(related) > 0 {
		document.Children = append(document.Children, related...)
		evidenceItems = appendOperationEvidence(evidenceItems, linkEvidence)
	}
	initialAST, err := ast.CanonicalJSON(document)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	createOperation := governance.OperationV1{
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
	}
	operations := []governance.OperationV1{createOperation}
	if primaryEntity != nil {
		binding, err := buildSetPageEntityBindingOperation(
			params, pageID, primaryEntity, governance.OperationBase{
				StateVersion: &baseVersion,
			},
		)
		if err != nil {
			return nil, err
		}
		operations = append(operations, binding)
	}
	return operations, nil
}

func (c *ProposalComposer) buildUpdatePageOperations(
	ctx context.Context,
	params ComposeParams,
	route PageRoute,
	linkTargets []relatedPageTarget,
	entityReferences []entityReferenceTarget,
) ([]governance.OperationV1, error) {
	if route.PageID == nil || *route.PageID == uuid.Nil {
		return nil, ErrInvalidJob
	}
	revision, snapshot, err := c.pages.CurrentContent(ctx, *route.PageID)
	if err != nil {
		return nil, err
	}
	if route.PlannedRevisionID != nil && (revision == nil || revision.ID != *route.PlannedRevisionID) {
		return nil, fmt.Errorf("%w: page=%s revision changed since planning", ErrPlanTargetConflict, *route.PageID)
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
	endMatter := inspectArticleEndMatter(document)
	appendIndex := endMatter.FirstIndex
	if appendIndex < 0 {
		appendIndex = len(document.Children)
	}
	initialAppendIndex := appendIndex
	operations := make([]governance.OperationV1, 0, len(route.Blocks))
	primaryEntity := primaryEntityForRoute(params, route)
	for _, planned := range route.Blocks {
		citationIDs, blockEvidence, err := c.createCitations(
			ctx, params, plannedBlockEvidence(planned),
		)
		if err != nil {
			return nil, err
		}
		block, err := c.compilePlannedBlockWithReferences(planned, citationIDs, entityReferences)
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
			if planned.TargetBlockHash != nil && hash != *planned.TargetBlockHash {
				return nil, fmt.Errorf("%w: block=%s hash changed since planning", ErrPlanTargetConflict, *planned.TargetBlockID)
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
	linkEvidence := operationEvidenceFromCandidates(relatedPageEvidence(linkTargets))
	related, err := c.compileRelatedPagesBlocks(
		params.Plan.Profile.Language, *route.PageID, linkTargets, existingReferences,
	)
	if err != nil {
		return nil, err
	}
	if len(related) == 2 && endMatter.SeeAlsoList != nil {
		parentID := endMatter.SeeAlsoList.ID
		for index, item := range related[1].Children {
			operation, err := insertBlockOperationWithParent(
				params.WikiID, *route.PageID, base, &parentID,
				len(endMatter.SeeAlsoList.Children)+index, item,
				linkEvidence, "import plan extends the standard See also section",
			)
			if err != nil {
				return nil, err
			}
			operations = append(operations, operation)
		}
	} else if len(related) == 2 && endMatter.SeeAlsoIndex >= 0 {
		insertIndex := endMatter.SeeAlsoIndex +
			(appendIndex - initialAppendIndex) + 1
		operation, err := insertBlockOperation(
			params.WikiID, *route.PageID, base, insertIndex, related[1],
			linkEvidence, "import plan completes the standard See also section",
		)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	} else {
		for _, block := range related {
			operation, err := insertBlockOperation(
				params.WikiID, *route.PageID, base, appendIndex, block,
				linkEvidence, "import plan adds reviewed See also references",
			)
			if err != nil {
				return nil, err
			}
			operations = append(operations, operation)
			appendIndex++
		}
	}
	if primaryEntity != nil {
		componentID, componentVersion, err := c.articleInfoboxDefinition(
			ctx, params.WikiID,
		)
		if err != nil {
			return nil, err
		}
		existingInfobox := articleInfobox(document, componentID)
		if existingInfobox == nil {
			infobox, err := c.newArticleInfoboxBlock(
				componentID, componentVersion, primaryEntity.EntityID,
				params.Plan.Profile.Language,
			)
			if err != nil {
				return nil, err
			}
			operation, err := insertBlockOperation(
				params.WikiID, *route.PageID, base, 0, infobox,
				operationEvidenceFromCandidates(primaryEntity.Evidence),
				"import plan adds the standard article information box",
			)
			if err != nil {
				return nil, err
			}
			operations = append(operations, operation)
		} else if existingInfobox.EntityID != primaryEntity.EntityID.String() {
			infobox, err := c.newArticleInfoboxBlock(
				componentID, componentVersion, primaryEntity.EntityID,
				params.Plan.Profile.Language,
			)
			if err != nil {
				return nil, err
			}
			infobox.ID = existingInfobox.ID
			hash, err := governance.BlockHash(existingInfobox)
			if err != nil {
				return nil, err
			}
			blockID := existingInfobox.ID
			payload, err := json.Marshal(map[string]any{"block": infobox})
			if err != nil {
				return nil, err
			}
			operations = append(operations, governance.OperationV1{
				SchemaVersion: 1, OperationType: governance.OpReplaceBlock,
				Base: base, ExpectedHash: &hash,
				Target: governance.OperationTarget{
					WikiID: &params.WikiID, PageID: route.PageID,
					BlockID: &blockID,
				},
				Evidence: operationEvidenceFromCandidates(primaryEntity.Evidence),
				Risk: governance.OperationRisk{
					Level: governance.RiskMedium,
					Reasons: []string{
						"import plan aligns the article infobox with its primary Entity",
					},
				},
				Payload: payload,
			})
		}
		binding, err := buildSetPageEntityBindingOperation(
			params, *route.PageID, primaryEntity, base,
		)
		if err != nil {
			return nil, err
		}
		operations = append(operations, binding)
	}
	return operations, nil
}

type relatedPageTarget struct {
	PageID   uuid.UUID
	Title    string
	Evidence []CandidateEvidence
}

func relatedPageTargets(plan *ImportPlan, sourceTitle string, createPageIDs map[string]uuid.UUID) []relatedPageTarget {
	if plan == nil {
		return nil
	}
	sourceKey := normalizedPlanText(sourceTitle)
	result := make([]relatedPageTarget, 0)
	seen := map[uuid.UUID]bool{}
	for _, route := range plan.Routes {
		if route.Action != RouteLink || !relatedTitleContains(route.RelatedTo, sourceKey) {
			continue
		}
		pageID := uuid.Nil
		if route.PageID != nil {
			pageID = *route.PageID
		} else if createPageIDs != nil {
			pageID = createPageIDs[normalizedPlanText(route.Title)]
		}
		if pageID == uuid.Nil || seen[pageID] {
			continue
		}
		seen[pageID] = true
		result = append(result, relatedPageTarget{
			PageID: pageID, Title: route.Title, Evidence: route.Evidence,
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

func (c *ProposalComposer) compileRelatedPagesBlocks(
	language string,
	sourcePageID uuid.UUID,
	targets []relatedPageTarget,
	existing map[uuid.UUID]bool,
) ([]*ast.Block, error) {
	filtered := filterRelatedPageTargets(sourcePageID, targets, existing)
	if len(filtered) == 0 {
		return nil, nil
	}
	headingID, err := c.ids.NewString()
	if err != nil {
		return nil, err
	}
	title := "参见"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		title = "See also"
	}
	headingContent, err := json.Marshal([]*ast.InlineNode{{
		Type: ast.InlineText, Text: title,
	}})
	if err != nil {
		return nil, err
	}
	heading := &ast.Block{
		ID: headingID, Type: ast.BlockHeading, Level: 2,
		Content: headingContent,
	}
	listID, err := c.ids.NewString()
	if err != nil {
		return nil, err
	}
	list := &ast.Block{
		ID: listID, Type: ast.BlockBulletList, Children: []*ast.Block{},
	}
	for _, target := range filtered {
		itemID, err := c.ids.NewString()
		if err != nil {
			return nil, err
		}
		paragraphID, err := c.ids.NewString()
		if err != nil {
			return nil, err
		}
		content, err := json.Marshal([]*ast.InlineNode{{
			Type:         ast.InlinePageReference,
			TargetPageID: target.PageID.String(),
			DisplayText:  target.Title,
		}})
		if err != nil {
			return nil, err
		}
		list.Children = append(list.Children, &ast.Block{
			ID: itemID, Type: ast.BlockListItem,
			Children: []*ast.Block{{
				ID: paragraphID, Type: ast.BlockParagraph, Content: content,
			}},
		})
	}
	blocks := []*ast.Block{heading, list}
	if err := ast.Validate(&ast.Document{
		Type: "document", SchemaVersion: 1, Children: blocks,
	}); err != nil {
		return nil, err
	}
	return blocks, nil
}

func (c *ProposalComposer) newArticleInfoboxBlock(
	componentID uuid.UUID,
	componentVersion int,
	entityID uuid.UUID,
	language string,
) (*ast.Block, error) {
	blockID, err := c.ids.NewString()
	if err != nil {
		return nil, err
	}
	displayConfig, err := json.Marshal(map[string]any{
		"title": "", "language": importPlanLanguage(language),
		"property_keys": []string{},
	})
	if err != nil {
		return nil, err
	}
	block := &ast.Block{
		ID: blockID, Type: ast.BlockComponent,
		ComponentID:      componentID.String(),
		ComponentVersion: componentVersion,
		EntityID:         entityID.String(), DisplayConfig: displayConfig,
	}
	if err := ast.Validate(&ast.Document{
		Type: "document", SchemaVersion: ast.SchemaVersion,
		Children: []*ast.Block{block},
	}); err != nil {
		return nil, err
	}
	return block, nil
}

func (c *ProposalComposer) articleInfoboxDefinition(
	ctx context.Context,
	wikiID uuid.UUID,
) (uuid.UUID, int, error) {
	value, version, err := c.components.PublishedByKey(
		ctx, wikiID, component.BuiltinArticleInfoboxKey,
	)
	if err != nil {
		return uuid.Nil, 0, err
	}
	return value.ID, version.Version, nil
}

func articleInfobox(document *ast.Document, componentID uuid.UUID) *ast.Block {
	if document == nil {
		return nil
	}
	for _, block := range document.Children {
		if block.Type == ast.BlockComponent &&
			block.ComponentID == componentID.String() {
			return block
		}
	}
	return nil
}

type articleEndMatter struct {
	FirstIndex   int
	SeeAlsoIndex int
	SeeAlsoList  *ast.Block
}

func inspectArticleEndMatter(document *ast.Document) articleEndMatter {
	result := articleEndMatter{FirstIndex: -1, SeeAlsoIndex: -1}
	if document == nil {
		return result
	}
	for index, block := range document.Children {
		if block.Type != ast.BlockHeading || block.Level != 2 {
			continue
		}
		title := normalizedSentenceKey(blockInlineText(block))
		if !isArticleEndMatterTitle(title) {
			continue
		}
		if result.FirstIndex < 0 {
			result.FirstIndex = index
		}
		if !isSeeAlsoTitle(title) || result.SeeAlsoIndex >= 0 {
			continue
		}
		result.SeeAlsoIndex = index
		if index+1 < len(document.Children) {
			candidate := document.Children[index+1]
			if candidate.Type == ast.BlockBulletList || candidate.Type == ast.BlockOrderedList {
				result.SeeAlsoList = candidate
			}
		}
	}
	return result
}

func blockInlineText(block *ast.Block) string {
	nodes, err := block.InlineContent()
	if err != nil {
		return ""
	}
	var result strings.Builder
	for _, node := range nodes {
		switch node.Type {
		case ast.InlineText, ast.InlineCode:
			result.WriteString(node.Text)
		default:
			result.WriteString(node.DisplayText)
		}
	}
	return result.String()
}

func isSeeAlsoTitle(title string) bool {
	switch title {
	case "see also", "related topics", "related articles", "参见", "相关条目", "相关主题":
		return true
	default:
		return false
	}
}

func isArticleEndMatterTitle(title string) bool {
	if isSeeAlsoTitle(title) {
		return true
	}
	switch title {
	case "references", "reference", "bibliography", "further reading", "external links",
		"参考文献", "参考资料", "引用文献", "拓展阅读", "扩展阅读", "外部链接":
		return true
	default:
		return false
	}
}

func operationEvidenceFromCandidates(
	items []CandidateEvidence,
) []governance.OperationEvidence {
	result := make([]governance.OperationEvidence, 0, min(len(items), 8))
	seen := make(map[uuid.UUID]bool)
	for _, item := range items {
		if item.ChunkID == uuid.Nil || seen[item.ChunkID] {
			continue
		}
		seen[item.ChunkID] = true
		chunkID := item.ChunkID
		result = append(result, governance.OperationEvidence{
			SourceChunkID: &chunkID,
		})
		if len(result) == 8 {
			break
		}
	}
	return result
}

func insertBlockOperation(
	wikiID, pageID uuid.UUID,
	base governance.OperationBase,
	index int,
	block *ast.Block,
	evidence []governance.OperationEvidence,
	reason string,
) (governance.OperationV1, error) {
	return insertBlockOperationWithParent(
		wikiID, pageID, base, nil, index, block, evidence, reason,
	)
}

func insertBlockOperationWithParent(
	wikiID, pageID uuid.UUID,
	base governance.OperationBase,
	parentBlockID *string,
	index int,
	block *ast.Block,
	evidence []governance.OperationEvidence,
	reason string,
) (governance.OperationV1, error) {
	payload, err := json.Marshal(map[string]any{
		"parent_block_id": parentBlockID, "index": index, "block": block,
	})
	if err != nil {
		return governance.OperationV1{}, err
	}
	return governance.OperationV1{
		SchemaVersion: 1, OperationType: governance.OpInsertBlock,
		Base:     base,
		Target:   governance.OperationTarget{WikiID: &wikiID, PageID: &pageID},
		Evidence: evidence,
		Risk: governance.OperationRisk{
			Level: governance.RiskLow, Reasons: []string{reason},
		},
		Payload: payload,
	}, nil
}

func buildSetPageEntityBindingOperation(
	params ComposeParams,
	pageID uuid.UUID,
	primary *primaryEntityTarget,
	base governance.OperationBase,
) (governance.OperationV1, error) {
	if primary == nil || primary.EntityID == uuid.Nil {
		return governance.OperationV1{}, ErrInvalidJob
	}
	payload, err := json.Marshal(map[string]any{
		"role":     knowledge.BindingRolePrimary,
		"language": importPlanLanguage(params.Plan.Profile.Language),
	})
	if err != nil {
		return governance.OperationV1{}, err
	}
	return governance.OperationV1{
		SchemaVersion: 1,
		OperationType: governance.OpSetPageEntityBinding,
		Base:          base,
		Target: governance.OperationTarget{
			WikiID: &params.WikiID, PageID: &pageID,
			EntityID: &primary.EntityID,
		},
		Evidence: operationEvidenceFromCandidates(primary.Evidence),
		Risk: governance.OperationRisk{
			Level: governance.RiskMedium,
			Reasons: []string{
				"AI import binds the article to its reviewed primary Entity",
			},
		},
		Payload: payload,
	}, nil
}

func (c *ProposalComposer) compilePlannedBlock(planned PlannedBlock, citationIDs []uuid.UUID) (*ast.Block, error) {
	return c.compilePlannedBlockWithReferences(planned, citationIDs, nil)
}

func (c *ProposalComposer) compilePlannedBlockWithReferences(planned PlannedBlock, citationIDs []uuid.UUID,
	entityReferences []entityReferenceTarget) (*ast.Block, error) {
	blockID, err := c.ids.NewString()
	if err != nil {
		return nil, err
	}
	switch planned.Type {
	case string(ast.BlockHeading):
		// Heading evidence remains on the enclosing Proposal operation, but only
		// factual prose renders inline citation markers. This avoids citation
		// noise such as a superscript after every section title.
		block, err := c.textBlock(blockID, ast.BlockHeading, strings.TrimSpace(planned.Text), nil, nil)
		if err != nil {
			return nil, err
		}
		block.Level = planned.Level
		block.IsToggleable = planned.IsToggleable
		if planned.IsToggleable {
			block.Children, err = c.compilePlannedChildren(
				planned.Children, citationIDs, entityReferences,
			)
			if err != nil {
				return nil, err
			}
		}
		if err := validateCompiledBlock(block); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockParagraph):
		return c.textBlock(blockID, ast.BlockParagraph, strings.TrimSpace(planned.Text), citationIDs, entityReferences)

	case string(ast.BlockCode):
		content, err := json.Marshal(planned.Text)
		if err != nil {
			return nil, err
		}
		block := &ast.Block{
			ID: blockID, Type: ast.BlockCode, Content: content,
			Language: planned.Language, Caption: planned.Caption,
		}
		if err := validateCompiledBlock(block); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockPreformatted):
		content, err := json.Marshal(planned.Text)
		if err != nil {
			return nil, err
		}
		block := &ast.Block{
			ID: blockID, Type: ast.BlockPreformatted, Content: content,
			Kind: planned.Kind, Caption: planned.Caption,
		}
		if err := validateCompiledBlock(block); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockEquation):
		block := &ast.Block{
			ID: blockID, Type: ast.BlockEquation,
			Expression: planned.Expression, Caption: planned.Caption,
		}
		if err := validateCompiledBlock(block); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockTodo):
		block, err := c.textBlock(
			blockID, ast.BlockTodo, strings.TrimSpace(planned.Text),
			citationIDs, entityReferences,
		)
		if err != nil {
			return nil, err
		}
		checked := false
		if planned.Checked != nil {
			checked = *planned.Checked
		}
		block.Checked = &checked
		block.Children, err = c.compilePlannedChildren(
			planned.Children, citationIDs, entityReferences,
		)
		if err != nil {
			return nil, err
		}
		if err := validateCompiledBlock(block); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockToggle):
		block, err := c.textBlock(
			blockID, ast.BlockToggle, strings.TrimSpace(planned.Text),
			citationIDs, entityReferences,
		)
		if err != nil {
			return nil, err
		}
		block.Children, err = c.compilePlannedChildren(
			planned.Children, citationIDs, entityReferences,
		)
		if err != nil {
			return nil, err
		}
		if err := validateCompiledBlock(block); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockQuote):
		paragraphID, err := c.ids.NewString()
		if err != nil {
			return nil, err
		}
		paragraph, err := c.textBlock(paragraphID, ast.BlockParagraph, strings.TrimSpace(planned.Text), citationIDs, entityReferences)
		if err != nil {
			return nil, err
		}
		block := &ast.Block{ID: blockID, Type: ast.BlockQuote, Children: []*ast.Block{paragraph}}
		if err := validateCompiledBlock(block); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockBulletList), string(ast.BlockOrderedList):
		listType := ast.BlockBulletList
		if planned.Type == string(ast.BlockOrderedList) {
			listType = ast.BlockOrderedList
		}
		list := &ast.Block{ID: blockID, Type: listType, Children: []*ast.Block{}}
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
			paragraph, err := c.textBlock(paragraphID, ast.BlockParagraph, strings.TrimSpace(rawItem), itemCitations, entityReferences)
			if err != nil {
				return nil, err
			}
			list.Children = append(list.Children, &ast.Block{
				ID: itemID, Type: ast.BlockListItem, Children: []*ast.Block{paragraph},
			})
		}
		if err := validateCompiledBlock(list); err != nil {
			return nil, err
		}
		return list, nil

	case string(ast.BlockCallout):
		paragraphID, err := c.ids.NewString()
		if err != nil {
			return nil, err
		}
		paragraph, err := c.textBlock(
			paragraphID, ast.BlockParagraph, strings.TrimSpace(planned.Text),
			citationIDs, entityReferences,
		)
		if err != nil {
			return nil, err
		}
		children, err := c.compilePlannedChildren(
			planned.Children, citationIDs, entityReferences,
		)
		if err != nil {
			return nil, err
		}
		block := &ast.Block{
			ID: blockID, Type: ast.BlockCallout, Kind: planned.Kind,
			Icon: planned.Icon, Color: ast.CalloutColor(planned.Color),
			Children: append([]*ast.Block{paragraph}, children...),
		}
		if err := validateCompiledBlock(block); err != nil {
			return nil, err
		}
		return block, nil

	case string(ast.BlockTable):
		table := &ast.Block{
			ID: blockID, Type: ast.BlockTable, Caption: planned.Caption,
			HasColumnHeader: planned.HasColumnHeader,
			HasRowHeader:    planned.HasRowHeader,
			Children:        []*ast.Block{},
		}
		for rowIndex, plannedRow := range planned.Rows {
			rowID, err := c.ids.NewString()
			if err != nil {
				return nil, err
			}
			row := &ast.Block{ID: rowID, Type: ast.BlockTableRow, Children: []*ast.Block{}}
			for cellIndex, cellText := range plannedRow {
				cellID, err := c.ids.NewString()
				if err != nil {
					return nil, err
				}
				paragraphID, err := c.ids.NewString()
				if err != nil {
					return nil, err
				}
				cellCitations := []uuid.UUID{}
				if rowIndex == len(planned.Rows)-1 && cellIndex == len(plannedRow)-1 {
					cellCitations = citationIDs
				}
				paragraph, err := c.textBlock(
					paragraphID, ast.BlockParagraph, cellText,
					cellCitations, entityReferences,
				)
				if err != nil {
					return nil, err
				}
				row.Children = append(row.Children, &ast.Block{
					ID: cellID, Type: ast.BlockTableCell,
					Children: []*ast.Block{paragraph},
				})
			}
			table.Children = append(table.Children, row)
		}
		if err := validateCompiledBlock(table); err != nil {
			return nil, err
		}
		return table, nil
	default:
		return nil, ErrInvalidJob
	}
}

func (c *ProposalComposer) compilePlannedChildren(
	children []PlannedBlock,
	citationIDs []uuid.UUID,
	entityReferences []entityReferenceTarget,
) ([]*ast.Block, error) {
	result := make([]*ast.Block, 0, len(children))
	for _, child := range children {
		block, err := c.compilePlannedBlockWithReferences(
			child, citationIDs, entityReferences,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, block)
	}
	return result, nil
}

func validateCompiledBlock(block *ast.Block) error {
	return ast.ValidateBlock(block)
}

func plannedBlockEvidence(block PlannedBlock) []CandidateEvidence {
	result := append([]CandidateEvidence(nil), block.Evidence...)
	for _, child := range block.Children {
		result = mergeCandidateEvidence(result, plannedBlockEvidence(child))
		if len(result) >= 8 {
			return result[:8]
		}
	}
	return result
}

func (c *ProposalComposer) textBlock(
	blockID string,
	blockType ast.BlockType,
	text string,
	citationIDs []uuid.UUID,
	entityReferences []entityReferenceTarget,
) (*ast.Block, error) {
	nodes, err := inlineNodesWithEntityReferences(text, entityReferences)
	if err != nil {
		return nil, err
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
