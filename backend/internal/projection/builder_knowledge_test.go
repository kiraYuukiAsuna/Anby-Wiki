package projection_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/projection"
	"github.com/anby/wiki/backend/testkit"
)

type knowledgeTargets struct {
	entityID   uuid.UUID
	claimID    uuid.UUID
	citationID uuid.UUID
}

func makeKnowledgeTargets(t *testing.T, d *testkit.DB) knowledgeTargets {
	t.Helper()
	ctx := context.Background()
	entityID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO entity (id, wiki_id, entity_type_id, canonical_key, status, created_by)
		VALUES ($1, $2, $3, $4, 'active', $5)`,
		entityID, testkit.DefaultWikiID, testkit.EntityTypeCharacterID,
		"projection-entity-"+entityID.String(), testkit.SystemActorID); err != nil {
		t.Fatalf("插入 entity 失败: %v", err)
	}
	claimID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO claim (
			id, subject_entity_id, property_id, value_type, value_json,
			status, verification_status, origin_type, created_by)
		VALUES ($1, $2, $3, 'date', '"2024-07-04"'::jsonb,
			'published', 'human_verified', 'human', $4)`,
		claimID, entityID, testkit.PropertyReleaseDateID, testkit.SystemActorID); err != nil {
		t.Fatalf("插入 claim 失败: %v", err)
	}
	return knowledgeTargets{
		entityID: entityID, claimID: claimID,
		citationID: d.MakeCitation(t, testkit.SystemActorID),
	}
}

func appendASTRevision(t *testing.T, d *testkit.DB, pageID uuid.UUID, parent *uuid.UUID, raw string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	snapshotID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO content_snapshot (id, schema_version, ast_json, content_hash, size_bytes)
		VALUES ($1, 1, $2::jsonb, $3, $4)`,
		snapshotID, raw, snapshotID.String(), len(raw)); err != nil {
		t.Fatalf("插入 AST snapshot 失败: %v", err)
	}
	revisionID := d.NewID(t)
	if _, err := d.Pool.Exec(ctx, `
		INSERT INTO revision (id, page_id, parent_revision_id, content_snapshot_id, actor_id)
		VALUES ($1, $2, $3, $4, $5)`,
		revisionID, pageID, parent, snapshotID, testkit.SystemActorID); err != nil {
		t.Fatalf("插入 AST revision 失败: %v", err)
	}
	if _, err := d.Pool.Exec(ctx,
		`UPDATE page SET current_revision_id = $2 WHERE id = $1`, pageID, revisionID); err != nil {
		t.Fatalf("更新 Current 失败: %v", err)
	}
	return revisionID
}

func knowledgeRegistry(d *testkit.DB) *projection.Registry {
	reg := projection.NewRegistry()
	reg.Register(projection.NewEntityMentionsBuilder(d.Pool))
	reg.Register(projection.NewClaimUsageBuilder(d.Pool))
	reg.Register(projection.NewCitationUsageBuilder(d.Pool))
	return reg
}

func TestKnowledgeUsageBuildersAndReverseQueries(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	targets := makeKnowledgeTargets(t, d)
	pageID := d.MakePage(t, testkit.MainNamespaceID, "knowledge-usage", "Knowledge Usage", testkit.SystemActorID)
	blockID := d.NewID(t)
	raw := fmt.Sprintf(`{"type":"document","schema_version":1,"children":[
		{"id":%q,"type":"paragraph","content":[
			{"type":"entity_reference","entity_id":%q,"display_text":"安比"},
			{"type":"text","text":" / "},
			{"type":"claim_reference","claim_id":%q,"display_text":"生日"},
			{"type":"citation_reference","citation_id":%q,"display_text":"设定集"}
		]}
	]}`, blockID, targets.entityID, targets.claimID, targets.citationID)
	revisionID := appendASTRevision(t, d, pageID, nil, raw)

	rebuilder := projection.NewRebuilder(d.Pool, knowledgeRegistry(d), nil)
	if rebuilt, err := rebuilder.RebuildPage(context.Background(), pageID); err != nil || !rebuilt {
		t.Fatalf("RebuildPage = (%v, %v)", rebuilt, err)
	}
	// 重复重建必须幂等。
	if rebuilt, err := rebuilder.RebuildPage(context.Background(), pageID); err != nil || !rebuilt {
		t.Fatalf("重复 RebuildPage = (%v, %v)", rebuilt, err)
	}

	var mentionRevision, mentionBlock uuid.UUID
	var mentionNode, mentionText string
	if err := d.Pool.QueryRow(context.Background(), `
		SELECT revision_id, block_id, node_id, mention_text
		FROM entity_mention_projection WHERE page_id = $1 AND entity_id = $2`,
		pageID, targets.entityID).Scan(&mentionRevision, &mentionBlock, &mentionNode, &mentionText); err != nil {
		t.Fatal(err)
	}
	if mentionRevision != revisionID || mentionBlock != blockID || mentionNode != "0" || mentionText != "安比" {
		t.Fatalf("entity mention 异常: rev=%s block=%s node=%s text=%s", mentionRevision, mentionBlock, mentionNode, mentionText)
	}
	usageRows := []struct {
		table, column string
		target        uuid.UUID
		wantNode      string
	}{
		{table: "claim_usage", column: "claim_id", target: targets.claimID, wantNode: "2"},
		{table: "citation_usage", column: "citation_id", target: targets.citationID, wantNode: "3"},
	}
	for _, row := range usageRows {
		var gotRevision uuid.UUID
		var gotNode string
		query := fmt.Sprintf("SELECT revision_id, node_id FROM %s WHERE page_id = $1 AND %s = $2", row.table, row.column)
		if err := d.Pool.QueryRow(context.Background(), query, pageID, row.target).Scan(&gotRevision, &gotNode); err != nil {
			t.Fatal(err)
		}
		if gotRevision != revisionID || gotNode != row.wantNode {
			t.Fatalf("%s = rev:%s node:%s，期望 %s/%s", row.table, gotRevision, gotNode, revisionID, row.wantNode)
		}
	}

	queries := projection.NewQueries(d.Pool)
	entityPage, err := queries.EntityMentions(context.Background(), targets.entityID, "", 20)
	if err != nil || len(entityPage.Items) != 1 || entityPage.Items[0].RevisionID != revisionID || entityPage.Items[0].MentionText != "安比" {
		t.Fatalf("EntityMentions = (%+v, %v)", entityPage, err)
	}
	claimPage, err := queries.ClaimUsages(context.Background(), targets.claimID, "", 20)
	if err != nil || len(claimPage.Items) != 1 || claimPage.Items[0].RevisionID != revisionID {
		t.Fatalf("ClaimUsages = (%+v, %v)", claimPage, err)
	}
	citationPage, err := queries.CitationUsages(context.Background(), targets.citationID, "", 20)
	if err != nil || len(citationPage.Items) != 1 || citationPage.Items[0].RevisionID != revisionID {
		t.Fatalf("CitationUsages = (%+v, %v)", citationPage, err)
	}

	// 反向查询只读投影：删除 mention 行后，即使权威 AST 仍含引用也返回空。
	if _, err := d.Pool.Exec(context.Background(),
		`DELETE FROM entity_mention_projection WHERE page_id = $1`, pageID); err != nil {
		t.Fatal(err)
	}
	entityPage, err = queries.EntityMentions(context.Background(), targets.entityID, "", 20)
	if err != nil || len(entityPage.Items) != 0 {
		t.Fatalf("删除投影后 EntityMentions = (%+v, %v)，期望空（不得扫描 AST）", entityPage, err)
	}
}

func TestKnowledgeUsageBuildersRepublishRemovesOldRows(t *testing.T) {
	d := testkit.Open(t)
	d.Reset(t)
	targets := makeKnowledgeTargets(t, d)
	pageID := d.MakePage(t, testkit.MainNamespaceID, "knowledge-republish", "Knowledge Republish", testkit.SystemActorID)
	blockID := d.NewID(t)
	raw := fmt.Sprintf(`{"type":"document","schema_version":1,"children":[
		{"id":%q,"type":"paragraph","content":[{"type":"entity_reference","entity_id":%q,"display_text":"旧引用"}]}
	]}`, blockID, targets.entityID)
	rev1 := appendASTRevision(t, d, pageID, nil, raw)
	rebuilder := projection.NewRebuilder(d.Pool, knowledgeRegistry(d), nil)
	if _, err := rebuilder.RebuildPage(context.Background(), pageID); err != nil {
		t.Fatal(err)
	}
	rev2 := appendASTRevision(t, d, pageID, &rev1, testAST)
	if _, err := rebuilder.RebuildPage(context.Background(), pageID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM entity_mention_projection WHERE page_id = $1`, pageID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("重发空文档后旧 mention 仍有 %d 行", count)
	}
	for _, typ := range []string{projection.ProjectionEntityMentions, projection.ProjectionClaimUsage, projection.ProjectionCitationUsage} {
		st := readState(t, d, typ, pageID)
		if st == nil || st.status != projection.StatusOK || st.sourceRevisionID != rev2 {
			t.Fatalf("%s state = %+v，期望 source rev2", typ, st)
		}
	}
}
