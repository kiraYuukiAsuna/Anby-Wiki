package governance_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func TestPreviewPageProposal_BaseCurrentProposedAndNoWrites(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actor := tdb.MakeActor(t, "human", "editor")
	ids := id.NewGenerator()
	txm := db.NewTxManager(tdb.Pool)
	pageRepo := page.NewRepository(tdb.Pool)
	pages := page.NewService(pageRepo, txm, ids)
	govRepo := governance.NewRepository(tdb.Pool)
	gov := governance.NewService(govRepo, txm, ids)

	p, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: testkit.DefaultWikiID, NamespaceID: testkit.MainNamespaceID,
		Title: "Preview", ActorID: actor,
	})
	if err != nil {
		t.Fatal(err)
	}
	blockID := "00000000-0000-7000-8000-000000000701"
	baseDoc := documentWithParagraphs(t, paragraph(t, blockID, "Base"))
	baseRaw, _ := json.Marshal(baseDoc)
	baseRev, err := pages.Publish(ctx, page.PublishParams{PageID: p.ID, ActorID: actor, AST: baseRaw})
	if err != nil {
		t.Fatal(err)
	}
	blockHash, _ := governance.BlockHash(baseDoc.Children[0])
	proposal, err := gov.CreateProposal(ctx, governance.CreateProposalParams{
		TargetType: governance.TargetPage, TargetID: &p.ID, BaseRevisionID: &baseRev.ID,
		CreatedBy: actor, IdempotencyKey: "preview-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	op := governance.OperationV1{
		SchemaVersion: 1, OperationType: governance.OpReplaceBlock,
		Base:         governance.OperationBase{RevisionID: &baseRev.ID},
		Target:       governance.OperationTarget{PageID: &p.ID, BlockID: &blockID},
		ExpectedHash: &blockHash,
		Evidence:     []governance.OperationEvidence{{Note: "review source"}},
		Risk:         governance.OperationRisk{Level: governance.RiskLow, Reasons: []string{}},
		Payload:      mustJSON(t, map[string]any{"block": paragraph(t, blockID, "Proposed")}),
	}
	opRaw, _ := json.Marshal(op)
	if _, err := gov.AddOperationV1(ctx, proposal.ID, opRaw); err != nil {
		t.Fatal(err)
	}

	currentDoc := documentWithParagraphs(t, paragraph(t, blockID, "Current"))
	currentRaw, _ := json.Marshal(currentDoc)
	if _, err := pages.Publish(ctx, page.PublishParams{
		PageID: p.ID, ActorID: actor, ExpectedRevisionID: &baseRev.ID, AST: currentRaw,
	}); err != nil {
		t.Fatal(err)
	}
	before := authorityCounts(t, tdb)

	preview, err := governance.NewPreviewService(govRepo, pages, governance.NewPagePatchEngine()).
		PreviewPageProposal(ctx, proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Stale || preview.Base.RevisionID == nil || preview.Current.RevisionID == nil ||
		*preview.Base.RevisionID == *preview.Current.RevisionID {
		t.Fatalf("Base/Current stale 语义错误: %+v", preview)
	}
	proposedDoc, err := ast.Parse(preview.Proposed.AST)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := proposedDoc.Children[0].InlineContent()
	if nodes[0].Text != "Proposed" || preview.Impact.ChangedBlocks != 1 || len(preview.Evidence) != 1 {
		t.Fatalf("preview=%+v proposed nodes=%+v", preview, nodes)
	}
	after := authorityCounts(t, tdb)
	if before != after {
		t.Fatalf("Preview 写入权威数据: before=%v after=%v", before, after)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func authorityCounts(t *testing.T, tdb *testkit.DB) [5]int {
	t.Helper()
	var out [5]int
	for i, table := range []string{"revision", "content_snapshot", "claim", "audit_event", "outbox_event"} {
		if err := tdb.Pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&out[i]); err != nil {
			t.Fatal(err)
		}
	}
	return out
}
