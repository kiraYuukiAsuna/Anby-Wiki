package collection_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/collection"
	"github.com/anby/wiki/backend/internal/knowledge"
	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/testkit"
)

func newService(tdb *testkit.DB) *collection.Service {
	return collection.NewService(
		collection.NewRepository(tdb.Pool),
		page.NewRepository(tdb.Pool),
		db.NewTxManager(tdb.Pool),
		id.NewGenerator(),
	)
}

func makeRevision(t *testing.T, tdb *testkit.DB, pageID, actorID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	snapshotID := tdb.NewID(t)
	revisionID := tdb.NewID(t)
	astJSON := `{"type":"document","schema_version":1,"children":[]}`
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO content_snapshot
		(id,content_hash,ast_json,schema_version,size_bytes)
		VALUES ($1,$2,$3::jsonb,1,$4)`,
		snapshotID, "sha256:"+snapshotID.String(), astJSON, len(astJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO revision
		(id,page_id,content_snapshot_id,actor_id,summary)
		VALUES ($1,$2,$3,$4,'collection source')`,
		revisionID, pageID, snapshotID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE page SET current_revision_id=$2 WHERE id=$1`,
		pageID, revisionID); err != nil {
		t.Fatal(err)
	}
	return revisionID
}

func newKnowledgeService(tdb *testkit.DB) *knowledge.Service {
	pageRepo := page.NewRepository(tdb.Pool)
	return knowledge.NewService(
		knowledge.NewRepository(tdb.Pool), pageRepo,
		db.NewTxManager(tdb.Pool), id.NewGenerator(),
	)
}

func TestParseRuleRejectsUnknownAndAmbiguousShapes(t *testing.T) {
	valid := []json.RawMessage{
		json.RawMessage(`{"version":1,"kind":"entity_type","entity_type":"character"}`),
		json.RawMessage(`{"version":1,"kind":"claim_exists","property":"release_date"}`),
	}
	for _, raw := range valid {
		if _, err := collection.ParseRule(raw); err != nil {
			t.Fatalf("valid rule %s: %v", raw, err)
		}
	}
	invalid := []json.RawMessage{
		json.RawMessage(``),
		json.RawMessage(`null`),
		json.RawMessage(`{"version":2,"kind":"entity_type","entity_type":"character"}`),
		json.RawMessage(`{"version":1,"kind":"dynamic","property":"release_date"}`),
		json.RawMessage(`{"version":1,"kind":"entity_type","entity_type":"character","property":"release_date"}`),
		json.RawMessage(`{"version":1,"kind":"claim_exists","property":"release_date","extra":true}`),
		json.RawMessage(`{"version":1,"kind":"entity_type","entity_type":"character"} {}`),
	}
	for _, raw := range invalid {
		if _, err := collection.ParseRule(raw); !errors.Is(err, collection.ErrInvalidRule) {
			t.Fatalf("invalid rule %s err=%v", raw, err)
		}
	}
}

func TestManualCollectionMixedMembersSourceRevisionAndPagination(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "collection-author")
	sourcePageID := tdb.MakePage(t, testkit.MainNamespaceID, "source", "Source", actorID)
	sourceRevisionID := makeRevision(t, tdb, sourcePageID, actorID)
	memberPageID := tdb.MakePage(t, testkit.MainNamespaceID, "member", "Member Page", actorID)
	knowledgeService := newKnowledgeService(tdb)
	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "character", CanonicalKey: "anby",
		Labels: []knowledge.LabelInput{{
			Language: "zh-Hans", Label: "安比", IsPrimary: true,
		}}, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := newService(tdb)
	value, err := service.Create(ctx, collection.CreateParams{
		WikiID: testkit.DefaultWikiID, CollectionType: collection.TypeManual,
		Title: "角色与页面", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ReplaceManualMembers(ctx, value.ID, actorID, []collection.MemberInput{
		{MemberType: collection.MemberEntity, EntityID: &entity.ID, SortKey: "01", SourceRevisionID: sourceRevisionID},
		{MemberType: collection.MemberPage, PageID: &memberPageID, SortKey: "02", SourceRevisionID: sourceRevisionID},
	}); err != nil {
		t.Fatal(err)
	}
	first, err := service.ListMembers(ctx, value.ID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].DisplayTitle != "安比" ||
		first.Items[0].SourceRevisionID != sourceRevisionID || first.NextCursor == nil {
		t.Fatalf("first=%+v", first)
	}
	second, err := service.ListMembers(ctx, value.ID, *first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].DisplayTitle != "Member Page" ||
		second.NextCursor != nil {
		t.Fatalf("second=%+v", second)
	}

	err = service.ReplaceManualMembers(ctx, value.ID, actorID, []collection.MemberInput{
		{MemberType: collection.MemberPage, PageID: &memberPageID, SortKey: "01", SourceRevisionID: sourceRevisionID},
		{MemberType: collection.MemberPage, PageID: &memberPageID, SortKey: "02", SourceRevisionID: sourceRevisionID},
	})
	if !errors.Is(err, collection.ErrInvalidMember) {
		t.Fatalf("duplicate err=%v", err)
	}
	after, err := service.ListMembers(ctx, value.ID, "", 10)
	if err != nil || len(after.Items) != 2 {
		t.Fatalf("atomic rollback after=%+v err=%v", after, err)
	}
}

func TestRuleCollectionValidationAndIdempotentRebuild(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "rule-author")
	sourcePageID := tdb.MakePage(t, testkit.MainNamespaceID, "rule-source", "Rule Source", actorID)
	sourceRevisionID := makeRevision(t, tdb, sourcePageID, actorID)
	knowledgeService := newKnowledgeService(tdb)
	character, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "character", CanonicalKey: "character-one",
		Labels:  []knowledge.LabelInput{{Language: "en", Label: "Character One", IsPrimary: true}},
		ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "person", CanonicalKey: "person-one",
		Labels:  []knowledge.LabelInput{{Language: "en", Label: "Person One", IsPrimary: true}},
		ActorID: actorID,
	}); err != nil {
		t.Fatal(err)
	}
	service := newService(tdb)
	if _, err := service.Create(ctx, collection.CreateParams{
		WikiID: testkit.DefaultWikiID, CollectionType: collection.TypeRule,
		Title: "Invalid", Rule: json.RawMessage(`{"version":1,"kind":"entity_type","entity_type":"missing"}`),
		ActorID: actorID,
	}); !errors.Is(err, collection.ErrInvalidRule) {
		t.Fatalf("unknown reference err=%v", err)
	}
	value, err := service.Create(ctx, collection.CreateParams{
		WikiID: testkit.DefaultWikiID, CollectionType: collection.TypeRule,
		Title: "Characters", Rule: json.RawMessage(`{"version":1,"kind":"entity_type","entity_type":"character"}`),
		ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		count, err := service.RebuildRule(ctx, value.ID, sourceRevisionID, actorID)
		if err != nil || count != 1 {
			t.Fatalf("rebuild count=%d err=%v", count, err)
		}
	}
	members, err := service.ListMembers(ctx, value.ID, "", 10)
	if err != nil || len(members.Items) != 1 ||
		members.Items[0].EntityID == nil || *members.Items[0].EntityID != character.ID ||
		members.Items[0].SourceType != collection.TypeRule {
		t.Fatalf("members=%+v err=%v", members, err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE entity SET status='deleted' WHERE id=$1`, character.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := service.RebuildRule(ctx, value.ID, sourceRevisionID, actorID); err != nil || count != 0 {
		t.Fatalf("rebuild after mismatch count=%d err=%v", count, err)
	}
	members, err = service.ListMembers(ctx, value.ID, "", 10)
	if err != nil || len(members.Items) != 0 {
		t.Fatalf("stale members were not removed: members=%+v err=%v", members, err)
	}
}

func TestManualCollectionRejectsWrongRevisionAndMemberShape(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "invalid-member-author")
	pageID := tdb.MakePage(t, testkit.MainNamespaceID, "member", "Member", actorID)
	service := newService(tdb)
	value, err := service.Create(ctx, collection.CreateParams{
		WikiID: testkit.DefaultWikiID, CollectionType: collection.TypeManual,
		Title: "Invalid members", ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	unknownRevision := tdb.NewID(t)
	err = service.ReplaceManualMembers(ctx, value.ID, actorID, []collection.MemberInput{{
		MemberType: collection.MemberPage, PageID: &pageID,
		SortKey: "01", SourceRevisionID: unknownRevision,
	}})
	if !errors.Is(err, collection.ErrInvalidMember) {
		t.Fatalf("unknown revision err=%v", err)
	}

	otherWikiID := tdb.NewID(t)
	otherNamespaceID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO wiki_site (id,site_key,name,default_language,settings_json)
		VALUES ($1,$2,'Other','en','{}'::jsonb)`,
		otherWikiID, "other-"+otherWikiID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `
		INSERT INTO namespace (id,wiki_id,namespace_key,canonical_name,display_name,is_content)
		VALUES ($1,$2,'main','Main','Main',true)`,
		otherNamespaceID, otherWikiID); err != nil {
		t.Fatal(err)
	}
	otherPageID := tdb.NewID(t)
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO page
		(id,wiki_id,namespace_id,normalized_title,display_title,language,content_model,status,created_by)
		VALUES ($1,$2,$3,'other','Other','en','block-v1','active',$4)`,
		otherPageID, otherWikiID, otherNamespaceID, actorID); err != nil {
		t.Fatal(err)
	}
	otherRevisionID := makeRevision(t, tdb, otherPageID, actorID)
	for name, input := range map[string]collection.MemberInput{
		"member": {
			MemberType: collection.MemberPage, PageID: &otherPageID,
			SortKey: "01", SourceRevisionID: otherRevisionID,
		},
		"source revision": {
			MemberType: collection.MemberPage, PageID: &pageID,
			SortKey: "01", SourceRevisionID: otherRevisionID,
		},
	} {
		if err := service.ReplaceManualMembers(ctx, value.ID, actorID, []collection.MemberInput{input}); !errors.Is(err, collection.ErrInvalidMember) {
			t.Fatalf("cross-wiki %s err=%v", name, err)
		}
	}
}

func TestClaimExistsRuleOnlyMaterializesPublishedClaims(t *testing.T) {
	tdb := testkit.Open(t)
	tdb.Reset(t)
	ctx := context.Background()
	actorID := tdb.MakeActor(t, "human", "claim-rule-author")
	sourcePageID := tdb.MakePage(t, testkit.MainNamespaceID, "claim-rule-source", "Claim Rule Source", actorID)
	sourceRevisionID := makeRevision(t, tdb, sourcePageID, actorID)
	knowledgeService := newKnowledgeService(tdb)
	entity, err := knowledgeService.CreateEntity(ctx, knowledge.CreateEntityParams{
		WikiID: testkit.DefaultWikiID, TypeKey: "character", CanonicalKey: "claim-character",
		Labels: []knowledge.LabelInput{{
			Language: "en", Label: "Claim Character", IsPrimary: true,
		}},
		ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := knowledgeService.CreateClaim(ctx, knowledge.CreateClaimParams{
		SubjectEntityID: entity.ID, PropertyKey: "release_date",
		Value: knowledge.DateValue("2026-07-24"), OriginType: knowledge.OriginHuman, ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := newService(tdb)
	value, err := service.Create(ctx, collection.CreateParams{
		WikiID: testkit.DefaultWikiID, CollectionType: collection.TypeRule,
		Title: "Released", Rule: json.RawMessage(`{"version":1,"kind":"claim_exists","property":"release_date"}`),
		ActorID: actorID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if count, err := service.RebuildRule(ctx, value.ID, sourceRevisionID, actorID); err != nil || count != 1 {
		t.Fatalf("published rebuild count=%d err=%v", count, err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE claim SET status='deprecated' WHERE id=$1`, claim.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := service.RebuildRule(ctx, value.ID, sourceRevisionID, actorID); err != nil || count != 0 {
		t.Fatalf("non-published rebuild count=%d err=%v", count, err)
	}
}
