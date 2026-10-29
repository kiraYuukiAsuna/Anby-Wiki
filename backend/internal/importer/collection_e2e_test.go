package importer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/ast"
	"github.com/anby/wiki/backend/internal/collection"
	"github.com/anby/wiki/backend/internal/governance"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

// Each run creates and removes its own schema; the supplied database must
// permit schema creation. No production credentials or shared fixtures are used.
func TestImportCollectionApplyAndRollbackE2E(t *testing.T) {
	databaseURL := os.Getenv("IMPORT_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set IMPORT_E2E_DATABASE_URL to test atomic import membership writes")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schemaName := "import_collections_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaSQL); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schemaSQL+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName + ",public"
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schemaSQL+",public")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema, err := os.ReadFile("../../migrations/000001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	ids, txm := id.NewGenerator(), db.NewTxManager(pool)
	pagesRepo := page.NewRepository(pool)
	pages := page.NewService(pagesRepo, txm, ids)
	collections := collection.NewService(collection.NewRepository(pool), pagesRepo, txm, ids)
	knowledgeService := knowledge.NewService(knowledge.NewRepository(pool), pagesRepo, txm, ids)
	knowledgePatch := governance.NewKnowledgePatchEngine(knowledgeService)
	repo := governance.NewRepository(pool)
	proposals := governance.NewService(repo, txm, ids)
	reviews := governance.NewReviewService(repo, txm, ids, governance.NewRiskEvaluator(knowledgeService))
	apply := governance.NewApplyService(repo, pages, governance.NewPagePatchEngine(), knowledgePatch,
		governance.NewConflictService(repo, pages, knowledgeService, txm, ids), txm, ids).WithCollections(collections)
	rollback := governance.NewRollbackService(repo, pages, knowledgePatch, txm, ids).WithCollections(collections)
	seedWiki := func() (uuid.UUID, uuid.UUID, uuid.UUID) {
		t.Helper()
		wikiID, actorID, namespaceID := uuid.New(), uuid.New(), uuid.New()
		err := txm.InTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO wiki_site
				(id,site_key,name,default_language,owner_actor_id) VALUES ($1,$2,'Import test','en',$3)`,
				wikiID, wikiID.String(), actorID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO actor (id,wiki_id,actor_type,display_name)
				VALUES ($1,$2,'human','Import reviewer')`, actorID, wikiID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO namespace
				(id,wiki_id,namespace_key,canonical_name,display_name,is_content)
				VALUES ($1,$2,'main','Main','Main',true)`, namespaceID, wikiID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return wikiID, actorID, namespaceID
	}
	wikiID, actorID, namespaceID := seedWiki()
	newCollection := func(wikiID, actorID uuid.UUID, kind string) uuid.UUID {
		t.Helper()
		params := collection.CreateParams{WikiID: wikiID, ActorID: actorID, CollectionType: kind, Title: uuid.NewString()}
		if kind == collection.TypeDynamic {
			params.Rule = json.RawMessage(`{"version":1,"member_type":"page","text":"JWT"}`)
		} else if kind == collection.TypeRule {
			params.Rule = json.RawMessage(`{"version":1,"kind":"entity_type","entity_type":"concept"}`)
		}
		value, err := collections.Create(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		return value.ID
	}
	firstCollection := newCollection(wikiID, actorID, collection.TypeManual)
	oldCollection := newCollection(wikiID, actorID, collection.TypeManual)
	marshal := func(value any) json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	document := json.RawMessage(`{"schema_version":1,"type":"document","children":[]}`)
	existingPage, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: wikiID, NamespaceID: namespaceID, Title: "Existing JWT", Language: "en",
		ContentModel: "block-v1", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	oldRevision, err := pages.Publish(ctx, page.PublishParams{PageID: existingPage.ID, ActorID: actorID, AST: document})
	if err != nil {
		t.Fatal(err)
	}
	_, baseline, err := pages.CurrentContent(ctx, existingPage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := collections.ReplaceManualMembers(ctx, wikiID, oldCollection, actorID, []collection.MemberInput{{
		MemberType: collection.MemberPage, PageID: &existingPage.ID, SortKey: "Original sort", SourceRevisionID: oldRevision.ID,
	}}); err != nil {
		t.Fatal(err)
	}
	createOperation := func(pageID uuid.UUID) governance.OperationV1 {
		zero := 0
		return governance.OperationV1{
			SchemaVersion: 1, OperationType: governance.OpCreatePage, Base: governance.OperationBase{StateVersion: &zero},
			Target:   governance.OperationTarget{WikiID: &wikiID, NamespaceID: &namespaceID, PageID: &pageID},
			Evidence: []governance.OperationEvidence{{Note: "isolated import fixture"}},
			Risk:     governance.OperationRisk{Level: governance.RiskMedium, Reasons: []string{"new page"}},
			Payload:  marshal(map[string]any{"title": pageID.String(), "language": "en", "content_model": "block-v1", "initial_ast": document}),
		}
	}
	membership := func(pageID, collectionID uuid.UUID) governance.OperationV1 {
		return collectionMembershipOperations(wikiID, pageID, PageRoute{
			Title: "JWT", Collections: []PlannedCollectionMembership{{
				CollectionID: collectionID, Title: "Authentication", Reason: "Token format", Confidence: 0.9,
			}},
		})[0]
	}
	approve := func(targetType string, targetID uuid.UUID, operations []governance.OperationV1) uuid.UUID {
		t.Helper()
		zero := 0
		proposal, err := proposals.CreateProposal(ctx, governance.CreateProposalParams{
			WikiID: wikiID, TargetType: targetType, TargetID: &targetID,
			BaseStateVersion: &zero, CreatedBy: actorID, IdempotencyKey: uuid.NewString(),
		})
		if err != nil {
			t.Fatal(err)
		}
		rawOperations := make([][]byte, len(operations))
		for index := range operations {
			rawOperations[index] = marshal(operations[index])
		}
		if _, err := proposals.AddOperationsV1(ctx, proposal.ID, rawOperations); err != nil {
			t.Fatal(err)
		}
		submitted, err := reviews.Submit(ctx, proposal.ID)
		if err != nil {
			t.Fatal(err)
		}
		if submitted.ReviewTask == nil || submitted.Decision.AutoApprove {
			t.Fatal("topic import bypassed human review")
		}
		if _, err := reviews.Decide(ctx, wikiID, submitted.ReviewTask.ID, actorID, true, "Review import topics"); err != nil {
			t.Fatal(err)
		}
		return proposal.ID
	}
	newPageID := uuid.New()
	operations := []governance.OperationV1{createOperation(newPageID), membership(newPageID, firstCollection), {
		SchemaVersion: 1, OperationType: governance.OpInsertBlock,
		Base:     governance.OperationBase{RevisionID: &oldRevision.ID},
		Target:   governance.OperationTarget{PageID: &existingPage.ID},
		Evidence: []governance.OperationEvidence{}, Risk: governance.OperationRisk{Level: governance.RiskMedium, Reasons: []string{}},
		Payload: marshal(map[string]any{"parent_block_id": nil, "index": 0,
			"block": map[string]any{"id": uuid.NewString(), "type": ast.BlockParagraph,
				"content": []map[string]any{{"type": "text", "text": "JWT uses authentication tokens."}}}}),
	}, membership(existingPage.ID, firstCollection), membership(existingPage.ID, oldCollection)}
	proposalID := approve(governance.TargetWiki, wikiID, operations)
	applied, err := apply.Apply(ctx, proposalID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	newPage, err := pages.GetPage(ctx, newPageID)
	if err != nil {
		t.Fatal(err)
	}
	updatedPage, err := pages.GetPage(ctx, existingPage.ID)
	if err != nil {
		t.Fatal(err)
	}
	members, err := collections.ListMembers(ctx, wikiID, firstCollection, "", 100)
	if err != nil || len(members.Items) != 2 {
		t.Fatalf("batch members=%#v error=%v", members, err)
	}
	for _, item := range members.Items {
		want := newPage.CurrentRevisionID
		if *item.PageID == existingPage.ID {
			want = updatedPage.CurrentRevisionID
		}
		if item.SourceRevisionID == nil || *item.SourceRevisionID != *want || *want == oldRevision.ID {
			t.Fatalf("membership points to pre-publication revision: %#v", item)
		}
	}
	oldMembers, err := collections.ListMembers(ctx, wikiID, oldCollection, "", 100)
	if err != nil || len(oldMembers.Items) != 1 || *oldMembers.Items[0].SourceRevisionID != oldRevision.ID ||
		oldMembers.Items[0].SortKey != "Original sort" {
		t.Fatalf("duplicate addition changed old membership: %#v error=%v", oldMembers, err)
	}
	if replay, err := apply.Apply(ctx, proposalID, actorID); err != nil || !replay.Idempotent {
		t.Fatalf("apply replay=%#v error=%v", replay, err)
	}
	if _, err := rollback.Rollback(ctx, applied.ChangeBatchID, actorID); err != nil {
		t.Fatal(err)
	}
	members, err = collections.ListMembers(ctx, wikiID, firstCollection, "", 100)
	if err != nil || len(members.Items) != 0 {
		t.Fatalf("rollback left new memberships: %#v error=%v", members, err)
	}
	oldMembers, err = collections.ListMembers(ctx, wikiID, oldCollection, "", 100)
	if err != nil || len(oldMembers.Items) != 1 || *oldMembers.Items[0].SourceRevisionID != oldRevision.ID {
		t.Fatalf("rollback removed old membership: %#v error=%v", oldMembers, err)
	}
	newPage, err = pages.GetPage(ctx, newPageID)
	if err != nil {
		t.Fatal(err)
	}
	if newPage.DeletedAt == nil {
		t.Fatal("rollback did not compensate new page")
	}
	compensation, snapshot, err := pages.CurrentContent(ctx, existingPage.ID)
	if err != nil || compensation.ID == oldRevision.ID || snapshot.ContentHash != baseline.ContentHash {
		t.Fatalf("rollback did not restore content through a new revision: %#v error=%v", compensation, err)
	}

	t.Run("late membership failure rolls back published page and earlier member", func(t *testing.T) {
		pageID := uuid.New()
		bad := membership(pageID, oldCollection)
		bad.Payload = marshal(map[string]any{"sort_key": "Bad", "source_revision_id": uuid.New()})
		proposalID := approve(governance.TargetWiki, wikiID, []governance.OperationV1{
			createOperation(pageID), membership(pageID, firstCollection), bad,
		})
		if _, err := apply.Apply(ctx, proposalID, actorID); !errors.Is(err, collection.ErrInvalidMember) {
			t.Fatalf("invalid provenance error=%v", err)
		}
		if _, err := pages.GetPage(ctx, pageID); !errors.Is(err, page.ErrPageNotFound) {
			t.Fatalf("failed batch left a page: %v", err)
		}
		members, err := collections.ListMembers(ctx, wikiID, firstCollection, "", 100)
		if err != nil || len(members.Items) != 0 {
			t.Fatalf("failed batch left membership: %#v error=%v", members, err)
		}
	})
	foreignWiki, foreignActor, foreignNamespace := seedWiki()
	foreignCollection := newCollection(foreignWiki, foreignActor, collection.TypeManual)
	dynamicCollection := newCollection(wikiID, actorID, collection.TypeDynamic)
	ruleCollection := newCollection(wikiID, actorID, collection.TypeRule)
	foreignPage, err := pages.CreatePage(ctx, page.CreatePageParams{
		WikiID: foreignWiki, NamespaceID: foreignNamespace, Title: "Foreign JWT",
		Language: "en", ContentModel: "block-v1", ActorID: foreignActor,
	})
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := NewPagePlanner(nil, pages, nil, ids).WithCollections(collections).recallCollections(ctx, wikiID)
	if err != nil || len(catalogue) != 2 {
		t.Fatalf("planner includes foreign or query-managed collection: %#v error=%v", catalogue, err)
	}
	for _, test := range []struct {
		name         string
		targetType   string
		targetID     uuid.UUID
		collectionID uuid.UUID
		pageID       uuid.UUID
	}{
		{"cross wiki collection", governance.TargetWiki, wikiID, foreignCollection, existingPage.ID},
		{"cross wiki page", governance.TargetWiki, wikiID, firstCollection, foreignPage.ID},
		{"dynamic collection", governance.TargetWiki, wikiID, dynamicCollection, existingPage.ID},
		{"rule collection", governance.TargetWiki, wikiID, ruleCollection, existingPage.ID},
		{"collection proposal requires explicit revision", governance.TargetCollection, firstCollection, firstCollection, existingPage.ID},
	} {
		t.Run(test.name, func(t *testing.T) {
			proposalID := approve(test.targetType, test.targetID, []governance.OperationV1{membership(test.pageID, test.collectionID)})
			if _, err := apply.Apply(ctx, proposalID, actorID); err == nil {
				t.Fatal("invalid topic membership was applied")
			}
		})
	}
}
