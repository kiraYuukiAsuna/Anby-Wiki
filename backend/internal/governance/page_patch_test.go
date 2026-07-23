package governance_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/governance"
)

func TestPagePatchEngine_BlockOperationsArePureAndOrdered(t *testing.T) {
	pageID := uuid.MustParse("00000000-0000-7000-8000-000000000601")
	aID := "00000000-0000-7000-8000-000000000602"
	bID := "00000000-0000-7000-8000-000000000603"
	cID := "00000000-0000-7000-8000-000000000604"
	doc := documentWithParagraphs(t, paragraph(t, aID, "A"), paragraph(t, bID, "B"))
	originalHash, _ := ast.ContentHash(doc)
	aHash, _ := governance.BlockHash(doc.Children[0])

	ops := []governance.OperationV1{
		pageOp(pageID, governance.OpReplaceBlock, &aID, &aHash,
			map[string]any{"block": paragraph(t, aID, "A2")}),
		pageOp(pageID, governance.OpInsertBlock, nil, nil,
			map[string]any{"parent_block_id": nil, "index": 2, "block": paragraph(t, cID, "C")}),
		pageOp(pageID, governance.OpMoveBlock, &cID, nil,
			map[string]any{"parent_block_id": nil, "index": 0}),
		pageOp(pageID, governance.OpDeleteBlock, &bID, nil, map[string]any{}),
	}
	out, err := governance.NewPagePatchEngine().Apply(doc, pageID, ops)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Children) != 2 || out.Children[0].ID != cID || out.Children[1].ID != aID {
		t.Fatalf("unexpected order: %+v", out.Children)
	}
	gotText, _ := out.Children[1].InlineContent()
	if gotText[0].Text != "A2" {
		t.Fatalf("replace text=%q", gotText[0].Text)
	}
	stillOriginal, _ := ast.ContentHash(doc)
	if stillOriginal != originalHash {
		t.Fatal("Patch Engine 修改了输入文档")
	}
}

func TestPagePatchEngine_ReferencesAndConflictRejection(t *testing.T) {
	pageID := uuid.MustParse("00000000-0000-7000-8000-000000000611")
	blockID := "00000000-0000-7000-8000-000000000612"
	entityID := uuid.MustParse("00000000-0000-7000-8000-000000000613")
	claimID := uuid.MustParse("00000000-0000-7000-8000-000000000614")
	doc := documentWithParagraphs(t, paragraph(t, blockID, "text"))

	insertEntity := pageOp(pageID, governance.OpInsertEntityReference, &blockID, nil,
		map[string]any{"index": 1, "display_text": "实体"})
	insertEntity.Target.EntityID = &entityID
	insertClaim := pageOp(pageID, governance.OpInsertClaimReference, &blockID, nil,
		map[string]any{"index": 2, "display_text": "事实"})
	insertClaim.Target.ClaimID = &claimID
	out, err := governance.NewPagePatchEngine().Apply(doc, pageID, []governance.OperationV1{insertEntity, insertClaim})
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := out.Children[0].InlineContent()
	if len(nodes) != 3 || nodes[1].EntityID != entityID.String() || nodes[2].ClaimID != claimID.String() {
		t.Fatalf("unexpected refs: %+v", nodes)
	}

	wrong := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	stale := pageOp(pageID, governance.OpDeleteBlock, &blockID, &wrong, map[string]any{})
	if _, err := governance.NewPagePatchEngine().Apply(doc, pageID, []governance.OperationV1{stale}); !errors.Is(err, governance.ErrPatchTargetModified) {
		t.Fatalf("stale err=%v", err)
	}
	missingID := "00000000-0000-7000-8000-000000000699"
	missing := pageOp(pageID, governance.OpDeleteBlock, &missingID, nil, map[string]any{})
	if _, err := governance.NewPagePatchEngine().Apply(doc, pageID, []governance.OperationV1{missing}); err == nil {
		t.Fatal("missing target 应拒绝")
	}
}

func pageOp(pageID uuid.UUID, typ string, blockID *string, expected *string, payload any) governance.OperationV1 {
	raw, _ := json.Marshal(payload)
	return governance.OperationV1{
		SchemaVersion: 1, OperationType: typ,
		Target:       governance.OperationTarget{PageID: &pageID, BlockID: blockID},
		ExpectedHash: expected, Payload: raw,
	}
}

func paragraph(t *testing.T, id, text string) *ast.Block {
	t.Helper()
	content, err := json.Marshal([]*ast.InlineNode{{Type: ast.InlineText, Text: text}})
	if err != nil {
		t.Fatal(err)
	}
	return &ast.Block{ID: id, Type: ast.BlockParagraph, Content: content}
}

func documentWithParagraphs(t *testing.T, blocks ...*ast.Block) *ast.Document {
	t.Helper()
	doc := &ast.Document{Type: "document", SchemaVersion: 1, Children: blocks}
	if err := ast.Validate(doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
